package rules

import (
	"fmt"
	"strings"

	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/application/savings"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func isGCPResource(r domain.Resource) bool {
	if r.Attributes == nil {
		return false
	}
	return r.Attributes["gcp_self_link"] != "" || r.Attributes["gcp_project"] != ""
}

// GCEDownsizeCandidate recommends GCE machine downsizing from utilization and catalog pricing.
type GCEDownsizeCandidate struct {
	Catalog *pricing.Catalog
}

func (GCEDownsizeCandidate) Name() string { return "gce_downsize_candidate" }

func (e GCEDownsizeCandidate) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	maxP95, err := rule.thresholdInt("max_p95_cpu_percent", 25)
	if err != nil {
		return EvaluatorResult{NotEvaluated: true, Reason: err.Error()}
	}
	minCoverage, err := rule.thresholdInt("min_sample_coverage_percent", 50)
	if err != nil {
		return EvaluatorResult{NotEvaluated: true, Reason: err.Error()}
	}
	headroom, err := rule.thresholdInt("safety_headroom_bps", 1500)
	if err != nil {
		return EvaluatorResult{NotEvaluated: true, Reason: err.Error()}
	}
	cat := e.Catalog
	if cat == nil {
		cat = view.Catalog()
	}
	if cat == nil || cat.IsEmpty() {
		return EvaluatorResult{NotEvaluated: true, Reason: "pricing catalog not loaded"}
	}

	var findings []CandidateFinding
	for _, res := range view.ResourcesOfKind(domain.KindComputeInstance) {
		if !isGCPResource(res) || stoppedState(res.State) {
			continue
		}
		curType := res.Attributes["machine_type"]
		if curType == "" || strings.HasPrefix(curType, "custom-") {
			continue
		}
		p95, coverage, notes, ok := view.UtilizationMetric(res.ID, "CPUUtilization", domain.SignalP95)
		if !ok || coverage*100 < float64(minCoverage) || p95 > float64(maxP95) {
			continue
		}
		region := view.regionProviderID(res.RegionID)
		sel := cat.SortedGCEAlternatives(region, curType, pricing.GCECandidateConfig{HeadroomBasisPoints: headroom})
		if sel.Accepted == nil {
			continue
		}
		investigationOnly := catalogStale(cat, view.ObservedAt())
		assumptions := []string{
			"On-demand shared-tenancy GCE pricing; excludes persistent disk, networking, and committed use discounts.",
			fmt.Sprintf("Safety headroom %d bps applied to gross hourly delta.", headroom),
		}
		for _, n := range notes {
			if strings.Contains(strings.ToLower(n), "memory") {
				investigationOnly = true
				assumptions = append(assumptions, n)
			}
		}
		est := savings.MonthlyRightsizingFromHourly(sel.Accepted.BaselineHourlyMinor, sel.Accepted.TargetHourlyMinor, sel.Currency, headroom, pricing.RecomputeInputs("gce", map[string]string{
			"machine_type_from": curType,
			"machine_type_to":   sel.Accepted.TargetType,
			"region":            region,
		}))
		findings = append(findings, CandidateFinding{
			Title:       rule.Title,
			Description: fmt.Sprintf("GCE instance %q (%s) p95 CPU %.1f%% supports %s → %s.", res.Name, curType, p95, curType, sel.Accepted.TargetType),
			ResourceIDs: []types.ResourceID{res.ID},
			Evidence: []EvidenceDraft{{
				Kind: domain.EvidenceMetric, ResourceID: res.ID,
				Summary: fmt.Sprintf("CPU p95=%.1f%%, coverage=%.0f%%", p95, coverage*100),
			}},
			Assumptions: assumptions,
			Confidence:  types.PercentageFromFloat(0.75),
			Savings: &SavingsDraft{
				Estimate:          est,
				InvestigationOnly: investigationOnly,
			},
		})
	}
	return EvaluatorResult{Findings: findings}
}

// GCPIdleExternalIP flags reserved external IPs without forwarding use.
type GCPIdleExternalIP struct{}

func (GCPIdleExternalIP) Name() string { return "gcp_idle_external_ip" }

func (GCPIdleExternalIP) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	var findings []CandidateFinding
	for _, res := range view.ResourcesOfKind(domain.KindElasticIP) {
		if !isGCPResource(res) {
			continue
		}
		if !gcpIdleAddressState(res.State) {
			continue
		}
		findings = append(findings, CandidateFinding{
			Title:       rule.Title,
			Description: fmt.Sprintf("External IP %q is %s and may incur idle address charges.", res.Name, res.State),
			ResourceIDs: []types.ResourceID{res.ID},
			Evidence: []EvidenceDraft{{
				Kind: domain.EvidenceResource, ResourceID: res.ID,
				Summary: fmt.Sprintf("address state=%s", res.State),
			}},
			Assumptions: []string{"Idle IP charges apply to RESERVED addresses not attached to in-use resources."},
			Confidence:  types.PercentageFromFloat(0.85),
		})
	}
	return EvaluatorResult{Findings: findings}
}

