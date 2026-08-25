package terraform

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PlanChangeKind classifies forecast plan changes (separate from live-state findings).
type PlanChangeKind string

const (
	PlanCreate PlanChangeKind = "create"
	PlanUpdate PlanChangeKind = "update"
	PlanDelete PlanChangeKind = "delete"
	PlanNoOp   PlanChangeKind = "no-op"
)

// CostDirection estimates spend impact direction (catalog/heuristic, not measured billing).
type CostDirection string

const (
	CostIncrease CostDirection = "increase"
	CostDecrease CostDirection = "decrease"
	CostNeutral  CostDirection = "neutral"
	CostUnknown  CostDirection = "unknown"
)

// PlanChangeFinding is a forecast finding from plan JSON (not live inventory).
type PlanChangeFinding struct {
	TFAddress     string         `json:"terraform_address"`
	Change        PlanChangeKind `json:"change"`
	CostDirection CostDirection  `json:"cost_direction"`
	PolicyID      string         `json:"policy_id,omitempty"`
	PolicyResult  string         `json:"policy_result,omitempty"`
	Message       string         `json:"message"`
	SourceKind    string         `json:"source_kind"` // always "terraform_plan"
}

// PlanAnalysis aggregates plan-time checks.
type PlanAnalysis struct {
	Changes          []PlanChangeFinding `json:"changes"`
	PolicyViolations int                 `json:"policy_violations"`
	CostIncrease     int                 `json:"cost_increase_count"`
	CostDecrease     int                 `json:"cost_decrease_count"`
	Note             string              `json:"note"`
}

// AnalyzePlan inspects Terraform plan JSON resource changes without executing Terraform.
func AnalyzePlan(changes []PlanResourceChange) PlanAnalysis {
	var out PlanAnalysis
	out.Note = "Plan findings are forecasts from supplied plan JSON; live-state findings remain authoritative for drift."

	for _, ch := range changes {
		kind := classifyChange(ch.Actions)
		dir := estimateCostDirection(ch)
		policyID, policyResult, msg := checkPlanPolicy(ch, kind)

		f := PlanChangeFinding{
			TFAddress:     ch.Address,
			Change:        kind,
			CostDirection: dir,
			PolicyID:      policyID,
			PolicyResult:  policyResult,
			Message:       msg,
			SourceKind:    "terraform_plan",
		}
		out.Changes = append(out.Changes, f)

		switch dir {
		case CostIncrease:
			out.CostIncrease++
		case CostDecrease:
			out.CostDecrease++
		}
		if policyResult == "fail" || policyResult == "warn" {
			out.PolicyViolations++
		}
	}
	return out
}

// PlanResourceChange is a normalized plan delta (from adapter).
type PlanResourceChange struct {
	Address string
	Type    string
	Actions []string
	Before  map[string]string
	After   map[string]string
}

func classifyChange(actions []string) PlanChangeKind {
	if len(actions) == 0 {
		return PlanNoOp
	}
	set := map[string]bool{}
	for _, a := range actions {
		set[a] = true
	}
	switch {
	case set["create"] && !set["delete"]:
		return PlanCreate
	case set["delete"] && !set["create"]:
		return PlanDelete
	case set["update"]:
		return PlanUpdate
	default:
		return PlanNoOp
	}
}

func estimateCostDirection(ch PlanResourceChange) CostDirection {
	switch classifyChange(ch.Actions) {
	case PlanDelete:
		return CostDecrease
	case PlanCreate:
		return CostIncrease
	case PlanUpdate:
		return compareSizing(ch.Before, ch.After)
	default:
		return CostNeutral
	}
}

func compareSizing(before, after map[string]string) CostDirection {
	for _, key := range []string{"instance_type", "machine_type", "size", "volume_size", "size_slug"} {
		b, bok := before[key]
		a, aok := after[key]
		if !bok || !aok || b == a {
			continue
		}
		if sizeRank(a) < sizeRank(b) {
			return CostDecrease
		}
		if sizeRank(a) > sizeRank(b) {
			return CostIncrease
		}
	}
	return CostUnknown
}

