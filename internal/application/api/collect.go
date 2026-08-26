package api

import (
	"context"
	"fmt"
	"time"

	awsinventory "github.com/kernul-io/cloudopt/internal/adapters/aws-inventory"
	"github.com/kernul-io/cloudopt/internal/adapters/config"
	gcpinventory "github.com/kernul-io/cloudopt/internal/adapters/gcp-inventory"
	"github.com/kernul-io/cloudopt/internal/application/audit"
	"github.com/kernul-io/cloudopt/internal/application/collect"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/application/security"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// CollectService orchestrates provider inventory collectors and persistence.
type CollectService struct {
	Repo      ports.StorageRepository
	Settings  config.Settings
	Collector ports.InventoryCollector
	Audit     *audit.Log
}

// Collect runs inventory collection and optionally persists the snapshot.
func (s *CollectService) Collect(ctx context.Context, opts ports.CollectOptions, progress ports.ProgressReporter) (*ports.CollectResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	provider := opts.Provider
	if provider == "" {
		provider = types.ProviderAWS
		opts.Provider = provider
	}
	budget := security.DefaultBudget(provider)
	opts.MaxConcurrent = budget.ClampConcurrency(opts.MaxConcurrent)

	collector, err := s.resolveCollector(ctx, opts)
	if err != nil {
		return nil, err
	}
	pf, err := collector.Preflight(ctx, opts)
	if err != nil {
		return nil, err
	}
	if !opts.Offline && len(pf.MissingActions) > 0 {
		return nil, ports.ErrMissingPermissions(pf.MissingActions)
	}
	if s.Audit != nil {
		_ = s.Audit.Append(audit.Event{
			Kind:      audit.EventCollectionScope,
			Workspace: s.Settings.WorkspaceDir,
			Details: map[string]string{
				"provider": string(provider),
				"dry_run":  fmt.Sprintf("%t", opts.DryRun),
				"offline":  fmt.Sprintf("%t", opts.Offline),
			},
		})
	}
	if opts.DryRun {
		return &ports.CollectResult{Preflight: pf, DryRun: true}, nil
	}

	accountID := opts.AccountID
	if accountID == "" && pf != nil && pf.ProviderAccountID != "" {
		switch provider {
		case types.ProviderGCP:
			accountID = types.AccountID("acct-gcp-" + pf.ProviderAccountID)
		default:
			accountID = types.AccountID("acct-aws-" + pf.ProviderAccountID)
		}
	}
	if accountID == "" {
		accountID = types.AccountID("acct-" + string(provider))
	}

	lc := &collect.Lifecycle{Repo: s.Repo, TTL: s.Settings.IncompleteSnapshotTTL()}
	var snapID types.SnapshotID
	if opts.Resume {
		snapID, err = lc.PrepareSnapshotID(ctx, accountID, provider, opts.SnapshotID)
	} else {
		snapID, err = lc.PrepareSnapshotID(ctx, accountID, provider, "")
	}
	if err != nil {
		return nil, err
	}
	opts.SnapshotID = snapID

	started := time.Now()
	snap, err := collector.Collect(ctx, opts, progress)
	if err != nil {
		_ = lc.Fail(ctx, snapID)
		return nil, err
	}
	if snap == nil {
		return &ports.CollectResult{Preflight: pf, DryRun: true}, nil
	}
	snap.ID = snapID
	if err := budget.ValidateSnapshotResult(snap, time.Since(started), 0); err != nil {
		_ = lc.Fail(ctx, snapID)
		return nil, err
	}
	if err := lc.Finalize(ctx, snap); err != nil {
		return nil, fmt.Errorf("save snapshot: %w", err)
	}
	return &ports.CollectResult{
		SnapshotID: snap.ID,
		Preflight:  pf,
		Partial:    snap.Status == domain.SnapshotPartial,
	}, nil
}

func (s *CollectService) resolveCollector(ctx context.Context, opts ports.CollectOptions) (ports.InventoryCollector, error) {
	if s.Collector != nil {
		return s.Collector, nil
	}
	provider := opts.Provider
	if provider == "" {
		provider = types.ProviderAWS
	}
	switch provider {
	case types.ProviderGCP:
		if opts.Offline {
			root := opts.FixtureRoot
			if root == "" {
				root = "testdata/gcp-inventory"
			}
			return gcpinventory.NewFixtureCollector(root)
		}
		return gcpinventory.NewLiveCollector(ctx, gcpinventory.LiveOptions{
			ImpersonateServiceAccount: opts.ImpersonateServiceAccount,
		})
	default:
		if opts.Offline {
			root := opts.FixtureRoot
			if root == "" {
				root = "testdata/aws-inventory"
			}
			return awsinventory.NewFixtureCollector(root)
		}
		return awsinventory.NewLiveCollector(ctx, opts.RoleARN, opts.ExternalID)
	}
}
