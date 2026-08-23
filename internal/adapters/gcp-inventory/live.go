package gcpinventory

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/compute/metadata"
	"google.golang.org/api/cloudresourcemanager/v3"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/serviceusage/v1"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"

	"github.com/kernul-io/cloudopt/internal/application/ports"
)

// LiveOptions configures ADC and optional service-account impersonation.
type LiveOptions struct {
	ImpersonateServiceAccount string
}

// LiveBackend calls Google Cloud read-only APIs via official SDK clients.
type LiveBackend struct {
	opts LiveOptions
}

// NewLiveCollector builds a collector using Application Default Credentials.
func NewLiveCollector(ctx context.Context, opts LiveOptions) (ports.InventoryCollector, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return NewCollector(&LiveBackend{opts: opts}), nil
}

func (l *LiveBackend) clientOptions(ctx context.Context) []option.ClientOption {
	var opts []option.ClientOption
	if l.opts.ImpersonateServiceAccount != "" {
		//nolint:staticcheck // impersonate package wiring deferred; flag documented for sandbox tests
		opts = append(opts, option.ImpersonateCredentials(l.opts.ImpersonateServiceAccount))
	}
	_ = ctx
	return opts
}

func (l *LiveBackend) CallerIdentity(ctx context.Context) (CallerIdentity, error) {
	if err := ctx.Err(); err != nil {
		return CallerIdentity{}, err
	}
	email, _ := metadata.EmailWithContext(ctx, "")
	projectID, _ := metadata.ProjectIDWithContext(ctx)
	principal := email
	if principal == "" {
		principal = "adc"
	}
	return CallerIdentity{
		Email:     email,
		ProjectID: projectID,
		Principal: principal,
	}, nil
}

func (l *LiveBackend) ListProjects(ctx context.Context, organizationID, folderID string, explicit []string) ([]Project, error) {
	if len(explicit) > 0 {
		crm, err := cloudresourcemanager.NewService(ctx, l.clientOptions(ctx)...)
		if err != nil {
			return nil, err
		}
		var out []Project
		for _, id := range explicit {
			name := "projects/" + id
			p, err := crm.Projects.Get(name).Context(ctx).Do()
			if err != nil {
				return nil, mapCRMError("projects.get", id, err)
			}
			out = append(out, Project{
				ProjectID:   p.ProjectId,
				DisplayName: p.DisplayName,
				Parent:      p.Parent,
			})
		}
		return out, nil
	}
	crm, err := cloudresourcemanager.NewService(ctx, l.clientOptions(ctx)...)
	if err != nil {
		return nil, err
	}
	parent := ""
	switch {
	case folderID != "":
		parent = "folders/" + folderID
	case organizationID != "":
		parent = "organizations/" + organizationID
	default:
		return nil, fmt.Errorf("organization, folder, or explicit --projects required for live GCP discovery")
	}
	var out []Project
	req := crm.Projects.List().Parent(parent)
	err = req.Pages(ctx, func(page *cloudresourcemanager.ListProjectsResponse) error {
		for _, p := range page.Projects {
			if p.State != "ACTIVE" {
				continue
			}
			out = append(out, Project{
				ProjectID:   p.ProjectId,
				DisplayName: p.DisplayName,
				Parent:      p.Parent,
			})
		}
		return nil
	})
	if err != nil {
		return nil, mapCRMError("projects.list", parent, err)
	}
	return out, nil
}

func (l *LiveBackend) ListRegions(ctx context.Context, projectID string) ([]string, error) {
	compute, err := compute.NewService(ctx, l.clientOptions(ctx)...)
	if err != nil {
		return nil, err
	}
	resp, err := compute.Regions.List(projectID).Context(ctx).Do()
	if err != nil {
		return nil, mapComputeError("regions.list", projectID, err)
	}
	var names []string
	for _, r := range resp.Items {
		if r.Name != "" {
			names = append(names, r.Name)
		}
	}
	return names, nil
}

func (l *LiveBackend) ListEnabledServices(ctx context.Context, projectID string) ([]string, error) {
	svc, err := serviceusage.NewService(ctx, l.clientOptions(ctx)...)
	if err != nil {
		return nil, err
	}
	name := "projects/" + projectID
	var enabled []string
	err = svc.Services.List(name).Filter("state:ENABLED").Pages(ctx, func(page *serviceusage.ListServicesResponse) error {
		for _, s := range page.Services {
			if s.Config != nil && s.Config.Name != "" {
				enabled = append(enabled, s.Config.Name)
			}
		}
		return nil
	})
	if err != nil {
		return nil, mapGCPError("serviceusage", "services.list", projectID, "PERMISSION_DENIED", err.Error())
	}
	return enabled, nil
}

func (l *LiveBackend) CollectScoped(ctx context.Context, projectID, region, zone, pageToken string) (*ScopedPage, error) {
	compute, err := compute.NewService(ctx, l.clientOptions(ctx)...)
	if err != nil {
		return nil, err
	}
	var inv ScopedInventory
	if region == "global" {
		if err := l.collectGlobal(ctx, compute, projectID, &inv); err != nil {
			return nil, err
		}
		return &ScopedPage{Inventory: inv}, nil
	}
	if zone != "" {
		if err := l.collectZone(ctx, compute, projectID, zone, &inv); err != nil {
			return nil, err
		}
		return &ScopedPage{Inventory: inv}, nil
	}
	if err := l.collectRegion(ctx, compute, projectID, region, &inv); err != nil {
		return nil, err
	}
	_ = pageToken
	return &ScopedPage{Inventory: inv}, nil
}

