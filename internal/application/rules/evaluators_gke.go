package rules

import (
	"fmt"
	"strings"

	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/application/savings"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

const gkeNodePoolSavingsHeadroomBPS int64 = 1500

// GKENodePoolDownsizeCandidate recommends removing one node from a low-CPU GKE node pool.
type GKENodePoolDownsizeCandidate struct {
	Catalog *pricing.Catalog
}

func (GKENodePoolDownsizeCandidate) Name() string {
	return "gke_node_pool_downsize_candidate"
}

func (e GKENodePoolDownsizeCandidate) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	maxP95, err := rule.thresholdInt("max_p95_cpu_percent", 25)
	if err != nil {
		return EvaluatorResult{NotEvaluated: true, Reason: err.Error()}
	}
	minCoverage, err := rule.thresholdInt("min_sample_coverage_percent", 50)
	if err != nil {
		return EvaluatorResult{NotEvaluated: true, Reason: err.Error()}
	}
	minNodeCount, err := rule.thresholdInt("min_node_count", 2)
	if err != nil {
		return EvaluatorResult{NotEvaluated: true, Reason: err.Error()}
	}

	var findings []CandidateFinding
	var eligiblePools, poolsWithMetrics int
	for _, pool := range view.ResourcesOfKind(domain.KindKubernetesNodePool) {
		if !isGCPResource(pool) || !gkeNodePoolActive(pool.State) {
			continue
		}
		nodeCount := parseIntAttr(pool.Attributes, "node_count", 0)
		if nodeCount < minNodeCount {
			continue
		}
		eligiblePools++
		p95, coverage, notes, ok := view.UtilizationMetric(pool.ID, "CPUUtilization", domain.SignalP95)
		if !ok {
			continue
		}
		poolsWithMetrics++
		if p95 > float64(maxP95) || coverage*100 < float64(minCoverage) {
			continue
		}

		targetNodeCount := nodeCount - 1
		machineType := pool.Attributes["machine_type"]
		assumptions := []string{
			"Recommendation removes only one node per review cycle.",
			"Validate memory utilization, pod requests, disruption budgets, scheduling constraints, and autoscaler limits before resizing.",
			"Node-pool CPU is used as a capacity proxy; Kubernetes workload demand is not analyzed.",
		}
		assumptions = append(assumptions, notes...)
		savingsDraft := gkeNodePoolSavings(firstCatalog(e.Catalog, view), view, pool, machineType, nodeCount, targetNodeCount, &assumptions)

		findings = append(findings, CandidateFinding{
			Title: rule.Title,
			Description: fmt.Sprintf(
				"GKE node pool %q has %d nodes with p95 CPU %.1f%%; review target node count %d.",
				pool.Name, nodeCount, p95, targetNodeCount,
			),
			ResourceIDs: []types.ResourceID{pool.ID},
			Evidence: []EvidenceDraft{
				{
					Kind:       domain.EvidenceMetric,
					ResourceID: pool.ID,
					Summary:    fmt.Sprintf("CPU p95=%.1f%%, sample coverage=%.0f%%", p95, coverage*100),
					Detail: map[string]string{
						"p95_cpu_percent": fmt.Sprintf("%.2f", p95),
						"sample_coverage": fmt.Sprintf("%.4f", coverage),
					},
				},
				{
					Kind:       domain.EvidenceResource,
					ResourceID: pool.ID,
					Summary:    fmt.Sprintf("node_count=%d, candidate_node_count=%d", nodeCount, targetNodeCount),
					Detail: map[string]string{
						"machine_type":         machineType,
						"node_count":           fmt.Sprintf("%d", nodeCount),
						"candidate_node_count": fmt.Sprintf("%d", targetNodeCount),
					},
				},
			},
			Assumptions: assumptions,
			Confidence:  types.PercentageFromFloat(0.65),
			Savings:     savingsDraft,
		})
	}
	if eligiblePools > 0 && poolsWithMetrics == 0 {
		return EvaluatorResult{NotEvaluated: true, Reason: "no CPU p95 utilization signals for eligible GKE node pools"}
	}
	return EvaluatorResult{Findings: findings}
}

func gkeNodePoolActive(state string) bool {
	return strings.EqualFold(strings.TrimSpace(state), "RUNNING")
}

func gkeNodePoolSavings(
	cat *pricing.Catalog,
	view *SnapshotView,
	pool domain.Resource,
	machineType string,
	nodeCount, targetNodeCount int64,
	assumptions *[]string,
) *SavingsDraft {
	overlapKey := fmt.Sprintf("containers:%s:node-count", pool.ID)
	inputs := pricing.RecomputeInputs("gke_node_pool_downsize", map[string]string{
		"region":               view.regionProviderID(pool.RegionID),
		"machine_type":         machineType,
		"node_count":           fmt.Sprintf("%d", nodeCount),
		"candidate_node_count": fmt.Sprintf("%d", targetNodeCount),
		"headroom_bps":         fmt.Sprintf("%d", gkeNodePoolSavingsHeadroomBPS),
	})
	investigation := domain.SavingsEstimate{
		Class:      domain.SavingsMonthlyRecurring,
		OverlapKey: overlapKey,
		Inputs:     inputs,
	}
	if cat == nil || cat.IsEmpty() || machineType == "" {
		*assumptions = append(*assumptions, "Matching Compute Engine pricing is unavailable; savings are withheld.")
		return &SavingsDraft{Estimate: investigation, InvestigationOnly: true}
	}

	region := view.regionProviderID(pool.RegionID)
	hourly, currency, found := cat.GCEHourlyMinor(region, machineType)
	if !found || hourly <= 0 {
		*assumptions = append(*assumptions, "Matching Compute Engine pricing is unavailable; savings are withheld.")
		return &SavingsDraft{Estimate: investigation, InvestigationOnly: true}
	}
	est := savings.MonthlyRightsizingFromHourly(
		hourly*nodeCount,
		hourly*targetNodeCount,
		currency,
		gkeNodePoolSavingsHeadroomBPS,
		inputs,
	)
	est.OverlapKey = overlapKey
	est.Assumptions = append(est.Assumptions, "Estimate covers node VM compute only; it excludes GKE management, storage, networking, and discounts.")
	if catalogStale(cat, view.ObservedAt()) {
		est.GrossMonthlyMinor = 0
		est.LowMonthlyMinor = 0
		est.HighMonthlyMinor = 0
		*assumptions = append(*assumptions, "Pricing catalog is stale; savings are withheld.")
		return &SavingsDraft{Estimate: est, InvestigationOnly: true}
	}
	return &SavingsDraft{Estimate: est}
}
