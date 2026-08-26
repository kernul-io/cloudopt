package collect

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// Lifecycle coordinates resumable collection and cleanup of abandoned snapshots.
type Lifecycle struct {
	Repo ports.StorageRepository
	TTL  time.Duration
}

// PrepareSnapshotID cleans stale in-progress rows and returns an ID for a new collection attempt.
func (l *Lifecycle) PrepareSnapshotID(ctx context.Context, accountID types.AccountID, provider types.Provider, resumeID types.SnapshotID) (types.SnapshotID, error) {
	if l == nil || l.Repo == nil {
		return newSnapshotID()
	}
	if err := l.CleanupAbandoned(ctx, accountID); err != nil {
		return "", err
	}
	if resumeID != "" {
		snap, err := l.Repo.GetSnapshot(ctx, resumeID)
		if err != nil {
			return "", fmt.Errorf("resume snapshot: %w", err)
		}
		if snap.Status != domain.SnapshotInProgress {
			return "", fmt.Errorf("snapshot %q is not in_progress", resumeID)
		}
		return resumeID, nil
	}
	id, err := newSnapshotID()
	if err != nil {
		return "", err
	}
	now := types.NowUTC()
	stub := &domain.CollectionSnapshot{
		ID:            id,
		AccountID:     accountID,
		Provider:      provider,
		Status:        domain.SnapshotInProgress,
		SchemaVersion: 1,
		StartedAt:     now,
	}
	if err := l.Repo.SaveInProgressSnapshot(ctx, stub); err != nil {
		return "", err
	}
	return id, nil
}

// Finalize replaces an in-progress shell with the completed snapshot payload.
func (l *Lifecycle) Finalize(ctx context.Context, snap *domain.CollectionSnapshot) error {
	if snap == nil {
		return fmt.Errorf("snapshot is nil")
	}
	if err := l.Repo.DeleteSnapshot(ctx, snap.ID); err != nil {
		return fmt.Errorf("finalize: remove in_progress shell: %w", err)
	}
	return l.Repo.SaveSnapshot(ctx, snap)
}

// Fail marks an in-progress snapshot failed after interruption.
func (l *Lifecycle) Fail(ctx context.Context, id types.SnapshotID) error {
	if l == nil || l.Repo == nil || id == "" {
		return nil
	}
	return l.Repo.MarkSnapshotFailed(ctx, id)
}

// CleanupAbandoned deletes or fails in-progress snapshots older than TTL.
func (l *Lifecycle) CleanupAbandoned(ctx context.Context, accountID types.AccountID) error {
	if l == nil || l.Repo == nil {
		return nil
	}
	cutoff := time.Now().UTC().Add(-l.TTL)
	stale, err := l.Repo.ListSnapshots(ctx, ports.ListSnapshotFilter{
		AccountID: accountID,
		Status:    domain.SnapshotInProgress,
	})
	if err != nil {
		return err
	}
	for _, s := range stale {
		started, err := time.Parse(time.RFC3339Nano, s.StartedAt.Canonical())
		if err != nil {
			started, _ = time.Parse(time.RFC3339, s.StartedAt.Canonical())
		}
		if !started.IsZero() && started.After(cutoff) {
			continue
		}
		_ = l.Repo.DeleteSnapshot(ctx, s.ID)
	}
	return nil
}

func newSnapshotID() (types.SnapshotID, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("snapshot id entropy: %w", err)
	}
	return types.SnapshotID("snap-" + hex.EncodeToString(b[:])), nil
}
