package api_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/adapters/config"
	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestWorkspaceBackupRestore(t *testing.T) {
	dir := t.TempDir()
	settings := config.Settings{
		WorkspaceDir: dir,
		ConfigDir:    filepath.Join(dir, "config"),
		DataDir:      filepath.Join(dir, "data"),
		ReportsDir:   filepath.Join(dir, "reports"),
		TempDir:      filepath.Join(dir, "tmp"),
		LogFormat:    "text",
		LogLevel:     "info",
	}
	rt := api.NewRuntime(settings)
	require.NoError(t, rt.Init(context.Background()))

	backupPath := filepath.Join(dir, "data", "backups", "test.db")
	require.NoError(t, rt.BackupWorkspace(context.Background(), backupPath))

	restorePath := filepath.Join(dir, "data", "cloudopt.db")
	require.NoError(t, os.Remove(restorePath))
	require.NoError(t, rt.RestoreWorkspace(context.Background(), backupPath))

	_, err := api.OpenStorage(context.Background(), settings)
	require.NoError(t, err)
}

func TestPurgeAccount_verifiable(t *testing.T) {
	dir := t.TempDir()
	settings := config.Settings{
		WorkspaceDir: dir,
		ConfigDir:    filepath.Join(dir, "config"),
		DataDir:      filepath.Join(dir, "data"),
		ReportsDir:   filepath.Join(dir, "reports"),
		TempDir:      filepath.Join(dir, "tmp"),
		LogFormat:    "text",
		LogLevel:     "info",
	}
	rt := api.NewRuntime(settings)
	require.NoError(t, rt.Init(context.Background()))
	_, err := rt.ImportFixture(context.Background(), filepath.Join("..", "..", "..", "testdata", "fixtures", "aws-minimal.yaml"))
	require.NoError(t, err)

	_, err = rt.PurgeAccount(context.Background(), types.AccountID("acct-fixture-001"))
	require.NoError(t, err)
	require.NoError(t, rt.VerifyAccountPurged(context.Background(), types.AccountID("acct-fixture-001")))
}

func TestExportDiagnostics_noTelemetryByDefault(t *testing.T) {
	dir := t.TempDir()
	settings := config.Settings{
		WorkspaceDir: dir,
		ConfigDir:    filepath.Join(dir, "config"),
		DataDir:      filepath.Join(dir, "data"),
		ReportsDir:   filepath.Join(dir, "reports"),
		TempDir:      filepath.Join(dir, "tmp"),
		LogFormat:    "text",
		LogLevel:     "info",
	}
	rt := api.NewRuntime(settings)
	require.NoError(t, rt.Init(context.Background()))
	out := filepath.Join(dir, "tmp", "diag.json")
	require.NoError(t, rt.ExportDiagnostics(context.Background(), out))
}

func TestCorruptedDatabaseRejected(t *testing.T) {
	dir := t.TempDir()
	settings := config.Settings{
		WorkspaceDir: dir,
		ConfigDir:    filepath.Join(dir, "config"),
		DataDir:      filepath.Join(dir, "data"),
		ReportsDir:   filepath.Join(dir, "reports"),
		TempDir:      filepath.Join(dir, "tmp"),
		LogFormat:    "text",
		LogLevel:     "info",
	}
	require.NoError(t, os.MkdirAll(settings.DataDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(settings.DataDir, "cloudopt.db"), []byte("not-sqlite"), 0o600))
	_, err := api.OpenStorage(context.Background(), settings)
	require.Error(t, err)
}
