package capabilities

import (
	"sort"

	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// MatrixRow compares one capability dimension across providers.
type MatrixRow struct {
	Dimension   string            `json:"dimension"`
	Capability  string            `json:"capability"`
	Description string            `json:"description,omitempty"`
	ByProvider  map[string]string `json:"by_provider"`
}

// Matrix is the cross-provider capability comparison for CLI and reports.
type Matrix struct {
	SchemaVersion string      `json:"schema_version"`
	Providers     []string    `json:"providers"`
	Rows          []MatrixRow `json:"rows"`
}

// BuildMatrix compares capability entries across provider manifests.
func BuildMatrix(manifests []ports.CapabilityManifest, providerFilter []types.Provider) *Matrix {
	filter := make(map[types.Provider]struct{})
	for _, p := range providerFilter {
		filter[p] = struct{}{}
	}
	var selected []ports.CapabilityManifest
	for _, m := range manifests {
		if len(filter) > 0 {
			if _, ok := filter[m.Provider]; !ok {
				continue
			}
		}
		selected = append(selected, m)
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Provider < selected[j].Provider })

	m := &Matrix{SchemaVersion: SchemaVersion}
	for _, man := range selected {
		m.Providers = append(m.Providers, string(man.Provider))
	}
	type key struct {
		dim, id string
	}
	rows := map[key]*MatrixRow{}
	add := func(dim string, entries []ports.CapabilityEntry) {
		for _, e := range entries {
			k := key{dim, e.ID}
			row, ok := rows[k]
			if !ok {
				row = &MatrixRow{
					Dimension:   dim,
					Capability:  e.ID,
					Description: e.Description,
					ByProvider:  map[string]string{},
				}
				rows[k] = row
			}
			for _, man := range selected {
				row.ByProvider[string(man.Provider)] = string(ports.CapabilityUnsupported)
			}
		}
	}
	for _, man := range selected {
		add("inventory", man.Inventory)
		add("billing", man.Billing)
		add("metrics", man.Metrics)
		add("pricing", man.Pricing)
	}
	for _, man := range selected {
		fill := func(dim string, entries []ports.CapabilityEntry) {
			for _, e := range entries {
				k := key{dim, e.ID}
				if row, ok := rows[k]; ok {
					row.ByProvider[string(man.Provider)] = string(e.Availability)
				}
			}
		}
		fill("inventory", man.Inventory)
		fill("billing", man.Billing)
		fill("metrics", man.Metrics)
		fill("pricing", man.Pricing)
	}
	for _, row := range rows {
		m.Rows = append(m.Rows, *row)
	}
	sort.Slice(m.Rows, func(i, j int) bool {
		if m.Rows[i].Dimension != m.Rows[j].Dimension {
			return m.Rows[i].Dimension < m.Rows[j].Dimension
		}
		return m.Rows[i].Capability < m.Rows[j].Capability
	})
	return m
}

// MatrixForScope limits the matrix to providers present in an engagement or snapshot.
func MatrixForScope(manifests []ports.CapabilityManifest, scope []types.Provider) *Matrix {
	if len(scope) == 0 {
		return BuildMatrix(manifests, nil)
	}
	return BuildMatrix(manifests, scope)
}
