package rules

import (
	"fmt"
	"time"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// OrphanedVolumeSnapshot flags snapshots whose source volume/disk no longer exists.
type OrphanedVolumeSnapshot struct{}

func (OrphanedVolumeSnapshot) Name() string { return "orphaned_volume_snapshot" }

func (OrphanedVolumeSnapshot) Evaluate(view *SnapshotView, rule RuleSpec) EvaluatorResult {
	minAgeDays, err := rule.thresholdInt("min_age_days", 7)
	if err != nil {
		return EvaluatorResult{NotEvaluated: true, Reason: err.Error()}
	}
	minAge := time.Duration(minAgeDays) * 24 * time.Hour
	observed := view.ObservedAt().Time

	// Build a set of all volume/disk provider IDs present in the snapshot.
	volumeIDs := make(map[string]bool)
	for _, res := range view.ResourcesOfKind(domain.KindBlockVolume) {
		if res.ProviderResourceID != "" {
			volumeIDs[res.ProviderResourceID] = true
		}
	}

	var findings []CandidateFinding
	for _, res := range view.ResourcesOfKind(domain.KindSnapshot) {
		sourceID := snapshotSourceID(res)
		if sourceID == "" {
			continue // Cannot determine orphan status; skip.
		}
		// If the source volume still exists, not orphaned.
		if volumeIDs[sourceID] {
			continue
		}

		// Apply grace period based on snapshot creation time.
		createdRaw := res.Attributes["created_at"]
		if createdRaw == "" {
			createdRaw = res.Attributes["started_at"]
		}
		if createdRaw != "" {
			if created, err := types.ParseTimestamp(createdRaw); err == nil {
				if observed.Sub(created.Time) < minAge {
					continue // Too recent; could be normal lifecycle lag.
				}
			}
		}

		findings = append(findings, CandidateFinding{
			Title:       rule.Title,
			Description: fmt.Sprintf("Snapshot %q (%s) references source volume %q which is not present in the current inventory.", res.Name, res.ProviderResourceID, sourceID),
			ResourceIDs: []types.ResourceID{res.ID},
			Evidence: []EvidenceDraft{
				{
					Kind:       domain.EvidenceResource,
					ResourceID: res.ID,
					Summary:    fmt.Sprintf("source_volume_id=%s", sourceID),
					Detail: map[string]string{
						"snapshot_id":          res.ProviderResourceID,
						"source_volume_id":     sourceID,
						"state":                res.State,
					},
				},
				{
					Kind:       domain.EvidenceDerived,
					ResourceID: res.ID,
					Summary:    "source volume not found in current inventory",
					Detail:     map[string]string{"source_volume_exists": "false"},
				},
			},
			Assumptions: []string{
				"Inventory collection covered all regions/projects where the source volume may have existed.",
			},
			Confidence: types.PercentageFromFloat(0.9),
		})
	}
	return EvaluatorResult{Findings: findings}
}

// snapshotSourceID extracts the source volume identifier from AWS or GCP snapshot attributes.
func snapshotSourceID(res domain.Resource) string {
	if res.Attributes != nil {
		if v := res.Attributes["volume_id"]; v != "" {
			return v
		}
		if v := res.Attributes["source_disk"]; v != "" {
			return v
		}
	}
	return ""
}
