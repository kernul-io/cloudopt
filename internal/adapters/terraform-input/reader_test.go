package terraforminput_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	terraforminput "github.com/kernul-io/cloudopt/internal/adapters/terraform-input"
	"github.com/kernul-io/cloudopt/internal/application/terraform"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestLoadStateFixtureAWS(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "terraform")
	r := terraforminput.NewReader()
	resources, err := r.LoadStateFile(context.Background(), filepath.Join(root, "state-multicloud.json"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(resources), 6)

	inv := []domain.Resource{
		{ID: "res-i-running", ProviderResourceID: "i-0running01", Kind: domain.KindComputeInstance, Name: "app-server-1",
			Tags: []domain.Tag{{Key: "Owner", Value: "platform-team"}, {Key: "Project", Value: "checkout"}}},
		{ID: "res-vol-unattached", ProviderResourceID: "vol-orphan01", Kind: domain.KindBlockVolume},
	}
	result := terraform.Correlate(resources, terraform.CorrelateOptions{
		Resources: inv,
		Provider:  types.ProviderAWS,
	})
	require.Len(t, result.Links, 2)
	require.Equal(t, terraform.ConfidenceHigh, result.Links[0].Confidence)
}

func TestLoadStateAllProviders(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "terraform")
	r := terraforminput.NewReader()
	resources, err := r.LoadStateFile(context.Background(), filepath.Join(root, "state-providers.json"))
	require.NoError(t, err)
	require.Len(t, resources, 4)

	inv := []domain.Resource{
		{ID: "g", ProviderResourceID: "projects/demo/zones/us-central1-a/instances/gce-1", Kind: domain.KindComputeInstance},
		{ID: "a", ProviderResourceID: "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm-1", Kind: domain.KindComputeInstance},
		{ID: "d", ProviderResourceID: "512345678", Kind: domain.KindComputeInstance},
	}
	result := terraform.Correlate(resources, terraform.CorrelateOptions{Resources: inv, Provider: types.ProviderMulti})
	require.Len(t, result.Links, 3)
}

func TestLoadPlanSample(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "terraform")
	r := terraforminput.NewReader()
	changes, err := r.LoadPlanFile(context.Background(), filepath.Join(root, "plan-sample.json"))
	require.NoError(t, err)
	pa := terraform.AnalyzePlan(changes)
	require.GreaterOrEqual(t, pa.PolicyViolations, 1)
	require.GreaterOrEqual(t, pa.CostIncrease, 1)
	require.GreaterOrEqual(t, pa.CostDecrease, 1)
	for _, c := range pa.Changes {
		require.Equal(t, "terraform_plan", c.SourceKind)
	}
}

func TestLoadMappings(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "terraform")
	m, err := terraforminput.LoadMappings(filepath.Join(root, "mappings.yaml"))
	require.NoError(t, err)
	require.Len(t, m, 1)
}

func TestMalformedStateJSON(t *testing.T) {
	r := terraforminput.NewReader()
	_, err := r.LoadStateFile(context.Background(), filepath.Join("..", "..", "..", "testdata", "terraform", "mappings.yaml"))
	require.Error(t, err)
}
