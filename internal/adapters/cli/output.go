package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

// Result is the stable machine-readable command outcome written to stdout.
type Result struct {
	Status  string `json:"status"`
	Command string `json:"command,omitempty"`
	Message string `json:"message,omitempty"`
	Version string `json:"version,omitempty"`

	Analysis  *AnalyzePayload       `json:"analysis,omitempty"`
	Report    *ReportPayload        `json:"report,omitempty"`
	Collect   *CollectPayload       `json:"collect,omitempty"`
	Cost      *CostCollectPayload   `json:"cost_collect,omitempty"`
	Reconcile *CostReconcilePayload `json:"cost_reconcile,omitempty"`
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

type ReportPayload struct {
	Path          string `json:"path"`
	Format        string `json:"format"`
	AnalysisRunID string `json:"analysis_run_id"`
	SnapshotID    string `json:"snapshot_id"`
}

type CollectPayload struct {
	SnapshotID string            `json:"snapshot_id,omitempty"`
	DryRun     bool              `json:"dry_run,omitempty"`
	Partial    bool              `json:"partial,omitempty"`
	Preflight  *CollectPreflight `json:"preflight,omitempty"`
}

type CollectPreflight struct {
	ProviderAccountID string   `json:"provider_account_id"`
	CallerARN         string   `json:"caller_arn"`
	SelectedRegions   []string `json:"selected_regions"`
	MissingActions    []string `json:"missing_actions,omitempty"`
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

// EmitCollectResult writes successful collect metadata to stdout.
func EmitCollectResult(result *api.CollectResult) error {
	if result == nil {
		return EmitOK("collect", "")
	}
	payload := &CollectPayload{
		SnapshotID: string(result.SnapshotID),
		DryRun:     result.DryRun,
		Partial:    result.Partial,
	}
	if result.Preflight != nil {
		payload.Preflight = &CollectPreflight{
			ProviderAccountID: result.Preflight.ProviderAccountID,
			CallerARN:         result.Preflight.CallerARN,
			SelectedRegions:   result.Preflight.SelectedRegions,
			MissingActions:    result.Preflight.MissingActions,
		}
	}
	return writeResult(os.Stdout, Result{
		Status:  StatusOK,
		Command: "collect",
		Collect: payload,
	})
}

type CostCollectPayload struct {
	SnapshotID string                           `json:"snapshot_id,omitempty"`
	DryRun     bool                             `json:"dry_run,omitempty"`
	Partial    bool                             `json:"partial,omitempty"`
	Preflight  *CostCollectPreflightPayload     `json:"preflight,omitempty"`
	Reconcile  *ports.CostReconciliationSummary `json:"reconciliation,omitempty"`
}

type CostCollectPreflightPayload struct {
	ProviderAccountID string   `json:"provider_account_id"`
	CallerARN         string   `json:"caller_arn"`
	LookbackDays      int      `json:"lookback_days"`
	MissingActions    []string `json:"missing_actions,omitempty"`
}

type CostReconcilePayload struct {
	SnapshotID string                           `json:"snapshot_id"`
	Reconcile  *ports.CostReconciliationSummary `json:"reconciliation"`
}

func EmitCostCollectResult(result *api.CostCollectResult) error {
	if result == nil {
		return EmitOK("collect", "")
	}
	payload := &CostCollectPayload{
		SnapshotID: result.SnapshotID,
		DryRun:     result.DryRun,
		Partial:    result.Partial,
		Reconcile:  result.Reconcile,
	}
	if result.Preflight != nil {
		payload.Preflight = &CostCollectPreflightPayload{
			ProviderAccountID: result.Preflight.ProviderAccountID,
			CallerARN:         result.Preflight.CallerARN,
			LookbackDays:      result.Preflight.LookbackDays,
			MissingActions:    result.Preflight.MissingActions,
		}
	}
	return writeResult(os.Stdout, Result{
		Status:  StatusOK,
		Command: "collect",
		Cost:    payload,
	})
}

func EmitCostReconcileResult(result *api.ReconcileCostResult) error {
	if result == nil {
		return EmitOK("cost-reconcile", "")
	}
	return writeResult(os.Stdout, Result{
		Status:  StatusOK,
		Command: "cost-reconcile",
		Reconcile: &CostReconcilePayload{
			SnapshotID: result.SnapshotID,
			Reconcile:  result.Reconcile,
		},
	})
}

// EmitReportResult writes successful report metadata to stdout.
func EmitReportResult(result *ports.ReportResult) error {
	if result == nil {
		return fmt.Errorf("report result is nil")
	}
	return writeResult(os.Stdout, Result{
		Status:  StatusOK,
		Command: "report",
		Report: &ReportPayload{
			Path:          result.DocumentPath,
			Format:        string(result.Format),
			AnalysisRunID: string(result.AnalysisRunID),
			SnapshotID:    string(result.SnapshotID),
		},
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
