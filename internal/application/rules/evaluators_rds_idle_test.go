package rules

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestRDSIdleInstanceEligibility(t *testing.T) {
	tests := []struct {
		name     string
		resource domain.Resource
		idle     float64
		coverage float64
		want     int
	}{
		{
			name:     "idle available RDS instance at boundaries",
			resource: rdsIdleTestDatabase("res-rds", "available"),
			idle:     3,
			coverage: 0.5,
			want:     1,
		},
		{
			name:     "below idle threshold",
			resource: rdsIdleTestDatabase("res-rds", "available"),
			idle:     2,
			coverage: 1,
		},
		{
			name:     "below coverage threshold",
			resource: rdsIdleTestDatabase("res-rds", "available"),
			idle:     3,
			coverage: 0.49,
		},
		{
			name:     "stopped RDS instance",
			resource: rdsIdleTestDatabase("res-rds", "stopped"),
			idle:     3,
			coverage: 1,
		},
		{
			name: "GCP database",
			resource: domain.Resource{
				ID:                 "res-cloudsql",
				Kind:               domain.KindDatabase,
				ProviderResourceID: "projects/test/instances/database",
				State:              "RUNNABLE",
				Attributes:         map[string]string{"gcp_project": "test", "tier": "db-n1-standard-2"},
			},
			idle:     3,
			coverage: 1,
		},
	}

	rule := RuleSpec{
		Title: "Persistently idle RDS instance",
		Thresholds: map[string]string{
			"min_idle_periods":            "3",
			"min_sample_coverage_percent": "50",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := rdsIdleTestSnapshot(tt.resource, tt.idle, tt.coverage)
			result := (RDSIdleInstance{}).Evaluate(NewSnapshotView(snapshot, nil), rule)
			require.False(t, result.NotEvaluated)
			require.Len(t, result.Findings, tt.want)
		})
	}
}

func TestRDSIdleInstanceSavingsHandling(t *testing.T) {
	resource := rdsIdleTestDatabase("res-rds", "available")
	snapshot := rdsIdleTestSnapshot(resource, 3, 1)
	snapshot.UtilizationSignals = append(snapshot.UtilizationSignals, domain.UtilizationSignal{
		ResourceID:    resource.ID,
		MetricName:    "DatabaseConnections",
		Kind:          domain.SignalMean,
		Value:         0.25,
		CoverageRatio: 1,
	})
	rule := RuleSpec{Title: "Persistently idle RDS instance"}

	currentCatalog := pricing.NewCatalog([]domain.PricingRecord{{
		Service:       "AmazonRDS",
		Region:        "us-east-1",
		PurchaseModel: domain.PurchaseOnDemand,
		Currency:      "USD",
		EffectiveDate: testTimestamp(t, "2026-08-01T00:00:00Z"),
		Unit:          "hour",
		PriceMinor:    20,
		Attributes: map[string]string{
			"instance_class": "db.t3.medium",
			"engine":         "postgres",
		},
	}}, "test")
	result := (RDSIdleInstance{Catalog: currentCatalog}).Evaluate(NewSnapshotView(snapshot, currentCatalog), rule)
	require.Len(t, result.Findings, 1)
	require.Len(t, result.Findings[0].Evidence, 2)
	require.NotNil(t, result.Findings[0].Savings)
	require.False(t, result.Findings[0].Savings.InvestigationOnly)
	require.Positive(t, result.Findings[0].Savings.Estimate.GrossMonthlyMinor)
	require.Equal(t, "database:res-rds:lifecycle", result.Findings[0].Savings.Estimate.OverlapKey)

	staleCatalog := pricing.NewCatalog([]domain.PricingRecord{{
		Service:       "AmazonRDS",
		Region:        "us-east-1",
		PurchaseModel: domain.PurchaseOnDemand,
		Currency:      "USD",
		EffectiveDate: testTimestamp(t, "2025-01-01T00:00:00Z"),
		Unit:          "hour",
		PriceMinor:    20,
		Attributes: map[string]string{
			"instance_class": "db.t3.medium",
			"engine":         "postgres",
		},
	}}, "test")
	result = (RDSIdleInstance{Catalog: staleCatalog}).Evaluate(NewSnapshotView(snapshot, staleCatalog), rule)
	require.Len(t, result.Findings, 1)
	require.True(t, result.Findings[0].Savings.InvestigationOnly)
	require.Zero(t, result.Findings[0].Savings.Estimate.GrossMonthlyMinor)
	require.Zero(t, result.Findings[0].Savings.Estimate.LowMonthlyMinor)
	require.Zero(t, result.Findings[0].Savings.Estimate.HighMonthlyMinor)

	result = (RDSIdleInstance{}).Evaluate(NewSnapshotView(snapshot, nil), rule)
	require.Len(t, result.Findings, 1)
	require.NotNil(t, result.Findings[0].Savings)
	require.True(t, result.Findings[0].Savings.InvestigationOnly)
	require.Zero(t, result.Findings[0].Savings.Estimate.GrossMonthlyMinor)
}

func TestRDSIdleInstanceRejectsInvalidThreshold(t *testing.T) {
	snapshot := rdsIdleTestSnapshot(rdsIdleTestDatabase("res-rds", "available"), 3, 1)
	rule := RuleSpec{Thresholds: map[string]string{"min_sample_coverage_percent": "invalid"}}

	result := (RDSIdleInstance{}).Evaluate(NewSnapshotView(snapshot, nil), rule)

	require.True(t, result.NotEvaluated)
	require.Contains(t, result.Reason, "min_sample_coverage_percent")
}

func rdsIdleTestDatabase(id types.ResourceID, state string) domain.Resource {
	return domain.Resource{
		ID:                 id,
		Kind:               domain.KindDatabase,
		ProviderResourceID: "database-1",
		RegionID:           "region-us-east-1",
		Name:               "database-1",
		State:              state,
		Attributes: map[string]string{
			"instance_class": "db.t3.medium",
			"engine":         "postgres",
		},
	}
}

func rdsIdleTestSnapshot(resource domain.Resource, idle, coverage float64) *domain.CollectionSnapshot {
	observed := types.NewTimestamp(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	return &domain.CollectionSnapshot{
		StartedAt:   types.NewTimestamp(time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)),
		CompletedAt: &observed,
		Resources:   []domain.Resource{resource},
		Regions: []domain.Region{{
			ID:               "region-us-east-1",
			ProviderRegionID: "us-east-1",
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
