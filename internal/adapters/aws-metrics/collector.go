package awsmetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	appmetrics "github.com/kernul-io/cloudopt/internal/application/metrics"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/domain"
	domaintypes "github.com/kernul-io/cloudopt/internal/domain/types"
)

const collectorSource = "aws-metrics/cloudwatch"

// CWAPI is the CloudWatch surface used by the collector (mockable in tests).
type CWAPI interface {
	GetMetricData(ctx context.Context, params *cloudwatch.GetMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error)
}

type STSAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// Collector collects AWS utilization metrics via CloudWatch.
type Collector struct {
	STS            STSAPI
	CW             CWAPI
	CapabilitiesFn func() (ports.CapabilityManifest, error)
	FixtureLoader  func(root string) ([]fixtureSeries, error)
	Cache          map[string][]domain.MetricPoint
	cacheMu        sync.RWMutex
}

func NewCollector(sts STSAPI, cw CWAPI) *Collector {
	return &Collector{
		STS: sts,
		CW:  cw,
		CapabilitiesFn: func() (ports.CapabilityManifest, error) {
			return LoadCapabilities()
		},
		FixtureLoader: loadFixtureSeries,
		Cache:         map[string][]domain.MetricPoint{},
	}
}

func (c *Collector) Capabilities() ports.CapabilityManifest {
	m, err := c.CapabilitiesFn()
	if err != nil {
		return ports.CapabilityManifest{Provider: domaintypes.ProviderAWS}
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
	pf.MissingActions = c.probeCW(ctx)
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

	if !opts.Offline && len(pf.MissingActions) > 0 {
		return nil, ports.ErrMissingPermissions(pf.MissingActions)
	}

	plans := buildPlans(inventory)
	var series []domain.MetricSeries
	var diags []domain.MetricDiagnostic
	partial := false

	if opts.Offline {
		root := opts.FixtureRoot
		if root == "" {
			root = "testdata/aws-metrics"
		}
		fixtures, err := c.FixtureLoader(root)
		if err != nil {
			return nil, err
		}
		series, diags = c.collectFromFixtures(fixtures, plans, window, obs)
	} else {
		var err error
		series, diags, partial, err = c.collectLive(ctx, opts, plans, window, obs)
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
		Service: "cloudwatch",
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

func buildPlans(inventory *domain.CollectionSnapshot) []queryPlan {
	if inventory == nil {
		return nil
	}
	var plans []queryPlan
	for _, res := range inventory.Resources {
		for _, spec := range specsForResource(res) {
			dim := spec.DimValueFn(res)
			if dim == "" {
				continue
			}
			key := fmt.Sprintf("%s|%s|%s|%s", spec.Namespace, spec.Name, spec.Dimension, dim)
			plans = append(plans, queryPlan{Resource: res, Spec: spec, Key: key})
		}
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].Key < plans[j].Key })
	return plans
}

func (c *Collector) collectFromFixtures(fixtures []fixtureSeries, plans []queryPlan, window domain.MetricObservationWindow, obs domaintypes.Timestamp) ([]domain.MetricSeries, []domain.MetricDiagnostic) {
	index := map[string]fixtureSeries{}
	for _, f := range fixtures {
		key := fixtureKey(f.Namespace, f.MetricName, f.DimensionName, f.DimensionValue)
		index[key] = f
	}
	var series []domain.MetricSeries
	var diags []domain.MetricDiagnostic
	for _, plan := range plans {
		dimVal := plan.Spec.DimValueFn(plan.Resource)
		key := fixtureKey(plan.Spec.Namespace, plan.Spec.Name, plan.Spec.Dimension, dimVal)
		f, ok := index[key]
		if !ok || f.Missing {
			diags = append(diags, domain.MetricDiagnostic{
				Code:       "metric_missing",
				ResourceID: plan.Resource.ID,
				MetricName: plan.Spec.Name,
				Message:    "no fixture series for planned CloudWatch query",
				Severity:   "warning",
			})
			continue
		}
		pts := mapFixturePoints(f, window)
		series = append(series, domain.MetricSeries{
			ResourceID: plan.Resource.ID,
			Name:       plan.Spec.Name,
			Statistic:  plan.Spec.Statistic,
			Points:     pts,
			Provenance: domain.Provenance{
				Quality:    domain.QualityObserved,
				Source:     collectorSource + "|" + plan.Spec.Namespace + "|" + f.Scenario,
				ObservedAt: obs,
			},
		})
	}
	return series, diags
}

func (c *Collector) collectLive(ctx context.Context, opts ports.MetricsCollectOptions, plans []queryPlan, window domain.MetricObservationWindow, obs domaintypes.Timestamp) ([]domain.MetricSeries, []domain.MetricDiagnostic, bool, error) {
	maxReq := opts.MaxAPIRequests
	if maxReq <= 0 {
		maxReq = 100
	}
	maxConc := opts.MaxConcurrent
	if maxConc <= 0 {
		maxConc = 5
	}
	partial := false
	var series []domain.MetricSeries
	var diags []domain.MetricDiagnostic

	batches := batchPlans(plans, 100)
	reqCount := 0
	for _, batch := range batches {
		if err := ctx.Err(); err != nil {
			return series, diags, partial, err
		}
		if reqCount >= maxReq {
			partial = true
			diags = append(diags, domain.MetricDiagnostic{
				Code:     "api_limit",
				Message:  "CloudWatch GetMetricData request limit reached",
				Severity: "warning",
			})
			break
		}
		out, err := c.fetchBatch(ctx, batch, window)
		reqCount++
		if err != nil {
			if isAccessDenied(err) {
				return nil, diags, true, nil
			}
			return nil, diags, partial, err
		}
		for _, plan := range batch {
			pts := out[plan.Key]
			if len(pts) == 0 {
				diags = append(diags, domain.MetricDiagnostic{
					Code:       "metric_empty",
					ResourceID: plan.Resource.ID,
					MetricName: plan.Spec.Name,
					Message:    "CloudWatch returned no datapoints",
					Severity:   "info",
				})
				continue
			}
			series = append(series, domain.MetricSeries{
				ResourceID: plan.Resource.ID,
				Name:       plan.Spec.Name,
				Statistic:  plan.Spec.Statistic,
				Points:     pts,
				Provenance: domain.Provenance{
					Quality:    domain.QualityObserved,
					Source:     collectorSource + "|" + plan.Spec.Namespace,
					ObservedAt: obs,
				},
			})
		}
	}
	_ = maxConc // reserved for future worker pool; batches are sequential for determinism offline parity
	return series, diags, partial, nil
}

func (c *Collector) fetchBatch(ctx context.Context, batch []queryPlan, window domain.MetricObservationWindow) (map[string][]domain.MetricPoint, error) {
	cacheKey := batchCacheKey(batch, window)
	c.cacheMu.RLock()
	if pts, ok := c.Cache[cacheKey]; ok {
		c.cacheMu.RUnlock()
		return map[string][]domain.MetricPoint{batch[0].Key: pts}, nil
	}
	c.cacheMu.RUnlock()

	var queries []cwtypes.MetricDataQuery
	for i, plan := range batch {
		id := fmt.Sprintf("m%d", i)
		queries = append(queries, cwtypes.MetricDataQuery{
			Id: aws.String(id),
			MetricStat: &cwtypes.MetricStat{
				Metric: &cwtypes.Metric{
					Namespace:  aws.String(plan.Spec.Namespace),
					MetricName: aws.String(plan.Spec.Name),
					Dimensions: []cwtypes.Dimension{{
						Name:  aws.String(plan.Spec.Dimension),
						Value: aws.String(plan.Spec.DimValueFn(plan.Resource)),
					}},
				},
				Period: aws.Int32(int32(window.PeriodSeconds)),
				Stat:   aws.String(plan.Spec.Statistic),
			},
		})
	}
	resp, err := c.CW.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
		StartTime:         aws.Time(window.Start.Time),
		EndTime:           aws.Time(window.End.Time),
		MetricDataQueries: queries,
	})
	if err != nil {
		return nil, err
	}
	out := map[string][]domain.MetricPoint{}
	for i, plan := range batch {
		id := fmt.Sprintf("m%d", i)
		for _, res := range resp.MetricDataResults {
			if aws.ToString(res.Id) != id {
				continue
			}
			var pts []domain.MetricPoint
			for j, ts := range res.Timestamps {
				if j >= len(res.Values) {
					break
				}
				pts = append(pts, domain.MetricPoint{
					Timestamp: domaintypes.NewTimestamp(ts),
					Value:     res.Values[j],
					Unit:      plan.Spec.Unit,
					Quality:   domain.QualityObserved,
				})
			}
			sort.Slice(pts, func(a, b int) bool {
				return pts[a].Timestamp.Before(pts[b].Timestamp.Time)
			})
			out[plan.Key] = pts
		}
	}
	c.cacheMu.Lock()
	if len(batch) == 1 {
		c.Cache[cacheKey] = out[batch[0].Key]
	}
	c.cacheMu.Unlock()
	return out, nil
}

