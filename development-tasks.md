# tcbroker Development Tasks

## Current Status

tcbroker v1.0.0 is complete with the following features:
- ✅ Rules-based YAML configuration
- ✅ Traffic mirroring with TC flower filters
- ✅ Packet header rewriting (MAC/IP)
- ✅ CLI commands: start, stop, status, validate, version
- ✅ Docker test environment
- ✅ CI/CD pipeline (GitHub Actions)
- ✅ Comprehensive documentation

---

## Future Enhancements (Phase 3+)

### Additional TC Actions

**Priority:** Medium

Potential new actions to implement:

1. **drop / shot**
   - Use case: Security policies, DDoS mitigation, packet loss testing
   - Implementation: `action drop` or `action shot`

2. **police (rate limiting)**
   - Use case: QoS, bandwidth control, DoS mitigation
   - Configuration: rate, burst, action on exceed

3. **vlan (VLAN operations)**
   - Use case: Network segmentation, VLAN trunking
   - Operations: push, pop, modify VLAN tags

4. **skbedit (QoS/priority)**
   - Use case: Traffic prioritization, queue mapping
   - Configuration: priority, queue_mapping, mark

5. **sample (packet sampling)**
   - Use case: High-traffic monitoring, sFlow/NetFlow
   - Configuration: sampling rate, truncation size

### Operational Improvements

**Priority:** Low

1. **Daemon Mode**
   - Background process with Unix socket communication
   - Real-time statistics collection
   - Dynamic configuration updates

2. **REST API**
   - HTTP API server for remote control
   - Web UI integration
   - System integration support

3. **systemd Integration**
   - Service file for automatic startup
   - Log management with journald
   - Health monitoring

4. **Advanced Features**
   - Multiple destination support
   - Load balancing
   - Encapsulation (VXLAN, etc.)
   - Stateful filtering

### eBPF Integration (Phase 4)

**Priority:** TBD

Potential migration to eBPF for:
- More detailed statistics
- Custom packet processing
- Better performance at scale
- Advanced filtering capabilities

---

## Known Limitations

Current implementation limitations:

- Mirror-only action (no redirect mode in current version)
- No encapsulation support (VXLAN, etc.)
- Stateless packet processing
- Limited to TC capabilities
- Performance constraints with high traffic volumes

---

## Contributing

See [README.md](README.md) for contribution guidelines.

## References

- [Linux Traffic Control HOWTO](https://tldp.org/HOWTO/Traffic-Control-HOWTO/)
- [tc man page](https://man7.org/linux/man-pages/man8/tc.8.html)
- [tc-flower man page](https://man7.org/linux/man-pages/man8/tc-flower.8.html)
- [tc-mirred man page](https://man7.org/linux/man-pages/man8/tc-mirred.8.html)
