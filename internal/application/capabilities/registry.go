package capabilities

import (
	"fmt"

	awsbilling "github.com/kernul-io/cloudopt/internal/adapters/aws-billing"
	awsinventory "github.com/kernul-io/cloudopt/internal/adapters/aws-inventory"
	awsmetrics "github.com/kernul-io/cloudopt/internal/adapters/aws-metrics"
	awspricing "github.com/kernul-io/cloudopt/internal/adapters/aws-pricing"
	gcpbilling "github.com/kernul-io/cloudopt/internal/adapters/gcp-billing"
	gcpinventory "github.com/kernul-io/cloudopt/internal/adapters/gcp-inventory"
	gcpmetrics "github.com/kernul-io/cloudopt/internal/adapters/gcp-metrics"
	gcppricing "github.com/kernul-io/cloudopt/internal/adapters/gcp-pricing"
	providerstubs "github.com/kernul-io/cloudopt/internal/adapters/provider-stubs"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// AllProviderManifests returns merged capability manifests for contract tests and reporting.
func AllProviderManifests() ([]ports.CapabilityManifest, error) {
	aws, err := loadAWS()
	if err != nil {
		return nil, err
	}
	gcp, err := loadGCP()
	if err != nil {
		return nil, err
	}
	stubs := []ports.CapabilityManifest{
		providerstubs.AzureManifest(),
		providerstubs.DigitalOceanManifest(),
		providerstubs.IncompleteFakeManifest(),
	}
	return append([]ports.CapabilityManifest{aws, gcp}, stubs...), nil
}

func loadAWS() (ports.CapabilityManifest, error) {
	inv, err := awsinventory.LoadCapabilities()
	if err != nil {
		return ports.CapabilityManifest{}, fmt.Errorf("aws inventory capabilities: %w", err)
	}
	bill, err := awsbilling.LoadCapabilities()
	if err != nil {
		return ports.CapabilityManifest{}, fmt.Errorf("aws billing capabilities: %w", err)
	}
	met, err := awsmetrics.LoadCapabilities()
	if err != nil {
		return ports.CapabilityManifest{}, fmt.Errorf("aws metrics capabilities: %w", err)
	}
	pr, err := awspricing.LoadCapabilities()
	if err != nil {
		return ports.CapabilityManifest{}, fmt.Errorf("aws pricing capabilities: %w", err)
	}
	m, err := MergeManifests(inv, bill, met, pr)
	if err != nil {
		return ports.CapabilityManifest{}, err
	}
	m.Provider = types.ProviderAWS
	m.Advertised = true
	m.CommitmentModels = appendUniqueStrings(m.CommitmentModels,
		"savings_plans", "reserved_instances", "spot")
	m.KnownLimitations = appendUniqueStrings(m.KnownLimitations,
		"Resource-level Cost Explorer granularity is not available for all services.")
	return m, nil
}

func loadGCP() (ports.CapabilityManifest, error) {
	inv, err := gcpinventory.LoadCapabilities()
	if err != nil {
		return ports.CapabilityManifest{}, fmt.Errorf("gcp inventory capabilities: %w", err)
	}
	bill, err := gcpbilling.LoadCapabilities()
	if err != nil {
		return ports.CapabilityManifest{}, fmt.Errorf("gcp billing capabilities: %w", err)
	}
	met, err := gcpmetrics.LoadCapabilities()
	if err != nil {
		return ports.CapabilityManifest{}, fmt.Errorf("gcp metrics capabilities: %w", err)
	}
	pr, err := gcppricing.LoadCapabilities()
	if err != nil {
		return ports.CapabilityManifest{}, fmt.Errorf("gcp pricing capabilities: %w", err)
	}
	m, err := MergeManifests(inv, bill, met, pr)
	if err != nil {
		return ports.CapabilityManifest{}, err
	}
	m.Provider = types.ProviderGCP
	m.Advertised = true
	m.CommitmentModels = appendUniqueStrings(m.CommitmentModels,
		"committed_use_discount", "sustained_use_discount")
	m.KnownLimitations = appendUniqueStrings(m.KnownLimitations,
		"Live BigQuery billing export queries require offline fixtures or export configuration.")
	return m, nil
}

// ManifestForProvider returns the merged manifest for a known provider.
func ManifestForProvider(p types.Provider) (ports.CapabilityManifest, error) {
	all, err := AllProviderManifests()
	if err != nil {
		return ports.CapabilityManifest{}, err
	}
	for _, m := range all {
		if m.Provider == p {
			return m, nil
		}
	}
	return ports.CapabilityManifest{}, fmt.Errorf("unknown provider %q", p)
}
