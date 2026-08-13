-- Per-service collection coverage for partial snapshot semantics.

CREATE TABLE IF NOT EXISTS snapshot_service_coverage (
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    service TEXT NOT NULL,
    region TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (snapshot_id, service, region)
);

CREATE INDEX IF NOT EXISTS idx_snapshot_service_coverage_snapshot
    ON snapshot_service_coverage(snapshot_id);
