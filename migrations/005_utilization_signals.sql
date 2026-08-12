-- Derived utilization signals and metrics collection metadata.

CREATE TABLE IF NOT EXISTS snapshot_metrics_meta (
    snapshot_id TEXT PRIMARY KEY REFERENCES snapshots(id) ON DELETE CASCADE,
    window_start TEXT NOT NULL,
    window_end TEXT NOT NULL,
    period_seconds INTEGER NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    business_hour_start INTEGER NOT NULL DEFAULT 9,
    business_hour_end INTEGER NOT NULL DEFAULT 17,
    source TEXT NOT NULL,
    partial INTEGER NOT NULL DEFAULT 0,
    diagnostics_json TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS utilization_signals (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    resource_id TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    kind TEXT NOT NULL,
    value REAL NOT NULL,
    unit TEXT NOT NULL,
    sample_count INTEGER NOT NULL,
    expected_samples INTEGER NOT NULL,
    coverage_ratio REAL NOT NULL,
    zero_samples INTEGER NOT NULL DEFAULT 0,
    missing_samples INTEGER NOT NULL DEFAULT 0,
    query_json TEXT NOT NULL DEFAULT '{}',
    notes_json TEXT NOT NULL DEFAULT '[]',
    quality TEXT NOT NULL,
    source TEXT NOT NULL,
    observed_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_utilization_signals_snapshot ON utilization_signals(snapshot_id);
