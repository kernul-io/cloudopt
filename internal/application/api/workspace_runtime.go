package api

import (
	"context"
	"fmt"

	"github.com/kernul-io/cloudopt/internal/adapters/config"
	"github.com/kernul-io/cloudopt/internal/application/audit"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// NewAuditLog opens the workspace audit log using configured paths.
func NewAuditLog(settings config.Settings) (*audit.Log, error) {
	return audit.NewLog(settings.WorkspaceDir, settings.AuditLogPath)
}

// Runtime workspace operations.

func (r *Runtime) SecureWorkspace(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ws := &WorkspaceService{Settings: r.Settings}
	return ws.SecureWorkspacePermissions()
}

func (r *Runtime) BackupWorkspace(ctx context.Context, dest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	log, _ := NewAuditLog(r.Settings)
	ws := &WorkspaceService{Settings: r.Settings, Audit: log}
	return ws.BackupDatabase(ctx, dest)
}

func (r *Runtime) RestoreWorkspace(ctx context.Context, src string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	log, _ := NewAuditLog(r.Settings)
	ws := &WorkspaceService{Settings: r.Settings, Audit: log}
	return ws.RestoreDatabase(ctx, src)
}

func (r *Runtime) PurgeAccount(ctx context.Context, accountID types.AccountID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	repo, err := OpenStorage(ctx, r.Settings)
	if err != nil {
		return 0, err
	}
	log, _ := NewAuditLog(r.Settings)
	ws := &WorkspaceService{Settings: r.Settings, Audit: log}
	n, err := ws.PurgeAccountData(ctx, repo, accountID)
	if err != nil {
		return 0, err
	}
	if r.Settings.RetentionCompleteSnapshots >= 0 {
		_, _ = repo.ApplyRetention(ctx, accountID, r.Settings.RetentionCompleteSnapshots)
	}
	return n, nil
}

func (r *Runtime) ExportDiagnostics(ctx context.Context, dest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	log, _ := NewAuditLog(r.Settings)
	ws := &WorkspaceService{Settings: r.Settings, Audit: log}
	return ws.ExportDiagnostics(ctx, dest)
}

func (r *Runtime) ExportEngagementArchive(ctx context.Context, dest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	log, _ := NewAuditLog(r.Settings)
	ws := &WorkspaceService{Settings: r.Settings, Audit: log}
	return ws.ExportEngagementArchive(ctx, dest)
}

// VerifyAccountPurged returns an error if any snapshots remain for the account.
func (r *Runtime) VerifyAccountPurged(ctx context.Context, accountID types.AccountID) error {
	repo, err := OpenStorage(ctx, r.Settings)
	if err != nil {
		return err
	}
	snaps, err := repo.ListSnapshots(ctx, ports.ListSnapshotFilter{AccountID: accountID})
	if err != nil {
		return err
	}
	if len(snaps) > 0 {
		return fmt.Errorf("account %q still has %d snapshot(s)", accountID, len(snaps))
	}
	return nil
}
