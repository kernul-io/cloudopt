package rules

import (
	"fmt"
	"net"
	"strings"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// AWSIdleElasticIP flags allocated Elastic IP addresses with no association.
type AWSIdleElasticIP struct{}

func (AWSIdleElasticIP) Name() string { return "aws_idle_elastic_ip" }

func (AWSIdleElasticIP) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	var findings []CandidateFinding
	for _, res := range view.ResourcesOfKind(domain.KindElasticIP) {
		if !isAWSElasticIP(res) || !strings.EqualFold(strings.TrimSpace(res.State), "available") {
			continue
		}
		findings = append(findings, CandidateFinding{
			Title:       rule.Title,
			Description: fmt.Sprintf("Elastic IP %q (%s) is allocated but not associated with a resource.", res.Name, res.ProviderResourceID),
			ResourceIDs: []types.ResourceID{res.ID},
			Evidence: []EvidenceDraft{{
				Kind:       domain.EvidenceResource,
				ResourceID: res.ID,
				Summary:    "Elastic IP state=available",
				Detail: map[string]string{
					"state":                res.State,
					"provider_resource_id": res.ProviderResourceID,
					"public_ip":            res.Name,
				},
			}},
			Assumptions: []string{"AWS inventory collection completed for the address's region."},
			Confidence:  types.PercentageFromFloat(0.95),
		})
	}
	return EvaluatorResult{Findings: findings}
}

func isAWSElasticIP(res domain.Resource) bool {
	if isGCPResource(res) {
		return false
	}
	return strings.HasPrefix(res.ProviderResourceID, "eipalloc-") || net.ParseIP(res.ProviderResourceID) != nil
}
