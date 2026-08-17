package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestPercentileBoundaries(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	require.Equal(t, 1.0, Percentile(vals, 0))
	require.Equal(t, 10.0, Percentile(vals, 1))
	require.Equal(t, 10.0, Percentile(vals, 0.95))
	require.Equal(t, 10.0, Percentile(vals, 0.99))
	require.Equal(t, 5.0, Percentile(vals, 0.5))
}

func TestZeroVsMissingDatapoint(t *testing.T) {
	window := domain.MetricObservationWindow{
		Start:         types.NewTimestamp(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		End:           types.NewTimestamp(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)),
		PeriodSeconds: 3600,
	}
	series := domain.MetricSeries{
		ResourceID: "res-i-test",
		Name:       "CPUUtilization",
		Statistic:  "Average",
		Points: []domain.MetricPoint{
			{Timestamp: types.NewTimestamp(time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)), Value: 0, Unit: "Percent", Quality: domain.QualityObserved},
			{Timestamp: types.NewTimestamp(time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)), Value: 10, Unit: "Percent", Quality: domain.QualityUnavailable},
		},
	}
	signals, diags := DeriveSignals(series, DefaultDeriveOptions(window, "test", types.NowUTC()))
	require.NotEmpty(t, signals)
	var base domain.UtilizationSignal
	for _, s := range signals {
		if s.Kind == domain.SignalMean {
			base = s
		}
	}
	require.Equal(t, 1, base.ZeroSamples)
	require.Equal(t, 1, base.MissingSamples)
	require.NotEmpty(t, diags)
}

func TestBusinessHoursAndDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	window := domain.MetricObservationWindow{
		Start:             types.NewTimestamp(time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)),
		End:               types.NewTimestamp(time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)),
		PeriodSeconds:     3600,
		TimeZone:          "America/New_York",
		BusinessHourStart: 9,
		BusinessHourEnd:   17,
	}
	var pts []domain.MetricPoint
	for h := 0; h < 24; h++ {
		ts := time.Date(2026, 3, 8, h, 30, 0, 0, loc).UTC()
		pts = append(pts, domain.MetricPoint{
			Timestamp: types.NewTimestamp(ts),
			Value:     float64(h),
			Unit:      "Percent",
			Quality:   domain.QualityObserved,
		})
	}
	series := domain.MetricSeries{Name: "CPUUtilization", Statistic: "Average", Points: pts}
	active := activeHourValues(series, window)
	require.NotEmpty(t, active)
	for _, v := range active {
		require.GreaterOrEqual(t, v, 9.0)
		require.Less(t, v, 17.0)
	}
}

func TestSparseSamplesCoverage(t *testing.T) {
	window := domain.MetricObservationWindow{
		Start:         types.NewTimestamp(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		End:           types.NewTimestamp(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)),
		PeriodSeconds: 3600,
	}
	series := domain.MetricSeries{
		Name: "CPUUtilization",
		Points: []domain.MetricPoint{
			{Timestamp: types.NewTimestamp(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)), Value: 4, Unit: "Percent", Quality: domain.QualityObserved},
		},
	}
	signals, diags := DeriveSignals(series, DefaultDeriveOptions(window, "test", types.NowUTC()))
	var cov float64
	for _, s := range signals {
		if s.Kind == domain.SignalSampleCoverage {
			cov = s.Value
		}
	}
	require.Less(t, cov, 0.5)
	require.NotEmpty(t, diags)
}

func TestUnitConversion(t *testing.T) {
	require.InDelta(t, 0.8, BytesPerSecondToMbps(100_000), 0.001)
	require.Equal(t, "bytes_per_second", NormalizeUnitLabel("Bytes/Second"))
}

func TestMemoryMissingNote(t *testing.T) {
	window := domain.MetricObservationWindow{
		Start:         types.NewTimestamp(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		End:           types.NewTimestamp(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)),
		PeriodSeconds: 3600,
	}
	series := domain.MetricSeries{
		Name: "CPUUtilization",
		Points: []domain.MetricPoint{
			{Timestamp: types.NewTimestamp(time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)), Value: 12, Unit: "Percent", Quality: domain.QualityObserved},
		},
	}
	opts := DefaultDeriveOptions(window, "test", types.NowUTC())
	opts.MemoryMetricMissing = true
	_, diags := DeriveSignals(series, opts)
	require.NotEmpty(t, diags)
}
