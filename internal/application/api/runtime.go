package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kernul-io/cloudopt/internal/adapters/config"
	"github.com/kernul-io/cloudopt/internal/adapters/fixture"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

// Runtime implements operational commands for the CLI.
type Runtime struct {
	Settings           config.Settings
	lastAnalyze        *AnalyzeResult
	lastReport         *ports.ReportResult
	lastCollect        *CollectResult
	lastCostCollect    *CostCollectResult
	lastMetricsCollect *MetricsCollectResult
	lastReconcile      *ReconcileCostResult
	AnalyzerVersion    string
}

func NewRuntime(settings config.Settings) *Runtime {
	return &Runtime{Settings: settings}
}

func (r *Runtime) Init(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	dirs := []string{
		r.Settings.ConfigDir,
		r.Settings.DataDir,
		r.Settings.ReportsDir,
		r.Settings.TempDir,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
	}

	configPath := filepath.Join(r.Settings.ConfigDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config already exists at %q (refusing to overwrite)", configPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat config path: %w", err)
	}

	content := config.ExampleYAML(r.Settings.WorkspaceDir)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	db, err := OpenStorage(ctx, r.Settings)
	if err != nil {
		return fmt.Errorf("initialize storage: %w", err)
	}

	if err := db.Close(); err != nil {
		return fmt.Errorf("close storage: %w", err)
	}

	return nil
}

func (r *Runtime) Analyze(ctx context.Context, opts ports.AnalyzeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repo, err := OpenStorage(ctx, r.Settings)
	if err != nil {
		return err
	}
	defer repo.Close() //nolint: errcheck
	svc := &AnalyzeService{Repo: repo}
	result, err := svc.Analyze(ctx, AnalyzeSettings{
		ConfigDir:         r.Settings.ConfigDir,
		RulesManifestPath: r.Settings.RulesManifestPath,
		SuppressionsPath:  r.Settings.SuppressionsPath,
	}, opts)
	if err != nil {
		return err
	}
	r.lastAnalyze = result
	return nil
}

// LastAnalyzeResult returns the most recent analyze output for CLI emission.
func (r *Runtime) LastAnalyzeResult() *AnalyzeResult {
	return r.lastAnalyze
}

func (r *Runtime) Report(ctx context.Context, opts ports.ReportOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repo, err := OpenStorage(ctx, r.Settings)
	if err != nil {
		return err
	}
	defer repo.Close() //nolint: errcheck
	opts.AnalyzerVersion = r.AnalyzerVersion
	if opts.AnalyzerVersion == "" {
		opts.AnalyzerVersion = "dev"
	}
	svc := &ReportService{Repo: repo, ReportsDir: r.Settings.ReportsDir}
	result, err := svc.Generate(ctx, AnalyzeSettings{
		ConfigDir:         r.Settings.ConfigDir,
		RulesManifestPath: r.Settings.RulesManifestPath,
		SuppressionsPath:  r.Settings.SuppressionsPath,
	}, opts)
	if err != nil {
		return err
	}
	r.lastReport = result
	return nil
}

// LastReportResult returns the most recent report output for CLI emission.
func (r *Runtime) LastReportResult() *ports.ReportResult {
	return r.lastReport
}

func (r *Runtime) ImportFixture(ctx context.Context, path string) (types.SnapshotID, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	repo, err := OpenStorage(ctx, r.Settings)
	if err != nil {
		return "", err
	}
	defer repo.Close() //nolint: errcheck
	importer := fixture.NewImporter(repo)
	return importer.Import(ctx, path)
}

// NotImplementedError indicates a command contract exists but behavior is deferred.
type NotImplementedError struct {
	Command string
}

func (e *NotImplementedError) Error() string {
	return fmt.Sprintf("%q is not implemented yet", e.Command)
}

func ErrNotImplemented(command string) error {
	return &NotImplementedError{Command: command}
}

func IsNotImplemented(err error) bool {
	var ni *NotImplementedError
	return errors.As(err, &ni)
}
