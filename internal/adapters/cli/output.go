package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/kernul-io/cloudopt/internal/application/api"
)

// Result is the stable machine-readable command outcome written to stdout.
type Result struct {
	Status  string `json:"status"`
	Command string `json:"command,omitempty"`
	Message string `json:"message,omitempty"`
	Version string `json:"version,omitempty"`

	Analysis *AnalyzePayload `json:"analysis,omitempty"`
}

// AnalyzePayload is the detailed analyze output when --json is set.
type AnalyzePayload struct {
	AnalysisRunID  string              `json:"analysis_run_id,omitempty"`
	SnapshotID     string              `json:"snapshot_id"`
	RulesetVersion string              `json:"ruleset_version"`
	Summary        AnalyzeSummary      `json:"summary"`
	Findings       []AnalyzeFinding    `json:"findings,omitempty"`
	Rules          []AnalyzeRuleStatus `json:"rules,omitempty"`
}

type AnalyzeSummary struct {
	Passed       int `json:"passed"`
	Failed       int `json:"failed"`
	Suppressed   int `json:"suppressed"`
	NotEvaluated int `json:"not_evaluated"`
	Errors       int `json:"errors"`
}

type AnalyzeFinding struct {
	ID          string   `json:"id"`
	RuleID      string   `json:"rule_id"`
	Fingerprint string   `json:"fingerprint"`
	Severity    string   `json:"severity"`
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	ResourceIDs []string `json:"resource_ids"`
	Confidence  float64  `json:"confidence"`
}

type AnalyzeRuleStatus struct {
	RuleID  string `json:"rule_id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

const (
	StatusOK             = "ok"
	StatusError          = "error"
	StatusNotImplemented = "not_implemented"
)

func writeResult(w io.Writer, r Result) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

// EmitAnalyzeResult writes a successful analyze payload to stdout.
func EmitAnalyzeResult(result *api.AnalyzeResult) error {
	payload := AnalyzePayload{
		AnalysisRunID:  string(result.AnalysisRunID),
		SnapshotID:     string(result.SnapshotID),
		RulesetVersion: result.RulesetVersion,
		Summary: AnalyzeSummary{
			Passed:       result.Summary.Passed,
			Failed:       result.Summary.Failed,
			Suppressed:   result.Summary.Suppressed,
			NotEvaluated: result.Summary.NotEvaluated,
			Errors:       result.Summary.Errors,
		},
	}
	for _, f := range result.Findings {
		ids := make([]string, len(f.ResourceIDs))
		for i, id := range f.ResourceIDs {
			ids[i] = string(id)
		}
		payload.Findings = append(payload.Findings, AnalyzeFinding{
			ID:          string(f.ID),
			RuleID:      f.RuleID,
			Fingerprint: f.Fingerprint,
			Severity:    string(f.Severity),
			Category:    f.Category,
			Title:       f.Title,
			Description: f.Description,
			ResourceIDs: ids,
			Confidence:  f.Confidence.Float64(),
		})
	}
	for _, ex := range result.RuleExecutions {
		payload.Rules = append(payload.Rules, AnalyzeRuleStatus{
			RuleID:  ex.RuleID,
			Status:  string(ex.Status),
			Message: ex.Message,
		})
	}
	return writeResult(os.Stdout, Result{
		Status:   StatusOK,
		Command:  "analyze",
		Analysis: &payload,
	})
}

// EmitOK writes a successful command result to stdout.
func EmitOK(command, message string) error {
	return writeResult(os.Stdout, Result{
		Status:  StatusOK,
		Command: command,
		Message: message,
	})
}

// EmitError writes a failed command result to stdout.
func EmitError(command, message string) error {
	return writeResult(os.Stdout, Result{
		Status:  StatusError,
		Command: command,
		Message: message,
	})
}

// EmitNotImplemented writes a deferred-feature result to stdout.
func EmitNotImplemented(command, message string) error {
	return writeResult(os.Stdout, Result{
		Status:  StatusNotImplemented,
		Command: command,
		Message: message,
	})
}

// EmitVersion writes version metadata to stdout.
func EmitVersion(version string) error {
	return writeResult(os.Stdout, Result{
		Status:  StatusOK,
		Command: "version",
		Version: version,
	})
}

// FormatResult returns JSON for tests.
func FormatResult(r Result) (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}
	return string(b) + "\n", nil
}
