package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	EnvPrefix = "COA_"

	defaultConfigRel = ".cloudopt/config.yaml"
)

// Settings holds local workspace paths and runtime preferences.
type Settings struct {
	WorkspaceDir      string `yaml:"workspace_dir"`
	ConfigDir         string `yaml:"config_dir"`
	DataDir           string `yaml:"data_dir"`
	ReportsDir        string `yaml:"reports_dir"`
	TempDir           string `yaml:"temp_dir"`
	LogFormat         string `yaml:"log_format"` // "text" or "json"
	LogLevel          string `yaml:"log_level"`
	RulesManifestPath string `yaml:"rules_manifest_path"`
	SuppressionsPath  string `yaml:"suppressions_path"`

	RetentionCompleteSnapshots int    `yaml:"retention_complete_snapshots"`
	IncompleteSnapshotTTLHours int    `yaml:"incomplete_snapshot_ttl_hours"`
	MetadataEncryptionEnv      string `yaml:"metadata_encryption_env"`
	AuditLogPath               string `yaml:"audit_log_path"`
	TelemetryEnabled           bool   `yaml:"telemetry_enabled"`
}

// Overrides from flags and environment (higher precedence than file).
type Overrides struct {
	WorkspaceDir string
	ConfigDir    string
	DataDir      string
	ReportsDir   string
	TempDir      string
	LogFormat    string
	LogLevel     string
	ConfigFile   string
}

// Load merges overrides, environment variables, and optional config file.
// Precedence: flags (Overrides) > environment > config file > defaults.
func Load(over Overrides) (Settings, error) {
	base, err := defaultSettings(over.WorkspaceDir)
	if err != nil {
		return Settings{}, err
	}

	configPath := over.ConfigFile
	if configPath == "" {
		configPath = envString("CONFIG_FILE", "")
	}
	if configPath == "" {
		configPath = filepath.Join(base.ConfigDir, "config.yaml")
	}

	if data, err := os.ReadFile(configPath); err == nil {
		var fromFile Settings
		if err := yaml.Unmarshal(data, &fromFile); err != nil {
			return Settings{}, fmt.Errorf("parse config file %q: %w", configPath, err)
		}
		mergeSettings(&base, fromFile)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Settings{}, fmt.Errorf("read config file %q: %w", configPath, err)
	}

	applyEnv(&base)
	applyOverrides(&base, over)

	if err := base.Validate(); err != nil {
		return Settings{}, err
	}
	return base, nil
}

func defaultSettings(workspace string) (Settings, error) {
	if workspace == "" {
		var err error
		workspace, err = os.UserHomeDir()
		if err != nil {
			return Settings{}, fmt.Errorf("resolve home directory: %w", err)
		}
		workspace = filepath.Join(workspace, ".cloudopt")
	}

	return Settings{
		WorkspaceDir: workspace,
		ConfigDir:    filepath.Join(workspace, "config"),
		DataDir:      filepath.Join(workspace, "data"),
		ReportsDir:   filepath.Join(workspace, "reports"),
		TempDir:      filepath.Join(workspace, "tmp"),
		LogFormat:    "text",
		LogLevel:     "info",

		RetentionCompleteSnapshots: 5,
		IncompleteSnapshotTTLHours: 24,
		MetadataEncryptionEnv:      "COA_METADATA_KEY",
		TelemetryEnabled:           false,
	}, nil
}

func applyEnv(s *Settings) {
	if v := envString("WORKSPACE_DIR", ""); v != "" {
		s.WorkspaceDir = v
	}
	if v := envString("CONFIG_DIR", ""); v != "" {
		s.ConfigDir = v
	}
	if v := envString("DATA_DIR", ""); v != "" {
		s.DataDir = v
	}
	if v := envString("REPORTS_DIR", ""); v != "" {
		s.ReportsDir = v
	}
	if v := envString("TEMP_DIR", ""); v != "" {
		s.TempDir = v
	}
	if v := envString("LOG_FORMAT", ""); v != "" {
		s.LogFormat = v
	}
	if v := envString("LOG_LEVEL", ""); v != "" {
		s.LogLevel = v
	}
}

