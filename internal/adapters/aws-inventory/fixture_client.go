package awsinventory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/kernul-io/cloudopt/internal/application/ports"
)

// FixtureStore serves recorded Describe* responses from disk for offline development.
type FixtureStore struct {
	Root string
}

// NewFixtureCollector returns a collector backed by JSON fixtures (no live AWS).
func NewFixtureCollector(root string) (ports.InventoryCollector, error) {
	if root == "" {
		return nil, fmt.Errorf("fixture root is required for offline collection")
	}
	store := &FixtureStore{Root: root}
	stsMock := &fixtureSTS{account: "111122223333", arn: "arn:aws:sts::111122223333:assumed-role/offline/cloudopt"}
	ec2Global := &fixtureEC2{store: store, region: "us-east-1"}
	ec2Factory := func(region string) EC2API {
		return &fixtureEC2{store: store, region: region}
	}
	rdsFactory := func(region string) RDSAPI {
		return &fixtureRDS{store: store, region: region}
	}
	return NewCollector(stsMock, ec2Global, ec2Factory, rdsFactory), nil
}

type fixtureSTS struct {
	account string
	arn     string
}

func (f *fixtureSTS) GetCallerIdentity(ctx context.Context, _ *sts.GetCallerIdentityInput, _ ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &sts.GetCallerIdentityOutput{
		Account: &f.account,
		Arn:     &f.arn,
	}, nil
}

type fixtureEC2 struct {
	store  *FixtureStore
	region string
	mu     sync.Mutex
	calls  map[string]int
}

func (f *fixtureEC2) load(name string, dest any) error {
	path := filepath.Join(f.store.Root, f.region, name+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join(f.store.Root, "default", name+".json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (f *fixtureEC2) DescribeRegions(ctx context.Context, _ *ec2.DescribeRegionsInput, _ ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out ec2.DescribeRegionsOutput
	if err := f.load("describe-regions", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (f *fixtureEC2) DescribeInstances(ctx context.Context, in *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls["DescribeInstances"]++
	call := f.calls["DescribeInstances"]
	f.mu.Unlock()

	var out ec2.DescribeInstancesOutput
	if err := f.load("describe-instances", &out); err != nil {
		return nil, err
	}
	if call > 1 && in.NextToken != nil && *in.NextToken == "page2" {
		var page2 ec2.DescribeInstancesOutput
		if err := f.load("describe-instances-page2", &page2); err == nil {
			return &page2, nil
		}
		return &ec2.DescribeInstancesOutput{}, nil
	}
	if out.NextToken != nil && *out.NextToken != "" && (in.NextToken == nil || *in.NextToken == "") {
		return &out, nil
	}
	if in.NextToken != nil && *in.NextToken == "page2" {
		return &ec2.DescribeInstancesOutput{}, nil
	}
	return &out, nil
}

func (f *fixtureEC2) DescribeInstanceTypes(ctx context.Context, _ *ec2.DescribeInstanceTypesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out ec2.DescribeInstanceTypesOutput
	if err := f.load("describe-instance-types", &out); err != nil {
		return &ec2.DescribeInstanceTypesOutput{}, nil
	}
	return &out, nil
}

func (f *fixtureEC2) DescribeVolumes(ctx context.Context, _ *ec2.DescribeVolumesInput, _ ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out ec2.DescribeVolumesOutput
	_ = f.load("describe-volumes", &out)
	return &out, nil
}

func (f *fixtureEC2) DescribeSnapshots(ctx context.Context, _ *ec2.DescribeSnapshotsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSnapshotsOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out ec2.DescribeSnapshotsOutput
	_ = f.load("describe-snapshots", &out)
	return &out, nil
}

func (f *fixtureEC2) DescribeVpcs(ctx context.Context, _ *ec2.DescribeVpcsInput, _ ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out ec2.DescribeVpcsOutput
	_ = f.load("describe-vpcs", &out)
	return &out, nil
}

func (f *fixtureEC2) DescribeSubnets(ctx context.Context, _ *ec2.DescribeSubnetsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out ec2.DescribeSubnetsOutput
	_ = f.load("describe-subnets", &out)
	return &out, nil
}

func (f *fixtureEC2) DescribeRouteTables(ctx context.Context, _ *ec2.DescribeRouteTablesInput, _ ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out ec2.DescribeRouteTablesOutput
	_ = f.load("describe-route-tables", &out)
	return &out, nil
}

func (f *fixtureEC2) DescribeNatGateways(ctx context.Context, _ *ec2.DescribeNatGatewaysInput, _ ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out ec2.DescribeNatGatewaysOutput
	_ = f.load("describe-nat-gateways", &out)
	return &out, nil
}

func (f *fixtureEC2) DescribeAddresses(ctx context.Context, _ *ec2.DescribeAddressesInput, _ ...func(*ec2.Options)) (*ec2.DescribeAddressesOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out ec2.DescribeAddressesOutput
	_ = f.load("describe-addresses", &out)
	return &out, nil
}

type fixtureRDS struct {
	store  *FixtureStore
	region string
}

func (f *fixtureRDS) DescribeDBInstances(ctx context.Context, _ *rds.DescribeDBInstancesInput, _ ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out rds.DescribeDBInstancesOutput
	_ = f.load("describe-db-instances", &out)
	return &out, nil
}

func (f *fixtureRDS) load(name string, dest any) error {
	path := filepath.Join(f.store.Root, f.region, name+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join(f.store.Root, "default", name+".json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}
