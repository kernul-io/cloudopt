package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/adapters/cli"
	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

func TestOfflineAWSCollectAnalyzeReport(t *testing.T) {
	dir := t.TempDir()
	settings, err := cli.LoadSettings(cli.Config{}.Overrides)
	require.NoError(t, err)
	settings.WorkspaceDir = dir
	settings.ConfigDir = dir
	settings.DataDir = dir
	settings.ReportsDir = filepath.Join(dir, "reports")
	settings.TempDir = filepath.Join(dir, "tmp")
	for _, d := range []string{settings.ConfigDir, settings.DataDir, settings.ReportsDir, settings.TempDir} {
		require.NoError(t, os.MkdirAll(d, 0o750))
	}

	rt := api.NewRuntime(settings)
	require.NoError(t, rt.Init(context.Background()))

	root := filepath.Join("..", "..", "..", "testdata", "aws-inventory")
	res, err := rt.Collect(context.Background(), ports.CollectOptions{
		Provider:    "aws",
		Offline:     true,
		FixtureRoot: root,
		Regions:     []string{"us-east-1"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.SnapshotID)

	require.NoError(t, rt.Analyze(context.Background(), ports.AnalyzeOptions{Persist: true}))
	require.NoError(t, rt.Report(context.Background(), ports.ReportOptions{Format: ports.ReportJSON}))
}
