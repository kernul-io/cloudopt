package domain

import "github.com/kernul-io/cloudopt/internal/application/domain/types"

// RelationshipKind describes an edge in the resource graph.
type RelationshipKind string

const (
	RelAttachedTo     RelationshipKind = "attached_to"
	RelInSubnet       RelationshipKind = "in_subnet"
	RelInVPC          RelationshipKind = "in_vpc"
	RelRoutesVia      RelationshipKind = "routes_via"
	RelAssociatedWith RelationshipKind = "associated_with"
)

// Relationship links two resources. TargetMissing is true when the target
// provider ID is known but the resource is not present in this snapshot.
type Relationship struct {
	ID                   int64
	Kind                 RelationshipKind
	FromResourceID       types.ResourceID
	ToResourceID         types.ResourceID
	ToProviderResourceID string
	TargetMissing        bool
	Provenance           Provenance
}
