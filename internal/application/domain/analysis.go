package domain

import "github.com/kernul-io/cloudopt/internal/application/domain/types"

// EvidenceKind classifies supporting data for a finding.
type EvidenceKind string

const (
	EvidenceMetric       EvidenceKind = "metric"
	EvidenceCost         EvidenceKind = "cost"
	EvidenceResource     EvidenceKind = "resource_state"
	EvidenceRelationship EvidenceKind = "relationship"
	EvidenceDerived      EvidenceKind = "derived"
)

// Evidence references observable data backing a finding.
type Evidence struct {
	ID         int64
	Kind       EvidenceKind
	ResourceID types.ResourceID
	Summary    string
	Detail     map[string]string
	Provenance Provenance
}

// FindingSeverity ranks optimization issues.
type FindingSeverity string

const (
	SeverityInfo     FindingSeverity = "info"
	SeverityLow      FindingSeverity = "low"
	SeverityMedium   FindingSeverity = "medium"
	SeverityHigh     FindingSeverity = "high"
	SeverityCritical FindingSeverity = "critical"
)

// Finding is a deterministic optimization issue from an analysis run.
type Finding struct {
	ID          types.FindingID
	RuleID      string
	Fingerprint string
	Severity    FindingSeverity
	Category    string
	Title       string
	Description string
	ResourceIDs []types.ResourceID
	EvidenceIDs []int64
	Assumptions []string
	Confidence  types.Percentage
	Provenance  Provenance
}

// Recommendation describes safe remediation for a finding.
type Recommendation struct {
	ID                int64
	FindingID         types.FindingID
	Summary           string
	Steps             []string
	RiskLevel         string
	EstSavings        *types.Money
	EstSavingsLow     *types.Money
	EstSavingsHigh    *types.Money
	SavingsClass      SavingsClassification
	InvestigationOnly bool
	OverlapKey        string
	SavingsInputs     map[string]string
	Provenance        Provenance
}

// AnalysisRunStatus tracks analysis execution.
type AnalysisRunStatus string

const (
	AnalysisInProgress AnalysisRunStatus = "in_progress"
	AnalysisComplete   AnalysisRunStatus = "complete"
	AnalysisFailed     AnalysisRunStatus = "failed"
)

// AnalysisRun stores findings and recommendations for one snapshot evaluation.
type AnalysisRun struct {
	ID              types.AnalysisRunID
	SnapshotID      types.SnapshotID
	Status          AnalysisRunStatus
	RuleSetVersion  string
	StartedAt       types.Timestamp
	CompletedAt     *types.Timestamp
	Findings        []Finding
	Recommendations []Recommendation
	Evidence        []Evidence
}
