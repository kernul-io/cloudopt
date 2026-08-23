package awsbilling

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	appbilling "github.com/kernul-io/cloudopt/internal/application/billing"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain"
	domaintypes "github.com/kernul-io/cloudopt/internal/domain/types"
)

const collectorSource = "aws-billing/cost-explorer"

// CEAPI is the Cost Explorer surface used by the collector (mockable in tests).
type CEAPI interface {
	GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
}

// Collector collects AWS billing via Cost Explorer.
type Collector struct {
	STS            STSAPI
	CE             CEAPI
	CapabilitiesFn func() (ports.CapabilityManifest, error)
	FixtureLoader  func(root string) ([]ceResultByTime, error)
}

type STSAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

func NewCollector(sts STSAPI, ce CEAPI) *Collector {
	return &Collector{
		STS: sts,
		CE:  ce,
		CapabilitiesFn: func() (ports.CapabilityManifest, error) {
			return LoadCapabilities()
		},
		FixtureLoader: loadFixtures,
	}
}

func (c *Collector) Capabilities() ports.CapabilityManifest {
	m, err := c.CapabilitiesFn()
	if err != nil {
		return ports.CapabilityManifest{Provider: domaintypes.ProviderAWS}
	}
	return m
}

func (c *Collector) Preflight(ctx context.Context, opts ports.CostCollectOptions) (*ports.BillingPreflight, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	caps, err := c.CapabilitiesFn()
	if err != nil {
		return nil, err
	}
	lookback := opts.LookbackDays
	if lookback <= 0 {
		lookback = 30
	}
	pf := &ports.BillingPreflight{
		LookbackDays: lookback,
		Capabilities: caps,
	}
	if opts.Offline {
		pf.ProviderAccountID = "000000000000"
		pf.CallerARN = "arn:aws:iam::000000000000:user/offline-fixture"
		return pf, nil
	}
	identity, err := c.STS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("caller identity: %w", err)
	}
	pf.ProviderAccountID = aws.ToString(identity.Account)
	pf.CallerARN = aws.ToString(identity.Arn)
	pf.MissingActions = c.probeCE(ctx, lookback)
	return pf, nil
}

func (c *Collector) Collect(ctx context.Context, opts ports.CostCollectOptions, inventory *domain.CollectionSnapshot) (*ports.BillingCollectResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pf, err := c.Preflight(ctx, opts)
	if err != nil {
		return nil, err
	}
	if opts.DryRun {
		return &ports.BillingCollectResult{}, nil
	}
	lookback := pf.LookbackDays
	start, end := billingWindow(time.Now().UTC(), lookback)
	interval := domain.BillingInterval{
		Start:     domaintypes.NewTimestamp(start),
		End:       domaintypes.NewTimestamp(end),
		Collected: domaintypes.NowUTC(),
	}
	obs := interval.Collected

	if !opts.Offline && len(pf.MissingActions) > 0 {
		return nil, ports.ErrMissingPermissions(pf.MissingActions)
	}

	var pages []ceResultByTime
	if opts.Offline {
		root := opts.FixtureRoot
		if root == "" {
			root = "testdata/aws-billing"
		}
		pages, err = c.FixtureLoader(root)
	} else {
		pages, err = c.fetchLive(ctx, start, end)
	}
	if err != nil {
		return nil, err
	}

	inputs, sourceTotals, diag := normalizeCEPages(pages, interval)
	idx := appbilling.BuildInventoryIndex(inventory)
	costs := appbilling.Attribute(inputs, idx, interval, collectorSource, obs)

	partial := false
	coverage := []domain.ServiceCollectionStatus{{
		Service: "cost_explorer",
		Status:  domain.ServiceCollectionOK,
	}}
	if len(diag) > 0 {
		partial = true
		coverage[0].Status = domain.ServiceCollectionPartial
		coverage[0].Message = "see cost diagnostics"
	}

	return &ports.BillingCollectResult{
		Costs:        costs,
		SourceTotals: sourceTotals,
		Interval:     interval,
		Coverage:     coverage,
		Diagnostics:  diag,
		Partial:      partial,
	}, nil
}

func (c *Collector) probeCE(ctx context.Context, lookback int) []string {
	start, end := billingWindow(time.Now().UTC(), lookback)
	_, err := c.CE.GetCostAndUsage(ctx, &costexplorer.GetCostAndUsageInput{
		TimePeriod:  &types.DateInterval{Start: aws.String(formatCEDate(start)), End: aws.String(formatCEDate(end))},
		Granularity: types.GranularityMonthly,
		Metrics:     []string{"UnblendedCost"},
	})
	if err != nil {
		if isAccessDenied(err) {
			return []string{"ce:GetCostAndUsage", "ce:GetDimensionValues"}
		}
	}
	return nil
}

