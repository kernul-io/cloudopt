package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kernul-io/cloudopt/internal/adapters/logging"
	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

func runReport(cmd *cobra.Command, cfg *Config, opts ports.ReportOptions) error {
	settings, err := LoadSettings(cfg.Overrides)
	if err != nil {
		_ = EmitError("report", err.Error())
		return &ExitError{Code: ExitCode(err), Err: err}
	}

	logger := logging.New(settings.LogFormat, settings.LogLevel)
	rt := api.NewRuntime(settings)
	rt.AnalyzerVersion = Version
	runner := &Runner{Logger: logger, Run: rt}

	if err := runner.Execute(cmd.Context(), func(ctx context.Context) error {
		return rt.Report(ctx, opts)
	}); err != nil {
		code := ReportExitCode(err)
		_ = EmitError("report", err.Error())
		return &ExitError{Code: code, Err: err}
	}

	result := rt.LastReportResult()
	if result == nil {
		return &ExitError{Code: ReportExitCode(fmt.Errorf("missing report result")), Err: fmt.Errorf("missing report result")}
	}

	if opts.Format == ports.ReportJSON {
		if err := EmitReportResult(result); err != nil {
			return &ExitError{Code: ExitCode(err), Err: err}
		}
		return nil
	}

	msg := fmt.Sprintf("path=%s format=%s analysis_run=%s snapshot=%s",
		result.DocumentPath, result.Format, result.AnalysisRunID, result.SnapshotID)
	_ = EmitOK("report", msg)
	return nil
}

func parseReportFormat(s string) (ports.ReportFormat, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "html", "":
		return ports.ReportHTML, nil
	case "json":
		return ports.ReportJSON, nil
	default:
		return "", fmt.Errorf("unsupported report format %q (use html or json)", s)
	}
}

func runImportFixture(cmd *cobra.Command, cfg *Config, fixturePath string) error {
	settings, err := LoadSettings(cfg.Overrides)
	if err != nil {
		_ = EmitError("import-fixture", err.Error())
		return &ExitError{Code: ExitCode(err), Err: err}
	}

	logger := logging.New(settings.LogFormat, settings.LogLevel)
	rt := api.NewRuntime(settings)
	runner := &Runner{Logger: logger, Run: rt}

	var snapID types.SnapshotID
	if err := runner.Execute(cmd.Context(), func(ctx context.Context) error {
		var importErr error
		snapID, importErr = rt.ImportFixture(ctx, fixturePath)
		return importErr
	}); err != nil {
		_ = EmitError("import-fixture", err.Error())
		return &ExitError{Code: ExitCode(err), Err: err}
	}

	_ = EmitOK("import-fixture", fmt.Sprintf("snapshot_id=%s", snapID))
	return nil
}
