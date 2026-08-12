//go:build integration

package awsinventory_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/adapters/aws-inventory"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

func TestLiveCollector_integration(t *testing.T) {
	if os.Getenv("COA_AWS_INTEGRATION") != "1" {
		t.Skip("set COA_AWS_INTEGRATION=1 to run live AWS inventory test")
	}
	collector, err := awsinventory.NewLiveCollector(context.Background(), os.Getenv("COA_AWS_ROLE_ARN"), os.Getenv("COA_AWS_EXTERNAL_ID"))
	require.NoError(t, err)
	regions := os.Getenv("COA_AWS_REGIONS")
	var regionList []string
	if regions != "" {
		regionList = []string{regions}
	}
	opts := ports.CollectOptions{Regions: regionList, DryRun: true}
	pf, err := collector.Preflight(context.Background(), opts)
	require.NoError(t, err)
	require.NotEmpty(t, pf.ProviderAccountID)
}
