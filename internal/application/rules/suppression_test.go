package rules_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/rules"
)

func TestSuppressionActiveByFingerprint(t *testing.T) {
	snap := loadFixtureSnapshot(t)
	reg := rules.DefaultRegistry(nil)
	manifest, err := rules.LoadManifest("", reg)
	require.NoError(t, err)

	fp := rules.Fingerprint("storage.unattached_block_volume", "1.0.0", []types.ResourceID{"res-vol-unattached"})
	supp := rules.NewSuppressionIndex([]rules.SuppressionEntry{
		{Fingerprint: fp, Reason: "demo waiver", ExpiresAt: "2099-01-01T00:00:00Z"},
	}, snap.StartedAt.Time)

	out, err := rules.Engine{}.Analyze(rules.AnalyzeInput{
		Snapshot: snap, Manifest: manifest, Registry: reg, Suppressions: supp,
	})
	require.NoError(t, err)

	var unattachedFinding bool
	for _, f := range out.Findings {
		if f.RuleID == "storage.unattached_block_volume" {
			unattachedFinding = true
		}
	}
	require.False(t, unattachedFinding)
	require.GreaterOrEqual(t, out.Summary.Suppressed+out.Summary.Failed, 1)
}
