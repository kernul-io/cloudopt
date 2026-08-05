-- Core canonical model tables.

CREATE TABLE IF NOT EXISTS accounts (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    provider_account_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    default_currency TEXT NOT NULL,
    quality TEXT NOT NULL,
    source TEXT NOT NULL,
    observed_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS snapshots (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id),
    provider TEXT NOT NULL,
    status TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    external_key TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    completed_at TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_snapshots_account_external_key
    ON snapshots(account_id, external_key)
    WHERE external_key != '';

CREATE INDEX IF NOT EXISTS idx_snapshots_account ON snapshots(account_id);
CREATE INDEX IF NOT EXISTS idx_snapshots_status ON snapshots(status);

CREATE TABLE IF NOT EXISTS snapshot_regions (
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    id TEXT NOT NULL,
    provider_region_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    quality TEXT NOT NULL,
    source TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    PRIMARY KEY (snapshot_id, id)
);

CREATE TABLE IF NOT EXISTS resources (
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    id TEXT NOT NULL,
    kind TEXT NOT NULL,
    provider_resource_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    region_id TEXT NOT NULL,
    name TEXT NOT NULL,
    state TEXT NOT NULL,
    attributes_json TEXT NOT NULL DEFAULT '{}',
    quality TEXT NOT NULL,
    source TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    PRIMARY KEY (snapshot_id, id)
);

CREATE INDEX IF NOT EXISTS idx_resources_snapshot ON resources(snapshot_id);

CREATE TABLE IF NOT EXISTS resource_tags (
    snapshot_id TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (snapshot_id, resource_id, key),
    FOREIGN KEY (snapshot_id, resource_id) REFERENCES resources(snapshot_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS relationships (
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL,
    from_resource_id TEXT NOT NULL,
    to_resource_id TEXT NOT NULL,
    to_provider_resource_id TEXT NOT NULL DEFAULT '',
    target_missing INTEGER NOT NULL DEFAULT 0,
    quality TEXT NOT NULL,
    source TEXT NOT NULL,
    observed_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_relationships_snapshot ON relationships(snapshot_id);

CREATE TABLE IF NOT EXISTS cost_records (
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    resource_id TEXT NOT NULL,
    service TEXT NOT NULL,
    amount_minor INTEGER NOT NULL,
    currency TEXT NOT NULL,
    granularity TEXT NOT NULL,
    period_start TEXT NOT NULL,
    period_end TEXT NOT NULL,
    quality TEXT NOT NULL,
    source TEXT NOT NULL,
    observed_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_costs_snapshot ON cost_records(snapshot_id);

CREATE TABLE IF NOT EXISTS metric_series (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    resource_id TEXT NOT NULL,
    name TEXT NOT NULL,
    statistic TEXT NOT NULL,
    quality TEXT NOT NULL,
    source TEXT NOT NULL,
    observed_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS metric_points (
    series_id INTEGER NOT NULL REFERENCES metric_series(id) ON DELETE CASCADE,
    ts TEXT NOT NULL,
    value REAL NOT NULL,
    unit TEXT NOT NULL,
    quality TEXT NOT NULL,
    PRIMARY KEY (series_id, ts)
);

CREATE TABLE IF NOT EXISTS analysis_runs (
    id TEXT PRIMARY KEY,
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    rule_set_version TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_analysis_snapshot ON analysis_runs(snapshot_id);

CREATE TABLE IF NOT EXISTS evidence (
    analysis_run_id TEXT NOT NULL REFERENCES analysis_runs(id) ON DELETE CASCADE,
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    summary TEXT NOT NULL,
    detail_json TEXT NOT NULL DEFAULT '{}',
    quality TEXT NOT NULL,
    source TEXT NOT NULL,
    observed_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS findings (
    analysis_run_id TEXT NOT NULL REFERENCES analysis_runs(id) ON DELETE CASCADE,
    id TEXT NOT NULL,
    rule_id TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    severity TEXT NOT NULL,
    category TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    resource_ids_json TEXT NOT NULL,
    evidence_ids_json TEXT NOT NULL,
    assumptions_json TEXT NOT NULL,
    confidence_bps INTEGER NOT NULL,
    quality TEXT NOT NULL,
    source TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    PRIMARY KEY (analysis_run_id, id)
);

CREATE TABLE IF NOT EXISTS recommendations (
    analysis_run_id TEXT NOT NULL REFERENCES analysis_runs(id) ON DELETE CASCADE,
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    finding_id TEXT NOT NULL,
    summary TEXT NOT NULL,
    steps_json TEXT NOT NULL,
    risk_level TEXT NOT NULL,
    savings_minor INTEGER,
    savings_currency TEXT,
    quality TEXT NOT NULL,
    source TEXT NOT NULL,
    observed_at TEXT NOT NULL
);
