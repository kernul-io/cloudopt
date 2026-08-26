package collect_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sqliterepository "github.com/kernul-io/cloudopt/internal/adapters/sqlite-repository"
	"github.com/kernul-io/cloudopt/internal/application/collect"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestLifecycle_cleanupAbandoned(t *testing.T) {
	dir := t.TempDir()
	db, err := sqliterepository.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	repo := sqliterepository.NewRepository(db)
	require.NoError(t, repo.Migrate(context.Background()))

	old := time.Now().UTC().Add(-48 * time.Hour)
	staleID := types.SnapshotID("snap-stale")
	require.NoError(t, repo.SaveInProgressSnapshot(context.Background(), &domain.CollectionSnapshot{
		ID:            staleID,
		AccountID:     "acct-test",
		Provider:      types.ProviderAWS,
		Status:        domain.SnapshotInProgress,
		SchemaVersion: 1,
		StartedAt:     types.NewTimestamp(old),
	}))

	lc := &collect.Lifecycle{Repo: repo, TTL: time.Nanosecond}
	require.NoError(t, lc.CleanupAbandoned(context.Background(), "acct-test"))
	_, err = repo.GetSnapshot(context.Background(), staleID)
	require.Error(t, err)
}
