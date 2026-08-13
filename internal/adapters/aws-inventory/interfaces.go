package awsinventory

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// EC2API is the read-only EC2 surface used by the collector (mockable in tests).
type EC2API interface {
	DescribeRegions(ctx context.Context, params *ec2.DescribeRegionsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error)
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	DescribeInstanceTypes(ctx context.Context, params *ec2.DescribeInstanceTypesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error)
	DescribeVolumes(ctx context.Context, params *ec2.DescribeVolumesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)
	DescribeSnapshots(ctx context.Context, params *ec2.DescribeSnapshotsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSnapshotsOutput, error)
	DescribeVpcs(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeRouteTables(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
	DescribeNatGateways(ctx context.Context, params *ec2.DescribeNatGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error)
	DescribeAddresses(ctx context.Context, params *ec2.DescribeAddressesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAddressesOutput, error)
}

// RDSAPI is the read-only RDS surface used by the collector.
type RDSAPI interface {
	DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
}

// STSAPI resolves caller identity for preflight.
type STSAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// EC2ClientFactory builds a regional EC2 client.
type EC2ClientFactory func(region string) EC2API

// RDSClientFactory builds a regional RDS client.
type RDSClientFactory func(region string) RDSAPI

// PaginateInstances collects all reservation instances in a region.
func PaginateInstances(ctx context.Context, client EC2API, region string) ([]types.Instance, error) {
	var out []types.Instance
	var token *string
	for {
		resp, err := describeInstancesPage(ctx, client, region, token)
		if err != nil {
			return nil, err
		}
		for _, res := range resp.Reservations {
			out = append(out, res.Instances...)
		}
		if resp.NextToken == nil || *resp.NextToken == "" {
			break
		}
		token = resp.NextToken
	}
	return out, nil
}

func describeInstancesPage(ctx context.Context, client EC2API, region string, token *string) (*ec2.DescribeInstancesOutput, error) {
	var result *ec2.DescribeInstancesOutput
	err := withRetry(ctx, defaultRetryConfig(), func() error {
		resp, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{NextToken: token})
		if err != nil {
			return mapAWSError("ec2", "DescribeInstances", region, err)
		}
		result = resp
		return nil
	}, retryableAPIErr)
	return result, err
}
