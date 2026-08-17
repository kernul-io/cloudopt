package ports

import (
	"context"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// PricingLoadOptions configures catalog loading (offline fixture or live API).
type PricingLoadOptions struct {
	Provider    types.Provider
	Offline     bool
	FixtureRoot string
	Regions     []string
}

// PricingCatalogResult is normalized pricing output for the application layer.
type PricingCatalogResult struct {
	Records    []domain.PricingRecord
	Source     string
	Partial    bool
	Diagnostic string
}

// PricingSource loads provider list prices into canonical pricing records.
type PricingSource interface {
	Capabilities() CapabilityManifest
	LoadCatalog(ctx context.Context, opts PricingLoadOptions) (*PricingCatalogResult, error)
}
