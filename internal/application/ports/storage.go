package ports

import (
	"context"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
)

// ListSnapshotFilter limits snapshot listing.
type ListSnapshotFilter struct {
	AccountID    types.AccountID
	CompleteOnly bool
	Limit        int
}

// StorageRepository persists canonical snapshots and analysis runs.
type StorageRepository interface {
	Migrate(ctx context.Context) error
	SchemaVersion(ctx context.Context) (int, error)

	SaveSnapshot(ctx context.Context, snap *domain.CollectionSnapshot) error
	GetSnapshot(ctx context.Context, id types.SnapshotID) (*domain.CollectionSnapshot, error)
	ListSnapshots(ctx context.Context, filter ListSnapshotFilter) ([]domain.SnapshotSummary, error)
	MarkSnapshotFailed(ctx context.Context, id types.SnapshotID) error

	SaveAnalysisRun(ctx context.Context, run *domain.AnalysisRun) error
	GetAnalysisRun(ctx context.Context, id types.AnalysisRunID) (*domain.AnalysisRun, error)

	DeleteSnapshot(ctx context.Context, id types.SnapshotID) error
	DeleteSnapshotsByAccount(ctx context.Context, accountID types.AccountID) (int, error)
	ApplyRetention(ctx context.Context, accountID types.AccountID, keepComplete int) (deleted int, err error)
}

// FixtureImporter loads offline fixture data into storage.
type FixtureImporter interface {
	Import(ctx context.Context, path string) (types.SnapshotID, error)
}
