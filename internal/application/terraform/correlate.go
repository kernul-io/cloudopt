package terraform

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// CorrelateOptions configures correlation against inventory and optional analysis output.
type CorrelateOptions struct {
	SnapshotID      types.SnapshotID
	AnalysisRunID   types.AnalysisRunID
	Mappings        []UserMapping
	Resources       []domain.Resource
	Provider        types.Provider
	Findings        []domain.Finding
	Recommendations map[types.FindingID]domain.Recommendation
}

// Correlate matches live inventory resources to parsed Terraform resources.
func Correlate(tfResources []ManagedResource, opts CorrelateOptions) CorrelationResult {
	mappingByResource := map[types.ResourceID]UserMapping{}
	for _, m := range opts.Mappings {
		mappingByResource[m.ResourceID] = m
	}

	tfByID := indexTFByCloudID(tfResources)
	usedTF := map[string]bool{}

	var links []CorrelationLink
	var unmatchedLive []UnmatchedLive

	for _, res := range opts.Resources {
		if m, ok := mappingByResource[res.ID]; ok {
			tf := findTFByAddress(tfResources, m.TFAddress)
			link := CorrelationLink{
				ResourceID:      res.ID,
				Provider:        opts.Provider,
				ProviderCloudID: res.ProviderResourceID,
				TFAddress:       m.TFAddress,
				Method:          MethodUserMapping,
				Confidence:      ConfidenceHigh,
				Ambiguous:       false,
			}
			if tf != nil {
				enrichFromTF(&link, *tf)
				usedTF[tf.Address] = true
			}
			links = append(links, link)
			continue
		}

		direct := matchByProviderID(res, tfByID)
		if len(direct) == 1 {
			link := buildLink(res, opts.Provider, direct[0], MethodProviderID, ConfidenceHigh, false, nil)
			links = append(links, link)
			usedTF[direct[0].Address] = true
			continue
		}
		if len(direct) > 1 {
			cands := candidatesFromTF(direct, MethodProviderID, "multiple Terraform resources share the same cloud ID")
			links = append(links, CorrelationLink{
				ResourceID:      res.ID,
				Provider:        opts.Provider,
				ProviderCloudID: res.ProviderResourceID,
				Method:          MethodProviderID,
				Confidence:      ConfidenceAmbiguous,
				Ambiguous:       true,
				Candidates:      cands,
			})
			continue
		}

		tagMatches := matchByTags(res, tfResources)
		if len(tagMatches) == 1 {
			link := buildLink(res, opts.Provider, tagMatches[0], MethodTagLabel, ConfidenceMedium, false, nil)
			links = append(links, link)
			usedTF[tagMatches[0].Address] = true
			continue
		}
		if len(tagMatches) > 1 {
			cands := candidatesFromTF(tagMatches, MethodTagLabel, "tag/label overlap")
			links = append(links, CorrelationLink{
				ResourceID:      res.ID,
				Provider:        opts.Provider,
				ProviderCloudID: res.ProviderResourceID,
				Method:          MethodTagLabel,
				Confidence:      ConfidenceAmbiguous,
				Ambiguous:       true,
				Candidates:      cands,
			})
			continue
		}

		nameMatches := matchByNameHeuristic(res, tfResources, usedTF)
		if len(nameMatches) == 1 {
			link := buildLink(res, opts.Provider, nameMatches[0], MethodNameHeuristic, ConfidenceLow, false, nil)
			links = append(links, link)
			usedTF[nameMatches[0].Address] = true
			continue
		}
		if len(nameMatches) > 1 {
			cands := candidatesFromTF(nameMatches, MethodNameHeuristic, "name similarity only — requires human selection")
			links = append(links, CorrelationLink{
				ResourceID:      res.ID,
				Provider:        opts.Provider,
				ProviderCloudID: res.ProviderResourceID,
				Method:          MethodNameHeuristic,
				Confidence:      ConfidenceAmbiguous,
				Ambiguous:       true,
				Candidates:      cands,
			})
			continue
		}

		unmatchedLive = append(unmatchedLive, UnmatchedLive{
			ResourceID:      res.ID,
			ProviderCloudID: res.ProviderResourceID,
			Kind:            string(res.Kind),
			Reason:          "no matching Terraform resource in supplied state/plan",
		})
	}

	var unmatchedTF []UnmatchedTF
	for _, tf := range tfResources {
		if tf.Mode == "data" {
			continue
		}
		if usedTF[tf.Address] {
			continue
		}
		unmatchedTF = append(unmatchedTF, UnmatchedTF{
			TFAddress: tf.Address,
			Type:      tf.Type,
			Reason:    "no live inventory resource correlated",
		})
	}

	sort.Slice(links, func(i, j int) bool {
		return links[i].ResourceID < links[j].ResourceID
	})
	sort.Slice(unmatchedLive, func(i, j int) bool {
		return unmatchedLive[i].ResourceID < unmatchedLive[j].ResourceID
	})
	sort.Slice(unmatchedTF, func(i, j int) bool {
		return unmatchedTF[i].TFAddress < unmatchedTF[j].TFAddress
	})

	result := CorrelationResult{
		SchemaVersion: "1.0.0",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		SnapshotID:    string(opts.SnapshotID),
		AnalysisRunID: string(opts.AnalysisRunID),
		Links:         links,
		UnmatchedLive: unmatchedLive,
		UnmatchedTF:   unmatchedTF,
	}

	if len(opts.Findings) > 0 {
		linkByResource := map[types.ResourceID]CorrelationLink{}
		for _, l := range links {
			if l.Ambiguous || l.TFAddress == "" {
				continue
			}
			linkByResource[l.ResourceID] = l
		}
		result.EnrichedFindings = enrichFindings(opts.Findings, opts.Recommendations, linkByResource)
	}

	return result
}

