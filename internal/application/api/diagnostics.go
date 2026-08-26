package api

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kernul-io/cloudopt/internal/application/audit"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/application/security"
	"github.com/kernul-io/cloudopt/internal/domain"
)

// DiagnosticsDocument is anonymized local diagnostics the user may export voluntarily.
type DiagnosticsDocument struct {
	SchemaVersion string             `json:"schema_version"`
	GeneratedAt   time.Time          `json:"generated_at"`
	GoVersion     string             `json:"go_version"`
	GOOS          string             `json:"goos"`
	GOARCH        string             `json:"goarch"`
	Workspace     string             `json:"workspace_dir"`
	Telemetry     bool               `json:"telemetry_enabled"`
	Storage       DiagnosticsStorage `json:"storage"`
}

// DiagnosticsStorage summarizes local database health without customer identifiers.
type DiagnosticsStorage struct {
	MigrationVersion int `json:"migration_version"`
	CanonicalSchema  int `json:"canonical_schema_version"`
	SnapshotCount    int `json:"snapshot_count"`
	CompleteCount    int `json:"complete_snapshot_count"`
}

// ExportDiagnostics writes anonymized diagnostics JSON to dest.
func (w *WorkspaceService) ExportDiagnostics(ctx context.Context, dest string) error {
	if err := security.ValidateOutputPath(dest, w.Settings.WorkspaceDir, w.Settings.TempDir); err != nil {
		return err
	}
	repo, err := OpenStorage(ctx, w.Settings)
	if err != nil {
		return err
	}
	doc := DiagnosticsDocument{
		SchemaVersion: "1.0.0",
		GeneratedAt:   time.Now().UTC(),
		GoVersion:     runtime.Version(),
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		Workspace:     w.Settings.WorkspaceDir,
		Telemetry:     w.Settings.TelemetryEnabled,
	}
	doc.Storage.MigrationVersion, _ = repo.SchemaVersion(ctx)
	doc.Storage.CanonicalSchema, _ = repo.CanonicalSchemaVersion(ctx)
	snaps, err := repo.ListSnapshots(ctx, ports.ListSnapshotFilter{})
	if err != nil {
		return err
	}
	doc.Storage.SnapshotCount = len(snaps)
	for _, s := range snaps {
		if s.Status == domain.SnapshotComplete {
			doc.Storage.CompleteCount++
		}
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := security.SecureFile(dest, b); err != nil {
		return err
	}
	if w.Audit != nil {
		_ = w.Audit.Append(audit.Event{
			Kind:      audit.EventExport,
			Workspace: w.Settings.WorkspaceDir,
			Details:   map[string]string{"dest": dest, "kind": "diagnostics"},
		})
	}
	return nil
}

// ExportEngagementArchive zips reports and metadata without credentials or raw provider payloads.
func (w *WorkspaceService) ExportEngagementArchive(ctx context.Context, dest string) error {
	if err := security.ValidateOutputPath(dest, w.Settings.WorkspaceDir, w.Settings.ReportsDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), security.DirPerm); err != nil {
		return err
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, security.FilePerm)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	manifest := map[string]string{
		"kind":               "engagement_archive",
		"schema_version":     "1.0.0",
		"generated_at":       time.Now().UTC().Format(time.RFC3339),
		"credentials_policy": "never_included",
	}
	mb, _ := json.MarshalIndent(manifest, "", "  ")
	wr, err := zw.Create("manifest.json")
	if err != nil {
		_ = zw.Close()
		_ = f.Close()
		return err
	}
	if _, err := wr.Write(append(mb, '\n')); err != nil {
		_ = zw.Close()
		_ = f.Close()
		return err
	}
	walkErr := filepath.Walk(w.Settings.ReportsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(w.Settings.ReportsDir, path)
		if err != nil {
			return err
		}
		entry, err := zw.Create(filepath.Join("reports", rel))
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = entry.Write(data)
		return err
	})
	if walkErr != nil {
		_ = zw.Close()
		_ = f.Close()
		return walkErr
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if w.Audit != nil {
		_ = w.Audit.Append(audit.Event{
			Kind:      audit.EventExport,
			Workspace: w.Settings.WorkspaceDir,
			Details:   map[string]string{"archive": dest, "kind": "engagement"},
		})
	}
	return nil
}
