package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kernul-io/cloudopt/internal/application/rules"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// BuildInput aggregates persisted analysis output and rule execution details.
type BuildInput struct {
	AnalyzerVersion string
	GeneratedAt     time.Time
	Metadata        Metadata
	Snapshot        *domain.CollectionSnapshot
	Run             *domain.AnalysisRun
	Executions      []rules.RuleExecution
	RuleTitles      map[string]string
	Redact          bool
}

// Build constructs a consultant report document from analysis artifacts.
func Build(in BuildInput) (*Document, error) {
	if in.Run == nil || in.Snapshot == nil {
		return nil, fmt.Errorf("snapshot and analysis run are required")
	}
	if in.GeneratedAt.IsZero() {
		in.GeneratedAt = time.Now().UTC()
	}
	aliases := NewAliasMap(in.Snapshot, in.Redact)

	evidenceByID := map[int64]domain.Evidence{}
	for _, e := range in.Run.Evidence {
		evidenceByID[e.ID] = e
	}
	recByFinding := map[types.FindingID]domain.Recommendation{}
	for _, r := range in.Run.Recommendations {
		recByFinding[r.FindingID] = r
	}

	scope, warnings := buildScope(in.Snapshot, aliases)
	costs := buildCosts(in.Snapshot)
	savings := buildSavings(in.Snapshot, in.Run.Findings, recByFinding)

	findings := buildFindings(in.Run.Findings, evidenceByID, recByFinding, in.Snapshot, aliases)
	appendix := buildAppendix(in.Executions, in.Snapshot, aliases)

	summary := rules.Summary{}
	for _, ex := range in.Executions {
		switch ex.Status {
		case rules.RulePassed:
			summary.Passed++
		case rules.RuleFailed:
			summary.Failed++
		case rules.RuleSuppressed:
			summary.Suppressed++
		case rules.RuleNotEvaluated:
			summary.NotEvaluated++
		case rules.RuleError:
			summary.Errors++
		}
	}

	headline := fmt.Sprintf("%d prioritized findings across %d resources", len(findings), len(in.Snapshot.Resources))
	if in.Metadata.ProjectName != "" {
		headline = fmt.Sprintf("%s — %s", in.Metadata.ProjectName, headline)
	}

	doc := &Document{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   in.GeneratedAt.UTC(),
		Analyzer: AnalyzerMeta{
			Version:        in.AnalyzerVersion,
			SnapshotID:     string(in.Run.SnapshotID),
			AnalysisRunID:  string(in.Run.ID),
			RuleSetVersion: in.Run.RuleSetVersion,
		},
		Customer: CustomerMeta{
			CustomerName: in.Metadata.CustomerName,
			ProjectName:  in.Metadata.ProjectName,
			PreparedBy:   in.Metadata.PreparedBy,
		},
		Scope: scope,
		Executive: ExecutiveSummary{
			Headline:           headline,
			FindingCount:       len(findings),
			ChecksPassed:       summary.Passed,
			ChecksFailed:       summary.Failed,
			ChecksSuppressed:   summary.Suppressed,
			ChecksNotEvaluated: summary.NotEvaluated,
			CheckErrors:        summary.Errors,
			SummaryText: fmt.Sprintf(
				"Deterministic rules evaluated inventory and cost signals from snapshot %s. "+
					"Findings include evidence and remediation guidance; savings estimates are not aggregated across overlapping recommendations.",
				in.Run.SnapshotID,
			),
		},
		Costs:      costs,
		Savings:    savings,
		Findings:   findings,
		Appendix:   appendix,
		Disclaimer: disclaimerText,
	}
	doc.Scope.DataQualityNotes = warnings
	return doc, nil
}