func batchPlans(plans []queryPlan, size int) [][]queryPlan {
	if size <= 0 {
		size = 100
	}
	var batches [][]queryPlan
	for i := 0; i < len(plans); i += size {
		end := i + size
		if end > len(plans) {
			end = len(plans)
		}
		batches = append(batches, plans[i:end])
	}
	return batches
}

func batchCacheKey(batch []queryPlan, window domain.MetricObservationWindow) string {
	var b strings.Builder
	b.WriteString(window.Start.Canonical())
	b.WriteByte('|')
	b.WriteString(window.End.Canonical())
	for _, p := range batch {
		b.WriteByte('|')
		b.WriteString(p.Key)
	}
	return b.String()
}

func (c *Collector) probeCW(ctx context.Context) []string {
	end := time.Now().UTC()
	start := end.Add(-time.Hour)
	_, err := c.CW.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
		StartTime: aws.Time(start),
		EndTime:   aws.Time(end),
		MetricDataQueries: []cwtypes.MetricDataQuery{{
			Id: aws.String("probe"),
			MetricStat: &cwtypes.MetricStat{
				Metric: &cwtypes.Metric{
					Namespace:  aws.String("AWS/EC2"),
					MetricName: aws.String("CPUUtilization"),
					Dimensions: []cwtypes.Dimension{{Name: aws.String("InstanceId"), Value: aws.String("i-probe")}},
				},
				Period: aws.Int32(3600),
				Stat:   aws.String("Average"),
			},
		}},
	})
	if err != nil && isAccessDenied(err) {
		return []string{"cloudwatch:GetMetricData", "cloudwatch:ListMetrics"}
	}
	return nil
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

