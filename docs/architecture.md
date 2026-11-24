# tcbroker Architecture

## 1. System Overview

`tcbroker` is a stateful mini-NPB (Network Packet Broker) built directly on Linux Traffic Control (`tc`) with a single Go dependency (`gopkg.in/yaml.v3`). Both the CLI and the embedded WebUI/API share the same `pkg/manager` engine and `/run/tcbroker/state.json` state file.

```mermaid
flowchart TD
    User["Operator / Script"] --> CLI["cmd/tcbroker (stdlib flag CLI)"]
    Browser["Browser (Mini-NPB WebUI)"] --> Server["pkg/server (HTTP + go:embed)"]
    Prom["Prometheus"] -->|GET /metrics| Server

    CLI --> Config["pkg/config (Strict YAML + Validation)"]
    CLI --> Manager["pkg/manager (Diff / Swap / Rollback / Drift)"]
    Server --> Config
    Server --> Manager

    Manager <--> State["pkg/state (/run/tcbroker/state.json)"]
    Manager --> TC["pkg/tc (CommandExecutor + Parser)"]
    TC --> Filter["pkg/filter (TC Arg Builder)"]
    TC <--> Kernel["Linux Kernel TC (clsact + flower + mirred)"]
```

## 2. Package Responsibilities

| Package | Responsibility |
|---|---|
| `cmd/tcbroker` | Zero-dependency CLI (`apply`, `status`, `delete`/`stop`, `validate`, `init`, `serve`/`ui`, `version`) using standard library `flag` with flexible argument ordering |
| `pkg/config` | Declarative YAML schema (`ports`, `rules`, flexible `dst`, optional `filters`, `vlan`, `rate_limit`, `rewrite`), strict `KnownFields` decoding, validation, and operational warnings |
| `pkg/filter` | Builds exact `tc filter add/del` argument lists with `pref`/`handle`, safe non-dropping `police`, `skbmod`/`pedit`/`csum`, non-destructive `vlan push`+`pop`, and `mirred egress mirror` |
| `pkg/state` | Atomic JSON persistence (`0644` for non-root `status`), deterministic spec hashing, and `ExportConfig()` / `ExportYAML()` reconstruction |
| `pkg/tc` | Injectable `CommandExecutor` abstraction, read/write `Runner`, targeted `clsact` qdisc & `flower` filter operations, and `tc -s filter show` parser |
| `pkg/manager` | Stateful reconciliation engine: idempotent diffing, two-generation blue/green priority swap (`20000–24999` ↔ `25000–29999`), rollback on failure, external-tool coexistence, and drift detection |
| `pkg/server` | Optional embedded HTTP server (`tcbroker serve`) hosting the 3-column SVG topology WebUI, REST API (with `--read-only` and Bearer `--token` guards), Prometheus `/metrics`, and `--demo` simulator |

## 3. Packet Processing Pipeline inside Linux TC

Each managed rule expands into one or more `flower` filters attached to the source interface's `clsact` qdisc (`ingress` and/or `egress` hook):

```mermaid
flowchart LR
    Pkt["Incoming IPv4 Packet"] --> Match["flower 5-tuple match"]
    Match -->|"No match"| Next["Next filter / Host stack"]
    Match -->|"Match"| Police{"rate_limit?"}
    Police -->|"Exceed"| Next
    Police -->|"Conform / None"| RW{"rewrite?"}
    RW -->|"MAC / IP"| Mod["skbmod / pedit + csum ip4h"]
    RW -->|"None"| VLAN{"vlan push?"}
    Mod --> VLAN
    VLAN -->|"Push ID"| VPush["vlan push id N -> mirred mirror -> vlan pop -> continue"]
    VLAN -->|"Pop / None"| Mirred["mirred egress mirror dev dst -> continue"]
    VPush --> Next
    Mirred --> Next
```

Key safety guarantees:
- **Non-dropping rate cap**: `action police ... conform-exceed continue/pipe` skips only the mirror action when exceeding the cap; original production traffic is never dropped.
- **Non-destructive VLAN push**: `vlan push id <ID> pipe` → `mirred egress mirror ... pipe` → `vlan pop continue` tags only the mirrored copy while leaving the original packet unmodified.
- **External tool safety**: Dedicated priority range `20000–29999` (skipping any priority already used externally). Cleanup removes only `tcbroker`-owned priorities and only removes `clsact` if `tcbroker` created it and no filters remain on either hook.

## 4. Idempotent Reconciliation & Rollback

```mermaid
sequenceDiagram
    participant Mgr as pkg/manager
    participant TC as Linux Kernel TC
    participant St as state.json

    Mgr->>St: Load current managed filters
    Mgr->>TC: List live filters on affected (src, hook)
    Note over Mgr: Compute diff (Keep / Append / Trim / Blue-Green Swap)
    Mgr->>TC: Ensure clsact qdisc (track ownership)
    Mgr->>TC: Add new filters (with opposite generation pref on swap)
    alt Any filter add fails
        Mgr->>TC: Delete newly added filters in reverse order (rollback)
        Mgr-->>Mgr: Return error (previous rules stay intact)
    else All adds succeed
        Mgr->>TC: Delete obsolete filters by pref/handle
        Mgr->>TC: Delete clsact only if owned & empty
        Mgr->>St: Atomically save updated state.json
    end
```
