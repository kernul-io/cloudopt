package domain

import "github.com/kernul-io/cloudopt/internal/application/domain/types"

// ResourceKind classifies inventory entities for graph traversal and rules.
type ResourceKind string

const (
	KindComputeInstance ResourceKind = "compute_instance"
	KindInstanceType    ResourceKind = "instance_type"
	KindBlockVolume     ResourceKind = "block_volume"
	KindDatabase        ResourceKind = "database_instance"
	KindNATGateway      ResourceKind = "nat_gateway"
	KindSubnet          ResourceKind = "subnet"
	KindVPC             ResourceKind = "vpc"
	KindElasticIP       ResourceKind = "elastic_ip"
	KindSnapshot        ResourceKind = "volume_snapshot"
	KindRouteTable      ResourceKind = "route_table"
)

// Resource is a node in the inventory graph.
type Resource struct {
	ID                 types.ResourceID
	Kind               ResourceKind
	ProviderResourceID string
	AccountID          types.AccountID
	RegionID           types.RegionID
	Name               string
	State              string
	Tags               []Tag
	Attributes         map[string]string
	Provenance         Provenance
}
