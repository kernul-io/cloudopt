package cli_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/adapters/cli"
	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestOfflineGCPCollectAnalyze(t *testing.T) {
	dir := t.TempDir()
	settings, err := cli.LoadSettings(cli.Config{}.Overrides)
	require.NoError(t, err)
	settings.WorkspaceDir = dir
	settings.ConfigDir = dir
	settings.DataDir = dir
	settings.ReportsDir = filepath.Join(dir, "reports")
	settings.TempDir = filepath.Join(dir, "tmp")
	require.NoError(t, api.NewRuntime(settings).Init(context.Background()))

	rt := api.NewRuntime(settings)
	root := filepath.Join("..", "..", "..", "testdata", "gcp-inventory")
	res, err := rt.Collect(context.Background(), ports.CollectOptions{
		Provider:    types.ProviderGCP,
		Offline:     true,
		FixtureRoot: root,
		Regions:     []string{"us-central1"},
		Zones:       []string{"us-central1-a"},
		Projects:    []string{"app-workloads-demo", "shared-net-demo"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.SnapshotID)

	billingRoot := filepath.Join("..", "..", "..", "testdata", "gcp-billing")
	_, err = rt.CollectCost(context.Background(), ports.CostCollectOptions{
		Provider:    types.ProviderGCP,
		Offline:     true,
		FixtureRoot: billingRoot,
		SnapshotID:  res.SnapshotID,
	})
	require.NoError(t, err)

	metricsRoot := filepath.Join("..", "..", "..", "testdata", "gcp-metrics")
	_, err = rt.CollectMetrics(context.Background(), ports.MetricsCollectOptions{
		Provider:    types.ProviderGCP,
		Offline:     true,
		FixtureRoot: metricsRoot,
		SnapshotID:  res.SnapshotID,
	})
	require.NoError(t, err)

	require.NoError(t, rt.Analyze(context.Background(), ports.AnalyzeOptions{Persist: true, SnapshotID: res.SnapshotID}))
}
