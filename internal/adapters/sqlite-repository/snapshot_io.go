package sqliterepository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func upsertAccount(ctx context.Context, tx *sql.Tx, acc domain.Account) error {
	q, s, o := provColumns(acc.Provenance)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO accounts (id, provider, provider_account_id, display_name, default_currency, quality, source, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			display_name = excluded.display_name,
			default_currency = excluded.default_currency,
			quality = excluded.quality,
			source = excluded.source,
			observed_at = excluded.observed_at`,
		string(acc.ID), string(acc.Provider), acc.ProviderAccountID, acc.DisplayName, acc.DefaultCurrency,
		q, s, o,
	)
	return err
}

func insertSnapshot(ctx context.Context, tx *sql.Tx, snap *domain.CollectionSnapshot) error {
	var completed *string
	if snap.CompletedAt != nil {
		c := snap.CompletedAt.Canonical()
		completed = &c
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO snapshots (id, account_id, provider, status, schema_version, external_key, started_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		string(snap.ID), string(snap.AccountID), string(snap.Provider), string(snap.Status),
		snap.SchemaVersion, snap.ExternalKey, snap.StartedAt.Canonical(), completed,
	)
	return err
}

func insertRegions(ctx context.Context, tx *sql.Tx, snap *domain.CollectionSnapshot) error {
	for _, reg := range snap.Regions {
		q, s, o := provColumns(reg.Provenance)
		_, err := tx.ExecContext(ctx, `
			INSERT INTO snapshot_regions (snapshot_id, id, provider_region_id, display_name, quality, source, observed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			string(snap.ID), string(reg.ID), reg.ProviderRegionID, reg.DisplayName, q, s, o,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func insertResources(ctx context.Context, tx *sql.Tx, snap *domain.CollectionSnapshot) error {
	for _, res := range snap.Resources {
		attrs, err := encodeJSON(res.Attributes)
		if err != nil {
			return err
		}
		q, s, o := provColumns(res.Provenance)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO resources (snapshot_id, id, kind, provider_resource_id, account_id, region_id, name, state, attributes_json, quality, source, observed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			string(snap.ID), string(res.ID), string(res.Kind), res.ProviderResourceID,
			string(res.AccountID), string(res.RegionID), res.Name, res.State, attrs, q, s, o,
		)
		if err != nil {
			return err
		}
		for _, tag := range res.Tags {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO resource_tags (snapshot_id, resource_id, key, value) VALUES (?, ?, ?, ?)`,
				string(snap.ID), string(res.ID), tag.Key, tag.Value,
			)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func insertRelationships(ctx context.Context, tx *sql.Tx, snap *domain.CollectionSnapshot) error {
	for _, rel := range snap.Relationships {
		q, s, o := provColumns(rel.Provenance)
		targetMissing := 0
		if rel.TargetMissing {
			targetMissing = 1
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO relationships (snapshot_id, kind, from_resource_id, to_resource_id, to_provider_resource_id, target_missing, quality, source, observed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			string(snap.ID), string(rel.Kind), string(rel.FromResourceID), string(rel.ToResourceID),
			rel.ToProviderResourceID, targetMissing, q, s, o,
		)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		rel.ID = id
	}
	return nil
}

func insertCosts(ctx context.Context, tx *sql.Tx, snap *domain.CollectionSnapshot) error {
	for _, c := range snap.Costs {
		if err := insertOneCost(ctx, tx, snap.ID, &c); err != nil {
			return err
		}
	}
	return nil
}

func insertOneCost(ctx context.Context, tx *sql.Tx, snapID types.SnapshotID, c *domain.CostRecord) error {
	q, s, o := provColumns(c.Provenance)
	basis := string(c.Basis)
	if basis == "" {
		basis = string(domain.CostBasisAmortizedNet)
	}
	charge := string(c.ChargeKind)
	if charge == "" {
		charge = string(domain.ChargeUsage)
	}
	attrMethod := string(c.Attribution.Method)
	if attrMethod == "" {
		attrMethod = string(domain.AttributionDirectResourceID)
	}
	srcStart := c.SourceInterval.Start.Canonical()
	srcEnd := c.SourceInterval.End.Canonical()
	srcCollected := c.SourceInterval.Collected.Canonical()
	res, err := tx.ExecContext(ctx, `
			INSERT INTO cost_records (
				snapshot_id, resource_id, service, amount_minor, currency, granularity,
				period_start, period_end, quality, source, observed_at,
				cost_basis, charge_kind, region_id, attribution_method, attribution_heuristic,
				attribution_confidence, source_interval_start, source_interval_end, source_collected_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(snapID), string(c.ResourceID), c.Service, c.Amount.AmountMinor, c.Amount.Currency,
		string(c.Granularity), c.PeriodStart.Canonical(), c.PeriodEnd.Canonical(), q, s, o,
		basis, charge, string(c.RegionID), attrMethod, c.Attribution.HeuristicID, c.Attribution.Confidence,
		srcStart, srcEnd, srcCollected,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	c.ID = id
	return nil
}

func insertMetrics(ctx context.Context, tx *sql.Tx, snap *domain.CollectionSnapshot) error {
	for _, series := range snap.Metrics {
		q, s, o := provColumns(series.Provenance)
		res, err := tx.ExecContext(ctx, `
			INSERT INTO metric_series (snapshot_id, resource_id, name, statistic, quality, source, observed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			string(snap.ID), string(series.ResourceID), series.Name, series.Statistic, q, s, o,
		)
		if err != nil {
			return err
		}
		seriesID, _ := res.LastInsertId()
		series.ID = seriesID
		for _, pt := range series.Points {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO metric_points (series_id, ts, value, unit, quality)
				VALUES (?, ?, ?, ?, ?)`,
				seriesID, pt.Timestamp.Canonical(), pt.Value, pt.Unit, string(pt.Quality),
			)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func insertUtilizationSignal(ctx context.Context, tx *sql.Tx, snapID types.SnapshotID, s *domain.UtilizationSignal) error {
	q, src, o := provColumns(s.Provenance)
	queryJSON, err := json.Marshal(s.Query)
	if err != nil {
		return err
	}
	notesJSON, err := json.Marshal(s.Notes)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO utilization_signals (
			snapshot_id, resource_id, metric_name, kind, value, unit,
			sample_count, expected_samples, coverage_ratio, zero_samples, missing_samples,
			query_json, notes_json, quality, source, observed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(snapID), string(s.ResourceID), s.MetricName, string(s.Kind), s.Value, s.Unit,
		s.SampleCount, s.ExpectedSamples, s.CoverageRatio, s.ZeroSamples, s.MissingSamples,
		string(queryJSON), string(notesJSON), q, src, o,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	s.ID = id
	return nil
}

func insertMetricsMeta(ctx context.Context, tx *sql.Tx, snapID types.SnapshotID, meta *domain.MetricsCollectionMeta) error {
	diagJSON, err := json.Marshal(meta.Diagnostics)
	if err != nil {
		return err
	}
	partial := 0
	if meta.Partial {
		partial = 1
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO snapshot_metrics_meta (
			snapshot_id, window_start, window_end, period_seconds, timezone,
			business_hour_start, business_hour_end, source, partial, diagnostics_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(snapID),
		meta.Window.Start.Canonical(),
		meta.Window.End.Canonical(),
		meta.Window.PeriodSeconds,
		meta.Window.TimeZone,
		meta.Window.BusinessHourStart,
		meta.Window.BusinessHourEnd,
		meta.Source,
		partial,
		string(diagJSON),
	)
	return err
}

func loadMetricsMeta(ctx context.Context, db queryer, snapID types.SnapshotID) (*domain.MetricsCollectionMeta, error) {
	row := db.QueryRowContext(ctx, `
		SELECT window_start, window_end, period_seconds, timezone, business_hour_start, business_hour_end,
			source, partial, diagnostics_json
		FROM snapshot_metrics_meta WHERE snapshot_id = ?`, string(snapID))
	var meta domain.MetricsCollectionMeta
	var partial int
	var diagJSON string
	var wStart, wEnd string
	if err := row.Scan(&wStart, &wEnd, &meta.Window.PeriodSeconds, &meta.Window.TimeZone,
		&meta.Window.BusinessHourStart, &meta.Window.BusinessHourEnd, &meta.Source, &partial, &diagJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	start, err := types.ParseTimestamp(wStart)
	if err != nil {
		return nil, err
	}
	end, err := types.ParseTimestamp(wEnd)
	if err != nil {
		return nil, err
	}
	meta.Window.Start = start
	meta.Window.End = end
	meta.Partial = partial != 0
	if diagJSON != "" {
		_ = json.Unmarshal([]byte(diagJSON), &meta.Diagnostics)
	}
	return &meta, nil
}

func loadUtilizationSignals(ctx context.Context, db queryer, snapID types.SnapshotID) ([]domain.UtilizationSignal, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, resource_id, metric_name, kind, value, unit, sample_count, expected_samples,
			coverage_ratio, zero_samples, missing_samples, query_json, notes_json, quality, source, observed_at
		FROM utilization_signals WHERE snapshot_id = ? ORDER BY id`, string(snapID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.UtilizationSignal
	for rows.Next() {
		var s domain.UtilizationSignal
		var q, src string
		var observed types.Timestamp
		var queryJSON, notesJSON string
		if err := rows.Scan(&s.ID, &s.ResourceID, &s.MetricName, &s.Kind, &s.Value, &s.Unit,
			&s.SampleCount, &s.ExpectedSamples, &s.CoverageRatio, &s.ZeroSamples, &s.MissingSamples,
			&queryJSON, &notesJSON, &q, &src, &tsScan{&observed}); err != nil {
			return nil, err
		}
		s.Provenance = domain.Provenance{Quality: domain.DataQuality(q), Source: src, ObservedAt: observed}
		_ = json.Unmarshal([]byte(queryJSON), &s.Query)
		_ = json.Unmarshal([]byte(notesJSON), &s.Notes)
		out = append(out, s)
	}
	return out, rows.Err()
}

func insertServiceCoverage(ctx context.Context, tx *sql.Tx, snap *domain.CollectionSnapshot) error {
	for _, svc := range snap.Coverage.Services {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO snapshot_service_coverage (snapshot_id, service, region, status, message)
			VALUES (?, ?, ?, ?, ?)`,
			string(snap.ID), svc.Service, svc.Region, string(svc.Status), svc.Message,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func loadServiceCoverage(ctx context.Context, db queryer, snapID types.SnapshotID) (domain.CollectionCoverage, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT service, region, status, message
		FROM snapshot_service_coverage WHERE snapshot_id = ? ORDER BY service, region`, string(snapID))
	if err != nil {
		return domain.CollectionCoverage{}, err
	}
	defer func() { _ = rows.Close() }()
	var out domain.CollectionCoverage
	for rows.Next() {
		var svc domain.ServiceCollectionStatus
		var status string
		if err := rows.Scan(&svc.Service, &svc.Region, &status, &svc.Message); err != nil {
			return domain.CollectionCoverage{}, err
		}
		svc.Status = domain.ServiceCollectionState(status)
		out.Services = append(out.Services, svc)
	}
	return out, rows.Err()
}

func loadAccount(ctx context.Context, db queryer, id types.AccountID) (domain.Account, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, provider, provider_account_id, display_name, default_currency, quality, source, observed_at
		FROM accounts WHERE id = ?`, string(id))
	var acc domain.Account
	var q, s string
	var observed types.Timestamp
	if err := row.Scan(&acc.ID, &acc.Provider, &acc.ProviderAccountID, &acc.DisplayName,
		&acc.DefaultCurrency, &q, &s, &tsScan{&observed}); err != nil {
		return domain.Account{}, err
	}
	acc.Provenance = domain.Provenance{Quality: domain.DataQuality(q), Source: s, ObservedAt: observed}
	return acc, nil
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func loadRegions(ctx context.Context, db queryer, snapID types.SnapshotID) ([]domain.Region, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, provider_region_id, display_name, quality, source, observed_at
		FROM snapshot_regions WHERE snapshot_id = ? ORDER BY id`, string(snapID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Region
	for rows.Next() {
		var reg domain.Region
		var q, s string
		var observed types.Timestamp
		if err := rows.Scan(&reg.ID, &reg.ProviderRegionID, &reg.DisplayName, &q, &s, &tsScan{&observed}); err != nil {
			return nil, err
		}
		reg.Provenance = domain.Provenance{Quality: domain.DataQuality(q), Source: s, ObservedAt: observed}
		out = append(out, reg)
	}
	return out, rows.Err()
}

func loadResources(ctx context.Context, db queryer, snapID types.SnapshotID) ([]domain.Resource, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, kind, provider_resource_id, account_id, region_id, name, state, attributes_json, quality, source, observed_at
		FROM resources WHERE snapshot_id = ? ORDER BY id`, string(snapID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Resource
	for rows.Next() {
		var res domain.Resource
		var q, s string
		var attrsJSON string
		var observed types.Timestamp
		if err := rows.Scan(&res.ID, &res.Kind, &res.ProviderResourceID, &res.AccountID, &res.RegionID,
			&res.Name, &res.State, &attrsJSON, &q, &s, &tsScan{&observed}); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(attrsJSON), &res.Attributes)
		if res.Attributes == nil {
			res.Attributes = map[string]string{}
		}
		res.Provenance = domain.Provenance{Quality: domain.DataQuality(q), Source: s, ObservedAt: observed}
		out = append(out, res)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	tagRows, err := db.QueryContext(ctx, `
		SELECT resource_id, key, value FROM resource_tags WHERE snapshot_id = ? ORDER BY resource_id, key`,
		string(snapID),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tagRows.Close() }()
	tagsByResource := map[types.ResourceID][]domain.Tag{}
	for tagRows.Next() {
		var resourceID types.ResourceID
		var tag domain.Tag
		if err := tagRows.Scan(&resourceID, &tag.Key, &tag.Value); err != nil {
			return nil, err
		}
		tagsByResource[resourceID] = append(tagsByResource[resourceID], tag)
	}
	if err := tagRows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Tags = tagsByResource[out[i].ID]
	}
	return out, nil
}

func loadRelationships(ctx context.Context, db queryer, snapID types.SnapshotID) ([]domain.Relationship, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, kind, from_resource_id, to_resource_id, to_provider_resource_id, target_missing, quality, source, observed_at
		FROM relationships WHERE snapshot_id = ? ORDER BY id`, string(snapID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Relationship
	for rows.Next() {
		var rel domain.Relationship
		var q, s string
		var targetMissing int
		var observed types.Timestamp
		if err := rows.Scan(&rel.ID, &rel.Kind, &rel.FromResourceID, &rel.ToResourceID,
			&rel.ToProviderResourceID, &targetMissing, &q, &s, &tsScan{&observed}); err != nil {
			return nil, err
		}
		rel.TargetMissing = targetMissing == 1
		rel.Provenance = domain.Provenance{Quality: domain.DataQuality(q), Source: s, ObservedAt: observed}
		out = append(out, rel)
	}
	return out, rows.Err()
}

func loadCosts(ctx context.Context, db queryer, snapID types.SnapshotID) ([]domain.CostRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, resource_id, service, amount_minor, currency, granularity, period_start, period_end,
			quality, source, observed_at, cost_basis, charge_kind, region_id, attribution_method,
			attribution_heuristic, attribution_confidence, source_interval_start, source_interval_end, source_collected_at
		FROM cost_records WHERE snapshot_id = ? ORDER BY id`, string(snapID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.CostRecord
	for rows.Next() {
		var c domain.CostRecord
		var q, s string
		var observed types.Timestamp
		var basis, charge, regionID, attrMethod, heuristic, srcStart, srcEnd, srcCollected string
		var confidence float64
		if err := rows.Scan(&c.ID, &c.ResourceID, &c.Service, &c.Amount.AmountMinor, &c.Amount.Currency,
			&c.Granularity, &tsScan{&c.PeriodStart}, &tsScan{&c.PeriodEnd}, &q, &s, &tsScan{&observed},
			&basis, &charge, &regionID, &attrMethod, &heuristic, &confidence, &srcStart, &srcEnd, &srcCollected); err != nil {
			return nil, err
		}
		c.Basis = domain.CostBasis(basis)
		c.ChargeKind = domain.CostChargeKind(charge)
		c.RegionID = types.RegionID(regionID)
		c.Attribution = domain.CostAttribution{
			Method:      domain.AttributionMethod(attrMethod),
			HeuristicID: heuristic,
			Confidence:  confidence,
		}
		if srcStart != "" {
			if t, err := types.ParseTimestamp(srcStart); err == nil {
				c.SourceInterval.Start = t
			}
		}
		if srcEnd != "" {
			if t, err := types.ParseTimestamp(srcEnd); err == nil {
				c.SourceInterval.End = t
			}
		}
		if srcCollected != "" {
			if t, err := types.ParseTimestamp(srcCollected); err == nil {
				c.SourceInterval.Collected = t
			}
		}
		c.Provenance = domain.Provenance{Quality: domain.DataQuality(q), Source: s, ObservedAt: observed}
		out = append(out, c)
	}
	return out, rows.Err()
}

func loadMetrics(ctx context.Context, db queryer, snapID types.SnapshotID) ([]domain.MetricSeries, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, resource_id, name, statistic, quality, source, observed_at
		FROM metric_series WHERE snapshot_id = ? ORDER BY id`, string(snapID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.MetricSeries
	seriesIDs := []int64{}
	for rows.Next() {
		var series domain.MetricSeries
		var q, s string
		var observed types.Timestamp
		if err := rows.Scan(&series.ID, &series.ResourceID, &series.Name, &series.Statistic, &q, &s, &tsScan{&observed}); err != nil {
			return nil, err
		}
		series.Provenance = domain.Provenance{Quality: domain.DataQuality(q), Source: s, ObservedAt: observed}
		out = append(out, series)
		seriesIDs = append(seriesIDs, series.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(seriesIDs) == 0 {
		return out, nil
	}

	ptRows, err := db.QueryContext(ctx, `
		SELECT series_id, ts, value, unit, quality FROM metric_points
		WHERE series_id IN (`+sqlPlaceholders(len(seriesIDs))+`) ORDER BY series_id, ts`,
		int64SliceToAny(seriesIDs)...,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = ptRows.Close() }()
	pointsBySeries := map[int64][]domain.MetricPoint{}
	for ptRows.Next() {
		var seriesID int64
		var pt domain.MetricPoint
		var qPt string
		if err := ptRows.Scan(&seriesID, &tsScan{&pt.Timestamp}, &pt.Value, &pt.Unit, &qPt); err != nil {
			return nil, err
		}
		pt.Quality = domain.DataQuality(qPt)
		pointsBySeries[seriesID] = append(pointsBySeries[seriesID], pt)
	}
	if err := ptRows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Points = pointsBySeries[out[i].ID]
	}
	return out, nil
}

func sqlPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	s := "?"
	for i := 1; i < n; i++ {
		s += ",?"
	}
	return s
}

func int64SliceToAny(v []int64) []any {
	out := make([]any, len(v))
	for i := range v {
		out[i] = v[i]
	}
	return out
}

func insertEvidence(ctx context.Context, tx *sql.Tx, run *domain.AnalysisRun) (map[int64]int64, error) {
	idMap := make(map[int64]int64)
	for i, ev := range run.Evidence {
		detail, err := encodeJSON(ev.Detail)
		if err != nil {
			return nil, err
		}
		q, s, o := provColumns(ev.Provenance)
		res, err := tx.ExecContext(ctx, `
			INSERT INTO evidence (analysis_run_id, kind, resource_id, summary, detail_json, quality, source, observed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			string(run.ID), string(ev.Kind), string(ev.ResourceID), ev.Summary, detail, q, s, o,
		)
		if err != nil {
			return nil, err
		}
		newID, _ := res.LastInsertId()
		if ev.ID != 0 {
			idMap[ev.ID] = newID
		}
		run.Evidence[i].ID = newID
	}
	return idMap, nil
}

func insertFindings(ctx context.Context, tx *sql.Tx, run *domain.AnalysisRun, _ map[int64]int64) error {
	for _, f := range run.Findings {
		resIDs, err := encodeJSON(f.ResourceIDs)
		if err != nil {
			return err
		}
		evIDs, err := encodeJSON(f.EvidenceIDs)
		if err != nil {
			return err
		}
		asmp, err := encodeJSON(f.Assumptions)
		if err != nil {
			return err
		}
		q, s, o := provColumns(f.Provenance)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO findings (analysis_run_id, id, rule_id, fingerprint, severity, category, title, description,
				resource_ids_json, evidence_ids_json, assumptions_json, confidence_bps, quality, source, observed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			string(run.ID), string(f.ID), f.RuleID, f.Fingerprint, string(f.Severity), f.Category,
			f.Title, f.Description, resIDs, evIDs, asmp, f.Confidence.BasisPoints, q, s, o,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func insertRecommendations(ctx context.Context, tx *sql.Tx, run *domain.AnalysisRun) error {
	for _, rec := range run.Recommendations {
		steps, err := encodeJSON(rec.Steps)
		if err != nil {
			return err
		}
		q, s, o := provColumns(rec.Provenance)
		var savingsMinor sql.NullInt64
		var savingsCurrency sql.NullString
		var savingsLow sql.NullInt64
		var savingsHigh sql.NullInt64
		var savingsClass sql.NullString
		var overlapKey sql.NullString
		var inputsJSON sql.NullString
		if rec.EstSavings != nil {
			savingsMinor = sql.NullInt64{Int64: rec.EstSavings.AmountMinor, Valid: true}
			savingsCurrency = sql.NullString{String: rec.EstSavings.Currency, Valid: true}
		}
		if rec.EstSavingsLow != nil {
			savingsLow = sql.NullInt64{Int64: rec.EstSavingsLow.AmountMinor, Valid: true}
		}
		if rec.EstSavingsHigh != nil {
			savingsHigh = sql.NullInt64{Int64: rec.EstSavingsHigh.AmountMinor, Valid: true}
		}
		if rec.SavingsClass != "" {
			savingsClass = sql.NullString{String: string(rec.SavingsClass), Valid: true}
		}
		if rec.OverlapKey != "" {
			overlapKey = sql.NullString{String: rec.OverlapKey, Valid: true}
		}
		if len(rec.SavingsInputs) > 0 {
			raw, err := encodeJSON(rec.SavingsInputs)
			if err != nil {
				return err
			}
			inputsJSON = sql.NullString{String: raw, Valid: true}
		}
		inv := 0
		if rec.InvestigationOnly {
			inv = 1
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO recommendations (
				analysis_run_id, finding_id, summary, steps_json, risk_level,
				savings_minor, savings_currency, savings_low_minor, savings_high_minor,
				savings_class, investigation_only, overlap_key, savings_inputs_json,
				quality, source, observed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			string(run.ID), string(rec.FindingID), rec.Summary, steps, rec.RiskLevel,
			savingsMinor, savingsCurrency, savingsLow, savingsHigh, savingsClass, inv, overlapKey, inputsJSON,
			q, s, o,
		)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		rec.ID = id
	}
	return nil
}

func loadEvidence(ctx context.Context, db queryer, runID types.AnalysisRunID) ([]domain.Evidence, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, kind, resource_id, summary, detail_json, quality, source, observed_at
		FROM evidence WHERE analysis_run_id = ? ORDER BY id`, string(runID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Evidence
	for rows.Next() {
		var ev domain.Evidence
		var q, s string
		var detailJSON string
		var observed types.Timestamp
		if err := rows.Scan(&ev.ID, &ev.Kind, &ev.ResourceID, &ev.Summary, &detailJSON, &q, &s, &tsScan{&observed}); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(detailJSON), &ev.Detail)
		ev.Provenance = domain.Provenance{Quality: domain.DataQuality(q), Source: s, ObservedAt: observed}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func loadFindings(ctx context.Context, db queryer, runID types.AnalysisRunID) ([]domain.Finding, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, rule_id, fingerprint, severity, category, title, description, resource_ids_json, evidence_ids_json,
			assumptions_json, confidence_bps, quality, source, observed_at
		FROM findings WHERE analysis_run_id = ? ORDER BY id`, string(runID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Finding
	for rows.Next() {
		var f domain.Finding
		var q, s string
		var resJSON, evJSON, asmpJSON string
		var observed types.Timestamp
		if err := rows.Scan(&f.ID, &f.RuleID, &f.Fingerprint, &f.Severity, &f.Category, &f.Title, &f.Description,
			&resJSON, &evJSON, &asmpJSON, &f.Confidence.BasisPoints, &q, &s, &tsScan{&observed}); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(resJSON), &f.ResourceIDs); err != nil {
			return nil, fmt.Errorf("decode resource ids: %w", err)
		}
		if err := json.Unmarshal([]byte(evJSON), &f.EvidenceIDs); err != nil {
			return nil, fmt.Errorf("decode evidence ids: %w", err)
		}
		if err := json.Unmarshal([]byte(asmpJSON), &f.Assumptions); err != nil {
			return nil, fmt.Errorf("decode assumptions: %w", err)
		}
		f.Provenance = domain.Provenance{Quality: domain.DataQuality(q), Source: s, ObservedAt: observed}
		out = append(out, f)
	}
	return out, rows.Err()
}

func loadRecommendations(ctx context.Context, db queryer, runID types.AnalysisRunID) ([]domain.Recommendation, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, finding_id, summary, steps_json, risk_level,
			savings_minor, savings_currency, savings_low_minor, savings_high_minor,
			savings_class, investigation_only, overlap_key, savings_inputs_json,
			quality, source, observed_at
		FROM recommendations WHERE analysis_run_id = ? ORDER BY id`, string(runID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Recommendation
	for rows.Next() {
		var rec domain.Recommendation
		var q, s string
		var stepsJSON string
		var savingsMinor sql.NullInt64
		var savingsCurrency sql.NullString
		var savingsLow sql.NullInt64
		var savingsHigh sql.NullInt64
		var savingsClass sql.NullString
		var inv int
		var overlapKey sql.NullString
		var inputsJSON sql.NullString
		var observed types.Timestamp
		if err := rows.Scan(&rec.ID, &rec.FindingID, &rec.Summary, &stepsJSON, &rec.RiskLevel,
			&savingsMinor, &savingsCurrency, &savingsLow, &savingsHigh, &savingsClass, &inv, &overlapKey, &inputsJSON,
			&q, &s, &tsScan{&observed}); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(stepsJSON), &rec.Steps); err != nil {
			return nil, err
		}
		if savingsMinor.Valid {
			rec.EstSavings = &types.Money{AmountMinor: savingsMinor.Int64, Currency: savingsCurrency.String}
		}
		if savingsLow.Valid {
			rec.EstSavingsLow = &types.Money{AmountMinor: savingsLow.Int64, Currency: savingsCurrency.String}
		}
		if savingsHigh.Valid {
			rec.EstSavingsHigh = &types.Money{AmountMinor: savingsHigh.Int64, Currency: savingsCurrency.String}
		}
		if savingsClass.Valid {
			rec.SavingsClass = domain.SavingsClassification(savingsClass.String)
		}
		rec.InvestigationOnly = inv != 0
		if overlapKey.Valid {
			rec.OverlapKey = overlapKey.String
		}
		if inputsJSON.Valid && inputsJSON.String != "" {
			_ = json.Unmarshal([]byte(inputsJSON.String), &rec.SavingsInputs)
		}
		rec.Provenance = domain.Provenance{Quality: domain.DataQuality(q), Source: s, ObservedAt: observed}
		out = append(out, rec)
	}
	return out, rows.Err()
}
