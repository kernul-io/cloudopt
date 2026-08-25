package terraforminput

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/kernul-io/cloudopt/internal/application/terraform"
)

// LoadMappings reads explicit user correlation mappings (YAML).
func LoadMappings(path string) ([]terraform.UserMapping, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mappings: %w", err)
	}
	var doc struct {
		Mappings []terraform.UserMapping `yaml:"mappings"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse mappings yaml: %w", err)
	}
	return doc.Mappings, nil
}
