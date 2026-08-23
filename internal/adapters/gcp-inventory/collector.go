package gcpinventory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// Collector implements read-only GCP inventory collection.
type Collector struct {
	Backend        InventoryBackend
	CapabilitiesFn func() (ports.CapabilityManifest, error)
	Retry          RetryConfig
}

func NewCollector(backend InventoryBackend) *Collector {
	return &Collector{
		Backend: backend,
		CapabilitiesFn: func() (ports.CapabilityManifest, error) {
			return LoadCapabilities()
		},
		Retry: defaultRetryConfig(),
	}
}

func (c *Collector) Capabilities() ports.CapabilityManifest {
	m, err := c.CapabilitiesFn()
	if err != nil {
		return ports.CapabilityManifest{Provider: types.ProviderGCP}
	}
	return m
}

func (c *Collector) Preflight(ctx context.Context, opts ports.CollectOptions) (*ports.InventoryPreflight, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	caps, err := c.CapabilitiesFn()
	if err != nil {
		return nil, err
	}
	identity, err := c.Backend.CallerIdentity(ctx)
	if err != nil {
		return nil, err
	}
	projects, err := c.Backend.ListProjects(ctx, opts.OrganizationID, opts.FolderID, opts.Projects)
	if err != nil {
		return nil, err
	}
	projectIDs := make([]string, 0, len(projects))
	for _, p := range projects {
		projectIDs = append(projectIDs, p.ProjectID)
	}
	sort.Strings(projectIDs)

	regions, err := c.resolveRegions(ctx, opts, projectIDs)
	if err != nil {
		return nil, err
	}

	enabled := map[string][]string{}
	var missing []string
	for _, pid := range projectIDs {
		services, err := c.Backend.ListEnabledServices(ctx, pid)
		if err != nil {
			if errorsIsAccessDenied(err) {
				missing = append(missing, "serviceusage.services.list:"+pid)
				continue
			}
			return nil, err
		}
		enabled[pid] = services
		if !containsService(services, "compute.googleapis.com") {
			missing = append(missing, "compute.googleapis.com disabled:"+pid)
		}
	}

	scope := collectionScope(opts, projectIDs)
	pf := &ports.InventoryPreflight{
		ProviderAccountID:  identity.ProjectID,
		CallerARN:          identity.Principal,
		CallerEmail:        identity.Email,
		SelectedRegions:    regions,
		AccessibleProjects: projectIDs,
		SelectedProjects:   projectIDs,
		ReachableServices:  []string{"compute", "sqladmin", "container", "serviceusage"},
		MissingActions:     missing,
		Capabilities:       caps,
		EnabledAPIs:        enabled,
		CollectionScope:    scope,
	}
	return pf, nil
}

