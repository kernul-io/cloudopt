-- Extended billing attribution fields on canonical cost records.

ALTER TABLE cost_records ADD COLUMN cost_basis TEXT NOT NULL DEFAULT 'amortized_net';
ALTER TABLE cost_records ADD COLUMN charge_kind TEXT NOT NULL DEFAULT 'usage';
ALTER TABLE cost_records ADD COLUMN region_id TEXT NOT NULL DEFAULT '';
ALTER TABLE cost_records ADD COLUMN attribution_method TEXT NOT NULL DEFAULT 'direct_resource_id';
ALTER TABLE cost_records ADD COLUMN attribution_heuristic TEXT NOT NULL DEFAULT '';
ALTER TABLE cost_records ADD COLUMN attribution_confidence REAL NOT NULL DEFAULT 1.0;
ALTER TABLE cost_records ADD COLUMN source_interval_start TEXT NOT NULL DEFAULT '';
ALTER TABLE cost_records ADD COLUMN source_interval_end TEXT NOT NULL DEFAULT '';
ALTER TABLE cost_records ADD COLUMN source_collected_at TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS snapshot_billing_source_totals (
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    currency TEXT NOT NULL,
    amount_minor INTEGER NOT NULL,
    PRIMARY KEY (snapshot_id, currency)
);
