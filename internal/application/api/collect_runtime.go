package api

import (
	"context"
	"fmt"

	"github.com/kernul-io/cloudopt/internal/application/audit"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// CollectOptions re-exports ports collect configuration for the runtime.
type CollectOptions = ports.CollectOptions

// CollectResult is the machine-readable collect payload for CLI output.
type CollectResult struct {
	SnapshotID types.SnapshotID  `json:"snapshot_id,omitempty"`
	DryRun     bool              `json:"dry_run,omitempty"`
	Partial    bool              `json:"partial,omitempty"`
	Preflight  *CollectPreflight `json:"preflight,omitempty"`
}

// CollectPreflight is a JSON-safe preflight summary.
type CollectPreflight struct {
	ProviderAccountID  string              `json:"provider_account_id"`
	CallerARN          string              `json:"caller_arn,omitempty"`
	CallerEmail        string              `json:"caller_email,omitempty"`
	SelectedRegions    []string            `json:"selected_regions"`
	SelectedProjects   []string            `json:"selected_projects,omitempty"`
	AccessibleProjects []string            `json:"accessible_projects,omitempty"`
	MissingActions     []string            `json:"missing_actions,omitempty"`
	CollectionScope    string              `json:"collection_scope,omitempty"`
	EnabledAPIs        map[string][]string `json:"enabled_apis,omitempty"`
}

func (r *Runtime) Collect(ctx context.Context, opts CollectOptions) (*ports.CollectResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch opts.Provider {
	case types.ProviderAWS, "":
		opts.Provider = types.ProviderAWS
	case types.ProviderGCP:
		// supported
	default:
		return nil, fmt.Errorf("provider %q is not supported yet; use aws or gcp", opts.Provider)
	}

	repo, err := OpenStorage(ctx, r.Settings)
	if err != nil {
		return nil, err
	}
	defer repo.Close() //nolint: errcheck
	progress := opts.Progress
	if progress == nil {
		progress = ports.NopProgress{}
	}
	auditLog, _ := audit.NewLog(r.Settings.WorkspaceDir, r.Settings.AuditLogPath)
	svc := &CollectService{Repo: repo, Settings: r.Settings, Audit: auditLog}
	out, err := svc.Collect(ctx, opts, progress)
	if err != nil {
		return nil, err
	}
	r.lastCollect = MapCollectResult(out)
	return out, nil
}

// MapCollectResult converts ports output to CLI JSON DTO.
func MapCollectResult(out *ports.CollectResult) *CollectResult {
	if out == nil {
		return nil
	}
	res := &CollectResult{
		SnapshotID: out.SnapshotID,
		DryRun:     out.DryRun,
		Partial:    out.Partial,
	}
	if out.Preflight != nil {
		res.Preflight = &CollectPreflight{
			ProviderAccountID:  out.Preflight.ProviderAccountID,
			CallerARN:          out.Preflight.CallerARN,
			CallerEmail:        out.Preflight.CallerEmail,
			SelectedRegions:    out.Preflight.SelectedRegions,
			SelectedProjects:   out.Preflight.SelectedProjects,
			AccessibleProjects: out.Preflight.AccessibleProjects,
			MissingActions:     out.Preflight.MissingActions,
			CollectionScope:    out.Preflight.CollectionScope,
			EnabledAPIs:        out.Preflight.EnabledAPIs,
		}
	}
	return res
}

// LastCollectResult returns the most recent collect output for CLI emission.
func (r *Runtime) LastCollectResult() *CollectResult {
	return r.lastCollect
}
