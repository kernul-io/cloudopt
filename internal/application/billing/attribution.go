package billing

import (
	"fmt"
	"math"
	"sort"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
)

// DefaultReconcileToleranceBasisPoints allows 0.5% rounding drift between CE totals and attributed rows.
const DefaultReconcileToleranceBasisPoints int64 = 50

// ReconciliationTolerance documents the basis-point tolerance used in Reconcile.
var ReconciliationTolerance = DefaultReconcileToleranceBasisPoints

// Reconcile compares source totals to attributed and unattributed cost rows.
func Reconcile(sourceTotals map[string]types.Money, costs []domain.CostRecord, toleranceBasisPoints int64) domain.CostReconciliation {
	if toleranceBasisPoints <= 0 {
		toleranceBasisPoints = DefaultReconcileToleranceBasisPoints
	}
	attributed := map[string]int64{}
	unattributed := map[string]int64{}
	for _, c := range costs {
		cur := c.Amount.Currency
		if cur == "" {
			continue
		}
		if c.ResourceID == domain.UnattributedResourceID || c.Attribution.Method == domain.AttributionUnattributed {
			unattributed[cur] += c.Amount.AmountMinor
		} else {
			attributed[cur] += c.Amount.AmountMinor
		}
	}
	sourceMinor := map[string]int64{}
	for cur, m := range sourceTotals {
		sourceMinor[cur] = m.AmountMinor
	}
	discrepancy := map[string]int64{}
	within := true
	currencies := unionKeys(sourceMinor, attributed, unattributed)
	for _, cur := range currencies {
		sum := attributed[cur] + unattributed[cur]
		src := sourceMinor[cur]
		diff := src - sum
		discrepancy[cur] = diff
		if !withinTolerance(src, diff, toleranceBasisPoints) {
			within = false
		}
	}
	return domain.CostReconciliation{
		SourceTotal:          toMoneyMap(sourceMinor),
		AttributedTotal:      toMoneyMap(attributed),
		UnattributedTotal:    toMoneyMap(unattributed),
		Discrepancy:          toMoneyMap(discrepancy),
		WithinTolerance:      within,
		ToleranceBasisPoints: toleranceBasisPoints,
	}
}

