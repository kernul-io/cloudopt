package ports

import (
	"context"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
)

// MetricsCollectOptions configures CloudWatch (or other) utilization collection.
type MetricsCollectOptions struct {
	Provider          types.Provider
	AccountID         types.AccountID
	RoleARN           string
	ExternalID        string
	SnapshotID        types.SnapshotID
	LookbackDays      int
	PeriodSeconds     int
	TimeZone          string
	BusinessHourStart int
	BusinessHourEnd   int
	MaxConcurrent     int
	MaxDatapoints     int
	MaxAPIRequests    int
	DryRun            bool
	Offline           bool
	FixtureRoot       string
	Progress          ProgressReporter `json:"-"`
}

// MetricsPreflight summarizes metrics API access before collection.
type MetricsPreflight struct {
	ProviderAccountID string
	CallerARN         string
	LookbackDays      int
	PeriodSeconds     int
	MissingActions    []string
	Capabilities      CapabilityManifest
}

// MetricsCollectOutput is normalized metrics output before persistence.
type MetricsCollectOutput struct {
	Series      []domain.MetricSeries
	Signals     []domain.UtilizationSignal
	Window      domain.MetricObservationWindow
	Coverage    []domain.ServiceCollectionStatus
	Diagnostics []domain.MetricDiagnostic
	Partial     bool
}

// MetricsSource collects provider utilization metrics into canonical series.
type MetricsSource interface {
	Capabilities() CapabilityManifest
	Preflight(ctx context.Context, opts MetricsCollectOptions) (*MetricsPreflight, error)
	Collect(ctx context.Context, opts MetricsCollectOptions, inventory *domain.CollectionSnapshot) (*MetricsCollectOutput, error)
}

// MetricsCollectResult is returned after metrics collection and snapshot merge.
type MetricsCollectResult struct {
	SnapshotID types.SnapshotID `json:"snapshot_id,omitempty"`
	DryRun     bool             `json:"dry_run,omitempty"`
	Partial    bool             `json:"partial,omitempty"`
	Preflight  *MetricsPreflight
	Series     int `json:"series_count,omitempty"`
	Signals    int `json:"signals_count,omitempty"`
}
