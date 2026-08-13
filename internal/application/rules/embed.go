package rules

import _ "embed"

// DefaultRulesYAML is the built-in rule manifest for offline analysis.
//
//go:embed default_rules.yaml
var DefaultRulesYAML []byte
