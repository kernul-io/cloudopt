package domain

import (
	"time"

	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// CostGranularity describes the billing period for a cost record.
type CostGranularity string

const (
	CostDaily   CostGranularity = "daily"
	CostMonthly CostGranularity = "monthly"
)

// CostRecord attributes spend to a resource for a period.
type CostRecord struct {
	ID             int64
	ResourceID     types.ResourceID
	Service        string
	RegionID       types.RegionID
	Amount         types.Money
	Basis          CostBasis
	ChargeKind     CostChargeKind
	Granularity    CostGranularity
	PeriodStart    types.Timestamp
	PeriodEnd      types.Timestamp
	Attribution    CostAttribution
	SourceInterval BillingInterval
	Provenance     Provenance
}

// MetricPoint is a single utilization observation.
type MetricPoint struct {
	Timestamp types.Timestamp
	Value     float64
	Unit      string
	Quality   DataQuality
}

// MetricSeries is a named metric attached to a resource.
type MetricSeries struct {
	ID         int64
	ResourceID types.ResourceID
	Name       string
	Statistic  string
	Points     []MetricPoint
	Provenance Provenance
}

// BillingPeriod helper for comparisons in tests.
func (c CostRecord) PeriodDuration() time.Duration {
	return c.PeriodEnd.Sub(c.PeriodStart.Time)
}
