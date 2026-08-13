package api

import (
	"context"
	"fmt"

	awsbilling "github.com/kernul-io/cloudopt/internal/adapters/aws-billing"
	appbilling "github.com/kernul-io/cloudopt/internal/application/billing"
	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

// CostCollectService orchestrates billing collection and snapshot updates.
type CostCollectService struct {
	Repo   ports.StorageRepository
	Source ports.BillingSource
}

func (s *CostCollectService) Collect(ctx context.Context, opts ports.CostCollectOptions) (*ports.CostCollectResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	source, err := s.resolveSource(ctx, opts)
	if err != nil {
		return nil, err
	}
	pf, err := source.Preflight(ctx, opts)
	if err != nil {
		return nil, err
	}
	if opts.DryRun {
		return &ports.CostCollectResult{DryRun: true, Preflight: pf}, nil
	}
	snap, err := s.loadTargetSnapshot(ctx, opts)
	if err != nil {
		return nil, err
	}
	progress := opts.Progress
	if progress == nil {
		progress = ports.NopProgress{}
	}
	progress.Step("collecting billing via Cost Explorer")
	out, err := source.Collect(ctx, opts, snap)
	if err != nil {
		return nil, err
	}
	recon := appbilling.Reconcile(out.SourceTotals, out.Costs, appbilling.DefaultReconcileToleranceBasisPoints)
	if err := s.Repo.ReplaceSnapshotCosts(ctx, snap.ID, out.Costs, out.Coverage, out.SourceTotals); err != nil {
		return nil, fmt.Errorf("persist costs: %w", err)
	}
	return &ports.CostCollectResult{
		SnapshotID: snap.ID,
		Partial:    out.Partial,
		Preflight:  pf,
		Reconcile:  mapReconcileSummary(recon),
	}, nil
}

func (s *CostCollectService) ReconcileSnapshot(ctx context.Context, snapshotID types.SnapshotID) (*domain.CostReconciliation, error) {
	snap, err := s.Repo.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	sourceTotals, err := s.Repo.GetSnapshotBillingSourceTotals(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	if len(sourceTotals) == 0 {
		sourceTotals = sumCostRows(snap.Costs)
	}
	recon := appbilling.Reconcile(sourceTotals, snap.Costs, appbilling.DefaultReconcileToleranceBasisPoints)
	return &recon, nil
}

func sumCostRows(costs []domain.CostRecord) map[string]types.Money {
	totals := map[string]types.Money{}
	for _, c := range costs {
		cur := c.Amount.Currency
		if cur == "" {
			continue
		}
		if prev, ok := totals[cur]; ok {
			if sum, err := prev.Add(c.Amount); err == nil {
				totals[cur] = sum
			}
		} else {
			totals[cur] = c.Amount
		}
	}
	return totals
}

func (s *CostCollectService) loadTargetSnapshot(ctx context.Context, opts ports.CostCollectOptions) (*domain.CollectionSnapshot, error) {
	if opts.SnapshotID != "" {
		return s.Repo.GetSnapshot(ctx, opts.SnapshotID)
	}
	list, err := s.Repo.ListSnapshots(ctx, ports.ListSnapshotFilter{
		AccountID: opts.AccountID,
		Limit:     1,
	})
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no snapshot found; collect inventory first or pass --snapshot-id")
	}
	return s.Repo.GetSnapshot(ctx, list[0].ID)
}

func (s *CostCollectService) resolveSource(ctx context.Context, opts ports.CostCollectOptions) (ports.BillingSource, error) {
	if s.Source != nil {
		return s.Source, nil
	}
	if opts.Offline {
		root := opts.FixtureRoot
		if root == "" {
			root = "testdata/aws-billing"
		}
		return awsbilling.NewFixtureBillingSource(root), nil
	}
	return awsbilling.NewLiveBillingSource(ctx, opts.RoleARN, opts.ExternalID)
}

func mapReconcileSummary(r domain.CostReconciliation) *ports.CostReconciliationSummary {
	sum := &ports.CostReconciliationSummary{
		WithinTolerance:      r.WithinTolerance,
		ToleranceBasisPoints: r.ToleranceBasisPoints,
		SourceTotal:          moneyMapMinor(r.SourceTotal),
		AttributedTotal:      moneyMapMinor(r.AttributedTotal),
		UnattributedTotal:    moneyMapMinor(r.UnattributedTotal),
		DiscrepancyMinor:     moneyMapMinor(r.Discrepancy),
	}
	return sum
}

func moneyMapMinor(m map[string]types.Money) map[string]int64 {
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v.AmountMinor
	}
	return out
}
