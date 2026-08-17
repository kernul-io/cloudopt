package domain

// SavingsClassification separates on-demand rightsizing from commitments.
type SavingsClassification string

const (
	SavingsMonthlyRecurring SavingsClassification = "monthly_recurring"
	SavingsOneTime          SavingsClassification = "one_time"
	SavingsCommitment       SavingsClassification = "commitment"
)

// SavingsEstimate is auditable monthly (or one-time) savings from stored inputs.
type SavingsEstimate struct {
	BaselineMinor       int64
	CandidateMinor      int64
	GrossMonthlyMinor   int64
	LowMonthlyMinor     int64
	HighMonthlyMinor    int64
	ImplementationMinor int64
	Currency            string
	Class               SavingsClassification
	InvestigationOnly   bool
	OverlapKey          string
	Inputs              map[string]string
	Assumptions         []string
}
