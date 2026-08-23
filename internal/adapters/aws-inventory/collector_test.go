package awsinventory

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain"
)

func TestFixtureCollector_offlineInventory(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "aws-inventory")
	collector, err := NewFixtureCollector(root)
	require.NoError(t, err)

	opts := ports.CollectOptions{Regions: []string{"us-east-1"}, Offline: true}
	pf, err := collector.Preflight(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, "111122223333", pf.ProviderAccountID)

	snap, err := collector.Collect(context.Background(), opts, ports.NopProgress{})
	require.NoError(t, err)
	require.Equal(t, domain.SnapshotComplete, snap.Status)
	require.NotEmpty(t, snap.Resources)
}

func TestFilterRegions(t *testing.T) {
	got := filterRegions([]string{"us-east-1", "eu-west-1", "ap-south-1"}, []string{"us-east-1", "eu-west-1"}, []string{"eu-west-1"})
	require.Equal(t, []string{"us-east-1"}, got)
}

func TestWithRetry_throttling(t *testing.T) {
	attempts := 0
	err := withRetry(context.Background(), RetryConfig{MaxAttempts: 4, BaseDelay: 1, MaxDelay: 2}, func() error {
		attempts++
		if attempts < 3 {
			return &APIError{Code: "Throttling", Retryable: true}
		}
		return nil
	}, retryableAPIErr)
	require.NoError(t, err)
	require.Equal(t, 3, attempts)
}

func TestMapAWSError_accessDenied(t *testing.T) {
	err := mapAWSError("ec2", "DescribeInstances", "us-east-1", &fakeAPIErr{code: "AccessDenied"})
	require.Error(t, err)
	require.True(t, errorsIsAccessDenied(err))
}

type fakeAPIErr struct{ code string }

func (f *fakeAPIErr) Error() string        { return f.code }
func (f *fakeAPIErr) ErrorCode() string    { return f.code }
func (f *fakeAPIErr) ErrorMessage() string { return "denied" }
func (f *fakeAPIErr) ErrorFault() smithy.ErrorFault {
	return smithy.FaultUnknown
}

func TestCollect_missingPermissions(t *testing.T) {
	stubSTS := &stubInventorySTS{}
	denyEC2 := &denyDescribeInstancesEC2{}
	collector := NewCollector(stubSTS, denyEC2, func(string) EC2API { return denyEC2 }, func(string) RDSAPI { return &noopRDS{} })
	opts := ports.CollectOptions{Regions: []string{"us-east-1"}}

	_, err := collector.Collect(context.Background(), opts, ports.NopProgress{})
	require.Error(t, err)
	require.True(t, ports.IsMissingPermissions(err))
}

type stubInventorySTS struct{}

func (stubInventorySTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return &sts.GetCallerIdentityOutput{
		Account: aws.String("123456789012"),
		Arn:     aws.String("arn:aws:iam::123456789012:user/test"),
	}, nil
}

type denyDescribeInstancesEC2 struct{}

func (denyDescribeInstancesEC2) DescribeRegions(context.Context, *ec2.DescribeRegionsInput, ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	return &ec2.DescribeRegionsOutput{}, nil
}

func (denyDescribeInstancesEC2) DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return nil, fmt.Errorf("%w: denied", ErrAccessDenied)
}

func (denyDescribeInstancesEC2) DescribeInstanceTypes(context.Context, *ec2.DescribeInstanceTypesInput, ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
	return &ec2.DescribeInstanceTypesOutput{}, nil
}

func (denyDescribeInstancesEC2) DescribeVolumes(context.Context, *ec2.DescribeVolumesInput, ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	return &ec2.DescribeVolumesOutput{}, nil
}

func (denyDescribeInstancesEC2) DescribeSnapshots(context.Context, *ec2.DescribeSnapshotsInput, ...func(*ec2.Options)) (*ec2.DescribeSnapshotsOutput, error) {
	return &ec2.DescribeSnapshotsOutput{}, nil
}

func (denyDescribeInstancesEC2) DescribeVpcs(context.Context, *ec2.DescribeVpcsInput, ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	return &ec2.DescribeVpcsOutput{}, nil
}

func (denyDescribeInstancesEC2) DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput, ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	return &ec2.DescribeSubnetsOutput{}, nil
}

func (denyDescribeInstancesEC2) DescribeRouteTables(context.Context, *ec2.DescribeRouteTablesInput, ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	return &ec2.DescribeRouteTablesOutput{}, nil
}

func (denyDescribeInstancesEC2) DescribeNatGateways(context.Context, *ec2.DescribeNatGatewaysInput, ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error) {
	return &ec2.DescribeNatGatewaysOutput{}, nil
}

func (denyDescribeInstancesEC2) DescribeAddresses(context.Context, *ec2.DescribeAddressesInput, ...func(*ec2.Options)) (*ec2.DescribeAddressesOutput, error) {
	return &ec2.DescribeAddressesOutput{}, nil
}

type noopRDS struct{}

func (noopRDS) DescribeDBInstances(context.Context, *rds.DescribeDBInstancesInput, ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	return &rds.DescribeDBInstancesOutput{}, nil
}

func TestCollect_cancelled(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "aws-inventory")
	collector, err := NewFixtureCollector(root)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = collector.Collect(ctx, ports.CollectOptions{Regions: []string{"us-east-1"}}, ports.NopProgress{})
	require.Error(t, err)
}

func TestPaginateInstances(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "aws-inventory")
	store := &FixtureStore{Root: root}
	client := &fixtureEC2{store: store, region: "us-east-1"}
	instances, err := PaginateInstances(context.Background(), client, "us-east-1")
	require.NoError(t, err)
	require.Len(t, instances, 1)
}

func TestLoadCapabilities(t *testing.T) {
	caps, err := LoadCapabilities()
	require.NoError(t, err)
	require.Equal(t, "ec2_instances", caps.Inventory[0].ID)
	require.NotEmpty(t, IAMLeastPrivilegePolicy())
}
