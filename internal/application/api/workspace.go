package api

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kernul-io/cloudopt/internal/adapters/config"
	sqliterepository "github.com/kernul-io/cloudopt/internal/adapters/sqlite-repository"
	"github.com/kernul-io/cloudopt/internal/application/audit"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/application/security"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// WorkspaceService handles backup, restore, deletion, and diagnostics.
type WorkspaceService struct {
	Settings config.Settings
	Audit    *audit.Log
}

// SecureWorkspacePermissions chmods workspace dirs and sensitive files.
func (w *WorkspaceService) SecureWorkspacePermissions() error {
	dirs := []string{w.Settings.ConfigDir, w.Settings.DataDir, w.Settings.ReportsDir, w.Settings.TempDir}
	for _, d := range dirs {
		if err := security.SecureMkdirAll(d); err != nil {
			return err
		}
		if err := security.HardenExistingPath(d, true); err != nil {
			return err
		}
	}
	cfg := filepath.Join(w.Settings.ConfigDir, "config.yaml")
	if _, err := os.Stat(cfg); err == nil {
		if err := security.HardenExistingPath(cfg, false); err != nil {
			return err
		}
	}
	db := DatabasePath(w.Settings)
	if _, err := os.Stat(db); err == nil {
		if err := security.HardenExistingPath(db, false); err != nil {
			return err
		}
	}
	return nil
}

// BackupDatabase copies the SQLite file to dest after schema compatibility checks.
func (w *WorkspaceService) BackupDatabase(ctx context.Context, dest string) error {
	if err := security.ValidateOutputPath(dest, w.Settings.WorkspaceDir, w.Settings.DataDir); err != nil {
		return err
	}
	if err := w.ensureStorageCompatible(ctx); err != nil {
		return err
	}
	src := DatabasePath(w.Settings)
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = in.Close() }()
	if err := os.MkdirAll(filepath.Dir(dest), security.DirPerm); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, security.FilePerm)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy backup: %w", err)
	}
	if w.Audit != nil {
		_ = w.Audit.Append(audit.Event{
			Kind:      audit.EventBackup,
			Workspace: w.Settings.WorkspaceDir,
			Details:   map[string]string{"dest": dest},
		})
	}
	return nil
}

// RestoreDatabase replaces the workspace database from a backup file.
func (w *WorkspaceService) RestoreDatabase(ctx context.Context, src string) error {
	if !filepath.IsAbs(src) {
		return fmt.Errorf("restore source must be absolute")
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer func() { _ = in.Close() }()
	dest := DatabasePath(w.Settings)
	if err := os.MkdirAll(filepath.Dir(dest), security.DirPerm); err != nil {
		return err
	}
	tmp := dest + ".restore-" + time.Now().UTC().Format("20060102T150405Z")
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, security.FilePerm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	db, err := sqliterepository.Open(dest)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	repo := sqliterepository.NewRepository(db)
	if err := repo.Migrate(ctx); err != nil {
		return err
	}
	canonical, _ := repo.CanonicalSchemaVersion(ctx)
	migration, _ := repo.SchemaVersion(ctx)
	if err := security.CheckDatabaseCompatibility(canonical, migration); err != nil {
		return err
	}
	if w.Audit != nil {
		_ = w.Audit.Append(audit.Event{
			Kind:      audit.EventRestore,
			Workspace: w.Settings.WorkspaceDir,
			Details:   map[string]string{"src": src},
		})
	}
	return nil
}

// PurgeAccountData deletes all snapshots and account rows for one canonical account id.
func (w *WorkspaceService) PurgeAccountData(ctx context.Context, repo ports.StorageRepository, accountID types.AccountID) (int, error) {
	n, err := repo.DeleteSnapshotsByAccount(ctx, accountID)
	if err != nil {
		return 0, err
	}
	if w.Audit != nil {
		_ = w.Audit.Append(audit.Event{
			Kind:      audit.EventDeletion,
			Workspace: w.Settings.WorkspaceDir,
			Details: map[string]string{
				"account_id": string(accountID),
				"snapshots":  fmt.Sprintf("%d", n),
			},
		})
	}
	return n, nil
}

func (w *WorkspaceService) ensureStorageCompatible(ctx context.Context) error {
	repo, err := OpenStorage(ctx, w.Settings)
	if err != nil {
		return err
	}
	type compat interface {
		SchemaVersion(context.Context) (int, error)
		CanonicalSchemaVersion(context.Context) (int, error)
	}
	c, ok := repo.(compat)
	if !ok {
		return nil
	}
	migration, _ := c.SchemaVersion(ctx)
	canonical, _ := c.CanonicalSchemaVersion(ctx)
	return security.CheckDatabaseCompatibility(canonical, migration)
}
