package fixture

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

// Importer loads offline fixture YAML into canonical storage.
type Importer struct {
	Repo ports.StorageRepository
}

func NewImporter(repo ports.StorageRepository) *Importer {
	return &Importer{Repo: repo}
}

type fixtureFile struct {
	FormatVersion int                   `yaml:"format_version"`
	ExternalKey   string                `yaml:"external_key"`
	ObservedAt    string                `yaml:"observed_at"`
	Source        string                `yaml:"source"`
	Account       fixtureAccount        `yaml:"account"`
	Regions       []fixtureRegion       `yaml:"regions"`
	Resources     []fixtureResource     `yaml:"resources"`
	Relationships []fixtureRelationship `yaml:"relationships"`
	Costs         []fixtureCost         `yaml:"costs"`
	Metrics       []fixtureMetric       `yaml:"metrics"`
}

type fixtureAccount struct {
	ID                string `yaml:"id"`
	Provider          string `yaml:"provider"`
	ProviderAccountID string `yaml:"provider_account_id"`
	DisplayName       string `yaml:"display_name"`
	DefaultCurrency   string `yaml:"default_currency"`
}

type fixtureRegion struct {
	ID               string `yaml:"id"`
	ProviderRegionID string `yaml:"provider_region_id"`
	DisplayName      string `yaml:"display_name"`
}

type fixtureResource struct {
	ID                 string            `yaml:"id"`
	Kind               string            `yaml:"kind"`
	RegionID           string            `yaml:"region_id"`
	ProviderResourceID string            `yaml:"provider_resource_id"`
	Name               string            `yaml:"name"`
	State              string            `yaml:"state"`
	Tags               map[string]string `yaml:"tags"`
	Attributes         map[string]string `yaml:"attributes"`
}

type fixtureRelationship struct {
	Kind                 string `yaml:"kind"`
	FromResourceID       string `yaml:"from_resource_id"`
	ToResourceID         string `yaml:"to_resource_id"`
	ToProviderResourceID string `yaml:"to_provider_resource_id"`
	TargetMissing        bool   `yaml:"target_missing"`
}

type fixtureCost struct {
	ResourceID  string  `yaml:"resource_id"`
	Service     string  `yaml:"service"`
	AmountMajor float64 `yaml:"amount_major"`
	Currency    string  `yaml:"currency"`
	Granularity string  `yaml:"granularity"`
	PeriodStart string  `yaml:"period_start"`
	PeriodEnd   string  `yaml:"period_end"`
}

type fixtureMetric struct {
	ResourceID string `yaml:"resource_id"`
	Name       string `yaml:"name"`
	Statistic  string `yaml:"statistic"`
	Points     []struct {
		Timestamp string  `yaml:"timestamp"`
		Value     float64 `yaml:"value"`
		Unit      string  `yaml:"unit"`
	} `yaml:"points"`
}

