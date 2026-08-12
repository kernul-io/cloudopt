package awsinventory

import (
	"context"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
)

func (c *Collector) collectVPCs(ctx context.Context, client EC2API, region string, accountID types.AccountID, regID types.RegionID, obs types.Timestamp, index map[string]types.ResourceID) ([]domain.Resource, error) {
	var resources []domain.Resource
	var token *string
	for {
		var page *ec2.DescribeVpcsOutput
		err := withRetry(ctx, c.Retry, func() error {
			resp, err := client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{NextToken: token})
			if err != nil {
				return mapAWSError("ec2", "DescribeVpcs", region, err)
			}
			page = resp
			return nil
		}, retryableAPIErr)
		if err != nil {
			return nil, err
		}
		for _, vpc := range page.Vpcs {
			pid := aws.ToString(vpc.VpcId)
			res := domain.Resource{
				ID:                 registerID(index, pid),
				Kind:               domain.KindVPC,
				ProviderResourceID: pid,
				AccountID:          accountID,
				RegionID:           regID,
				Name:               tagName(vpc.Tags),
				State:              string(vpc.State),
				Tags:               mapTags(vpc.Tags),
				Attributes:         map[string]string{"cidr": aws.ToString(vpc.CidrBlock)},
				Provenance:         domain.CollectProvenance(collectorSource, obs),
			}
			resources = append(resources, res)
		}
		if page.NextToken == nil || *page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	return resources, nil
}

func (c *Collector) collectSubnets(ctx context.Context, client EC2API, region string, accountID types.AccountID, regID types.RegionID, obs types.Timestamp, index map[string]types.ResourceID) ([]domain.Resource, []domain.Relationship, error) {
	var resources []domain.Resource
	var rels []domain.Relationship
	var token *string
	for {
		var page *ec2.DescribeSubnetsOutput
		err := withRetry(ctx, c.Retry, func() error {
			resp, err := client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{NextToken: token})
			if err != nil {
				return mapAWSError("ec2", "DescribeSubnets", region, err)
			}
			page = resp
			return nil
		}, retryableAPIErr)
		if err != nil {
			return nil, nil, err
		}
		for _, sn := range page.Subnets {
			pid := aws.ToString(sn.SubnetId)
			id := registerID(index, pid)
			resources = append(resources, domain.Resource{
				ID:                 id,
				Kind:               domain.KindSubnet,
				ProviderResourceID: pid,
				AccountID:          accountID,
				RegionID:           regID,
				Name:               tagName(sn.Tags),
				State:              "available",
				Tags:               mapTags(sn.Tags),
				Provenance:         domain.CollectProvenance(collectorSource, obs),
			})
			vpcID := aws.ToString(sn.VpcId)
			if vpcID != "" {
				toID, ok := lookupID(index, vpcID)
				rels = append(rels, domain.Relationship{
					Kind:                 domain.RelInVPC,
					FromResourceID:       id,
					ToResourceID:         toID,
					ToProviderResourceID: vpcID,
					TargetMissing:        !ok,
					Provenance:           domain.CollectProvenance(collectorSource, obs),
				})
			}
		}
		if page.NextToken == nil || *page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	return resources, rels, nil
}

func (c *Collector) collectNAT(ctx context.Context, client EC2API, region string, accountID types.AccountID, regID types.RegionID, obs types.Timestamp, index map[string]types.ResourceID) ([]domain.Resource, []domain.Relationship, error) {
	var resources []domain.Resource
	var rels []domain.Relationship
	var token *string
	for {
		var page *ec2.DescribeNatGatewaysOutput
		err := withRetry(ctx, c.Retry, func() error {
			resp, err := client.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{NextToken: token})
			if err != nil {
				return mapAWSError("ec2", "DescribeNatGateways", region, err)
			}
			page = resp
			return nil
		}, retryableAPIErr)
		if err != nil {
			return nil, nil, err
		}
		for _, ng := range page.NatGateways {
			pid := aws.ToString(ng.NatGatewayId)
			id := registerID(index, pid)
			resources = append(resources, domain.Resource{
				ID:                 id,
				Kind:               domain.KindNATGateway,
				ProviderResourceID: pid,
				AccountID:          accountID,
				RegionID:           regID,
				Name:               tagName(ng.Tags),
				State:              string(ng.State),
				Tags:               mapTags(ng.Tags),
				Provenance:         domain.CollectProvenance(collectorSource, obs),
			})
			subnetID := aws.ToString(ng.SubnetId)
			if subnetID != "" {
				toID, ok := lookupID(index, subnetID)
				rels = append(rels, domain.Relationship{
					Kind:                 domain.RelInSubnet,
					FromResourceID:       id,
					ToResourceID:         toID,
					ToProviderResourceID: subnetID,
					TargetMissing:        !ok,
					Provenance:           domain.CollectProvenance(collectorSource, obs),
				})
			}
		}
		if page.NextToken == nil || *page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	return resources, rels, nil
}

