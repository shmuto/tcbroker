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

    echo -e "${BLUE}Step 3: Applying mirroring rules${NC}"
    $TCBROKER_BIN apply "$CONFIG_FILE"
    echo -e "${GREEN}✓ Rules applied${NC}"
    echo

    echo -e "${BLUE}Step 4: Verifying idempotent re-apply${NC}"
    $TCBROKER_BIN apply "$CONFIG_FILE"
    echo -e "${GREEN}✓ Idempotent apply verified${NC}"
    echo

    echo -e "${BLUE}Step 5: Checking status (without config file)${NC}"
    $TCBROKER_BIN status
    echo

    echo -e "${BLUE}Step 6: Checking single-rule detail and JSON status${NC}"
    $TCBROKER_BIN status client-to-mirror
    $TCBROKER_BIN status --json > /dev/null
    echo -e "${GREEN}✓ Status views verified${NC}"
    echo

    echo -e "${BLUE}Step 7: Deleting a single rule${NC}"
    $TCBROKER_BIN delete client-to-mirror
    $TCBROKER_BIN status
    echo -e "${GREEN}✓ Single rule deleted${NC}"
    echo

    echo -e "${BLUE}Step 8: Stopping all remaining rules (without config file)${NC}"
    $TCBROKER_BIN stop
    $TCBROKER_BIN status
    echo -e "${GREEN}✓ All rules removed${NC}"
    echo

    echo -e "${GREEN}=== Test completed ===${NC}"
}

run_interactive() {
    echo -e "${YELLOW}=== Interactive tcbroker Testing ===${NC}"
    echo

    echo -e "${BLUE}Applying mirroring rules...${NC}"
    $TCBROKER_BIN apply "$CONFIG_FILE"
    echo

    echo -e "${BLUE}Current status:${NC}"
    $TCBROKER_BIN status
    echo

    echo -e "${GREEN}=== Ready for testing ===${NC}"
    echo
    echo "Available commands:"
    echo "  1. Live status monitoring:"
    echo "     $TCBROKER_BIN status --watch"
    echo
    echo "  2. Inspect a single rule:"
    echo "     $TCBROKER_BIN status client-to-mirror"
    echo
    echo "  3. Delete a single rule:"
    echo "     $TCBROKER_BIN delete client-to-mirror"
    echo
    echo "  4. Stop all managed rules:"
    echo "     $TCBROKER_BIN stop"
    echo
    echo "  5. Exit interactive mode:"
    echo "     exit"
    echo

    exec /bin/bash
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
    help        Show this help message

Examples:
    $0 test
    $0 interactive

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
    help|--help|-h)
        show_help
        ;;
    *)
        echo -e "${RED}Unknown command: $1${NC}"
        show_help
        exit 1
        ;;
esac