func (c *Collector) Collect(ctx context.Context, opts ports.CollectOptions, progress ports.ProgressReporter) (*domain.CollectionSnapshot, error) {
	if progress == nil {
		progress = ports.NopProgress{}
	}
	pf, err := c.Preflight(ctx, opts)
	if err != nil {
		return nil, err
	}
	if !opts.Offline && len(pf.MissingActions) > 0 {
		return nil, ports.ErrMissingPermissions(pf.MissingActions)
	}
	if opts.DryRun {
		return nil, nil
	}

	started := types.NowUTC()
	accountID := opts.AccountID
	if accountID == "" {
		accountID = types.AccountID("acct-gcp-" + pf.ProviderAccountID)
	}

	projects := pf.SelectedProjects
	regions := pf.SelectedRegions
	zones := opts.Zones

	var (
		mu         sync.Mutex
		regionsOut []domain.Region
		resources  []domain.Resource
		rels       []domain.Relationship
		coverage   []domain.ServiceCollectionStatus
		partial    bool
		idIndex    = map[string]types.ResourceID{}
		regionSeen = map[string]struct{}{}
	)

	maxWorkers := opts.MaxConcurrent
	if maxWorkers <= 0 {
		maxWorkers = 3
	}
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for _, projectID := range projects {
		for _, region := range regions {
			wg.Add(1)
			sem <- struct{}{}
			go func(projectID, region string) {
				defer wg.Done()
				defer func() { <-sem }()
				if err := ctx.Err(); err != nil {
					return
				}
				progress.Step(fmt.Sprintf("project %s region %s", projectID, region))
				regKey := region
				regID := types.RegionID("reg-gcp-" + regKey)

				mu.Lock()
				res, r, cov, regPartial, err := c.collectProjectRegion(ctx, projectID, region, "", accountID, regID, started, idIndex)
				if _, ok := regionSeen[regKey]; !ok {
					regionSeen[regKey] = struct{}{}
					regionsOut = append(regionsOut, domain.Region{
						ID:               regID,
						ProviderRegionID: regKey,
						DisplayName:      regKey,
						Provenance:       domain.CollectProvenance(collectorSource, started),
					})
				}
				resources = append(resources, res...)
				rels = append(rels, r...)
				coverage = append(coverage, cov...)
				if regPartial {
					partial = true
				}
				if err != nil {
					coverage = append(coverage, domain.ServiceCollectionStatus{
						Service: "project/region",
						Region:  projectID + "/" + region,
						Status:  domain.ServiceCollectionFailed,
						Message: redactMessage(err.Error()),
					})
					partial = true
				}
				mu.Unlock()
			}(projectID, region)
		}
		// Global + zonal passes per project
		wg.Add(1)
		sem <- struct{}{}
		go func(projectID string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				return
			}
			progress.Step("project " + projectID + " global")
			globalRegID := types.RegionID("reg-gcp-global")
			mu.Lock()
			if _, ok := regionSeen["global"]; !ok {
				regionSeen["global"] = struct{}{}
				regionsOut = append(regionsOut, domain.Region{
					ID:               globalRegID,
					ProviderRegionID: "global",
					DisplayName:      "global",
					Provenance:       domain.CollectProvenance(collectorSource, started),
				})
			}

			res, r, cov, regPartial, err := c.collectProjectRegion(ctx, projectID, "global", "", accountID, globalRegID, started, idIndex)
			resources = append(resources, res...)
			rels = append(rels, r...)
			coverage = append(coverage, cov...)
			if regPartial {
				partial = true
			}
			if err != nil {
				coverage = append(coverage, domain.ServiceCollectionStatus{
					Service: "project/global",
					Region:  projectID,
					Status:  domain.ServiceCollectionFailed,
					Message: redactMessage(err.Error()),
				})
				partial = true
			}
			mu.Unlock()
		}(projectID)

		zoneList := zones
		if len(zoneList) == 0 {
			zoneList = defaultZonesForRegions(regions)
		}
		for _, zone := range zoneList {
			wg.Add(1)
			sem <- struct{}{}
			go func(projectID, zone string) {
				defer wg.Done()
				defer func() { <-sem }()
				if err := ctx.Err(); err != nil {
					return
				}
				region := regionFromZone(zone)
				progress.Step(fmt.Sprintf("project %s zone %s", projectID, zone))
				regID := types.RegionID("reg-gcp-" + region)
				mu.Lock()
				if _, ok := regionSeen[region]; !ok {
					regionSeen[region] = struct{}{}
					regionsOut = append(regionsOut, domain.Region{
						ID:               regID,
						ProviderRegionID: region,
						DisplayName:      region,
						Provenance:       domain.CollectProvenance(collectorSource, started),
					})
				}

				res, r, cov, regPartial, err := c.collectProjectRegion(ctx, projectID, region, zone, accountID, regID, started, idIndex)
				resources = append(resources, res...)
				rels = append(rels, r...)
				coverage = append(coverage, cov...)
				if regPartial {
					partial = true
				}
				if err != nil {
					coverage = append(coverage, domain.ServiceCollectionStatus{
						Service: "project/zone",
						Region:  projectID + "/" + zone,
						Status:  domain.ServiceCollectionFailed,
						Message: redactMessage(err.Error()),
					})
					partial = true
				}
				mu.Unlock()
			}(projectID, zone)
		}
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return nil, ErrCancelled
	}

	sort.Slice(regionsOut, func(i, j int) bool {
		return regionsOut[i].ProviderRegionID < regionsOut[j].ProviderRegionID
	})

	resources = dedupeResources(resources)
	rels = dedupeRelationships(rels)

	status := domain.SnapshotComplete
	if partial {
		status = domain.SnapshotPartial
	}
	completed := types.NowUTC()
	snapID, err := newSnapshotID()
	if err != nil {
		return nil, err
	}

	display := "GCP " + pf.ProviderAccountID
	if len(projects) == 1 {
		display = "GCP project " + projects[0]
	}

	snap := &domain.CollectionSnapshot{
		ID:            snapID,
		AccountID:     accountID,
		Provider:      types.ProviderGCP,
		Status:        status,
		SchemaVersion: 1,
		StartedAt:     started,
		CompletedAt:   &completed,
		Account: domain.Account{
			ID:                accountID,
			Provider:          types.ProviderGCP,
			ProviderAccountID: pf.ProviderAccountID,
			DisplayName:       display,
			DefaultCurrency:   "USD",
			Provenance:        domain.CollectProvenance(collectorSource, completed),
		},
		Regions:       regionsOut,
		Resources:     resources,
		Relationships: rels,
		Coverage:      domain.CollectionCoverage{Services: coverage},
	}
	_ = pf
	return snap, nil
}