func (c *Collector) collectRouteTables(ctx context.Context, client EC2API, region string, accountID types.AccountID, regID types.RegionID, obs types.Timestamp, index map[string]types.ResourceID) ([]domain.Resource, error) {
	var resources []domain.Resource
	var token *string
	for {
		var page *ec2.DescribeRouteTablesOutput
		err := withRetry(ctx, c.Retry, func() error {
			resp, err := client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{NextToken: token})
			if err != nil {
				return mapAWSError("ec2", "DescribeRouteTables", region, err)
			}
			page = resp
			return nil
		}, retryableAPIErr)
		if err != nil {
			return nil, err
		}
		for _, rt := range page.RouteTables {
			pid := aws.ToString(rt.RouteTableId)
			resources = append(resources, domain.Resource{
				ID:                 registerID(index, pid),
				Kind:               domain.KindRouteTable,
				ProviderResourceID: pid,
				AccountID:          accountID,
				RegionID:           regID,
				Name:               tagName(rt.Tags),
				State:              "available",
				Tags:               mapTags(rt.Tags),
				Attributes:         map[string]string{"vpc_id": aws.ToString(rt.VpcId)},
				Provenance:         domain.CollectProvenance(collectorSource, obs),
			})
		}
		if page.NextToken == nil || *page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	return resources, nil
}

func (c *Collector) collectEIPs(ctx context.Context, client EC2API, region string, accountID types.AccountID, regID types.RegionID, obs types.Timestamp, index map[string]types.ResourceID) ([]domain.Resource, error) {
	var resources []domain.Resource
	err := withRetry(ctx, c.Retry, func() error {
		resp, err := client.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{})
		if err != nil {
			return mapAWSError("ec2", "DescribeAddresses", region, err)
		}
		for _, addr := range resp.Addresses {
			pid := aws.ToString(addr.AllocationId)
			if pid == "" {
				pid = aws.ToString(addr.PublicIp)
			}
			resources = append(resources, domain.Resource{
				ID:                 registerID(index, pid),
				Kind:               domain.KindElasticIP,
				ProviderResourceID: pid,
				AccountID:          accountID,
				RegionID:           regID,
				Name:               aws.ToString(addr.PublicIp),
				State:              eipState(addr),
				Tags:               mapTags(addr.Tags),
				Provenance:         domain.CollectProvenance(collectorSource, obs),
			})
		}
		return nil
	}, retryableAPIErr)
	return resources, err
}

func eipState(addr ec2types.Address) string {
	if addr.AssociationId != nil {
		return "associated"
	}
	return "available"
}

func (c *Collector) collectInstances(ctx context.Context, client EC2API, region string, accountID types.AccountID, regID types.RegionID, obs types.Timestamp, index map[string]types.ResourceID) ([]domain.Resource, []domain.Relationship, map[string]struct{}, error) {
	instances, err := PaginateInstances(ctx, client, region)
	if err != nil {
		return nil, nil, nil, err
	}
	typesUsed := map[string]struct{}{}
	var resources []domain.Resource
	var rels []domain.Relationship
	for _, inst := range instances {
		pid := aws.ToString(inst.InstanceId)
		id := registerID(index, pid)
		it := string(inst.InstanceType)
		typesUsed[it] = struct{}{}
		resources = append(resources, domain.Resource{
			ID:                 id,
			Kind:               domain.KindComputeInstance,
			ProviderResourceID: pid,
			AccountID:          accountID,
			RegionID:           regID,
			Name:               tagName(inst.Tags),
			State:              string(inst.State.Name),
			Tags:               mapTags(inst.Tags),
			Attributes: map[string]string{
				"instance_type": it,
				"platform":      string(inst.Platform),
			},
			Provenance: domain.CollectProvenance(collectorSource, obs),
		})
		subnetID := aws.ToString(inst.SubnetId)
		if subnetID != "" {
			toID, ok := lookupID(index, subnetID)
			rels = append(rels, domain.Relationship{
				Kind:                 domain.RelInSubnet,
				FromResourceID:       id,
				ToResourceID:         toID,
				ToProviderResourceID: subnetID,
				TargetMissing:        !ok,
				Provenance:           domain.CollectProvenance(collectorSource, obs),
			})
		}
	}
	return resources, rels, typesUsed, nil
}

