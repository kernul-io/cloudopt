package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/kernul-io/cloudopt/internal/adapters/logging"
	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func newMetricsCommand(cfg *Config) *cobra.Command {
	var opts ports.MetricsCollectOptions
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Collect AWS CloudWatch utilization metrics for a snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Provider = types.ProviderAWS
			return runCollectMetrics(cmd, cfg, opts)
		},
	}
	bindAWSMetricsFlags(cmd, &opts)
	return cmd
}

func bindAWSMetricsFlags(cmd *cobra.Command, opts *ports.MetricsCollectOptions) {
	cmd.Flags().StringVar((*string)(&opts.AccountID), "account-id", "", "Canonical account id filter when selecting latest snapshot")
	cmd.Flags().StringVar(&opts.RoleARN, "role-arn", "", "Optional IAM role ARN to assume")
	cmd.Flags().StringVar(&opts.ExternalID, "external-id", "", "External ID for role assumption")
	cmd.Flags().StringVar((*string)(&opts.SnapshotID), "snapshot-id", "", "Snapshot to attach metrics to (default: latest)")
	cmd.Flags().IntVar(&opts.LookbackDays, "lookback-days", 14, "Metrics lookback window in days")
	cmd.Flags().IntVar(&opts.PeriodSeconds, "period-seconds", 3600, "CloudWatch aggregation period in seconds")
	cmd.Flags().StringVar(&opts.TimeZone, "timezone", "UTC", "Business-hours IANA time zone")
	cmd.Flags().IntVar(&opts.BusinessHourStart, "business-hour-start", 9, "Business hours start (local hour, inclusive)")
	cmd.Flags().IntVar(&opts.BusinessHourEnd, "business-hour-end", 17, "Business hours end (local hour, exclusive)")
	cmd.Flags().IntVar(&opts.MaxConcurrent, "max-concurrent", 5, "Maximum concurrent CloudWatch batch workers")
	cmd.Flags().IntVar(&opts.MaxDatapoints, "max-datapoints", 1440, "Maximum datapoints per series")
	cmd.Flags().IntVar(&opts.MaxAPIRequests, "max-api-requests", 100, "Maximum CloudWatch API requests per run")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preflight metrics access without collecting")
	cmd.Flags().BoolVar(&opts.Offline, "offline", false, "Use recorded CloudWatch fixtures")
	cmd.Flags().StringVar(&opts.FixtureRoot, "fixture-root", "", "Fixture directory for --offline (default testdata/aws-metrics)")
}

func runCollectMetrics(cmd *cobra.Command, cfg *Config, opts ports.MetricsCollectOptions) error {
	settings, err := LoadSettings(cfg.Overrides)
	if err != nil {
		_ = EmitError("collect", err.Error())
		return &ExitError{Code: ExitCode(err), Err: err}
	}
	logger := logging.New(settings.LogFormat, settings.LogLevel)
	rt := api.NewRuntime(settings)
	runner := &Runner{Logger: logger, Run: rt}
	opts.Progress = &stderrProgress{Logger: logger}

	var metricsRes *api.MetricsCollectResult
	runErr := runner.Execute(cmd.Context(), func(ctx context.Context) error {
		out, err := rt.CollectMetrics(ctx, opts)
		if err != nil {
			return err
		}
		metricsRes = api.MapMetricsCollectResult(out)
		return nil
	})
	if runErr != nil {
		code := CollectionExitCode(runErr)
		_ = EmitError("collect", runErr.Error())
		return &ExitError{Code: code, Err: runErr}
	}
	if metricsRes != nil {
		_ = EmitMetricsCollectResult(metricsRes)
	} else {
		_ = EmitOK("collect", "")
	}
	return nil
}
