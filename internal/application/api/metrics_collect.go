package api

import (
	"context"
	"fmt"

	awsmetrics "github.com/kernul-io/cloudopt/internal/adapters/aws-metrics"
	gcpmetrics "github.com/kernul-io/cloudopt/internal/adapters/gcp-metrics"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

const metricsCollectorSource = "aws-metrics/cloudwatch"

// MetricsCollectService orchestrates utilization metrics collection and snapshot updates.
type MetricsCollectService struct {
	Repo   ports.StorageRepository
	Source ports.MetricsSource
}

func (s *MetricsCollectService) Collect(ctx context.Context, opts ports.MetricsCollectOptions) (*ports.MetricsCollectResult, error) {
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
	if !opts.Offline && len(pf.MissingActions) > 0 {
		return nil, ports.ErrMissingPermissions(pf.MissingActions)
	}
	if opts.DryRun {
		return &ports.MetricsCollectResult{DryRun: true, Preflight: pf}, nil
	}
	snap, err := s.loadTargetSnapshot(ctx, opts)
	if err != nil {
		return nil, err
	}
	progress := opts.Progress
	if progress == nil {
		progress = ports.NopProgress{}
	}
	progress.Step(metricsProgressLabel(opts))
	out, err := source.Collect(ctx, opts, snap)
	if err != nil {
		return nil, err
	}
	meta := &domain.MetricsCollectionMeta{
		Window:      out.Window,
		Diagnostics: out.Diagnostics,
		Partial:     out.Partial,
		Source:      metricsSourceLabel(opts),
	}
	if err := s.Repo.ReplaceSnapshotMetrics(ctx, snap.ID, out.Series, out.Signals, meta, out.Coverage); err != nil {
		return nil, fmt.Errorf("persist metrics: %w", err)
	}
	return &ports.MetricsCollectResult{
		SnapshotID: snap.ID,
		Partial:    out.Partial,
		Preflight:  pf,
		Series:     len(out.Series),
		Signals:    len(out.Signals),
	}, nil
}

func (s *MetricsCollectService) loadTargetSnapshot(ctx context.Context, opts ports.MetricsCollectOptions) (*domain.CollectionSnapshot, error) {
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

func (s *MetricsCollectService) resolveSource(ctx context.Context, opts ports.MetricsCollectOptions) (ports.MetricsSource, error) {
	if s.Source != nil {
		return s.Source, nil
	}
	switch opts.Provider {
	case types.ProviderGCP:
		if opts.Offline {
			root := opts.FixtureRoot
			if root == "" {
				root = "testdata/gcp-metrics"
			}
			return gcpmetrics.NewFixtureMetricsSource(root), nil
		}
		return gcpmetrics.NewLiveMetricsSource(ctx, "")
	default:
		if opts.Offline {
			root := opts.FixtureRoot
			if root == "" {
				root = "testdata/aws-metrics"
			}
			return awsmetrics.NewFixtureMetricsSource(root), nil
		}
		return awsmetrics.NewLiveMetricsSource(ctx, opts.RoleARN, opts.ExternalID)
	}
}

func metricsProgressLabel(opts ports.MetricsCollectOptions) string {
	if opts.Provider == types.ProviderGCP {
		return "collecting utilization metrics via Cloud Monitoring"
	}
	return "collecting utilization metrics via CloudWatch"
}

func metricsSourceLabel(opts ports.MetricsCollectOptions) string {
	if opts.Provider == types.ProviderGCP {
		return "gcp-metrics/cloud-monitoring"
	}
	return metricsCollectorSource
}
