package report

import (
	"fmt"
	"sort"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
)

// AliasMap assigns stable aliases when redacting identifiers.
type AliasMap struct {
	AccountAlias  string
	ResourceAlias map[types.ResourceID]string
	RedactEnabled bool
}

// NewAliasMap builds aliases from snapshot resources (deterministic ordering).
func NewAliasMap(snap *domain.CollectionSnapshot, redact bool) *AliasMap {
	m := &AliasMap{
		ResourceAlias: make(map[types.ResourceID]string),
		RedactEnabled: redact,
	}
	if snap == nil {
		return m
	}
	if redact {
		m.AccountAlias = "Account-1"
	} else {
		m.AccountAlias = snap.Account.DisplayName
		if m.AccountAlias == "" {
			m.AccountAlias = string(snap.AccountID)
		}
	}

	ids := make([]types.ResourceID, 0, len(snap.Resources))
	for _, r := range snap.Resources {
		ids = append(ids, r.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for i, id := range ids {
		if redact {
			m.ResourceAlias[id] = formatResourceAlias(i + 1)
		} else {
			res := resourceByID(snap, id)
			if res.Name != "" {
				m.ResourceAlias[id] = res.Name
			} else {
				m.ResourceAlias[id] = string(id)
			}
		}
	}
	return m
}

func formatResourceAlias(n int) string {
	return "Resource-" + fmt.Sprintf("%03d", n)
}

func resourceByID(snap *domain.CollectionSnapshot, id types.ResourceID) domain.Resource {
	for _, r := range snap.Resources {
		if r.ID == id {
			return r
		}
	}
	return domain.Resource{ID: id}
}

func (m *AliasMap) Resource(id types.ResourceID) string {
	if m == nil {
		return string(id)
	}
	if alias, ok := m.ResourceAlias[id]; ok {
		return alias
	}
	return string(id)
}

func (m *AliasMap) RedactDetail(detail map[string]string) map[string]string {
	if !m.RedactEnabled || len(detail) == 0 {
		out := make(map[string]string, len(detail))
		for k, v := range detail {
			out[k] = v
		}
		return out
	}
	out := make(map[string]string, len(detail))
	for k, v := range detail {
		out[k] = v
	}
	return out
}
