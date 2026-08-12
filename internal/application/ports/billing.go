package ports

import (
	"context"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
)

// CostCollectOptions configures AWS (or other) billing collection.
type CostCollectOptions struct {
	Provider     types.Provider
	AccountID    types.AccountID
	RoleARN      string
	ExternalID   string
	SnapshotID   types.SnapshotID
	LookbackDays int
	DryRun       bool
	Offline      bool
	FixtureRoot  string
	Progress     ProgressReporter `json:"-"`
}

// BillingPreflight summarizes billing API access before collection.
type BillingPreflight struct {
	ProviderAccountID string
	CallerARN         string
	LookbackDays      int
	MissingActions    []string
	Capabilities      CapabilityManifest
}

// BillingCollectResult is normalized billing output before persistence.
type BillingCollectResult struct {
	Costs        []domain.CostRecord
	SourceTotals map[string]types.Money
	Interval     domain.BillingInterval
	Coverage     []domain.ServiceCollectionStatus
	Diagnostics  []domain.CostDiagnostic
	Partial      bool
}

// BillingSource collects provider billing into canonical cost records.
// A Cost and Usage Report (CUR) importer can implement this interface later.
type BillingSource interface {
	Capabilities() CapabilityManifest
	Preflight(ctx context.Context, opts CostCollectOptions) (*BillingPreflight, error)
	Collect(ctx context.Context, opts CostCollectOptions, inventory *domain.CollectionSnapshot) (*BillingCollectResult, error)
}

// CostCollectResult is returned after billing collection and snapshot merge.
type CostCollectResult struct {
	SnapshotID types.SnapshotID `json:"snapshot_id,omitempty"`
	DryRun     bool             `json:"dry_run,omitempty"`
	Partial    bool             `json:"partial,omitempty"`
	Preflight  *BillingPreflight
	Reconcile  *CostReconciliationSummary `json:"reconciliation,omitempty"`
}

// CostReconciliationSummary is a high-level totals check after collection.
type CostReconciliationSummary struct {
	SourceTotal          map[string]int64 `json:"source_total_minor"`
	AttributedTotal      map[string]int64 `json:"attributed_total_minor"`
	UnattributedTotal    map[string]int64 `json:"unattributed_total_minor"`
	DiscrepancyMinor     map[string]int64 `json:"discrepancy_minor"`
	WithinTolerance      bool             `json:"within_tolerance"`
	ToleranceBasisPoints int64            `json:"tolerance_basis_points"`
}
