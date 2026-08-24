package capabilities_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/capabilities"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestContractSuiteProviders(t *testing.T) {
	all, err := capabilities.AllProviderManifests()
	require.NoError(t, err)
	results := capabilities.RunContractSuite(all)
	require.NotEmpty(t, results)

	byProvider := map[types.Provider]capabilities.ContractResult{}
	for _, r := range results {
		byProvider[r.Provider] = r
	}

	require.True(t, byProvider[types.ProviderAWS].Passed)
	require.True(t, byProvider[types.ProviderAWS].Advertised)
	require.True(t, byProvider[types.ProviderGCP].Passed)
	require.True(t, byProvider[types.ProviderAzure].Passed)
	require.False(t, byProvider[types.ProviderAzure].Advertised)
	require.True(t, byProvider[types.ProviderDigitalOcean].Passed)
	require.False(t, byProvider[types.ProviderDigitalOcean].Advertised)
	require.True(t, byProvider[types.Provider("incomplete-fake")].Passed)
}

func TestCapabilityMatrixAWSGCP(t *testing.T) {
	all, err := capabilities.AllProviderManifests()
	require.NoError(t, err)
	matrix := capabilities.MatrixForScope(all, []types.Provider{types.ProviderAWS, types.ProviderGCP})
	require.NotEmpty(t, matrix.Providers)
	require.Contains(t, matrix.Providers, "aws")
	require.Contains(t, matrix.Providers, "gcp")
	require.NotEmpty(t, matrix.Rows)
}

func TestMixedCurrencyPortfolioDoesNotConvert(t *testing.T) {
	// Covered in engagement package test; sanity check category normalization.
	cat := capabilities.ServiceCategory(types.ProviderGCP, "Compute Engine")
	require.Equal(t, "compute", cat)
}