// GCPDiskTypeOptimize recommends pd-standard → pd-balanced when attached.
type GCPDiskTypeOptimize struct {
	Catalog *pricing.Catalog
}

func (GCPDiskTypeOptimize) Name() string { return "gcp_disk_type_optimize" }

func (e GCPDiskTypeOptimize) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	cat := e.Catalog
	if cat == nil {
		cat = view.Catalog()
	}
	if cat == nil || cat.IsEmpty() {
		return EvaluatorResult{NotEvaluated: true, Reason: "pricing catalog not loaded"}
	}
	var findings []CandidateFinding
	for _, vol := range view.ResourcesOfKind(domain.KindBlockVolume) {
		if !isGCPResource(vol) {
			continue
		}
		if vol.Attributes["disk_type"] != "pd-standard" {
			continue
		}
		if vol.Attributes["attached"] == "false" && !view.VolumeAttached(vol.ID) {
			continue
		}
		region := view.regionProviderID(vol.RegionID)
		curMinor, cur, ok := cat.GCPDiskPerGBMonthMinor(region, "pd-standard")
		if !ok {
			continue
		}
		targetMinor, _, ok := cat.GCPDiskPerGBMonthMinor(region, "pd-balanced")
		if !ok || targetMinor >= curMinor {
			continue
		}
		sizeGB := parseIntAttr(vol.Attributes, "size_gb", 0)
		if sizeGB <= 0 {
			continue
		}
		delta := (curMinor - targetMinor) * sizeGB
		findings = append(findings, CandidateFinding{
			Title:       rule.Title,
			Description: fmt.Sprintf("Disk %q (%d GB pd-standard) may move to pd-balanced.", vol.Name, sizeGB),
			ResourceIDs: []types.ResourceID{vol.ID},
			Assumptions: []string{"Assumes pd-balanced meets IOPS/latency needs; verify workload IO profile."},
			Confidence:  types.PercentageFromFloat(0.7),
			Savings: &SavingsDraft{
				Estimate: domain.SavingsEstimate{
					Class:             domain.SavingsMonthlyRecurring,
					GrossMonthlyMinor: delta,
					Currency:          cur,
					OverlapKey:        "gcp-disk-" + string(vol.ID),
					Inputs: pricing.RecomputeInputs("gcp_disk", map[string]string{
						"disk_type_from": "pd-standard",
						"disk_type_to":   "pd-balanced",
						"size_gb":        fmt.Sprintf("%d", sizeGB),
						"region":         region,
					}),
				},
			},
		})
	}
	return EvaluatorResult{Findings: findings}
}

// CloudSQLDownsizeCandidate recommends Cloud SQL tier reduction from CPU utilization.
type CloudSQLDownsizeCandidate struct {
	Catalog *pricing.Catalog
}

func (CloudSQLDownsizeCandidate) Name() string { return "cloudsql_downsize_candidate" }

func (e CloudSQLDownsizeCandidate) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	maxP95, _ := rule.thresholdInt("max_p95_cpu_percent", 30)
	headroom, _ := rule.thresholdInt("safety_headroom_bps", 1500)
	cat := e.Catalog
	if cat == nil {
		cat = view.Catalog()
	}
	if cat == nil || cat.IsEmpty() {
		return EvaluatorResult{NotEvaluated: true, Reason: "pricing catalog not loaded"}
	}
	var findings []CandidateFinding
	for _, db := range view.ResourcesOfKind(domain.KindDatabase) {
		if !isGCPResource(db) {
			continue
		}
		tier := db.Attributes["tier"]
		if tier == "" {
			continue
		}
		p95, coverage, _, ok := view.UtilizationMetric(db.ID, "CPUUtilization", domain.SignalP95)
		if !ok || coverage*100 < 50 || p95 > float64(maxP95) {
			continue
		}
		region := view.regionProviderID(db.RegionID)
		curMinor, cur, ok := cat.CloudSQLHourlyMinor(region, tier, "POSTGRES")
		if !ok {
			continue
		}
		targetTier := "db-custom-1-3840"
		if tier == targetTier {
			continue
		}
		targetMinor, _, ok := cat.CloudSQLHourlyMinor(region, targetTier, "POSTGRES")
		if !ok || targetMinor >= curMinor {
			continue
		}
		est := savings.MonthlyRightsizingFromHourly(curMinor, targetMinor, cur, headroom, pricing.RecomputeInputs("cloudsql", map[string]string{
			"tier_from": tier,
			"tier_to":   targetTier,
			"region":    region,
		}))
		findings = append(findings, CandidateFinding{
			Title:       rule.Title,
			Description: fmt.Sprintf("Cloud SQL %q tier %s p95 CPU %.1f%% may downsize to %s.", db.Name, tier, p95, targetTier),
			ResourceIDs: []types.ResourceID{db.ID},
			Confidence:  types.PercentageFromFloat(0.7),
			Assumptions: []string{"PostgreSQL zonal tier; validate connections and memory before change."},
			Savings:     &SavingsDraft{Estimate: est, InvestigationOnly: catalogStale(cat, view.ObservedAt())},
		})
	}
	return EvaluatorResult{Findings: findings}
}

