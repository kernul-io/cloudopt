package domain

import "github.com/kernul-io/cloudopt/internal/application/domain/types"

// CostReconciliation holds attributed vs source totals for one snapshot interval.
type CostReconciliation struct {
	SourceTotal          map[string]types.Money
	AttributedTotal      map[string]types.Money
	UnattributedTotal    map[string]types.Money
	Discrepancy          map[string]types.Money
	WithinTolerance      bool
	ToleranceBasisPoints int64
}