func indexTFByCloudID(resources []ManagedResource) map[string][]ManagedResource {
	out := map[string][]ManagedResource{}
	for _, tf := range resources {
		if tf.Mode == "data" {
			continue
		}
		id := cloudIDFromTF(tf)
		if id == "" {
			continue
		}
		out[id] = append(out[id], tf)
	}
	return out
}

func cloudIDFromTF(tf ManagedResource) string {
	key := providerIDAttribute(tf.Type)
	if v, ok := tf.Values[key]; ok && v != "" {
		return v
	}
	// Some imports expose arn instead of id for AWS.
	if v, ok := tf.Values["arn"]; ok && strings.HasPrefix(v, "arn:") {
		return v
	}
	return ""
}

func matchByProviderID(res domain.Resource, index map[string][]ManagedResource) []ManagedResource {
	if res.ProviderResourceID == "" {
		return nil
	}
	if hits, ok := index[res.ProviderResourceID]; ok {
		return hits
	}
	return nil
}

func matchByTags(res domain.Resource, all []ManagedResource) []ManagedResource {
	liveTags := tagSet(res.Tags)
	if len(liveTags) == 0 {
		return nil
	}
	var hits []ManagedResource
	for _, tf := range all {
		if tf.Mode == "data" {
			continue
		}
		for _, field := range tagFields(tf.Type) {
			raw, ok := tf.Values[field]
			if !ok || raw == "" {
				continue
			}
			tfTags := parseTagBlob(raw)
			if tagsOverlap(liveTags, tfTags) {
				hits = append(hits, tf)
				break
			}
		}
	}
	return hits
}

func matchByNameHeuristic(res domain.Resource, all []ManagedResource, used map[string]bool) []ManagedResource {
	if res.Name == "" {
		return nil
	}
	nameLower := strings.ToLower(res.Name)
	var hits []ManagedResource
	for _, tf := range all {
		if tf.Mode == "data" || used[tf.Address] {
			continue
		}
		if strings.EqualFold(tf.Name, res.Name) {
			hits = append(hits, tf)
			continue
		}
		if tagName, ok := tf.Values["tags.Name"]; ok && strings.EqualFold(tagName, res.Name) {
			hits = append(hits, tf)
			continue
		}
		if strings.Contains(strings.ToLower(tf.Address), nameLower) {
			hits = append(hits, tf)
		}
	}
	return hits
}

func tagSet(tags []domain.Tag) map[string]string {
	out := map[string]string{}
	for _, t := range tags {
		out[strings.ToLower(t.Key)] = strings.ToLower(t.Value)
	}
	return out
}

func parseTagBlob(raw string) map[string]string {
	// Values from state JSON are flattened as "key=value,key2=value2" in our adapter.
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		out[strings.ToLower(kv[0])] = strings.ToLower(kv[1])
	}
	return out
}

func tagsOverlap(a, b map[string]string) bool {
	matches := 0
	for k, v := range a {
		if bv, ok := b[k]; ok && bv == v {
			matches++
		}
	}
	return matches >= 2 || (matches == 1 && len(a) == 1 && len(b) == 1)
}

