package engagement

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kernul-io/cloudopt/internal/application/capabilities"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// MergeSnapshots combines provider snapshots into one engagement snapshot without currency conversion.
func MergeSnapshots(name, externalKey string, members []*domain.CollectionSnapshot) (*domain.CollectionSnapshot, error) {
	if len(members) == 0 {
		return nil, fmt.Errorf("at least one member snapshot is required")
	}
	observed := members[0].StartedAt
	var completed *types.Timestamp
	if members[0].CompletedAt != nil {
		c := *members[0].CompletedAt
		completed = &c
	}

	merged := &domain.CollectionSnapshot{
		AccountID:     types.AccountID("engagement-" + slug(name)),
		Provider:      types.ProviderMulti,
		Status:        domain.SnapshotComplete,
		SchemaVersion: 1,
		ExternalKey:   externalKey,
		StartedAt:     observed,
		CompletedAt:   completed,
		Account: domain.Account{
			ID:          types.AccountID("engagement-" + slug(name)),
			Provider:    types.ProviderMulti,
			DisplayName: name,
		},
		Engagement: &domain.EngagementMeta{Name: name},
	}

	regionKey := map[string]types.RegionID{}
	resourceKey := map[string]types.ResourceID{}

	for _, snap := range members {
		if snap == nil {
			return nil, fmt.Errorf("member snapshot is nil")
		}
		if snap.Status != domain.SnapshotComplete && snap.Status != domain.SnapshotPartial {
			return nil, fmt.Errorf("member snapshot %q is not complete", snap.ID)
		}
		merged.Engagement.Members = append(merged.Engagement.Members, domain.EngagementMember{
			Provider:          snap.Provider,
			AccountID:         snap.AccountID,
			DisplayName:       snap.Account.DisplayName,
			DefaultCurrency:   snap.Account.DefaultCurrency,
			SourceExternalKey: snap.ExternalKey,
		})

		prefix := string(snap.Provider)
		for _, reg := range snap.Regions {
			key := prefix + ":" + string(reg.ID)
			if _, ok := regionKey[key]; ok {
				continue
			}
			newID := types.RegionID(prefix + "-" + string(reg.ID))
			regionKey[key] = newID
			merged.Regions = append(merged.Regions, domain.Region{
				ID:               newID,
				ProviderRegionID: reg.ProviderRegionID,
				DisplayName:      reg.DisplayName,
				Provenance:       reg.Provenance,
			})
		}

		for _, res := range snap.Resources {
			oldID := string(res.ID)
			key := prefix + ":" + oldID
			newID := types.ResourceID(prefix + "-" + oldID)
			resourceKey[key] = newID
			attrs := map[string]string{}
			for k, v := range res.Attributes {
				attrs[k] = v
			}
			attrs["cloud_provider"] = string(snap.Provider)
			regID := regionKey[prefix+":"+string(res.RegionID)]
			merged.Resources = append(merged.Resources, domain.Resource{
				ID:                 newID,
				Kind:               res.Kind,
				ProviderResourceID: res.ProviderResourceID,
				AccountID:          merged.AccountID,
				RegionID:           regID,
				Name:               res.Name,
				State:              res.State,
				Tags:               append([]domain.Tag(nil), res.Tags...),
				Attributes:         attrs,
				Provenance:         res.Provenance,
			})
		}
	}

	for _, snap := range members {
		prefix := string(snap.Provider)
		for _, rel := range snap.Relationships {
			fromKey := prefix + ":" + string(rel.FromResourceID)
			toKey := prefix + ":" + string(rel.ToResourceID)
			fromID, ok := resourceKey[fromKey]
			if !ok {
				continue
			}
			toID := resourceKey[toKey]
			merged.Relationships = append(merged.Relationships, domain.Relationship{
				Kind:                 rel.Kind,
				FromResourceID:       fromID,
				ToResourceID:         toID,
				ToProviderResourceID: rel.ToProviderResourceID,
				TargetMissing:        rel.TargetMissing,
				Provenance:           rel.Provenance,
			})
		}
		for _, c := range snap.Costs {
			resID := c.ResourceID
			if resID != domain.UnattributedResourceID {
				key := prefix + ":" + string(resID)
				resID = resourceKey[key]
			}
			merged.Costs = append(merged.Costs, domain.CostRecord{
				ResourceID:  resID,
				Service:     c.Service,
				Amount:      c.Amount,
				Basis:       c.Basis,
				ChargeKind:  c.ChargeKind,
				Granularity: c.Granularity,
				PeriodStart: c.PeriodStart,
				PeriodEnd:   c.PeriodEnd,
				RegionID:    c.RegionID,
				Attribution: c.Attribution,
				Provenance:  c.Provenance,
			})
		}
		merged.Metrics = append(merged.Metrics, remapMetrics(snap.Metrics, prefix, resourceKey)...)
		merged.UtilizationSignals = append(merged.UtilizationSignals, remapSignals(snap.UtilizationSignals, prefix, resourceKey)...)
		if snap.MetricsMeta != nil {
			merged.MetricsMeta = snap.MetricsMeta
		}
		merged.Coverage.Services = append(merged.Coverage.Services, snap.Coverage.Services...)
	}

	sort.Slice(merged.Engagement.Members, func(i, j int) bool {
		return merged.Engagement.Members[i].Provider < merged.Engagement.Members[j].Provider
	})
	return merged, nil
}

