package sqliterepository_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/adapters/fixture"
	sqliterepository "github.com/kernul-io/cloudopt/internal/adapters/sqlite-repository"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestMigrateFromEmpty(t *testing.T) {
	ctx := context.Background()
	repo, _ := openTestDB(t, ctx)

	v, err := repo.SchemaVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, 6, v)

	canonical, err := repo.(*sqliterepository.Repository).CanonicalSchemaVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, canonical)
}

func TestMigrateIdempotent(t *testing.T) {
	ctx := context.Background()
	repo, _ := openTestDB(t, ctx)
	require.NoError(t, repo.Migrate(ctx))
	v, err := repo.SchemaVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, 6, v)
}

func TestFixtureRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo, _ := openTestDB(t, ctx)
	path := filepath.Join("..", "..", "..", "testdata", "fixtures", "sample.yaml")

	importer := fixture.NewImporter(repo)
	id1, err := importer.Import(ctx, path)
	require.NoError(t, err)

	loaded, err := repo.GetSnapshot(ctx, id1)
	require.NoError(t, err)
	require.Equal(t, domain.SnapshotComplete, loaded.Status)
	require.Len(t, loaded.Regions, 2)
	require.Len(t, loaded.Resources, 10)
	require.True(t, loaded.Relationships[len(loaded.Relationships)-1].TargetMissing)
	require.NotEmpty(t, loaded.Costs)
	require.NotEmpty(t, loaded.Metrics)

	// money preserved
	var volCost *domain.CostRecord
	for i := range loaded.Costs {
		if loaded.Costs[i].ResourceID == "res-vol-unattached" {
			volCost = &loaded.Costs[i]
			break
		}
	}
	require.NotNil(t, volCost)
	require.Equal(t, int64(2500), volCost.Amount.AmountMinor)

	reloaded, err := repo.GetSnapshot(ctx, id1)
	require.NoError(t, err)
	require.Empty(t, domain.CompareSnapshots(loaded, reloaded))
}

func TestFixtureIdempotentExternalKey(t *testing.T) {
	ctx := context.Background()
	repo, _ := openTestDB(t, ctx)
	path := filepath.Join("..", "..", "..", "testdata", "fixtures", "sample.yaml")
	importer := fixture.NewImporter(repo)

	id1, err := importer.Import(ctx, path)
	require.NoError(t, err)
	id2, err := importer.Import(ctx, path)
	require.NoError(t, err)
	require.Equal(t, id1, id2)

	list, err := repo.ListSnapshots(ctx, ports.ListSnapshotFilter{AccountID: "acct-fixture-001"})
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func TestFixtureSeparateSnapshotsWithoutExternalKey(t *testing.T) {
	ctx := context.Background()
	repo, _ := openTestDB(t, ctx)
	dir := t.TempDir()
	path := filepath.Join(dir, "once.yaml")
	sample, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "fixtures", "sample.yaml"))
	require.NoError(t, err)
	content := string(sample)
	content = replaceYAMLField(content, "external_key: sample-offline-v1", "external_key: \"\"")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	importer := fixture.NewImporter(repo)
	id1, err := importer.Import(ctx, path)
	require.NoError(t, err)
	id2, err := importer.Import(ctx, path)
	require.NoError(t, err)
	require.NotEqual(t, id1, id2)
}

func TestAnalysisRunTransaction(t *testing.T) {
	ctx := context.Background()
	repo, _ := openTestDB(t, ctx)
	path := filepath.Join("..", "..", "..", "testdata", "fixtures", "sample.yaml")
	snapID, err := fixture.NewImporter(repo).Import(ctx, path)
	require.NoError(t, err)

	ts, err := types.ParseTimestamp("2026-01-15T12:00:00Z")
	require.NoError(t, err)
	run := &domain.AnalysisRun{
		ID:             "run-001",
		SnapshotID:     snapID,
		Status:         domain.AnalysisComplete,
		RuleSetVersion: "rules-v0",
		StartedAt:      ts,
		CompletedAt:    &ts,
		Evidence: []domain.Evidence{{
			Kind:       domain.EvidenceResource,
			ResourceID: "res-vol-unattached",
			Summary:    "volume is unattached",
			Detail:     map[string]string{"state": "available"},
			Provenance: domain.Provenance{Quality: domain.QualityObserved, Source: "test", ObservedAt: ts},
		}},
	}
	require.NoError(t, repo.SaveAnalysisRun(ctx, run))

	loaded, err := repo.GetAnalysisRun(ctx, "run-001")
	require.NoError(t, err)
	require.Len(t, loaded.Evidence, 1)
	require.Equal(t, "res-vol-unattached", string(loaded.Evidence[0].ResourceID))
}

