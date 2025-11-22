#!/usr/bin/env bash
# Container-side test script
# This script runs inside the tcbroker container
set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

CONFIG_FILE="/workspace/tests/config.yaml"
TCBROKER_BIN="/workspace/tcbroker"

#==============================================================================
# Test Functions
#==============================================================================

run_automated_test() {
    echo -e "${YELLOW}=== tcbroker Automated Test ===${NC}"
    echo

    echo -e "${BLUE}Step 1: Validating configuration syntax${NC}"
    $TCBROKER_BIN validate "$CONFIG_FILE"
    echo -e "${GREEN}✓ Configuration syntax valid${NC}"
    echo

    echo -e "${BLUE}Step 2: Validating with interface checks${NC}"
    $TCBROKER_BIN validate "$CONFIG_FILE" --check-interfaces
    echo -e "${GREEN}✓ All interfaces exist${NC}"
    echo

    echo -e "${BLUE}Step 3: Starting packet mirroring/redirect${NC}"
    $TCBROKER_BIN start "$CONFIG_FILE"
    echo -e "${GREEN}✓ Rules applied${NC}"
    echo

    echo -e "${BLUE}Step 4: Checking status${NC}"
    $TCBROKER_BIN status "$CONFIG_FILE"
    echo

    echo -e "${BLUE}Step 5: Viewing statistics${NC}"
    $TCBROKER_BIN status "$CONFIG_FILE" --stats
    echo

    echo -e "${BLUE}Step 6: Stopping${NC}"
    $TCBROKER_BIN stop "$CONFIG_FILE"
    echo -e "${GREEN}✓ Rules removed${NC}"
    echo

    echo -e "${GREEN}=== Test completed ===${NC}"
}

run_interactive() {
    echo -e "${YELLOW}=== Interactive tcbroker Testing ===${NC}"
    echo

    echo -e "${BLUE}Starting packet mirroring/redirect...${NC}"
    $TCBROKER_BIN start "$CONFIG_FILE"
    echo

    echo -e "${BLUE}Current status:${NC}"
    $TCBROKER_BIN status "$CONFIG_FILE"
    echo

    echo -e "${GREEN}=== Ready for testing ===${NC}"
    echo
    echo "Available commands:"
    echo "  1. View status with stats:"
    echo "     $TCBROKER_BIN status $CONFIG_FILE --stats"
    echo
    echo "  2. Check tc rules on specific interface:"
    echo "     tc filter show dev broker2client ingress"
    echo
    echo "  3. Stop mirroring:"
    echo "     $TCBROKER_BIN stop $CONFIG_FILE"
    echo
    echo "  4. Exit interactive mode:"
    echo "     exit"
    echo

    exec /bin/bash
}

verify_checksum() {
    echo -e "${BLUE}Verifying checksum recalculation...${NC}"

    # Get container PIDs (from host perspective)
    CLIENT_PID=$(docker inspect -f '{{.State.Pid}}' tcbroker-client 2>/dev/null || echo "")
    SERVER_PID=$(docker inspect -f '{{.State.Pid}}' tcbroker-server 2>/dev/null || echo "")

    if [ -z "$CLIENT_PID" ] || [ -z "$SERVER_PID" ]; then
        echo -e "${RED}Error: Cannot get container PIDs. This test should be run from host.${NC}"
        exit 1
    fi

    $TCBROKER_BIN start "$CONFIG_FILE"

    echo "Starting packet capture on server2broker..."
    timeout 5 nsenter -t $SERVER_PID -n tcpdump -i server2broker -nn -v icmp > /tmp/capture.txt 2>&1 &
    TCPDUMP_PID=$!
    sleep 1

    echo "Sending ICMP packet..."
    nsenter -t $CLIENT_PID -n ping -c 1 -I client2broker 192.168.20.2 || true

    sleep 2
    kill $TCPDUMP_PID 2>/dev/null || true

    if grep -q "cksum" /tmp/capture.txt; then
        echo -e "${GREEN}✓ Checksum information found in capture${NC}"
        grep "cksum" /tmp/capture.txt || true
    else
        echo -e "${YELLOW}! No checksum info in capture${NC}"
    fi

    $TCBROKER_BIN stop "$CONFIG_FILE"
    rm -f /tmp/capture.txt
}

#==============================================================================
# Main
#==============================================================================

show_help() {
    cat <<EOF
Usage: $0 [COMMAND]

Container-side test execution script.
Note: This script expects veth interfaces to be already set up by setup.sh

Commands:
    test        Run automated test suite (default)
    interactive Start interactive testing mode
    checksum    Verify checksum recalculation (requires nsenter)
    help        Show this help message

Examples:
    $0 test
    $0 interactive
    $0 checksum

Note: Run 'tests/setup.sh' from the host to setup the test environment first.

EOF
}

case "${1:-test}" in
    test)
        run_automated_test
        ;;
    interactive)
        run_interactive
        ;;
    checksum)
        verify_checksum
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
