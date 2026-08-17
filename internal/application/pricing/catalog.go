package pricing

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

const hoursPerMonth = 730

// Catalog indexes pricing records for deterministic lookups.
type Catalog struct {
	Records []domain.PricingRecord
	Source  string
}

// EmptyCatalog returns a catalog with no records.
func EmptyCatalog() *Catalog {
	return &Catalog{}
}

// NewCatalog builds lookup indexes from records.
func NewCatalog(records []domain.PricingRecord, source string) *Catalog {
	return &Catalog{Records: append([]domain.PricingRecord(nil), records...), Source: source}
}

func (c *Catalog) IsEmpty() bool {
	return c == nil || len(c.Records) == 0
}

// EC2HourlyMinor returns on-demand Linux shared-tenancy hourly price in minor units.
func (c *Catalog) EC2HourlyMinor(region, instanceType string) (int64, string, bool) {
	if c == nil {
		return 0, "", false
	}
	for _, r := range c.Records {
		if r.Service != "AmazonEC2" || r.Region != region || r.PurchaseModel != domain.PurchaseOnDemand {
			continue
		}
		if r.Attributes["instance_type"] == instanceType && r.Unit == "hour" {
			return r.PriceMinor, r.Currency, true
		}
	}
	return 0, "", false
}

// EBSPerGBMonthMinor returns volume type price per GB-month.
func (c *Catalog) EBSPerGBMonthMinor(region, volumeType string) (int64, string, bool) {
	if c == nil {
		return 0, "", false
	}
	for _, r := range c.Records {
		if r.Service != "AmazonEBS" || r.Region != region || r.PurchaseModel != domain.PurchaseOnDemand {
			continue
		}
		if r.Attributes["volume_type"] == volumeType && r.Unit == "gb_month" {
			return r.PriceMinor, r.Currency, true
		}
	}
	return 0, "", false
}

// RDSHourlyMinor returns RDS instance class hourly price.
func (c *Catalog) RDSHourlyMinor(region, instanceClass, engine string) (int64, string, bool) {
	if c == nil {
		return 0, "", false
	}
	for _, r := range c.Records {
		if r.Service != "AmazonRDS" || r.Region != region || r.PurchaseModel != domain.PurchaseOnDemand {
			continue
		}
		if r.Attributes["instance_class"] == instanceClass && r.Attributes["engine"] == engine && r.Unit == "hour" {
			return r.PriceMinor, r.Currency, true
		}
	}
	return 0, "", false
}

// NATHourlyMinor returns NAT gateway hourly price.
func (c *Catalog) NATHourlyMinor(region string) (int64, string, bool) {
	if c == nil {
		return 0, "", false
	}
	for _, r := range c.Records {
		if r.Service != "AmazonVPC" || r.Region != region {
			continue
		}
		if r.Attributes["resource"] == "nat_gateway" && r.Unit == "hour" {
			return r.PriceMinor, r.Currency, true
		}
	}
	return 0, "", false
}

// MonthlyFromHourly converts hourly minor price to monthly using fixed 730h/month.
func MonthlyFromHourly(hourlyMinor int64) int64 {
	return hourlyMinor * hoursPerMonth
}

// RecomputeInputs builds evidence detail for savings recomputation.
func RecomputeInputs(kind string, fields map[string]string) map[string]string {
	out := map[string]string{"pricing_kind": kind, "hours_per_month": fmt.Sprintf("%d", hoursPerMonth)}
	for k, v := range fields {
		out[k] = v
	}
	return out
}

// StalePricing returns true when catalog effective date is older than maxAge from observedAt.
func StalePricing(effective types.Timestamp, observedAt types.Timestamp, maxAge time.Duration) bool {
	if effective.IsZero() || observedAt.IsZero() {
		return false
	}
	return observedAt.Sub(effective.Time) > maxAge
}