// NewLiveMetricsSource builds a CloudWatch metrics source from the default credential chain.
func NewLiveMetricsSource(ctx context.Context, roleARN, externalID string) (ports.MetricsSource, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	stsClient := sts.NewFromConfig(cfg)
	if roleARN != "" {
		creds := stscreds.NewAssumeRoleProvider(stsClient, roleARN, func(o *stscreds.AssumeRoleOptions) {
			o.ExternalID = aws.String(externalID)
		})
		cfg.Credentials = aws.NewCredentialsCache(creds)
	}
	return NewCollector(stsClient, cloudwatch.NewFromConfig(cfg)), nil
}

// NewFixtureMetricsSource uses recorded CloudWatch JSON fixtures.
func NewFixtureMetricsSource(root string) ports.MetricsSource {
	c := NewCollector(nil, nil)
	c.FixtureLoader = func(r string) ([]fixtureSeries, error) {
		return loadFixtureSeries(r)
	}
	_ = root
	return &fixtureSource{root: root, inner: c}
}

type fixtureSource struct {
	root  string
	inner *Collector
}

func (f *fixtureSource) Capabilities() ports.CapabilityManifest { return f.inner.Capabilities() }

func (f *fixtureSource) Preflight(ctx context.Context, opts ports.MetricsCollectOptions) (*ports.MetricsPreflight, error) {
	opts.Offline = true
	return f.inner.Preflight(ctx, opts)
}

