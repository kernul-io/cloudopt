package pricing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/domain"
)

func TestSortedGCEAlternatives(t *testing.T) {
	c := NewCatalog([]domain.PricingRecord{
		{Service: "ComputeEngine", Region: "us-central1", PurchaseModel: domain.PurchaseOnDemand, Unit: "hour", PriceMinor: 19400, Currency: "USD", Attributes: map[string]string{"machine_type": "n2-standard-4"}},
		{Service: "ComputeEngine", Region: "us-central1", PurchaseModel: domain.PurchaseOnDemand, Unit: "hour", PriceMinor: 9700, Currency: "USD", Attributes: map[string]string{"machine_type": "n2-standard-2"}},
		{Service: "ComputeEngine", Region: "us-central1", PurchaseModel: domain.PurchaseOnDemand, Unit: "hour", PriceMinor: 4850, Currency: "USD", Attributes: map[string]string{"machine_type": "n2-standard-1"}},
	}, "test")
	sel := c.SortedGCEAlternatives("us-central1", "n2-standard-4", GCECandidateConfig{HeadroomBasisPoints: 1500})
	require.NotNil(t, sel.Accepted)
	require.Equal(t, "n2-standard-1", sel.Accepted.TargetType)
}

func TestRejectCustomGCEMachine(t *testing.T) {
	c := NewCatalog(nil, "test")
	sel := c.SortedGCEAlternatives("us-central1", "custom-4-8192", GCECandidateConfig{})
	require.Nil(t, sel.Accepted)
	require.NotEmpty(t, sel.Rejected)
}
