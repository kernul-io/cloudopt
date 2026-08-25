package terraform

import (
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// CorrelationMethod describes how a live resource was linked to Terraform.
type CorrelationMethod string

const (
	MethodProviderID    CorrelationMethod = "provider_id"
	MethodTagLabel      CorrelationMethod = "tag_label"
	MethodUserMapping   CorrelationMethod = "user_mapping"
	MethodModuleMeta    CorrelationMethod = "module_metadata"
	MethodNameHeuristic CorrelationMethod = "name_heuristic"
)

// ConfidenceLevel ranks correlation certainty (never high for heuristic-only ambiguous matches).
type ConfidenceLevel string

const (
	ConfidenceHigh      ConfidenceLevel = "high"
	ConfidenceMedium    ConfidenceLevel = "medium"
	ConfidenceLow       ConfidenceLevel = "low"
	ConfidenceAmbiguous ConfidenceLevel = "ambiguous"
)

// ManagedResource is a Terraform-managed resource parsed from state or plan JSON (untrusted input).
type ManagedResource struct {
	Address       string
	ModulePath    string
	Type          string
	Name          string
	ProviderType  string
	ProviderAlias string
	IndexKey      string
	Mode          string
	SourceFile    string
	Values        map[string]string
}

// CorrelationCandidate is an alternate Terraform address when ownership is unclear.
type CorrelationCandidate struct {
	TFAddress string            `json:"terraform_address"`
	Method    CorrelationMethod `json:"method"`
	Reason    string            `json:"reason"`
}

// ConfigurableAttribute highlights a Terraform attribute relevant to remediation.
type ConfigurableAttribute struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Sensitive   bool   `json:"sensitive,omitempty"`
	Description string `json:"description,omitempty"`
}

// CorrelationLink binds a canonical inventory resource to Terraform metadata.
type CorrelationLink struct {
	ResourceID      types.ResourceID        `json:"resource_id"`
	Provider        types.Provider          `json:"provider"`
	ProviderCloudID string                  `json:"provider_resource_id"`
	TFAddress       string                  `json:"terraform_address,omitempty"`
	Method          CorrelationMethod       `json:"method,omitempty"`
	Confidence      ConfidenceLevel         `json:"confidence"`
	Ambiguous       bool                    `json:"ambiguous"`
	Candidates      []CorrelationCandidate  `json:"candidates,omitempty"`
	TFProvider      string                  `json:"terraform_provider,omitempty"`
	ProviderAlias   string                  `json:"provider_alias,omitempty"`
	ModulePath      string                  `json:"module_path,omitempty"`
	SourceFile      string                  `json:"source_file,omitempty"`
	Attributes      []ConfigurableAttribute `json:"attributes,omitempty"`
}

// UserMapping is an explicit consultant-provided link (YAML input).
type UserMapping struct {
	ResourceID types.ResourceID `yaml:"resource_id"`
	TFAddress  string           `yaml:"terraform_address"`
	Note       string           `yaml:"note,omitempty"`
}

// EnrichedFinding attaches IaC context to an optimization finding (live-state observation).
type EnrichedFinding struct {
	FindingID       types.FindingID   `json:"finding_id"`
	RuleID          string            `json:"rule_id"`
	Title           string            `json:"title"`
	SourceKind      string            `json:"source_kind"` // always "live_state" for findings
	ResourceLinks   []CorrelationLink `json:"resource_links"`
	Remediation     *RemediationGuide `json:"remediation,omitempty"`
	PatchSuggestion *PatchSuggestion  `json:"patch_suggestion,omitempty"`
}

// RemediationGuide is human-reviewable guidance (never auto-applied).
type RemediationGuide struct {
	Summary        string   `json:"summary"`
	Prerequisites  []string `json:"prerequisites"`
	Steps          []string `json:"steps"`
	ExpectedImpact string   `json:"expected_impact"`
	Validation     []string `json:"validation"`
	Rollback       []string `json:"rollback"`
	RiskLevel      string   `json:"risk_level"`
}

// PatchSuggestion is optional HCL-oriented hint; omitted when a safe edit cannot be represented.
type PatchSuggestion struct {
	TFAddress      string          `json:"terraform_address"`
	Attribute      string          `json:"attribute"`
	SuggestedValue string          `json:"suggested_value"`
	Confidence     ConfidenceLevel `json:"confidence"`
	RequiresReview bool            `json:"requires_review"`
}

// CorrelationResult is the CI-friendly aggregate output for correlate command.
type CorrelationResult struct {
	SchemaVersion    string            `json:"schema_version"`
	GeneratedAt      string            `json:"generated_at"`
	SnapshotID       string            `json:"snapshot_id,omitempty"`
	AnalysisRunID    string            `json:"analysis_run_id,omitempty"`
	StateSource      string            `json:"state_source,omitempty"`
	PlanSource       string            `json:"plan_source,omitempty"`
	Links            []CorrelationLink `json:"links"`
	UnmatchedLive    []UnmatchedLive   `json:"unmatched_live"`
	UnmatchedTF      []UnmatchedTF     `json:"unmatched_terraform"`
	EnrichedFindings []EnrichedFinding `json:"enriched_findings,omitempty"`
	PlanAnalysis     *PlanAnalysis     `json:"plan_analysis,omitempty"`
	ExitHint         string            `json:"exit_hint"`
}

type UnmatchedLive struct {
	ResourceID      types.ResourceID `json:"resource_id"`
	ProviderCloudID string           `json:"provider_resource_id"`
	Kind            string           `json:"kind"`
	Reason          string           `json:"reason"`
}

type UnmatchedTF struct {
	TFAddress string `json:"terraform_address"`
	Type      string `json:"type"`
	Reason    string `json:"reason"`
}
