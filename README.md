# tcbroker

A command-line tool for managing Linux Traffic Control (TC) packet mirroring through declarative YAML configuration.

## Features

- **Declarative Configuration**: Define mirroring rules in simple YAML
- **5-Tuple Filtering**: Filter by protocol, IP addresses, and ports
- **Packet Rewriting**: Modify MAC/IP addresses for L2/L3 forwarding
- **Real-time Monitoring**: View statistics per rule or interface
- **CLI-driven**: Easy-to-use commands with dry-run and debug modes

## Quick Start

### Installation

**From Source:**
```sh
git clone https://github.com/shmuto/tcbroker.git
cd tcbroker
make build
```

**Using Docker:**
```sh
docker pull ghcr.io/shmuto/tcbroker:latest
```

**Pre-built Binaries:**
Download from [GitHub Releases](https://github.com/shmuto/tcbroker/releases)

### Basic Usage

**1. Create Configuration**

```yaml
# config.yaml
rules:
  - name: http-mirror
    src_intf: eth0
    dst_intf: eth1
    filters:
      - ip_proto: tcp
        dst_port: 80

  - name: https-with-rewrite
    src_intf: eth0
    dst_intf: eth1
    rewrite:
      dst_mac: "52:54:00:12:34:56"
    filters:
      - ip_proto: tcp
        dst_port: 443
```

**2. Validate & Apply**

```sh
# Validate configuration
./tcbroker validate config.yaml

# Start mirroring (requires sudo)
sudo ./tcbroker start config.yaml

# Check status
./tcbroker status config.yaml --summary

# Stop mirroring
sudo ./tcbroker stop config.yaml
```

**3. Using Docker**

```sh
# Run with config file mounted
docker run --rm --privileged --network host \
  -v $(pwd)/config.yaml:/config/config.yaml \
  ghcr.io/shmuto/tcbroker:latest start /config/config.yaml

# Check status
docker run --rm --privileged --network host \
  -v $(pwd)/config.yaml:/config/config.yaml \
  ghcr.io/shmuto/tcbroker:latest status /config/config.yaml --summary
```

Note: `--privileged` and `--network host` are required for TC operations.

## Commands

- `tcbroker start <config>` - Apply configuration and start mirroring
- `tcbroker stop <config>` - Stop mirroring and clean up
- `tcbroker status [config]` - Show current status
  - `--summary` - Simple per-rule statistics table
  - `--stats` - Detailed packet/byte counts
  - `--all` - Show all TC rules on system
- `tcbroker validate <config>` - Validate configuration
  - `--check-interfaces` - Verify interfaces exist
- `tcbroker version` - Show version information

### Command Options

- `--debug` - Print TC commands being executed
- `--dry-run` - Preview commands without executing
- `--force` - Clean existing rules before applying (start only)

## Configuration

### Structure

```yaml
rules:
  - name: <string>              # Required: Rule identifier
    src_intf: <string>          # Required: Source interface
    dst_intf: <string>          # Required: Destination interface
    rewrite:                    # Optional: Packet rewriting
      dst_mac: <mac>
      src_mac: <mac>
      dst_ip: <ip>
      src_ip: <ip>
    filters:                    # Required: At least one
      - ip_proto: <tcp|udp|icmp>
        src_ip: <ip/cidr>
        dst_ip: <ip/cidr>
        src_port: <int>
        dst_port: <int>
```

### Examples

**Simple mirroring:**
```yaml
- name: dns-monitor
  src_intf: eth0
  dst_intf: eth1
  filters:
    - ip_proto: udp
      dst_port: 53
```

**With MAC rewrite (L2):**
```yaml
- name: l2-forward
  src_intf: eth0
  dst_intf: eth1
  rewrite:
    dst_mac: "52:54:00:12:34:56"
  filters:
    - ip_proto: tcp
      dst_port: 80
```

**With IP + MAC rewrite (L3/NAT):**
```yaml
- name: nat-forward
  src_intf: eth0
  dst_intf: eth1
  rewrite:
    dst_ip: "10.0.0.100"
    dst_mac: "52:54:00:12:34:56"
  filters:
    - ip_proto: tcp
      dst_port: 22
```

## Testing

```bash
# Unit tests
make test

# Docker integration tests
make docker-test

# Manual testing in container
docker compose -f tests/compose.yaml up -d
docker exec -it tcbroker-broker bash
```

## Documentation

- [Architecture](docs/architecture.md) - System design and diagrams
- [Development Tasks](development-tasks.md) - Future enhancements roadmap

## How It Works

`tcbroker` configures the Linux kernel's Traffic Control (TC) subsystem:

1. Attaches `clsact` qdisc to source interfaces
2. Adds `flower` filters with match criteria
3. Applies `skbmod` (MAC rewrite) and `pedit` (IP rewrite) actions
4. Executes `mirred mirror` action to copy packets
5. Appends `continue` to allow multiple rules per interface

See [Architecture](docs/architecture.md) for detailed diagrams.

## Requirements

- Go 1.21+
- Linux with `tc` command
- Root privileges for applying rules

## Troubleshooting

**Permission denied:**
- Use `sudo` with `start` and `stop` commands

**Interface not found:**
- Check with `ip link show`
- Use `--check-interfaces` with validate

**Preview commands:**
- Use `--dry-run` flag to see TC commands without executing

## Development

```bash
# Build
make build

# Run tests
make test

# Clean
make clean

# Docker tests
make docker-test
```

### Creating a Release

1. **Push a version tag:**
   ```bash
   git tag -a v1.0.1 -m "Release v1.0.1"
   git push origin v1.0.1
   ```

2. **Create GitHub Release (manual):**
   - Go to GitHub → Releases → "Draft a new release"
   - Select the tag (v1.0.1)
   - Write release notes
   - Click "Publish release"

3. **Automated on release:**
   - Binaries are automatically built and attached
   - Docker images are built and pushed to GHCR

See [development-tasks.md](development-tasks.md) for roadmap and progress.

## Contributing

Contributions welcome! Before submitting:

1. Run tests: `make test`
2. Ensure Docker tests pass: `make docker-test`
3. Run linter: `golangci-lint run`

## License

MIT License - see LICENSE file for details.

## Version

Current version: **1.0.0** (see [VERSION](VERSION) file)

For changelog, see [CHANGELOG.md](CHANGELOG.md).
