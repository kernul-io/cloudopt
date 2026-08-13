package awsbilling

import (
	_ "embed"

	"gopkg.in/yaml.v3"

	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

//go:embed capabilities.yaml
var capabilitiesYAML []byte

// LoadCapabilities parses the embedded AWS billing capability manifest.
func LoadCapabilities() (ports.CapabilityManifest, error) {
	var raw struct {
		Provider        types.Provider `yaml:"provider"`
		Schema          string         `yaml:"schema"`
		Billing         []capEntry     `yaml:"billing"`
		SupportedChecks []string       `yaml:"supported_checks"`
	}
	if err := yaml.Unmarshal(capabilitiesYAML, &raw); err != nil {
		return ports.CapabilityManifest{}, err
	}
	m := ports.CapabilityManifest{
		Provider:        raw.Provider,
		Schema:          raw.Schema,
		SupportedChecks: raw.SupportedChecks,
	}
	for _, e := range raw.Billing {
		m.Billing = append(m.Billing, ports.CapabilityEntry{
			ID:          e.ID,
			Description: e.Description,
			Available:   e.Available,
			APIActions:  e.APIActions,
		})
	}
	return m, nil
}

type capEntry struct {
	ID          string   `yaml:"id"`
	Description string   `yaml:"description"`
	Available   bool     `yaml:"available"`
	APIActions  []string `yaml:"api_actions"`
}
