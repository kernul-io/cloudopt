package billing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestReconcileCreditsRefundsAndRounding(t *testing.T) {
	source := map[string]types.Money{
		"USD": {AmountMinor: 10000, Currency: "USD"},
	}
	costs := []domain.CostRecord{
		{ResourceID: "res-1", Amount: types.Money{AmountMinor: 8000, Currency: "USD"}, Attribution: domain.CostAttribution{Method: domain.AttributionDirectResourceID}},
		{ResourceID: domain.UnattributedResourceID, Amount: types.Money{AmountMinor: -500, Currency: "USD"}, ChargeKind: domain.ChargeCredit, Attribution: domain.CostAttribution{Method: domain.AttributionUnattributed}},
		{ResourceID: "res-2", Amount: types.Money{AmountMinor: 2500, Currency: "USD"}, Attribution: domain.CostAttribution{Method: domain.AttributionDirectResourceID}},
	}
	recon := Reconcile(source, costs, DefaultReconcileToleranceBasisPoints)
	require.True(t, recon.WithinTolerance)
	require.Equal(t, int64(10000), recon.SourceTotal["USD"].AmountMinor)
	require.Equal(t, int64(10500), recon.AttributedTotal["USD"].AmountMinor)
	require.Equal(t, int64(-500), recon.UnattributedTotal["USD"].AmountMinor)
}

func TestReconcileMultipleCurrenciesNeverMixed(t *testing.T) {
	source := map[string]types.Money{
		"USD": {AmountMinor: 5000, Currency: "USD"},
		"EUR": {AmountMinor: 900, Currency: "EUR"},
	}
	costs := []domain.CostRecord{
		{ResourceID: "res-1", Amount: types.Money{AmountMinor: 5000, Currency: "USD"}, Attribution: domain.CostAttribution{Method: domain.AttributionDirectResourceID}},
		{ResourceID: domain.UnattributedResourceID, Amount: types.Money{AmountMinor: 900, Currency: "EUR"}, Attribution: domain.CostAttribution{Method: domain.AttributionUnattributed}},
	}
	recon := Reconcile(source, costs, DefaultReconcileToleranceBasisPoints)
	require.True(t, recon.WithinTolerance)
	require.Equal(t, int64(0), recon.Discrepancy["USD"].AmountMinor)
	require.Equal(t, int64(0), recon.Discrepancy["EUR"].AmountMinor)
}

func TestReconcileIncompleteMonthWithinTolerance(t *testing.T) {
	source := map[string]types.Money{"USD": {AmountMinor: 10003, Currency: "USD"}}
	costs := []domain.CostRecord{
		{ResourceID: "res-1", Amount: types.Money{AmountMinor: 10000, Currency: "USD"}, Attribution: domain.CostAttribution{Method: domain.AttributionDirectResourceID}},
	}
	recon := Reconcile(source, costs, DefaultReconcileToleranceBasisPoints)
	require.True(t, recon.WithinTolerance)
	require.Equal(t, int64(3), recon.Discrepancy["USD"].AmountMinor)
}

func TestAttributeDirectTagSharedUnattributed(t *testing.T) {
	snap := &domain.CollectionSnapshot{
		Resources: []domain.Resource{
			{ID: "res-ec2", ProviderResourceID: "i-abc", RegionID: "reg-us-east-1", Tags: []domain.Tag{{Key: "Owner", Value: "team-a"}}},
			{ID: "res-vol", ProviderResourceID: "vol-1", RegionID: "reg-us-east-1"},
		},
	}
	idx := BuildInventoryIndex(snap)
	interval := domain.BillingInterval{}
	inputs := []AttributionInput{
		{ProviderResourceID: "i-abc", Service: "EC2", Region: "us-east-1", Amount: types.FromMajorUnits(10, "USD", 100), Basis: domain.CostBasisAmortizedNet, ChargeKind: domain.ChargeUsage, Granularity: domain.CostMonthly},
		{TagOwner: "team-a", Service: "RDS", Region: "us-east-1", Amount: types.FromMajorUnits(5, "USD", 100), Basis: domain.CostBasisAmortizedNet, ChargeKind: domain.ChargeUsage, Granularity: domain.CostMonthly},
		{SharedPool: true, Service: "Support", Region: "global", Amount: types.FromMajorUnits(4, "USD", 100), Basis: domain.CostBasisAmortizedNet, ChargeKind: domain.ChargeSupport, Granularity: domain.CostMonthly},
		{ProviderResourceID: "missing", Service: "S3", Region: "us-east-1", Amount: types.FromMajorUnits(1, "USD", 100), Basis: domain.CostBasisAmortizedNet, ChargeKind: domain.ChargeUsage, Granularity: domain.CostMonthly},
	}
	out := Attribute(inputs, idx, interval, "test", types.NowUTC())
	require.Len(t, out, 5) // direct + tag + 2 shared splits + unattributed
}
