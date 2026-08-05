package domain

import "github.com/kernul-io/cloudopt/internal/application/domain/types"

// Region is a provider region within an account.
type Region struct {
	ID               types.RegionID
	ProviderRegionID string
	DisplayName      string
	Provenance       Provenance
}