func buildScope(snap *domain.CollectionSnapshot, aliases *AliasMap) (ScopeSection, []string) {
	var warnings []string
	providers := []string{string(snap.Provider)}
	regions := make([]string, 0, len(snap.Regions))
	for _, r := range snap.Regions {
		label := r.DisplayName
		if label == "" {
			label = r.ProviderRegionID
		}
		regions = append(regions, label)
	}
	sort.Strings(regions)

	var obsStart, obsEnd types.Timestamp
	for _, c := range snap.Costs {
		if obsStart.IsZero() || c.PeriodStart.Before(obsStart.Time) {
			obsStart = c.PeriodStart
		}
		if obsEnd.IsZero() || c.PeriodEnd.After(obsEnd.Time) {
			obsEnd = c.PeriodEnd
		}
	}
	if obsStart.IsZero() && !snap.StartedAt.IsZero() {
		obsStart = snap.StartedAt
		obsEnd = snap.StartedAt
		if snap.CompletedAt != nil {
			obsEnd = *snap.CompletedAt
		}
	}

	if len(snap.Costs) == 0 && snap.Provider == types.ProviderGCP {
		warnings = append(warnings, "No GCP billing rows were present; cost-based checks and CUD analysis may be not evaluated. Use collect gcp cost with BigQuery export or --offline fixtures.")
	}
	if len(snap.Costs) == 0 {
		warnings = append(warnings, "No billing cost records were present in the snapshot; cost-based checks may be not evaluated.")
	} else if snap.Provider == types.ProviderGCP {
		warnings = append(warnings, "GCP costs reflect BigQuery billing export normalization (credits, SUD, CUD, and taxes as separate effects); list-price catalog does not backfill missing export rows.")
	}
	if len(snap.Metrics) == 0 {
		warnings = append(warnings, "No utilization metrics were present in the snapshot; utilization-based checks may be not evaluated.")
	} else if snap.MetricsMeta != nil && snap.MetricsMeta.Partial {
		warnings = append(warnings, "Utilization metrics collection was partial; review appendix for coverage and diagnostics.")
	}
	if snap.MetricsMeta != nil && !snap.MetricsMeta.Window.Start.IsZero() {
		if obsStart.IsZero() || snap.MetricsMeta.Window.Start.Before(obsStart.Time) {
			obsStart = snap.MetricsMeta.Window.Start
		}
		if obsEnd.IsZero() || snap.MetricsMeta.Window.End.After(obsEnd.Time) {
			obsEnd = snap.MetricsMeta.Window.End
		}
	}
	for _, rel := range snap.Relationships {
		if rel.TargetMissing {
			warnings = append(warnings, fmt.Sprintf("Relationship target missing for resource %s (external reference only).", rel.FromResourceID))
			break
		}
	}

	acctName := snap.Account.DisplayName
	if aliases.RedactEnabled {
		acctName = aliases.AccountAlias
	}

	return ScopeSection{
		Providers:        providers,
		Regions:          regions,
		ResourceCount:    len(snap.Resources),
		ObservationStart: obsStart.Canonical(),
		ObservationEnd:   obsEnd.Canonical(),
		Accounts: []AccountScope{{
			DisplayName: acctName,
			Alias:       aliases.AccountAlias,
			Provider:    string(snap.Provider),
		}},
	}, warnings
}