func (c *Collector) collectProjectRegion(
	ctx context.Context,
	projectID, region, zone string,
	accountID types.AccountID,
	regID types.RegionID,
	obs types.Timestamp,
	idIndex map[string]types.ResourceID,
) ([]domain.Resource, []domain.Relationship, []domain.ServiceCollectionStatus, bool, error) {
	var all ScopedInventory
	var coverage []domain.ServiceCollectionStatus
	partial := false
	scopeLabel := projectID + "/" + region
	if zone != "" {
		scopeLabel = projectID + "/" + zone
	}

	token := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, coverage, true, ErrCancelled
		}
		var page *ScopedPage
		err := withRetry(ctx, c.Retry, func() error {
			p, err := c.Backend.CollectScoped(ctx, projectID, region, zone, token)
			if err != nil {
				return err
			}
			page = p
			return nil
		}, retryableAPIErr)
		if err != nil {
			st := domain.ServiceCollectionFailed
			if errorsIsAccessDenied(err) {
				st = domain.ServiceCollectionPartial
			} else if errorsIsAPIDisabled(err) {
				st = domain.ServiceCollectionSkipped
			}
			coverage = append(coverage, domain.ServiceCollectionStatus{
				Service: "inventory",
				Region:  scopeLabel,
				Status:  st,
				Message: redactMessage(err.Error()),
			})
			return nil, nil, coverage, true, err
		}
		mergeScoped(&all, page.Inventory)
		if page.NextToken == "" {
			break
		}
		token = page.NextToken
	}

	res, rels := mapScopedInventory(all, projectID, region, zone, accountID, regID, obs, idIndex)
	coverage = append(coverage, domain.ServiceCollectionStatus{
		Service: "inventory",
		Region:  scopeLabel,
		Status:  domain.ServiceCollectionOK,
	})
	return res, rels, coverage, partial, nil
}

func mergeScoped(dst *ScopedInventory, src ScopedInventory) {
	dst.Networks = append(dst.Networks, src.Networks...)
	dst.Instances = append(dst.Instances, src.Instances...)
	dst.Disks = append(dst.Disks, src.Disks...)
	dst.Snapshots = append(dst.Snapshots, src.Snapshots...)
	dst.Images = append(dst.Images, src.Images...)
	dst.MachineTypes = append(dst.MachineTypes, src.MachineTypes...)
	dst.Subnets = append(dst.Subnets, src.Subnets...)
	dst.Routes = append(dst.Routes, src.Routes...)
	dst.Addresses = append(dst.Addresses, src.Addresses...)
	dst.CloudNAT = append(dst.CloudNAT, src.CloudNAT...)
	dst.ForwardingRules = append(dst.ForwardingRules, src.ForwardingRules...)
	dst.SQLInstances = append(dst.SQLInstances, src.SQLInstances...)
	dst.GKEClusters = append(dst.GKEClusters, src.GKEClusters...)
}

