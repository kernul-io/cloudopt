package rules

import (
	"fmt"
	"strings"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// MissingCostAllocationTags flags resources missing required tag keys.
type MissingCostAllocationTags struct{}

func (MissingCostAllocationTags) Name() string { return "missing_cost_allocation_tags" }

func (MissingCostAllocationTags) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	required := rule.thresholdStringList("required_tag_keys", []string{"Owner", "CostCenter"})
	if len(required) == 0 {
		return EvaluatorResult{NotEvaluated: true, Reason: "required_tag_keys threshold is empty"}
	}

	kinds := rule.Applicability.ResourceKinds
	if len(kinds) == 0 {
		kinds = []string{
			string(domain.KindComputeInstance),
			string(domain.KindBlockVolume),
			string(domain.KindDatabase),
		}
	}

	var findings []CandidateFinding
	for _, kindName := range kinds {
		for _, res := range view.ResourcesOfKind(domain.ResourceKind(kindName)) {
			var missing []string
			for _, key := range required {
				if resourceTagValue(res, key) == "" {
					missing = append(missing, key)
				}
			}
			if len(missing) == 0 {
				continue
			}
			findings = append(findings, CandidateFinding{
				Title: rule.Title,
				Description: fmt.Sprintf(
					"Resource %q (%s, kind=%s) is missing required tags: %s.",
					res.Name, res.ProviderResourceID, res.Kind, strings.Join(missing, ", "),
				),
				ResourceIDs: []types.ResourceID{res.ID},
				Evidence: []EvidenceDraft{
					{
						Kind:       domain.EvidenceResource,
						ResourceID: res.ID,
						Summary:    fmt.Sprintf("missing tags: %s", strings.Join(missing, ", ")),
						Detail: map[string]string{
							"missing_tag_keys": strings.Join(missing, ","),
						},
					},
				},
				Assumptions: []string{"Tag completeness reflects the collected inventory only."},
				Confidence:  types.PercentageFromFloat(1.0),
			})
		}
	}
	return EvaluatorResult{Findings: findings}
}
