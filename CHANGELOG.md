# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.0] - 2026-11-24

### Added
- **Stateful TC Management (`pkg/state`, `pkg/manager`)**
  - Atomic runtime state file (`/run/tcbroker/state.json`, mode `0644`) and dedicated priority range (`20000–29999`) with two-generation blue/green swaps and automatic rollback on failure.
  - Safe coexistence with external TC tools: targeted `pref`/`handle` deletion and ownership-tracked `clsact` qdisc cleanup.
  - Config-less `tcbroker status [rule]` (with `--watch`/`-w`, `--json`, `--raw`, `--all`, and drift detection) and `tcbroker delete [rule...]` (aliases `stop`, `rm`).
  - Added `tcbroker init` starter template generator and `tcbroker version`.
- **Embedded Mini-NPB WebUI & Prometheus Metrics (`tcbroker serve` / alias `ui`)**
  - Zero-dependency embedded WebUI (`go:embed`, no external CDN) with 3-column SVG flow topology, traffic-scaled wires, live PPS/bps sparklines, Dry-Run/Apply modals, YAML export, and one-click drift reconciliation.
  - Prometheus `/metrics` endpoint exposing rule and per-filter health, packet, byte, and drop counters.
  - `--read-only` (non-root viewing), `--token` Bearer authentication, and `--demo` live traffic simulation mode.
- **NPB Packet Conditioning & Port Metadata**
  - `ports:` section in YAML for human-readable interface labels and roles (`tap` / `tool`) displayed across CLI `status`, `validate`, and WebUI.
  - Non-destructive `vlan: { push: <1-4094> }` (push before mirror, pop afterward) and `vlan: { pop: true }`.
  - Safe non-dropping `rate_limit` via `tc police ... conform-exceed continue/pipe`.
- **Self-Contained Docker Playground (`playground/`)**
  - Single-container real Linux TC playground using internal `ip netns` (`make playground`), runnable on Linux, macOS, and Windows with Docker alone.
- **Automated Releases & Multi-Arch Packages**
  - Tag push (`v*.*.*`) or `workflow_dispatch` automatically verifies tests, builds `linux/amd64` + `linux/arm64` archives with `checksums.txt`, creates the GitHub Release, and publishes multi-arch Docker images (`:2.0.0`, `:2.0`, `:2`, `:latest`) to GHCR.
- **Documentation**
  - Added `docs/product-vision.md`, refreshed `docs/architecture.md`, and embedded live WebUI screenshot in `README.md`.

### Changed
- Refreshed declarative YAML schema (`ports`, `rules`, `src`, optional `direction`, flexible `dst`, optional `filters`, `vlan`, `rate_limit`, `rewrite`) with strict unknown-field checking (`KnownFields(true)`).
- Replaced `cobra`/`pflag`/`mousetrap` with a standard-library `flag` CLI dispatcher (external Go dependency reduced to `gopkg.in/yaml.v3` only).
- Added `pull_request` trigger to CI workflow (`.github/workflows/ci.yml`).
- Simplified `Dockerfile` with native Buildx cross-compilation and `VERSION` injection.

### Fixed
- Fixed `action csum` tokenization (`ip4h`), missing trailing `pipe`, and `skbmod` multi-attribute syntax in `pkg/filter/builder.go`.
- Fixed `staticcheck QF1002` lint issue in `pkg/manager/manager.go`.

### Removed
- Removed obsolete `development-tasks.md` (superseded by `docs/product-vision.md`) and unused helper functions.

## [1.0.0] - 2025-11-22

### Added
- Initial release of `tcbroker` with stateless CLI (`start`, `stop`, `status`, `validate`), 5-tuple `flower` filtering, `mirred` mirroring, MAC/IP rewriting, and Docker test environment.
