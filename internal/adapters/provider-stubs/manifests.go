package providerstubs

import (
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// AzureManifest is a placeholder manifest for skipped Azure implementation (not advertised).
func AzureManifest() ports.CapabilityManifest {
	return ports.CapabilityManifest{
		Provider:   types.ProviderAzure,
		Schema:     "2",
		Advertised: false,
		KnownLimitations: []string{
			"Azure adapters are not implemented in this delivery loop (see build-reports/skipped-steps.md).",
		},
		Inventory: []ports.CapabilityEntry{
			{
				ID:           "virtual_machines",
				Description:  "Azure VM inventory",
				Available:    false,
				Availability: ports.CapabilityUnsupported,
			},
		},
	}
}

// DigitalOceanManifest is a placeholder manifest for skipped DO implementation (not advertised).
func DigitalOceanManifest() ports.CapabilityManifest {
	return ports.CapabilityManifest{
		Provider:   types.ProviderDigitalOcean,
		Schema:     "2",
		Advertised: false,
		KnownLimitations: []string{
			"DigitalOcean adapters are not implemented in this delivery loop (see build-reports/skipped-steps.md).",
		},
		Inventory: []ports.CapabilityEntry{
			{
				ID:           "droplets",
				Description:  "Droplet inventory",
				Available:    false,
				Availability: ports.CapabilityUnsupported,
			},
		},
	}
}

// IncompleteFakeManifest is used by contract tests to ensure incomplete providers fail validation.
func IncompleteFakeManifest() ports.CapabilityManifest {
	return ports.CapabilityManifest{
		Provider:   types.Provider("incomplete-fake"),
		Schema:     "2",
		Advertised: false,
		Inventory: []ports.CapabilityEntry{
			{
				ID:          "partial_only",
				Description: "Missing availability metadata on purpose",
				Available:   false,
			},
		},
	}
}
