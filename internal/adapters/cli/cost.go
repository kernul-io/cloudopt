package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/kernul-io/cloudopt/internal/adapters/logging"
	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func runCollectCost(cmd *cobra.Command, cfg *Config, opts ports.CostCollectOptions) error {
	settings, err := LoadSettings(cfg.Overrides)
	if err != nil {
		_ = EmitError("collect", err.Error())
		return &ExitError{Code: ExitCode(err), Err: err}
	}
	logger := logging.New(settings.LogFormat, settings.LogLevel)
	rt := api.NewRuntime(settings)
	runner := &Runner{Logger: logger, Run: rt}
	opts.Provider = types.ProviderAWS
	opts.Progress = &stderrProgress{Logger: logger}

	var out *ports.CostCollectResult
	runErr := runner.Execute(cmd.Context(), func(ctx context.Context) error {
		res, err := rt.CollectCost(ctx, opts)
		if err != nil {
			return err
		}
		out = res
		return nil
	})
	if runErr != nil {
		code := CollectionExitCode(runErr)
		_ = EmitError("collect", runErr.Error())
		return &ExitError{Code: code, Err: runErr}
	}
	_ = EmitCostCollectResult(api.MapCostCollectResult(out))
	return nil
}

func newCostReconcileCommand(cfg *Config) *cobra.Command {
	var snapshotID string
	cmd := &cobra.Command{
		Use:   "cost-reconcile",
		Short: "Show billing source vs attributed vs unattributed totals for a snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := LoadSettings(cfg.Overrides)
			if err != nil {
				_ = EmitError("cost-reconcile", err.Error())
				return &ExitError{Code: ExitCode(err), Err: err}
			}
			logger := logging.New(settings.LogFormat, settings.LogLevel)
			rt := api.NewRuntime(settings)
			runner := &Runner{Logger: logger, Run: rt}
			var res *api.ReconcileCostResult
			runErr := runner.Execute(cmd.Context(), func(ctx context.Context) error {
				if snapshotID == "" {
					repo, err := api.OpenStorage(ctx, settings)
					if err != nil {
						return err
					}
					defer repo.Close() //nolint: errcheck
					list, err := repo.ListSnapshots(ctx, ports.ListSnapshotFilter{Limit: 1})
					if err != nil {
						return err
					}
					if len(list) == 0 {
						return errNoSnapshot
					}
					snapshotID = string(list[0].ID)
				}
				var err error
				res, err = rt.ReconcileCost(ctx, snapshotID)
				return err
			})
			if runErr != nil {
				_ = EmitError("cost-reconcile", runErr.Error())
				return &ExitError{Code: ExitCode(runErr), Err: runErr}
			}
			return EmitCostReconcileResult(res)
		},
	}
	cmd.Flags().StringVar(&snapshotID, "snapshot-id", "", "Snapshot to reconcile (default: latest)")
	return cmd
}

var errNoSnapshot = errSnapshotNotFound{}

type errSnapshotNotFound struct{}

func (errSnapshotNotFound) Error() string { return "no snapshot found" }
