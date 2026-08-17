package api

import (
	"context"

	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// CostCollectResult is the CLI JSON payload for billing collection.
type CostCollectResult struct {
	SnapshotID string                           `json:"snapshot_id,omitempty"`
	DryRun     bool                             `json:"dry_run,omitempty"`
	Partial    bool                             `json:"partial,omitempty"`
	Preflight  *CostCollectPreflight            `json:"preflight,omitempty"`
	Reconcile  *ports.CostReconciliationSummary `json:"reconciliation,omitempty"`
}

type CostCollectPreflight struct {
	ProviderAccountID string   `json:"provider_account_id"`
	CallerARN         string   `json:"caller_arn"`
	LookbackDays      int      `json:"lookback_days"`
	MissingActions    []string `json:"missing_actions,omitempty"`
}

// ReconcileCostResult is emitted by the cost reconciliation command.
type ReconcileCostResult struct {
	SnapshotID string                           `json:"snapshot_id"`
	Reconcile  *ports.CostReconciliationSummary `json:"reconciliation"`
}

func (r *Runtime) CollectCost(ctx context.Context, opts ports.CostCollectOptions) (*ports.CostCollectResult, error) {
	repo, err := OpenStorage(ctx, r.Settings)
	if err != nil {
		return nil, err
	}
	defer repo.Close() //nolint: errcheck
	progress := opts.Progress
	if progress == nil {
		progress = ports.NopProgress{}
	}
	opts.Progress = progress
	svc := &CostCollectService{Repo: repo}
	out, err := svc.Collect(ctx, opts)
	if err != nil {
		return nil, err
	}
	r.lastCostCollect = MapCostCollectResult(out)
	return out, nil
}

func (r *Runtime) ReconcileCost(ctx context.Context, snapshotID string) (*ReconcileCostResult, error) {
	repo, err := OpenStorage(ctx, r.Settings)
	if err != nil {
		return nil, err
	}
	defer repo.Close() //nolint: errcheck
	svc := &CostCollectService{Repo: repo}
	recon, err := svc.ReconcileSnapshot(ctx, types.SnapshotID(snapshotID))
	if err != nil {
		return nil, err
	}
	res := &ReconcileCostResult{
		SnapshotID: snapshotID,
		Reconcile:  mapReconcileSummary(*recon),
	}
	r.lastReconcile = res
	return res, nil
}

func MapCostCollectResult(out *ports.CostCollectResult) *CostCollectResult {
	if out == nil {
		return nil
	}
	res := &CostCollectResult{
		SnapshotID: string(out.SnapshotID),
		DryRun:     out.DryRun,
		Partial:    out.Partial,
		Reconcile:  out.Reconcile,
	}
	if out.Preflight != nil {
		res.Preflight = &CostCollectPreflight{
			ProviderAccountID: out.Preflight.ProviderAccountID,
			CallerARN:         out.Preflight.CallerARN,
			LookbackDays:      out.Preflight.LookbackDays,
			MissingActions:    out.Preflight.MissingActions,
		}
	}
	return res
}
