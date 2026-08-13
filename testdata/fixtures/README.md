# Offline fixture format (Step 02)

YAML files describe a **complete** collection snapshot for local development and tests. This is not the rule manifest format (Step 03).

## Semantics

- **Default import**: each `Import` call assigns a new random `snapshot_id`. Importing the same file twice creates **two separate immutable snapshots**.
- **Idempotent import**: set top-level `external_key` to a stable string. A second import with the same `account.id` and `external_key` returns the existing snapshot ID without duplicating rows.

## Required sections

| Section | Purpose |
|---------|---------|
| `format_version` | Must be `1` |
| `external_key` | Optional idempotency key |
| `account` | One cloud account |
| `regions` | At least one region (sample uses two) |
| `resources` | Inventory nodes with `kind`, `provider_resource_id`, `state`, optional `tags` and `attributes` |
| `relationships` | Graph edges; set `target_missing: true` when the target resource is absent |
| `costs` | Money in **major units** (`amount_major`) plus `currency` |
| `metrics` | Named series with `points` (`timestamp`, `value`, `unit`) |

## Sample

See [sample.yaml](sample.yaml) — one account, `us-east-1` and `eu-west-1`, running/stopped instances, attached/unattached volumes, RDS, NAT/subnet/VPC, costs, and CPU utilization metrics.

## Usage (tests / future CLI)

```go
importer := fixture.NewImporter(repo)
id, err := importer.Import(ctx, "testdata/fixtures/sample.yaml")
```