// Import parses a fixture file and persists a complete snapshot.
func (im *Importer) Import(ctx context.Context, path string) (types.SnapshotID, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read fixture: %w", err)
	}
	var doc fixtureFile
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("parse fixture: %w", err)
	}
	if doc.FormatVersion != 1 {
		return "", fmt.Errorf("unsupported format_version %d", doc.FormatVersion)
	}

	observed, err := types.ParseTimestamp(defaultString(doc.ObservedAt, time.Now().UTC().Format(time.RFC3339Nano)))
	if err != nil {
		return "", fmt.Errorf("observed_at: %w", err)
	}
	source := defaultString(doc.Source, path)
	prov := domain.Provenance{Quality: domain.QualityObserved, Source: source, ObservedAt: observed}

	snapID, err := newSnapshotID()
	if err != nil {
		return "", err
	}

	accountID := types.AccountID(doc.Account.ID)
	snap := &domain.CollectionSnapshot{
		ID:            snapID,
		AccountID:     accountID,
		Provider:      types.Provider(doc.Account.Provider),
		Status:        domain.SnapshotComplete,
		SchemaVersion: 1,
		ExternalKey:   doc.ExternalKey,
		StartedAt:     observed,
		CompletedAt:   &observed,
		Account: domain.Account{
			ID:                accountID,
			Provider:          types.Provider(doc.Account.Provider),
			ProviderAccountID: doc.Account.ProviderAccountID,
			DisplayName:       doc.Account.DisplayName,
			DefaultCurrency:   doc.Account.DefaultCurrency,
			Provenance:        prov,
		},
	}

	for _, r := range doc.Regions {
		snap.Regions = append(snap.Regions, domain.Region{
			ID:               types.RegionID(r.ID),
			ProviderRegionID: r.ProviderRegionID,
			DisplayName:      r.DisplayName,
			Provenance:       prov,
		})
	}

	for _, r := range doc.Resources {
		var tags []domain.Tag
		for k, v := range r.Tags {
			tags = append(tags, domain.Tag{Key: k, Value: v})
		}
		snap.Resources = append(snap.Resources, domain.Resource{
			ID:                 types.ResourceID(r.ID),
			Kind:               domain.ResourceKind(r.Kind),
			ProviderResourceID: r.ProviderResourceID,
			AccountID:          accountID,
			RegionID:           types.RegionID(r.RegionID),
			Name:               r.Name,
			State:              r.State,
			Tags:               tags,
			Attributes:         r.Attributes,
			Provenance:         prov,
		})
	}

	for _, rel := range doc.Relationships {
		snap.Relationships = append(snap.Relationships, domain.Relationship{
			Kind:                 domain.RelationshipKind(rel.Kind),
			FromResourceID:       types.ResourceID(rel.FromResourceID),
			ToResourceID:         types.ResourceID(rel.ToResourceID),
			ToProviderResourceID: rel.ToProviderResourceID,
			TargetMissing:        rel.TargetMissing,
			Provenance:           prov,
		})
	}

	for _, c := range doc.Costs {
		start, err := types.ParseTimestamp(c.PeriodStart)
		if err != nil {
			return "", fmt.Errorf("cost period_start: %w", err)
		}
		end, err := types.ParseTimestamp(c.PeriodEnd)
		if err != nil {
			return "", fmt.Errorf("cost period_end: %w", err)
		}
		snap.Costs = append(snap.Costs, domain.CostRecord{
			ResourceID:  types.ResourceID(c.ResourceID),
			Service:     c.Service,
			Amount:      types.FromMajorUnits(c.AmountMajor, c.Currency, 100),
			Basis:       domain.CostBasisAmortizedNet,
			ChargeKind:  domain.ChargeUsage,
			Granularity: domain.CostGranularity(c.Granularity),
			PeriodStart: start,
			PeriodEnd:   end,
			Attribution: domain.CostAttribution{Method: domain.AttributionDirectResourceID, Confidence: 0.9},
			Provenance:  prov,
		})
	}

	for _, m := range doc.Metrics {
		series := domain.MetricSeries{
			ResourceID: types.ResourceID(m.ResourceID),
			Name:       m.Name,
			Statistic:  m.Statistic,
			Provenance: prov,
		}
		for _, pt := range m.Points {
			ts, err := types.ParseTimestamp(pt.Timestamp)
			if err != nil {
				return "", fmt.Errorf("metric timestamp: %w", err)
			}
			series.Points = append(series.Points, domain.MetricPoint{
				Timestamp: ts,
				Value:     pt.Value,
				Unit:      pt.Unit,
				Quality:   domain.QualityObserved,
			})
		}
		snap.Metrics = append(snap.Metrics, series)
	}

	if err := im.Repo.SaveSnapshot(ctx, snap); err != nil {
		return "", fmt.Errorf("save snapshot: %w", err)
	}
	return snap.ID, nil
}

func newSnapshotID() (types.SnapshotID, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return types.SnapshotID("snap-" + hex.EncodeToString(b[:])), nil
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
