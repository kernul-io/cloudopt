package ports

import (
	"context"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// CollectOptions configures inventory collection from the CLI.
type CollectOptions struct {
	Provider      types.Provider
	AccountID     types.AccountID
	RoleARN       string
	ExternalID    string
	Regions       []string
	RegionsAllow  []string
	RegionsDeny   []string
	DryRun        bool
	Offline       bool
	FixtureRoot   string
	OutputJSON    bool
	MaxConcurrent int
	Progress      ProgressReporter `json:"-"`

	// SnapshotID optionally preallocates/resumes an in-progress collection shell.
	SnapshotID types.SnapshotID
	Resume     bool

	// GCP scope (Application Default Credentials; no secrets stored).
	OrganizationID            string
	FolderID                  string
	Projects                  []string
	Zones                     []string
	ImpersonateServiceAccount string
	BillingAccountID          string
}

// CollectResult is returned after a successful collection or preflight.
type CollectResult struct {
	SnapshotID types.SnapshotID `json:"snapshot_id,omitempty"`
	Preflight  *InventoryPreflight
	DryRun     bool `json:"dry_run,omitempty"`
	Partial    bool `json:"partial,omitempty"`
}

// InventoryPreflight summarizes access and intended scope before collection.
type InventoryPreflight struct {
	ProviderAccountID string
	CallerARN         string
	SelectedRegions   []string
	ReachableServices []string
	MissingActions    []string
	Capabilities      CapabilityManifest

	// GCP preflight fields (empty for other providers).
	CallerEmail        string
	AccessibleProjects []string
	SelectedProjects   []string
	EnabledAPIs        map[string][]string // project ID -> enabled service names
	CollectionScope    string
}

// CapabilityManifest describes supported inventory signals for a provider adapter.
type CapabilityManifest struct {
	Provider         types.Provider
	Schema           string
	Advertised       bool
	Inventory        []CapabilityEntry
	Billing          []CapabilityEntry
	Metrics          []CapabilityEntry
	Pricing          []CapabilityEntry
	SupportedChecks  []string
	CommitmentModels []string
	KnownLimitations []string
}

// CapabilityAvailability distinguishes why a capability is not usable.
type CapabilityAvailability string

const (
	CapabilitySupported        CapabilityAvailability = "supported"
	CapabilityUnsupported      CapabilityAvailability = "unsupported"
	CapabilityPermissionDenied CapabilityAvailability = "permission_denied"
	CapabilityCollectionFailed CapabilityAvailability = "collection_failed"
	CapabilityDataAbsent       CapabilityAvailability = "data_absent"
)

// CapabilityEntry is one named capability with availability metadata.
type CapabilityEntry struct {
	ID           string
	Description  string
	Available    bool
	Availability CapabilityAvailability
	APIActions   []string
}

// InventoryCollector performs read-only inventory collection for one provider.
type InventoryCollector interface {
	Capabilities() CapabilityManifest
	Preflight(ctx context.Context, opts CollectOptions) (*InventoryPreflight, error)
	Collect(ctx context.Context, opts CollectOptions, progress ProgressReporter) (*domain.CollectionSnapshot, error)
}

// ProgressReporter receives human-oriented progress (stderr only in CLI).
type ProgressReporter interface {
	Step(message string)
}

// NopProgress discards progress events.
type NopProgress struct{}

func (NopProgress) Step(string) {}
