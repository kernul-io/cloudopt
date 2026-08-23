package gcpbilling

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/compute/metadata"

	appbilling "github.com/kernul-io/cloudopt/internal/application/billing"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain"
	domaintypes "github.com/kernul-io/cloudopt/internal/domain/types"
)

// BQRunner executes export queries (mockable; live uses BigQuery REST pagination).
type BQRunner interface {
	QueryExport(ctx context.Context, project, dataset, table string, start, end time.Time, pageToken string) (rows []exportRow, nextToken string, err error)
	CallerProject(ctx context.Context) (string, error)
}

// Collector ingests Cloud Billing BigQuery export rows.
type Collector struct {
	BQ             BQRunner
	CapabilitiesFn func() (ports.CapabilityManifest, error)
	FixtureLoader  func(root string) (*billingFixture, error)
}

func NewCollector(bq BQRunner) *Collector {
	return &Collector{
		BQ: bq,
		CapabilitiesFn: func() (ports.CapabilityManifest, error) {
			return LoadCapabilities()
		},
		FixtureLoader: loadBillingFixture,
	}
}

func (c *Collector) Capabilities() ports.CapabilityManifest {
	m, err := c.CapabilitiesFn()
	if err != nil {
		return ports.CapabilityManifest{Provider: domaintypes.ProviderGCP}
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
		pf.ProviderAccountID = "billing-export-demo"
		pf.CallerARN = "user:offline-fixture@gcp"
		return pf, nil
	}
	if c.BQ == nil {
		return pf, nil
	}
	proj, err := c.BQ.CallerProject(ctx)
	if err != nil {
		return nil, errWrap("preflight", err)
	}
	pf.ProviderAccountID = proj
	pf.CallerARN = "adc:" + proj
	if opts.BillingExportProject == "" || opts.BigQueryDataset == "" || opts.BigQueryTable == "" {
		pf.MissingActions = []string{"bigquery.jobs.create", "configure billing export table"}
	}
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
	if !opts.Offline && len(pf.MissingActions) > 0 {
		return nil, ports.ErrMissingPermissions(pf.MissingActions)
	}
	lookback := pf.LookbackDays
	start, end := billingWindow(time.Now().UTC(), lookback)
	interval := domain.BillingInterval{
		Start:     domaintypes.NewTimestamp(start),
		End:       domaintypes.NewTimestamp(end),
		Collected: domaintypes.NowUTC(),
	}
	obs := interval.Collected

	if !opts.Offline {
		if opts.BillingExportProject == "" || opts.BigQueryDataset == "" || opts.BigQueryTable == "" {
			return nil, ExportNotConfigured{}
		}
		if c.BQ == nil {
			return nil, fmt.Errorf("gcp billing: BigQuery client not configured")
		}
	}

	var fixture *billingFixture
	if opts.Offline {
		root := opts.FixtureRoot
		if root == "" {
			root = "testdata/gcp-billing"
		}
		fixture, err = c.FixtureLoader(root)
		if err != nil {
			return nil, err
		}
	} else {
		fixture, err = c.fetchLivePages(ctx, opts, start, end)
		if err != nil {
			return nil, err
		}
	}

	inputs, sourceTotals, diag := normalizeExport(fixture, interval)
	idx := appbilling.BuildInventoryIndex(inventory)
	costs := appbilling.Attribute(inputs, idx, interval, collectorSource, obs)

	partial := fixture != nil && (fixture.IncompleteMonth || fixture.BillingLagDays > 0 || len(diag) > 0)
	coverage := []domain.ServiceCollectionStatus{{
		Service: "bigquery_billing_export",
		Status:  domain.ServiceCollectionOK,
	}}
	if partial {
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

func (c *Collector) fetchLivePages(ctx context.Context, opts ports.CostCollectOptions, start, end time.Time) (*billingFixture, error) {
	var all []exportRow
	token := ""
	for {
		page, next, err := c.BQ.QueryExport(ctx, opts.BillingExportProject, opts.BigQueryDataset, opts.BigQueryTable, start, end, token)
		if err != nil {
			return nil, errWrap("query export", err)
		}
		all = append(all, page...)
		if next == "" {
			break
		}
		token = next
	}
	return &billingFixture{Rows: all}, nil
}

func billingWindow(now time.Time, lookbackDays int) (time.Time, time.Time) {
	if lookbackDays <= 0 {
		lookbackDays = 30
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	start := end.AddDate(0, 0, -lookbackDays)
	return start, end
}

type billingFixture struct {
	Rows            []exportRow          `json:"rows"`
	SourceTotals    map[string]moneyJSON `json:"source_totals"`
	IncompleteMonth bool                 `json:"incomplete_month,omitempty"`
	BillingLagDays  int                  `json:"billing_lag_days,omitempty"`
	Pages           []billingFixturePage `json:"pages,omitempty"`
}

type billingFixturePage struct {
	Rows []exportRow `json:"rows"`
}

type moneyJSON struct {
	AmountMajor float64 `json:"amount_major"`
	Currency    string  `json:"currency"`
}

type exportRow struct {
	Service      string  `json:"service"`
	SKU          string  `json:"sku,omitempty"`
	Region       string  `json:"region"`
	ResourceName string  `json:"resource_name"`
	CostMajor    float64 `json:"cost"`
	Currency     string  `json:"currency"`
	CostType     string  `json:"cost_type,omitempty"`
	Basis        string  `json:"basis,omitempty"`
	PeriodStart  string  `json:"period_start"`
	PeriodEnd    string  `json:"period_end"`
	LabelOwner   string  `json:"label_owner,omitempty"`
	SharedPool   bool    `json:"shared_pool,omitempty"`
	Commitment   bool    `json:"commitment,omitempty"`
	SUD          bool    `json:"sud,omitempty"`
}

func loadBillingFixture(root string) (*billingFixture, error) {
	path := filepath.Join(root, "billing-demo.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read billing fixture: %w", err)
	}
	var doc billingFixture
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse billing fixture: %w", err)
	}
	if len(doc.Pages) > 0 {
		for _, p := range doc.Pages {
			doc.Rows = append(doc.Rows, p.Rows...)
		}
	}
	return &doc, nil
}

func normalizeExport(fix *billingFixture, interval domain.BillingInterval) ([]appbilling.AttributionInput, map[string]domaintypes.Money, []domain.CostDiagnostic) {
	var inputs []appbilling.AttributionInput
	sourceTotals := map[string]domaintypes.Money{}
	var diagnostics []domain.CostDiagnostic
	if fix == nil {
		return nil, sourceTotals, diagnostics
	}
	if fix.IncompleteMonth {
		diagnostics = append(diagnostics, domain.CostDiagnostic{
			Code: "incomplete_month", Message: "current invoice month is incomplete in billing export", Severity: "info",
		})
	}
	if fix.BillingLagDays > 0 {
		diagnostics = append(diagnostics, domain.CostDiagnostic{
			Code:     "billing_lag",
			Message:  fmt.Sprintf("billing export lag %d days behind usage", fix.BillingLagDays),
			Severity: "warn",
		})
	}
	for cur, m := range fix.SourceTotals {
		currency := m.Currency
		if currency == "" {
			currency = cur
		}
		sourceTotals[currency] = domaintypes.FromMajorUnits(m.AmountMajor, currency, 100)
	}
	for _, row := range fix.Rows {
		in, diag := rowToInput(row)
		if diag != nil {
			diagnostics = append(diagnostics, *diag)
			continue
		}
		inputs = append(inputs, in)
	}
	if len(sourceTotals) == 0 {
		// Derive totals from rows when fixture omits explicit source_totals.
		for _, in := range inputs {
			if in.Amount.Currency == "" {
				continue
			}
			if prev, ok := sourceTotals[in.Amount.Currency]; ok {
				if sum, err := prev.Add(in.Amount); err == nil {
					sourceTotals[in.Amount.Currency] = sum
				}
			} else {
				sourceTotals[in.Amount.Currency] = in.Amount
			}
		}
	}
	_ = interval
	return inputs, sourceTotals, diagnostics
}

func rowToInput(row exportRow) (appbilling.AttributionInput, *domain.CostDiagnostic) {
	start, err1 := parseExportDate(row.PeriodStart)
	end, err2 := parseExportDate(row.PeriodEnd)
	if err1 != nil || err2 != nil {
		return appbilling.AttributionInput{}, &domain.CostDiagnostic{
			Code: "invalid_period", Message: "skipped malformed billing row period", Severity: "warn",
		}
	}
	cur := row.Currency
	if cur == "" {
		cur = "USD"
	}
	amount := domaintypes.FromMajorUnits(row.CostMajor, cur, 100)
	basis := domain.CostBasisNetUnblended
	switch strings.ToLower(row.Basis) {
	case "list":
		basis = domain.CostBasisUnblended
	case "credit", "adjustment":
		basis = domain.CostBasisNetUnblended
	case "effective", "":
		basis = domain.CostBasisNetUnblended
	}
	kind := domain.ChargeUsage
	switch strings.ToLower(row.CostType) {
	case "tax":
		kind = domain.ChargeTax
	case "adjustment":
		kind = domain.ChargeSupport
	case "credit", "refund":
		kind = domain.ChargeCredit
	default:
		if amount.AmountMinor < 0 {
			kind = domain.ChargeCredit
		}
	}
	if row.Commitment {
		kind = domain.ChargeUsage
	}
	in := appbilling.AttributionInput{
		ProviderResourceID: strings.TrimSpace(row.ResourceName),
		Service:            row.Service,
		Region:             row.Region,
		Amount:             amount,
		Basis:              basis,
		ChargeKind:         kind,
		Granularity:        domain.CostMonthly,
		PeriodStart:        domaintypes.NewTimestamp(start),
		PeriodEnd:          domaintypes.NewTimestamp(end),
		SharedPool:         row.SharedPool,
		TagOwner:           row.LabelOwner,
	}
	if row.SUD && in.ChargeKind == domain.ChargeUsage {
		in.Basis = domain.CostBasisNetUnblended
	}
	if row.Commitment {
		in.Service = strings.TrimSpace(in.Service + " Commitment")
	}
	return in, nil
}

func parseExportDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}

