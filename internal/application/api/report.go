package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/application/report"
	"github.com/kernul-io/cloudopt/internal/application/rules"
)

// ReportService builds consultant reports from persisted analysis runs.
type ReportService struct {
	Repo       ports.StorageRepository
	ReportsDir string
}

// Generate loads analysis data, rebuilds rule execution summary, and writes the report file.
func (s *ReportService) Generate(ctx context.Context, settings AnalyzeSettings, opts ports.ReportOptions) (*ports.ReportResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	run, err := s.resolveRun(ctx, opts.AnalysisRunID)
	if err != nil {
		return nil, err
	}
	snap, err := s.Repo.GetSnapshot(ctx, run.SnapshotID)
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}

	reg := rules.DefaultRegistry()
	manifestPath := settings.RulesManifestPath
	if manifestPath == "" {
		manifestPath = os.Getenv("COA_RULES_MANIFEST")
	}
	manifest, err := rules.LoadManifest(manifestPath, reg)
	if err != nil {
		return nil, err
	}
	suppPath := settings.SuppressionsPath
	if suppPath == "" {
		suppPath = rules.DefaultSuppressionsPath(settings.ConfigDir)
	}
	suppEntries, err := rules.LoadSuppressions(suppPath)
	if err != nil {
		return nil, err
	}
	refTime := snap.StartedAt.Time
	if run.CompletedAt != nil {
		refTime = run.CompletedAt.Time
	}
	suppIndex := rules.NewSuppressionIndex(suppEntries, refTime)

	engine := rules.Engine{}
	out, err := engine.Analyze(rules.AnalyzeInput{
		Snapshot:     snap,
		Manifest:     manifest,
		Registry:     reg,
		Suppressions: suppIndex,
	})
	if err != nil {
		return nil, fmt.Errorf("rule execution summary: %w", err)
	}

	meta, err := report.LoadMetadata(settings.ConfigDir)
	if err != nil {
		return nil, err
	}

	doc, err := report.Build(report.BuildInput{
		AnalyzerVersion: opts.AnalyzerVersion,
		GeneratedAt:     time.Now().UTC(),
		Metadata:        meta,
		Snapshot:        snap,
		Run:             run,
		Executions:      out.Executions,
		Redact:          opts.RedactIdentifiers,
	})
	if err != nil {
		return nil, err
	}
	if err := report.ValidateDocument(doc); err != nil {
		return nil, err
	}

	format := opts.Format
	if format == "" {
		format = ports.ReportHTML
	}
	reportsDir := s.ReportsDir
	outPath, err := resolveReportPath(opts.OutputPath, reportsDir, run, format)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o750); err != nil {
		return nil, fmt.Errorf("create reports directory: %w", err)
	}

	switch format {
	case ports.ReportJSON:
		data, err := report.ToJSON(doc)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(outPath, data, 0o600); err != nil {
			return nil, fmt.Errorf("write report: %w", err)
		}
	case ports.ReportHTML:
		html, err := report.RenderHTML(doc)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(outPath, []byte(html), 0o600); err != nil {
			return nil, fmt.Errorf("write report: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported report format %q", format)
	}

	return &ports.ReportResult{
		DocumentPath:  outPath,
		Format:        format,
		AnalysisRunID: run.ID,
		SnapshotID:    run.SnapshotID,
	}, nil
}

func (s *ReportService) resolveRun(ctx context.Context, id types.AnalysisRunID) (*domain.AnalysisRun, error) {
	if id != "" {
		return s.Repo.GetAnalysisRun(ctx, id)
	}
	run, err := s.Repo.GetLatestAnalysisRun(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("no analysis run found; run cloudopt analyze first: %w", err)
	}
	return run, nil
}

func resolveReportPath(outputPath, reportsDir string, run *domain.AnalysisRun, format ports.ReportFormat) (string, error) {
	if outputPath != "" {
		return outputPath, nil
	}
	if reportsDir == "" {
		return "", fmt.Errorf("reports directory is not configured")
	}
	ext := ".html"
	if format == ports.ReportJSON {
		ext = ".json"
	}
	base := strings.ReplaceAll(string(run.ID), "/", "-")
	name := fmt.Sprintf("report-%s%s", base, ext)
	return filepath.Join(reportsDir, name), nil
}
