package rules

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestGCEIdleInstanceEligibility(t *testing.T) {
	tests := []struct {
		name     string
		resource domain.Resource
		idle     float64
		coverage float64
		want     int
	}{
		{
			name:     "idle running GCE instance",
			resource: gceTestInstance("res-gce", "RUNNING"),
			idle:     3,
			coverage: 0.5,
			want:     1,
		},
		{
			name:     "below idle threshold",
			resource: gceTestInstance("res-gce", "RUNNING"),
			idle:     2,
			coverage: 1,
		},
		{
			name:     "below coverage threshold",
			resource: gceTestInstance("res-gce", "RUNNING"),
			idle:     3,
			coverage: 0.49,
		},
		{
			name:     "stopped GCE instance",
			resource: gceTestInstance("res-gce", "TERMINATED"),
			idle:     3,
			coverage: 1,
		},
		{
			name: "AWS instance",
			resource: domain.Resource{
				ID:                 "res-aws",
				Kind:               domain.KindComputeInstance,
				ProviderResourceID: "i-123",
				State:              "running",
				Attributes:         map[string]string{"instance_type": "m5.large"},
			},
			idle:     3,
			coverage: 1,
		},
	}

	rule := RuleSpec{
		Title: "Idle GCE instance",
		Thresholds: map[string]string{
			"min_idle_periods":            "3",
			"min_sample_coverage_percent": "50",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := gceIdleSnapshot(tt.resource, tt.idle, tt.coverage)
			result := (GCEIdleInstance{}).Evaluate(NewSnapshotView(snapshot, nil), rule)
			require.False(t, result.NotEvaluated)
			require.Len(t, result.Findings, tt.want)
		})
	}
}

func TestGCEIdleInstanceSavingsHandling(t *testing.T) {
	observed := testTimestamp(t, "2026-09-01T00:00:00Z")
	resource := gceTestInstance("res-gce", "RUNNING")
	snapshot := gceIdleSnapshot(resource, 3, 1)
	snapshot.CompletedAt = &observed
	rule := RuleSpec{Title: "Idle GCE instance", Thresholds: map[string]string{"min_idle_periods": "3"}}

	currentCatalog := pricing.NewCatalog([]domain.PricingRecord{{
		Service:       "ComputeEngine",
		Region:        "us-central1",
		PurchaseModel: domain.PurchaseOnDemand,
		Currency:      "USD",
		EffectiveDate: testTimestamp(t, "2026-08-01T00:00:00Z"),
		Unit:          "hour",
		PriceMinor:    10,
		Attributes:    map[string]string{"machine_type": "n2-standard-2"},
	}}, "test")
	result := (GCEIdleInstance{Catalog: currentCatalog}).Evaluate(NewSnapshotView(snapshot, currentCatalog), rule)
	require.Len(t, result.Findings, 1)
	require.NotNil(t, result.Findings[0].Savings)
	require.False(t, result.Findings[0].Savings.InvestigationOnly)
	require.Positive(t, result.Findings[0].Savings.Estimate.GrossMonthlyMinor)

	staleCatalog := pricing.NewCatalog([]domain.PricingRecord{{
		Service:       "ComputeEngine",
		Region:        "us-central1",
		PurchaseModel: domain.PurchaseOnDemand,
		Currency:      "USD",
		EffectiveDate: testTimestamp(t, "2025-01-01T00:00:00Z"),
		Unit:          "hour",
		PriceMinor:    10,
		Attributes:    map[string]string{"machine_type": "n2-standard-2"},
	}}, "test")
	result = (GCEIdleInstance{Catalog: staleCatalog}).Evaluate(NewSnapshotView(snapshot, staleCatalog), rule)
	require.Len(t, result.Findings, 1)
	require.True(t, result.Findings[0].Savings.InvestigationOnly)
	require.Zero(t, result.Findings[0].Savings.Estimate.GrossMonthlyMinor)

	result = (GCEIdleInstance{}).Evaluate(NewSnapshotView(snapshot, nil), rule)
	require.Len(t, result.Findings, 1)
	require.Nil(t, result.Findings[0].Savings)
}

