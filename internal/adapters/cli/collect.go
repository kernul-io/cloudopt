package cli

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/kernul-io/cloudopt/internal/adapters/logging"
	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/exitcodes"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func newCollectCommand(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Collect read-only snapshots from configured cloud accounts",
	}
	cmd.AddCommand(newCollectAWSCommand(cfg))
	cmd.AddCommand(newCollectGCPCommand(cfg))

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

func newCollectGCPCommand(cfg *Config) *cobra.Command {
	var opts ports.CollectOptions
	var costOpts ports.CostCollectOptions
	var metricsOpts ports.MetricsCollectOptions
	cmd := &cobra.Command{
		Use:   "gcp",
		Short: "Google Cloud Platform collection commands",
	}
	inventory := &cobra.Command{
		Use:   "inventory",
		Short: "Collect read-only GCP inventory into a canonical snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Provider = types.ProviderGCP
			return runCollect(cmd, cfg, opts)
		},
	}
	bindGCPInventoryFlags(inventory, &opts)
	cost := &cobra.Command{
		Use:   "cost",
		Short: "Collect GCP billing from BigQuery export and attribute to inventory",
		RunE: func(cmd *cobra.Command, args []string) error {
			costOpts.Provider = types.ProviderGCP
			return runCollectCost(cmd, cfg, costOpts)
		},
	}
	bindGCPCostFlags(cost, &costOpts)
	metrics := &cobra.Command{
		Use:   "metrics",
		Short: "Collect GCP Cloud Monitoring utilization metrics for a snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			metricsOpts.Provider = types.ProviderGCP
			return runCollectMetrics(cmd, cfg, metricsOpts)
		},
	}
	bindGCPMetricsFlags(metrics, &metricsOpts)
	cmd.AddCommand(inventory)
	cmd.AddCommand(cost)
	cmd.AddCommand(metrics)
	return cmd
}

func bindGCPCostFlags(cmd *cobra.Command, opts *ports.CostCollectOptions) {
	cmd.Flags().StringVar((*string)(&opts.AccountID), "account-id", "", "Canonical account id filter when selecting latest snapshot")
	cmd.Flags().StringVar((*string)(&opts.SnapshotID), "snapshot-id", "", "Snapshot to attach costs to (default: latest)")
	cmd.Flags().IntVar(&opts.LookbackDays, "lookback-days", 30, "Billing lookback window in days")
	cmd.Flags().StringVar(&opts.BillingExportProject, "billing-export-project", "", "Project containing the BigQuery billing export dataset")
	cmd.Flags().StringVar(&opts.BigQueryDataset, "bigquery-dataset", "", "BigQuery dataset for billing export")
	cmd.Flags().StringVar(&opts.BigQueryTable, "bigquery-table", "", "BigQuery table for billing export")
	cmd.Flags().StringVar(&opts.ImpersonateServiceAccount, "impersonate-service-account", "", "Service account to impersonate for billing export read")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preflight billing access without collecting")
	cmd.Flags().BoolVar(&opts.Offline, "offline", false, "Use recorded BigQuery export fixtures")
	cmd.Flags().StringVar(&opts.FixtureRoot, "fixture-root", "", "Fixture directory for --offline (default testdata/gcp-billing)")
}

func bindGCPMetricsFlags(cmd *cobra.Command, opts *ports.MetricsCollectOptions) {
	cmd.Flags().StringVar((*string)(&opts.AccountID), "account-id", "", "Canonical account id filter when selecting latest snapshot")
	cmd.Flags().StringVar((*string)(&opts.SnapshotID), "snapshot-id", "", "Snapshot to attach metrics to (default: latest)")
	cmd.Flags().IntVar(&opts.LookbackDays, "lookback-days", 14, "Metrics lookback window in days")
	cmd.Flags().IntVar(&opts.PeriodSeconds, "period-seconds", 3600, "Monitoring alignment period in seconds")
	cmd.Flags().StringVar(&opts.TimeZone, "timezone", "UTC", "Business-hours IANA time zone")
	cmd.Flags().IntVar(&opts.BusinessHourStart, "business-hour-start", 9, "Business hours start (local hour, inclusive)")
	cmd.Flags().IntVar(&opts.BusinessHourEnd, "business-hour-end", 17, "Business hours end (local hour, exclusive)")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preflight metrics access without collecting")
	cmd.Flags().BoolVar(&opts.Offline, "offline", false, "Use recorded Monitoring fixtures")
	cmd.Flags().StringVar(&opts.FixtureRoot, "fixture-root", "", "Fixture directory for --offline (default testdata/gcp-metrics)")
}

func bindGCPInventoryFlags(cmd *cobra.Command, opts *ports.CollectOptions) {
	cmd.Flags().StringVar((*string)(&opts.AccountID), "account-id", "", "Canonical account id (default derived from ADC project)")
	cmd.Flags().StringVar(&opts.OrganizationID, "organization-id", "", "GCP organization ID for project discovery")
	cmd.Flags().StringVar(&opts.FolderID, "folder-id", "", "GCP folder ID for project discovery")
	cmd.Flags().StringSliceVar(&opts.Projects, "projects", nil, "Explicit GCP project IDs to scan")
	cmd.Flags().StringSliceVar(&opts.Regions, "regions", nil, "Explicit regions to scan")
	cmd.Flags().StringSliceVar(&opts.RegionsAllow, "regions-allow", nil, "Allow-list regions")
	cmd.Flags().StringSliceVar(&opts.RegionsDeny, "regions-deny", nil, "Deny-list regions")
	cmd.Flags().StringSliceVar(&opts.Zones, "zones", nil, "Explicit zones to scan (Compute zonal resources)")
	cmd.Flags().StringVar(&opts.ImpersonateServiceAccount, "impersonate-service-account", "", "Service account email to impersonate (ADC source credentials required)")
	cmd.Flags().StringVar(&opts.BillingAccountID, "billing-account-id", "", "Billing account ID for preflight scope reporting only")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preflight access and scope without collecting resources")
	cmd.Flags().BoolVar(&opts.Offline, "offline", false, "Use recorded fixtures instead of live GCP APIs")
	cmd.Flags().StringVar(&opts.FixtureRoot, "fixture-root", "", "Fixture directory for --offline (default testdata/gcp-inventory)")
	cmd.Flags().IntVar(&opts.MaxConcurrent, "max-concurrent", 3, "Maximum concurrent project/region workers")
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
			costOpts.Provider = types.ProviderAWS
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
