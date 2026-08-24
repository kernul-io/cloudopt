package rules

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest is a versioned rule set loaded from YAML.
type Manifest struct {
	RulesetVersion string     `yaml:"ruleset_version"`
	Rules          []RuleSpec `yaml:"rules"`
}

// RuleSpec configures a registered evaluator (no arbitrary code execution).
type RuleSpec struct {
	ID                   string            `yaml:"id"`
	Version              string            `yaml:"version"`
	Title                string            `yaml:"title"`
	Category             string            `yaml:"category"`
	Severity             string            `yaml:"severity"`
	Evaluator            string            `yaml:"evaluator"`
	Enabled              bool              `yaml:"enabled"`
	Applicability        ApplicabilitySpec `yaml:"applicability"`
	Thresholds           map[string]string `yaml:"thresholds"`
	RequiredSignals      []string          `yaml:"required_signals"`
	RequiredCapabilities []string          `yaml:"required_capabilities"`
	Providers            []string          `yaml:"providers"`
	Remediation          string            `yaml:"remediation"`
}

// ApplicabilitySpec limits which resources a rule considers.
type ApplicabilitySpec struct {
	ResourceKinds []string `yaml:"resource_kinds"`
}

// ParseManifest unmarshals YAML into a Manifest.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse rule manifest: %w", err)
	}
	return &m, nil
}

// Validate checks the manifest and returns all validation errors.
func (m *Manifest) Validate(reg *Registry) []error {
	var errs []error
	if strings.TrimSpace(m.RulesetVersion) == "" {
		errs = append(errs, fmt.Errorf("ruleset_version is required"))
	}
	if len(m.Rules) == 0 {
		errs = append(errs, fmt.Errorf("at least one rule is required"))
	}
	seen := make(map[string]struct{})
	for i, rule := range m.Rules {
		prefix := fmt.Sprintf("rules[%d]", i)
		if rule.ID == "" {
			errs = append(errs, fmt.Errorf("%s: id is required", prefix))
		} else if _, dup := seen[rule.ID]; dup {
			errs = append(errs, fmt.Errorf("%s: duplicate rule id %q", prefix, rule.ID))
		} else {
			seen[rule.ID] = struct{}{}
		}
		if rule.Version == "" {
			errs = append(errs, fmt.Errorf("%s (%s): version is required", prefix, rule.ID))
		}
		if rule.Title == "" {
			errs = append(errs, fmt.Errorf("%s (%s): title is required", prefix, rule.ID))
		}
		if rule.Category == "" {
			errs = append(errs, fmt.Errorf("%s (%s): category is required", prefix, rule.ID))
		}
		if rule.Severity == "" {
			errs = append(errs, fmt.Errorf("%s (%s): severity is required", prefix, rule.ID))
		}
		if rule.Evaluator == "" {
			errs = append(errs, fmt.Errorf("%s (%s): evaluator is required", prefix, rule.ID))
		} else if reg != nil && !reg.Has(rule.Evaluator) {
			errs = append(errs, fmt.Errorf("%s (%s): unknown evaluator %q", prefix, rule.ID, rule.Evaluator))
		}
		if len(rule.RequiredSignals) == 0 {
			errs = append(errs, fmt.Errorf("%s (%s): required_signals must not be empty", prefix, rule.ID))
		}
	}
	return errs
}

func (r RuleSpec) thresholdInt(key string, fallback int64) (int64, error) {
	raw, ok := r.Thresholds[key]
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	var v int64
	_, err := fmt.Sscan(raw, &v)
	if err != nil {
		return 0, fmt.Errorf("threshold %q: invalid integer %q", key, raw)
	}
	return v, nil
}

func (r RuleSpec) thresholdStringList(key string, fallback []string) []string {
	raw, ok := r.Thresholds[key]
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
