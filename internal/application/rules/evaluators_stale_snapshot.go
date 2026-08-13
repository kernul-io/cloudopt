// TODO: if it useful for anyone
package rules

import (
	"fmt"
	"time"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
)

// StaleVolumeSnapshot flags volume snapshots older than a configured threshold.
type StaleVolumeSnapshot struct{}

func (StaleVolumeSnapshot) Name() string { return "stale_volume_snapshot" }

func (StaleVolumeSnapshot) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	maxDays, err := rule.thresholdInt("max_age_days", 90)
	if err != nil {
		return EvaluatorResult{NotEvaluated: true, Reason: err.Error()}
	}
	maxAge := time.Duration(maxDays) * 24 * time.Hour
	observed := view.ObservedAt().Time

	snapshots := view.ResourcesOfKind(domain.KindSnapshot)
	if len(snapshots) == 0 {
		return EvaluatorResult{
			NotEvaluated: true,
			Reason:       "no volume_snapshot resources in snapshot",
		}
	}

	var findings []CandidateFinding
	for _, res := range snapshots {
		createdRaw, ok := res.Attributes["created_at"]
		if !ok || createdRaw == "" {
			continue
		}
		created, err := types.ParseTimestamp(createdRaw)
		if err != nil {
			continue
		}
		age := observed.Sub(created.Time)
		if age < maxAge {
			continue
		}
		findings = append(findings, CandidateFinding{
			Title: rule.Title,
			Description: fmt.Sprintf(
				"Volume snapshot %q (%s) was created %s ago (created_at=%s, threshold=%dd).",
				res.Name, res.ProviderResourceID, age.Truncate(time.Hour), created.Canonical(), maxDays,
			),
			ResourceIDs: []types.ResourceID{res.ID},
			Evidence: []EvidenceDraft{
				{
					Kind:       domain.EvidenceResource,
					ResourceID: res.ID,
					Summary:    fmt.Sprintf("created_at=%s", created.Canonical()),
					Detail: map[string]string{
						"created_at":           createdRaw,
						"provider_resource_id": res.ProviderResourceID,
						"age_hours":            fmt.Sprintf("%d", int64(age.Hours())),
					},
				},
				{
					Kind:       domain.EvidenceDerived,
					ResourceID: res.ID,
					Summary:    fmt.Sprintf("observed_at=%s", observed.UTC().Format(time.RFC3339Nano)),
					Detail: map[string]string{
						"max_age_days": fmt.Sprintf("%d", maxDays),
					},
				},
			},
			Assumptions: []string{"Snapshot creation time is taken from resource attributes, not live API metadata."},
			Confidence:  types.PercentageFromFloat(0.85),
		})
	}
	return EvaluatorResult{Findings: findings}
}
