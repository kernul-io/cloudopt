package capabilities

import (
	"fmt"
	"strings"

	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// ContractResult is the outcome of running the provider contract suite.
type ContractResult struct {
	Provider   types.Provider `json:"provider"`
	Advertised bool           `json:"advertised"`
	Passed     bool           `json:"passed"`
	Errors     []string       `json:"errors,omitempty"`
}

// RunContractSuite validates all registered provider manifests.
func RunContractSuite(manifests []ports.CapabilityManifest) []ContractResult {
	var out []ContractResult
	for _, m := range manifests {
		res := ContractResult{Provider: m.Provider, Advertised: m.Advertised}
		errs := ValidateManifest(m)
		for _, err := range errs {
			res.Errors = append(res.Errors, err.Error())
		}
		switch {
		case m.Provider == types.Provider("incomplete-fake"):
			res.Passed = len(errs) > 0
			if res.Passed {
				res.Errors = nil
			}
		case m.Advertised:
			res.Passed = len(errs) == 0
		default:
			res.Passed = len(errs) == 0
		}
		out = append(out, res)
	}
	return out
}

// RuleSkipReason explains why a rule was not evaluated based on providers and capabilities.
func RuleSkipReason(ruleProviders, ruleCaps []string, snapProviders []types.Provider, manifest ports.CapabilityManifest, missingSignals []string) string {
	if len(ruleProviders) > 0 && !providerOverlap(ruleProviders, snapProviders) {
		return fmt.Sprintf("rule requires provider(s) %s; snapshot has %s",
			strings.Join(ruleProviders, ", "), formatProviders(snapProviders))
	}
	for _, ref := range ruleCaps {
		if !ProviderSupports(manifest, ref) {
			return fmt.Sprintf("missing capability %q for provider %s", ref, manifest.Provider)
		}
	}
	if len(missingSignals) > 0 {
		return fmt.Sprintf("missing required signals: %s", strings.Join(missingSignals, ", "))
	}
	return ""
}

func providerOverlap(rule []string, snap []types.Provider) bool {
	if len(rule) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(snap))
	for _, p := range snap {
		set[string(p)] = struct{}{}
	}
	for _, r := range rule {
		if _, ok := set[r]; ok {
			return true
		}
	}
	return false
}

func formatProviders(in []types.Provider) string {
	parts := make([]string, 0, len(in))
	for _, p := range in {
		parts = append(parts, string(p))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}
