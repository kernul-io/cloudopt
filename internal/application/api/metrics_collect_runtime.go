package api

import (
	"context"

	"github.com/kernul-io/cloudopt/internal/application/ports"
)

// MetricsCollectResult is the CLI JSON payload for metrics collection.
type MetricsCollectResult struct {
	SnapshotID string                   `json:"snapshot_id,omitempty"`
	DryRun     bool                     `json:"dry_run,omitempty"`
	Partial    bool                     `json:"partial,omitempty"`
	Preflight  *MetricsCollectPreflight `json:"preflight,omitempty"`
	Series     int                      `json:"series_count,omitempty"`
	Signals    int                      `json:"signals_count,omitempty"`
}

type MetricsCollectPreflight struct {
	ProviderAccountID string   `json:"provider_account_id"`
	CallerARN         string   `json:"caller_arn"`
	LookbackDays      int      `json:"lookback_days"`
	PeriodSeconds     int      `json:"period_seconds"`
	MissingActions    []string `json:"missing_actions,omitempty"`
}

func (r *Runtime) CollectMetrics(ctx context.Context, opts ports.MetricsCollectOptions) (*ports.MetricsCollectResult, error) {
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
	svc := &MetricsCollectService{Repo: repo}
	out, err := svc.Collect(ctx, opts)
	if err != nil {
		return nil, err
	}
	r.lastMetricsCollect = MapMetricsCollectResult(out)
	return out, nil
}

func MapMetricsCollectResult(out *ports.MetricsCollectResult) *MetricsCollectResult {
	if out == nil {
		return nil
	}
	res := &MetricsCollectResult{
		SnapshotID: string(out.SnapshotID),
		DryRun:     out.DryRun,
		Partial:    out.Partial,
		Series:     out.Series,
		Signals:    out.Signals,
	}
	if out.Preflight != nil {
		res.Preflight = &MetricsCollectPreflight{
			ProviderAccountID: out.Preflight.ProviderAccountID,
			CallerARN:         out.Preflight.CallerARN,
			LookbackDays:      out.Preflight.LookbackDays,
			PeriodSeconds:     out.Preflight.PeriodSeconds,
			MissingActions:    out.Preflight.MissingActions,
		}
	}
	return res
}
