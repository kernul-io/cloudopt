package capabilities

import (
	"fmt"
	"strings"

	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

const SchemaVersion = "2"

// MergeManifests combines partial manifests for one provider into a single contract document.
func MergeManifests(parts ...ports.CapabilityManifest) (ports.CapabilityManifest, error) {
	if len(parts) == 0 {
		return ports.CapabilityManifest{}, fmt.Errorf("at least one manifest part is required")
	}
	out := parts[0]
	normalizeManifest(&out)
	for i := 1; i < len(parts); i++ {
		p := parts[i]
		normalizeManifest(&p)
		if p.Provider != "" && out.Provider != "" && p.Provider != out.Provider {
			return ports.CapabilityManifest{}, fmt.Errorf("provider mismatch: %q vs %q", out.Provider, p.Provider)
		}
		if p.Provider != "" {
			out.Provider = p.Provider
		}
		if p.Schema != "" {
			out.Schema = p.Schema
		}
		if p.Advertised {
			out.Advertised = true
		}
		out.Inventory = append(out.Inventory, p.Inventory...)
		out.Billing = append(out.Billing, p.Billing...)
		out.Metrics = append(out.Metrics, p.Metrics...)
		out.Pricing = append(out.Pricing, p.Pricing...)
		out.SupportedChecks = appendUniqueStrings(out.SupportedChecks, p.SupportedChecks...)
		out.CommitmentModels = appendUniqueStrings(out.CommitmentModels, p.CommitmentModels...)
		out.KnownLimitations = appendUniqueStrings(out.KnownLimitations, p.KnownLimitations...)
	}
	if out.Schema == "" {
		out.Schema = SchemaVersion
	}
	return out, nil
}

func normalizeManifest(m *ports.CapabilityManifest) {
	normalizeEntries(&m.Inventory)
	normalizeEntries(&m.Billing)
	normalizeEntries(&m.Metrics)
	normalizeEntries(&m.Pricing)
}

func normalizeEntries(entries *[]ports.CapabilityEntry) {
	for i := range *entries {
		e := &(*entries)[i]
		if e.Availability == "" {
			if e.Available {
				e.Availability = ports.CapabilitySupported
			} else {
				e.Availability = ports.CapabilityUnsupported
			}
		}
	}
}

func appendUniqueStrings(base []string, add ...string) []string {
	seen := make(map[string]struct{}, len(base))
	for _, s := range base {
		seen[s] = struct{}{}
	}
	for _, s := range add {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		base = append(base, s)
	}
	return base
}

// ValidateManifest checks versioned provider capability contracts.
func ValidateManifest(m ports.CapabilityManifest) []error {
	var errs []error
	if m.Provider == "" {
		errs = append(errs, fmt.Errorf("provider is required"))
	}
	if strings.TrimSpace(m.Schema) == "" {
		errs = append(errs, fmt.Errorf("schema is required"))
	}
	if m.Advertised && len(m.Inventory) == 0 {
		errs = append(errs, fmt.Errorf("advertised provider %q must declare inventory capabilities", m.Provider))
	}
	validateEntries := func(group string, entries []ports.CapabilityEntry) {
		seen := make(map[string]struct{})
		for _, e := range entries {
			if e.ID == "" {
				errs = append(errs, fmt.Errorf("%s: capability id is required", group))
				continue
			}
			if _, dup := seen[e.ID]; dup {
				errs = append(errs, fmt.Errorf("%s: duplicate capability id %q", group, e.ID))
			}
			seen[e.ID] = struct{}{}
			if e.Availability == "" {
				errs = append(errs, fmt.Errorf("%s.%s: availability is required", group, e.ID))
			}
		}
	}
	validateEntries("inventory", m.Inventory)
	validateEntries("billing", m.Billing)
	validateEntries("metrics", m.Metrics)
	validateEntries("pricing", m.Pricing)
	return errs
}

// CapabilityRef names a capability as dimension.id (e.g. metrics.cloudwatch_ec2).
func CapabilityRef(dimension, id string) string {
	return dimension + "." + id
}

// ProviderSupports returns true when the manifest declares a supported capability.
func ProviderSupports(m ports.CapabilityManifest, ref string) bool {
	dim, id, ok := strings.Cut(ref, ".")
	if !ok {
		return false
	}
	var entries []ports.CapabilityEntry
	switch dim {
	case "inventory":
		entries = m.Inventory
	case "billing":
		entries = m.Billing
	case "metrics":
		entries = m.Metrics
	case "pricing":
		entries = m.Pricing
	default:
		return false
	}
	for _, e := range entries {
		if e.ID == id && e.Availability == ports.CapabilitySupported {
			return true
		}
	}
	return false
}

// AdvertisedProviders lists providers that pass the contract suite for this release.
func AdvertisedProviders(all []ports.CapabilityManifest) []types.Provider {
	var out []types.Provider
	for _, m := range all {
		if m.Advertised && len(ValidateManifest(m)) == 0 {
			out = append(out, m.Provider)
		}
	}
	return out
}
