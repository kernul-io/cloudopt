package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kernul-io/cloudopt/internal/adapters/logging"
	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

func runAnalyze(cmd *cobra.Command, cfg *Config, opts ports.AnalyzeOptions) error {
	settings, err := LoadSettings(cfg.Overrides)
	if err != nil {
		_ = EmitError("analyze", err.Error())
		return &ExitError{Code: ExitCode(err), Err: err}
	}

	logger := logging.New(settings.LogFormat, settings.LogLevel)
	rt := api.NewRuntime(settings)
	runner := &Runner{Logger: logger, Run: rt}

	if err := runner.Execute(cmd.Context(), func(ctx context.Context) error {
		return rt.Analyze(ctx, opts)
	}); err != nil {
		code := AnalysisExitCode(err)
		_ = EmitError("analyze", err.Error())
		return &ExitError{Code: code, Err: err}
	}

	result := rt.LastAnalyzeResult()
	if result == nil {
		return &ExitError{Code: AnalysisExitCode(fmt.Errorf("missing analyze result")), Err: fmt.Errorf("missing analyze result")}
	}

	if opts.JSONDetail {
		if err := EmitAnalyzeResult(result); err != nil {
			return &ExitError{Code: ExitCode(err), Err: err}
		}
		return nil
	}

	msg := fmt.Sprintf(
		"snapshot=%s ruleset=%s passed=%d failed=%d suppressed=%d not_evaluated=%d errors=%d findings=%d",
		result.SnapshotID,
		result.RulesetVersion,
		result.Summary.Passed,
		result.Summary.Failed,
		result.Summary.Suppressed,
		result.Summary.NotEvaluated,
		result.Summary.Errors,
		len(result.Findings),
	)
	_ = EmitOK("analyze", msg)
	return nil
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
