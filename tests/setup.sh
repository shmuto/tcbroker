#!/usr/bin/env bash
# Host-side test setup script
# This script must be run from the host (not inside a container)
set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/compose.yaml"

# Docker parameters
DOCKER_COMPOSE="docker compose -f $COMPOSE_FILE"
DOCKER_EXEC="docker exec tcbroker-broker"

#==============================================================================
# Setup Functions
#==============================================================================

setup_veth() {
    cleanup_veth # Ensure a clean slate before setting up new veth pairs
    echo -e "${BLUE}Setting up veth pairs and connecting to containers...${NC}"

    # Get container PIDs
    CLIENT_PID=$(docker inspect -f '{{.State.Pid}}' tcbroker-client)
    SERVER_PID=$(docker inspect -f '{{.State.Pid}}' tcbroker-server)
    MIRROR_PID=$(docker inspect -f '{{.State.Pid}}' tcbroker-mirror)
    BROKER_PID=$(docker inspect -f '{{.State.Pid}}' tcbroker-broker)

    # Verify all PIDs were obtained
    if [ -z "$CLIENT_PID" ] || [ -z "$SERVER_PID" ] || [ -z "$MIRROR_PID" ] || [ -z "$BROKER_PID" ]; then
        echo -e "${RED}Error: One or more test containers are not running.${NC}"
        exit 1
    fi

    # Create veth pair 1: broker2client <-> client2broker
    ip link add broker2client type veth peer name client2broker

    # Move client2broker into client container's network namespace
    ip link set client2broker netns $CLIENT_PID
    ip link set broker2client netns $BROKER_PID

    # Configure interface for client side
    nsenter -t $CLIENT_PID -n ip link set client2broker up
    nsenter -t $CLIENT_PID -n ip link set client2broker address 00:00:00:00:10:02
    nsenter -t $CLIENT_PID -n ip addr add 192.168.10.2/24 dev client2broker

    # Configure interface for broker side
    nsenter -t $BROKER_PID -n ip link set broker2client up
    nsenter -t $BROKER_PID -n ip link set broker2client address 00:00:00:00:10:01
    nsenter -t $BROKER_PID -n ip addr add 192.168.10.1/24 dev broker2client

    echo -e "${GREEN}✓ Connected broker2client <-> client2broker (in client container)${NC}"

    # Create veth pair 2: broker2server <-> server2broker
    ip link add broker2server type veth peer name server2broker

    # Move server2broker into server container's network namespace
    ip link set server2broker netns $SERVER_PID
    ip link set broker2server netns $BROKER_PID

    # Configure server-side interface
    nsenter -t $SERVER_PID -n ip link set server2broker up
    nsenter -t $SERVER_PID -n ip link set server2broker address 00:00:00:00:20:02
    nsenter -t $SERVER_PID -n ip addr add 192.168.20.2/24 dev server2broker

    # Configure broker-side interface
    nsenter -t $BROKER_PID -n ip link set broker2server up
    nsenter -t $BROKER_PID -n ip link set broker2server address 00:00:00:00:20:01
    nsenter -t $BROKER_PID -n ip addr add 192.168.20.1/24 dev broker2server

    echo -e "${GREEN}✓ Connected broker2server <-> server2broker (in server container)${NC}"

    # Create veth pair 3: broker2mirror <-> mirror2broker
    ip link add broker2mirror type veth peer name mirror2broker

    # Move mirror2broker into mirror container's network namespace
    ip link set mirror2broker netns $MIRROR_PID
    ip link set broker2mirror netns $BROKER_PID

    # Configure mirror-side interface
    nsenter -t $MIRROR_PID -n ip link set mirror2broker up
    nsenter -t $MIRROR_PID -n ip link set mirror2broker address 00:00:00:00:30:02
    nsenter -t $MIRROR_PID -n ip addr add 192.168.30.2/24 dev mirror2broker

    # Configure broker-side interface
    nsenter -t $BROKER_PID -n ip link set broker2mirror up
    nsenter -t $BROKER_PID -n ip link set broker2mirror address 00:00:00:00:30:01
    nsenter -t $BROKER_PID -n ip addr add 192.168.30.1/24 dev broker2mirror

    echo -e "${GREEN}✓ Connected broker2mirror <-> mirror2broker (in mirror container)${NC}"

    echo -e "${GREEN}✓ All veth pairs ready and connected to containers${NC}"
}

