package savings

import (
	"fmt"
	"math"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
)

const defaultPricingUncertaintyBps int64 = 500 // ±5%

// Policy configures conservative savings aggregation.
type Policy struct {
	HeadroomBasisPoints      int64
	MinConfidenceBasisPoints int64
	PricingUncertaintyBps    int64
}

// DefaultPolicy returns conservative defaults.
func DefaultPolicy() Policy {
	return Policy{
		HeadroomBasisPoints:      1500,
		MinConfidenceBasisPoints: 7000,
		PricingUncertaintyBps:    defaultPricingUncertaintyBps,
	}
}

// MonthlyRightsizingFromHourly computes gross monthly savings between hourly rates.
func MonthlyRightsizingFromHourly(baselineHourlyMinor, candidateHourlyMinor int64, currency string, headroomBps int64, inputs map[string]string) domain.SavingsEstimate {
	if baselineHourlyMinor <= candidateHourlyMinor {
		return domain.SavingsEstimate{Currency: currency, Class: domain.SavingsMonthlyRecurring, Inputs: inputs}
	}
	grossHourly := baselineHourlyMinor - candidateHourlyMinor
	adjHourly := applyHeadroom(grossHourly, headroomBps)
	grossMonthly := adjHourly * 730
	low, high := uncertaintyBand(grossMonthly, defaultPricingUncertaintyBps)
	return domain.SavingsEstimate{
		BaselineMinor:     baselineHourlyMinor * 730,
		CandidateMinor:    candidateHourlyMinor * 730,
		GrossMonthlyMinor: grossMonthly,
		LowMonthlyMinor:   low,
		HighMonthlyMinor:  high,
		Currency:          currency,
		Class:             domain.SavingsMonthlyRecurring,
		Inputs:            inputs,
	}
}

// MonthlyEBSMigration computes savings moving volume types at a given size GiB.
func MonthlyEBSMigration(sizeGiB int64, fromMinor, toMinor int64, currency string, inputs map[string]string) domain.SavingsEstimate {
	if sizeGiB <= 0 || fromMinor <= toMinor {
		return domain.SavingsEstimate{Currency: currency, Class: domain.SavingsMonthlyRecurring, Inputs: inputs}
	}
	gross := (fromMinor - toMinor) * sizeGiB
	low, high := uncertaintyBand(gross, defaultPricingUncertaintyBps)
	return domain.SavingsEstimate{
		BaselineMinor:     fromMinor * sizeGiB,
		CandidateMinor:    toMinor * sizeGiB,
		GrossMonthlyMinor: gross,
		LowMonthlyMinor:   low,
		HighMonthlyMinor:  high,
		Currency:          currency,
		Class:             domain.SavingsMonthlyRecurring,
		Inputs:            inputs,
	}
}

func applyHeadroom(grossHourly int64, headroomBps int64) int64 {
	if headroomBps <= 0 {
		return grossHourly
	}
	reduction := grossHourly * headroomBps / 10000
	out := grossHourly - reduction
	if out < 0 {
		return 0
	}
	return out
}

func uncertaintyBand(gross int64, bps int64) (low, high int64) {
	if gross <= 0 {
		return 0, 0
	}
	delta := gross * bps / 10000
	low = gross - delta
	if low < 0 {
		low = 0
	}
	high = gross + delta
	return low, high
}

// OverlapDecision clears savings on lower-priority overlapping recommendations.
func ApplyOverlapPolicy(findings []domain.Finding, recs *[]domain.Recommendation) {
	if recs == nil || len(*recs) == 0 {
		return
	}
	type keyed struct {
		idx        int
		overlapKey string
		priority   int
		savings    int64
	}
	byKey := map[string][]keyed{}
	for i := range *recs {
		rec := &(*recs)[i]
		if rec.OverlapKey == "" || rec.EstSavings == nil {
			continue
		}
		pri := overlapPriority(rec.OverlapKey)
		byKey[rec.OverlapKey] = append(byKey[rec.OverlapKey], keyed{
			idx:        i,
			overlapKey: rec.OverlapKey,
			priority:   pri,
			savings:    rec.EstSavings.AmountMinor,
		})
	}
	for key, group := range byKey {
		if len(group) < 2 {
			continue
		}
		winner := group[0]
		for _, g := range group[1:] {
			if g.priority > winner.priority || (g.priority == winner.priority && g.savings > winner.savings) {
				winner = g
			}
		}
		for _, g := range group {
			if g.idx == winner.idx {
				continue
			}
			rec := &(*recs)[g.idx]
			clearSavings(rec, fmt.Sprintf("overlaps with finding on %s; savings excluded from totals", key))
			if fIdx := findingIndex(findings, rec.FindingID); fIdx >= 0 {
				findings[fIdx].Assumptions = append(findings[fIdx].Assumptions,
					"Savings suppressed due to overlapping recommendation on the same resource.")
			}
		}
	}
}

func overlapPriority(overlapKey string) int {
	switch {
	case overlapKey == "":
		return 0
	default:
		return 1
	}
}

func clearSavings(rec *domain.Recommendation, note string) {
	rec.EstSavings = nil
	rec.EstSavingsLow = nil
	rec.EstSavingsHigh = nil
	rec.InvestigationOnly = true
	if rec.SavingsInputs == nil {
		rec.SavingsInputs = map[string]string{}
	}
	rec.SavingsInputs["overlap_suppressed"] = "true"
	rec.SavingsInputs["overlap_note"] = note
}

func findingIndex(findings []domain.Finding, id types.FindingID) int {
	for i, f := range findings {
		if f.ID == id {
			return i
		}
	}
	return -1
}

// AggregateMonthlyTotals sums non-overlapping, non-investigation monthly savings by currency.
func AggregateMonthlyTotals(recs []domain.Recommendation) (low, high map[string]float64) {
	low = map[string]float64{}
	high = map[string]float64{}
	for _, rec := range recs {
		if rec.InvestigationOnly || rec.EstSavings == nil {
			continue
		}
		if rec.SavingsClass != "" && rec.SavingsClass != domain.SavingsMonthlyRecurring {
			continue
		}
		cur := rec.EstSavings.Currency
		mid := float64(rec.EstSavings.AmountMinor) / 100.0
		low[cur] += mid
		high[cur] += mid
		if rec.EstSavingsLow != nil && rec.EstSavingsLow.Currency == cur {
			low[cur] -= mid
			low[cur] += float64(rec.EstSavingsLow.AmountMinor) / 100.0
		}
		if rec.EstSavingsHigh != nil && rec.EstSavingsHigh.Currency == cur {
			high[cur] -= mid
			high[cur] += float64(rec.EstSavingsHigh.AmountMinor) / 100.0
		}
	}
	for cur := range low {
		low[cur] = math.Max(0, low[cur])
		high[cur] = math.Max(low[cur], high[cur])
	}
	return low, high
}
