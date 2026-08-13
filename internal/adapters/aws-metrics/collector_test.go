package awsmetrics

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

func TestFixtureMetricsCollection(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "aws-metrics")
	src := NewFixtureMetricsSource(root)
	opts := ports.MetricsCollectOptions{Offline: true, LookbackDays: 14, PeriodSeconds: 3600}
	pf, err := src.Preflight(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, 14, pf.LookbackDays)

	completed := domain.CollectionSnapshot{
		Resources: []domain.Resource{
			{ID: "res-i-0running01", Kind: domain.KindComputeInstance, ProviderResourceID: "i-0running01"},
			{ID: "res-vol-attached01", Kind: domain.KindBlockVolume, ProviderResourceID: "vol-attached01"},
			{ID: "res-demo-db-1", Kind: domain.KindDatabase, ProviderResourceID: "demo-db-1"},
		},
	}
	out, err := src.Collect(context.Background(), opts, &completed)
	require.NoError(t, err)
	require.NotEmpty(t, out.Series)
	require.NotEmpty(t, out.Signals)
}

func TestPartialCoverageWhenCloudWatchDenied(t *testing.T) {
	c := NewCollector(&stubSTS{}, &denyCW{})
	opts := ports.MetricsCollectOptions{LookbackDays: 7}
	out, err := c.Collect(context.Background(), opts, &domain.CollectionSnapshot{})
	require.NoError(t, err)
	require.True(t, out.Partial)
	require.Empty(t, out.Series)
	require.NotEmpty(t, out.Diagnostics)
}

func TestCollectCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	src := NewFixtureMetricsSource("testdata/aws-metrics")
	_, err := src.Collect(ctx, ports.MetricsCollectOptions{Offline: true}, &domain.CollectionSnapshot{})
	require.Error(t, err)
}

type stubSTS struct{}

func (stubSTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return &sts.GetCallerIdentityOutput{
		Account: aws.String("123456789012"),
		Arn:     aws.String("arn:aws:iam::123456789012:user/test"),
	}, nil
}

type denyCW struct{}

func (denyCW) GetMetricData(context.Context, *cloudwatch.GetMetricDataInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	return nil, &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
}
