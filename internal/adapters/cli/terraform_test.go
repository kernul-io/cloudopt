package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/adapters/cli"
	"github.com/kernul-io/cloudopt/internal/application/api"
)

func TestTerraformCorrelateFlow(t *testing.T) {
	work := t.TempDir()
	overrides := cli.Config{}.Overrides
	overrides.WorkspaceDir = work

	settings, err := cli.LoadSettings(overrides)
	require.NoError(t, err)
	rt := api.NewRuntime(settings)
	require.NoError(t, rt.Init(context.Background()))

	fixture := filepath.Join("..", "..", "..", "testdata", "fixtures", "aws-minimal.yaml")
	_, err = rt.ImportFixture(context.Background(), fixture)
	require.NoError(t, err)
	require.NoError(t, rt.Analyze(context.Background(), api.AnalyzeOptions{Persist: true}))

	statePath := filepath.Join("..", "..", "..", "testdata", "terraform", "state-multicloud.json")
	mdOut := filepath.Join(work, "tf-summary.md")
	jsonOut := filepath.Join(work, "tf.json")

	result, err := rt.TerraformCorrelate(context.Background(), api.TerraformCorrelateOptions{
		StateJSONPath:  statePath,
		PlanJSONPath:   filepath.Join("..", "..", "..", "testdata", "terraform", "plan-sample.json"),
		EnrichFindings: true,
		MarkdownOut:    mdOut,
		JSONOut:        jsonOut,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Result.Links)
	require.FileExists(t, mdOut)
	require.FileExists(t, jsonOut)

	b, err := os.ReadFile(mdOut)
	require.NoError(t, err)
	require.Contains(t, string(b), "Terraform correlation summary")
}