// NewFixtureBillingSource uses recorded BigQuery export JSON fixtures.
func NewFixtureBillingSource(root string) ports.BillingSource {
	c := NewCollector(nil)
	c.FixtureLoader = func(string) (*billingFixture, error) {
		return loadBillingFixture(root)
	}
	return c
}

// NewLiveBillingSource builds a collector with ADC (optional impersonation via client options).
func NewLiveBillingSource(ctx context.Context, opts ports.CostCollectOptions) (ports.BillingSource, error) {
	_ = ctx
	_ = opts.ImpersonateServiceAccount
	bq, err := newLiveBQRunner(ctx)
	if err != nil {
		return nil, err
	}
	return NewCollector(bq), nil
}

type liveBQRunner struct {
	project string
}

func newLiveBQRunner(ctx context.Context) (*liveBQRunner, error) {
	proj := ""
	if metadata.OnGCE() {
		proj, _ = metadata.ProjectIDWithContext(ctx)
	}
	return &liveBQRunner{project: proj}, nil
}

func (l *liveBQRunner) CallerProject(context.Context) (string, error) {
	if l.project == "" {
		return "", fmt.Errorf("unable to determine GCP project from ADC")
	}
	return l.project, nil
}

func (l *liveBQRunner) QueryExport(ctx context.Context, project, dataset, table string, start, end time.Time, pageToken string) ([]exportRow, string, error) {
	_ = ctx
	_ = project
	_ = dataset
	_ = table
	_ = start
	_ = end
	_ = pageToken
	return nil, "", fmt.Errorf("live BigQuery billing export query not implemented in this build; use --offline fixtures")
}
