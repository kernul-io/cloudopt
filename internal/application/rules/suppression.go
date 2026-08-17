package rules

import (
	"strings"
	"time"

	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// SuppressionEntry suppresses findings matching fingerprint, resource, and/or rule.
type SuppressionEntry struct {
	Fingerprint string           `yaml:"fingerprint"`
	ResourceID  types.ResourceID `yaml:"resource_id"`
	RuleID      string           `yaml:"rule_id"`
	Reason      string           `yaml:"reason"`
	ExpiresAt   string           `yaml:"expires_at"`
}

// SuppressionList is loaded from workspace configuration.
type SuppressionList struct {
	Suppressions []SuppressionEntry `yaml:"suppressions"`
}

// SuppressionIndex applies active suppressions at evaluation time.
type SuppressionIndex struct {
	entries []SuppressionEntry
	now     time.Time
}

func NewSuppressionIndex(entries []SuppressionEntry, now time.Time) *SuppressionIndex {
	return &SuppressionIndex{entries: entries, now: now.UTC()}
}

func (s *SuppressionIndex) Active(entry SuppressionEntry) bool {
	if strings.TrimSpace(entry.Reason) == "" {
		return false
	}
	if entry.ExpiresAt == "" {
		return true
	}
	exp, err := types.ParseTimestamp(entry.ExpiresAt)
	if err != nil {
		return false
	}
	return s.now.Before(exp.Time)
}

func (s *SuppressionIndex) IsSuppressed(fingerprint string, ruleID string, resourceIDs []types.ResourceID) (bool, string) {
	for _, e := range s.entries {
		if !s.Active(e) {
			continue
		}
		if e.Fingerprint != "" && e.Fingerprint != fingerprint {
			continue
		}
		if e.RuleID != "" && e.RuleID != ruleID {
			continue
		}
		if e.ResourceID != "" {
			match := false
			for _, id := range resourceIDs {
				if id == e.ResourceID {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		return true, e.Reason
	}
	return false, ""
}