func (c *Collector) collectInstanceTypes(ctx context.Context, client EC2API, region string, accountID types.AccountID, regID types.RegionID, obs types.Timestamp, typesUsed map[string]struct{}, index map[string]types.ResourceID) ([]domain.Resource, error) {
	var names []string
	for t := range typesUsed {
		names = append(names, t)
	}
	var resources []domain.Resource
	var token *string
	for {
		var page *ec2.DescribeInstanceTypesOutput
		err := withRetry(ctx, c.Retry, func() error {
			resp, err := client.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
				InstanceTypes: ec2NameList(names),
				NextToken:     token,
			})
			if err != nil {
				return mapAWSError("ec2", "DescribeInstanceTypes", region, err)
			}
			page = resp
			return nil
		}, retryableAPIErr)
		if err != nil {
			return nil, err
		}
		for _, it := range page.InstanceTypes {
			name := string(it.InstanceType)
			pid := "instance-type:" + name
			vcpu := ""
			if it.VCpuInfo != nil && it.VCpuInfo.DefaultVCpus != nil {
				vcpu = strconv.FormatInt(int64(*it.VCpuInfo.DefaultVCpus), 10)
			}
			mem := ""
			if it.MemoryInfo != nil && it.MemoryInfo.SizeInMiB != nil {
				mem = strconv.FormatInt(*it.MemoryInfo.SizeInMiB, 10)
			}
			resources = append(resources, domain.Resource{
				ID:                 registerID(index, pid),
				Kind:               domain.KindInstanceType,
				ProviderResourceID: name,
				AccountID:          accountID,
				RegionID:           regID,
				Name:               name,
				State:              "available",
				Attributes: map[string]string{
					"vcpus":      vcpu,
					"memory_mib": mem,
				},
				Provenance: domain.CollectProvenance(collectorSource, obs),
			})
		}
		if page.NextToken == nil || *page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	return resources, nil
}

func ec2NameList(names []string) []ec2types.InstanceType {
	out := make([]ec2types.InstanceType, len(names))
	for i, n := range names {
		out[i] = ec2types.InstanceType(n)
	}
	return out
}

func (c *Collector) collectVolumes(ctx context.Context, client EC2API, region string, accountID types.AccountID, regID types.RegionID, obs types.Timestamp, index map[string]types.ResourceID) ([]domain.Resource, []domain.Relationship, error) {
	var resources []domain.Resource
	var rels []domain.Relationship
	var token *string
	for {
		var page *ec2.DescribeVolumesOutput
		err := withRetry(ctx, c.Retry, func() error {
			resp, err := client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{NextToken: token})
			if err != nil {
				return mapAWSError("ec2", "DescribeVolumes", region, err)
			}
			page = resp
			return nil
		}, retryableAPIErr)
		if err != nil {
			return nil, nil, err
		}
		for _, vol := range page.Volumes {
			pid := aws.ToString(vol.VolumeId)
			id := registerID(index, pid)
			size := ""
			if vol.Size != nil {
				size = strconv.FormatInt(int64(*vol.Size), 10)
			}
			resources = append(resources, domain.Resource{
				ID:                 id,
				Kind:               domain.KindBlockVolume,
				ProviderResourceID: pid,
				AccountID:          accountID,
				RegionID:           regID,
				Name:               tagName(vol.Tags),
				State:              string(vol.State),
				Tags:               mapTags(vol.Tags),
				Attributes:         map[string]string{"size_gib": size},
				Provenance:         domain.CollectProvenance(collectorSource, obs),
			})
			for _, att := range vol.Attachments {
				instID := aws.ToString(att.InstanceId)
				if instID == "" {
					continue
				}
				toID, ok := lookupID(index, instID)
				rels = append(rels, domain.Relationship{
					Kind:                 domain.RelAttachedTo,
					FromResourceID:       id,
					ToResourceID:         toID,
					ToProviderResourceID: instID,
					TargetMissing:        !ok,
					Provenance:           domain.CollectProvenance(collectorSource, obs),
				})
			}
		}
		if page.NextToken == nil || *page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	return resources, rels, nil
}

