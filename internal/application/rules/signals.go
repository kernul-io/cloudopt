package rules

import (
	"strconv"
	"time"

	appmetrics "github.com/kernul-io/cloudopt/internal/application/metrics"
	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// Catalog returns the pricing catalog attached to the snapshot view.
func (v *SnapshotView) Catalog() *pricing.Catalog {
	if v == nil {
		return nil
	}
	return v.catalog
}

// UtilizationMetric returns a derived or persisted utilization signal value.
func (v *SnapshotView) UtilizationMetric(resourceID types.ResourceID, metricName string, kind domain.UtilizationSignalKind) (value float64, coverage float64, notes []string, ok bool) {
	if v == nil || v.Snapshot == nil {
		return 0, 0, nil, false
	}
	for _, sig := range v.Snapshot.UtilizationSignals {
		if sig.ResourceID == resourceID && sig.MetricName == metricName && sig.Kind == kind {
			return sig.Value, sig.CoverageRatio, append([]string(nil), sig.Notes...), true
		}
	}
	// Fallback: derive from raw metric series for offline fixtures without persisted signals.
	var series *domain.MetricSeries
	for i := range v.Snapshot.Metrics {
		if v.Snapshot.Metrics[i].ResourceID == resourceID && v.Snapshot.Metrics[i].Name == metricName {
			series = &v.Snapshot.Metrics[i]
			break
		}
	}
	if series == nil {
		return 0, 0, nil, false
	}
	window := domain.MetricObservationWindow{
		Start:         v.Snapshot.StartedAt,
		End:           v.observed,
		PeriodSeconds: 3600,
	}
	if v.Snapshot.MetricsMeta != nil {
		window = v.Snapshot.MetricsMeta.Window
	}
	opts := appmetrics.DefaultDeriveOptions(window, "snapshot-view", v.observed)
	opts.MemoryMetricMissing = true
	signals, _ := appmetrics.DeriveSignals(*series, opts)
	for _, sig := range signals {
		if sig.Kind == kind {
			return sig.Value, sig.CoverageRatio, append([]string(nil), sig.Notes...), true
		}
		if sig.Kind == domain.SignalSampleCoverage {
			coverage = sig.Value
		}
	}
	return 0, coverage, nil, false
}

func (v *SnapshotView) regionProviderID(regionID types.RegionID) string {
	for _, r := range v.Snapshot.Regions {
		if r.ID == regionID {
			return r.ProviderRegionID
		}
	}
	return string(regionID)
}

func parseIntAttr(attrs map[string]string, key string, fallback int64) int64 {
	if attrs == nil {
		return fallback
	}
	raw := attrs[key]
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return v
}

func catalogStale(cat *pricing.Catalog, observed types.Timestamp) bool {
	if cat == nil || len(cat.Records) == 0 {
		return true
	}
	effective := cat.Records[0].EffectiveDate
	return pricing.StalePricing(effective, observed, 180*24*time.Hour)
}
