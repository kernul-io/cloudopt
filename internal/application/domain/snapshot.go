package domain

import "github.com/kernul-io/cloudopt/internal/application/domain/types"

// SnapshotStatus tracks collection lifecycle.
type SnapshotStatus string

const (
	SnapshotInProgress SnapshotStatus = "in_progress"
	SnapshotComplete   SnapshotStatus = "complete"
	SnapshotPartial    SnapshotStatus = "partial"
	SnapshotFailed     SnapshotStatus = "failed"
)

// CollectionSnapshot is an immutable inventory and cost/metrics bundle after completion.
type CollectionSnapshot struct {
	ID            types.SnapshotID
	AccountID     types.AccountID
	Provider      types.Provider
	Status        SnapshotStatus
	SchemaVersion int
	ExternalKey   string // optional idempotency key from importer/collector
	StartedAt     types.Timestamp
	CompletedAt   *types.Timestamp

	Account            Account
	Regions            []Region
	Resources          []Resource
	Relationships      []Relationship
	Costs              []CostRecord
	Metrics            []MetricSeries
	UtilizationSignals []UtilizationSignal
	MetricsMeta        *MetricsCollectionMeta
	Coverage           CollectionCoverage
}

// IsAnalyzable returns true when the snapshot finished successfully with full coverage.
func (s *CollectionSnapshot) IsAnalyzable() bool {
	return s.Status == SnapshotComplete
}

// IsAnalyzableAllowPartial permits analysis on intentionally partial inventory snapshots.
func (s *CollectionSnapshot) IsAnalyzableAllowPartial() bool {
	return s.Status == SnapshotComplete || s.Status == SnapshotPartial
}

// SnapshotSummary is a list view without full graph payload.
type SnapshotSummary struct {
	ID          types.SnapshotID
	AccountID   types.AccountID
	Provider    types.Provider
	Status      SnapshotStatus
	StartedAt   types.Timestamp
	CompletedAt *types.Timestamp
	ExternalKey string
}
