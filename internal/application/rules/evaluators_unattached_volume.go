package rules

import (
	"fmt"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
)

// UnattachedBlockVolume flags block volumes that are not attached to an instance.
type UnattachedBlockVolume struct{}

func (UnattachedBlockVolume) Name() string { return "unattached_block_volume" }

func (UnattachedBlockVolume) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	var findings []CandidateFinding
	for _, res := range view.ResourcesOfKind(domain.KindBlockVolume) {
		attached := view.VolumeAttached(res.ID)
		if attached {
			continue
		}
		if !availableVolumeState(res.State) && res.State != "" && res.State != "in-use" {
			// Treat unknown non in-use states as unattached when graph has no attachment edge.
		} else if res.State == "in-use" {
			continue
		}

		desc := fmt.Sprintf("Block volume %q (%s) in state %q has no attached_to relationship in the snapshot.", res.Name, res.ProviderResourceID, res.State)
		evidence := []EvidenceDraft{
			{
				Kind:       domain.EvidenceResource,
				ResourceID: res.ID,
				Summary:    fmt.Sprintf("volume state=%s", res.State),
				Detail: map[string]string{
					"state":                res.State,
					"provider_resource_id": res.ProviderResourceID,
				},
			},
			{
				Kind:       domain.EvidenceRelationship,
				ResourceID: res.ID,
				Summary:    "no attached_to edge from volume",
				Detail:     map[string]string{"attached": "false"},
			},
		}
		if total, ok := view.SumCostsMinor(res.ID); ok {
			evidence = append(evidence, EvidenceDraft{
				Kind:       domain.EvidenceCost,
				ResourceID: res.ID,
				Summary:    fmt.Sprintf("recorded cost %s", total.String()),
				Detail: map[string]string{
					"amount_minor": fmt.Sprintf("%d", total.AmountMinor),
					"currency":     total.Currency,
				},
			})
			desc += fmt.Sprintf(" Recorded cost: %s.", total.String())
		}

		findings = append(findings, CandidateFinding{
			Title:       rule.Title,
			Description: desc,
			ResourceIDs: []types.ResourceID{res.ID},
			Evidence:    evidence,
			Assumptions: []string{"Attachment graph is complete for the collection scope."},
			Confidence:  types.PercentageFromFloat(0.95),
		})
	}
	return EvaluatorResult{Findings: findings}
}
