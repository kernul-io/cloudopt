package report

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Metadata is customer/project context configured outside the rule engine.
type Metadata struct {
	CustomerName string `yaml:"customer_name"`
	ProjectName  string `yaml:"project_name"`
	PreparedBy   string `yaml:"prepared_by"`
}

// LoadMetadata reads optional report metadata from configDir/report.yaml.
func LoadMetadata(configDir string) (Metadata, error) {
	path := filepath.Join(configDir, "report.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Metadata{}, nil
		}
		return Metadata{}, fmt.Errorf("read report metadata: %w", err)
	}
	var m Metadata
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Metadata{}, fmt.Errorf("parse report metadata: %w", err)
	}
	return m, nil
}
