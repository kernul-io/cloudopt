package ports

import (
	"context"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
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
}

// CapabilityManifest describes supported inventory signals for a provider adapter.
type CapabilityManifest struct {
	Provider        types.Provider
	Schema          string
	Inventory       []CapabilityEntry
	Billing         []CapabilityEntry
	Metrics         []CapabilityEntry
	Pricing         []CapabilityEntry
	SupportedChecks []string
}

// CapabilityEntry is one named capability with availability metadata.
type CapabilityEntry struct {
	ID          string
	Description string
	Available   bool
	APIActions  []string
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
