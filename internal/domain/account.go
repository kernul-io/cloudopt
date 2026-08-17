package domain

import "github.com/kernul-io/cloudopt/internal/domain/types"

// Account is a provider-linked billing and inventory scope.
type Account struct {
	ID                types.AccountID
	Provider          types.Provider
	ProviderAccountID string
	DisplayName       string
	DefaultCurrency   string
	Provenance        Provenance
}