func (l *LiveBackend) collectGlobal(ctx context.Context, svc *compute.Service, projectID string, inv *ScopedInventory) error {
	nets, err := svc.Networks.List(projectID).Context(ctx).Do()
	if err != nil {
		return mapComputeError("networks.list", projectID, err)
	}
	for _, n := range nets.Items {
		inv.Networks = append(inv.Networks, Network{
			SelfLink:  n.SelfLink,
			Name:      n.Name,
			ProjectID: projectID,
		})
	}
	imgs, err := svc.Images.List(projectID).Context(ctx).Do()
	if err != nil {
		return mapComputeError("images.list", projectID, err)
	}
	for _, img := range imgs.Items {
		inv.Images = append(inv.Images, Image{
			SelfLink:  img.SelfLink,
			Name:      img.Name,
			ProjectID: projectID,
			Family:    img.Family,
			Status:    img.Status,
		})
	}
	return nil
}

func (l *LiveBackend) collectRegion(ctx context.Context, svc *compute.Service, projectID, region string, inv *ScopedInventory) error {
	subnets, err := svc.Subnetworks.List(projectID, region).Context(ctx).Do()
	if err != nil {
		return mapComputeError("subnetworks.list", projectID+"/"+region, err)
	}
	for _, sn := range subnets.Items {
		inv.Subnets = append(inv.Subnets, Subnet{
			SelfLink:  sn.SelfLink,
			Name:      sn.Name,
			ProjectID: projectID,
			Region:    region,
			Network:   sn.Network,
			CIDR:      sn.IpCidrRange,
		})
	}
	sql, err := sqladmin.NewService(ctx, l.clientOptions(ctx)...)
	if err != nil {
		return err
	}
	dbList, err := sql.Instances.List(projectID).Context(ctx).Do()
	if err == nil {
		for _, db := range dbList.Items {
			if db.Region != "" && !regionMatches(db.Region, region) {
				continue
			}
			inv.SQLInstances = append(inv.SQLInstances, SQLInstance{
				SelfLink:        db.SelfLink,
				Name:            db.Name,
				ProjectID:       projectID,
				Region:          db.Region,
				DatabaseVersion: db.DatabaseVersion,
				State:           db.State,
				Tier:            tierName(db.Settings),
			})
		}
	}
	container, err := container.NewService(ctx, l.clientOptions(ctx)...)
	if err != nil {
		return err
	}
	parent := "projects/" + projectID + "/locations/" + region
	clusters, err := container.Projects.Locations.Clusters.List(parent).Context(ctx).Do()
	if err == nil {
		for _, cl := range clusters.Clusters {
			gke := GKECluster{
				SelfLink:   cl.SelfLink,
				Name:       cl.Name,
				ProjectID:  projectID,
				Location:   cl.Location,
				Status:     cl.Status,
				Network:    cl.Network,
				Subnetwork: cl.Subnetwork,
			}
			for _, np := range cl.NodePools {
				gke.NodePools = append(gke.NodePools, GKENodePool{
					SelfLink:    np.SelfLink,
					Name:        np.Name,
					MachineType: np.Config.MachineType,
					Status:      np.Status,
					NodeCount:   np.InitialNodeCount,
				})
			}
			inv.GKEClusters = append(inv.GKEClusters, gke)
		}
	}
	return nil
}

func (l *LiveBackend) collectZone(ctx context.Context, svc *compute.Service, projectID, zone string, inv *ScopedInventory) error {
	instList, err := svc.Instances.List(projectID, zone).Context(ctx).Do()
	if err != nil {
		return mapComputeError("instances.list", projectID+"/"+zone, err)
	}
	for _, inst := range instList.Items {
		gcpInst := Instance{
			SelfLink:    inst.SelfLink,
			Name:        inst.Name,
			ProjectID:   projectID,
			Zone:        zone,
			Region:      regionFromZone(zone),
			Status:      inst.Status,
			MachineType: lastPathSegment(inst.MachineType),
			Labels:      inst.Labels,
		}
		if len(inst.NetworkInterfaces) > 0 {
			gcpInst.Network = inst.NetworkInterfaces[0].Network
			gcpInst.Subnetwork = inst.NetworkInterfaces[0].Subnetwork
		}
		for _, d := range inst.Disks {
			gcpInst.Disks = append(gcpInst.Disks, InstanceDisk{
				DeviceName: d.DeviceName,
				Source:     d.Source,
				Boot:       d.Boot,
			})
		}
		inv.Instances = append(inv.Instances, gcpInst)
	}
	disks, err := svc.Disks.List(projectID, zone).Context(ctx).Do()
	if err != nil {
		return mapComputeError("disks.list", projectID+"/"+zone, err)
	}
	for _, d := range disks.Items {
		inv.Disks = append(inv.Disks, Disk{
			SelfLink:  d.SelfLink,
			Name:      d.Name,
			ProjectID: projectID,
			Zone:      zone,
			SizeGB:    d.SizeGb,
			Type:      d.Type,
			Status:    d.Status,
			Users:     d.Users,
			Labels:    d.Labels,
		})
	}
	return nil
}

func mapCRMError(op, scope string, err error) error {
	if err == nil {
		return nil
	}
	return mapGCPError("resourcemanager", op, scope, "PERMISSION_DENIED", err.Error())
}

func mapComputeError(op, scope string, err error) error {
	if err == nil {
		return nil
	}
	return mapGCPError("compute", op, scope, "PERMISSION_DENIED", err.Error())
}

func tierName(settings *sqladmin.Settings) string {
	if settings == nil {
		return ""
	}
	return settings.Tier
}

func lastPathSegment(url string) string {
	if url == "" {
		return ""
	}
	parts := strings.Split(url, "/")
	return parts[len(parts)-1]
}

func regionMatches(dbRegion, region string) bool {
	return strings.Contains(dbRegion, region)
}
