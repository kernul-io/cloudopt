package report

import (
	"encoding/json"
	"fmt"
)

// ToJSON serializes the report document with stable field ordering from struct tags.
func ToJSON(doc *Document) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("report document is nil")
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal report: %w", err)
	}
	b = append(b, '\n')
	return b, nil
}

// ValidateDocument checks required contract fields for tests and CLI guards.
func ValidateDocument(doc *Document) error {
	if doc == nil {
		return fmt.Errorf("document is nil")
	}
	if doc.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if doc.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %q", doc.SchemaVersion)
	}
	if doc.Analyzer.Version == "" {
		return fmt.Errorf("analyzer.version is required")
	}
	if doc.Analyzer.SnapshotID == "" {
		return fmt.Errorf("analyzer.snapshot_id is required")
	}
	if doc.Analyzer.AnalysisRunID == "" {
		return fmt.Errorf("analyzer.analysis_run_id is required")
	}
	if doc.Analyzer.RuleSetVersion == "" {
		return fmt.Errorf("analyzer.ruleset_version is required")
	}
	if doc.Disclaimer == "" {
		return fmt.Errorf("disclaimer is required")
	}
	if doc.GeneratedAt.IsZero() {
		return fmt.Errorf("generated_at is required")
	}
	return nil
}
