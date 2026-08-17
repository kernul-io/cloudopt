package rules

import (
	"fmt"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// StoppedInstanceStorageCost flags stopped compute instances with non-zero cost.
type StoppedInstanceStorageCost struct{}

func (StoppedInstanceStorageCost) Name() string { return "stopped_instance_storage_cost" }

func (StoppedInstanceStorageCost) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	minMinor, err := rule.thresholdInt("min_cost_minor", 1)
	if err != nil {
		return EvaluatorResult{NotEvaluated: true, Reason: err.Error()}
	}

	var findings []CandidateFinding
	for _, res := range view.ResourcesOfKind(domain.KindComputeInstance) {
		if !stoppedState(res.State) {
			continue
		}
		total, ok := view.SumCostsMinor(res.ID)
		if !ok {
			continue
		}
		if total.AmountMinor < minMinor {
			continue
		}
		findings = append(findings, CandidateFinding{
			Title:       rule.Title,
			Description: fmt.Sprintf("Compute instance %q (%s) is %s but has recorded cost %s in the snapshot period.", res.Name, res.ProviderResourceID, res.State, total.String()),
			ResourceIDs: []types.ResourceID{res.ID},
			Evidence: []EvidenceDraft{
				{
					Kind:       domain.EvidenceResource,
					ResourceID: res.ID,
					Summary:    fmt.Sprintf("instance state=%s", res.State),
					Detail: map[string]string{
						"state":                res.State,
						"provider_resource_id": res.ProviderResourceID,
					},
				},
				{
					Kind:       domain.EvidenceCost,
					ResourceID: res.ID,
					Summary:    fmt.Sprintf("aggregated cost %s", total.String()),
					Detail: map[string]string{
						"amount_minor": fmt.Sprintf("%d", total.AmountMinor),
						"currency":     total.Currency,
					},
				},
			},
			Assumptions: []string{"Cost records reflect storage and other charges still billed while the instance is stopped."},
			Confidence:  types.PercentageFromFloat(0.9),
		})
	}
	return EvaluatorResult{Findings: findings}
}
