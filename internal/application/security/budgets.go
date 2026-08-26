package security

import (
	"fmt"
	"time"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// CollectionBudget bounds provider collection work for safety and cost control.
type CollectionBudget struct {
	MaxResources     int
	MaxAPICalls      int
	MaxDuration      time.Duration
	MaxConcurrent    int
	MaxCostQueryRows int
	MaxDiskBytes     int64
}

// DefaultBudget returns conservative defaults per provider.
func DefaultBudget(provider types.Provider) CollectionBudget {
	b := CollectionBudget{
		MaxResources:     50_000,
		MaxAPICalls:      25_000,
		MaxDuration:      45 * time.Minute,
		MaxConcurrent:    8,
		MaxCostQueryRows: 500_000,
		MaxDiskBytes:     512 << 20,
	}
	switch provider {
	case types.ProviderGCP:
		b.MaxConcurrent = 6
	case types.ProviderAWS, "":
		b.MaxConcurrent = 8
	default:
		b.MaxConcurrent = 4
	}
	return b
}

// ClampConcurrency applies budget limits to requested concurrency.
func (b CollectionBudget) ClampConcurrency(requested int) int {
	if requested <= 0 {
		return b.MaxConcurrent
	}
	if requested > b.MaxConcurrent {
		return b.MaxConcurrent
	}
	return requested
}

// ValidateSnapshotResult checks a finished snapshot against budget limits.
func (b CollectionBudget) ValidateSnapshotResult(snap *domain.CollectionSnapshot, elapsed time.Duration, apiCalls int) error {
	if snap == nil {
		return fmt.Errorf("snapshot is nil")
	}
	if b.MaxResources > 0 && len(snap.Resources) > b.MaxResources {
		return fmt.Errorf("resource budget exceeded: %d > %d", len(snap.Resources), b.MaxResources)
	}
	if b.MaxAPICalls > 0 && apiCalls > b.MaxAPICalls {
		return fmt.Errorf("api call budget exceeded: %d > %d", apiCalls, b.MaxAPICalls)
	}
	if b.MaxDuration > 0 && elapsed > b.MaxDuration {
		return fmt.Errorf("collection time budget exceeded: %s > %s", elapsed, b.MaxDuration)
	}
	return nil
}
