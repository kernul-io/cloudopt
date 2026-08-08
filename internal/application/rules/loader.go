package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadManifest reads and validates a rule manifest from disk or embedded default.
func LoadManifest(path string, reg *Registry) (*Manifest, error) {
	data := DefaultRulesYAML
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read rules manifest: %w", err)
		}
		data = b
	}
	m, err := ParseManifest(data)
	if err != nil {
		return nil, err
	}
	if errs := m.Validate(reg); len(errs) > 0 {
		return nil, &ManifestValidationError{Errors: errs}
	}
	return m, nil
}

// LoadSuppressions reads optional suppression entries from a YAML file.
func LoadSuppressions(path string) ([]SuppressionEntry, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read suppressions: %w", err)
	}
	var list SuppressionList
	if err := yaml.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse suppressions: %w", err)
	}
	for i, s := range list.Suppressions {
		if strings.TrimSpace(s.Reason) == "" {
			return nil, fmt.Errorf("suppressions[%d]: reason is mandatory", i)
		}
	}
	return list.Suppressions, nil
}

// DefaultSuppressionsPath returns the conventional suppressions file location.
func DefaultSuppressionsPath(configDir string) string {
	return filepath.Join(configDir, "suppressions.yaml")
}

// ManifestValidationError aggregates manifest validation failures.
type ManifestValidationError struct {
	Errors []error
}

func (e *ManifestValidationError) Error() string {
	if len(e.Errors) == 0 {
		return "invalid rule manifest"
	}
	msg := e.Errors[0].Error()
	for _, err := range e.Errors[1:] {
		msg += "; " + err.Error()
	}
	return msg
}
