package rules

import (
	"sort"
	"strings"

	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// SnapshotView indexes a collection snapshot for rule evaluation.
type SnapshotView struct {
	Snapshot *domain.CollectionSnapshot
	observed types.Timestamp
	catalog  *pricing.Catalog

	resourcesByID   map[types.ResourceID]domain.Resource
	resourcesByKind map[domain.ResourceKind][]domain.Resource
	costsByResource map[types.ResourceID][]domain.CostRecord
	attachedVolumes map[types.ResourceID]bool
}

// NewSnapshotView builds lookup indexes for evaluators.
func NewSnapshotView(snap *domain.CollectionSnapshot, catalog *pricing.Catalog) *SnapshotView {
	v := &SnapshotView{
		Snapshot:        snap,
		catalog:         catalog,
		resourcesByID:   make(map[types.ResourceID]domain.Resource),
		resourcesByKind: make(map[domain.ResourceKind][]domain.Resource),
		costsByResource: make(map[types.ResourceID][]domain.CostRecord),
		attachedVolumes: make(map[types.ResourceID]bool),
	}
	if snap.CompletedAt != nil {
		v.observed = *snap.CompletedAt
	} else {
		v.observed = snap.StartedAt
	}
	for _, r := range snap.Resources {
		v.resourcesByID[r.ID] = r
		v.resourcesByKind[r.Kind] = append(v.resourcesByKind[r.Kind], r)
	}
	for _, c := range snap.Costs {
		v.costsByResource[c.ResourceID] = append(v.costsByResource[c.ResourceID], c)
	}
	for _, rel := range snap.Relationships {
		if rel.Kind == domain.RelAttachedTo {
			v.attachedVolumes[rel.FromResourceID] = true
		}
	}
	return v
}

func (v *SnapshotView) ObservedAt() types.Timestamp {
	return v.observed
}

func (v *SnapshotView) ResourcesOfKind(kind domain.ResourceKind) []domain.Resource {
	return v.resourcesByKind[kind]
}

func (v *SnapshotView) Resource(id types.ResourceID) (domain.Resource, bool) {
	r, ok := v.resourcesByID[id]
	return r, ok
}

func (v *SnapshotView) CostsForResource(id types.ResourceID) []domain.CostRecord {
	return v.costsByResource[id]
}

func (v *SnapshotView) SumCostsMinor(id types.ResourceID) (types.Money, bool) {
	costs := v.costsByResource[id]
	if len(costs) == 0 {
		return types.Money{}, false
	}
	var sum types.Money
	var have bool
	for _, c := range costs {
		if !have {
			sum = c.Amount
			have = true
			continue
		}
		next, err := sum.Add(c.Amount)
		if err != nil {
			continue
		}
		sum = next
	}
	return sum, have
}

func (v *SnapshotView) VolumeAttached(id types.ResourceID) bool {
	return v.attachedVolumes[id]
}

func (v *SnapshotView) HasSignal(name string) bool {
	switch strings.ToLower(name) {
	case "resources":
		return len(v.Snapshot.Resources) > 0
	case "relationships":
		return len(v.Snapshot.Relationships) > 0
	case "costs":
		return len(v.Snapshot.Costs) > 0
	case "metrics":
		return len(v.Snapshot.Metrics) > 0 || len(v.Snapshot.UtilizationSignals) > 0
	case "pricing":
		return v.catalog != nil && !v.catalog.IsEmpty()
	default:
		return false
	}
}

func (v *SnapshotView) MissingSignals(required []string) []string {
	var missing []string
	for _, s := range required {
		if !v.HasSignal(s) {
			missing = append(missing, s)
		}
	}
	sort.Strings(missing)
	return missing
}

func resourceTagValue(r domain.Resource, key string) string {
	for _, t := range r.Tags {
		if t.Key == key && strings.TrimSpace(t.Value) != "" {
			return t.Value
		}
	}
	return ""
}

func stoppedState(state string) bool {
	s := strings.ToLower(strings.TrimSpace(state))
	switch s {
	case "stopped", "stopping", "suspended", "terminated":
		return true
	default:
		return false
	}
}

func gcpIdleAddressState(state string) bool {
	s := strings.ToUpper(strings.TrimSpace(state))
	return s == "RESERVED" || s == "RESERVE"
}

func isAWSResource(res domain.Resource) bool {
	if res.Attributes != nil && (res.Attributes["gcp_self_link"] != "" || res.Attributes["gcp_project"] != "") {
		return false
	}
	id := res.ProviderResourceID
	return strings.HasPrefix(id, "i-") || strings.HasPrefix(id, "vol-") || strings.HasPrefix(id, "db-")
}

func availableVolumeState(state string) bool {
	s := strings.ToLower(strings.TrimSpace(state))
	return s == "available" || s == "unattached"
}