// SortedEC2Alternatives returns same-family smaller instance types with prices, plus rejections.
func (c *Catalog) SortedEC2Alternatives(region, currentType string, cfg EC2CandidateConfig) EC2CandidateSelection {
	sel := EC2CandidateSelection{CurrentType: currentType}
	if c == nil {
		sel.Rejected = append(sel.Rejected, CandidateRejection{TargetType: currentType, Reason: "pricing catalog unavailable"})
		return sel
	}
	curMinor, curCur, ok := c.EC2HourlyMinor(region, currentType)
	if !ok {
		sel.Rejected = append(sel.Rejected, CandidateRejection{TargetType: currentType, Reason: "current instance type not in pricing catalog"})
		return sel
	}
	sel.BaselineHourlyMinor = curMinor
	sel.Currency = curCur

	var candidates []ec2Alt
	for _, r := range c.Records {
		if r.Service != "AmazonEC2" || r.Region != region || r.Unit != "hour" {
			continue
		}
		target := r.Attributes["instance_type"]
		if target == "" || target == currentType {
			continue
		}
		if reason := rejectEC2Alternative(currentType, target, cfg); reason != "" {
			sel.Rejected = append(sel.Rejected, CandidateRejection{TargetType: target, Reason: reason})
			continue
		}
		if r.PriceMinor >= curMinor {
			sel.Rejected = append(sel.Rejected, CandidateRejection{TargetType: target, Reason: "not cheaper than current on-demand rate"})
			continue
		}
		candidates = append(candidates, ec2Alt{typ: target, hourly: r.PriceMinor})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].hourly == candidates[j].hourly {
			return candidates[i].typ < candidates[j].typ
		}
		return candidates[i].hourly < candidates[j].hourly
	})
	if len(candidates) == 0 {
		sel.Rejected = append(sel.Rejected, CandidateRejection{TargetType: "", Reason: "no compatible cheaper instance types in catalog"})
		return sel
	}
	best := candidates[0]
	sel.Accepted = &EC2AcceptedCandidate{
		TargetType:          best.typ,
		TargetHourlyMinor:   best.hourly,
		BaselineHourlyMinor: curMinor,
		Currency:            curCur,
	}
	for _, alt := range candidates[1:] {
		sel.Rejected = append(sel.Rejected, CandidateRejection{
			TargetType: alt.typ,
			Reason:     "lower-priority alternative; best candidate selected deterministically",
		})
	}
	return sel
}

type ec2Alt struct {
	typ    string
	hourly int64
}

// EC2CandidateConfig controls headroom and architecture constraints.
type EC2CandidateConfig struct {
	HeadroomBasisPoints int64
}

// EC2CandidateSelection is the outcome of deterministic EC2 candidate picking.
type EC2CandidateSelection struct {
	CurrentType         string
	BaselineHourlyMinor int64
	Currency            string
	Accepted            *EC2AcceptedCandidate
	Rejected            []CandidateRejection
}

// EC2AcceptedCandidate is the chosen downsize target.
type EC2AcceptedCandidate struct {
	TargetType          string
	TargetHourlyMinor   int64
	BaselineHourlyMinor int64
	Currency            string
}

// CandidateRejection documents why an alternative was not chosen.
type CandidateRejection struct {
	TargetType string
	Reason     string
}

func rejectEC2Alternative(current, target string, cfg EC2CandidateConfig) string {
	curFam, curSize, okCur := parseInstanceType(current)
	tgtFam, tgtSize, okTgt := parseInstanceType(target)
	if !okCur || !okTgt {
		return "unrecognized instance type format"
	}
	if curFam != tgtFam {
		return fmt.Sprintf("incompatible instance family (%s vs %s); no architecture migration", curFam, tgtFam)
	}
	if instanceSizeRank(tgtSize) >= instanceSizeRank(curSize) {
		return "target size is not smaller than current"
	}
	if strings.HasPrefix(curFam, "t") && tgtSize == "micro" && cfg.HeadroomBasisPoints < 2000 {
		return "burstable micro size requires higher safety headroom"
	}
	return ""
}

func parseInstanceType(s string) (family, size string, ok bool) {
	parts := strings.Split(s, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

var sizeOrder = map[string]int{
	"nano": 0, "micro": 1, "small": 2, "medium": 3, "large": 4, "xlarge": 5,
	"2xlarge": 6, "3xlarge": 7, "4xlarge": 8,
}

func instanceSizeRank(size string) int {
	if strings.HasPrefix(size, "xlarge") && size != "xlarge" {
		// 2xlarge, 4xlarge, etc.
		if r, ok := sizeOrder[size]; ok {
			return r
		}
	}
	if r, ok := sizeOrder[size]; ok {
		return r
	}
	return 999
}
