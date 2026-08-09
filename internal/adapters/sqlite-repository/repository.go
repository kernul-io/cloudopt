package sqliterepository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

// Repository implements ports.StorageRepository with SQLite.
type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Migrate(ctx context.Context) error {
	return Migrate(ctx, r.db)
}

func (r *Repository) SchemaVersion(ctx context.Context) (int, error) {
	return SchemaVersion(ctx, r.db)
}

func (r *Repository) CanonicalSchemaVersion(ctx context.Context) (int, error) {
	return CanonicalSchemaVersion(ctx, r.db)
}

var (
	ErrSnapshotNotFound     = errors.New("snapshot not found")
	ErrAnalysisRunNotFound  = errors.New("analysis run not found")
	ErrSnapshotNotComplete  = errors.New("snapshot is not complete and cannot be replaced")
	ErrDuplicateExternalKey = errors.New("snapshot with external_key already exists for account")
)

func (r *Repository) SaveSnapshot(ctx context.Context, snap *domain.CollectionSnapshot) error {
	if snap == nil {
		return fmt.Errorf("snapshot is nil")
	}
	if snap.Status != domain.SnapshotComplete {
		return fmt.Errorf("only complete snapshots can be saved atomically")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if snap.ExternalKey != "" {
		var existing string
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM snapshots WHERE account_id = ? AND external_key = ?`,
			string(snap.AccountID), snap.ExternalKey,
		).Scan(&existing)
		if err == nil {
			snap.ID = types.SnapshotID(existing)
			return tx.Commit() // idempotent: already stored
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}

	if err := upsertAccount(ctx, tx, snap.Account); err != nil {
		return err
	}
	if err := insertSnapshot(ctx, tx, snap); err != nil {
		return err
	}
	if err := insertRegions(ctx, tx, snap); err != nil {
		return err
	}
	if err := insertResources(ctx, tx, snap); err != nil {
		return err
	}
	if err := insertRelationships(ctx, tx, snap); err != nil {
		return err
	}
	if err := insertCosts(ctx, tx, snap); err != nil {
		return err
	}
	if err := insertMetrics(ctx, tx, snap); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *Repository) GetSnapshot(ctx context.Context, id types.SnapshotID) (*domain.CollectionSnapshot, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT account_id, provider, status, schema_version, external_key, started_at, completed_at
		FROM snapshots WHERE id = ?`, string(id))

	var snap domain.CollectionSnapshot
	snap.ID = id
	var completed sql.NullString
	var accountID string
	if err := row.Scan(&accountID, &snap.Provider, &snap.Status, &snap.SchemaVersion,
		&snap.ExternalKey, &tsScan{&snap.StartedAt}, &completed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSnapshotNotFound
		}
		return nil, err
	}
	snap.AccountID = types.AccountID(accountID)
	if completed.Valid {
		t, err := types.ParseTimestamp(completed.String)
		if err != nil {
			return nil, err
		}
		snap.CompletedAt = &t
	}

	acc, err := loadAccount(ctx, r.db, snap.AccountID)
	if err != nil {
		return nil, err
	}
	snap.Account = acc

	regions, err := loadRegions(ctx, r.db, id)
	if err != nil {
		return nil, err
	}
	snap.Regions = regions

	resources, err := loadResources(ctx, r.db, id)
	if err != nil {
		return nil, err
	}
	snap.Resources = resources

	rels, err := loadRelationships(ctx, r.db, id)
	if err != nil {
		return nil, err
	}
	snap.Relationships = rels

	costs, err := loadCosts(ctx, r.db, id)
	if err != nil {
		return nil, err
	}
	snap.Costs = costs

	metrics, err := loadMetrics(ctx, r.db, id)
	if err != nil {
		return nil, err
	}
	snap.Metrics = metrics

	return &snap, nil
}