func TestAWSIdleElasticIP(t *testing.T) {
	tests := []struct {
		name     string
		resource domain.Resource
		want     int
	}{
		{
			name: "unassociated allocation",
			resource: domain.Resource{
				ID:                 "res-eip-idle",
				Kind:               domain.KindElasticIP,
				ProviderResourceID: "eipalloc-123",
				Name:               "198.51.100.1",
				State:              "available",
			},
			want: 1,
		},
		{
			name: "legacy address identifier",
			resource: domain.Resource{
				ID:                 "res-eip-legacy",
				Kind:               domain.KindElasticIP,
				ProviderResourceID: "198.51.100.2",
				Name:               "198.51.100.2",
				State:              "AVAILABLE",
			},
			want: 1,
		},
		{
			name: "associated allocation",
			resource: domain.Resource{
				ID:                 "res-eip-used",
				Kind:               domain.KindElasticIP,
				ProviderResourceID: "eipalloc-456",
				State:              "associated",
			},
		},
		{
			name: "GCP reserved address",
			resource: domain.Resource{
				ID:                 "res-gcp-ip",
				Kind:               domain.KindElasticIP,
				ProviderResourceID: "projects/test/regions/us-central1/addresses/ip",
				State:              "RESERVED",
				Attributes:         map[string]string{"gcp_project": "test"},
			},
		},
	}

	rule := RuleSpec{Title: "Idle AWS Elastic IP"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := &domain.CollectionSnapshot{Resources: []domain.Resource{tt.resource}}
			result := (AWSIdleElasticIP{}).Evaluate(NewSnapshotView(snapshot, nil), rule)
			require.Len(t, result.Findings, tt.want)
			if tt.want == 1 {
				require.NotEmpty(t, result.Findings[0].Evidence)
				require.Nil(t, result.Findings[0].Savings)
			}
		})
	}
}

func gceTestInstance(id types.ResourceID, state string) domain.Resource {
	return domain.Resource{
		ID:                 id,
		Kind:               domain.KindComputeInstance,
		ProviderResourceID: "projects/test/zones/us-central1-a/instances/vm",
		RegionID:           "region-us-central1",
		Name:               "vm",
		State:              state,
		Attributes: map[string]string{
			"gcp_self_link": "projects/test/zones/us-central1-a/instances/vm",
			"gcp_project":   "test",
			"machine_type":  "n2-standard-2",
		},
	}
}

func gceIdleSnapshot(resource domain.Resource, idle, coverage float64) *domain.CollectionSnapshot {
	observed := types.NewTimestamp(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	return &domain.CollectionSnapshot{
		StartedAt:   types.NewTimestamp(time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)),
		CompletedAt: &observed,
		Resources:   []domain.Resource{resource},
		Regions: []domain.Region{{
			ID:               "region-us-central1",
			ProviderRegionID: "us-central1",
		}},
		UtilizationSignals: []domain.UtilizationSignal{{
			ResourceID:    resource.ID,
			MetricName:    "CPUUtilization",
			Kind:          domain.SignalIdlePeriods,
			Value:         idle,
			CoverageRatio: coverage,
		}},
	}
}

func testTimestamp(t *testing.T, raw string) types.Timestamp {
	t.Helper()
	ts, err := types.ParseTimestamp(raw)
	require.NoError(t, err)
	return ts
}

func TestOrphanedVolumeSnapshot(t *testing.T) {
	vol := domain.Resource{
		ID:                 "res-vol",
		Kind:               domain.KindBlockVolume,
		ProviderResourceID: "vol-123",
	}
	orphanAWS := domain.Resource{
		ID:                 "res-snap-aws",
		Kind:               domain.KindSnapshot,
		ProviderResourceID: "snap-1",
		State:              "completed",
		Attributes: map[string]string{
			"volume_id":  "vol-deleted",
			"started_at": "2026-01-01T00:00:00Z",
		},
	}
	orphanGCP := domain.Resource{
		ID:                 "res-snap-gcp",
		Kind:               domain.KindSnapshot,
		ProviderResourceID: "projects/p/global/snapshots/snap-2",
		State:              "READY",
		Attributes: map[string]string{
			"source_disk": "projects/p/zones/us-central1-a/disks/disk-deleted",
			"created_at":  "2026-01-01T00:00:00Z",
		},
	}
	valid := domain.Resource{
		ID:                 "res-snap-valid",
		Kind:               domain.KindSnapshot,
		ProviderResourceID: "snap-3",
		State:              "completed",
		Attributes: map[string]string{
			"volume_id":  "vol-123",
			"started_at": "2026-01-01T00:00:00Z",
		},
	}
	tooRecent := domain.Resource{
		ID:                 "res-snap-recent",
		Kind:               domain.KindSnapshot,
		ProviderResourceID: "snap-4",
		State:              "completed",
		Attributes: map[string]string{
			"volume_id":  "vol-deleted",
			"started_at": "2026-09-23T00:00:00Z",
		},
	}

	observed := types.NewTimestamp(time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))
	snap := &domain.CollectionSnapshot{
		StartedAt:   observed,
		CompletedAt: &observed,
		Resources:   []domain.Resource{vol, orphanAWS, orphanGCP, valid, tooRecent},
	}

	rule := RuleSpec{
		Title:      "Orphaned volume snapshot",
		Thresholds: map[string]string{"min_age_days": "7"},
	}
	result := (OrphanedVolumeSnapshot{}).Evaluate(NewSnapshotView(snap, nil), rule)
	require.Len(t, result.Findings, 2)
	require.Equal(t, []types.ResourceID{"res-snap-aws"}, result.Findings[0].ResourceIDs)
	require.Equal(t, []types.ResourceID{"res-snap-gcp"}, result.Findings[1].ResourceIDs)
}
