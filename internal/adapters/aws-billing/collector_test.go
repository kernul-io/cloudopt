package awsbilling

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain"
)

func TestFixtureCostCollection(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "aws-billing")
	src := NewFixtureBillingSource(root)
	opts := ports.CostCollectOptions{Offline: true, LookbackDays: 30}
	pf, err := src.Preflight(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, 30, pf.LookbackDays)

	inv := &domain.CollectionSnapshot{
		Resources: []domain.Resource{
			{ID: "res-aws-i-0running01", ProviderResourceID: "i-0running01", RegionID: "reg-us-east-1", Tags: []domain.Tag{{Key: "Owner", Value: "platform-team"}}},
			{ID: "res-aws-vol-orphan01", ProviderResourceID: "vol-orphan01", RegionID: "reg-us-east-1"},
		},
	}
	out, err := src.Collect(context.Background(), opts, inv)
	require.NoError(t, err)
	require.NotEmpty(t, out.Costs)
	require.NotEmpty(t, out.SourceTotals)
	require.Contains(t, out.SourceTotals, "USD")
}

func TestPartialCoverageWhenCostExplorerDenied(t *testing.T) {
	c := NewCollector(&stubSTS{}, &denyCE{})
	opts := ports.CostCollectOptions{LookbackDays: 7}
	out, err := c.Collect(context.Background(), opts, &domain.CollectionSnapshot{})
	require.NoError(t, err)
	require.True(t, out.Partial)
	require.Empty(t, out.Costs)
	require.NotEmpty(t, out.Diagnostics)
}

type stubSTS struct{}

func (stubSTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return &sts.GetCallerIdentityOutput{
		Account: aws.String("123456789012"),
		Arn:     aws.String("arn:aws:iam::123456789012:user/test"),
	}, nil
}

type denyCE struct{}

func (denyCE) GetCostAndUsage(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
	return nil, &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
}
