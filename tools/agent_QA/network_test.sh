#!/bin/bash
# Network Test - Simulate network issues to test body read failures

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Network Simulation Test for Profile Upload Body Read Failures${NC}"
echo

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   echo -e "${RED}This script must be run as root for network simulation${NC}"
   echo "Usage: sudo ./network_test.sh [test_type]"
   echo "Test types: delay, loss, corrupt, bandwidth"
   exit 1
fi

TEST_TYPE=${1:-delay}
INTERFACE="lo"  # Use loopback interface for local testing

cleanup() {
    echo -e "\n${YELLOW}Cleaning up network rules...${NC}"
    tc qdisc del dev $INTERFACE root 2>/dev/null || true
    echo -e "${GREEN}Cleanup complete${NC}"
}

# Set trap to cleanup on exit
trap cleanup EXIT

case $TEST_TYPE in
    "delay")
        echo -e "${YELLOW}Adding 3-second delay to loopback interface${NC}"
        tc qdisc add dev $INTERFACE root netem delay 3000ms
        echo "This will make body reads take longer and potentially timeout"
        ;;
    "loss")
        echo -e "${YELLOW}Adding 20% packet loss to loopback interface${NC}"
        tc qdisc add dev $INTERFACE root netem loss 20%
        echo "This will cause packet retransmissions and slow body reads"
        ;;
    "corrupt")
        echo -e "${YELLOW}Adding 5% packet corruption to loopback interface${NC}"
        tc qdisc add dev $INTERFACE root netem corrupt 5%
        echo "This will cause packet retransmissions and potential read errors"
        ;;
    "bandwidth")
        echo -e "${YELLOW}Limiting bandwidth to 10kbps on loopback interface${NC}"
        tc qdisc add dev $INTERFACE root tbf rate 10kbit burst 1540 limit 1540
        echo "This will make large body uploads very slow"
        ;;
    *)
        echo -e "${RED}Unknown test type: $TEST_TYPE${NC}"
        echo "Valid types: delay, loss, corrupt, bandwidth"
        exit 1
        ;;
esac

echo -e "\n${GREEN}Network simulation active!${NC}"
echo "Current network rules:"
tc qdisc show dev $INTERFACE

echo -e "\n${YELLOW}Now run your profile upload test in another terminal:${NC}"
echo "  ./huge_body_test.py 50    # 50MB upload"
echo "  ./slow_stream_test.py 1.0 # 1 second per chunk"
echo "  ddprof -l notice -S test-service SomeApp 10"
echo

echo -e "${YELLOW}Press Enter to remove network simulation and exit...${NC}"
read -r 