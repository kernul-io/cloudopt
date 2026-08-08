package rules

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
)

// RuleStatus describes per-rule execution outcome for summaries.
type RuleStatus string

const (
	RulePassed       RuleStatus = "passed"
	RuleFailed       RuleStatus = "failed"
	RuleSuppressed   RuleStatus = "suppressed"
	RuleNotEvaluated RuleStatus = "not_evaluated"
	RuleError        RuleStatus = "error"
)

// RuleExecution captures one rule's outcome.
type RuleExecution struct {
	RuleID   string
	Status   RuleStatus
	Findings []domain.Finding
	Message  string
}

// AnalyzeInput selects rules and snapshot data for evaluation.
type AnalyzeInput struct {
	Snapshot       *domain.CollectionSnapshot
	Manifest       *Manifest
	Registry       *Registry
	Suppressions   *SuppressionIndex
	RuleFilter     []string
	CategoryFilter []string
}

// AnalyzeOutput is the deterministic evaluation result.
type AnalyzeOutput struct {
	RulesetVersion  string
	Executions      []RuleExecution
	Findings        []domain.Finding
	Recommendations []domain.Recommendation
	Evidence        []domain.Evidence
	Summary         Summary
}

// Summary counts rule outcomes.
type Summary struct {
	Passed       int
	Failed       int
	Suppressed   int
	NotEvaluated int
	Errors       int
}

// Engine evaluates manifests against snapshots.
type Engine struct{}

func (Engine) Analyze(in AnalyzeInput) (*AnalyzeOutput, error) {
	if in.Snapshot == nil || in.Manifest == nil || in.Registry == nil {
		return nil, fmt.Errorf("snapshot, manifest, and registry are required")
	}
	if !in.Snapshot.IsAnalyzable() {
		return nil, fmt.Errorf("snapshot %q is not analyzable (status=%s)", in.Snapshot.ID, in.Snapshot.Status)
	}

	view := NewSnapshotView(in.Snapshot)
	prov := domain.Provenance{
		Quality:    domain.QualityDerived,
		Source:     "rule-engine",
		ObservedAt: view.ObservedAt(),
	}

	var executions []RuleExecution
	var allFindings []domain.Finding
	var allRecs []domain.Recommendation
	var allEvidence []domain.Evidence
	var summary Summary

	nextEvidenceID := int64(1)

	for _, rule := range in.Manifest.Rules {
		if !rule.Enabled {
			continue
		}
		if !matchesFilter(rule.ID, in.RuleFilter) {
			continue
		}
		if !matchesCategory(rule.Category, in.CategoryFilter) {
			continue
		}

		exec := RuleExecution{RuleID: rule.ID}
		missing := view.MissingSignals(rule.RequiredSignals)
		if len(missing) > 0 {
			exec.Status = RuleNotEvaluated
			exec.Message = fmt.Sprintf("missing required signals: %s", strings.Join(missing, ", "))
			summary.NotEvaluated++
			executions = append(executions, exec)
			continue
		}

		ev, err := in.Registry.Get(rule.Evaluator)
		if err != nil {
			exec.Status = RuleError
			exec.Message = err.Error()
			summary.Errors++
			executions = append(executions, exec)
			continue
		}

		result := safeEvaluate(ev, view, rule)
		if result.NotEvaluated {
			exec.Status = RuleNotEvaluated
			exec.Message = result.Reason
			summary.NotEvaluated++
			executions = append(executions, exec)
			continue
		}

		ruleFindings := make([]domain.Finding, 0, len(result.Findings))
		suppressedCount := 0
		for _, cand := range result.Findings {
			fp := Fingerprint(rule.ID, rule.Version, cand.ResourceIDs)
			if in.Suppressions != nil {
				if ok, reason := in.Suppressions.IsSuppressed(fp, rule.ID, cand.ResourceIDs); ok {
					suppressedCount++
					_ = reason
					continue
				}
			}

			evidenceIDs := make([]int64, 0, len(cand.Evidence))
			for _, draft := range cand.Evidence {
				e := domain.Evidence{
					ID:         nextEvidenceID,
					Kind:       draft.Kind,
					ResourceID: draft.ResourceID,
					Summary:    draft.Summary,
					Detail:     draft.Detail,
					Provenance: prov,
				}
				allEvidence = append(allEvidence, e)
				evidenceIDs = append(evidenceIDs, nextEvidenceID)
				nextEvidenceID++
			}

			findingID := types.FindingID("find-" + fp[:16])
			conf := cand.Confidence
			if conf.BasisPoints == 0 {
				conf = types.PercentageFromFloat(1.0)
			}
			f := domain.Finding{
				ID:          findingID,
				RuleID:      rule.ID,
				Fingerprint: fp,
				Severity:    domain.FindingSeverity(rule.Severity),
				Category:    rule.Category,
				Title:       defaultString(cand.Title, rule.Title),
				Description: cand.Description,
				ResourceIDs: cand.ResourceIDs,
				EvidenceIDs: evidenceIDs,
				Assumptions: cand.Assumptions,
				Confidence:  conf,
				Provenance:  prov,
			}
			ruleFindings = append(ruleFindings, f)
			allFindings = append(allFindings, f)

			if strings.TrimSpace(rule.Remediation) != "" {
				allRecs = append(allRecs, domain.Recommendation{
					ID:         int64(len(allRecs) + 1),
					FindingID:  findingID,
					Summary:    strings.TrimSpace(rule.Remediation),
					Steps:      remediationSteps(rule.Remediation),
					RiskLevel:  "medium",
					Provenance: prov,
				})
			}
		}

		exec.Findings = ruleFindings
		switch {
		case len(ruleFindings) == 0 && suppressedCount > 0:
			exec.Status = RuleSuppressed
			summary.Suppressed++
		case len(ruleFindings) == 0:
			exec.Status = RulePassed
			summary.Passed++
		default:
			exec.Status = RuleFailed
			summary.Failed++
		}
		executions = append(executions, exec)
	}

	sort.Slice(allFindings, func(i, j int) bool {
		if allFindings[i].RuleID != allFindings[j].RuleID {
			return allFindings[i].RuleID < allFindings[j].RuleID
		}
		return allFindings[i].Fingerprint < allFindings[j].Fingerprint
	})

	return &AnalyzeOutput{
		RulesetVersion:  in.Manifest.RulesetVersion,
		Executions:      executions,
		Findings:        allFindings,
		Recommendations: allRecs,
		Evidence:        allEvidence,
		Summary:         summary,
	}, nil
}

func safeEvaluate(ev Evaluator, view *SnapshotView, rule RuleSpec) (res EvaluatorResult) {
	defer func() {
		if r := recover(); r != nil {
			res = EvaluatorResult{NotEvaluated: true, Reason: fmt.Sprintf("evaluator panic: %v", r)}
		}
	}()
	return ev.Evaluate(view, rule)
}

func matchesFilter(ruleID string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, f := range filter {
		if f == ruleID {
			return true
		}
	}
	return false
}

func matchesCategory(category string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, f := range filter {
		if f == category {
			return true
		}
	}
	return false
}

func remediationSteps(text string) []string {
	lines := strings.Split(text, "\n")
	var steps []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			steps = append(steps, line)
		}
	}
	return steps
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

// NewAnalysisRunID generates a unique analysis run identifier.
func NewAnalysisRunID() (types.AnalysisRunID, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return types.AnalysisRunID("run-" + hex.EncodeToString(b[:])), nil
}

// CompletedAt returns the completion timestamp for an analysis run.
func CompletedAt(start types.Timestamp) *types.Timestamp {
	now := types.NewTimestamp(time.Now().UTC())
	return &now
}
