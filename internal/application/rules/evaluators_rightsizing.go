package rules

import (
	"fmt"
	"strings"

	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/application/savings"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// EC2DownsizeCandidate recommends same-family instance downsizing from utilization + catalog pricing.
type EC2DownsizeCandidate struct {
	Catalog *pricing.Catalog
}

func (EC2DownsizeCandidate) Name() string { return "ec2_downsize_candidate" }

func (e EC2DownsizeCandidate) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	maxP95, err := rule.thresholdInt("max_p95_cpu_percent", 25)
	if err != nil {
		return EvaluatorResult{NotEvaluated: true, Reason: err.Error()}
	}
	minCoverage, err := rule.thresholdInt("min_sample_coverage_percent", 70)
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
		if !isAWSResource(res) {
			continue
		}
		if stoppedState(res.State) {
			continue
		}
		curType := res.Attributes["instance_type"]
		if curType == "" {
			continue
		}
		p95, coverage, notes, ok := view.UtilizationMetric(res.ID, "CPUUtilization", domain.SignalP95)
		if !ok {
			continue
		}
		if coverage*100 < float64(minCoverage) {
			continue
		}
		if p95 > float64(maxP95) {
			continue
		}
		region := view.regionProviderID(res.RegionID)
		sel := cat.SortedEC2Alternatives(region, curType, pricing.EC2CandidateConfig{HeadroomBasisPoints: headroom})
		evidence := []EvidenceDraft{
			{
				Kind:       domain.EvidenceMetric,
				ResourceID: res.ID,
				Summary:    fmt.Sprintf("CPU p95=%.1f%%, sample coverage=%.0f%%", p95, coverage*100),
				Detail: map[string]string{
					"p95_cpu_percent": fmt.Sprintf("%.2f", p95),
					"sample_coverage": fmt.Sprintf("%.4f", coverage),
				},
			},
		}
		for _, rej := range sel.Rejected {
			evidence = append(evidence, EvidenceDraft{
				Kind:       domain.EvidenceDerived,
				ResourceID: res.ID,
				Summary:    fmt.Sprintf("rejected candidate %s: %s", rej.TargetType, rej.Reason),
				Detail: map[string]string{
					"candidate_type": rej.TargetType,
					"reject_reason":  rej.Reason,
				},
			})
		}
		investigationOnly := false
		assumptions := []string{
			"On-demand Linux shared tenancy pricing; excludes EBS, data transfer, and purchase discounts.",
			fmt.Sprintf("Safety headroom %d bps applied to gross hourly delta.", headroom),
		}
		for _, n := range notes {
			if strings.Contains(strings.ToLower(n), "memory") {
				investigationOnly = true
				assumptions = append(assumptions, n)
			}
		}
		if catalogStale(cat, view.ObservedAt()) {
			investigationOnly = true
			assumptions = append(assumptions, "Pricing catalog effective date is stale relative to observation window.")
		}
		if sel.Accepted == nil {
			continue
		}
		inputs := pricing.RecomputeInputs("ec2_downsize", map[string]string{
			"region":                region,
			"current_instance_type": curType,
			"target_instance_type":  sel.Accepted.TargetType,
			"baseline_hourly_minor": fmt.Sprintf("%d", sel.Accepted.BaselineHourlyMinor),
			"target_hourly_minor":   fmt.Sprintf("%d", sel.Accepted.TargetHourlyMinor),
			"headroom_bps":          fmt.Sprintf("%d", headroom),
			"pricing_source":        cat.Source,
		})
		est := savings.MonthlyRightsizingFromHourly(
			sel.Accepted.BaselineHourlyMinor,
			sel.Accepted.TargetHourlyMinor,
			sel.Accepted.Currency,
			headroom,
			inputs,
		)
		est.OverlapKey = fmt.Sprintf("compute:%s:rightsizing", res.ID)
		conf := types.PercentageFromFloat(0.75)
		if investigationOnly {
			conf = types.PercentageFromFloat(0.45)
			est.GrossMonthlyMinor = 0
		}
		findings = append(findings, CandidateFinding{
			Title:       rule.Title,
			Description: fmt.Sprintf("Instance %q (%s) shows sustained low CPU (p95 %.1f%%) vs type %s; candidate %s.", res.Name, curType, p95, curType, sel.Accepted.TargetType),
			ResourceIDs: []types.ResourceID{res.ID},
			Evidence:    evidence,
			Assumptions: assumptions,
			Confidence:  conf,
			Savings:     &SavingsDraft{Estimate: est, InvestigationOnly: investigationOnly},
		})
	}
	return EvaluatorResult{Findings: findings}
}

