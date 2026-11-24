#!/usr/bin/env bash
set -euo pipefail

TRAFFIC_INTERVAL="${TRAFFIC_INTERVAL:-0.4}"

cleanup() {
    echo ""
    echo "[playground] Shutting down..."
    tcbroker stop >/dev/null 2>&1 || true
    kill $(jobs -p) 2>/dev/null || true
}
trap cleanup INT TERM EXIT

echo "[playground] Setting up virtual network namespaces inside container..."

# Clean any leftover netns (e.g., on container restart)
for ns in client server ids zeek stg; do
    ip netns del "$ns" 2>/dev/null || true
done

# Enable IPv4 forwarding on the broker (root netns)
sysctl -w net.ipv4.ip_forward=1 >/dev/null

# Helper: create a netns + veth pair connected to the broker (root netns)
setup_link() {
    local ns="$1" broker_if="$2" ns_if="$3" broker_ip="$4" ns_ip="$5" ns_mac="$6" gw="${7:-}"

    ip netns add "$ns"
    ip netns exec "$ns" ip link set lo up

    ip link delete "$broker_if" 2>/dev/null || true
    ip link add "$broker_if" type veth peer name "$ns_if"
    ip link set "$ns_if" netns "$ns"

    ip link set "$broker_if" up
    if [ -n "$broker_ip" ]; then
        ip addr add "$broker_ip" dev "$broker_if"
    fi

    if [ -n "$ns_mac" ]; then
        ip netns exec "$ns" ip link set "$ns_if" address "$ns_mac"
    fi
    ip netns exec "$ns" ip link set "$ns_if" up
    if [ -n "$ns_ip" ]; then
        ip netns exec "$ns" ip addr add "$ns_ip" dev "$ns_if"
    fi
    if [ -n "$gw" ]; then
        ip netns exec "$ns" ip route add default via "$gw"
    fi
}

# 1. Production path: client (10.10.0.2) <-> broker <-> server (10.20.0.2)
setup_link client tap-wan  cli0  10.10.0.1/24 10.10.0.2/24  52:54:00:10:00:02 10.10.0.1
setup_link server tap-srv  srv0  10.20.0.1/24 10.20.0.2/24  52:54:00:20:00:02 10.20.0.1

# 2. Tool / Mirror ports
setup_link ids    tool-ids  ids0  ""           ""            52:54:00:30:00:02
setup_link zeek   tool-zeek zeek0 ""           ""            52:54:00:40:00:02
setup_link stg    tool-stg  stg0  10.50.0.1/24 10.50.0.53/24 52:54:00:50:00:53 10.50.0.1

# Put passive sensor NICs in promiscuous mode so tcpdump captures all mirrored frames
ip netns exec ids  ip link set ids0  promisc on
ip netns exec zeek ip link set zeek0 promisc on

# 3. Start lightweight server listeners (HTTP :80 & UDP :53) in 'server' netns
ip netns exec server python3 -c '
import http.server, socketserver
class Quiet(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = b"ok from server\n"
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *args): pass
socketserver.TCPServer.allow_reuse_address = True
with socketserver.TCPServer(("0.0.0.0", 80), Quiet) as httpd:
    httpd.serve_forever()
' &

ip netns exec server python3 -c '
import socket
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.bind(("0.0.0.0", 53))
while True:
    data, addr = s.recvfrom(2048)
    s.sendto(b"dns-reply:" + data, addr)
' &

# 4. Start shadow UDP :53 receiver in 'stg' netns (proves L2+L3 rewrite + checksum recalculation!)
ip netns exec stg python3 -c '
import socket
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.bind(("0.0.0.0", 53))
count = 0
while True:
    data, addr = s.recvfrom(2048)
    count += 1
    if count == 1 or count % 25 == 0:
        print(f"[stg shadow receiver] received rewritten UDP/53 packet #{count} from {addr[0]}:{addr[1]} ({len(data)} bytes)", flush=True)
' &

# 5. Apply real Linux TC mirroring rules via tcbroker
echo "[playground] Applying real Linux TC rules from /playground/config.yaml..."
if ! tcbroker apply /playground/config.yaml; then
    echo ""
    echo "[playground] ERROR: Failed to apply Linux TC rules."
    echo "  If you see 'Specified qdisc kind is unknown', your host Linux kernel does not"
    echo "  have sch_clsact / cls_flower / act_mirred / act_vlan / act_police enabled."
    echo "  Try: sudo modprobe sch_clsact cls_flower act_mirred act_vlan act_police act_skbmod act_pedit act_csum"
    echo "  Or explore the WebUI without kernel TC using: tcbroker serve --demo"
    exit 1
fi
tcbroker status

# 6. Start background live traffic generator in 'client' netns (HTTP + UDP/53 + ICMP)
echo "[playground] Starting live traffic generator (client 10.10.0.2 -> server 10.20.0.2)..."
ip netns exec client python3 -c "
import socket, time, urllib.request
interval = float('${TRAFFIC_INTERVAL}')
udp = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
udp.settimeout(0.2)
seq = 0
while True:
    seq += 1
    try:
        with urllib.request.urlopen('http://10.20.0.2/api/health', timeout=1) as r:
            r.read()
    except Exception:
        pass
    try:
        udp.sendto(f'query-{seq}.example.internal'.encode(), ('10.20.0.2', 53))
        udp.recvfrom(1024)
    except Exception:
        pass
    time.sleep(interval)
" &

echo ""
echo "======================================================================"
echo " tcbroker Real Linux TC Playground is LIVE!"
echo "----------------------------------------------------------------------"
echo " WebUI:   http://localhost:8080"
echo " Metrics: http://localhost:8080/metrics"
echo ""
echo " Inspect real mirrored packets with tcpdump in another terminal:"
echo "   # See VLAN 100 tagged HTTP packets arriving at IDS:"
echo "   docker exec -it tcbroker-playground ip netns exec ids tcpdump -i ids0 -e -nn -c 10"
echo ""
echo "   # See MAC/IP rewritten UDP/53 packets arriving at staging:"
echo "   docker exec -it tcbroker-playground ip netns exec stg tcpdump -i stg0 -e -nn -c 10"
echo ""
echo "   # Run CLI commands directly against live kernel TC:"
echo "   docker exec -it tcbroker-playground tcbroker status"
echo "======================================================================"
echo ""

# Run mini-NPB WebUI server in foreground (real kernel TC, not --demo!)
exec tcbroker serve --listen 0.0.0.0:8080
