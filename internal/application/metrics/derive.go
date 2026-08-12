package metrics

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
)

// DeriveOptions configures signal derivation for one series.
type DeriveOptions struct {
	Window              domain.MetricObservationWindow
	Source              string
	ObservedAt          types.Timestamp
	IdleThreshold       float64
	MinIdlePeriods      int
	MemoryMetricMissing bool
}

// DefaultDeriveOptions returns conservative defaults for CPU-style percent metrics.
func DefaultDeriveOptions(window domain.MetricObservationWindow, source string, at types.Timestamp) DeriveOptions {
	return DeriveOptions{
		Window:         window,
		Source:         source,
		ObservedAt:     at,
		IdleThreshold:  5.0,
		MinIdlePeriods: 3,
	}
}

// DeriveSignals computes deterministic utilization signals from one metric series.
func DeriveSignals(series domain.MetricSeries, opts DeriveOptions) ([]domain.UtilizationSignal, []domain.MetricDiagnostic) {
	query := queryFromSeries(series)
	values, zeroCount, missing := sampleValues(series, opts.Window)
	expected := expectedSamples(opts.Window)
	coverage := coverageRatio(len(values), expected, missing)

	var diags []domain.MetricDiagnostic
	if len(values) == 0 && missing == 0 {
		diags = append(diags, domain.MetricDiagnostic{
			Code:       "metric_empty",
			ResourceID: series.ResourceID,
			MetricName: series.Name,
			Message:    "metric series returned no datapoints in the observation window",
			Severity:   "warning",
		})
	}
	if coverage < 0.5 && expected > 0 {
		diags = append(diags, domain.MetricDiagnostic{
			Code:       "insufficient_coverage",
			ResourceID: series.ResourceID,
			MetricName: series.Name,
			Message:    "sample coverage below 50% of expected periods",
			Severity:   "warning",
		})
	}

	unit := normalizedUnit(series)
	base := domain.UtilizationSignal{
		ResourceID:      series.ResourceID,
		MetricName:      series.Name,
		Unit:            unit,
		SampleCount:     len(values),
		ExpectedSamples: expected,
		CoverageRatio:   coverage,
		ZeroSamples:     zeroCount,
		MissingSamples:  missing,
		Query:           query,
		Window:          opts.Window,
		Provenance: domain.Provenance{
			Quality:    domain.QualityDerived,
			Source:     opts.Source + "/derive",
			ObservedAt: opts.ObservedAt,
		},
	}

	if len(values) == 0 {
		return []domain.UtilizationSignal{coverageSignal(base, coverage)}, diags
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	out := []domain.UtilizationSignal{
		signalCopy(base, domain.SignalMean, mean(values), nil),
		signalCopy(base, domain.SignalMaximum, sorted[len(sorted)-1], nil),
		signalCopy(base, domain.SignalP95, Percentile(sorted, 0.95), nil),
		signalCopy(base, domain.SignalP99, Percentile(sorted, 0.99), nil),
		coverageSignal(base, coverage),
		signalCopy(base, domain.SignalTrend, trend(values), []string{"trend compares mean of first vs second half of samples"}),
		signalCopy(base, domain.SignalIdlePeriods, float64(countIdleRuns(values, opts.IdleThreshold, opts.MinIdlePeriods)), []string{"idle when value <= idle threshold for consecutive periods"}),
	}

	active := activeHourValues(series, opts.Window)
	if len(active) > 0 {
		sort.Float64s(active)
		out = append(out, signalCopy(base, domain.SignalActiveHourP95, Percentile(active, 0.95), []string{"computed over configured business hours in " + opts.Window.TimeZone}))
	}

	if opts.MemoryMetricMissing && isCPUMetric(series.Name) {
		for i := range out {
			if out[i].Kind == domain.SignalP95 || out[i].Kind == domain.SignalMean {
				out[i].Notes = append(out[i].Notes, "memory evidence unavailable; do not rightsizing from CPU alone")
			}
		}
		diags = append(diags, domain.MetricDiagnostic{
			Code:       "memory_unavailable",
			ResourceID: series.ResourceID,
			MetricName: series.Name,
			Message:    "relevant memory metric missing; CPU-only utilization limits rightsizing confidence",
			Severity:   "info",
		})
	}

	return out, diags
}

func queryFromSeries(series domain.MetricSeries) domain.MetricQueryIdentity {
	ns := ""
	src := series.Provenance.Source
	if i := strings.Index(src, "|"); i >= 0 {
		rest := src[i+1:]
		if j := strings.Index(rest, "|"); j >= 0 {
			ns = rest[:j]
		} else {
			ns = rest
		}
	}
	return domain.MetricQueryIdentity{
		Namespace:     ns,
		MetricName:    series.Name,
		Statistic:     series.Statistic,
		PeriodSeconds: periodFromPoints(series),
	}
}

func periodFromPoints(series domain.MetricSeries) int {
	if len(series.Points) < 2 {
		return 0
	}
	d := series.Points[1].Timestamp.Sub(series.Points[0].Timestamp.Time)
	if d <= 0 {
		return 0
	}
	return int(d.Seconds())
}

func sampleValues(series domain.MetricSeries, window domain.MetricObservationWindow) (values []float64, zeroCount, missing int) {
	start, end := window.Start.Time, window.End.Time
	for _, pt := range series.Points {
		if pt.Quality == domain.QualityUnavailable {
			missing++
			continue
		}
		ts := pt.Timestamp.Time
		if ts.Before(start) || !ts.Before(end) {
			continue
		}
		v := NormalizeValue(pt.Value, pt.Unit)
		values = append(values, v)
		if v == 0 {
			zeroCount++
		}
	}
	return values, zeroCount, missing
}

func expectedSamples(window domain.MetricObservationWindow) int {
	if window.PeriodSeconds <= 0 {
		return 0
	}
	dur := window.End.Sub(window.Start.Time)
	if dur <= 0 {
		return 0
	}
	return int(dur / time.Duration(window.PeriodSeconds))
}

func coverageRatio(samples, expected, missing int) float64 {
	if expected <= 0 {
		if samples == 0 {
			return 0
		}
		return 1
	}
	return float64(samples) / float64(expected)
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func trend(v []float64) float64 {
	if len(v) < 2 {
		return 0
	}
	mid := len(v) / 2
	first := mean(v[:mid])
	second := mean(v[mid:])
	return second - first
}

func countIdleRuns(v []float64, threshold float64, minRun int) int {
	if minRun <= 0 {
		minRun = 1
	}
	runs := 0
	run := 0
	for _, x := range v {
		if x <= threshold {
			run++
			if run == minRun {
				runs++
			}
			continue
		}
		run = 0
	}
	return runs
}

func activeHourValues(series domain.MetricSeries, window domain.MetricObservationWindow) []float64 {
	loc := time.UTC
	if window.TimeZone != "" && window.TimeZone != "UTC" {
		if l, err := time.LoadLocation(window.TimeZone); err == nil {
			loc = l
		}
	}
	start, end := window.Start.Time, window.End.Time
	var out []float64
	for _, pt := range series.Points {
		if pt.Quality == domain.QualityUnavailable {
			continue
		}
		ts := pt.Timestamp.Time
		if ts.Before(start) || !ts.Before(end) {
			continue
		}
		local := ts.In(loc)
		h := local.Hour()
		if !hourInBusiness(h, window.BusinessHourStart, window.BusinessHourEnd) {
			continue
		}
		out = append(out, NormalizeValue(pt.Value, pt.Unit))
	}
	return out
}

func hourInBusiness(hour, start, end int) bool {
	if start == end {
		return true
	}
	if start < end {
		return hour >= start && hour < end
	}
	return hour >= start || hour < end
}

func signalCopy(base domain.UtilizationSignal, kind domain.UtilizationSignalKind, value float64, notes []string) domain.UtilizationSignal {
	s := base
	s.Kind = kind
	s.Value = value
	s.Notes = append([]string(nil), notes...)
	return s
}

func coverageSignal(base domain.UtilizationSignal, ratio float64) domain.UtilizationSignal {
	return signalCopy(base, domain.SignalSampleCoverage, ratio, nil)
}

func normalizedUnit(series domain.MetricSeries) string {
	if len(series.Points) == 0 {
		return ""
	}
	return NormalizeUnitLabel(series.Points[0].Unit)
}

func isCPUMetric(name string) bool {
	switch name {
	case "CPUUtilization", "cpu_utilization":
		return true
	default:
		return false
	}
}

// Percentile uses nearest-rank on sorted values (deterministic, do not average percentiles).
func Percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	rank := int(math.Ceil(p*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}
