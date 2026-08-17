package domain

import "github.com/kernul-io/cloudopt/internal/domain/types"

// ServiceCollectionStatus tracks read-only inventory collection for one service scope.
type ServiceCollectionStatus struct {
	Service string
	Region  string // empty for global/account-scoped services
	Status  ServiceCollectionState
	Message string
}

// ServiceCollectionState is the outcome of a single collector unit.
type ServiceCollectionState string

const (
	ServiceCollectionOK      ServiceCollectionState = "ok"
	ServiceCollectionPartial ServiceCollectionState = "partial"
	ServiceCollectionFailed  ServiceCollectionState = "failed"
	ServiceCollectionSkipped ServiceCollectionState = "skipped"
)

// CollectionCoverage summarizes per-service results on a snapshot.
type CollectionCoverage struct {
	Services []ServiceCollectionStatus
}

// HasFailures returns true when any service unit failed or was partial.
func (c CollectionCoverage) HasFailures() bool {
	for _, s := range c.Services {
		if s.Status == ServiceCollectionFailed || s.Status == ServiceCollectionPartial {
			return true
		}
	}
	return false
}

// ObservedAt helper for provenance on collected inventory.
func CollectProvenance(source string, at types.Timestamp) Provenance {
	return Provenance{
		Quality:    QualityObserved,
		Source:     source,
		ObservedAt: at,
	}
}
