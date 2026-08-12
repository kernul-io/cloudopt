package awsinventory

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/ports"
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