func (c *Collector) resolveRegions(ctx context.Context, opts ports.CollectOptions, projects []string) ([]string, error) {
	if len(opts.Regions) > 0 {
		return filterRegions(opts.Regions, opts.RegionsAllow, opts.RegionsDeny), nil
	}
	if len(projects) == 0 {
		return []string{"us-central1"}, nil
	}
	var names []string
	err := withRetry(ctx, c.Retry, func() error {
		regs, err := c.Backend.ListRegions(ctx, projects[0])
		if err != nil {
			return err
		}
		names = regs
		return nil
	}, retryableAPIErr)
	if err != nil {
		return nil, err
	}
	return filterRegions(names, opts.RegionsAllow, opts.RegionsDeny), nil
}

func collectionScope(opts ports.CollectOptions, projects []string) string {
	var parts []string
	if opts.OrganizationID != "" {
		parts = append(parts, "org="+opts.OrganizationID)
	}
	if opts.FolderID != "" {
		parts = append(parts, "folder="+opts.FolderID)
	}
	if len(projects) > 0 {
		parts = append(parts, "projects="+strings.Join(projects, ","))
	}
	if len(opts.Regions) > 0 {
		parts = append(parts, "regions="+strings.Join(opts.Regions, ","))
	}
	return strings.Join(parts, "; ")
}

func containsService(services []string, name string) bool {
	for _, s := range services {
		if s == name {
			return true
		}
	}
	return false
}

func newSnapshotID() (types.SnapshotID, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("snapshot id: %w", err)
	}
	return types.SnapshotID("snap-" + hex.EncodeToString(b[:])), nil
}

func registerID(index map[string]types.ResourceID, providerID string) types.ResourceID {
	id := canonicalID(providerID)
	index[providerID] = id
	return id
}

func lookupID(index map[string]types.ResourceID, providerID string) (types.ResourceID, bool) {
	id, ok := index[providerID]
	return id, ok
}

func canonicalID(providerID string) types.ResourceID {
	safe := strings.NewReplacer("/", "-", ":", "-").Replace(providerID)
	return types.ResourceID("res-" + safe)
}

func regionFromZone(zone string) string {
	if zone == "" {
		return ""
	}
	parts := strings.Split(zone, "-")
	if len(parts) < 2 {
		return zone
	}
	return strings.Join(parts[:len(parts)-1], "-")
}

func defaultZonesForRegions(regions []string) []string {
	var zones []string
	for _, r := range regions {
		zones = append(zones, r+"-a", r+"-b")
	}
	return zones
}

func filterRegions(regions, allow, deny []string) []string {
	allowSet := map[string]struct{}{}
	for _, r := range allow {
		allowSet[r] = struct{}{}
	}
	denySet := map[string]struct{}{}
	for _, r := range deny {
		denySet[r] = struct{}{}
	}
	var out []string
	for _, r := range regions {
		if len(allowSet) > 0 {
			if _, ok := allowSet[r]; !ok {
				continue
			}
		}
		if _, ok := denySet[r]; ok {
			continue
		}
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

func dedupeResources(in []domain.Resource) []domain.Resource {
	seen := map[types.ResourceID]struct{}{}
	out := make([]domain.Resource, 0, len(in))
	for _, r := range in {
		if _, ok := seen[r.ID]; ok {
			continue
		}
		seen[r.ID] = struct{}{}
		out = append(out, r)
	}
	return out
}

func dedupeRelationships(in []domain.Relationship) []domain.Relationship {
	type key struct {
		kind domain.RelationshipKind
		from types.ResourceID
		to   types.ResourceID
	}
	seen := map[key]struct{}{}
	out := make([]domain.Relationship, 0, len(in))
	for _, r := range in {
		k := key{kind: r.Kind, from: r.FromResourceID, to: r.ToResourceID}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, r)
	}
	return out
}
