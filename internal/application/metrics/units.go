package metrics

import "strings"

// NormalizeUnitLabel maps provider unit strings to canonical labels.
func NormalizeUnitLabel(unit string) string {
	switch strings.TrimSpace(unit) {
	case "Percent", "percent", "%":
		return "percent"
	case "Count", "count":
		return "count"
	case "Bytes", "bytes":
		return "bytes"
	case "Bytes/Second", "bytes/second":
		return "bytes_per_second"
	case "Count/Second", "count/second":
		return "count_per_second"
	case "Seconds", "seconds":
		return "seconds"
	default:
		return strings.ToLower(strings.ReplaceAll(unit, " ", "_"))
	}
}

// NormalizeValue converts known unit variants to a scalar in the canonical unit.
func NormalizeValue(value float64, unit string) float64 {
	switch NormalizeUnitLabel(unit) {
	case "percent":
		return value
	case "bytes":
		return value
	case "bytes_per_second":
		return value
	case "count", "count_per_second":
		return value
	default:
		return value
	}
}

// BytesPerSecondToMbps converts bytes/s to megabits/s (deterministic network normalization).
func BytesPerSecondToMbps(bps float64) float64 {
	return bps * 8 / 1_000_000
}
