package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/domain/types"
)

func TestMoneyAdd(t *testing.T) {
	a := types.Money{AmountMinor: 100, Currency: "USD"}
	b := types.Money{AmountMinor: 50, Currency: "USD"}
	sum, err := a.Add(b)
	require.NoError(t, err)
	require.Equal(t, int64(150), sum.AmountMinor)

	_, err = a.Add(types.Money{AmountMinor: 1, Currency: "EUR"})
	require.Error(t, err)
}

func TestFromMajorUnits(t *testing.T) {
	m := types.FromMajorUnits(42.50, "USD", 100)
	require.Equal(t, int64(4250), m.AmountMinor)
}

func TestPercentageFloat(t *testing.T) {
	p := types.PercentageFromFloat(0.125)
	require.InDelta(t, 0.125, p.Float64(), 0.0001)
}

func TestTimestampCanonical(t *testing.T) {
	ts, err := types.ParseTimestamp("2026-01-15T12:00:00Z")
	require.NoError(t, err)
	require.Equal(t, "2026-01-15T12:00:00Z", ts.Canonical())
}