func (f *fixtureSource) Collect(ctx context.Context, opts ports.MetricsCollectOptions, inv *domain.CollectionSnapshot) (*ports.MetricsCollectOutput, error) {
	opts.Offline = true
	if opts.FixtureRoot == "" {
		opts.FixtureRoot = f.root
	}
	return f.inner.Collect(ctx, opts, inv)
}

type fixtureFile struct {
	Series []fixtureSeries `json:"series"`
}

type fixtureSeries struct {
	ProviderResourceID string         `json:"provider_resource_id"`
	Namespace          string         `json:"namespace"`
	MetricName         string         `json:"metric_name"`
	DimensionName      string         `json:"dimension_name"`
	DimensionValue     string         `json:"dimension_value"`
	Statistic          string         `json:"statistic"`
	PeriodSeconds      int            `json:"period_seconds"`
	Unit               string         `json:"unit"`
	Scenario           string         `json:"scenario"`
	Missing            bool           `json:"missing,omitempty"`
	Datapoints         []fixturePoint `json:"datapoints"`
}

type fixturePoint struct {
	T           string  `json:"t,omitempty"`
	OffsetHours int     `json:"offset_hours,omitempty"`
	V           float64 `json:"v"`
}

func loadFixtureSeries(root string) ([]fixtureSeries, error) {
	path := filepath.Join(root, "metrics-demo.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read metrics fixture: %w", err)
	}
	var file fixtureFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse metrics fixture: %w", err)
	}
	for i := range file.Series {
		s := &file.Series[i]
		if s.DimensionValue == "" {
			s.DimensionValue = s.ProviderResourceID
		}
		if s.DimensionName == "" {
			s.DimensionName = dimensionForNamespace(s.Namespace)
		}
	}
	return file.Series, nil
}

func dimensionForNamespace(ns string) string {
	switch ns {
	case "AWS/EC2":
		return "InstanceId"
	case "AWS/EBS":
		return "VolumeId"
	case "AWS/RDS":
		return "DBInstanceIdentifier"
	case "AWS/NATGateway":
		return "NatGatewayId"
	default:
		return "ResourceId"
	}
}

func fixtureKey(ns, metric, dimName, dimVal string) string {
	return ns + "|" + metric + "|" + dimName + "|" + dimVal
}

func mapFixturePoints(f fixtureSeries, window domain.MetricObservationWindow) []domain.MetricPoint {
	var pts []domain.MetricPoint
	for _, dp := range f.Datapoints {
		var ts time.Time
		switch {
		case dp.T != "":
			parsed, err := time.Parse(time.RFC3339, dp.T)
			if err != nil {
				continue
			}
			ts = parsed
		default:
			ts = window.End.Add(time.Duration(dp.OffsetHours) * time.Hour)
		}
		if ts.Before(window.Start.Time) || !ts.Before(window.End.Time) {
			continue
		}
		pts = append(pts, domain.MetricPoint{
			Timestamp: domaintypes.NewTimestamp(ts),
			Value:     dp.V,
			Unit:      f.Unit,
			Quality:   domain.QualityObserved,
		})
	}
	sort.Slice(pts, func(i, j int) bool {
		return pts[i].Timestamp.Before(pts[j].Timestamp.Time)
	})
	return pts
}