func (c *Collector) collectSnapshots(ctx context.Context, client EC2API, region string, accountID types.AccountID, regID types.RegionID, obs types.Timestamp, index map[string]types.ResourceID) ([]domain.Resource, error) {
	var resources []domain.Resource
	var token *string
	owner := []string{"self"}
	for {
		var page *ec2.DescribeSnapshotsOutput
		err := withRetry(ctx, c.Retry, func() error {
			resp, err := client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{
				OwnerIds:  owner,
				NextToken: token,
			})
			if err != nil {
				return mapAWSError("ec2", "DescribeSnapshots", region, err)
			}
			page = resp
			return nil
		}, retryableAPIErr)
		if err != nil {
			return nil, err
		}
		for _, snap := range page.Snapshots {
			pid := aws.ToString(snap.SnapshotId)
			start := ""
			if snap.StartTime != nil {
				start = types.NewTimestamp(*snap.StartTime).Canonical()
			}
			resources = append(resources, domain.Resource{
				ID:                 registerID(index, pid),
				Kind:               domain.KindSnapshot,
				ProviderResourceID: pid,
				AccountID:          accountID,
				RegionID:           regID,
				Name:               tagName(snap.Tags),
				State:              string(snap.State),
				Tags:               mapTags(snap.Tags),
				Attributes: map[string]string{
					"volume_id":  aws.ToString(snap.VolumeId),
					"started_at": start,
				},
				Provenance: domain.CollectProvenance(collectorSource, obs),
			})
		}
		if page.NextToken == nil || *page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	return resources, nil
}

func (c *Collector) collectRDS(ctx context.Context, client RDSAPI, region string, accountID types.AccountID, regID types.RegionID, obs types.Timestamp, index map[string]types.ResourceID) ([]domain.Resource, error) {
	var resources []domain.Resource
	var marker *string
	for {
		var page *rds.DescribeDBInstancesOutput
		err := withRetry(ctx, c.Retry, func() error {
			resp, err := client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{Marker: marker})
			if err != nil {
				return mapAWSError("rds", "DescribeDBInstances", region, err)
			}
			page = resp
			return nil
		}, retryableAPIErr)
		if err != nil {
			return nil, err
		}
		for _, db := range page.DBInstances {
			pid := aws.ToString(db.DBInstanceIdentifier)
			resources = append(resources, domain.Resource{
				ID:                 registerID(index, pid),
				Kind:               domain.KindDatabase,
				ProviderResourceID: pid,
				AccountID:          accountID,
				RegionID:           regID,
				Name:               pid,
				State:              aws.ToString(db.DBInstanceStatus),
				Tags:               nil,
				Attributes: map[string]string{
					"engine":         aws.ToString(db.Engine),
					"instance_class": aws.ToString(db.DBInstanceClass),
				},
				Provenance: domain.CollectProvenance(collectorSource, obs),
			})
		}
		if page.Marker == nil || *page.Marker == "" {
			break
		}
		marker = page.Marker
	}
	return resources, nil
}

func tagName(tags []ec2types.Tag) string {
	for _, t := range tags {
		if aws.ToString(t.Key) == "Name" {
			return aws.ToString(t.Value)
		}
	}
	return ""
}

func mapTags(tags []ec2types.Tag) []domain.Tag {
	var out []domain.Tag
	for _, t := range tags {
		out = append(out, domain.Tag{Key: aws.ToString(t.Key), Value: aws.ToString(t.Value)})
	}
	return out
}
