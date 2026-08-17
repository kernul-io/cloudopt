package awsinventory

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

//go:embed capabilities.yaml
var capabilitiesYAML []byte

//go:embed iam-least-privilege.json
var iamPolicyJSON []byte

// IAMLeastPrivilegePolicy returns documented read-only IAM policy JSON.
func IAMLeastPrivilegePolicy() []byte {
	out := make([]byte, len(iamPolicyJSON))
	copy(out, iamPolicyJSON)
	return out
}

// LoadCapabilities parses the embedded AWS capability manifest.
func LoadCapabilities() (ports.CapabilityManifest, error) {
	var manifest ports.CapabilityManifest
	if err := yaml.Unmarshal(capabilitiesYAML, &manifest); err != nil {
		return ports.CapabilityManifest{}, fmt.Errorf("parse aws capabilities: %w", err)
	}
	if manifest.Provider == "" {
		manifest.Provider = types.ProviderAWS
	}
	return manifest, nil
}

// CapabilityManifestJSON returns the manifest as JSON for tooling.
func CapabilityManifestJSON(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m, err := LoadCapabilities()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}
