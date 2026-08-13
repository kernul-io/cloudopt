package cli

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/kernul-io/cloudopt/internal/adapters/config"
	"github.com/kernul-io/cloudopt/internal/adapters/logging"
	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/exitcodes"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// Config holds persistent CLI flags mapped to config overrides.
type Config struct {
	Overrides config.Overrides
}

// NewRootCommand builds the cobra command tree.
func NewRootCommand(cfg *Config) *cobra.Command {
	root := &cobra.Command{
		Use:           "cloudopt",
		Short:         "Cloud Optimization Analyzer — read-only multi-cloud cost and utilization analysis",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&cfg.Overrides.WorkspaceDir, "workspace-dir", "", "Local workspace root (overrides COA_WORKSPACE_DIR and config file)")
	root.PersistentFlags().StringVar(&cfg.Overrides.ConfigDir, "config-dir", "", "Configuration directory")
	root.PersistentFlags().StringVar(&cfg.Overrides.DataDir, "data-dir", "", "SQLite and snapshot data directory")
	root.PersistentFlags().StringVar(&cfg.Overrides.ReportsDir, "reports-dir", "", "Generated reports directory")
	root.PersistentFlags().StringVar(&cfg.Overrides.TempDir, "temp-dir", "", "Temporary files directory")
	root.PersistentFlags().StringVar(&cfg.Overrides.LogFormat, "log-format", "", "Log format: text or json (stderr)")
	root.PersistentFlags().StringVar(&cfg.Overrides.LogLevel, "log-level", "", "Log level: debug, info, warn, error")
	root.PersistentFlags().StringVar(&cfg.Overrides.ConfigFile, "config", "", "Path to config file")

	root.AddCommand(newVersionCommand())
	root.AddCommand(newInitCommand(cfg))
	root.AddCommand(newCollectCommand(cfg))
	root.AddCommand(newAnalyzeCommand(cfg))
	root.AddCommand(newReportCommand(cfg))
	root.AddCommand(newImportFixtureCommand(cfg))

	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information (JSON on stdout)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return EmitVersion(Version)
		},
	}
}

func newInitCommand(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create local workspace directories and default config (never overwrites existing config)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOperational(cmd, cfg, "init", (*api.Runtime).Init)
		},
	}
}

func newAnalyzeCommand(cfg *Config) *cobra.Command {
	var snapshotID, rulesCSV, categoriesCSV string
	var jsonOut, allowPartial bool
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Run deterministic optimization rules on collected data",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := api.AnalyzeOptions{
				SnapshotID:           types.SnapshotID(snapshotID),
				Persist:              true,
				JSONDetail:           jsonOut,
				AllowPartialSnapshot: allowPartial,
			}
			if rulesCSV != "" {
				opts.RuleIDs = splitCSV(rulesCSV)
			}
			if categoriesCSV != "" {
				opts.Categories = splitCSV(categoriesCSV)
			}
			return runAnalyze(cmd, cfg, opts)
		},
	}
	cmd.Flags().StringVar(&snapshotID, "snapshot-id", "", "Snapshot to analyze (default: latest complete)")
	cmd.Flags().StringVar(&rulesCSV, "rules", "", "Comma-separated rule IDs to run")
	cmd.Flags().StringVar(&categoriesCSV, "category", "", "Comma-separated rule categories to run")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit full analysis JSON on stdout")
	cmd.Flags().BoolVar(&allowPartial, "allow-partial-snapshot", false, "Allow analysis on partial inventory snapshots")
	return cmd
}

func newReportCommand(cfg *Config) *cobra.Command {
	var format, output, analysisRunID string
	var redact bool
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate a shareable report from analysis results",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := parseReportFormat(format)
			if err != nil {
				_ = EmitError("report", err.Error())
				return &ExitError{Code: exitcodes.InvalidInput, Err: err}
			}
			if jsonOut {
				f = ports.ReportJSON
			}
			opts := ports.ReportOptions{
				AnalysisRunID:     types.AnalysisRunID(analysisRunID),
				RedactIdentifiers: redact,
				Format:            f,
				OutputPath:        output,
			}
			return runReport(cmd, cfg, opts)
		},
	}
	cmd.Flags().StringVar(&format, "format", "html", "Report format: html or json")
	cmd.Flags().StringVar(&output, "output", "", "Output file path (default: reports dir)")
	cmd.Flags().StringVar(&analysisRunID, "analysis-run-id", "", "Analysis run to report (default: latest)")
	cmd.Flags().BoolVar(&redact, "redact", false, "Redact account and resource identifiers using stable aliases")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Write JSON report and emit result metadata on stdout")
	return cmd
}

func newImportFixtureCommand(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import-fixture",
		Short: "Import offline fixture YAML into local storage",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImportFixture(cmd, cfg, args[0])
		},
	}
	return cmd
}

type runtimeOp func(*api.Runtime, context.Context) error

func runOperational(cmd *cobra.Command, cfg *Config, name string, op runtimeOp) error {
	settings, err := LoadSettings(cfg.Overrides)
	if err != nil {
		_ = EmitError(name, err.Error())
		return &ExitError{Code: ExitCode(err), Err: err}
	}

	logger := logging.New(settings.LogFormat, settings.LogLevel)
	rt := api.NewRuntime(settings)
	runner := &Runner{Logger: logger, Run: rt}

	var runErr error
	if err := runner.Execute(cmd.Context(), func(ctx context.Context) error {
		runErr = op(rt, ctx)
		if runErr != nil && api.IsNotImplemented(runErr) {
			return nil
		}
		return runErr
	}); err != nil {
		code := mapCommandExit(name, err)
		_ = EmitError(name, err.Error())
		return &ExitError{Code: code, Err: err}
	}

	code := mapCommandExit(name, runErr)
	handleEmit(name, runErr)
	if code != exitcodes.Success {
		return &ExitError{Code: code, Err: runErr}
	}
	return nil
}

func mapCommandExit(command string, err error) int {
	if err == nil {
		return exitcodes.Success
	}
	switch command {
	case "collect":
		return CollectionExitCode(err)
	case "analyze":
		return AnalysisExitCode(err)
	case "report":
		return ReportExitCode(err)
	default:
		return ExitCode(err)
	}
}

func handleEmit(command string, err error) {
	if err == nil {
		_ = EmitOK(command, "")
		return
	}
	if api.IsNotImplemented(err) {
		_ = EmitNotImplemented(command, err.Error())
		return
	}
	_ = EmitError(command, err.Error())
}

// Execute runs the CLI and returns the process exit code.
func Execute(cfg *Config) int {
	root := NewRootCommand(cfg)
	if err := root.Execute(); err != nil {
		var ee *ExitError
		if errors.As(err, &ee) {
			return ee.Code
		}
		_ = EmitError("", err.Error())
		return ExitCode(err)
	}
	return exitcodes.Success
}
