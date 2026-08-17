package domain

import (
	"fmt"
	"reflect"
	"sort"
)

// CompareSnapshots reports field-level differences between two snapshots (for tests and validation).
func CompareSnapshots(a, b *CollectionSnapshot) []string {
	if a == nil || b == nil {
		return []string{"nil snapshot"}
	}
	var diffs []string
	if a.ID != b.ID {
		diffs = append(diffs, fmt.Sprintf("id: %q vs %q", a.ID, b.ID))
	}
	if a.Status != b.Status {
		diffs = append(diffs, fmt.Sprintf("status: %q vs %q", a.Status, b.Status))
	}
	if a.SchemaVersion != b.SchemaVersion {
		diffs = append(diffs, fmt.Sprintf("schema_version: %d vs %d", a.SchemaVersion, b.SchemaVersion))
	}
	if a.Account.ProviderAccountID != b.Account.ProviderAccountID {
		diffs = append(diffs, "account.provider_account_id mismatch")
	}
	if len(a.Regions) != len(b.Regions) {
		diffs = append(diffs, fmt.Sprintf("regions count: %d vs %d", len(a.Regions), len(b.Regions)))
	}
	if len(a.Resources) != len(b.Resources) {
		diffs = append(diffs, fmt.Sprintf("resources count: %d vs %d", len(a.Resources), len(b.Resources)))
	}
	if len(a.Relationships) != len(b.Relationships) {
		diffs = append(diffs, fmt.Sprintf("relationships count: %d vs %d", len(a.Relationships), len(b.Relationships)))
	}
	if len(a.Costs) != len(b.Costs) {
		diffs = append(diffs, fmt.Sprintf("costs count: %d vs %d", len(a.Costs), len(b.Costs)))
	}
	if len(a.Metrics) != len(b.Metrics) {
		diffs = append(diffs, fmt.Sprintf("metrics count: %d vs %d", len(a.Metrics), len(b.Metrics)))
	}

	sortResources := func(r []Resource) {
		sort.Slice(r, func(i, j int) bool { return r[i].ID < r[j].ID })
	}
	ra, rb := append([]Resource{}, a.Resources...), append([]Resource{}, b.Resources...)
	sortResources(ra)
	sortResources(rb)
	for i := range ra {
		if i >= len(rb) {
			break
		}
		if ra[i].ID != rb[i].ID || ra[i].ProviderResourceID != rb[i].ProviderResourceID {
			diffs = append(diffs, fmt.Sprintf("resource[%d] identity mismatch", i))
		}
		if ra[i].State != rb[i].State {
			diffs = append(diffs, fmt.Sprintf("resource %q state: %q vs %q", ra[i].ID, ra[i].State, rb[i].State))
		}
		if !reflect.DeepEqual(ra[i].Tags, rb[i].Tags) {
			diffs = append(diffs, fmt.Sprintf("resource %q tags mismatch", ra[i].ID))
		}
	}

	for i := range a.Relationships {
		if i >= len(b.Relationships) {
			break
		}
		ar, br := a.Relationships[i], b.Relationships[i]
		if ar.Kind != br.Kind || ar.FromResourceID != br.FromResourceID || ar.TargetMissing != br.TargetMissing {
			diffs = append(diffs, fmt.Sprintf("relationship[%d] mismatch", i))
		}
	}

	return diffs
}
