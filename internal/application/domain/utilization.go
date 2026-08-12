package domain

import "github.com/kernul-io/cloudopt/internal/application/domain/types"

// MetricQueryIdentity preserves raw metric identity for reproduction.
type MetricQueryIdentity struct {
	Namespace     string
	MetricName    string
	Dimensions    map[string]string
	Statistic     string
	PeriodSeconds int
}

// MetricObservationWindow describes the collected utilization interval.
type MetricObservationWindow struct {
	Start             types.Timestamp
	End               types.Timestamp
	PeriodSeconds     int
	TimeZone          string
	BusinessHourStart int // inclusive local hour 0-23
	BusinessHourEnd   int // exclusive local hour 0-24
}

// UtilizationSignalKind names a derived statistic from a metric series.
type UtilizationSignalKind string

const (
	SignalMean           UtilizationSignalKind = "mean"
	SignalMaximum        UtilizationSignalKind = "maximum"
	SignalP95            UtilizationSignalKind = "p95"
	SignalP99            UtilizationSignalKind = "p99"
	SignalActiveHourP95  UtilizationSignalKind = "active_hour_p95"
	SignalTrend          UtilizationSignalKind = "trend"
	SignalSampleCoverage UtilizationSignalKind = "sample_coverage"
	SignalIdlePeriods    UtilizationSignalKind = "idle_periods"
)

// UtilizationSignal is a deterministic derivative of a metric series.
type UtilizationSignal struct {
	ID              int64
	ResourceID      types.ResourceID
	MetricName      string
	Kind            UtilizationSignalKind
	Value           float64
	Unit            string
	SampleCount     int
	ExpectedSamples int
	CoverageRatio   float64
	ZeroSamples     int
	MissingSamples  int
	Query           MetricQueryIdentity
	Window          MetricObservationWindow
	Notes           []string
	Provenance      Provenance
}

// MetricDiagnostic records missing, delayed, or insufficient metric evidence.
type MetricDiagnostic struct {
	Code       string
	ResourceID types.ResourceID
	MetricName string
	Message    string
	Severity   string
}

// MetricsCollectionMeta summarizes a metrics refresh on a snapshot.
type MetricsCollectionMeta struct {
	Window      MetricObservationWindow
	Diagnostics []MetricDiagnostic
	Partial     bool
	Source      string
}