// EC2IdleInstance flags persistently idle running instances.
type EC2IdleInstance struct {
	Catalog *pricing.Catalog
}

func (EC2IdleInstance) Name() string { return "ec2_idle_instance" }

func (e EC2IdleInstance) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	minIdle, err := rule.thresholdInt("min_idle_periods", 3)
	if err != nil {
		return EvaluatorResult{NotEvaluated: true, Reason: err.Error()}
	}
	var findings []CandidateFinding
	for _, res := range view.ResourcesOfKind(domain.KindComputeInstance) {
		if !isAWSResource(res) {
			continue
		}
		if stoppedState(res.State) {
			continue
		}
		idle, _, _, ok := view.UtilizationMetric(res.ID, "CPUUtilization", domain.SignalIdlePeriods)
		if !ok || idle < float64(minIdle) {
			continue
		}
		assumptions := []string{"Idle periods derived from CPU utilization threshold; verify workload schedulers before stopping."}
		var savingsDraft *SavingsDraft
		if cat := firstCatalog(e.Catalog, view); cat != nil {
			region := view.regionProviderID(res.RegionID)
			curType := res.Attributes["instance_type"]
			if hourly, cur, ok := cat.EC2HourlyMinor(region, curType); ok && hourly > 0 {
				inputs := pricing.RecomputeInputs("ec2_idle", map[string]string{
					"region":                region,
					"instance_type":         curType,
					"baseline_hourly_minor": fmt.Sprintf("%d", hourly),
					"target_hourly_minor":   "0",
				})
				est := savings.MonthlyRightsizingFromHourly(hourly, 0, cur, 2000, inputs)
				est.OverlapKey = fmt.Sprintf("compute:%s:lifecycle", res.ID)
				est.Assumptions = append(est.Assumptions, "Assumes instance can be stopped or terminated; storage and IP charges may continue.")
				savingsDraft = &SavingsDraft{Estimate: est}
			}
		}
		findings = append(findings, CandidateFinding{
			Title:       rule.Title,
			Description: fmt.Sprintf("Instance %q shows %d idle CPU periods in the observation window.", res.Name, int(idle)),
			ResourceIDs: []types.ResourceID{res.ID},
			Evidence: []EvidenceDraft{{
				Kind:       domain.EvidenceMetric,
				ResourceID: res.ID,
				Summary:    fmt.Sprintf("idle_periods=%d", int(idle)),
				Detail:     map[string]string{"idle_periods": fmt.Sprintf("%d", int(idle))},
			}},
			Assumptions: assumptions,
			Confidence:  types.PercentageFromFloat(0.7),
			Savings:     savingsDraft,
		})
	}
	return EvaluatorResult{Findings: findings}
}

// EBSVolumeTypeOptimize recommends gp2 → gp3 when catalog shows savings.
type EBSVolumeTypeOptimize struct {
	Catalog *pricing.Catalog
}

func (EBSVolumeTypeOptimize) Name() string { return "ebs_volume_type_optimize" }

