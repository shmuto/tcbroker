# tcbroker

A stateful, declarative CLI tool for managing Linux Traffic Control (TC) packet mirroring.

## Features

- **Stateful & Config-less Operations**: Check `status`, monitor live rates (`--watch`), or `delete`/`stop` rules without passing the config file again
- **Idempotent Apply & Rollback**: Re-applying a config computes minimal changes, preserves counters on unchanged filters, and rolls back automatically on error
- **Safe Coexistence with Other TC Tools**: Tracks its own filters in a dedicated priority range (`20000–29999`) and never deletes external TC filters or pre-existing `clsact` qdiscs
- **Flexible YAML Configuration**: Supports `ingress`/`egress`/`both`, single or multiple destinations, optional `filters` (omit to match all IPv4), per-destination MAC/IP rewriting, and strict typo validation
- **Drift Detection**: Detects if managed filters were removed externally and restores them on the next `apply`

## Quick Start

### Installation

```sh
git clone https://github.com/shmuto/tcbroker.git
cd tcbroker
make build
```

### Basic Usage

**1. Generate a starter configuration**

```sh
./tcbroker init tcbroker.yaml
```

**2. Edit `tcbroker.yaml`**

```yaml
rules:
  # Minimal: mirror all IPv4 ingress packets from eth0 to eth1
  - name: mirror-all
    src: eth0
    dst: eth1

  # Filter HTTP/HTTPS and mirror to multiple destinations
  - name: web-traffic
    src: eth0
    direction: ingress # ingress (default) | egress | both
    dst:
      - eth1
      - intf: eth2
        rewrite:
          dst_mac: "52:54:00:12:34:56"
    filters:
      - proto: tcp
        dst_port: 80
      - proto: tcp
        dst_port: 443
```

**3. Validate, Preview, Apply & Monitor**

```sh
# Validate syntax, semantics, and interface existence
./tcbroker validate tcbroker.yaml --check-interfaces

# Preview diff and tc commands without root privileges
./tcbroker apply tcbroker.yaml --dry-run

# Apply configuration (idempotent; requires root)
sudo ./tcbroker apply tcbroker.yaml

# Check status and counters (no config file needed)
./tcbroker status

# Inspect a single rule in detail or monitor live PPS/bps
./tcbroker status web-traffic
./tcbroker status --watch

# Delete a single rule, or stop all tcbroker-managed rules
sudo ./tcbroker delete web-traffic
sudo ./tcbroker stop
```

## Commands

| Command | Description |
|---|---|
| `tcbroker init [file]` | Generate a starter YAML configuration (`-` prints to stdout) |
| `tcbroker validate <config>` | Strictly validate YAML syntax/semantics (`--check-interfaces`) |
| `tcbroker apply <config>` | Idempotently apply rules with rollback on error (alias: `start`; supports `--dry-run`) |
| `tcbroker status [rule]` | Show rule health and counters (`--watch`/`-w`, `--interval`, `--json`, `--raw`, `--all`) |
| `tcbroker delete [rule...]` | Delete specific rule(s) or all managed rules with `--all` (aliases: `stop`, `rm`; `stop` without args deletes all) |
| `tcbroker version` | Print version information |

### Global Flags

- `--state-file <path>` - Override state file path (default: `/run/tcbroker/state.json`, or `$TCBROKER_STATE_FILE`)
- `--debug` - Print all executed `tc` commands

## Configuration Reference

```yaml
rules:
  - name: <string>              # Required: unique rule identifier
    src: <string>               # Required: source interface
    direction: <ingress|egress|both> # Optional: default "ingress"
    dst:                        # Required: string, list of strings, or list of objects
      - <intf>
      - intf: <string>
        rewrite:                # Optional: overrides rule-level rewrite for this target
          dst_mac: <mac>
          src_mac: <mac>
          dst_ip: <ipv4>
          src_ip: <ipv4>
    rewrite:                    # Optional: default rewrite for all destinations in this rule
      dst_mac: <mac>
      src_mac: <mac>
      dst_ip: <ipv4>
      src_ip: <ipv4>
    filters:                    # Optional: omit to match all IPv4 traffic
      - proto: <tcp|udp|icmp>
        src_ip: <ipv4|cidr>
        dst_ip: <ipv4|cidr>
        src_port: <1-65535>     # Requires proto: tcp or udp
        dst_port: <1-65535>     # Requires proto: tcp or udp
```

## How It Works

1. Ensures a `clsact` qdisc exists on each `src` interface and records whether `tcbroker` created it.
2. Allocates dedicated `flower` filter priorities in `20000–29999` (skipping any priority already used by external tools) with explicit handles.
3. Applies optional `skbmod` (MAC rewrite) and `pedit` + `csum` (IPv4 rewrite + checksum recalculation), followed by `mirred egress mirror ... continue`.
4. Persists ownership metadata atomically to `/run/tcbroker/state.json` (mode `0644` so non-root users can run `status`).
5. On `delete`/`stop`, removes only `tcbroker`-owned filters by priority/handle, and removes `clsact` only if `tcbroker` created it and no other filters remain.

## Testing

```bash
# Unit tests
make test

# Build binary
make build

# Docker integration tests
make docker-test
```

## License

MIT License - see [LICENSE](LICENSE) for details.