func (c *Collector) fetchLive(ctx context.Context, start, end time.Time) ([]ceResultByTime, error) {
	var out []ceResultByTime
	token := (*string)(nil)
	for {
		resp, err := c.CE.GetCostAndUsage(ctx, &costexplorer.GetCostAndUsageInput{
			TimePeriod: &types.DateInterval{
				Start: aws.String(formatCEDate(start)),
				End:   aws.String(formatCEDate(end)),
			},
			Granularity: types.GranularityMonthly,
			Metrics: []string{
				"AmortizedCost",
				"NetAmortizedCost",
				"UnblendedCost",
			},
			GroupBy: []types.GroupDefinition{
				{Type: types.GroupDefinitionTypeDimension, Key: aws.String("SERVICE")},
				{Type: types.GroupDefinitionTypeDimension, Key: aws.String("REGION")},
			},
			NextPageToken: token,
		})
		if err != nil {
			return nil, mapCEError(err)
		}
		out = append(out, mapSDKResults(resp.ResultsByTime)...)
		if resp.NextPageToken == nil || *resp.NextPageToken == "" {
			break
		}
		token = resp.NextPageToken
	}
	return out, nil
}

func billingWindow(now time.Time, lookbackDays int) (time.Time, time.Time) {
	if lookbackDays <= 0 {
		lookbackDays = 30
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	start := end.AddDate(0, 0, -lookbackDays)
	return start, end
}

func formatCEDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// --- fixture JSON (provider-shaped, not stored in domain) ---

type ceFixtureFile struct {
	ResultsByTime []ceResultByTime `json:"ResultsByTime"`
}

type ceResultByTime struct {
	TimePeriod ceDateInterval `json:"TimePeriod"`
	Groups     []ceGroup      `json:"Groups"`
	Total      ceMetrics      `json:"Total"`
	Estimated  bool           `json:"Estimated"`
}

type ceDateInterval struct {
	Start string `json:"Start"`
	End   string `json:"End"`
}

type ceGroup struct {
	Keys    []string  `json:"Keys"`
	Metrics ceMetrics `json:"Metrics"`
}

type ceMetrics struct {
	AmortizedCost    ceMetricValue `json:"AmortizedCost"`
	NetAmortizedCost ceMetricValue `json:"NetAmortizedCost"`
	UnblendedCost    ceMetricValue `json:"UnblendedCost"`
}

type ceMetricValue struct {
	Amount string `json:"Amount"`
	Unit   string `json:"Unit"`
}

func loadFixtures(root string) ([]ceResultByTime, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read fixture root: %w", err)
	}
	var pages []ceResultByTime
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, ent.Name()))
		if err != nil {
			return nil, err
		}
		var doc ceFixtureFile
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", ent.Name(), err)
		}
		pages = append(pages, doc.ResultsByTime...)
	}
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].TimePeriod.Start < pages[j].TimePeriod.Start
	})
	return pages, nil
}

func normalizeCEPages(pages []ceResultByTime, interval domain.BillingInterval) ([]appbilling.AttributionInput, map[string]domaintypes.Money, []domain.CostDiagnostic) {
	var inputs []appbilling.AttributionInput
	sourceTotals := map[string]domaintypes.Money{}
	var diagnostics []domain.CostDiagnostic

	for _, page := range pages {
		start, err1 := parseCEDate(page.TimePeriod.Start)
		end, err2 := parseCEDate(page.TimePeriod.End)
		if err1 != nil || err2 != nil {
			diagnostics = append(diagnostics, domain.CostDiagnostic{
				Code: "invalid_period", Message: "skipped malformed billing period", Severity: "warn",
			})
			continue
		}
		if page.Estimated {
			diagnostics = append(diagnostics, domain.CostDiagnostic{
				Code: "incomplete_month", Message: "current month total is estimated/incomplete", Severity: "info",
			})
		}
		granularity := domain.CostMonthly
		if end.Sub(start) <= 32*24*time.Hour && end.Sub(start) < 31*24*time.Hour {
			granularity = domain.CostDaily
		}
		for cur, amt := range totalsFromMetrics(page.Total) {
			if prev, ok := sourceTotals[cur]; ok {
				sum, err := prev.Add(amt)
				if err != nil {
					diagnostics = append(diagnostics, domain.CostDiagnostic{
						Code: "currency_mismatch", Message: "mixed currencies in source totals", Severity: "error",
					})
					continue
				}
				sourceTotals[cur] = sum
			} else {
				sourceTotals[cur] = amt
			}
		}
		for _, g := range page.Groups {
			in := groupToInput(g, granularity, start, end)
			if in.Amount.Currency == "" {
				continue
			}
			inputs = append(inputs, in)
		}
	}
	_ = interval
	return inputs, sourceTotals, diagnostics
}

