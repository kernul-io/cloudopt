package rules_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	awspricing "github.com/kernul-io/cloudopt/internal/adapters/aws-pricing"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/application/rules"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestRightsizingFixtureAcceptedAndRejectedCandidates(t *testing.T) {
	snap := loadFixtureSnapshot(t)
	cat := loadPricingCatalog(t)
	reg := rules.DefaultRegistry(cat)
	manifest, err := rules.LoadManifest("", reg)
	require.NoError(t, err)

	out, err := rules.Engine{}.Analyze(rules.AnalyzeInput{
		Snapshot:       snap,
		Manifest:       manifest,
		Registry:       reg,
		Suppressions:   rules.NewSuppressionIndex(nil, snap.StartedAt.Time),
		PricingCatalog: cat,
	})
	require.NoError(t, err)

	var downsize, ebs int
	for _, f := range out.Findings {
		switch f.RuleID {
		case "compute.ec2_downsize_candidate":
			downsize++
			require.NotEmpty(t, f.Assumptions)
		case "storage.ebs_volume_type_optimize":
			ebs++
		}
	}
	require.GreaterOrEqual(t, ebs, 1, "expected gp2 volume optimization finding")

	// Demonstrate rejected alternatives captured in evidence summaries for downsize when present.
	if downsize > 0 {
		var rejectEvidence bool
		for _, e := range out.Evidence {
			if e.Summary != "" && contains(e.Summary, "rejected candidate") {
				rejectEvidence = true
			}
		}
		require.True(t, rejectEvidence)
	}
}

func loadPricingCatalog(t *testing.T) *pricing.Catalog {
	t.Helper()
	root := filepath.Join("..", "..", "..", "testdata", "aws-pricing")
	col := awspricing.NewCollector(root)
	res, err := col.LoadCatalog(context.Background(), ports.PricingLoadOptions{
		Provider: types.ProviderAWS, Offline: true, FixtureRoot: root,
	})
	require.NoError(t, err)
	return pricing.NewCatalog(res.Records, res.Source)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexSubstring(s, sub))
}

func indexSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
