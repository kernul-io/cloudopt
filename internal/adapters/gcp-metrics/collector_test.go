package gcpmetrics

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain"
)

func TestFixtureMetricsCollection(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "gcp-metrics")
	src := NewFixtureMetricsSource(root)
	inv := &domain.CollectionSnapshot{
		Resources: []domain.Resource{
			{
				ID:                 "res-gce",
				Kind:               domain.KindComputeInstance,
				ProviderResourceID: "projects/app-workloads-demo/zones/us-central1-a/instances/app-server-1",
				Attributes:         map[string]string{"gcp_self_link": "projects/app-workloads-demo/zones/us-central1-a/instances/app-server-1"},
			},
		},
	}
	out, err := src.Collect(context.Background(), ports.MetricsCollectOptions{Offline: true, LookbackDays: 14}, inv)
	require.NoError(t, err)
	require.NotEmpty(t, out.Series)
	require.NotEmpty(t, out.Signals)
}

func TestMissingPermissionsWhenMonitoringUnavailable(t *testing.T) {
	c := NewCollector(nil)
	opts := ports.MetricsCollectOptions{LookbackDays: 14}
	pf, err := c.Preflight(context.Background(), opts)
	require.NoError(t, err)
	require.NotEmpty(t, pf.MissingActions)

	_, err = c.Collect(context.Background(), opts, &domain.CollectionSnapshot{})
	require.Error(t, err)
	require.True(t, ports.IsMissingPermissions(err))
}