func (e EBSVolumeTypeOptimize) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	cat := firstCatalog(e.Catalog, view)
	if cat == nil || cat.IsEmpty() {
		return EvaluatorResult{NotEvaluated: true, Reason: "pricing catalog not loaded"}
	}
	var findings []CandidateFinding
	for _, vol := range view.ResourcesOfKind(domain.KindBlockVolume) {
		if !isAWSResource(vol) {
			continue
		}
		vtype := strings.ToLower(vol.Attributes["volume_type"])
		if vtype == "" {
			vtype = "gp2"
		}
		if vtype != "gp2" {
			continue
		}
		size := parseIntAttr(vol.Attributes, "size_gib", 0)
		if size <= 0 {
			continue
		}
		region := view.regionProviderID(vol.RegionID)
		fromMinor, cur, okFrom := cat.EBSPerGBMonthMinor(region, "gp2")
		toMinor, _, okTo := cat.EBSPerGBMonthMinor(region, "gp3")
		if !okFrom || !okTo || fromMinor <= toMinor {
			continue
		}
		inputs := pricing.RecomputeInputs("ebs_gp3", map[string]string{
			"region":              region,
			"size_gib":            fmt.Sprintf("%d", size),
			"from_type":           "gp2",
			"to_type":             "gp3",
			"from_gb_month_minor": fmt.Sprintf("%d", fromMinor),
			"to_gb_month_minor":   fmt.Sprintf("%d", toMinor),
		})
		est := savings.MonthlyEBSMigration(size, fromMinor, toMinor, cur, inputs)
		est.OverlapKey = fmt.Sprintf("storage:%s:type", vol.ID)
		findings = append(findings, CandidateFinding{
			Title:       rule.Title,
			Description: fmt.Sprintf("Volume %q (%d GiB gp2) can move to gp3 for lower per-GB-month cost.", vol.Name, size),
			ResourceIDs: []types.ResourceID{vol.ID},
			Evidence: []EvidenceDraft{{
				Kind:       domain.EvidenceDerived,
				ResourceID: vol.ID,
				Summary:    "catalog gp2 vs gp3 pricing",
				Detail: map[string]string{
					"volume_type": vtype,
					"size_gib":    fmt.Sprintf("%d", size),
				},
			}},
			Assumptions: []string{"Assumes gp3 baseline IOPS/throughput sufficient; performance tuning may add cost."},
			Confidence:  types.PercentageFromFloat(0.8),
			Savings:     &SavingsDraft{Estimate: est},
		})
	}
	return EvaluatorResult{Findings: findings}
}

// RDSDownsizeCandidate recommends smaller RDS class from low CPU utilization.
type RDSDownsizeCandidate struct {
	Catalog *pricing.Catalog
}

func (RDSDownsizeCandidate) Name() string { return "rds_downsize_candidate" }

func (e RDSDownsizeCandidate) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	maxP95, err := rule.thresholdInt("max_p95_cpu_percent", 30)
	if err != nil {
		return EvaluatorResult{NotEvaluated: true, Reason: err.Error()}
	}
	cat := firstCatalog(e.Catalog, view)
	if cat == nil || cat.IsEmpty() {
		return EvaluatorResult{NotEvaluated: true, Reason: "pricing catalog not loaded"}
	}
	headroom, _ := rule.thresholdInt("safety_headroom_bps", 1500)
	var findings []CandidateFinding
	for _, db := range view.ResourcesOfKind(domain.KindDatabase) {
		if !isAWSResource(db) {
			continue
		}
		p95, coverage, _, ok := view.UtilizationMetric(db.ID, "CPUUtilization", domain.SignalP95)
		if !ok || p95 > float64(maxP95) {
			continue
		}
		if coverage < 0.5 {
			findings = append(findings, CandidateFinding{
				Title:       rule.Title,
				Description: fmt.Sprintf("Database %q has sparse CPU metrics; rightsizing withheld pending better coverage.", db.Name),
				ResourceIDs: []types.ResourceID{db.ID},
				Assumptions: []string{"Investigation-only due to low sample coverage."},
				Confidence:  types.PercentageFromFloat(0.35),
				Savings:     &SavingsDraft{InvestigationOnly: true, Estimate: domain.SavingsEstimate{Class: domain.SavingsMonthlyRecurring, OverlapKey: fmt.Sprintf("database:%s:rightsizing", db.ID)}},
			})
			continue
		}
		curClass := db.Attributes["instance_class"]
		engine := db.Attributes["engine"]
		if curClass == "" {
			continue
		}
		target := smallerRDSClass(curClass)
		if target == "" || target == curClass {
			continue
		}
		region := view.regionProviderID(db.RegionID)
		curMinor, cur, okCur := cat.RDSHourlyMinor(region, curClass, engine)
		tgtMinor, _, okTgt := cat.RDSHourlyMinor(region, target, engine)
		if !okCur || !okTgt || tgtMinor >= curMinor {
			continue
		}
		inputs := pricing.RecomputeInputs("rds_downsize", map[string]string{
			"region":                region,
			"current_class":         curClass,
			"target_class":          target,
			"engine":                engine,
			"baseline_hourly_minor": fmt.Sprintf("%d", curMinor),
			"target_hourly_minor":   fmt.Sprintf("%d", tgtMinor),
		})
		est := savings.MonthlyRightsizingFromHourly(curMinor, tgtMinor, cur, headroom, inputs)
		est.OverlapKey = fmt.Sprintf("database:%s:rightsizing", db.ID)
		findings = append(findings, CandidateFinding{
			Title:       rule.Title,
			Description: fmt.Sprintf("RDS %q class %s shows low CPU p95 (%.1f%%); candidate %s.", db.Name, curClass, p95, target),
			ResourceIDs: []types.ResourceID{db.ID},
			Assumptions: []string{"Memory and connection limits must be validated before downsizing."},
			Confidence:  types.PercentageFromFloat(0.72),
			Savings:     &SavingsDraft{Estimate: est},
		})
	}
	return EvaluatorResult{Findings: findings}
}

