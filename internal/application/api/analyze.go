package api

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/application/rules"
)

// AnalyzeOptions configures a single analyze invocation.
type AnalyzeOptions = ports.AnalyzeOptions

// AnalyzeResult is the machine-readable analyze payload for CLI output.
type AnalyzeResult struct {
	AnalysisRunID  types.AnalysisRunID   `json:"analysis_run_id,omitempty"`
	SnapshotID     types.SnapshotID      `json:"snapshot_id"`
	RulesetVersion string                `json:"ruleset_version"`
	Summary        rules.Summary         `json:"summary"`
	Findings       []domain.Finding      `json:"findings,omitempty"`
	RuleExecutions []rules.RuleExecution `json:"rules,omitempty"`
}

// AnalyzeService runs deterministic rules against stored snapshots.
type AnalyzeService struct {
	Repo     ports.StorageRepository
	Registry *rules.Registry
}

// AnalyzeSettings holds paths for rule loading.
type AnalyzeSettings struct {
	ConfigDir         string
	RulesManifestPath string
	SuppressionsPath  string
}

func (s *AnalyzeService) Analyze(ctx context.Context, settings AnalyzeSettings, opts AnalyzeOptions) (*AnalyzeResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	reg := s.Registry
	catalog, catErr := LoadPricingCatalog(ctx, "")
	if catErr != nil {
		catalog = pricing.EmptyCatalog()
	}
	if reg == nil {
		reg = rules.DefaultRegistry(catalog)
	}

	manifestPath := settings.RulesManifestPath
	if manifestPath == "" {
		manifestPath = os.Getenv("COA_RULES_MANIFEST")
	}
	manifest, err := rules.LoadManifest(manifestPath, reg)
	if err != nil {
		return nil, err
	}

	snap, err := s.resolveSnapshot(ctx, opts.SnapshotID, opts)
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
	suppIndex := rules.NewSuppressionIndex(suppEntries, time.Now().UTC())

	engine := rules.Engine{}
	out, err := engine.Analyze(rules.AnalyzeInput{
		Snapshot:       snap,
		Manifest:       manifest,
		Registry:       reg,
		Suppressions:   suppIndex,
		RuleFilter:     opts.RuleIDs,
		CategoryFilter: opts.Categories,
		PricingCatalog: catalog,
	})
	if err != nil {
		return nil, err
	}

	runID, err := rules.NewAnalysisRunID()
	if err != nil {
		return nil, fmt.Errorf("analysis run id: %w", err)
	}
	started := snap.StartedAt
	completed := rules.CompletedAt(started)
	run := &domain.AnalysisRun{
		ID:              runID,
		SnapshotID:      snap.ID,
		Status:          domain.AnalysisComplete,
		RuleSetVersion:  out.RulesetVersion,
		StartedAt:       started,
		CompletedAt:     completed,
		Findings:        out.Findings,
		Recommendations: out.Recommendations,
		Evidence:        out.Evidence,
	}

	result := &AnalyzeResult{
		AnalysisRunID:  runID,
		SnapshotID:     snap.ID,
		RulesetVersion: out.RulesetVersion,
		Summary:        out.Summary,
		Findings:       out.Findings,
		RuleExecutions: out.Executions,
	}

	if opts.Persist {
		if err := s.Repo.SaveAnalysisRun(ctx, run); err != nil {
			return nil, fmt.Errorf("save analysis run: %w", err)
		}
	}

	return result, nil
}

func (s *AnalyzeService) resolveSnapshot(ctx context.Context, id types.SnapshotID, opts AnalyzeOptions) (*domain.CollectionSnapshot, error) {
	if id != "" {
		snap, err := s.Repo.GetSnapshot(ctx, id)
		if err != nil {
			return nil, err
		}
		if !snap.IsAnalyzable() {
			if snap.Status == domain.SnapshotPartial {
				if !opts.AllowPartialSnapshot {
					return nil, fmt.Errorf("snapshot %q is partial; re-run collect or pass --allow-partial-snapshot", id)
				}
			} else {
				return nil, fmt.Errorf("snapshot %q is not complete", id)
			}
		}
		return snap, nil
	}
	list, err := s.Repo.ListSnapshots(ctx, ports.ListSnapshotFilter{CompleteOnly: true, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no complete snapshots found; import a fixture or run collect first")
	}
	return s.Repo.GetSnapshot(ctx, list[0].ID)
}
