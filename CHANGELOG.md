# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.0] - 2025-01-20

### ⚠️ BREAKING CHANGES
- Complete configuration structure migration from `interfaces` to `rules` format
- Configuration files from v1.x are NOT compatible with v2.0.0
- Field name changes: `protocol` → `ip_proto` in filters
- No backward compatibility support

### Changed
- **Configuration Structure Redesign**
  - Migrated from nested `interfaces` → `mirrors` structure to flat `rules` structure
  - Simplified interface specification: `name` + `direction` → `src_intf` (direction always ingress)
  - Renamed `mirrors[].target` → `dst_intf`
  - Renamed `mode` → `action` (values: `mirror` or `redirect`)
  - Filter field renamed: `protocol` → `ip_proto` for consistency with tc syntax
  - Reduced nesting depth from 2 levels to 1 level

### Added
- **Packet Redirection and NAT Support**
  - Full support for `action: redirect` (packet forwarding/consumption)
  - Packet header rewriting via `rewrite` options:
    - MAC address rewriting: `dst_mac`, `src_mac`
    - IP address rewriting: `dst_ip`, `src_ip` (DNAT/SNAT)
  - Automatic checksum recalculation for IP/TCP/UDP/ICMP when IP is modified
  - L2 forwarding (MAC rewrite only) for same-subnet redirection
  - L3 NAT (IP + MAC rewrite) for cross-subnet forwarding

- **Enhanced Validation**
  - Validation rule: `rewrite` options cannot be used with `action: mirror`
  - MAC address format validation
  - IP address format validation
  - Comprehensive error messages for configuration issues

- **Improved Examples**
  - Consolidated all examples into `configs/examples/simple.yaml`
  - Examples for:
    - Simple mirroring (monitoring/observability)
    - L2 redirect with MAC rewrite
    - L3 redirect with DNAT
    - Full NAT with DNAT + SNAT

### Updated
- Complete rewrite of `pkg/config/types.go` with new `Rule` struct
- Complete rewrite of `pkg/config/validator.go` for rules-based validation
- Updated `pkg/filter/types.go` to use `IPProto` field
- Updated `pkg/filter/builder.go` to generate pedit and csum actions
- Rewrote all commands (`start`, `stop`, `status`, `validate`) for rules-based config
- Updated all unit tests to test rules-based structure
- Completely rewrote `docs/config-reference.md` with comprehensive documentation
- Updated README.md with new configuration format examples

### Removed
- Old `Interface` and `Mirror` struct types
- Support for `version` field in configuration (no longer needed)
- Old test configuration files (test-config.yaml, test-network-config.yaml)

### Migration Guide
To migrate from v1.x to v2.0.0, update your configuration file:

**Before (v1.x):**
```yaml
version: "1.0"
interfaces:
  - name: eth0
    direction: ingress
    mirrors:
      - name: mirror1
        target: eth1
        mode: mirror
        filters:
          - protocol: tcp
            dst_port: 80
```

**After (v2.0.0):**
```yaml
rules:
  - src_intf: eth0
    dst_intf: eth1
    action: mirror
    filters:
      - ip_proto: tcp
        dst_port: 80
```

## [1.0.0] - 2025-01-18

### Added
- Initial release of tcbroker
- Complete CLI with start, stop, status, validate, and version commands
- Declarative YAML configuration for packet mirroring rules
- 5-tuple filtering support (protocol, src/dst IP, src/dst port)
- Support for TCP, UDP, and ICMP protocols
- Traffic direction control (ingress, egress, both)
- Real-time status monitoring with statistics
- Dry-run mode for safe configuration testing
- Debug mode to show tc commands being executed
- Docker-based integration test environment
- Comprehensive documentation
  - Architecture diagrams (Mermaid format)
  - Configuration reference guide
  - Docker testing guide
- CI/CD pipeline with GitHub Actions
- Code linting with golangci-lint
- Pull request template

### Technical Details
- Uses Linux Traffic Control (tc) with clsact qdisc
- Flower matcher for packet filtering
- Mirred action for packet mirroring
- Idempotent operations (safe to re-apply configurations)

### Requirements
- Linux kernel with tc support
- Go 1.21+ (for building from source)
- Root/sudo privileges for tc operations

[1.0.0]: https://github.com/your-username/tcbroker/releases/tag/v1.0.0
