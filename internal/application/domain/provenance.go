package domain

import "github.com/kernul-io/cloudopt/internal/application/domain/types"

// DataQuality describes how a value was obtained.
type DataQuality string

const (
	QualityObserved    DataQuality = "observed"
	QualityDerived     DataQuality = "derived"
	QualityEstimated   DataQuality = "estimated"
	QualityUnavailable DataQuality = "unavailable"
	QualityStale       DataQuality = "stale"
)

// Provenance tracks source and observation time for canonical data.
type Provenance struct {
	Quality    DataQuality
	Source     string
	ObservedAt types.Timestamp
}
