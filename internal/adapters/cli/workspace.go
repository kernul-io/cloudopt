package cli

import (
	"context"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func newWorkspaceCommand(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Security, backup, deletion, and diagnostics for the local workspace",
	}
	cmd.AddCommand(newWorkspaceSecureCommand(cfg))
	cmd.AddCommand(newWorkspaceBackupCommand(cfg))
	cmd.AddCommand(newWorkspaceRestoreCommand(cfg))
	cmd.AddCommand(newWorkspacePurgeCommand(cfg))
	cmd.AddCommand(newWorkspaceDiagnosticsCommand(cfg))
	cmd.AddCommand(newWorkspaceExportArchiveCommand(cfg))
	return cmd
}

func newWorkspaceSecureCommand(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "secure-permissions",
		Short: "Apply restrictive permissions to workspace directories and config",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOperational(cmd, cfg, "workspace secure-permissions", (*api.Runtime).SecureWorkspace)
		},
	}
}

func newWorkspaceBackupCommand(cfg *Config) *cobra.Command {
	var dest string
	c := &cobra.Command{
		Use:   "backup",
		Short: "Copy the workspace SQLite database to a backup file",
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := LoadSettings(cfg.Overrides)
			if err != nil {
				return err
			}
			if dest == "" {
				dest = filepath.Join(settings.DataDir, "backups", "cloudopt-backup.db")
			}
			return runOperational(cmd, cfg, "workspace backup", func(rt *api.Runtime, ctx context.Context) error {
				return rt.BackupWorkspace(ctx, dest)
			})
		},
	}
	c.Flags().StringVar(&dest, "output", "", "Backup file path (default: data/backups/cloudopt-backup.db)")
	return c
}

func newWorkspaceRestoreCommand(cfg *Config) *cobra.Command {
	var src string
	c := &cobra.Command{
		Use:   "restore",
		Short: "Replace the workspace database from a backup file",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if src == "" {
				return cmd.MarkFlagRequired("from")
			}
			return runOperational(cmd, cfg, "workspace restore", func(rt *api.Runtime, ctx context.Context) error {
				return rt.RestoreWorkspace(ctx, src)
			})
		},
	}
	c.Flags().StringVar(&src, "from", "", "Absolute path to backup file")
	return c
}

func newWorkspacePurgeCommand(cfg *Config) *cobra.Command {
	var accountID string
	var verify bool
	c := &cobra.Command{
		Use:   "purge-account",
		Short: "Delete all local snapshots and account metadata for one account id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if accountID == "" {
				return cmd.MarkFlagRequired("account-id")
			}
			settings, err := LoadSettings(cfg.Overrides)
			if err != nil {
				return err
			}
			_ = settings
			return runOperational(cmd, cfg, "workspace purge-account", func(rt *api.Runtime, ctx context.Context) error {
				_, err := rt.PurgeAccount(ctx, types.AccountID(accountID))
				if err != nil {
					return err
				}
				if verify {
					return rt.VerifyAccountPurged(ctx, types.AccountID(accountID))
				}
				return nil
			})
		},
	}
	c.Flags().StringVar(&accountID, "account-id", "", "Canonical account id to purge")
	c.Flags().BoolVar(&verify, "verify", false, "Verify no snapshots remain after purge")
	return c
}

func newWorkspaceDiagnosticsCommand(cfg *Config) *cobra.Command {
	var dest string
	c := &cobra.Command{
		Use:   "export-diagnostics",
		Short: "Export anonymized local diagnostics (telemetry off by default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := LoadSettings(cfg.Overrides)
			if err != nil {
				return err
			}
			if dest == "" {
				dest = filepath.Join(settings.TempDir, "diagnostics.json")
			}
			return runOperational(cmd, cfg, "workspace export-diagnostics", func(rt *api.Runtime, ctx context.Context) error {
				return rt.ExportDiagnostics(ctx, dest)
			})
		},
	}
	c.Flags().StringVar(&dest, "output", "", "Output JSON path")
	return c
}

func newWorkspaceExportArchiveCommand(cfg *Config) *cobra.Command {
	var dest string
	c := &cobra.Command{
		Use:   "export-archive",
		Short: "Export engagement reports archive without credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := LoadSettings(cfg.Overrides)
			if err != nil {
				return err
			}
			if dest == "" {
				dest = filepath.Join(settings.ReportsDir, "engagement-archive.zip")
			}
			return runOperational(cmd, cfg, "workspace export-archive", func(rt *api.Runtime, ctx context.Context) error {
				return rt.ExportEngagementArchive(ctx, dest)
			})
		},
	}
	c.Flags().StringVar(&dest, "output", "", "Output zip path")
	return c
}