func (r *Repository) ListSnapshots(ctx context.Context, filter ports.ListSnapshotFilter) ([]domain.SnapshotSummary, error) {
	q := `SELECT id, account_id, provider, status, started_at, completed_at, external_key FROM snapshots WHERE 1=1`
	args := []any{}
	if filter.AccountID != "" {
		q += ` AND account_id = ?`
		args = append(args, string(filter.AccountID))
	}
	if filter.CompleteOnly {
		q += ` AND status = ?`
		args = append(args, string(domain.SnapshotComplete))
	}
	q += ` ORDER BY started_at DESC`
	if filter.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, filter.Limit)
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.SnapshotSummary
	for rows.Next() {
		var s domain.SnapshotSummary
		var completed sql.NullString
		if err := rows.Scan(&s.ID, &s.AccountID, &s.Provider, &s.Status,
			&tsScan{&s.StartedAt}, &completed, &s.ExternalKey); err != nil {
			return nil, err
		}
		if completed.Valid {
			t, err := types.ParseTimestamp(completed.String)
			if err != nil {
				return nil, err
			}
			s.CompletedAt = &t
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) MarkSnapshotFailed(ctx context.Context, id types.SnapshotID) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE snapshots SET status = ? WHERE id = ? AND status = ?`,
		string(domain.SnapshotFailed), string(id), string(domain.SnapshotInProgress),
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSnapshotNotFound
	}
	return nil
}

func (r *Repository) SaveAnalysisRun(ctx context.Context, run *domain.AnalysisRun) error {
	if run == nil {
		return fmt.Errorf("analysis run is nil")
	}
	if run.Status != domain.AnalysisComplete {
		return fmt.Errorf("only complete analysis runs can be saved atomically")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var snapStatus string
	err = tx.QueryRowContext(ctx, `SELECT status FROM snapshots WHERE id = ?`, string(run.SnapshotID)).Scan(&snapStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSnapshotNotFound
		}
		return err
	}
	if snapStatus != string(domain.SnapshotComplete) {
		return fmt.Errorf("analysis requires a complete snapshot")
	}

	var completed *string
	if run.CompletedAt != nil {
		s := run.CompletedAt.Canonical()
		completed = &s
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO analysis_runs (id, snapshot_id, status, rule_set_version, started_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		string(run.ID), string(run.SnapshotID), string(run.Status), run.RuleSetVersion,
		run.StartedAt.Canonical(), completed,
	)
	if err != nil {
		return err
	}

	evidenceIDMap, err := insertEvidence(ctx, tx, run)
	if err != nil {
		return err
	}
	if err := insertFindings(ctx, tx, run, evidenceIDMap); err != nil {
		return err
	}
	if err := insertRecommendations(ctx, tx, run); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *Repository) GetAnalysisRun(ctx context.Context, id types.AnalysisRunID) (*domain.AnalysisRun, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT snapshot_id, status, rule_set_version, started_at, completed_at
		FROM analysis_runs WHERE id = ?`, string(id))

	var run domain.AnalysisRun
	run.ID = id
	var completed sql.NullString
	if err := row.Scan(&run.SnapshotID, &run.Status, &run.RuleSetVersion,
		&tsScan{&run.StartedAt}, &completed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAnalysisRunNotFound
		}
		return nil, err
	}
	if completed.Valid {
		t, err := types.ParseTimestamp(completed.String)
		if err != nil {
			return nil, err
		}
		run.CompletedAt = &t
	}

	evidence, err := loadEvidence(ctx, r.db, id)
	if err != nil {
		return nil, err
	}
	run.Evidence = evidence

	findings, err := loadFindings(ctx, r.db, id)
	if err != nil {
		return nil, err
	}
	run.Findings = findings

	recs, err := loadRecommendations(ctx, r.db, id)
	if err != nil {
		return nil, err
	}
	run.Recommendations = recs

	return &run, nil
}

func (r *Repository) GetLatestAnalysisRun(ctx context.Context, snapshotID types.SnapshotID) (*domain.AnalysisRun, error) {
	var id string
	var err error
	if snapshotID != "" {
		err = r.db.QueryRowContext(ctx, `
			SELECT id FROM analysis_runs
			WHERE snapshot_id = ? AND status = ?
			ORDER BY completed_at DESC, started_at DESC
			LIMIT 1`, string(snapshotID), string(domain.AnalysisComplete)).Scan(&id)
	} else {
		err = r.db.QueryRowContext(ctx, `
			SELECT id FROM analysis_runs
			WHERE status = ?
			ORDER BY completed_at DESC, started_at DESC
			LIMIT 1`, string(domain.AnalysisComplete)).Scan(&id)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAnalysisRunNotFound
		}
		return nil, err
	}
	return r.GetAnalysisRun(ctx, types.AnalysisRunID(id))
}

func (r *Repository) DeleteSnapshot(ctx context.Context, id types.SnapshotID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM snapshots WHERE id = ?`, string(id))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSnapshotNotFound
	}
	return nil
}

func (r *Repository) DeleteSnapshotsByAccount(ctx context.Context, accountID types.AccountID) (int, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM snapshots WHERE account_id = ?`, string(accountID))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		_, _ = r.db.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, string(accountID))
	}
	return int(n), nil
}

func (r *Repository) ApplyRetention(ctx context.Context, accountID types.AccountID, keepComplete int) (int, error) {
	if keepComplete < 0 {
		return 0, fmt.Errorf("keepComplete must be >= 0")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM snapshots
		WHERE account_id = ? AND status = ?
		ORDER BY completed_at DESC, started_at DESC`,
		string(accountID), string(domain.SnapshotComplete),
	)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	deleted := 0
	for i := keepComplete; i < len(ids); i++ {
		if err := r.DeleteSnapshot(ctx, types.SnapshotID(ids[i])); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// tsScan implements sql.Scanner for types.Timestamp.
type tsScan struct {
	dst *types.Timestamp
}

func (t *tsScan) Scan(src any) error {
	switch v := src.(type) {
	case string:
		parsed, err := types.ParseTimestamp(v)
		if err != nil {
			return err
		}
		*t.dst = parsed
		return nil
	case []byte:
		parsed, err := types.ParseTimestamp(string(v))
		if err != nil {
			return err
		}
		*t.dst = parsed
		return nil
	case time.Time:
		*t.dst = types.NewTimestamp(v)
		return nil
	default:
		return fmt.Errorf("unsupported timestamp type %T", src)
	}
}

func encodeJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func provColumns(p domain.Provenance) (quality, source, observed string) {
	return string(p.Quality), p.Source, p.ObservedAt.Canonical()
}
