package api

import (
	"context"
	"fmt"
	"path/filepath"

	awspricing "github.com/kernul-io/cloudopt/internal/adapters/aws-pricing"
	gcppricing "github.com/kernul-io/cloudopt/internal/adapters/gcp-pricing"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// LoadPricingCatalog resolves offline AWS and GCP catalogs for analyze.
func LoadPricingCatalog(ctx context.Context, fixtureRoot string) (*pricing.Catalog, error) {
	var merged []domain.PricingRecord
	source := "merged-pricing-fixtures"

	awsRoot := fixtureRoot
	if awsRoot == "" {
		awsRoot = filepath.Join("testdata", "aws-pricing")
	}
	if recs, src, err := loadCatalog(ctx, types.ProviderAWS, awsRoot); err == nil {
		merged = append(merged, recs...)
		if src != "" {
			source = src
		}
	}

	gcpRoot := fixtureRoot
	if gcpRoot == "" || gcpRoot == filepath.Join("testdata", "aws-pricing") {
		gcpRoot = filepath.Join("testdata", "gcp-pricing")
	}
	if recs, src, err := loadCatalog(ctx, types.ProviderGCP, gcpRoot); err == nil {
		merged = append(merged, recs...)
		if len(merged) > 0 {
			source = "merged:aws+gcp-pricing-fixtures"
		} else if src != "" {
			source = src
		}
	}

	if len(merged) == 0 {
		return nil, fmt.Errorf("pricing catalog unavailable")
	}
	return pricing.NewCatalog(merged, source), nil
}

func loadCatalog(ctx context.Context, provider types.Provider, root string) ([]domain.PricingRecord, string, error) {
	try := []string{root, filepath.Join("..", "..", "..", "testdata", catalogDir(provider))}
	var lastErr error
	for _, r := range try {
		var res *ports.PricingCatalogResult
		var err error
		switch provider {
		case types.ProviderGCP:
			res, err = gcppricing.NewCollector(r).LoadCatalog(ctx, ports.PricingLoadOptions{
				Provider: provider, Offline: true, FixtureRoot: r,
			})
		default:
			res, err = awspricing.NewCollector(r).LoadCatalog(ctx, ports.PricingLoadOptions{
				Provider: provider, Offline: true, FixtureRoot: r,
			})
		}
		if err != nil {
			lastErr = err
			continue
		}
		return res.Records, res.Source, nil
	}
	return nil, "", lastErr
}

func catalogDir(provider types.Provider) string {
	if provider == types.ProviderGCP {
		return "gcp-pricing"
	}
	return "aws-pricing"
}
