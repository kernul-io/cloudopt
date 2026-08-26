package benchmark_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/adapters/config"
	"github.com/kernul-io/cloudopt/internal/application/api"
)

func BenchmarkEngagementAnalyze_small(b *testing.B) {
	runEngagementBench(b, filepath.Join("..", "..", "..", "testdata", "fixtures", "aws-minimal.yaml"))
}

func BenchmarkEngagementAnalyze_medium(b *testing.B) {
	runEngagementBench(b, filepath.Join("..", "..", "..", "testdata", "fixtures", "gcp-minimal.yaml"))
}

func BenchmarkEngagementAnalyze_large(b *testing.B) {
	runEngagementBench(b, filepath.Join("..", "..", "..", "testdata", "fixtures", "multicloud-engagement.yaml"))
}

func runEngagementBench(b *testing.B, fixture string) {
	b.Helper()
	dir := b.TempDir()
	settings := config.Settings{
		WorkspaceDir: dir,
		ConfigDir:    filepath.Join(dir, "config"),
		DataDir:      filepath.Join(dir, "data"),
		ReportsDir:   filepath.Join(dir, "reports"),
		TempDir:      filepath.Join(dir, "tmp"),
		LogFormat:    "text",
		LogLevel:     "error",
	}
	rt := api.NewRuntime(settings)
	require.NoError(b, rt.Init(context.Background()))
	_, err := rt.ImportFixture(context.Background(), fixture)
	require.NoError(b, err)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		require.NoError(b, rt.Analyze(context.Background(), api.AnalyzeOptions{Persist: true}))
	}
}
