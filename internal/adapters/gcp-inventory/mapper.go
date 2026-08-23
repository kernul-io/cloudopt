package gcpinventory

import (
	"fmt"
	"strconv"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func mapScopedInventory(
	inv ScopedInventory,
	projectID, region, zone string,
	accountID types.AccountID,
	regID types.RegionID,
	obs types.Timestamp,
	index map[string]types.ResourceID,
) ([]domain.Resource, []domain.Relationship) {
	var resources []domain.Resource
	var rels []domain.Relationship

	scope := "regional"
	if zone != "" {
		scope = "zonal"
	} else if region == "global" {
		scope = "global"
	}

	baseAttrs := func(selfLink string) map[string]string {
		m := map[string]string{
			"gcp_self_link": selfLink,
			"gcp_project":   projectID,
			"gcp_scope":     scope,
		}
		if zone != "" {
			m["gcp_zone"] = zone
		}
		return m
	}

	for _, net := range inv.Networks {
		pid := net.SelfLink
		if pid == "" {
			pid = fmt.Sprintf("projects/%s/global/networks/%s", projectID, net.Name)
		}
		attrs := baseAttrs(pid)
		attrs["hierarchy"] = net.ProjectID
		resources = append(resources, domain.Resource{
			ID:                 registerID(index, pid),
			Kind:               domain.KindVPC,
			ProviderResourceID: pid,
			AccountID:          accountID,
			RegionID:           regID,
			Name:               net.Name,
			State:              "READY",
			Attributes:         attrs,
			Provenance:         domain.CollectProvenance(collectorSource, obs),
		})
	}

	for _, sn := range inv.Subnets {
		pid := sn.SelfLink
		if pid == "" {
			pid = fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", projectID, sn.Region, sn.Name)
		}
		id := registerID(index, pid)
		attrs := baseAttrs(pid)
		attrs["cidr"] = sn.CIDR
		resources = append(resources, domain.Resource{
			ID:                 id,
			Kind:               domain.KindSubnet,
			ProviderResourceID: pid,
			AccountID:          accountID,
			RegionID:           regID,
			Name:               sn.Name,
			State:              "READY",
			Attributes:         attrs,
			Provenance:         domain.CollectProvenance(collectorSource, obs),
		})
		if sn.Network != "" {
			toID, ok := lookupID(index, sn.Network)
			rels = append(rels, domain.Relationship{
				Kind:                 domain.RelInVPC,
				FromResourceID:       id,
				ToResourceID:         toID,
				ToProviderResourceID: sn.Network,
				TargetMissing:        !ok,
				Provenance:           domain.CollectProvenance(collectorSource, obs),
			})
		}
	}

	for _, rt := range inv.Routes {
		pid := rt.SelfLink
		if pid == "" {
			pid = fmt.Sprintf("projects/%s/global/routes/%s", projectID, rt.Name)
		}
		attrs := baseAttrs(pid)
		attrs["dest_range"] = rt.DestRange
		attrs["next_hop"] = rt.NextHop
		resources = append(resources, domain.Resource{
			ID:                 registerID(index, pid),
			Kind:               domain.KindRouteTable,
			ProviderResourceID: pid,
			AccountID:          accountID,
			RegionID:           regID,
			Name:               rt.Name,
			State:              "active",
			Attributes:         attrs,
			Provenance:         domain.CollectProvenance(collectorSource, obs),
		})
	}

	for _, nat := range inv.CloudNAT {
		pid := nat.SelfLink
		if pid == "" {
			pid = fmt.Sprintf("projects/%s/regions/%s/routers/%s/%s", projectID, nat.Region, nat.Router, nat.Name)
		}
		id := registerID(index, pid)
		attrs := baseAttrs(pid)
		attrs["router"] = nat.Router
		resources = append(resources, domain.Resource{
			ID:                 id,
			Kind:               domain.KindNATGateway,
			ProviderResourceID: pid,
			AccountID:          accountID,
			RegionID:           regID,
			Name:               nat.Name,
			State:              "READY",
			Attributes:         attrs,
			Provenance:         domain.CollectProvenance(collectorSource, obs),
		})
		if nat.Network != "" {
			toID, ok := lookupID(index, nat.Network)
			rels = append(rels, domain.Relationship{
				Kind:                 domain.RelInVPC,
				FromResourceID:       id,
				ToResourceID:         toID,
				ToProviderResourceID: nat.Network,
				TargetMissing:        !ok,
				Provenance:           domain.CollectProvenance(collectorSource, obs),
			})
		}
	}

	for _, addr := range inv.Addresses {
		pid := addr.SelfLink
		if pid == "" {
			pid = fmt.Sprintf("projects/%s/regions/%s/addresses/%s", projectID, addr.Region, addr.Name)
		}
		attrs := baseAttrs(pid)
		attrs["address"] = addr.Address
		resources = append(resources, domain.Resource{
			ID:                 registerID(index, pid),
			Kind:               domain.KindElasticIP,
			ProviderResourceID: pid,
			AccountID:          accountID,
			RegionID:           regID,
			Name:               addr.Name,
			State:              addr.Status,
			Attributes:         attrs,
			Provenance:         domain.CollectProvenance(collectorSource, obs),
		})
	}

	for _, fr := range inv.ForwardingRules {
		pid := fr.SelfLink
		if pid == "" {
			pid = fmt.Sprintf("projects/%s/regions/%s/forwardingRules/%s", projectID, fr.Region, fr.Name)
		}
		attrs := baseAttrs(pid)
		attrs["target"] = fr.Target
		attrs["load_balancing_scheme"] = fr.LoadBalancingScheme
		resources = append(resources, domain.Resource{
			ID:                 registerID(index, pid),
			Kind:               domain.KindForwardingRule,
			ProviderResourceID: pid,
			AccountID:          accountID,
			RegionID:           regID,
			Name:               fr.Name,
			State:              "ACTIVE",
			Attributes:         attrs,
			Provenance:         domain.CollectProvenance(collectorSource, obs),
		})
	}

	for _, img := range inv.Images {
		pid := img.SelfLink
		if pid == "" {
			pid = fmt.Sprintf("projects/%s/global/images/%s", projectID, img.Name)
		}
		attrs := baseAttrs(pid)
		attrs["family"] = img.Family
		resources = append(resources, domain.Resource{
			ID:                 registerID(index, pid),
			Kind:               domain.KindMachineImage,
			ProviderResourceID: pid,
			AccountID:          accountID,
			RegionID:           regID,
			Name:               img.Name,
			State:              img.Status,
			Tags:               labelsToTags(img.Labels),
			Attributes:         attrs,
			Provenance:         domain.CollectProvenance(collectorSource, obs),
		})
	}

	for _, mt := range inv.MachineTypes {
		pid := mt.SelfLink
		if pid == "" {
			pid = fmt.Sprintf("projects/%s/zones/%s/machineTypes/%s", projectID, mt.Zone, mt.Name)
		}
		attrs := baseAttrs(pid)
		attrs["vcpu"] = strconv.FormatInt(mt.VCPUs, 10)
		attrs["memory_mb"] = strconv.FormatInt(mt.MemoryMB, 10)
		attrs["family"] = mt.Family
		resources = append(resources, domain.Resource{
			ID:                 registerID(index, pid),
			Kind:               domain.KindInstanceType,
			ProviderResourceID: pid,
			AccountID:          accountID,
			RegionID:           regID,
			Name:               mt.Name,
			State:              "available",
			Attributes:         attrs,
			Provenance:         domain.CollectProvenance(collectorSource, obs),
		})
	}

	for _, inst := range inv.Instances {
		pid := inst.SelfLink
		if pid == "" {
			pid = fmt.Sprintf("projects/%s/zones/%s/instances/%s", projectID, inst.Zone, inst.Name)
		}
		id := registerID(index, pid)
		attrs := baseAttrs(pid)
		attrs["machine_type"] = inst.MachineType
		if inst.Zone != "" {
			attrs["gcp_zone"] = inst.Zone
		}
		resources = append(resources, domain.Resource{
			ID:                 id,
			Kind:               domain.KindComputeInstance,
			ProviderResourceID: pid,
			AccountID:          accountID,
			RegionID:           regID,
			Name:               inst.Name,
			State:              inst.Status,
			Tags:               labelsToTags(inst.Labels),
			Attributes:         attrs,
			Provenance:         domain.CollectProvenance(collectorSource, obs),
		})
		if inst.Subnetwork != "" {
			toID, ok := lookupID(index, inst.Subnetwork)
			rels = append(rels, domain.Relationship{
				Kind:                 domain.RelInSubnet,
				FromResourceID:       id,
				ToResourceID:         toID,
				ToProviderResourceID: inst.Subnetwork,
				TargetMissing:        !ok,
				Provenance:           domain.CollectProvenance(collectorSource, obs),
			})
		}
		for _, d := range inst.Disks {
			if d.Source == "" {
				continue
			}
			toID, ok := lookupID(index, d.Source)
			rels = append(rels, domain.Relationship{
				Kind:                 domain.RelAttachedTo,
				FromResourceID:       id,
				ToResourceID:         toID,
				ToProviderResourceID: d.Source,
				TargetMissing:        !ok,
				Provenance:           domain.CollectProvenance(collectorSource, obs),
			})
		}
	}

	for _, vol := range inv.Disks {
		pid := vol.SelfLink
		if pid == "" {
			pid = fmt.Sprintf("projects/%s/zones/%s/disks/%s", projectID, vol.Zone, vol.Name)
		}
		id := registerID(index, pid)
		attrs := baseAttrs(pid)
		attrs["size_gb"] = strconv.FormatInt(vol.SizeGB, 10)
		attrs["disk_type"] = vol.Type
		if len(vol.Users) == 0 {
			attrs["attached"] = "false"
		} else {
			attrs["attached"] = "true"
		}
		resources = append(resources, domain.Resource{
			ID:                 id,
			Kind:               domain.KindBlockVolume,
			ProviderResourceID: pid,
			AccountID:          accountID,
			RegionID:           regID,
			Name:               vol.Name,
			State:              vol.Status,
			Tags:               labelsToTags(vol.Labels),
			Attributes:         attrs,
			Provenance:         domain.CollectProvenance(collectorSource, obs),
		})
	}

	for _, snap := range inv.Snapshots {
		pid := snap.SelfLink
		if pid == "" {
			pid = fmt.Sprintf("projects/%s/global/snapshots/%s", projectID, snap.Name)
		}
		attrs := baseAttrs(pid)
		attrs["source_disk"] = snap.SourceDisk
		resources = append(resources, domain.Resource{
			ID:                 registerID(index, pid),
			Kind:               domain.KindSnapshot,
			ProviderResourceID: pid,
			AccountID:          accountID,
			RegionID:           regID,
			Name:               snap.Name,
			State:              snap.Status,
			Tags:               labelsToTags(snap.Labels),
			Attributes:         attrs,
			Provenance:         domain.CollectProvenance(collectorSource, obs),
		})
	}

	for _, db := range inv.SQLInstances {
		pid := db.SelfLink
		if pid == "" {
			pid = fmt.Sprintf("projects/%s/instances/%s", projectID, db.Name)
		}
		attrs := baseAttrs(pid)
		attrs["tier"] = db.Tier
		attrs["database_version"] = db.DatabaseVersion
		resources = append(resources, domain.Resource{
			ID:                 registerID(index, pid),
			Kind:               domain.KindDatabase,
			ProviderResourceID: pid,
			AccountID:          accountID,
			RegionID:           regID,
			Name:               db.Name,
			State:              db.State,
			Tags:               labelsToTags(db.Labels),
			Attributes:         attrs,
			Provenance:         domain.CollectProvenance(collectorSource, obs),
		})
	}

	for _, cl := range inv.GKEClusters {
		pid := cl.SelfLink
		if pid == "" {
			pid = fmt.Sprintf("projects/%s/locations/%s/clusters/%s", projectID, cl.Location, cl.Name)
		}
		clusterID := registerID(index, pid)
		attrs := baseAttrs(pid)
		attrs["location"] = cl.Location
		resources = append(resources, domain.Resource{
			ID:                 clusterID,
			Kind:               domain.KindKubernetesCluster,
			ProviderResourceID: pid,
			AccountID:          accountID,
			RegionID:           regID,
			Name:               cl.Name,
			State:              cl.Status,
			Tags:               labelsToTags(cl.Labels),
			Attributes:         attrs,
			Provenance:         domain.CollectProvenance(collectorSource, obs),
		})
		for _, np := range cl.NodePools {
			npLink := np.SelfLink
			if npLink == "" {
				npLink = pid + "/nodePools/" + np.Name
			}
			npID := registerID(index, npLink)
			npAttrs := baseAttrs(npLink)
			npAttrs["machine_type"] = np.MachineType
			npAttrs["node_count"] = strconv.FormatInt(np.NodeCount, 10)
			resources = append(resources, domain.Resource{
				ID:                 npID,
				Kind:               domain.KindKubernetesNodePool,
				ProviderResourceID: npLink,
				AccountID:          accountID,
				RegionID:           regID,
				Name:               np.Name,
				State:              np.Status,
				Attributes:         npAttrs,
				Provenance:         domain.CollectProvenance(collectorSource, obs),
			})
			rels = append(rels, domain.Relationship{
				Kind:                 domain.RelAssociatedWith,
				FromResourceID:       npID,
				ToResourceID:         clusterID,
				ToProviderResourceID: pid,
				Provenance:           domain.CollectProvenance(collectorSource, obs),
			})
		}
	}

	return resources, rels
}

func labelsToTags(labels map[string]string) []domain.Tag {
	if len(labels) == 0 {
		return nil
	}
	out := make([]domain.Tag, 0, len(labels))
	for k, v := range labels {
		out = append(out, domain.Tag{Key: k, Value: v})
	}
	return out
}
