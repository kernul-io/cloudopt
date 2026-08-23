package gcpmetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	appmetrics "github.com/kernul-io/cloudopt/internal/application/metrics"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain"
	domaintypes "github.com/kernul-io/cloudopt/internal/domain/types"
)

// MonitoringRunner lists time series (mockable; live adapter deferred).
type MonitoringRunner interface {
	ListTimeSeries(ctx context.Context, projectID string, filter string, start, end time.Time) ([]monitorPoint, error)
	CallerEmail(ctx context.Context) (string, error)
}

type monitorPoint struct {
	Timestamp time.Time
	Value     float64
}

// Collector collects GCP utilization metrics via Cloud Monitoring.
type Collector struct {
	Mon            MonitoringRunner
	CapabilitiesFn func() (ports.CapabilityManifest, error)
	FixtureLoader  func(root string) ([]fixtureSeries, error)
}

func NewCollector(mon MonitoringRunner) *Collector {
	return &Collector{
		Mon: mon,
		CapabilitiesFn: func() (ports.CapabilityManifest, error) {
			return LoadCapabilities()
		},
		FixtureLoader: loadFixtureSeries,
	}
}

func (c *Collector) Capabilities() ports.CapabilityManifest {
	m, err := c.CapabilitiesFn()
	if err != nil {
		return ports.CapabilityManifest{Provider: domaintypes.ProviderGCP}
	}
	return m
}

func (c *Collector) Preflight(ctx context.Context, opts ports.MetricsCollectOptions) (*ports.MetricsPreflight, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	caps, err := c.CapabilitiesFn()
	if err != nil {
		return nil, err
	}
	lookback, period := normalizeOptions(opts)
	pf := &ports.MetricsPreflight{
		LookbackDays:  lookback,
		PeriodSeconds: period,
		Capabilities:  caps,
	}
	if opts.Offline {
		pf.ProviderAccountID = "gcp-metrics-fixture"
		pf.CallerARN = "user:offline-fixture@gcp"
		return pf, nil
	}
	if c.Mon == nil {
		pf.MissingActions = []string{"monitoring.timeSeries.list"}
		return pf, nil
	}
	email, err := c.Mon.CallerEmail(ctx)
	if err != nil {
		return nil, errWrap("preflight", err)
	}
	pf.CallerARN = email
	return pf, nil
}

func (c *Collector) Collect(ctx context.Context, opts ports.MetricsCollectOptions, inventory *domain.CollectionSnapshot) (*ports.MetricsCollectOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pf, err := c.Preflight(ctx, opts)
	if err != nil {
		return nil, err
	}
	if opts.DryRun {
		return &ports.MetricsCollectOutput{}, nil
	}
	if !opts.Offline && len(pf.MissingActions) > 0 {
		return nil, ports.ErrMissingPermissions(pf.MissingActions)
	}

	lookback, period := pf.LookbackDays, pf.PeriodSeconds
	endAnchor := time.Now().UTC()
	if opts.Offline && inventory != nil && inventory.CompletedAt != nil {
		endAnchor = inventory.CompletedAt.UTC()
	}
	start, end := metricsWindow(endAnchor, lookback)
	window := domain.MetricObservationWindow{
		Start:             domaintypes.NewTimestamp(start),
		End:               domaintypes.NewTimestamp(end),
		PeriodSeconds:     period,
		TimeZone:          opts.TimeZone,
		BusinessHourStart: opts.BusinessHourStart,
		BusinessHourEnd:   opts.BusinessHourEnd,
	}
	if window.TimeZone == "" {
		window.TimeZone = "UTC"
	}
	if window.BusinessHourStart == 0 && window.BusinessHourEnd == 0 {
		window.BusinessHourStart = 9
		window.BusinessHourEnd = 17
	}
	obs := domaintypes.NowUTC()

	plans := buildPlans(inventory)
	var series []domain.MetricSeries
	var diags []domain.MetricDiagnostic
	partial := false

	if opts.Offline {
		root := opts.FixtureRoot
		if root == "" {
			root = "testdata/gcp-metrics"
		}
		fixtures, err := c.FixtureLoader(root)
		if err != nil {
			return nil, err
		}
		series, diags = c.collectFromFixtures(fixtures, plans, window, obs)
	} else {
		series, diags, partial, err = c.collectLive(ctx, inventory, plans, window, obs)
		if err != nil {
			return nil, err
		}
	}

	memoryByResource := map[domaintypes.ResourceID]bool{}
	for _, s := range series {
		if s.Name == "FreeableMemory" {
			memoryByResource[s.ResourceID] = true
		}
	}
	var signals []domain.UtilizationSignal
	for _, s := range series {
		dopts := appmetrics.DefaultDeriveOptions(window, collectorSource, obs)
		if s.Name == "CPUUtilization" {
			if r := resourceKind(inventory, s.ResourceID); r == domain.KindComputeInstance || r == domain.KindDatabase {
				dopts.MemoryMetricMissing = !memoryByResource[s.ResourceID]
			}
		}
		sig, d := appmetrics.DeriveSignals(s, dopts)
		signals = append(signals, sig...)
		diags = append(diags, d...)
	}

	coverage := []domain.ServiceCollectionStatus{{
		Service: "cloud_monitoring",
		Status:  domain.ServiceCollectionOK,
	}}
	if partial || len(diags) > 0 {
		coverage[0].Status = domain.ServiceCollectionPartial
		coverage[0].Message = "see metric diagnostics"
		partial = true
	}

	return &ports.MetricsCollectOutput{
		Series:      series,
		Signals:     signals,
		Window:      window,
		Coverage:    coverage,
		Diagnostics: diags,
		Partial:     partial,
	}, nil
}

