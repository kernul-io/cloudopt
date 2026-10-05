package rules

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestCloudSQLIdleInstanceEligibility(t *testing.T) {
	tests := []struct {
		name     string
		resource domain.Resource
		idle     float64
		coverage float64
		want     int
	}{
		{
			name:     "idle runnable Cloud SQL at boundaries",
			resource: cloudSQLIdleTestDatabase("res-sql", "RUNNABLE"),
			idle:     3,
			coverage: 0.5,
			want:     1,
		},
		{
			name:     "below idle threshold",
			resource: cloudSQLIdleTestDatabase("res-sql", "RUNNABLE"),
			idle:     2,
			coverage: 1,
		},
		{
			name:     "below coverage threshold",
			resource: cloudSQLIdleTestDatabase("res-sql", "RUNNABLE"),
			idle:     3,
			coverage: 0.49,
		},
		{
			name:     "stopped Cloud SQL instance",
			resource: cloudSQLIdleTestDatabase("res-sql", "STOPPED"),
			idle:     3,
			coverage: 1,
		},
		{
			name: "AWS RDS database",
			resource: domain.Resource{
				ID:                 "res-rds",
				Kind:               domain.KindDatabase,
				ProviderResourceID: "database-1",
				State:              "available",
				Attributes: map[string]string{
					"instance_class": "db.t3.medium",
					"engine":         "postgres",
				},
			},
			idle:     3,
			coverage: 1,
		},
	}

	rule := RuleSpec{
		Title: "Persistently idle Cloud SQL instance",
		Thresholds: map[string]string{
			"min_idle_periods":            "3",
			"min_sample_coverage_percent": "50",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := cloudSQLIdleTestSnapshot(tt.resource, tt.idle, tt.coverage)
			result := (CloudSQLIdleInstance{}).Evaluate(NewSnapshotView(snapshot, nil), rule)
			require.False(t, result.NotEvaluated)
			require.Len(t, result.Findings, tt.want)
		})
	}
}

func TestCloudSQLIdleInstanceMissingMetrics(t *testing.T) {
	db := cloudSQLIdleTestDatabase("res-sql", "RUNNABLE")
	result := (CloudSQLIdleInstance{}).Evaluate(
		NewSnapshotView(&domain.CollectionSnapshot{Resources: []domain.Resource{db}}, nil),
		RuleSpec{Title: "Persistently idle Cloud SQL instance"},
	)

	require.True(t, result.NotEvaluated)
	require.Contains(t, result.Reason, "no CPU idle-period")
	require.Empty(t, result.Findings)
}

func TestCloudSQLIdleInstanceSavingsHandling(t *testing.T) {
	resource := cloudSQLIdleTestDatabase("res-sql", "RUNNABLE")
	snapshot := cloudSQLIdleTestSnapshot(resource, 3, 1)
	rule := RuleSpec{Title: "Persistently idle Cloud SQL instance"}

	currentCatalog := pricing.NewCatalog([]domain.PricingRecord{{
		Service:       "CloudSQL",
		Region:        "us-central1",
		PurchaseModel: domain.PurchaseOnDemand,
		Currency:      "USD",
		EffectiveDate: testTimestamp(t, "2026-08-01T00:00:00Z"),
		Unit:          "hour",
		PriceMinor:    82,
		Attributes: map[string]string{
			"tier":   "db-custom-2-7680",
			"engine": "postgres",
		},
	}}, "test")
	result := (CloudSQLIdleInstance{Catalog: currentCatalog}).Evaluate(NewSnapshotView(snapshot, currentCatalog), rule)
	require.Len(t, result.Findings, 1)
	require.NotNil(t, result.Findings[0].Savings)
	require.False(t, result.Findings[0].Savings.InvestigationOnly)
	require.Positive(t, result.Findings[0].Savings.Estimate.GrossMonthlyMinor)
	require.Equal(t, "database:res-sql:lifecycle", result.Findings[0].Savings.Estimate.OverlapKey)

	staleCatalog := pricing.NewCatalog([]domain.PricingRecord{{
		Service:       "CloudSQL",
		Region:        "us-central1",
		PurchaseModel: domain.PurchaseOnDemand,
		Currency:      "USD",
		EffectiveDate: testTimestamp(t, "2025-01-01T00:00:00Z"),
		Unit:          "hour",
		PriceMinor:    82,
		Attributes: map[string]string{
			"tier":   "db-custom-2-7680",
			"engine": "postgres",
		},
	}}, "test")
	result = (CloudSQLIdleInstance{Catalog: staleCatalog}).Evaluate(NewSnapshotView(snapshot, staleCatalog), rule)
	require.Len(t, result.Findings, 1)
	require.True(t, result.Findings[0].Savings.InvestigationOnly)
	require.Zero(t, result.Findings[0].Savings.Estimate.GrossMonthlyMinor)

	result = (CloudSQLIdleInstance{}).Evaluate(NewSnapshotView(snapshot, nil), rule)
	require.Len(t, result.Findings, 1)
	require.NotNil(t, result.Findings[0].Savings)
	require.True(t, result.Findings[0].Savings.InvestigationOnly)
	require.Zero(t, result.Findings[0].Savings.Estimate.GrossMonthlyMinor)
}

func TestCloudSQLIdleInstanceRejectsInvalidThreshold(t *testing.T) {
	snapshot := cloudSQLIdleTestSnapshot(cloudSQLIdleTestDatabase("res-sql", "RUNNABLE"), 3, 1)
	rule := RuleSpec{Thresholds: map[string]string{"min_idle_periods": "invalid"}}

	result := (CloudSQLIdleInstance{}).Evaluate(NewSnapshotView(snapshot, nil), rule)

	require.True(t, result.NotEvaluated)
	require.Contains(t, result.Reason, "min_idle_periods")
}

func cloudSQLIdleTestDatabase(id types.ResourceID, state string) domain.Resource {
	return domain.Resource{
		ID:                 id,
		Kind:               domain.KindDatabase,
		ProviderResourceID: "projects/demo/instances/database-1",
		RegionID:           "region-us-central1",
		Name:               "database-1",
		State:              state,
		Attributes: map[string]string{
			"gcp_project":      "demo",
			"gcp_self_link":    "https://www.googleapis.com/sql/v1beta4/projects/demo/instances/database-1",
			"tier":             "db-custom-2-7680",
			"database_version": "POSTGRES_15",
		},
	}
}

func cloudSQLIdleTestSnapshot(resource domain.Resource, idle, coverage float64) *domain.CollectionSnapshot {
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
