package api_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestMetricsCollectService_missingPermissions(t *testing.T) {
	t.Parallel()

	src := &stubMetricsSource{
		preflight: &ports.MetricsPreflight{
			ProviderAccountID: "123456789012",
			MissingActions:    []string{"cloudwatch:GetMetricData"},
		},
	}
	svc := &api.MetricsCollectService{
		Repo:   &stubStorageRepo{},
		Source: src,
	}

	_, err := svc.Collect(context.Background(), ports.MetricsCollectOptions{})
	require.Error(t, err)
	require.True(t, ports.IsMissingPermissions(err))
	require.False(t, src.collectCalled)
}

func TestMetricsCollectService_missingPermissionsDryRun(t *testing.T) {
	t.Parallel()

	src := &stubMetricsSource{
		preflight: &ports.MetricsPreflight{
			MissingActions: []string{"cloudwatch:GetMetricData"},
		},
	}
	svc := &api.MetricsCollectService{Source: src}

	_, err := svc.Collect(context.Background(), ports.MetricsCollectOptions{DryRun: true})
	require.Error(t, err)
	require.True(t, ports.IsMissingPermissions(err))
	require.False(t, src.collectCalled)
}

type stubMetricsSource struct {
	preflight     *ports.MetricsPreflight
	collectCalled bool
}

func (s *stubMetricsSource) Capabilities() ports.CapabilityManifest {
	return ports.CapabilityManifest{Provider: types.ProviderAWS}
}

func (s *stubMetricsSource) Preflight(context.Context, ports.MetricsCollectOptions) (*ports.MetricsPreflight, error) {
	return s.preflight, nil
}

func (s *stubMetricsSource) Collect(context.Context, ports.MetricsCollectOptions, *domain.CollectionSnapshot) (*ports.MetricsCollectOutput, error) {
	s.collectCalled = true
	return &ports.MetricsCollectOutput{}, nil
}

type stubStorageRepo struct{}

func (stubStorageRepo) Migrate(context.Context) error                                  { return nil }
func (stubStorageRepo) Close() error                                                   { return nil }
func (stubStorageRepo) SchemaVersion(context.Context) (int, error)                     { return 1, nil }
func (stubStorageRepo) CanonicalSchemaVersion(context.Context) (int, error)            { return 0, nil }
func (stubStorageRepo) SaveSnapshot(context.Context, *domain.CollectionSnapshot) error { return nil }
func (stubStorageRepo) SaveInProgressSnapshot(context.Context, *domain.CollectionSnapshot) error {
	return nil
}
func (stubStorageRepo) ReplaceSnapshotCosts(context.Context, types.SnapshotID, []domain.CostRecord, []domain.ServiceCollectionStatus, map[string]types.Money) error {
	return nil
}
func (stubStorageRepo) ReplaceSnapshotMetrics(context.Context, types.SnapshotID, []domain.MetricSeries, []domain.UtilizationSignal, *domain.MetricsCollectionMeta, []domain.ServiceCollectionStatus) error {
	return nil
}
func (stubStorageRepo) GetSnapshot(context.Context, types.SnapshotID) (*domain.CollectionSnapshot, error) {
	return nil, nil
}
func (stubStorageRepo) GetSnapshotBillingSourceTotals(context.Context, types.SnapshotID) (map[string]types.Money, error) {
	return nil, nil
}
func (stubStorageRepo) ListSnapshots(context.Context, ports.ListSnapshotFilter) ([]domain.SnapshotSummary, error) {
	return nil, nil
}
func (stubStorageRepo) MarkSnapshotFailed(context.Context, types.SnapshotID) error { return nil }
func (stubStorageRepo) SaveAnalysisRun(context.Context, *domain.AnalysisRun) error { return nil }
func (stubStorageRepo) GetAnalysisRun(context.Context, types.AnalysisRunID) (*domain.AnalysisRun, error) {
	return nil, nil
}
func (stubStorageRepo) GetLatestAnalysisRun(context.Context, types.SnapshotID) (*domain.AnalysisRun, error) {
	return nil, nil
}
func (stubStorageRepo) DeleteSnapshot(context.Context, types.SnapshotID) error { return nil }
func (stubStorageRepo) DeleteSnapshotsByAccount(context.Context, types.AccountID) (int, error) {
	return 0, nil
}
func (stubStorageRepo) ApplyRetention(context.Context, types.AccountID, int) (int, error) {
	return 0, nil
}
