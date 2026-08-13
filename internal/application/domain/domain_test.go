package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/domain"
)

func TestSnapshotAnalyzable(t *testing.T) {
	complete := domain.CollectionSnapshot{Status: domain.SnapshotComplete}
	require.True(t, complete.IsAnalyzable())

	failed := domain.CollectionSnapshot{Status: domain.SnapshotFailed}
	require.False(t, failed.IsAnalyzable())

	partial := domain.CollectionSnapshot{Status: domain.SnapshotPartial}
	require.False(t, partial.IsAnalyzable())
	require.True(t, partial.IsAnalyzableAllowPartial())
}

func TestCompareSnapshotsMatch(t *testing.T) {
	a := &domain.CollectionSnapshot{
		ID: "s1", Status: domain.SnapshotComplete, SchemaVersion: 1,
		Resources: []domain.Resource{{ID: "r1", State: "running", ProviderResourceID: "p1"}},
	}
	b := *a
	b.Resources = append([]domain.Resource{}, a.Resources...)
	require.Empty(t, domain.CompareSnapshots(a, &b))
}

func TestCompareSnapshotsDetectsDiff(t *testing.T) {
	a := &domain.CollectionSnapshot{ID: "s1", Resources: []domain.Resource{{ID: "r1", State: "running"}}}
	b := &domain.CollectionSnapshot{ID: "s1", Resources: []domain.Resource{{ID: "r1", State: "stopped"}}}
	diffs := domain.CompareSnapshots(a, b)
	require.NotEmpty(t, diffs)
}