func buildCosts(snap *domain.CollectionSnapshot) CostSection {
	totals := map[string]int64{}
	byService := map[string]int64{}
	byRegion := map[string]int64{}
	byOwner := map[string]int64{}
	var attributed, unattributed int64
	resourceOwners := map[types.ResourceID]string{}
	for _, res := range snap.Resources {
		for _, tag := range res.Tags {
			if tag.Key == "Owner" {
				resourceOwners[res.ID] = tag.Value
			}
		}
	}
	for _, c := range snap.Costs {
		cur := c.Amount.Currency
		if cur == "" {
			continue
		}
		totals[cur] += c.Amount.AmountMinor
		if c.ResourceID == domain.UnattributedResourceID {
			unattributed += c.Amount.AmountMinor
		} else {
			attributed += c.Amount.AmountMinor
		}
		if c.Service != "" {
			byService[c.Service] += c.Amount.AmountMinor
		}
		if c.RegionID != "" {
			byRegion[string(c.RegionID)] += c.Amount.AmountMinor
		}
		owner := resourceOwners[c.ResourceID]
		if owner == "" {
			owner = "unknown"
		}
		if c.ResourceID != domain.UnattributedResourceID {
			byOwner[owner] += c.Amount.AmountMinor
		}
	}
	var byCur []CurrencyTotal
	for cur, minor := range totals {
		byCur = append(byCur, CurrencyTotal{
			Currency:    cur,
			AmountMajor: float64(minor) / 100.0,
			Kind:        KindMeasured,
			Note:        "Sum of cost records in snapshot; mixed granularities are not normalized to a monthly run rate.",
		})
	}
	sort.Slice(byCur, func(i, j int) bool { return byCur[i].Currency < byCur[j].Currency })

	covNote := ""
	if attributed+unattributed > 0 {
		pct := float64(attributed) / float64(attributed+unattributed) * 100
		covNote = fmt.Sprintf("Attribution coverage: %.1f%% attributed by volume (%d minor units attributed, %d unattributed).", pct, attributed, unattributed)
	}

	return CostSection{
		Kind:                KindMeasured,
		ByCurrency:          byCur,
		PeriodNote:          "Totals are per recorded billing periods and must not be treated as annualized spend without normalization.",
		AttributionNote:     covNote,
		SpendByServiceMinor: byService,
		SpendByRegionMinor:  byRegion,
		SpendByOwnerMinor:   byOwner,
	}
}

func buildSavings(_ *domain.CollectionSnapshot, findings []domain.Finding, recByFinding map[types.FindingID]domain.Recommendation) SavingsSection {
	sec := SavingsSection{
		Kind: KindEstimate,
		Note: "No guaranteed savings. Monthly totals use low/high ranges and exclude investigation-only and overlap-suppressed estimates.",
	}
	var recs []domain.Recommendation
	for _, f := range findings {
		rec, ok := recByFinding[f.ID]
		if !ok {
			continue
		}
		if rec.InvestigationOnly || rec.EstSavings == nil {
			continue
		}
		line := SavingsLine{
			Description: f.Title,
			Currency:    rec.EstSavings.Currency,
			AmountMajor: float64(rec.EstSavings.AmountMinor) / 100.0,
			Kind:        KindEstimate,
			FindingID:   string(f.ID),
		}
		if rec.EstSavingsLow != nil {
			line.AmountLowMajor = float64(rec.EstSavingsLow.AmountMinor) / 100.0
		}
		if rec.EstSavingsHigh != nil {
			line.AmountHighMajor = float64(rec.EstSavingsHigh.AmountMinor) / 100.0
		}
		switch rec.SavingsClass {
		case domain.SavingsOneTime:
			sec.OneTime = append(sec.OneTime, line)
		case domain.SavingsCommitment:
			sec.CommitmentBased = append(sec.CommitmentBased, line)
		default:
			sec.MonthlyRecurring = append(sec.MonthlyRecurring, line)
		}
		recs = append(recs, rec)
	}
	sec.MonthlyTotalLow, sec.MonthlyTotalHigh = savingsAggregate(recs)
	sort.Slice(sec.MonthlyRecurring, func(i, j int) bool {
		if sec.MonthlyRecurring[i].FindingID != sec.MonthlyRecurring[j].FindingID {
			return sec.MonthlyRecurring[i].FindingID < sec.MonthlyRecurring[j].FindingID
		}
		return sec.MonthlyRecurring[i].Description < sec.MonthlyRecurring[j].Description
	})
	return sec
}