type queryPlan struct {
	Resource domain.Resource
	Spec     metricSpec
	Key      string
}

type metricSpec struct {
	MonitoredResource string
	MetricType        string
	CanonicalName     string
	Statistic         string
	Unit              string
}

func buildPlans(inventory *domain.CollectionSnapshot) []queryPlan {
	if inventory == nil {
		return nil
	}
	var plans []queryPlan
	for _, res := range inventory.Resources {
		for _, spec := range specsForResource(res) {
			key := res.ProviderResourceID + "|" + spec.MetricType
			plans = append(plans, queryPlan{Resource: res, Spec: spec, Key: key})
		}
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].Key < plans[j].Key })
	return plans
}

func specsForResource(r domain.Resource) []metricSpec {
	switch r.Kind {
	case domain.KindComputeInstance:
		return []metricSpec{{
			MonitoredResource: "gce_instance",
			MetricType:        "compute.googleapis.com/instance/cpu/utilization",
			CanonicalName:     "CPUUtilization",
			Statistic:         "Average",
			Unit:              "Percent",
		}}
	case domain.KindDatabase:
		return []metricSpec{{
			MonitoredResource: "cloudsql_database",
			MetricType:        "cloudsql.googleapis.com/database/cpu/utilization",
			CanonicalName:     "CPUUtilization",
			Statistic:         "Average",
			Unit:              "Percent",
		}}
	case domain.KindNATGateway:
		return []metricSpec{{
			MonitoredResource: "nat_gateway",
			MetricType:        "router.googleapis.com/nat/sent_bytes_count",
			CanonicalName:     "BytesOutToDestination",
			Statistic:         "Sum",
			Unit:              "Bytes",
		}}
	case domain.KindKubernetesNodePool:
		return []metricSpec{{
			MonitoredResource: "k8s_node",
			MetricType:        "kubernetes.io/node/cpu/allocatable_utilization",
			CanonicalName:     "CPUUtilization",
			Statistic:         "Average",
			Unit:              "Percent",
		}}
	default:
		return nil
	}
}

func (c *Collector) collectFromFixtures(fixtures []fixtureSeries, plans []queryPlan, window domain.MetricObservationWindow, obs domaintypes.Timestamp) ([]domain.MetricSeries, []domain.MetricDiagnostic) {
	index := map[string]fixtureSeries{}
	for _, f := range fixtures {
		index[f.ProviderResourceID+"|"+f.MetricType] = f
	}
	var series []domain.MetricSeries
	var diags []domain.MetricDiagnostic
	for _, plan := range plans {
		f, ok := index[plan.Resource.ProviderResourceID+"|"+plan.Spec.MetricType]
		if !ok || f.Missing {
			diags = append(diags, domain.MetricDiagnostic{
				Code:       "metric_missing",
				ResourceID: plan.Resource.ID,
				MetricName: plan.Spec.CanonicalName,
				Message:    "no fixture series for planned Monitoring query",
				Severity:   "warning",
			})
			continue
		}
		pts := mapFixturePoints(f, window)
		series = append(series, domain.MetricSeries{
			ResourceID: plan.Resource.ID,
			Name:       plan.Spec.CanonicalName,
			Statistic:  plan.Spec.Statistic,
			Points:     pts,
			Provenance: domain.Provenance{
				Quality:    domain.QualityObserved,
				Source:     collectorSource + "|" + plan.Spec.MetricType + "|" + f.Scenario,
				ObservedAt: obs,
			},
		})
	}
	return series, diags
}

