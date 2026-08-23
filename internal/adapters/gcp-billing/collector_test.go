package gcpbilling

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appbilling "github.com/kernul-io/cloudopt/internal/application/billing"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain"
)

func TestFixtureBillingCollection(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "gcp-billing")
	src := NewFixtureBillingSource(root)
	opts := ports.CostCollectOptions{Offline: true, LookbackDays: 30}
	pf, err := src.Preflight(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, 30, pf.LookbackDays)

	inv := &domain.CollectionSnapshot{
		Resources: []domain.Resource{
			{
				ID:                 "res-gce-app",
				ProviderResourceID: "projects/app-workloads-demo/zones/us-central1-a/instances/app-server-1",
				RegionID:           "reg-gcp-us-central1",
				Attributes:         map[string]string{"gcp_self_link": "projects/app-workloads-demo/zones/us-central1-a/instances/app-server-1"},
			},
			{
				ID:                 "res-disk-orphan",
				ProviderResourceID: "projects/app-workloads-demo/zones/us-central1-a/disks/orphan-data",
				RegionID:           "reg-gcp-us-central1",
				Attributes:         map[string]string{"gcp_self_link": "projects/app-workloads-demo/zones/us-central1-a/disks/orphan-data"},
			},
		},
	}
	out, err := src.Collect(context.Background(), opts, inv)
	require.NoError(t, err)
	require.NotEmpty(t, out.Costs)
	require.Contains(t, out.SourceTotals, "USD")

	recon := appbilling.Reconcile(out.SourceTotals, out.Costs, appbilling.DefaultReconcileToleranceBasisPoints)
	require.True(t, recon.WithinTolerance, "discrepancy: %+v", recon.Discrepancy)
	require.NotEmpty(t, out.Diagnostics)
}

func TestMissingPermissionsWhenExportNotConfigured(t *testing.T) {
	c := NewCollector(&stubBQ{})
	opts := ports.CostCollectOptions{LookbackDays: 7}
	pf, err := c.Preflight(context.Background(), opts)
	require.NoError(t, err)
	require.NotEmpty(t, pf.MissingActions)

	_, err = c.Collect(context.Background(), opts, &domain.CollectionSnapshot{})
	require.Error(t, err)
	require.True(t, ports.IsMissingPermissions(err))
}

func TestBigQueryPaginationFixture(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "gcp-billing")
	doc, err := loadBillingFixture(root)
	require.NoError(t, err)
	require.Greater(t, len(doc.Rows), 5)
}

func TestRowBillingEffects(t *testing.T) {
	in, diag := rowToInput(exportRow{
		Service: "Compute Engine", Region: "us-central1",
		ResourceName: "projects/p/z/instances/i",
		CostMajor:    10, Currency: "USD", SUD: true,
		PeriodStart: "2025-06-01", PeriodEnd: "2025-07-01",
	})
	require.Nil(t, diag)
	require.Equal(t, domain.CostBasisNetUnblended, in.Basis)
	require.Equal(t, domain.ChargeUsage, in.ChargeKind)
}

type stubBQ struct{}

func (stubBQ) CallerProject(context.Context) (string, error) { return "demo-proj", nil }

func (stubBQ) QueryExport(context.Context, string, string, string, time.Time, time.Time, string) ([]exportRow, string, error) {
	return nil, "", nil
}
