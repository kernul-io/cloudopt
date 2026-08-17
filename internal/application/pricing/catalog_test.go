package pricing_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	awspricing "github.com/kernul-io/cloudopt/internal/adapters/aws-pricing"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestEC2FamilyConstraintRejectsCrossArchitecture(t *testing.T) {
	cat := loadDemoCatalog(t)
	sel := cat.SortedEC2Alternatives("us-east-1", "m5.large", pricing.EC2CandidateConfig{HeadroomBasisPoints: 1500})
	require.NotNil(t, sel.Accepted)
	require.Equal(t, "m5.small", sel.Accepted.TargetType)
	var sawARMReject bool
	for _, rej := range sel.Rejected {
		if rej.TargetType == "m5a.large" {
			require.Contains(t, rej.Reason, "incompatible instance family")
			sawARMReject = true
		}
	}
	require.True(t, sawARMReject)
}

func TestBurstableMicroRequiresHeadroom(t *testing.T) {
	cat := loadDemoCatalog(t)
	sel := cat.SortedEC2Alternatives("us-east-1", "t3.small", pricing.EC2CandidateConfig{HeadroomBasisPoints: 500})
	var microRejected bool
	for _, rej := range sel.Rejected {
		if rej.TargetType == "t3.micro" {
			require.Contains(t, rej.Reason, "headroom")
			microRejected = true
		}
	}
	require.True(t, microRejected)
}

func TestStalePricingDetection(t *testing.T) {
	effective := types.NewTimestamp(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	observed := types.NewTimestamp(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	require.True(t, pricing.StalePricing(effective, observed, 180*24*time.Hour))
}

func loadDemoCatalog(t *testing.T) *pricing.Catalog {
	t.Helper()
	col := awspricing.NewCollector("../../../testdata/aws-pricing")
	res, err := col.LoadCatalog(context.Background(), ports.PricingLoadOptions{Provider: types.ProviderAWS, Offline: true, FixtureRoot: "../../../testdata/aws-pricing"})
	require.NoError(t, err)
	return pricing.NewCatalog(res.Records, res.Source)
}