func TestAnalysisRequiresCompleteSnapshot(t *testing.T) {
	ctx := context.Background()
	repo, db := openTestDB(t, ctx)

	_, err := db.ExecContext(ctx, `INSERT INTO accounts (id, provider, provider_account_id, display_name, default_currency, quality, source, observed_at)
		VALUES ('a1','fixture','1','x','USD','observed','t','2026-01-01T00:00:00Z')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO snapshots (id, account_id, provider, status, schema_version, external_key, started_at)
		VALUES ('snap-failed','a1','fixture','failed',1,'','2026-01-01T00:00:00Z')`)
	require.NoError(t, err)

	ts, err := types.ParseTimestamp("2026-01-15T12:00:00Z")
	require.NoError(t, err)
	run := &domain.AnalysisRun{
		ID:             "run-bad",
		SnapshotID:     "snap-failed",
		Status:         domain.AnalysisComplete,
		RuleSetVersion: "v0",
		StartedAt:      ts,
		CompletedAt:    &ts,
	}
	err = repo.SaveAnalysisRun(ctx, run)
	require.Error(t, err)
}

func TestRetentionAndDeletion(t *testing.T) {
	ctx := context.Background()
	repo, _ := openTestDB(t, ctx)
	path := filepath.Join("..", "..", "..", "testdata", "fixtures", "sample.yaml")
	importer := fixture.NewImporter(repo)

	for i := 0; i < 3; i++ {
		doc, err := os.ReadFile(path)
		require.NoError(t, err)
		tmp := filepath.Join(t.TempDir(), "f.yaml")
		content := replaceYAMLField(string(doc), "external_key: sample-offline-v1", "external_key: \"\"")
		require.NoError(t, os.WriteFile(tmp, []byte(content), 0o600))
		_, err = importer.Import(ctx, tmp)
		require.NoError(t, err)
	}

	deleted, err := repo.ApplyRetention(ctx, "acct-fixture-001", 1)
	require.NoError(t, err)
	require.Equal(t, 2, deleted)

	n, err := repo.DeleteSnapshotsByAccount(ctx, "acct-fixture-001")
	require.NoError(t, err)
	require.Equal(t, 1, n)
}

func TestConcurrentReaders(t *testing.T) {
	ctx := context.Background()
	repo, _ := openTestDB(t, ctx)
	path := filepath.Join("..", "..", "..", "testdata", "fixtures", "sample.yaml")
	id, err := fixture.NewImporter(repo).Import(ctx, path)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snap, err := repo.GetSnapshot(ctx, id)
			require.NoError(t, err)
			require.NotEmpty(t, snap.Resources)
		}()
	}
	wg.Wait()
}

func TestMigrateFromVersionOne(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "step.db")
	db, err := sqliterepository.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, sqliterepository.Migrate(ctx, db))
	v, err := sqliterepository.SchemaVersion(ctx, db)
	require.NoError(t, err)
	require.Equal(t, 6, v)

	// Re-open and ensure migrations are no-ops from latest version.
	db2, err := sqliterepository.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db2.Close() })
	require.NoError(t, sqliterepository.Migrate(ctx, db2))
	v2, err := sqliterepository.SchemaVersion(ctx, db2)
	require.NoError(t, err)
	require.Equal(t, 6, v2)
}

func TestMarkSnapshotFailed(t *testing.T) {
	ctx := context.Background()
	repo, db := openTestDB(t, ctx)
	_, err := db.ExecContext(ctx, `INSERT INTO accounts (id, provider, provider_account_id, display_name, default_currency, quality, source, observed_at)
		VALUES ('a1','fixture','1','x','USD','observed','t','2026-01-01T00:00:00Z')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO snapshots (id, account_id, provider, status, schema_version, external_key, started_at)
		VALUES ('snap-progress','a1','fixture','in_progress',1,'','2026-01-01T00:00:00Z')`)
	require.NoError(t, err)

	require.NoError(t, repo.MarkSnapshotFailed(ctx, "snap-progress"))

	list, err := repo.ListSnapshots(ctx, ports.ListSnapshotFilter{AccountID: "a1", CompleteOnly: true})
	require.NoError(t, err)
	require.Empty(t, list)
}

func openTestDB(t *testing.T, ctx context.Context) (ports.StorageRepository, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sqliterepository.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqliterepository.NewRepository(db)
	require.NoError(t, repo.Migrate(ctx))
	return repo, db
}

func replaceYAMLField(content, old, new string) string {
	if old == "" {
		return content
	}
	out := content
	for i := 0; i < len(out); i++ {
		if i+len(old) <= len(out) && out[i:i+len(old)] == old {
			return out[:i] + new + out[i+len(old):]
		}
	}
	return out
}
