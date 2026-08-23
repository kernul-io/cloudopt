//go:build integration

package gcpinventory_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	gcpinventory "github.com/kernul-io/cloudopt/internal/adapters/gcp-inventory"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

func TestLiveCollector_integration(t *testing.T) {
	if os.Getenv("COA_GCP_INTEGRATION") != "1" {
		t.Skip("set COA_GCP_INTEGRATION=1 to run live GCP inventory test")
	}
	collector, err := gcpinventory.NewLiveCollector(context.Background(), gcpinventory.LiveOptions{
		ImpersonateServiceAccount: os.Getenv("COA_GCP_IMPERSONATE_SA"),
	})
	require.NoError(t, err)
	projects := os.Getenv("COA_GCP_PROJECTS")
	var projectList []string
	if projects != "" {
		projectList = []string{projects}
	}
	opts := ports.CollectOptions{
		Projects: projectList,
		Regions:  []string{os.Getenv("COA_GCP_REGION")},
		DryRun:   true,
	}
	pf, err := collector.Preflight(context.Background(), opts)
	require.NoError(t, err)
	require.NotEmpty(t, pf.CallerEmail+pf.CallerARN)
}
