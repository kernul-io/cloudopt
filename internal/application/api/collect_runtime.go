package api

import (
	"context"
	"fmt"

	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
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
	ProviderAccountID string   `json:"provider_account_id"`
	CallerARN         string   `json:"caller_arn"`
	SelectedRegions   []string `json:"selected_regions"`
	MissingActions    []string `json:"missing_actions,omitempty"`
}

func (r *Runtime) Collect(ctx context.Context, opts CollectOptions) (*ports.CollectResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.Provider != "" && opts.Provider != types.ProviderAWS {
		return nil, fmt.Errorf("provider %q is not supported yet; use aws or --offline", opts.Provider)
	}
	opts.Provider = types.ProviderAWS

	repo, err := OpenStorage(ctx, r.Settings)
	if err != nil {
		return nil, err
	}
	progress := opts.Progress
	if progress == nil {
		progress = ports.NopProgress{}
	}
	svc := &CollectService{Repo: repo, Settings: r.Settings}
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
			ProviderAccountID: out.Preflight.ProviderAccountID,
			CallerARN:         out.Preflight.CallerARN,
			SelectedRegions:   out.Preflight.SelectedRegions,
			MissingActions:    out.Preflight.MissingActions,
		}
	}
	return res
}

// LastCollectResult returns the most recent collect output for CLI emission.
func (r *Runtime) LastCollectResult() *CollectResult {
	return r.lastCollect
}
