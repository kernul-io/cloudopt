package api

import (
	"context"
	"path/filepath"

	awspricing "github.com/kernul-io/cloudopt/internal/adapters/aws-pricing"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/application/pricing"
)

// LoadPricingCatalog resolves an offline catalog for analyze (live API deferred).
func LoadPricingCatalog(ctx context.Context, fixtureRoot string) (*pricing.Catalog, error) {
	root := fixtureRoot
	if root == "" {
		root = filepath.Join("testdata", "aws-pricing")
	}
	col := awspricing.NewCollector(root)
	res, err := col.LoadCatalog(ctx, ports.PricingLoadOptions{
		Provider:    types.ProviderAWS,
		Offline:     true,
		FixtureRoot: root,
	})
	if err != nil {
		// Try module-relative path for tests executed from subpackages.
		alt := filepath.Join("..", "..", "..", "testdata", "aws-pricing")
		col = awspricing.NewCollector(alt)
		res, err = col.LoadCatalog(ctx, ports.PricingLoadOptions{
			Provider:    types.ProviderAWS,
			Offline:     true,
			FixtureRoot: alt,
		})
		if err != nil {
			return nil, err
		}
	}
	return pricing.NewCatalog(res.Records, res.Source), nil
}
