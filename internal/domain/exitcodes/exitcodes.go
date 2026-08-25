package exitcodes

const (
	Success        = 0
	GeneralError   = 1
	InvalidInput   = 2
	CollectionFail = 3
	AnalysisFail   = 4
	PartialSuccess = 5
	// TerraformCorrelationAmbiguous indicates correlation completed with ambiguous matches (CI gradual adoption).
	TerraformCorrelationAmbiguous = 5
	// TerraformPlanPolicy indicates plan-time policy warnings under --strict-plan.
	TerraformPlanPolicy = 7
)