func groupToInput(g ceGroup, granularity domain.CostGranularity, start, end time.Time) appbilling.AttributionInput {
	service, region, resourceID := "", "", ""
	switch len(g.Keys) {
	case 1:
		service = g.Keys[0]
	case 2:
		service, region = g.Keys[0], g.Keys[1]
	default:
		if len(g.Keys) >= 3 {
			service, region, resourceID = g.Keys[0], g.Keys[1], g.Keys[2]
		}
	}
	amount, basis, kind := pickPrimaryAmount(g.Metrics)
	in := appbilling.AttributionInput{
		ProviderResourceID: strings.TrimSpace(resourceID),
		Service:            service,
		Region:             region,
		Amount:             amount,
		Basis:              basis,
		ChargeKind:         kind,
		Granularity:        granularity,
		PeriodStart:        domaintypes.NewTimestamp(start),
		PeriodEnd:          domaintypes.NewTimestamp(end),
	}
	if strings.HasPrefix(resourceID, "tag-owner:") {
		in.ProviderResourceID = ""
		in.TagOwner = strings.TrimPrefix(resourceID, "tag-owner:")
	}
	if strings.EqualFold(service, "AWS Support") || strings.Contains(strings.ToLower(service), "support") {
		in.ChargeKind = domain.ChargeSupport
	}
	if amount.AmountMinor < 0 {
		if kind == domain.ChargeUsage {
			in.ChargeKind = domain.ChargeCredit
		}
	}
	if len(g.Keys) > 0 && strings.EqualFold(g.Keys[len(g.Keys)-1], "shared") {
		in.SharedPool = true
		in.ProviderResourceID = ""
	}
	return in
}

func pickPrimaryAmount(m ceMetrics) (domaintypes.Money, domain.CostBasis, domain.CostChargeKind) {
	// Default optimization view: amortized net cost.
	if m.NetAmortizedCost.Amount != "" {
		amt, _ := parseCEMoney(m.NetAmortizedCost)
		return amt, domain.CostBasisAmortizedNet, chargeKindFromAmount(amt.AmountMinor)
	}
	if m.AmortizedCost.Amount != "" {
		amt, _ := parseCEMoney(m.AmortizedCost)
		return amt, domain.CostBasisAmortized, chargeKindFromAmount(amt.AmountMinor)
	}
	amt, _ := parseCEMoney(m.UnblendedCost)
	return amt, domain.CostBasisUnblended, chargeKindFromAmount(amt.AmountMinor)
}

func chargeKindFromAmount(minor int64) domain.CostChargeKind {
	if minor < 0 {
		return domain.ChargeCredit
	}
	return domain.ChargeUsage
}

func parseCEMoney(v ceMetricValue) (domaintypes.Money, string) {
	major, err := strconv.ParseFloat(strings.TrimSpace(v.Amount), 64)
	if err != nil {
		return domaintypes.Money{}, v.Unit
	}
	cur := v.Unit
	if cur == "" {
		cur = "USD"
	}
	return domaintypes.FromMajorUnits(major, cur, 100), cur
}

func totalsFromMetrics(m ceMetrics) map[string]domaintypes.Money {
	amt, _, _ := pickPrimaryAmount(m)
	if amt.Currency == "" {
		return nil
	}
	return map[string]domaintypes.Money{amt.Currency: amt}
}

func parseCEDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", strings.TrimSpace(s))
}

func mapSDKResults(in []types.ResultByTime) []ceResultByTime {
	var out []ceResultByTime
	for _, r := range in {
		page := ceResultByTime{
			TimePeriod: ceDateInterval{
				Start: aws.ToString(r.TimePeriod.Start),
				End:   aws.ToString(r.TimePeriod.End),
			},
			Estimated: r.Estimated,
		}
		if r.Total != nil {
			page.Total = mapSDKMetrics(r.Total)
		}
		for _, g := range r.Groups {
			page.Groups = append(page.Groups, ceGroup{
				Keys:    append([]string{}, g.Keys...),
				Metrics: mapSDKMetrics(g.Metrics),
			})
		}
		out = append(out, page)
	}
	return out
}

func mapSDKMetrics(m map[string]types.MetricValue) ceMetrics {
	out := ceMetrics{}
	if v, ok := m["AmortizedCost"]; ok {
		out.AmortizedCost = ceMetricValue{Amount: aws.ToString(v.Amount), Unit: aws.ToString(v.Unit)}
	}
	if v, ok := m["NetAmortizedCost"]; ok {
		out.NetAmortizedCost = ceMetricValue{Amount: aws.ToString(v.Amount), Unit: aws.ToString(v.Unit)}
	}
	if v, ok := m["UnblendedCost"]; ok {
		out.UnblendedCost = ceMetricValue{Amount: aws.ToString(v.Amount), Unit: aws.ToString(v.Unit)}
	}
	return out
}

// NewLiveBillingSource builds a Cost Explorer collector with optional role assumption.
func NewLiveBillingSource(ctx context.Context, roleARN, externalID string) (ports.BillingSource, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	if roleARN != "" {
		stsClient := sts.NewFromConfig(cfg)
		provider := stscreds.NewAssumeRoleProvider(stsClient, roleARN, func(o *stscreds.AssumeRoleOptions) {
			if externalID != "" {
				o.ExternalID = aws.String(externalID)
			}
		})
		cfg.Credentials = aws.NewCredentialsCache(provider)
	}
	stsAPI := sts.NewFromConfig(cfg)
	ce := costexplorer.NewFromConfig(cfg)
	return NewCollector(stsAPI, ce), nil
}

// NewFixtureBillingSource uses recorded CE JSON fixtures.
func NewFixtureBillingSource(root string) ports.BillingSource {
	c := NewCollector(nil, nil)
	c.FixtureLoader = func(string) ([]ceResultByTime, error) {
		return loadFixtures(root)
	}
	return c
}
