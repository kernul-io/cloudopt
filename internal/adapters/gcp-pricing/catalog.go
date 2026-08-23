package gcppricing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// Collector loads GCP list prices from offline fixtures.
type Collector struct {
	DefaultFixtureRoot string
}

func NewCollector(fixtureRoot string) *Collector {
	return &Collector{DefaultFixtureRoot: fixtureRoot}
}

type fixtureCatalog struct {
	CatalogVersion string `json:"catalog_version"`
	Source         string `json:"source"`
	EffectiveDate  string `json:"effective_date"`
	Currency       string `json:"currency"`
	Records        []struct {
		Service       string            `json:"service"`
		Region        string            `json:"region"`
		PurchaseModel string            `json:"purchase_model"`
		Unit          string            `json:"unit"`
		PriceMajor    float64           `json:"price_major"`
		Attributes    map[string]string `json:"attributes"`
	} `json:"records"`
}

func (c *Collector) Capabilities() (ports.CapabilityManifest, error) {
	return LoadCapabilities()
}

func (c *Collector) LoadCatalog(ctx context.Context, opts ports.PricingLoadOptions) (*ports.PricingCatalogResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root := opts.FixtureRoot
	if root == "" {
		root = c.DefaultFixtureRoot
	}
	if root == "" {
		return nil, fmt.Errorf("gcp pricing fixture root is required")
	}
	path := filepath.Join(root, "catalog-demo.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read gcp pricing fixture: %w", err)
	}
	var doc fixtureCatalog
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse gcp pricing fixture: %w", err)
	}
	effective, _ := types.ParseTimestamp(doc.EffectiveDate)
	if effective.IsZero() {
		effective = types.NewTimestamp(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	}
	source := doc.Source
	if source == "" {
		source = path
	}
	cur := doc.Currency
	if cur == "" {
		cur = "USD"
	}
	var records []domain.PricingRecord
	for i, row := range doc.Records {
		pm := domain.PurchaseModel(row.PurchaseModel)
		if pm == "" {
			pm = domain.PurchaseOnDemand
		}
		records = append(records, domain.PricingRecord{
			SKU:           fmt.Sprintf("gcp-%s-%s-%d", row.Service, row.Region, i),
			Service:       row.Service,
			Region:        row.Region,
			PurchaseModel: pm,
			Currency:      cur,
			EffectiveDate: effective,
			Unit:          row.Unit,
			PriceMinor:    types.FromMajorUnits(row.PriceMajor, cur, 100).AmountMinor,
			Source:        source,
			Attributes:    row.Attributes,
		})
	}
	return &ports.PricingCatalogResult{Records: records, Source: source}, nil
}

// DefaultCatalog loads repository testdata for GCP.
func DefaultCatalog(ctx context.Context) (*pricing.Catalog, error) {
	root := filepath.Join("testdata", "gcp-pricing")
	if _, err := os.Stat(root); err != nil {
		root = filepath.Join("..", "..", "..", "testdata", "gcp-pricing")
	}
	col := NewCollector(root)
	res, err := col.LoadCatalog(ctx, ports.PricingLoadOptions{Provider: types.ProviderGCP, Offline: true, FixtureRoot: root})
	if err != nil {
		return nil, err
	}
	return pricing.NewCatalog(res.Records, res.Source), nil
}