func savingsAggregate(recs []domain.Recommendation) (low, high map[string]float64) {
	low = map[string]float64{}
	high = map[string]float64{}
	for _, rec := range recs {
		if rec.InvestigationOnly || rec.EstSavings == nil || rec.SavingsClass == domain.SavingsCommitment || rec.SavingsClass == domain.SavingsOneTime {
			continue
		}
		cur := rec.EstSavings.Currency
		l := float64(rec.EstSavings.AmountMinor) / 100.0
		h := l
		if rec.EstSavingsLow != nil {
			l = float64(rec.EstSavingsLow.AmountMinor) / 100.0
		}
		if rec.EstSavingsHigh != nil {
			h = float64(rec.EstSavingsHigh.AmountMinor) / 100.0
		}
		low[cur] += l
		high[cur] += h
	}
	return low, high
}

func buildFindings(
	findings []domain.Finding,
	evidenceByID map[int64]domain.Evidence,
	recByFinding map[types.FindingID]domain.Recommendation,
	snap *domain.CollectionSnapshot,
	aliases *AliasMap,
) []FindingEntry {
	sorted := append([]domain.Finding(nil), findings...)
	sort.Slice(sorted, func(i, j int) bool {
		si, sj := severityRank(sorted[i].Severity), severityRank(sorted[j].Severity)
		if si != sj {
			return si > sj
		}
		if sorted[i].Category != sorted[j].Category {
			return sorted[i].Category < sorted[j].Category
		}
		if sorted[i].RuleID != sorted[j].RuleID {
			return sorted[i].RuleID < sorted[j].RuleID
		}
		return sorted[i].Fingerprint < sorted[j].Fingerprint
	})

	out := make([]FindingEntry, 0, len(sorted))
	for _, f := range sorted {
		entry := FindingEntry{
			ID:          string(f.ID),
			RuleID:      f.RuleID,
			Fingerprint: f.Fingerprint,
			Severity:    string(f.Severity),
			Category:    f.Category,
			Title:       f.Title,
			Description: f.Description,
			Confidence:  f.Confidence.Float64(),
			Assumptions: append([]string(nil), f.Assumptions...),
		}
		for _, rid := range f.ResourceIDs {
			res := resourceByID(snap, rid)
			ref := ResourceRef{
				Alias: aliases.Resource(rid),
				Kind:  string(res.Kind),
			}
			if !aliases.RedactEnabled {
				ref.Name = res.Name
			}
			entry.Resources = append(entry.Resources, ref)
		}
		sort.Slice(entry.Resources, func(i, j int) bool { return entry.Resources[i].Alias < entry.Resources[j].Alias })

		for _, eid := range f.EvidenceIDs {
			ev, ok := evidenceByID[eid]
			if !ok {
				entry.Evidence = append(entry.Evidence, EvidenceEntry{
					Kind:    "missing",
					Summary: "Evidence record not found in analysis run",
					Missing: true,
					KindTag: KindDerived,
				})
				continue
			}
			kindTag := KindMeasured
			switch ev.Kind {
			case domain.EvidenceDerived:
				kindTag = KindDerived
			case domain.EvidenceCost, domain.EvidenceMetric, domain.EvidenceResource, domain.EvidenceRelationship:
				kindTag = KindMeasured
			}
			entry.Evidence = append(entry.Evidence, EvidenceEntry{
				Kind:    string(ev.Kind),
				Summary: ev.Summary,
				Detail:  aliases.RedactDetail(ev.Detail),
				Source:  ev.Provenance.Source,
				KindTag: kindTag,
			})
		}

		rec := recByFinding[f.ID]
		entry.Remediation = RemediationEntry{
			Summary:   rec.Summary,
			Steps:     append([]string(nil), rec.Steps...),
			RiskLevel: rec.RiskLevel,
			Rollback:  rollbackForRule(f.RuleID),
			Kind:      KindRecommendation,
		}
		if entry.Remediation.Summary == "" {
			entry.Remediation.Summary = "Review resource configuration and apply changes during a maintenance window after validation."
		}
		out = append(out, entry)
	}
	return out
}

