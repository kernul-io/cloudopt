package rules_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/adapters/fixture"
	sqliterepository "github.com/kernul-io/cloudopt/internal/adapters/sqlite-repository"
	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/rules"
)

func TestFixtureAnalysisPredictableFindings(t *testing.T) {
	snap := loadFixtureSnapshot(t)
	reg := rules.DefaultRegistry(nil)
	manifest, err := rules.LoadManifest("", reg)
	require.NoError(t, err)

	engine := rules.Engine{}
	out, err := engine.Analyze(rules.AnalyzeInput{
		Snapshot:     snap,
		Manifest:     manifest,
		Registry:     reg,
		Suppressions: rules.NewSuppressionIndex(nil, snap.StartedAt.Time),
	})
	require.NoError(t, err)
	require.Equal(t, 4, out.Summary.Failed)
	require.Equal(t, 0, out.Summary.Passed)
	require.Len(t, out.Findings, 7)

	rulesSeen := map[string]int{}
	for _, f := range out.Findings {
		require.NotEmpty(t, f.Fingerprint)
		require.NotEmpty(t, f.EvidenceIDs)
		rulesSeen[f.RuleID]++
	}
	require.Equal(t, 1, rulesSeen["compute.stopped_instance_storage_cost"])
	require.Equal(t, 1, rulesSeen["storage.unattached_block_volume"])
	require.Equal(t, 1, rulesSeen["storage.stale_volume_snapshot"])
	require.Equal(t, 4, rulesSeen["governance.missing_cost_allocation_tags"])

	out2, err := engine.Analyze(rules.AnalyzeInput{
		Snapshot:     snap,
		Manifest:     manifest,
		Registry:     reg,
		Suppressions: rules.NewSuppressionIndex(nil, snap.StartedAt.Time),
	})
	require.NoError(t, err)
	for i := range out.Findings {
		require.Equal(t, out.Findings[i].Fingerprint, out2.Findings[i].Fingerprint)
	}
}

func TestStoppedInstanceThresholdBoundary(t *testing.T) {
	snap := &domain.CollectionSnapshot{
		ID:            "snap-test",
		Status:        domain.SnapshotComplete,
		SchemaVersion: 1,
		StartedAt:     mustTS(t, "2026-01-15T12:00:00Z"),
		CompletedAt:   ptrTS(mustTS(t, "2026-01-15T12:00:00Z")),
		Resources: []domain.Resource{
			{ID: "res-i-stopped", Kind: domain.KindComputeInstance, State: "stopped", Name: "x"},
		},
		Costs: []domain.CostRecord{
			{
				ResourceID: "res-i-stopped",
				Amount:     types.FromMajorUnits(1.50, "USD", 100),
			},
		},
	}
	reg := rules.DefaultRegistry(nil)
	manifest, err := rules.LoadManifest("", reg)
	require.NoError(t, err)

	out, err := rules.Engine{}.Analyze(rules.AnalyzeInput{Snapshot: snap, Manifest: manifest, Registry: reg})
	require.NoError(t, err)
	var stoppedFindings int
	for _, f := range out.Findings {
		if f.RuleID == "compute.stopped_instance_storage_cost" {
			stoppedFindings++
		}
	}
	require.Equal(t, 1, stoppedFindings)

	snap.Costs[0].Amount = types.FromMajorUnits(0.00, "USD", 100)
	out, err = rules.Engine{}.Analyze(rules.AnalyzeInput{Snapshot: snap, Manifest: manifest, Registry: reg})
	require.NoError(t, err)
	stoppedFindings = 0
	for _, f := range out.Findings {
		if f.RuleID == "compute.stopped_instance_storage_cost" {
			stoppedFindings++
		}
	}
	require.Equal(t, 0, stoppedFindings)
}

func TestMissingSignalsNotEvaluated(t *testing.T) {
	snap := &domain.CollectionSnapshot{
		ID:            "snap-test",
		Status:        domain.SnapshotComplete,
		StartedAt:     mustTS(t, "2026-01-15T12:00:00Z"),
		CompletedAt:   ptrTS(mustTS(t, "2026-01-15T12:00:00Z")),
		Resources:     []domain.Resource{{ID: "res-i-stopped", Kind: domain.KindComputeInstance, State: "stopped"}},
		Costs:         nil,
		Relationships: []domain.Relationship{},
	}
	reg := rules.DefaultRegistry(nil)
	manifest, err := rules.LoadManifest("", reg)
	require.NoError(t, err)

	out, err := rules.Engine{}.Analyze(rules.AnalyzeInput{Snapshot: snap, Manifest: manifest, Registry: reg})
	require.NoError(t, err)
	require.GreaterOrEqual(t, out.Summary.NotEvaluated, 1)
}

func TestSuppressionExpiration(t *testing.T) {
	snap := loadFixtureSnapshot(t)
	reg := rules.DefaultRegistry(nil)
	manifest, err := rules.LoadManifest("", reg)
	require.NoError(t, err)

	fp := rules.Fingerprint("storage.unattached_block_volume", "1.0.0", []types.ResourceID{"res-vol-unattached"})
	supp := rules.NewSuppressionIndex([]rules.SuppressionEntry{
		{
			Fingerprint: fp,
			Reason:      "accepted for demo",
			ExpiresAt:   "2020-01-01T00:00:00Z",
		},
	}, snap.StartedAt.Time)

	out, err := rules.Engine{}.Analyze(rules.AnalyzeInput{
		Snapshot: snap, Manifest: manifest, Registry: reg, Suppressions: supp,
	})
	require.NoError(t, err)
	require.NotZero(t, out.Summary.Failed)
}

func TestInvalidManifestAggregatesErrors(t *testing.T) {
	reg := rules.DefaultRegistry(nil)
	m, err := rules.ParseManifest([]byte(`
ruleset_version: ""
rules:
  - id: bad
    version: ""
    evaluator: does_not_exist
    required_signals: [costs]
`))
	require.NoError(t, err)
	errs := m.Validate(reg)
	require.Greater(t, len(errs), 1)
}

func loadFixtureSnapshot(t *testing.T) *domain.CollectionSnapshot {
	t.Helper()
	ctx := context.Background()
	repo := openTestRepo(t)
	importer := fixture.NewImporter(repo)
	fixturePath := filepath.Join("..", "..", "..", "testdata", "fixtures", "sample.yaml")
	id, err := importer.Import(ctx, fixturePath)
	require.NoError(t, err)
	snap, err := repo.GetSnapshot(ctx, id)
	require.NoError(t, err)
	return snap
}

func openTestRepo(t *testing.T) *sqliterepository.Repository {
	t.Helper()
	db, err := sqliterepository.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqliterepository.NewRepository(db)
	require.NoError(t, repo.Migrate(context.Background()))
	return repo
}

func mustTS(t *testing.T, s string) types.Timestamp {
	t.Helper()
	ts, err := types.ParseTimestamp(s)
	require.NoError(t, err)
	return ts
}

func ptrTS(ts types.Timestamp) *types.Timestamp {
	return &ts
}
