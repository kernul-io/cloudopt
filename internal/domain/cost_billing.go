package domain

import "github.com/kernul-io/cloudopt/internal/domain/types"

// UnattributedResourceID marks spend that could not be mapped to inventory.
const UnattributedResourceID types.ResourceID = "__unattributed__"

// CostBasis identifies which AWS (or provider) cost metric was normalized.
type CostBasis string

const (
	CostBasisAmortizedNet CostBasis = "amortized_net"
	CostBasisAmortized    CostBasis = "amortized"
	CostBasisUnblended    CostBasis = "unblended"
	CostBasisNetUnblended CostBasis = "net_unblended"
)

// CostChargeKind classifies a billing line item.
type CostChargeKind string

const (
	ChargeUsage   CostChargeKind = "usage"
	ChargeCredit  CostChargeKind = "credit"
	ChargeRefund  CostChargeKind = "refund"
	ChargeTax     CostChargeKind = "tax"
	ChargeSupport CostChargeKind = "support"
)

// AttributionMethod documents how a cost row was linked to a resource.
type AttributionMethod string

const (
	AttributionDirectResourceID AttributionMethod = "direct_resource_id"
	AttributionTagOwner         AttributionMethod = "tag_owner"
	AttributionSharedService    AttributionMethod = "shared_service"
	AttributionUnattributed     AttributionMethod = "unattributed"
)

// CostAttribution carries allocation metadata for diagnostics and reconciliation.
type CostAttribution struct {
	Method      AttributionMethod
	HeuristicID string
	Confidence  float64
}

// BillingInterval is the provider-reported window for a cost query.
type BillingInterval struct {
	Start     types.Timestamp
	End       types.Timestamp
	Collected types.Timestamp
}

// CostDiagnostic explains gaps in billing or attribution quality.
type CostDiagnostic struct {
	Code     string
	Message  string
	Severity string
}
