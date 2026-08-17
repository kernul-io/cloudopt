package savings_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/savings"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestMonthlyRightsizingRecompute(t *testing.T) {
	inputs := map[string]string{
		"baseline_hourly_minor": "960",
		"target_hourly_minor":   "480",
		"headroom_bps":          "1500",
	}
	est := savings.MonthlyRightsizingFromHourly(960, 480, "USD", 1500, inputs)
	require.Equal(t, int64(960*730), est.BaselineMinor)
	require.Greater(t, est.GrossMonthlyMinor, int64(0))
	require.Less(t, est.LowMonthlyMinor, est.GrossMonthlyMinor)
	require.Greater(t, est.HighMonthlyMinor, est.GrossMonthlyMinor)
	// Recompute from inputs
	grossHourly := int64(960 - 480)
	adj := grossHourly - grossHourly*1500/10000
	require.Equal(t, adj*730, est.GrossMonthlyMinor)
}

func TestOverlapSuppressesDuplicateSavings(t *testing.T) {
	findings := []domain.Finding{
		{ID: "f1", RuleID: "a"},
		{ID: "f2", RuleID: "b"},
	}
	recs := []domain.Recommendation{
		{
			FindingID:  "f1",
			OverlapKey: "compute:res-1:lifecycle",
			EstSavings: &types.Money{AmountMinor: 1000, Currency: "USD"},
		},
		{
			FindingID:  "f2",
			OverlapKey: "compute:res-1:lifecycle",
			EstSavings: &types.Money{AmountMinor: 2000, Currency: "USD"},
		},
	}
	savings.ApplyOverlapPolicy(findings, &recs)
	var withSavings int
	for _, r := range recs {
		if r.EstSavings != nil {
			withSavings++
		}
	}
	require.Equal(t, 1, withSavings)
}

func TestSparseMetricsInvestigationOnlyExcludedFromTotals(t *testing.T) {
	recs := []domain.Recommendation{
		{EstSavings: &types.Money{AmountMinor: 5000, Currency: "USD"}, InvestigationOnly: true},
		{EstSavings: &types.Money{AmountMinor: 1000, Currency: "USD"}, SavingsClass: domain.SavingsMonthlyRecurring},
	}
	low, high := savings.AggregateMonthlyTotals(recs)
	require.InDelta(t, 10.0, low["USD"], 0.01)
	require.InDelta(t, 10.0, high["USD"], 0.01)
}