func sizeRank(v string) int {
	// Minimal ordinal for demo policies; real deployments use pricing catalogs.
	ranks := map[string]int{
		"t3.micro": 1, "t3.small": 2, "t3.medium": 3, "m5.large": 4, "m5.xlarge": 5,
		"e2-micro": 1, "e2-small": 2, "e2-medium": 3, "n1-standard-4": 4,
		"s-1vcpu-1gb": 1, "s-2vcpu-4gb": 3,
	}
	if r, ok := ranks[v]; ok {
		return r
	}
	return 0
}

func checkPlanPolicy(ch PlanResourceChange, kind PlanChangeKind) (policyID, result, message string) {
	if kind == PlanCreate && strings.HasPrefix(ch.Type, "aws_nat_gateway") {
		return "avoid-standalone-nat", "warn", "Creating NAT Gateway — verify need; consider NAT instance or shared egress"
	}
	if kind == PlanUpdate {
		if b, ok := ch.Before["instance_type"]; ok {
			if a, ok2 := ch.After["instance_type"]; ok2 && sizeRank(a) > sizeRank(b) {
				return "rightsizing-guard", "warn", fmt.Sprintf("Instance type upsize %s -> %s increases cost", b, a)
			}
		}
	}
	if kind == PlanCreate && strings.Contains(ch.Type, "azurerm") && strings.Contains(ch.Type, "premium") {
		return "azure-premium-disk", "warn", "Premium SKU create — confirm performance tier"
	}
	return "", "pass", ""
}

// RenderMarkdownSummary produces a concise PR-comment style summary.
func RenderMarkdownSummary(result CorrelationResult) string {
	var b strings.Builder
	b.WriteString("## Terraform correlation summary\n\n")
	fmt.Fprintf(&b, "- Schema: `%s`\n", result.SchemaVersion)
	if result.SnapshotID != "" {
		fmt.Fprintf(&b, "- Snapshot: `%s`\n", result.SnapshotID)
	}
	matched := 0
	ambiguous := 0
	for _, l := range result.Links {
		if l.Ambiguous {
			ambiguous++
		} else if l.TFAddress != "" {
			matched++
		}
	}
	fmt.Fprintf(&b, "- Correlated: **%d** | Ambiguous: **%d** | Unmatched live: **%d** | Unmatched TF: **%d**\n\n",
		matched, ambiguous, len(result.UnmatchedLive), len(result.UnmatchedTF))

	if ambiguous > 0 {
		b.WriteString("### Ambiguous matches (human selection required)\n\n")
		for _, l := range result.Links {
			if !l.Ambiguous {
				continue
			}
			fmt.Fprintf(&b, "- `%s` (%s): %d candidates\n", l.ResourceID, l.ProviderCloudID, len(l.Candidates))
		}
		b.WriteString("\n")
	}

	if result.PlanAnalysis != nil {
		b.WriteString("### Plan forecast (not live state)\n\n")
		pa := result.PlanAnalysis
		fmt.Fprintf(&b, "- Policy violations/warnings: **%d** | Cost ↑ **%d** | Cost ↓ **%d**\n\n",
			pa.PolicyViolations, pa.CostIncrease, pa.CostDecrease)
		for _, c := range pa.Changes {
			if c.PolicyResult == "fail" || c.PolicyResult == "warn" {
				fmt.Fprintf(&b, "- `%s` (%s): %s\n", c.TFAddress, c.Change, c.Message)
			}
		}
	}

	b.WriteString("\n_Live-state findings and plan forecasts are reported separately; patches are never auto-applied._\n")
	return b.String()
}

// ToJSON serializes the correlation result.
func ToJSON(result CorrelationResult) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

// ExitCodeForResult maps result to stable CI exit semantics (gradual adoption).
func ExitCodeForResult(result CorrelationResult, strictPlan bool) int {
	if result.PlanAnalysis != nil && strictPlan && result.PlanAnalysis.PolicyViolations > 0 {
		return 7
	}
	for _, l := range result.Links {
		if l.Ambiguous {
			return 5
		}
	}
	return 0
}