func smallerRDSClass(cur string) string {
	switch cur {
	case "db.t3.medium":
		return "db.t3.small"
	case "db.t3.large":
		return "db.t3.medium"
	default:
		return ""
	}
}

// NATGatewayLowUtilization highlights NAT gateways with low traffic relative to fixed hourly cost.
type NATGatewayLowUtilization struct {
	Catalog *pricing.Catalog
}

func (NATGatewayLowUtilization) Name() string { return "nat_gateway_low_utilization" }

func (e NATGatewayLowUtilization) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	maxBytes, err := rule.thresholdInt("max_bytes_out_per_hour", 104857600) // 100 MiB/h
	if err != nil {
		return EvaluatorResult{NotEvaluated: true, Reason: err.Error()}
	}
	cat := firstCatalog(e.Catalog, view)
	if cat == nil || cat.IsEmpty() {
		return EvaluatorResult{NotEvaluated: true, Reason: "pricing catalog not loaded"}
	}
	var findings []CandidateFinding
	for _, nat := range view.ResourcesOfKind(domain.KindNATGateway) {
		if !isAWSResource(nat) {
			continue
		}
		mean, _, _, ok := view.UtilizationMetric(nat.ID, "BytesOutToDestination", domain.SignalMean)
		if !ok {
			continue
		}
		if mean > float64(maxBytes) {
			continue
		}
		region := view.regionProviderID(nat.RegionID)
		hourly, cur, ok := cat.NATHourlyMinor(region)
		if !ok {
			continue
		}
		inputs := pricing.RecomputeInputs("nat_low_util", map[string]string{
			"region":                region,
			"baseline_hourly_minor": fmt.Sprintf("%d", hourly),
			"mean_bytes_out":        fmt.Sprintf("%.0f", mean),
		})
		est := domain.SavingsEstimate{
			Class:             domain.SavingsMonthlyRecurring,
			Currency:          cur,
			BaselineMinor:     pricing.MonthlyFromHourly(hourly),
			GrossMonthlyMinor: 0,
			OverlapKey:        fmt.Sprintf("network:%s:nat", nat.ID),
			Inputs:            inputs,
		}
		findings = append(findings, CandidateFinding{
			Title:       rule.Title,
			Description: fmt.Sprintf("NAT gateway %q shows low egress (mean %.0f B/h); review VPC endpoints or route consolidation.", nat.Name, mean),
			ResourceIDs: []types.ResourceID{nat.ID},
			Assumptions: []string{"Investigation-only: requires architecture review before removing NAT capacity."},
			Confidence:  types.PercentageFromFloat(0.5),
			Savings:     &SavingsDraft{InvestigationOnly: true, Estimate: est},
		})
	}
	return EvaluatorResult{Findings: findings}
}

func firstCatalog(primary *pricing.Catalog, view *SnapshotView) *pricing.Catalog {
	if primary != nil && !primary.IsEmpty() {
		return primary
	}
	return view.Catalog()
}
