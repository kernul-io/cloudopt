package cli

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/kernul-io/cloudopt/internal/adapters/logging"
	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/exitcodes"
)

func newCollectCommand(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Collect read-only snapshots from configured cloud accounts",
	}
	cmd.AddCommand(newCollectAWSCommand(cfg))

	var provider string
	cmd.PersistentFlags().StringVar(&provider, "provider", "", "Limit collection to one provider (aws, gcp, azure, digitalocean)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if provider == "" {
			return cmd.Help()
		}
		if provider != "aws" {
			err := api.ErrNotImplemented("collect:" + provider)
			_ = EmitNotImplemented("collect", err.Error())
			return &ExitError{Code: exitcodes.Success, Err: err}
		}
		opts := ports.CollectOptions{Provider: types.ProviderAWS, Offline: true}
		opts.Offline, _ = cmd.Flags().GetBool("offline")
		if !opts.Offline {
			opts.Offline = true // default legacy collect --provider aws to offline demo unless live creds used via aws inventory
		}
		opts.DryRun, _ = cmd.Flags().GetBool("dry-run")
		return runCollect(cmd, cfg, opts)
	}
	cmd.PersistentFlags().Bool("offline", true, "Use offline fixtures (with --provider aws)")
	cmd.PersistentFlags().Bool("dry-run", false, "Preflight only (with --provider aws)")
	return cmd
}

func newCollectAWSCommand(cfg *Config) *cobra.Command {
	var opts ports.CollectOptions
	var costOpts ports.CostCollectOptions
	cmd := &cobra.Command{
		Use:   "aws",
		Short: "Amazon Web Services collection commands",
	}
	inventory := &cobra.Command{
		Use:   "inventory",
		Short: "Collect read-only AWS inventory into a canonical snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Provider = types.ProviderAWS
			return runCollect(cmd, cfg, opts)
		},
	}
	bindAWSInventoryFlags(inventory, &opts)
	cost := &cobra.Command{
		Use:   "cost",
		Short: "Collect AWS billing costs and attribute them to inventory resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCollectCost(cmd, cfg, costOpts)
		},
	}
	bindAWSCostFlags(cost, &costOpts)
	metrics := newMetricsCommand(cfg)
	cmd.AddCommand(inventory)
	cmd.AddCommand(cost)
	cmd.AddCommand(metrics)
	return cmd
}

func bindAWSCostFlags(cmd *cobra.Command, opts *ports.CostCollectOptions) {
	cmd.Flags().StringVar((*string)(&opts.AccountID), "account-id", "", "Canonical account id filter when selecting latest snapshot")
	cmd.Flags().StringVar(&opts.RoleARN, "role-arn", "", "Optional IAM role ARN to assume")
	cmd.Flags().StringVar(&opts.ExternalID, "external-id", "", "External ID for role assumption")
	cmd.Flags().StringVar((*string)(&opts.SnapshotID), "snapshot-id", "", "Snapshot to attach costs to (default: latest)")
	cmd.Flags().IntVar(&opts.LookbackDays, "lookback-days", 30, "Billing lookback window in days")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preflight billing access without collecting")
	cmd.Flags().BoolVar(&opts.Offline, "offline", false, "Use recorded Cost Explorer fixtures")
	cmd.Flags().StringVar(&opts.FixtureRoot, "fixture-root", "", "Fixture directory for --offline (default testdata/aws-billing)")
}

func bindAWSInventoryFlags(cmd *cobra.Command, opts *ports.CollectOptions) {
	cmd.Flags().StringVar((*string)(&opts.AccountID), "account-id", "", "Canonical account id (default derived from caller identity)")
	cmd.Flags().StringVar(&opts.RoleARN, "role-arn", "", "Optional IAM role ARN to assume")
	cmd.Flags().StringVar(&opts.ExternalID, "external-id", "", "External ID for role assumption")
	cmd.Flags().StringSliceVar(&opts.Regions, "regions", nil, "Explicit regions to scan")
	cmd.Flags().StringSliceVar(&opts.RegionsAllow, "regions-allow", nil, "Allow-list regions")
	cmd.Flags().StringSliceVar(&opts.RegionsDeny, "regions-deny", nil, "Deny-list regions")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Authenticate and show scope without collecting resources")
	cmd.Flags().BoolVar(&opts.Offline, "offline", false, "Use recorded fixtures instead of live AWS APIs")
	cmd.Flags().StringVar(&opts.FixtureRoot, "fixture-root", "", "Fixture directory for --offline (default testdata/aws-inventory)")
	cmd.Flags().IntVar(&opts.MaxConcurrent, "max-concurrent-regions", 3, "Maximum concurrent regions")
}

func runCollect(cmd *cobra.Command, cfg *Config, opts ports.CollectOptions) error {
	settings, err := LoadSettings(cfg.Overrides)
	if err != nil {
		_ = EmitError("collect", err.Error())
		return &ExitError{Code: ExitCode(err), Err: err}
	}
	logger := logging.New(settings.LogFormat, settings.LogLevel)
	rt := api.NewRuntime(settings)
	runner := &Runner{Logger: logger, Run: rt}
	opts.Progress = &stderrProgress{Logger: logger}

	var collectRes *api.CollectResult
	runErr := runner.Execute(cmd.Context(), func(ctx context.Context) error {
		out, err := rt.Collect(ctx, opts)
		if err != nil {
			return err
		}
		collectRes = api.MapCollectResult(out)
		return nil
	})
	if runErr != nil {
		code := CollectionExitCode(runErr)
		_ = EmitError("collect", runErr.Error())
		return &ExitError{Code: code, Err: runErr}
	}
	if collectRes != nil {
		_ = EmitCollectResult(collectRes)
	} else {
		_ = EmitOK("collect", "")
	}
	return nil
}

type stderrProgress struct {
	Logger zerolog.Logger
}

func (p *stderrProgress) Step(message string) {
	p.Logger.Info().Str("phase", "collect").Msg(message)
}
