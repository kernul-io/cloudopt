package gcpinventory

import "context"

// InventoryBackend lists projects and returns scoped inventory (mockable for tests).
type InventoryBackend interface {
	CallerIdentity(ctx context.Context) (CallerIdentity, error)
	ListProjects(ctx context.Context, organizationID, folderID string, explicit []string) ([]Project, error)
	ListRegions(ctx context.Context, projectID string) ([]string, error)
	ListEnabledServices(ctx context.Context, projectID string) ([]string, error)
	CollectScoped(ctx context.Context, projectID, region, zone string, pageToken string) (*ScopedPage, error)
}

// ScopedPage is one page of regional/zonal/global inventory for a project scope.
type ScopedPage struct {
	Inventory  ScopedInventory
	NextToken  string
	APIEnabled map[string]bool // service -> enabled for this project
}
