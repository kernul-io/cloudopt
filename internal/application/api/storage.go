package api

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/kernul-io/cloudopt/internal/adapters/config"
	sqliterepository "github.com/kernul-io/cloudopt/internal/adapters/sqlite-repository"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

const dbFileName = "cloudopt.db"

// OpenStorage opens the workspace SQLite database, runs migrations, and returns the repository.
func OpenStorage(ctx context.Context, settings config.Settings) (ports.StorageRepository, error) {
	path := filepath.Join(settings.DataDir, dbFileName)
	db, err := sqliterepository.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open storage: %w", err)
	}
	repo := sqliterepository.NewRepository(db)
	if err := repo.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate storage: %w", err)
	}
	return repo, nil
}

// DatabasePath returns the SQLite file path for the configured workspace.
func DatabasePath(settings config.Settings) string {
	return filepath.Join(settings.DataDir, dbFileName)
}