func remapMetrics(series []domain.MetricSeries, prefix string, resourceKey map[string]types.ResourceID) []domain.MetricSeries {
	out := make([]domain.MetricSeries, 0, len(series))
	for _, s := range series {
		key := prefix + ":" + string(s.ResourceID)
		rid, ok := resourceKey[key]
		if !ok {
			continue
		}
		s.ResourceID = rid
		out = append(out, s)
	}
	return out
}

func remapSignals(signals []domain.UtilizationSignal, prefix string, resourceKey map[string]types.ResourceID) []domain.UtilizationSignal {
	out := make([]domain.UtilizationSignal, 0, len(signals))
	for _, s := range signals {
		key := prefix + ":" + string(s.ResourceID)
		rid, ok := resourceKey[key]
		if !ok {
			continue
		}
		s.ResourceID = rid
		out = append(out, s)
	}
	return out
}

func slug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "default"
	}
	return out
}

// PortfolioSummary aggregates spend and resource counts without cross-currency conversion.
type PortfolioSummary struct {
	ByProvider     map[string]int64 `json:"spend_by_provider_minor"`
	ByCategory     map[string]int64 `json:"spend_by_category_minor"`
	ByOwner        map[string]int64 `json:"spend_by_owner_minor"`
	ByProjectTag   map[string]int64 `json:"spend_by_project_minor"`
	ResourceByProv map[string]int   `json:"resource_count_by_provider"`
}

// BuildPortfolio summarizes merged or single snapshots preserving native currencies in separate buckets.
func BuildPortfolio(snap *domain.CollectionSnapshot) PortfolioSummary {
	out := PortfolioSummary{
		ByProvider:     map[string]int64{},
		ByCategory:     map[string]int64{},
		ByOwner:        map[string]int64{},
		ByProjectTag:   map[string]int64{},
		ResourceByProv: map[string]int{},
	}
	if snap == nil {
		return out
	}
	owners := map[types.ResourceID]string{}
	projects := map[types.ResourceID]string{}
	for _, res := range snap.Resources {
		p := string(capabilities.ResourceProvider(res))
		if p == "" {
			p = string(snap.Provider)
		}
		out.ResourceByProv[p]++
		for _, tag := range res.Tags {
			if tag.Key == "Owner" && tag.Value != "" {
				owners[res.ID] = tag.Value
			}
			if (tag.Key == "Project" || tag.Key == "project") && tag.Value != "" {
				projects[res.ID] = tag.Value
			}
		}
	}
	for _, c := range snap.Costs {
		if c.ResourceID == domain.UnattributedResourceID {
			continue
		}
		res, ok := findResource(snap, c.ResourceID)
		prov := string(snap.Provider)
		if ok {
			if p := capabilities.ResourceProvider(res); p != "" {
				prov = string(p)
			}
		}
		key := prov + ":" + c.Amount.Currency
		out.ByProvider[key] += c.Amount.AmountMinor
		cat := capabilities.ServiceCategory(types.Provider(prov), c.Service)
		catKey := cat + ":" + c.Amount.Currency
		out.ByCategory[catKey] += c.Amount.AmountMinor
		owner := owners[c.ResourceID]
		if owner == "" {
			owner = "unknown"
		}
		out.ByOwner[owner+":"+c.Amount.Currency] += c.Amount.AmountMinor
		proj := projects[c.ResourceID]
		if proj == "" {
			proj = "unknown"
		}
		out.ByProjectTag[proj+":"+c.Amount.Currency] += c.Amount.AmountMinor
	}
	return out
}

func findResource(snap *domain.CollectionSnapshot, id types.ResourceID) (domain.Resource, bool) {
	for _, r := range snap.Resources {
		if r.ID == id {
			return r, true
		}
	}
	return domain.Resource{}, false
}
