package sqliterepository

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const migrationsDir = "migrations"

// Open opens (or creates) a SQLite database and returns a handle.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return db, nil
}

// Migrate applies forward-only SQL migrations from the embedded migrations directory.
func Migrate(ctx context.Context, db *sql.DB) error {
	if err := ensureMigrationTable(ctx, db); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationFS, migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)

	for _, name := range files {
		version, err := parseMigrationVersion(name)
		if err != nil {
			return err
		}
		applied, err := isMigrationApplied(ctx, db, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		body, err := migrationFS.ReadFile(migrationsDir + "/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := applyMigration(ctx, db, version, string(body)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}

func parseMigrationVersion(name string) (int, error) {
	prefix := strings.SplitN(name, "_", 2)[0]
	v, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("migration filename %q: invalid version prefix", name)
	}
	return v, nil
}

func ensureMigrationTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`)
	return err
}

func isMigrationApplied(ctx context.Context, db *sql.DB, version int) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, version).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func applyMigration(ctx context.Context, db *sql.DB, version int, sqlBody string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, sqlBody); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		version, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// SchemaVersion returns the highest applied migration version.
func SchemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var v sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v)
	if err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

// CanonicalSchemaVersion reads canonical_schema_version from schema_meta when present.
func CanonicalSchemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var val string
	err := db.QueryRowContext(ctx, `SELECT value FROM schema_meta WHERE key = 'canonical_schema_version'`).Scan(&val)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("parse canonical_schema_version: %w", err)
	}
	return n, nil
}
