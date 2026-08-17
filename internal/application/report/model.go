package report

import "time"

// SchemaVersion is the JSON/HTML report export contract version.
const SchemaVersion = "1.0.0"

// ValueKind classifies how a number or statement should be interpreted.
type ValueKind string

const (
	KindMeasured       ValueKind = "measured"
	KindDerived        ValueKind = "derived"
	KindEstimate       ValueKind = "estimate"
	KindRecommendation ValueKind = "recommendation"
)

// Document is the versioned consultant report payload (independent of persistence types).
type Document struct {
	SchemaVersion string           `json:"schema_version"`
	GeneratedAt   time.Time        `json:"generated_at"`
	Analyzer      AnalyzerMeta     `json:"analyzer"`
	Customer      CustomerMeta     `json:"customer"`
	Scope         ScopeSection     `json:"scope"`
	Executive     ExecutiveSummary `json:"executive_summary"`
	Costs         CostSection      `json:"costs"`
	Savings       SavingsSection   `json:"potential_savings"`
	Findings      []FindingEntry   `json:"findings"`
	Appendix      Appendix         `json:"appendix"`
	Disclaimer    string           `json:"disclaimer"`
}

type AnalyzerMeta struct {
	Version        string `json:"version"`
	SnapshotID     string `json:"snapshot_id"`
	AnalysisRunID  string `json:"analysis_run_id"`
	RuleSetVersion string `json:"ruleset_version"`
}

type CustomerMeta struct {
	CustomerName string `json:"customer_name,omitempty"`
	ProjectName  string `json:"project_name,omitempty"`
	PreparedBy   string `json:"prepared_by,omitempty"`
}

type ScopeSection struct {
	Providers        []string       `json:"providers"`
	Accounts         []AccountScope `json:"accounts"`
	Regions          []string       `json:"regions"`
	ObservationStart string         `json:"observation_period_start"`
	ObservationEnd   string         `json:"observation_period_end"`
	ResourceCount    int            `json:"resource_count"`
	DataQualityNotes []string       `json:"data_quality_warnings"`
}

type AccountScope struct {
	DisplayName string `json:"display_name"`
	Alias       string `json:"alias"`
	Provider    string `json:"provider"`
}

type ExecutiveSummary struct {
	Headline           string `json:"headline"`
	FindingCount       int    `json:"finding_count"`
	ChecksPassed       int    `json:"checks_passed"`
	ChecksFailed       int    `json:"checks_failed"`
	ChecksSuppressed   int    `json:"checks_suppressed"`
	ChecksNotEvaluated int    `json:"checks_not_evaluated"`
	CheckErrors        int    `json:"check_errors"`
	SummaryText        string `json:"summary_text"`
}

type CostSection struct {
	Kind                ValueKind        `json:"kind"`
	ByCurrency          []CurrencyTotal  `json:"by_currency"`
	PeriodNote          string           `json:"period_note"`
	AttributionNote     string           `json:"attribution_note,omitempty"`
	SpendByServiceMinor map[string]int64 `json:"spend_by_service_minor,omitempty"`
	SpendByRegionMinor  map[string]int64 `json:"spend_by_region_minor,omitempty"`
	SpendByOwnerMinor   map[string]int64 `json:"spend_by_owner_minor,omitempty"`
}

type CurrencyTotal struct {
	Currency    string    `json:"currency"`
	AmountMajor float64   `json:"amount_major"`
	Kind        ValueKind `json:"kind"`
	Note        string    `json:"note"`
}

type SavingsSection struct {
	Kind             ValueKind          `json:"kind"`
	Note             string             `json:"note"`
	MonthlyTotalLow  map[string]float64 `json:"monthly_total_low_major,omitempty"`
	MonthlyTotalHigh map[string]float64 `json:"monthly_total_high_major,omitempty"`
	OneTime          []SavingsLine      `json:"one_time"`
	MonthlyRecurring []SavingsLine      `json:"monthly_recurring"`
	CommitmentBased  []SavingsLine      `json:"commitment_based"`
}

type SavingsLine struct {
	Description       string    `json:"description"`
	Currency          string    `json:"currency,omitempty"`
	AmountMajor       float64   `json:"amount_major,omitempty"`
	AmountLowMajor    float64   `json:"amount_low_major,omitempty"`
	AmountHighMajor   float64   `json:"amount_high_major,omitempty"`
	Kind              ValueKind `json:"kind"`
	FindingID         string    `json:"finding_id,omitempty"`
	InvestigationOnly bool      `json:"investigation_only,omitempty"`
}

type FindingEntry struct {
	ID          string           `json:"id"`
	RuleID      string           `json:"rule_id"`
	Fingerprint string           `json:"fingerprint"`
	Severity    string           `json:"severity"`
	Category    string           `json:"category"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Confidence  float64          `json:"confidence"`
	Resources   []ResourceRef    `json:"resources"`
	Evidence    []EvidenceEntry  `json:"evidence"`
	Assumptions []string         `json:"assumptions"`
	Remediation RemediationEntry `json:"remediation"`
}

type ResourceRef struct {
	Alias string `json:"alias"`
	Kind  string `json:"kind,omitempty"`
	Name  string `json:"name,omitempty"`
}

type EvidenceEntry struct {
	Kind    string            `json:"kind"`
	Summary string            `json:"summary"`
	Detail  map[string]string `json:"detail,omitempty"`
	Source  string            `json:"source"`
	KindTag ValueKind         `json:"value_kind"`
	Missing bool              `json:"missing,omitempty"`
}

type RemediationEntry struct {
	Summary   string    `json:"summary"`
	Steps     []string  `json:"steps"`
	RiskLevel string    `json:"risk_level"`
	Rollback  string    `json:"rollback_guidance"`
	Kind      ValueKind `json:"kind"`
}

type Appendix struct {
	Suppressed   []RuleOutcome        `json:"suppressed_checks"`
	NotEvaluated []RuleOutcome        `json:"not_evaluated_checks"`
	Passed       []RuleOutcome        `json:"passed_checks,omitempty"`
	Utilization  *UtilizationAppendix `json:"utilization,omitempty"`
}

type UtilizationAppendix struct {
	WindowStart   string                     `json:"observation_window_start,omitempty"`
	WindowEnd     string                     `json:"observation_window_end,omitempty"`
	PeriodSeconds int                        `json:"period_seconds,omitempty"`
	Resources     []ResourceUtilizationEntry `json:"resources,omitempty"`
}

type ResourceUtilizationEntry struct {
	Alias          string        `json:"alias"`
	Metric         string        `json:"metric"`
	SampleCoverage float64       `json:"sample_coverage"`
	Signals        []SignalEntry `json:"signals"`
}

type SignalEntry struct {
	Kind    string    `json:"kind"`
	Value   float64   `json:"value"`
	Unit    string    `json:"unit"`
	KindTag ValueKind `json:"value_kind"`
}

type RuleOutcome struct {
	RuleID  string `json:"rule_id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}
