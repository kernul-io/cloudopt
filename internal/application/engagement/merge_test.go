package engagement_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/engagement"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestMergeSnapshotsPreservesCurrencies(t *testing.T) {
	aws := &domain.CollectionSnapshot{
		Provider: types.ProviderAWS,
		Status:   domain.SnapshotComplete,
		Account: domain.Account{
			ID:              "acct-aws",
			DisplayName:     "AWS",
			DefaultCurrency: "USD",
		},
		Resources: []domain.Resource{{
			ID:                 "res-a",
			Kind:               domain.KindComputeInstance,
			RegionID:           "reg-a",
			ProviderResourceID: "i-abc",
			Attributes:         map[string]string{"cloud_provider": "aws"},
		}},
		Regions: []domain.Region{{ID: "reg-a", ProviderRegionID: "us-east-1"}},
		Costs: []domain.CostRecord{{
			ResourceID: "res-a",
			Service:    "AmazonEC2",
			Amount:     types.FromMajorUnits(10, "USD", 100),
		}},
	}
	gcp := &domain.CollectionSnapshot{
		Provider: types.ProviderGCP,
		Status:   domain.SnapshotComplete,
		Account: domain.Account{
			ID:              "acct-gcp",
			DisplayName:     "GCP",
			DefaultCurrency: "EUR",
		},
		Resources: []domain.Resource{{
			ID:                 "res-g",
			Kind:               domain.KindComputeInstance,
			RegionID:           "reg-g",
			ProviderResourceID: "gce-1",
			Attributes:         map[string]string{"gcp_project": "demo"},
		}},
		Regions: []domain.Region{{ID: "reg-g", ProviderRegionID: "us-central1"}},
		Costs: []domain.CostRecord{{
			ResourceID: "res-g",
			Service:    "Compute Engine",
			Amount:     types.FromMajorUnits(20, "EUR", 100),
		}},
	}

	merged, err := engagement.MergeSnapshots("demo", "ext-1", []*domain.CollectionSnapshot{aws, gcp})
	require.NoError(t, err)
	require.Equal(t, types.ProviderMulti, merged.Provider)
	require.Len(t, merged.Engagement.Members, 2)

	portfolio := engagement.BuildPortfolio(merged)
	require.Equal(t, int64(1000), portfolio.ByProvider["aws:USD"])
	require.Equal(t, int64(2000), portfolio.ByProvider["gcp:EUR"])
	require.NotContains(t, portfolio.ByProvider, "aws:EUR")
}