func (c *Collector) collectLive(ctx context.Context, inv *domain.CollectionSnapshot, plans []queryPlan, window domain.MetricObservationWindow, obs domaintypes.Timestamp) ([]domain.MetricSeries, []domain.MetricDiagnostic, bool, error) {
	_ = ctx
	_ = inv
	_ = plans
	_ = window
	_ = obs
	return nil, []domain.MetricDiagnostic{{
		Code:     "live_deferred",
		Message:  "live Cloud Monitoring collection requires project-scoped credentials; use --offline fixtures",
		Severity: "warning",
	}}, true, nil
}

func metricsWindow(now time.Time, lookbackDays int) (time.Time, time.Time) {
	end := now.Truncate(time.Hour)
	start := end.Add(-time.Duration(lookbackDays) * 24 * time.Hour)
	return start, end
}

func normalizeOptions(opts ports.MetricsCollectOptions) (lookback, period int) {
	lookback = opts.LookbackDays
	if lookback <= 0 {
		lookback = 14
	}
	period = opts.PeriodSeconds
	if period <= 0 {
		period = 3600
	}
	return lookback, period
}

func resourceKind(inv *domain.CollectionSnapshot, id domaintypes.ResourceID) domain.ResourceKind {
	if inv == nil {
		return ""
	}
	for _, r := range inv.Resources {
		if r.ID == id {
			return r.Kind
		}
	}
	return ""
}

type fixtureFile struct {
	Series []fixtureSeries `json:"series"`
}

type fixtureSeries struct {
	ProviderResourceID string         `json:"provider_resource_id"`
	MetricType         string         `json:"metric_type"`
	Scenario           string         `json:"scenario"`
	Missing            bool           `json:"missing,omitempty"`
	Datapoints         []fixturePoint `json:"datapoints"`
}

type fixturePoint struct {
	OffsetHours int     `json:"offset_hours,omitempty"`
	V           float64 `json:"v"`
}

func loadFixtureSeries(root string) ([]fixtureSeries, error) {
	path := filepath.Join(root, "metrics-demo.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read gcp metrics fixture: %w", err)
	}
	var file fixtureFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse gcp metrics fixture: %w", err)
	}
	return file.Series, nil
}

func mapFixturePoints(f fixtureSeries, window domain.MetricObservationWindow) []domain.MetricPoint {
	var pts []domain.MetricPoint
	for _, dp := range f.Datapoints {
		ts := window.End.Add(time.Duration(dp.OffsetHours) * time.Hour)
		if ts.Before(window.Start.Time) || !ts.Before(window.End.Time) {
			continue
		}
		unit := "Percent"
		if strings.Contains(f.MetricType, "bytes") {
			unit = "Bytes"
		}
		val := dp.V
		if strings.Contains(f.MetricType, "cpu/utilization") && val <= 1.0 {
			val *= 100
		}
		pts = append(pts, domain.MetricPoint{
			Timestamp: domaintypes.NewTimestamp(ts),
			Value:     val,
			Unit:      unit,
			Quality:   domain.QualityObserved,
		})
	}
	return pts
}

// NewFixtureMetricsSource uses recorded Monitoring JSON fixtures.
func NewFixtureMetricsSource(root string) ports.MetricsSource {
	c := NewCollector(nil)
	c.FixtureLoader = func(string) ([]fixtureSeries, error) {
		return loadFixtureSeries(root)
	}
	return c
}

// NewLiveMetricsSource builds a Monitoring collector (live queries deferred).
func NewLiveMetricsSource(_ context.Context, _ string) (ports.MetricsSource, error) {
	return NewCollector(&stubMonitoring{}), nil
}

type stubMonitoring struct{}

func (stubMonitoring) CallerEmail(context.Context) (string, error) {
	return "user:adc@gcp", nil
}

func (stubMonitoring) ListTimeSeries(context.Context, string, string, time.Time, time.Time) ([]monitorPoint, error) {
	return nil, fmt.Errorf("live monitoring not implemented; use --offline")
}
