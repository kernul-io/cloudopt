package types

// AccountID identifies a cloud account in the canonical model.
type AccountID string

// RegionID identifies a region within an account.
type RegionID string

// ResourceID is the canonical identifier for a resource within a snapshot.
type ResourceID string

// SnapshotID identifies an immutable collection snapshot.
type SnapshotID string

// AnalysisRunID identifies a single analysis execution against a snapshot.
type AnalysisRunID string

// FindingID identifies a finding within an analysis run.
type FindingID string

// Provider names a cloud vendor in the canonical model.
type Provider string

const (
	ProviderAWS          Provider = "aws"
	ProviderGCP          Provider = "gcp"
	ProviderAzure        Provider = "azure"
	ProviderDigitalOcean Provider = "digitalocean"
	ProviderFixture      Provider = "fixture"
	ProviderMulti        Provider = "multi"
)