cleanup_veth() {
    echo -e "${BLUE}Cleaning up veth pairs...${NC}"

    # Get container PIDs
    CLIENT_PID=$(docker inspect -f '{{.State.Pid}}' tcbroker-client 2>/dev/null || true)
    SERVER_PID=$(docker inspect -f '{{.State.Pid}}' tcbroker-server 2>/dev/null || true)
    MIRROR_PID=$(docker inspect -f '{{.State.Pid}}' tcbroker-mirror 2>/dev/null || true)

    # Clean up host-side interfaces
    if ip link show broker2client &>/dev/null; then
        ip link delete broker2client
        echo -e "${GREEN}✓ Deleted host interface: broker2client${NC}"
    fi
    if ip link show broker2server &>/dev/null; then
        ip link delete broker2server
        echo -e "${GREEN}✓ Deleted host interface: broker2server${NC}"
    fi
    if ip link show broker2mirror &>/dev/null; then
        ip link delete broker2mirror
        echo -e "${GREEN}✓ Deleted host interface: broker2mirror${NC}"
    fi

    # Clean up container-side interfaces
    if [ -n "$CLIENT_PID" ]; then
        nsenter -t $CLIENT_PID -n ip link show client2broker &>/dev/null && \
        nsenter -t $CLIENT_PID -n ip link delete client2broker && \
        echo -e "${GREEN}✓ Deleted container interface: client2broker${NC}"
    fi
    if [ -n "$SERVER_PID" ]; then
        nsenter -t $SERVER_PID -n ip link show server2broker &>/dev/null && \
        nsenter -t $SERVER_PID -n ip link delete server2broker && \
        echo -e "${GREEN}✓ Deleted container interface: server2broker${NC}"
    fi
    if [ -n "$MIRROR_PID" ]; then
        nsenter -t $MIRROR_PID -n ip link show mirror2broker &>/dev/null && \
        nsenter -t $MIRROR_PID -n ip link delete mirror2broker && \
        echo -e "${GREEN}✓ Deleted container interface: mirror2broker${NC}"
    fi

    echo -e "${GREEN}✓ All veth pairs cleaned up${NC}"
}

#==============================================================================
# Main Functions
#==============================================================================

start_docker() {
    echo -e "${BLUE}Starting Docker containers...${NC}"
    $DOCKER_COMPOSE up -d
    echo -e "${GREEN}✓ Docker containers started${NC}"
    echo
    sleep 2
}

stop_docker() {
    echo -e "${BLUE}Stopping Docker containers...${NC}"
    cleanup_veth
    $DOCKER_COMPOSE down
    echo -e "${GREEN}✓ Docker containers stopped${NC}"
}

run_test() {
    echo -e "${BLUE}Running tests inside container...${NC}"
    $DOCKER_EXEC bash /workspace/tests/test.sh test
}

run_full_test() {
    echo -e "${YELLOW}=== Starting Full Docker Test ===${NC}"
    echo

    # Start Docker containers
    start_docker

    # Setup veth pairs
    setup_veth
    echo

    # Run tests inside container
    run_test

    echo
    echo -e "${GREEN}=== Full Test Completed ===${NC}"
}

run_interactive() {
    echo -e "${YELLOW}=== Starting Interactive Test Environment ===${NC}"
    echo

    # Start Docker containers
    start_docker

    # Setup veth pairs
    setup_veth
    echo

    # Run interactive mode inside container
    echo -e "${BLUE}Starting interactive session inside broker container...${NC}"
    $DOCKER_EXEC bash /workspace/tests/test.sh interactive
}

#==============================================================================
# CLI
#==============================================================================

show_help() {
    cat <<EOF
Usage: $0 [COMMAND]

Host-side test setup and orchestration script.

Commands:
    up              Start Docker containers
    down            Stop Docker containers and cleanup
    setup-veth      Setup veth pairs (requires containers running)
    cleanup-veth    Remove veth pairs
    test            Run full automated test (up + setup + test + cleanup)
    interactive     Start interactive testing environment
    help            Show this help message

Examples:
    sudo $0 test             # Run full automated test
    sudo $0 interactive      # Start interactive mode
    sudo $0 up               # Just start containers
    sudo $0 setup-veth       # Setup network after containers are up
    sudo $0 down             # Stop everything

Note: This script requires root privileges for network operations.

EOF
}

# Check if running as root
if [ "$EUID" -ne 0 ] && [ "$1" != "help" ] && [ "$1" != "--help" ] && [ "$1" != "-h" ]; then
    echo -e "${RED}Error: This script must be run as root${NC}"
    echo "Please run: sudo $0 $*"
    exit 1
fi

case "${1:-test}" in
    up)
        start_docker
        ;;
    down)
        stop_docker
        ;;
    setup-veth)
        setup_veth
        ;;
    cleanup-veth)
        cleanup_veth
        ;;
    test)
        run_full_test
        ;;
    interactive)
        run_interactive
        ;;
    help|--help|-h)
        show_help
        ;;
    *)
        echo -e "${RED}Unknown command: $1${NC}"
        show_help
        exit 1
        ;;
esac
