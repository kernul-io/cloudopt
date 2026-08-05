package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kernul-io/cloudopt/internal/adapters/config"
)

func TestLoad_precedenceFlagsOverEnvOverFile(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	fileBody := `workspace_dir: "/from/file"
log_format: "json"
log_level: "warn"
`
	if err := os.WriteFile(configPath, []byte(fileBody), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("COA_LOG_LEVEL", "debug")
	t.Setenv("COA_LOG_FORMAT", "text")

	settings, err := config.Load(config.Overrides{
		ConfigFile:   configPath,
		WorkspaceDir: "/from/flag",
		LogFormat:    "json",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if settings.WorkspaceDir != "/from/flag" {
		t.Errorf("workspace_dir = %q, want flag value", settings.WorkspaceDir)
	}
	if settings.LogFormat != "json" {
		t.Errorf("log_format = %q, want flag json", settings.LogFormat)
	}
	if settings.LogLevel != "debug" {
		t.Errorf("log_level = %q, want env debug over file warn", settings.LogLevel)
	}
}

func TestValidate_fieldErrors(t *testing.T) {
	_, err := config.Load(config.Overrides{
		WorkspaceDir: t.TempDir(),
		LogFormat:    "yaml",
		LogLevel:     "trace",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ve *config.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if len(ve.Fields) < 2 {
		t.Fatalf("expected multiple field errors, got %v", ve.Fields)
	}
	t.Setenv("COA_LOG_FORMAT", "super-secret-format")
	_, err2 := config.Load(config.Overrides{LogFormat: "yaml"})
	if err2 == nil {
		t.Fatal("expected validation error")
	}
	msg := err2.Error()
	if strings.Contains(msg, "super-secret") {
		t.Errorf("validation must not expose env values: %s", msg)
	}
}

func TestLoad_missingConfigFileIsOK(t *testing.T) {
	dir := t.TempDir()
	settings, err := config.Load(config.Overrides{WorkspaceDir: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.WorkspaceDir != dir {
		t.Errorf("workspace_dir = %q", settings.WorkspaceDir)
	}
}
