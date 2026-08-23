package gcpinventory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/kernul-io/cloudopt/internal/application/ports"
)

// FixtureBackend serves recorded inventory from testdata/gcp-inventory.
type FixtureBackend struct {
	Root  string
	mu    sync.Mutex
	pages map[string]int // scope key -> collect call count for pagination tests
}

type fixtureManifest struct {
	Identity struct {
		Email     string `json:"email"`
		UniqueID  string `json:"unique_id"`
		ProjectID string `json:"project_id"`
		Principal string `json:"principal"`
	} `json:"identity"`
	Projects           []Project           `json:"projects"`
	Regions            []string            `json:"regions"`
	EnabledAPIs        map[string][]string `json:"enabled_apis"`
	PartialProject     string              `json:"partial_project"`
	DisabledAPIProject string              `json:"disabled_api_project"`
}

type fixtureScopeFile struct {
	Inventory ScopedInventory  `json:"inventory"`
	NextToken string           `json:"next_token"`
	Page2     *ScopedInventory `json:"page2_inventory"`
}

// NewFixtureCollector returns offline GCP inventory collection.
func NewFixtureCollector(root string) (ports.InventoryCollector, error) {
	if root == "" {
		return nil, fmt.Errorf("fixture root is required for offline GCP collection")
	}
	return NewCollector(&FixtureBackend{Root: root, pages: map[string]int{}}), nil
}

func (f *FixtureBackend) loadManifest() (*fixtureManifest, error) {
	path := filepath.Join(f.Root, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m fixtureManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (f *FixtureBackend) CallerIdentity(ctx context.Context) (CallerIdentity, error) {
	if err := ctx.Err(); err != nil {
		return CallerIdentity{}, err
	}
	m, err := f.loadManifest()
	if err != nil {
		return CallerIdentity{}, err
	}
	return CallerIdentity{
		Email:     m.Identity.Email,
		UniqueID:  m.Identity.UniqueID,
		ProjectID: m.Identity.ProjectID,
		Principal: m.Identity.Principal,
	}, nil
}

func (f *FixtureBackend) ListProjects(_ context.Context, _, _ string, explicit []string) ([]Project, error) {
	m, err := f.loadManifest()
	if err != nil {
		return nil, err
	}
	if len(explicit) > 0 {
		set := map[string]struct{}{}
		for _, p := range explicit {
			set[p] = struct{}{}
		}
		var out []Project
		for _, p := range m.Projects {
			if _, ok := set[p.ProjectID]; ok {
				out = append(out, p)
			}
		}
		if len(out) == 0 {
			return nil, mapGCPError("resourcemanager", "projects.list", "global", "NOT_FOUND", "no matching projects")
		}
		return out, nil
	}
	return m.Projects, nil
}

func (f *FixtureBackend) ListRegions(_ context.Context, _ string) ([]string, error) {
	m, err := f.loadManifest()
	if err != nil {
		return nil, err
	}
	if len(m.Regions) == 0 {
		return []string{"us-central1"}, nil
	}
	return m.Regions, nil
}

func (f *FixtureBackend) ListEnabledServices(_ context.Context, projectID string) ([]string, error) {
	m, err := f.loadManifest()
	if err != nil {
		return nil, err
	}
	if m.DisabledAPIProject != "" && projectID == m.DisabledAPIProject {
		return []string{"serviceusage.googleapis.com"}, nil
	}
	if apis, ok := m.EnabledAPIs[projectID]; ok {
		return apis, nil
	}
	return []string{
		"compute.googleapis.com",
		"sqladmin.googleapis.com",
		"container.googleapis.com",
		"serviceusage.googleapis.com",
	}, nil
}

func (f *FixtureBackend) CollectScoped(_ context.Context, projectID, region, zone, pageToken string) (*ScopedPage, error) {
	m, err := f.loadManifest()
	if err != nil {
		return nil, err
	}
	if m.PartialProject != "" && projectID == m.PartialProject {
		return nil, mapGCPError("compute", "instances.list", projectID+"/"+region, "PERMISSION_DENIED", "permission denied on partial project fixture")
	}

	key := scopeKey(projectID, region, zone)
	f.mu.Lock()
	f.pages[key]++
	call := f.pages[key]
	f.mu.Unlock()

	path := filepath.Join(f.Root, "projects", projectID, scopePath(region, zone)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ScopedPage{Inventory: ScopedInventory{}}, nil
		}
		return nil, err
	}
	var file fixtureScopeFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}

	if file.NextToken != "" && call == 1 && pageToken == "" {
		return &ScopedPage{Inventory: file.Inventory, NextToken: file.NextToken}, nil
	}
	if file.Page2 != nil && pageToken == file.NextToken {
		return &ScopedPage{Inventory: *file.Page2}, nil
	}
	if pageToken != "" && file.Page2 == nil {
		return &ScopedPage{Inventory: ScopedInventory{}}, nil
	}
	return &ScopedPage{Inventory: file.Inventory}, nil
}

func scopeKey(projectID, region, zone string) string {
	return projectID + "|" + region + "|" + zone
}

func scopePath(region, zone string) string {
	if region == "global" {
		return "global"
	}
	if zone != "" {
		return filepath.Join("zones", zone)
	}
	return filepath.Join("regions", region)
}

// PaginateFixtureInstances is a test helper exercising fixture pagination.
func PaginateFixtureInstances(root, projectID, zone string) (int, error) {
	b := &FixtureBackend{Root: root, pages: map[string]int{}}
	region := regionFromZone(zone)
	total := 0
	token := ""
	for {
		page, err := b.CollectScoped(context.Background(), projectID, region, zone, token)
		if err != nil {
			return total, err
		}
		total += len(page.Inventory.Instances)
		if page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	return total, nil
}

// FilterProjectsForTest returns sorted project IDs from manifest.
func FilterProjectsForTest(root string, allow []string) ([]string, error) {
	b := &FixtureBackend{Root: root}
	projects, err := b.ListProjects(context.Background(), "", "", allow)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(projects))
	for _, p := range projects {
		out = append(out, p.ProjectID)
	}
	sort.Strings(out)
	return out, nil
}

// SimulateQuotaError returns a retryable quota error for tests.
func SimulateQuotaError() error {
	return &APIError{
		Service:   "compute",
		Operation: "instances.list",
		Scope:     "demo",
		Code:      "RESOURCE_EXHAUSTED",
		Message:   "quota exceeded",
		Retryable: true,
	}
}

// RedactForTest exposes redact for tests.
func RedactForTest(msg string) string { return redactMessage(msg) }

// ScopeLabel builds coverage region label.
func ScopeLabel(projectID, region, zone string) string {
	if zone != "" {
		return projectID + "/" + zone
	}
	if region == "global" {
		return projectID
	}
	return projectID + "/" + region
}

// NormalizeSelfLink trims GCP self links for index keys in fixtures.
func NormalizeSelfLink(link string) string {
	return strings.TrimPrefix(link, "https://www.googleapis.com/compute/v1/")
}
