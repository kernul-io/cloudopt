package pricing

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// GCEHourlyMinor returns on-demand GCE hourly price in minor units.
func (c *Catalog) GCEHourlyMinor(region, machineType string) (int64, string, bool) {
	if c == nil {
		return 0, "", false
	}
	for _, r := range c.Records {
		if r.Service != "ComputeEngine" || r.Region != region || r.Unit != "hour" {
			continue
		}
		if r.PurchaseModel != "" && r.PurchaseModel != "on_demand" {
			continue
		}
		if r.Attributes["machine_type"] == machineType {
			return r.PriceMinor, r.Currency, true
		}
	}
	return 0, "", false
}

// PDPerGBMonthMinor returns persistent disk price per GB-month.
func (c *Catalog) PDPerGBMonthMinor(region, diskType string) (int64, string, bool) {
	if c == nil {
		return 0, "", false
	}
	for _, r := range c.Records {
		if r.Service != "PersistentDisk" || r.Region != region {
			continue
		}
		if r.Attributes["disk_type"] == diskType && r.Unit == "gb_month" {
			return r.PriceMinor, r.Currency, true
		}
	}
	return 0, "", false
}

// CloudSQLHourlyMinor returns Cloud SQL tier hourly price.
func (c *Catalog) CloudSQLHourlyMinor(region, tier, engine string) (int64, string, bool) {
	if c == nil {
		return 0, "", false
	}
	for _, r := range c.Records {
		if r.Service != "CloudSQL" || r.Region != region {
			continue
		}
		if r.Attributes["tier"] == tier && r.Attributes["engine"] == engine && r.Unit == "hour" {
			return r.PriceMinor, r.Currency, true
		}
	}
	return 0, "", false
}

// GCECandidateConfig controls GCE rightsizing constraints.
type GCECandidateConfig struct {
	HeadroomBasisPoints int64
}

// GCECandidateSelection is the outcome of GCE candidate picking.
type GCECandidateSelection struct {
	CurrentType         string
	BaselineHourlyMinor int64
	Currency            string
	Accepted            *GCEAcceptedCandidate
	Rejected            []CandidateRejection
}

// GCEAcceptedCandidate is the chosen downsize target.
type GCEAcceptedCandidate struct {
	TargetType          string
	TargetHourlyMinor   int64
	BaselineHourlyMinor int64
	Currency            string
}

// SortedGCEAlternatives returns same-family smaller machine types with prices.
func (c *Catalog) SortedGCEAlternatives(region, currentType string, cfg GCECandidateConfig) GCECandidateSelection {
	sel := GCECandidateSelection{CurrentType: currentType}
	if c == nil {
		sel.Rejected = append(sel.Rejected, CandidateRejection{TargetType: currentType, Reason: "pricing catalog unavailable"})
		return sel
	}
	curMinor, curCur, ok := c.GCEHourlyMinor(region, currentType)
	if !ok {
		sel.Rejected = append(sel.Rejected, CandidateRejection{TargetType: currentType, Reason: "current machine type not in pricing catalog"})
		return sel
	}
	sel.BaselineHourlyMinor = curMinor
	sel.Currency = curCur

	var candidates []gceAlt
	for _, r := range c.Records {
		if r.Service != "ComputeEngine" || r.Region != region || r.Unit != "hour" {
			continue
		}
		if r.PurchaseModel != "" && r.PurchaseModel != "on_demand" {
			continue
		}
		target := r.Attributes["machine_type"]
		if target == "" || target == currentType {
			continue
		}
		if reason := rejectGCEAlternative(currentType, target, r.Attributes, cfg); reason != "" {
			sel.Rejected = append(sel.Rejected, CandidateRejection{TargetType: target, Reason: reason})
			continue
		}
		if r.PriceMinor >= curMinor {
			sel.Rejected = append(sel.Rejected, CandidateRejection{TargetType: target, Reason: "not cheaper than current on-demand rate"})
			continue
		}
		candidates = append(candidates, gceAlt{typ: target, hourly: r.PriceMinor})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].hourly == candidates[j].hourly {
			return candidates[i].typ < candidates[j].typ
		}
		return candidates[i].hourly < candidates[j].hourly
	})
	if len(candidates) == 0 {
		sel.Rejected = append(sel.Rejected, CandidateRejection{TargetType: "", Reason: "no compatible cheaper machine types in catalog"})
		return sel
	}
	best := candidates[0]
	sel.Accepted = &GCEAcceptedCandidate{
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

type gceAlt struct {
	typ    string
	hourly int64
}

func rejectGCEAlternative(current, target string, attrs map[string]string, cfg GCECandidateConfig) string {
	curFam, curVCPU, okCur := parseGCEMachineType(current)
	tgtFam, tgtVCPU, okTgt := parseGCEMachineType(target)
	if !okCur || !okTgt {
		return "unrecognized machine type format or custom shape"
	}
	if curFam != tgtFam {
		return fmt.Sprintf("incompatible machine family (%s vs %s)", curFam, tgtFam)
	}
	if tgtVCPU >= curVCPU {
		return "target vCPU count is not smaller than current"
	}
	if attrs["shared_core"] == "true" && cfg.HeadroomBasisPoints < 2000 {
		return "shared-core machine requires higher safety headroom"
	}
	if strings.Contains(current, "custom") || strings.Contains(target, "custom") {
		return "custom machine types require manual validation"
	}
	return ""
}

// GCPDiskPerGBMonthMinor aliases persistent disk pricing lookup.
func (c *Catalog) GCPDiskPerGBMonthMinor(region, diskType string) (int64, string, bool) {
	return c.PDPerGBMonthMinor(region, diskType)
}

// GCPNATHourlyMinor returns Cloud NAT gateway hourly price.
func (c *Catalog) GCPNATHourlyMinor(region string) (int64, string, bool) {
	if c == nil {
		return 0, "", false
	}
	for _, r := range c.Records {
		if r.Service != "CloudNAT" || r.Region != region {
			continue
		}
		if r.Attributes["resource"] == "nat_gateway" && r.Unit == "hour" {
			return r.PriceMinor, r.Currency, true
		}
	}
	return 0, "", false
}

func parseGCEMachineType(s string) (family string, vcpu int, ok bool) {
	parts := strings.Split(s, "-")
	if len(parts) < 2 {
		return "", 0, false
	}
	family = parts[0]
	if len(parts) >= 3 && parts[1] == "standard" {
		v, err := strconv.Atoi(parts[2])
		if err != nil {
			return family, 0, false
		}
		return family, v, true
	}
	if len(parts) >= 2 && parts[0] == "e2" {
		// e2-medium, e2-small — treat medium=2 vcpu, small=2 shared
		switch parts[1] {
		case "medium":
			return "e2", 2, true
		case "small":
			return "e2", 2, true
		case "micro":
			return "e2", 1, true
		}
	}
	return family, 0, false
}