func applyOverrides(s *Settings, over Overrides) {
	if over.WorkspaceDir != "" {
		s.WorkspaceDir = over.WorkspaceDir
	}
	if over.ConfigDir != "" {
		s.ConfigDir = over.ConfigDir
	}
	if over.DataDir != "" {
		s.DataDir = over.DataDir
	}
	if over.ReportsDir != "" {
		s.ReportsDir = over.ReportsDir
	}
	if over.TempDir != "" {
		s.TempDir = over.TempDir
	}
	if over.LogFormat != "" {
		s.LogFormat = over.LogFormat
	}
	if over.LogLevel != "" {
		s.LogLevel = over.LogLevel
	}
}

func mergeSettings(dst *Settings, src Settings) {
	if src.WorkspaceDir != "" {
		dst.WorkspaceDir = src.WorkspaceDir
	}
	if src.ConfigDir != "" {
		dst.ConfigDir = src.ConfigDir
	}
	if src.DataDir != "" {
		dst.DataDir = src.DataDir
	}
	if src.ReportsDir != "" {
		dst.ReportsDir = src.ReportsDir
	}
	if src.TempDir != "" {
		dst.TempDir = src.TempDir
	}
	if src.LogFormat != "" {
		dst.LogFormat = src.LogFormat
	}
	if src.LogLevel != "" {
		dst.LogLevel = src.LogLevel
	}
}

// Validate returns field-level errors without secret values.
func (s Settings) Validate() error {
	var msgs []string
	if s.WorkspaceDir == "" {
		msgs = append(msgs, "workspace_dir must not be empty")
	}
	if s.ConfigDir == "" {
		msgs = append(msgs, "config_dir must not be empty")
	}
	if s.DataDir == "" {
		msgs = append(msgs, "data_dir must not be empty")
	}
	if s.ReportsDir == "" {
		msgs = append(msgs, "reports_dir must not be empty")
	}
	if s.TempDir == "" {
		msgs = append(msgs, "temp_dir must not be empty")
	}
	if s.LogFormat != "text" && s.LogFormat != "json" {
		msgs = append(msgs, "log_format must be \"text\" or \"json\"")
	}
	switch strings.ToLower(s.LogLevel) {
	case "debug", "info", "warn", "error":
	default:
		msgs = append(msgs, "log_level must be debug, info, warn, or error")
	}
	if len(msgs) > 0 {
		return &ValidationError{Fields: msgs}
	}
	return nil
}

// ValidationError lists invalid configuration fields.
type ValidationError struct {
	Fields []string
}

func (e *ValidationError) Error() string {
	return "invalid configuration: " + strings.Join(e.Fields, "; ")
}

func envString(key, fallback string) string {
	if v, ok := os.LookupEnv(EnvPrefix + key); ok && v != "" {
		return v
	}
	return fallback
}

// IncompleteSnapshotTTL returns the configured TTL for abandoned in-progress snapshots.
func (s Settings) IncompleteSnapshotTTL() time.Duration {
	h := s.IncompleteSnapshotTTLHours
	if h <= 0 {
		h = 24
	}
	return time.Duration(h) * time.Hour
}

// DefaultConfigRel is the relative path written by init under the workspace.
func DefaultConfigRel() string {
	return defaultConfigRel
}

// ExampleYAML returns the default config file contents for init.
func ExampleYAML(workspace string) string {
	s, _ := defaultSettings(workspace)
	return fmt.Sprintf(`# Cloud Optimization Analyzer local configuration
workspace_dir: %q
config_dir: %q
data_dir: %q
reports_dir: %q
temp_dir: %q
log_format: "text"
log_level: "info"
retention_complete_snapshots: 5
incomplete_snapshot_ttl_hours: 24
metadata_encryption_env: "COA_METADATA_KEY"
telemetry_enabled: false
`, s.WorkspaceDir, s.ConfigDir, s.DataDir, s.ReportsDir, s.TempDir)
}