func findTFByAddress(all []ManagedResource, address string) *ManagedResource {
	for i := range all {
		if all[i].Address == address {
			return &all[i]
		}
	}
	return nil
}

func candidatesFromTF(resources []ManagedResource, method CorrelationMethod, reason string) []CorrelationCandidate {
	out := make([]CorrelationCandidate, 0, len(resources))
	for _, tf := range resources {
		out = append(out, CorrelationCandidate{
			TFAddress: tf.Address,
			Method:    method,
			Reason:    reason,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TFAddress < out[j].TFAddress })
	return out
}

func buildLink(res domain.Resource, provider types.Provider, tf ManagedResource, method CorrelationMethod, conf ConfidenceLevel, ambiguous bool, cands []CorrelationCandidate) CorrelationLink {
	link := CorrelationLink{
		ResourceID:      res.ID,
		Provider:        provider,
		ProviderCloudID: res.ProviderResourceID,
		TFAddress:       tf.Address,
		Method:          method,
		Confidence:      conf,
		Ambiguous:       ambiguous,
		Candidates:      cands,
	}
	enrichFromTF(&link, tf)
	return link
}

func enrichFromTF(link *CorrelationLink, tf ManagedResource) {
	link.TFProvider = tf.ProviderType
	link.ProviderAlias = tf.ProviderAlias
	link.ModulePath, _ = ParseModulePath(tf.Address)
	link.SourceFile = tf.SourceFile
	link.Attributes = configurableAttributes(tf)
}

func configurableAttributes(tf ManagedResource) []ConfigurableAttribute {
	candidates := []struct {
		name, desc string
	}{
		{"instance_type", "Compute instance size"},
		{"machine_type", "GCE machine type"},
		{"size", "Volume size"},
		{"volume_size", "EBS volume size (GiB)"},
		{"allocated_storage", "RDS allocated storage"},
		{"sku_name", "Azure SKU"},
		{"size_slug", "DigitalOcean droplet size"},
	}
	var out []ConfigurableAttribute
	for _, c := range candidates {
		if v, ok := tf.Values[c.name]; ok && v != "" {
			out = append(out, ConfigurableAttribute{
				Name:        c.name,
				Value:       v,
				Description: c.desc,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func enrichFindings(findings []domain.Finding, recs map[types.FindingID]domain.Recommendation, links map[types.ResourceID]CorrelationLink) []EnrichedFinding {
	var out []EnrichedFinding
	for _, f := range findings {
		ef := EnrichedFinding{
			FindingID:  f.ID,
			RuleID:     f.RuleID,
			Title:      f.Title,
			SourceKind: "live_state",
		}
		for _, rid := range f.ResourceIDs {
			if link, ok := links[rid]; ok {
				ef.ResourceLinks = append(ef.ResourceLinks, link)
			}
		}
		if rec, ok := recs[f.ID]; ok {
			ef.Remediation = remediationFromRecommendation(rec, ef.ResourceLinks)
			ef.PatchSuggestion = patchForFinding(f, ef.ResourceLinks)
		}
		out = append(out, ef)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FindingID < out[j].FindingID })
	return out
}

func remediationFromRecommendation(rec domain.Recommendation, links []CorrelationLink) *RemediationGuide {
	steps := append([]string(nil), rec.Steps...)
	if len(steps) == 0 {
		steps = []string{"Review the finding in the cloud console and apply changes through your IaC workflow."}
	}
	guide := &RemediationGuide{
		Summary:        rec.Summary,
		Steps:          steps,
		RiskLevel:      rec.RiskLevel,
		Prerequisites:  []string{"Confirm change window and ownership for affected resources", "Ensure Terraform state backup exists before editing configuration"},
		ExpectedImpact: "Cost or utilization improvement per finding evidence; validate in non-production first",
		Validation:     []string{"Run terraform plan in a review environment", "Re-run cloudopt analyze after apply to confirm finding resolution"},
		Rollback:       []string{"Revert the Terraform commit and apply", "Restore previous attribute values documented in plan output"},
	}
	if len(links) > 0 && links[0].TFAddress != "" {
		guide.Steps = append([]string{
			fmt.Sprintf("Edit Terraform resource %q (module %q)", links[0].TFAddress, links[0].ModulePath),
		}, guide.Steps...)
	}
	return guide
}
