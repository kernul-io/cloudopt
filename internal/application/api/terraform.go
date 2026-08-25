package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	terraforminput "github.com/kernul-io/cloudopt/internal/adapters/terraform-input"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/application/terraform"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// TerraformCorrelateOptions configures IaC correlation (read-only).
type TerraformCorrelateOptions struct {
	SnapshotID     types.SnapshotID
	AnalysisRunID  types.AnalysisRunID
	StateJSONPath  string
	PlanJSONPath   string
	MappingsPath   string
	MarkdownOut    string
	JSONOut        string
	StrictPlan     bool
	EnrichFindings bool
}

// TerraformCorrelateResult holds the last correlate output for CLI emission.
type TerraformCorrelateResult struct {
	Result   terraform.CorrelationResult
	ExitCode int
}

func (r *Runtime) TerraformCorrelate(ctx context.Context, opts TerraformCorrelateOptions) (*TerraformCorrelateResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.StateJSONPath == "" {
		return nil, fmt.Errorf("state json path is required (--state-json)")
	}

	reader := terraforminput.NewReader()
	tfResources, err := reader.LoadStateFile(ctx, opts.StateJSONPath)
	if err != nil {
		return nil, err
	}

	var mappings []terraform.UserMapping
	if opts.MappingsPath != "" {
		mappings, err = terraforminput.LoadMappings(opts.MappingsPath)
		if err != nil {
			return nil, err
		}
	}

	corrOpts := terraform.CorrelateOptions{Mappings: mappings}

	repo, err := OpenStorage(ctx, r.Settings)
	if err != nil {
		return nil, err
	}
	defer repo.Close() //nolint: errcheck
	snapID := opts.SnapshotID
	if snapID == "" {
		list, err := repo.ListSnapshots(ctx, ports.ListSnapshotFilter{CompleteOnly: true, Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(list) > 0 {
			snapID = list[0].ID
		}
	}
	if snapID != "" {
		snap, err := repo.GetSnapshot(ctx, snapID)
		if err != nil {
			return nil, err
		}
		corrOpts.SnapshotID = snapID
		corrOpts.Resources = snap.Resources
		corrOpts.Provider = snap.Provider

		loadFindings := opts.EnrichFindings || opts.AnalysisRunID != ""
		if loadFindings {
			runID := opts.AnalysisRunID
			if runID == "" {
				run, err := repo.GetLatestAnalysisRun(ctx, snapID)
				if err != nil {
					return nil, fmt.Errorf("latest analysis run: %w", err)
				}
				runID = run.ID
			}
			run, err := repo.GetAnalysisRun(ctx, runID)
			if err != nil {
				return nil, err
			}
			corrOpts.AnalysisRunID = runID
			corrOpts.Findings = run.Findings
			recs := map[types.FindingID]domain.Recommendation{}
			for _, rec := range run.Recommendations {
				recs[rec.FindingID] = rec
			}
			corrOpts.Recommendations = recs
		}
	}

	result := terraform.Correlate(tfResources, corrOpts)
	result.StateSource = opts.StateJSONPath

	if opts.PlanJSONPath != "" {
		changes, err := reader.LoadPlanFile(ctx, opts.PlanJSONPath)
		if err != nil {
			return nil, err
		}
		pa := terraform.AnalyzePlan(changes)
		result.PlanSource = opts.PlanJSONPath
		result.PlanAnalysis = &pa
	}

	switch {
	case result.PlanAnalysis != nil && opts.StrictPlan && result.PlanAnalysis.PolicyViolations > 0:
		result.ExitHint = "plan_policy_warnings"
	case hasAmbiguous(result.Links):
		result.ExitHint = "ambiguous_correlation"
	default:
		result.ExitHint = "ok"
	}

	if opts.JSONOut != "" {
		data, err := terraform.ToJSON(result)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(opts.JSONOut, append(data, '\n'), 0o600); err != nil {
			return nil, fmt.Errorf("write json output: %w", err)
		}
	}
	if opts.MarkdownOut != "" {
		md := terraform.RenderMarkdownSummary(result)
		if err := os.WriteFile(opts.MarkdownOut, []byte(md), 0o600); err != nil {
			return nil, fmt.Errorf("write markdown output: %w", err)
		}
	}

	r.lastTerraform = &TerraformCorrelateResult{
		Result:   result,
		ExitCode: terraform.ExitCodeForResult(result, opts.StrictPlan),
	}
	return r.lastTerraform, nil
}

func hasAmbiguous(links []terraform.CorrelationLink) bool {
	for _, l := range links {
		if l.Ambiguous {
			return true
		}
	}
	return false
}

// LastTerraformCorrelateResult returns the most recent correlate output.
func (r *Runtime) LastTerraformCorrelateResult() *TerraformCorrelateResult {
	return r.lastTerraform
}

// WriteTerraformCorrelationDefault writes JSON under reports dir when no explicit output paths set.
func WriteTerraformCorrelationDefault(reportsDir string, result terraform.CorrelationResult) (string, error) {
	if err := os.MkdirAll(reportsDir, 0o750); err != nil {
		return "", err
	}
	name := fmt.Sprintf("terraform-correlation-%s.json", strings.ReplaceAll(result.GeneratedAt, ":", "-"))
	path := filepath.Join(reportsDir, filepath.Base(name))
	data, err := terraform.ToJSON(result)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
