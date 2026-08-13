# Cloud Optimization Analyzer (COA)

**Cloud Optimization Analyzer** is a read-only CLI for multi-cloud cost and utilization analysis. It collects inventory and usage evidence from cloud providers, runs deterministic optimization rules, and produces shareable reports—all without mutating your infrastructure.

## Installation

<!-- TODO: add install one-liner when release artifacts or package manager distribution is available -->

_Pre-built binaries and install scripts will be documented here._

## Build from source

**Requirements**

- [Go](https://go.dev/dl/) **1.23** or newer

**Steps**

```bash
git clone https://github.com/kernul-io/cloudopt.git
cd cloudopt

go mod tidy
make build          # produces ./main
```

Run tests and lint before submitting changes:

```bash
make test
make lint           # requires golangci-lint
```

Cross-compile for Linux ARM64 (typical container target):

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o main ./cmd/
```

## What it does

<!-- TODO: expand as collect, analyze, and report capabilities ship -->

COA is designed to:

- **Collect** read-only snapshots from cloud APIs (inventory, billing, metrics) into a local workspace backed by SQLite.
- **Analyze** collected data with deterministic, evidence-backed rules (rightsizing, waste, attribution, and related findings).
- **Report** results in machine- and human-friendly formats for consultants and engineering teams.

Provider coverage starts with **AWS**; **GCP**, **Azure**, and **DigitalOcean** are planned on the same canonical model. Commands that are not yet implemented return stable JSON responses and documented exit codes instead of failing silently.

## Usage

Logs are written to **stderr** (text or JSON). Command output intended for scripting is **JSON on stdout**.

| Command   | Description |
|-----------|-------------|
| `version` | Print build version (JSON) |
| `init`    | Create workspace directories and default config (does not overwrite an existing config) |
| `collect` | Read-only cloud snapshots |
| `analyze` | Run rules on collected data |
| `report`  | Generate report output |

Quick start:

```bash
./main version
./main init --workspace-dir "$HOME/.cloudopt"
./main collect --workspace-dir "$HOME/.cloudopt"
```

Use `./main --help` and `./main <command> --help` for flags on each command.

## Configuration

Settings are merged from three sources, **highest precedence first**:

1. **Command flags** — e.g. `--workspace-dir`, `--log-format`, `--config`
2. **Environment variables** — prefix `COA_` (values from the environment are not echoed in error output)
3. **Config file** — default path `<workspace>/config/config.yaml`

Common environment variables:

| Variable | Purpose |
|----------|---------|
| `COA_WORKSPACE_DIR` | Local workspace root (default: `~/.cloudopt`) |
| `COA_CONFIG_DIR` | Configuration directory |
| `COA_DATA_DIR` | SQLite and snapshot data |
| `COA_REPORTS_DIR` | Generated reports |
| `COA_TEMP_DIR` | Temporary files |
| `COA_LOG_FORMAT` | `text` or `json` |
| `COA_LOG_LEVEL` | `debug`, `info`, `warn`, `error` |
| `COA_CONFIG_FILE` | Explicit config file path |

Example `config.yaml` fields: `workspace_dir`, `config_dir`, `data_dir`, `reports_dir`, `temp_dir`, `log_format`, `log_level`.

Cloud credentials belong in your environment or provider-specific config on the machine running COA—never commit secrets to the repository or workspace config.

## Contributing

Contributions are welcome. Please:

1. Open an issue to discuss substantial changes, or pick up an existing one.
2. Fork the repository and create a feature branch from `main`.
3. Keep changes focused; follow existing Go layout under `cmd/` and `internal/`.
4. Run `make test` and `make lint` before opening a pull request.
5. Describe what you changed and how you tested it in the PR description.

By contributing, you agree that your contributions will be licensed under the same terms as the project (GNU GPL v3).

## License

This project is licensed under the **GNU General Public License v3.0**. See [LICENSE](LICENSE) for the full text.
