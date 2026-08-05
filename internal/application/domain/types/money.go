package types

import (
	"fmt"
	"math"
)

// Money stores exact monetary amounts in minor currency units (e.g. cents).
type Money struct {
	AmountMinor int64
	Currency    string // ISO 4217, e.g. USD
}

func (m Money) String() string {
	if m.Currency == "" {
		return fmt.Sprintf("%d minor", m.AmountMinor)
	}
	major := float64(m.AmountMinor) / 100.0
	return fmt.Sprintf("%.2f %s", major, m.Currency)
}

// Add returns the sum when currencies match.
func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("currency mismatch: %q vs %q", m.Currency, other.Currency)
	}
	sum := m.AmountMinor + other.AmountMinor
	if (other.AmountMinor > 0 && sum < m.AmountMinor) || (other.AmountMinor < 0 && sum > m.AmountMinor) {
		return Money{}, fmt.Errorf("money overflow")
	}
	return Money{AmountMinor: sum, Currency: m.Currency}, nil
}

// FromMajorUnits converts major units (e.g. dollars) to minor units using the given scale.
func FromMajorUnits(major float64, currency string, minorPerMajor int64) Money {
	minor := int64(math.Round(major * float64(minorPerMajor)))
	return Money{AmountMinor: minor, Currency: currency}
}