func withinTolerance(source, diff int64, toleranceBasisPoints int64) bool {
	if source == 0 {
		return diff == 0
	}
	allowed := int64(math.Ceil(math.Abs(float64(source)) * float64(toleranceBasisPoints) / 10000.0))
	if allowed < 1 {
		allowed = 1
	}
	return abs64(diff) <= allowed
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func unionKeys(maps ...map[string]int64) []string {
	seen := map[string]struct{}{}
	for _, m := range maps {
		for k := range m {
			seen[k] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func toMoneyMap(minor map[string]int64) map[string]types.Money {
	out := make(map[string]types.Money, len(minor))
	for cur, amt := range minor {
		out[cur] = types.Money{AmountMinor: amt, Currency: cur}
	}
	return out
}

// InventoryIndex maps provider resource IDs to canonical IDs and tag owners.
type InventoryIndex struct {
	ByProviderID map[string]types.ResourceID
	OwnerByRes   map[types.ResourceID]string
	Resources    []domain.Resource
}

// BuildInventoryIndex indexes snapshot resources for attribution.
func BuildInventoryIndex(snap *domain.CollectionSnapshot) InventoryIndex {
	idx := InventoryIndex{
		ByProviderID: make(map[string]types.ResourceID),
		OwnerByRes:   make(map[types.ResourceID]string),
	}
	if snap == nil {
		return idx
	}
	idx.Resources = append([]domain.Resource{}, snap.Resources...)
	for _, res := range snap.Resources {
		if res.ProviderResourceID != "" {
			idx.ByProviderID[res.ProviderResourceID] = res.ID
		}
		for _, tag := range res.Tags {
			if tag.Key == "Owner" {
				idx.OwnerByRes[res.ID] = tag.Value
			}
		}
	}
	return idx
}

// AttributionInput is one normalized billing row before resource mapping.
type AttributionInput struct {
	ProviderResourceID string
	Service            string
	Region             string
	Amount             types.Money
	Basis              domain.CostBasis
	ChargeKind         domain.CostChargeKind
	Granularity        domain.CostGranularity
	PeriodStart        types.Timestamp
	PeriodEnd          types.Timestamp
	SharedPool         bool
	TagOwner           string
}

// Attribute maps billing inputs to canonical cost records using documented heuristics.
func Attribute(inputs []AttributionInput, idx InventoryIndex, interval domain.BillingInterval, source string, observed types.Timestamp) []domain.CostRecord {
	var out []domain.CostRecord
	for _, in := range inputs {
		out = append(out, attributeOne(in, idx, interval, source, observed)...)
	}
	return out
}

func attributeOne(in AttributionInput, idx InventoryIndex, interval domain.BillingInterval, source string, observed types.Timestamp) []domain.CostRecord {
	prov := domain.Provenance{Quality: domain.QualityObserved, Source: source, ObservedAt: observed}
	regionID := types.RegionID("")
	if in.Region != "" {
		regionID = types.RegionID("reg-" + in.Region)
	}
	base := domain.CostRecord{
		Service:        in.Service,
		RegionID:       regionID,
		Amount:         in.Amount,
		Basis:          in.Basis,
		ChargeKind:     in.ChargeKind,
		Granularity:    in.Granularity,
		PeriodStart:    in.PeriodStart,
		PeriodEnd:      in.PeriodEnd,
		SourceInterval: interval,
		Provenance:     prov,
	}
	if in.ProviderResourceID != "" {
		if id, ok := idx.ByProviderID[in.ProviderResourceID]; ok {
			rec := base
			rec.ResourceID = id
			rec.Attribution = domain.CostAttribution{
				Method:      domain.AttributionDirectResourceID,
				HeuristicID: "aws_ce_resource_id",
				Confidence:  0.95,
			}
			return []domain.CostRecord{rec}
		}
	}
	if in.TagOwner != "" {
		var matches []domain.Resource
		for _, res := range idx.Resources {
			if idx.OwnerByRes[res.ID] == in.TagOwner {
				matches = append(matches, res)
			}
		}
		if len(matches) == 1 {
			rec := base
			rec.ResourceID = matches[0].ID
			rec.Attribution = domain.CostAttribution{
				Method:      domain.AttributionTagOwner,
				HeuristicID: "owner_tag_single_match",
				Confidence:  0.6,
			}
			return []domain.CostRecord{rec}
		}
	}
	if in.SharedPool {
		var pool []domain.Resource
		targetRegion := types.RegionID("")
		if in.Region != "" && in.Region != "global" {
			targetRegion = types.RegionID("reg-" + in.Region)
		}
		for _, res := range idx.Resources {
			if targetRegion == "" || res.RegionID == targetRegion {
				pool = append(pool, res)
			}
		}
		if len(pool) > 0 {
			share, err := splitEven(in.Amount, len(pool))
			if err != nil {
				return unattributedRow(base, "shared_split_currency_mismatch")
			}
			var rows []domain.CostRecord
			for _, res := range pool {
				rec := base
				rec.Amount = share
				rec.ResourceID = res.ID
				rec.Attribution = domain.CostAttribution{
					Method:      domain.AttributionSharedService,
					HeuristicID: "even_split_regional_inventory",
					Confidence:  0.35,
				}
				rows = append(rows, rec)
			}
			return rows
		}
	}
	return unattributedRow(base, "no_matching_resource")
}

func splitEven(total types.Money, n int) (types.Money, error) {
	if n <= 0 {
		return types.Money{}, fmt.Errorf("invalid split count")
	}
	minor := total.AmountMinor / int64(n)
	return types.Money{AmountMinor: minor, Currency: total.Currency}, nil
}

func unattributedRow(base domain.CostRecord, heuristic string) []domain.CostRecord {
	rec := base
	rec.ResourceID = domain.UnattributedResourceID
	rec.Attribution = domain.CostAttribution{
		Method:      domain.AttributionUnattributed,
		HeuristicID: heuristic,
		Confidence:  1.0,
	}
	if rec.Provenance.Quality == domain.QualityObserved {
		rec.Provenance.Quality = domain.QualityDerived
	}
	return []domain.CostRecord{rec}
}
