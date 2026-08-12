package awsmetrics

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

//go:embed capabilities.yaml
var capabilitiesYAML []byte

//go:embed iam-least-privilege.json
var iamPolicyJSON []byte

func IAMLeastPrivilegePolicy() []byte {
	out := make([]byte, len(iamPolicyJSON))
	copy(out, iamPolicyJSON)
	return out
}

func LoadCapabilities() (ports.CapabilityManifest, error) {
	var manifest ports.CapabilityManifest
	if err := yaml.Unmarshal(capabilitiesYAML, &manifest); err != nil {
		return ports.CapabilityManifest{}, fmt.Errorf("parse aws metrics capabilities: %w", err)
	}
	if manifest.Provider == "" {
		manifest.Provider = types.ProviderAWS
	}
	return manifest, nil
}

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
