package domain

import "github.com/kernul-io/cloudopt/internal/domain/types"

// PurchaseModel names how a price is billed.
type PurchaseModel string

const (
	PurchaseOnDemand     PurchaseModel = "on_demand"
	PurchaseReserved     PurchaseModel = "reserved"
	PurchaseSavingsPlan  PurchaseModel = "savings_plan"
	PurchaseCommittedUse PurchaseModel = "committed_use"
)

// PricingRecord is a versioned catalog row used for savings math.
type PricingRecord struct {
	SKU           string
	Service       string
	Region        string
	PurchaseModel PurchaseModel
	Currency      string
	EffectiveDate types.Timestamp
	Unit          string // e.g. hour, gb_month
	PriceMinor    int64  // per unit in minor currency
	Source        string
	Attributes    map[string]string // instance_type, volume_type, etc.
}
