package capabilities

import (
	"sort"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// CoverageScores summarize data completeness for reporting (0–1 ratios).
type CoverageScores struct {
	InventoryAttribution float64  `json:"inventory_attribution"`
	CostAttribution      float64  `json:"cost_attribution"`
	MetricsCoverage      float64  `json:"metrics_coverage"`
	PricingFreshness     float64  `json:"pricing_freshness"`
	EvaluableSpend       float64  `json:"evaluable_spend"`
	Notes                []string `json:"notes,omitempty"`
}

// ScoreCoverage derives coverage metrics from a snapshot and optional pricing catalog presence.
func ScoreCoverage(snap *domain.CollectionSnapshot, pricingLoaded bool) CoverageScores {
	if snap == nil {
		return CoverageScores{}
	}
	out := CoverageScores{PricingFreshness: boolScore(pricingLoaded)}
	if len(snap.Resources) == 0 {
		out.Notes = append(out.Notes, "No inventory resources in snapshot.")
		return out
	}
	tagged := 0
	for _, r := range snap.Resources {
		if hasOwnerTag(r) {
			tagged++
		}
	}
	out.InventoryAttribution = float64(tagged) / float64(len(snap.Resources))

	var attributed, total int64
	for _, c := range snap.Costs {
		total += c.Amount.AmountMinor
		if c.ResourceID != domain.UnattributedResourceID {
			attributed += c.Amount.AmountMinor
		}
	}
	if total > 0 {
		out.CostAttribution = float64(attributed) / float64(total)
		out.EvaluableSpend = out.CostAttribution
	}

	if len(snap.UtilizationSignals) > 0 {
		var sum float64
		var n int
		for _, sig := range snap.UtilizationSignals {
			if sig.Kind == domain.SignalSampleCoverage {
				sum += sig.Value
				n++
			}
		}
		if n > 0 {
			out.MetricsCoverage = sum / float64(n)
		}
	} else if len(snap.Metrics) > 0 {
		out.MetricsCoverage = 1
	}

	if len(snap.Costs) == 0 {
		out.Notes = append(out.Notes, "No billing rows; cost-based rules may be not evaluated.")
		out.EvaluableSpend = 0
	}
	if !pricingLoaded {
		out.Notes = append(out.Notes, "Pricing catalog not loaded; rightsizing savings may be not evaluated.")
	}
	return out
}

func boolScore(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func hasOwnerTag(r domain.Resource) bool {
	for _, t := range r.Tags {
		if t.Key == "Owner" && t.Value != "" {
			return true
		}
	}
	return false
}

// MemberProviders returns distinct cloud providers represented in a snapshot.
func MemberProviders(snap *domain.CollectionSnapshot) []types.Provider {
	if snap == nil {
		return nil
	}
	if snap.Engagement != nil && len(snap.Engagement.Members) > 0 {
		seen := map[types.Provider]struct{}{}
		var out []types.Provider
		for _, m := range snap.Engagement.Members {
			if _, ok := seen[m.Provider]; ok {
				continue
			}
			seen[m.Provider] = struct{}{}
			out = append(out, m.Provider)
		}
		return out
	}
	if snap.Provider == types.ProviderMulti {
		seen := map[types.Provider]struct{}{}
		for _, r := range snap.Resources {
			p := ResourceProvider(r)
			if p == "" {
				continue
			}
			seen[p] = struct{}{}
		}
		var out []types.Provider
		for p := range seen {
			out = append(out, p)
		}
		sortProviders(out)
		return out
	}
	return []types.Provider{snap.Provider}
}

// ResourceProvider infers the native cloud for a canonical resource.
func ResourceProvider(r domain.Resource) types.Provider {
	if v := r.Attributes["cloud_provider"]; v != "" {
		return types.Provider(v)
	}
	if r.Attributes["gcp_self_link"] != "" || r.Attributes["gcp_project"] != "" {
		return types.ProviderGCP
	}
	id := r.ProviderResourceID
	if len(id) > 0 && (id[0] == 'i' && len(id) > 1 && id[1] == '-') ||
		len(id) > 3 && id[:3] == "vol" {
		return types.ProviderAWS
	}
	return ""
}

func sortProviders(in []types.Provider) {
	sort.Slice(in, func(i, j int) bool { return in[i] < in[j] })
}
