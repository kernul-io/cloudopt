package gcpinventory

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func testFixtureRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "testdata", "gcp-inventory")
}

func TestFixtureCollector_offlineInventory(t *testing.T) {
	collector, err := NewFixtureCollector(testFixtureRoot(t))
	require.NoError(t, err)

	opts := ports.CollectOptions{
		Provider: types.ProviderGCP,
		Regions:  []string{"us-central1"},
		Zones:    []string{"us-central1-a"},
		Offline:  true,
	}
	pf, err := collector.Preflight(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, "cloudopt-demo-host", pf.ProviderAccountID)
	require.GreaterOrEqual(t, len(pf.SelectedProjects), 2)

	snap, err := collector.Collect(context.Background(), opts, ports.NopProgress{})
	require.NoError(t, err)
	require.NotEqual(t, domain.SnapshotFailed, snap.Status)
	require.NotEmpty(t, snap.Resources)

	var instances, disks, sql, gke int
	for _, r := range snap.Resources {
		switch r.Kind {
		case domain.KindComputeInstance:
			instances++
		case domain.KindBlockVolume:
			disks++
		case domain.KindDatabase:
			sql++
		case domain.KindKubernetesCluster:
			gke++
		}
	}
	require.GreaterOrEqual(t, instances, 2)
	require.GreaterOrEqual(t, disks, 1)
	require.Equal(t, 1, sql)
	require.Equal(t, 1, gke)
}

func TestFilterRegions_gcp(t *testing.T) {
	got := filterRegions([]string{"us-central1", "europe-west1"}, []string{"us-central1"}, nil)
	require.Equal(t, []string{"us-central1"}, got)
}

func TestPaginateFixtureInstances(t *testing.T) {
	total, err := PaginateFixtureInstances(testFixtureRoot(t), "app-workloads-demo", "us-central1-a")
	require.NoError(t, err)
	require.Equal(t, 3, total)
}

func TestFixture_disabledAPIProject(t *testing.T) {
	backend := &FixtureBackend{Root: testFixtureRoot(t), pages: map[string]int{}}
	collector := NewCollector(backend)
	opts := ports.CollectOptions{
		Projects: []string{"api-disabled-demo"},
		Regions:  []string{"us-central1"},
	}
	pf, err := collector.Preflight(context.Background(), opts)
	require.NoError(t, err)
	require.NotEmpty(t, pf.MissingActions)

	_, err = collector.Collect(context.Background(), opts, ports.NopProgress{})
	require.Error(t, err)
	require.True(t, ports.IsMissingPermissions(err))
}

func TestCollect_missingPermissions(t *testing.T) {
	backend := &denyServiceUsageBackend{FixtureBackend: FixtureBackend{Root: testFixtureRoot(t), pages: map[string]int{}}}
	collector := NewCollector(backend)
	opts := ports.CollectOptions{
		Projects: []string{"app-workloads-demo"},
		Regions:  []string{"us-central1"},
	}

	_, err := collector.Collect(context.Background(), opts, ports.NopProgress{})
	require.Error(t, err)
	require.True(t, ports.IsMissingPermissions(err))
}

type denyServiceUsageBackend struct {
	FixtureBackend
}

func (d *denyServiceUsageBackend) ListEnabledServices(ctx context.Context, projectID string) ([]string, error) {
	return nil, mapGCPError("serviceusage", "services.list", projectID, "PERMISSION_DENIED", "permission denied")
}

func TestFixture_partialProject(t *testing.T) {
	root := testFixtureRoot(t)
	backend := &FixtureBackend{Root: root, pages: map[string]int{}}
	_, err := backend.CollectScoped(context.Background(), "app-workloads-demo-partial", "us-central1", "", "")
	require.Error(t, err)
	require.True(t, errorsIsAccessDenied(err))
}

func TestWithRetry_quota(t *testing.T) {
	attempts := 0
	err := withRetry(context.Background(), RetryConfig{MaxAttempts: 3, BaseDelay: 1, MaxDelay: 2}, func() error {
		attempts++
		if attempts < 2 {
			return SimulateQuotaError()
		}
		return nil
	}, retryableAPIErr)
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
}

func TestCollect_cancelled(t *testing.T) {
	collector, err := NewFixtureCollector(testFixtureRoot(t))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = collector.Collect(ctx, ports.CollectOptions{
		Regions: []string{"us-central1"},
		Zones:   []string{"us-central1-a"},
	}, ports.NopProgress{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrCancelled) || errors.Is(err, context.Canceled))
}

func TestLoadCapabilities_gcp(t *testing.T) {
	caps, err := LoadCapabilities()
	require.NoError(t, err)
	require.Equal(t, "compute_instances", caps.Inventory[0].ID)
	require.NotEmpty(t, IAMLeastPrivilegePolicy())
}

func TestProjectDiscovery_explicit(t *testing.T) {
	ids, err := FilterProjectsForTest(testFixtureRoot(t), []string{"shared-net-demo"})
	require.NoError(t, err)
	require.Equal(t, []string{"shared-net-demo"}, ids)
}

func TestGlobalResourceMapping(t *testing.T) {
	collector, err := NewFixtureCollector(testFixtureRoot(t))
	require.NoError(t, err)
	opts := ports.CollectOptions{
		Provider: types.ProviderGCP,
		Projects: []string{"app-workloads-demo", "shared-net-demo"},
		Regions:  []string{"us-central1"},
		Zones:    []string{"us-central1-a"},
	}
	snap, err := collector.Collect(context.Background(), opts, ports.NopProgress{})
	require.NoError(t, err)
	var vpc, image int
	for _, r := range snap.Resources {
		if r.Kind == domain.KindVPC {
			vpc++
		}
		if r.Kind == domain.KindMachineImage {
			image++
		}
	}
	require.GreaterOrEqual(t, vpc, 1)
	require.GreaterOrEqual(t, image, 1)
}
