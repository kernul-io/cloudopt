package fixture

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/kernul-io/cloudopt/internal/application/engagement"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

type engagementFile struct {
	FormatVersion int    `yaml:"format_version"`
	ExternalKey   string `yaml:"external_key"`
	Engagement    struct {
		Name    string `yaml:"name"`
		Members []struct {
			Path string `yaml:"path"`
		} `yaml:"members"`
	} `yaml:"engagement"`
}

// ImportEngagement loads member fixtures and persists a merged multi-cloud snapshot.
func (im *Importer) ImportEngagement(ctx context.Context, path string) (types.SnapshotID, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read engagement fixture: %w", err)
	}
	var doc engagementFile
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("parse engagement fixture: %w", err)
	}
	if doc.FormatVersion != 2 {
		return "", fmt.Errorf("engagement fixture requires format_version 2")
	}
	if len(doc.Engagement.Members) == 0 {
		return "", fmt.Errorf("engagement members are required")
	}
	baseDir := filepath.Dir(path)
	var members []*domain.CollectionSnapshot
	for _, m := range doc.Engagement.Members {
		memberPath := m.Path
		if !filepath.IsAbs(memberPath) {
			memberPath = filepath.Join(baseDir, memberPath)
		}
		id, err := im.Import(ctx, memberPath)
		if err != nil {
			return "", fmt.Errorf("import member %q: %w", memberPath, err)
		}
		snap, err := im.Repo.GetSnapshot(ctx, id)
		if err != nil {
			return "", err
		}
		members = append(members, snap)
	}
	merged, err := engagement.MergeSnapshots(doc.Engagement.Name, doc.ExternalKey, members)
	if err != nil {
		return "", err
	}
	snapID, err := newSnapshotID()
	if err != nil {
		return "", err
	}
	merged.ID = snapID
	if err := im.Repo.SaveSnapshot(ctx, merged); err != nil {
		return "", fmt.Errorf("save engagement snapshot: %w", err)
	}
	return snapID, nil
}