// GCPNATLowUtilization flags low Cloud NAT egress relative to hourly gateway cost.
type GCPNATLowUtilization struct {
	Catalog *pricing.Catalog
}

func (GCPNATLowUtilization) Name() string { return "gcp_nat_low_utilization" }

func (e GCPNATLowUtilization) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	maxBytes, _ := rule.thresholdInt("max_bytes_out_per_hour", 104857600)
	cat := e.Catalog
	if cat == nil {
		cat = view.Catalog()
	}
	var findings []CandidateFinding
	for _, nat := range view.ResourcesOfKind(domain.KindNATGateway) {
		if !isGCPResource(nat) {
			continue
		}
		mean, _, _, ok := view.UtilizationMetric(nat.ID, "BytesOutToDestination", domain.SignalMean)
		if !ok || mean > float64(maxBytes) {
			continue
		}
		region := view.regionProviderID(nat.RegionID)
		hourly, cur, ok := cat.GCPNATHourlyMinor(region)
		if !ok {
			continue
		}
		findings = append(findings, CandidateFinding{
			Title:       rule.Title,
			Description: fmt.Sprintf("Cloud NAT %q mean egress %.0f B/h below threshold.", nat.Name, mean),
			ResourceIDs: []types.ResourceID{nat.ID},
			Assumptions: []string{fmt.Sprintf("Gateway hourly cost ~%s %s; review VPC endpoints before removing NAT.", types.Money{AmountMinor: hourly, Currency: cur}.String(), "")},
			Confidence:  types.PercentageFromFloat(0.65),
			Savings: &SavingsDraft{
				Estimate: domain.SavingsEstimate{
					Class:             domain.SavingsMonthlyRecurring,
					GrossMonthlyMinor: pricing.MonthlyFromHourly(hourly),
					Currency:          cur,
					OverlapKey:        "gcp-nat-" + string(nat.ID),
				},
				InvestigationOnly: true,
			},
		})
	}
	return EvaluatorResult{Findings: findings}
}

// CommittedUseCoverage analyzes CUD-eligible spend (investigation-only, lock-in aware).
type CommittedUseCoverage struct{}

func (CommittedUseCoverage) Name() string { return "gcp_committed_use_coverage" }

func (CommittedUseCoverage) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	minMinor, _ := rule.thresholdInt("min_eligible_spend_minor", 5000)
	var eligible int64
	var currency string
	for _, c := range view.Snapshot.Costs {
		if c.ResourceID == domain.UnattributedResourceID {
			continue
		}
		if !strings.Contains(strings.ToLower(c.Service), "compute") {
			continue
		}
		res, ok := view.Resource(c.ResourceID)
		if !ok || !isGCPResource(res) {
			continue
		}
		if c.Basis == domain.CostBasisUnblended && c.ChargeKind == domain.ChargeUsage {
			eligible += c.Amount.AmountMinor
			currency = c.Amount.Currency
		}
	}
	if eligible < minMinor {
		return EvaluatorResult{Findings: nil}
	}
	return EvaluatorResult{Findings: []CandidateFinding{{
		Title:       rule.Title,
		Description: fmt.Sprintf("Approximately %s of on-demand Compute Engine spend may be CUD-eligible (investigation only).", types.Money{AmountMinor: eligible, Currency: currency}.String()),
		Assumptions: []string{"Committed use discounts require term commitment and reduce flexibility; verify baseline utilization stability."},
		Confidence:  types.PercentageFromFloat(0.5),
		Savings: &SavingsDraft{
			InvestigationOnly: true,
			Estimate: domain.SavingsEstimate{
				Class:    domain.SavingsCommitment,
				Currency: currency,
				Inputs: map[string]string{
					"billing_basis":  "unblended_compute_usage",
					"eligible_minor": fmt.Sprintf("%d", eligible),
				},
			},
		},
	}}}
}
