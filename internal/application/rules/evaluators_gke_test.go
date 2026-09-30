package rules

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestGKENodePoolDownsizeCandidateEligibility(t *testing.T) {
	tests := []struct {
		name     string
		pool     domain.Resource
		p95      float64
		coverage float64
		want     int
	}{
		{name: "eligible at thresholds", pool: gkeTestNodePool("2", "RUNNING"), p95: 25, coverage: 0.5, want: 1},
		{name: "single node", pool: gkeTestNodePool("1", "RUNNING"), p95: 10, coverage: 1},
		{name: "high CPU", pool: gkeTestNodePool("3", "RUNNING"), p95: 25.1, coverage: 1},
		{name: "low coverage", pool: gkeTestNodePool("3", "RUNNING"), p95: 10, coverage: 0.49},
		{name: "inactive pool", pool: gkeTestNodePool("3", "STOPPING"), p95: 10, coverage: 1},
		{name: "malformed node count", pool: gkeTestNodePool("many", "RUNNING"), p95: 10, coverage: 1},
		{
			name: "non GCP pool",
			pool: domain.Resource{
				ID:         "pool-aws",
				Kind:       domain.KindKubernetesNodePool,
				State:      "RUNNING",
				Attributes: map[string]string{"node_count": "3", "machine_type": "m5.large"},
			},
			p95:      10,
			coverage: 1,
		},
		{
			name: "wrong resource kind",
			pool: func() domain.Resource {
				pool := gkeTestNodePool("3", "RUNNING")
				pool.Kind = domain.KindComputeInstance
				return pool
			}(),
			p95:      10,
			coverage: 1,
		},
	}

	rule := gkeTestRule()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := gkeTestSnapshot(tt.pool, &domain.UtilizationSignal{
				ResourceID:    tt.pool.ID,
				MetricName:    "CPUUtilization",
				Kind:          domain.SignalP95,
				Value:         tt.p95,
				CoverageRatio: tt.coverage,
			})
			result := (GKENodePoolDownsizeCandidate{}).Evaluate(NewSnapshotView(snapshot, nil), rule)
			require.False(t, result.NotEvaluated)
			require.Len(t, result.Findings, tt.want)
			if tt.want == 1 {
				require.Contains(t, result.Findings[0].Description, "review target node count 1")
				require.Len(t, result.Findings[0].Evidence, 2)
				require.NotNil(t, result.Findings[0].Savings)
				require.True(t, result.Findings[0].Savings.InvestigationOnly)
			}
		})
	}
}

func TestGKENodePoolDownsizeCandidateMissingMetrics(t *testing.T) {
	pool := gkeTestNodePool("3", "RUNNING")
	result := (GKENodePoolDownsizeCandidate{}).Evaluate(
		NewSnapshotView(gkeTestSnapshot(pool, nil), nil),
		gkeTestRule(),
	)

	require.True(t, result.NotEvaluated)
	require.Contains(t, result.Reason, "no CPU p95")
	require.Empty(t, result.Findings)
}

func TestGKENodePoolDownsizeCandidateInvalidThreshold(t *testing.T) {
	rule := gkeTestRule()
	rule.Thresholds["min_node_count"] = "two"
	result := (GKENodePoolDownsizeCandidate{}).Evaluate(
		NewSnapshotView(gkeTestSnapshot(gkeTestNodePool("3", "RUNNING"), nil), nil),
		rule,
	)

	require.True(t, result.NotEvaluated)
	require.Contains(t, result.Reason, "min_node_count")
}

func TestGKENodePoolDownsizeCandidateSavings(t *testing.T) {
	pool := gkeTestNodePool("3", "RUNNING")
	signal := &domain.UtilizationSignal{
		ResourceID:    pool.ID,
		MetricName:    "CPUUtilization",
		Kind:          domain.SignalP95,
		Value:         10,
		CoverageRatio: 1,
	}
	snapshot := gkeTestSnapshot(pool, signal)

	t.Run("current pricing", func(t *testing.T) {
		catalog := gkeTestCatalog(t, "2026-08-01T00:00:00Z")
		result := (GKENodePoolDownsizeCandidate{Catalog: catalog}).Evaluate(NewSnapshotView(snapshot, catalog), gkeTestRule())

		require.Len(t, result.Findings, 1)
		draft := result.Findings[0].Savings
		require.NotNil(t, draft)
		require.False(t, draft.InvestigationOnly)
		require.EqualValues(t, 30*730, draft.Estimate.BaselineMinor)
		require.EqualValues(t, 20*730, draft.Estimate.CandidateMinor)
		require.Positive(t, draft.Estimate.GrossMonthlyMinor)
		require.Equal(t, "containers:pool-1:node-count", draft.Estimate.OverlapKey)
	})

	t.Run("stale pricing", func(t *testing.T) {
		catalog := gkeTestCatalog(t, "2025-01-01T00:00:00Z")
		result := (GKENodePoolDownsizeCandidate{Catalog: catalog}).Evaluate(NewSnapshotView(snapshot, catalog), gkeTestRule())

		require.Len(t, result.Findings, 1)
		draft := result.Findings[0].Savings
		require.NotNil(t, draft)
		require.True(t, draft.InvestigationOnly)
		require.Zero(t, draft.Estimate.GrossMonthlyMinor)
		require.Contains(t, result.Findings[0].Assumptions, "Pricing catalog is stale; savings are withheld.")
	})
}

func gkeTestRule() RuleSpec {
	return RuleSpec{
		Title: "GKE node pool downsize candidate",
		Thresholds: map[string]string{
			"max_p95_cpu_percent":         "25",
			"min_sample_coverage_percent": "50",
			"min_node_count":              "2",
		},
	}
}

func gkeTestNodePool(nodeCount, state string) domain.Resource {
	return domain.Resource{
		ID:                 "pool-1",
		Kind:               domain.KindKubernetesNodePool,
		ProviderResourceID: "projects/test/locations/us-central1/clusters/demo/nodePools/default-pool",
		RegionID:           "region-us-central1",
		Name:               "default-pool",
		State:              state,
		Attributes: map[string]string{
			"gcp_self_link": "projects/test/locations/us-central1/clusters/demo/nodePools/default-pool",
			"gcp_project":   "test",
			"machine_type":  "n2-standard-2",
			"node_count":    nodeCount,
		},
	}
}

func gkeTestSnapshot(pool domain.Resource, signal *domain.UtilizationSignal) *domain.CollectionSnapshot {
	observed := testTimestampNoHelper("2026-09-01T00:00:00Z")
	snapshot := &domain.CollectionSnapshot{
		StartedAt:   observed,
		CompletedAt: &observed,
		Resources:   []domain.Resource{pool},
		Regions: []domain.Region{{
			ID:               "region-us-central1",
			ProviderRegionID: "us-central1",
		}},
	}
	if signal != nil {
		snapshot.UtilizationSignals = []domain.UtilizationSignal{*signal}
	}
	return snapshot
}

func gkeTestCatalog(t *testing.T, effective string) *pricing.Catalog {
	t.Helper()
	return pricing.NewCatalog([]domain.PricingRecord{{
		Service:       "ComputeEngine",
		Region:        "us-central1",
		PurchaseModel: domain.PurchaseOnDemand,
		Currency:      "USD",
		EffectiveDate: testTimestamp(t, effective),
		Unit:          "hour",
		PriceMinor:    10,
		Attributes:    map[string]string{"machine_type": "n2-standard-2"},
	}}, "test")
}

func testTimestampNoHelper(raw string) types.Timestamp {
	ts, err := types.ParseTimestamp(raw)
	if err != nil {
		panic(err)
	}
	return ts
}