func rollbackForRule(ruleID string) string {
	switch {
	case strings.HasPrefix(ruleID, "compute."):
		return "Stop or resize instances via your standard change process; keep recent AMIs/snapshots before terminating."
	case strings.HasPrefix(ruleID, "storage."):
		return "Create a snapshot or backup before deleting volumes or snapshots; restore from snapshot if data is needed."
	case strings.HasPrefix(ruleID, "governance."):
		return "Tag changes are reversible; document previous tag values before bulk updates."
	default:
		return "Document current configuration and verify rollback steps in a test environment before production changes."
	}
}

func buildAppendix(executions []rules.RuleExecution, snap *domain.CollectionSnapshot, aliases *AliasMap) Appendix {
	var ap Appendix
	for _, ex := range executions {
		item := RuleOutcome{RuleID: ex.RuleID, Status: string(ex.Status), Message: ex.Message}
		switch ex.Status {
		case rules.RuleSuppressed:
			ap.Suppressed = append(ap.Suppressed, item)
		case rules.RuleNotEvaluated:
			ap.NotEvaluated = append(ap.NotEvaluated, item)
		case rules.RulePassed:
			ap.Passed = append(ap.Passed, item)
		}
	}
	sortOutcomes := func(items []RuleOutcome) {
		sort.Slice(items, func(i, j int) bool { return items[i].RuleID < items[j].RuleID })
	}
	sortOutcomes(ap.Suppressed)
	sortOutcomes(ap.NotEvaluated)
	sortOutcomes(ap.Passed)
	ap.Utilization = buildUtilizationAppendix(snap, aliases)
	return ap
}

func buildUtilizationAppendix(snap *domain.CollectionSnapshot, aliases *AliasMap) *UtilizationAppendix {
	if snap == nil || len(snap.UtilizationSignals) == 0 {
		return nil
	}
	out := &UtilizationAppendix{}
	if snap.MetricsMeta != nil {
		out.WindowStart = snap.MetricsMeta.Window.Start.Canonical()
		out.WindowEnd = snap.MetricsMeta.Window.End.Canonical()
		out.PeriodSeconds = snap.MetricsMeta.Window.PeriodSeconds
	}
	type key struct {
		res, metric string
	}
	grouped := map[key]*ResourceUtilizationEntry{}
	for _, sig := range snap.UtilizationSignals {
		k := key{string(sig.ResourceID), sig.MetricName}
		entry, ok := grouped[k]
		if !ok {
			alias := string(sig.ResourceID)
			if aliases != nil {
				alias = aliases.Resource(sig.ResourceID)
			}
			entry = &ResourceUtilizationEntry{Alias: alias, Metric: sig.MetricName}
			grouped[k] = entry
		}
		if sig.Kind == domain.SignalSampleCoverage {
			entry.SampleCoverage = sig.Value
			continue
		}
		entry.Signals = append(entry.Signals, SignalEntry{
			Kind:    string(sig.Kind),
			Value:   sig.Value,
			Unit:    sig.Unit,
			KindTag: KindDerived,
		})
	}
	for _, e := range grouped {
		sort.Slice(e.Signals, func(i, j int) bool { return e.Signals[i].Kind < e.Signals[j].Kind })
		out.Resources = append(out.Resources, *e)
	}
	sort.Slice(out.Resources, func(i, j int) bool {
		if out.Resources[i].Alias == out.Resources[j].Alias {
			return out.Resources[i].Metric < out.Resources[j].Metric
		}
		return out.Resources[i].Alias < out.Resources[j].Alias
	})
	return out
}

func severityRank(s domain.FindingSeverity) int {
	switch s {
	case domain.SeverityCritical:
		return 5
	case domain.SeverityHigh:
		return 4
	case domain.SeverityMedium:
		return 3
	case domain.SeverityLow:
		return 2
	case domain.SeverityInfo:
		return 1
	default:
		return 0
	}
}
