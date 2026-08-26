package ports

import (
	"context"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// ListSnapshotFilter limits snapshot listing.
type ListSnapshotFilter struct {
	AccountID    types.AccountID
	CompleteOnly bool
	Status       domain.SnapshotStatus
	Limit        int
}

// StorageRepository persists canonical snapshots and analysis runs.
type StorageRepository interface {
	Migrate(ctx context.Context) error
	Close() error
	SchemaVersion(ctx context.Context) (int, error)
	CanonicalSchemaVersion(ctx context.Context) (int, error)

	SaveSnapshot(ctx context.Context, snap *domain.CollectionSnapshot) error
	SaveInProgressSnapshot(ctx context.Context, snap *domain.CollectionSnapshot) error
	ReplaceSnapshotCosts(ctx context.Context, id types.SnapshotID, costs []domain.CostRecord, coverage []domain.ServiceCollectionStatus, sourceTotals map[string]types.Money) error
	ReplaceSnapshotMetrics(ctx context.Context, id types.SnapshotID, series []domain.MetricSeries, signals []domain.UtilizationSignal, meta *domain.MetricsCollectionMeta, coverage []domain.ServiceCollectionStatus) error
	GetSnapshot(ctx context.Context, id types.SnapshotID) (*domain.CollectionSnapshot, error)
	GetSnapshotBillingSourceTotals(ctx context.Context, id types.SnapshotID) (map[string]types.Money, error)
	ListSnapshots(ctx context.Context, filter ListSnapshotFilter) ([]domain.SnapshotSummary, error)
	MarkSnapshotFailed(ctx context.Context, id types.SnapshotID) error

	SaveAnalysisRun(ctx context.Context, run *domain.AnalysisRun) error
	GetAnalysisRun(ctx context.Context, id types.AnalysisRunID) (*domain.AnalysisRun, error)
	GetLatestAnalysisRun(ctx context.Context, snapshotID types.SnapshotID) (*domain.AnalysisRun, error)

	DeleteSnapshot(ctx context.Context, id types.SnapshotID) error
	DeleteSnapshotsByAccount(ctx context.Context, accountID types.AccountID) (int, error)
	ApplyRetention(ctx context.Context, accountID types.AccountID, keepComplete int) (deleted int, err error)
}

// FixtureImporter loads offline fixture data into storage.
type FixtureImporter interface {
	Import(ctx context.Context, path string) (types.SnapshotID, error)
}
