#!/bin/bash
# Metric History Cache Prototype Demo (Self-Contained)
#
# This script demonstrates anomaly detection end-to-end:
# 1. Builds the agent (if needed)
# 2. Starts the agent with metric history enabled
# 3. Waits for baseline metrics to accumulate
# 4. Generates a CPU spike
# 5. Waits for anomaly detection output
# 6. Stops the agent
#
# Run from repo root: ./docs/design/metric-history-demo.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONFIG_FILE="$SCRIPT_DIR/metric-history-demo-config.yaml"
AGENT_BIN="$REPO_ROOT/bin/agent/agent"
LOG_FILE="/tmp/metric-history-demo-$$.log"

# Timing configuration
BASELINE_WAIT=180      # 3 minutes for baseline (need 10+ points at 15s flush = 150s, plus buffer)
SPIKE_DURATION=30      # 30 second CPU spike
ANOMALY_TIMEOUT=120    # Wait up to 2 minutes for anomaly after spike
POST_ANOMALY_WAIT=10   # Wait 10 seconds after seeing anomaly

# Colors
RED='\033[1;31m'
GREEN='\033[1;32m'
YELLOW='\033[1;33m'
CYAN='\033[1;36m'
NC='\033[0m' # No Color

cleanup() {
    echo ""
    echo -e "${YELLOW}Cleaning up...${NC}"

    # Kill agent if running
    if [ -n "$AGENT_PID" ] && kill -0 "$AGENT_PID" 2>/dev/null; then
        echo "Stopping agent (PID $AGENT_PID)..."
        kill "$AGENT_PID" 2>/dev/null || true
        wait "$AGENT_PID" 2>/dev/null || true
    fi

    # Kill any leftover spike process
    if [ -n "$SPIKE_PID" ] && kill -0 "$SPIKE_PID" 2>/dev/null; then
        kill "$SPIKE_PID" 2>/dev/null || true
    fi

    # Kill the log tailer if running
    if [ -n "$TAIL_PID" ] && kill -0 "$TAIL_PID" 2>/dev/null; then
        kill "$TAIL_PID" 2>/dev/null || true
    fi

    echo -e "${GREEN}Demo complete. Log saved to: $LOG_FILE${NC}"
}

trap cleanup EXIT

echo -e "${CYAN}=== Metric History Anomaly Detection Demo ===${NC}"
echo ""
echo "This demo runs end-to-end automatically:"
echo "  1. Build agent (if needed)"
echo "  2. Start agent with metric_history enabled"
echo "  3. Wait ${BASELINE_WAIT}s for baseline metrics"
echo "  4. Generate ${SPIKE_DURATION}s CPU spike"
echo "  5. Wait for anomaly detection (up to ${ANOMALY_TIMEOUT}s)"
echo "  6. Stop agent ${POST_ANOMALY_WAIT}s after anomaly seen"
echo ""

# Step 1: Build agent if needed or if source is newer
NEEDS_BUILD=0
if [ ! -f "$AGENT_BIN" ]; then
    NEEDS_BUILD=1
    echo -e "${YELLOW}Agent binary not found, building...${NC}"
else
    # Check if any relevant source files are newer than the binary
    # Focus on metric_history and detector files
    NEWER_FILES=$(find "$REPO_ROOT/pkg/aggregator/metric_history" "$REPO_ROOT/pkg/aggregator/demultiplexer_agent.go" -newer "$AGENT_BIN" 2>/dev/null | head -5)
    if [ -n "$NEWER_FILES" ]; then
        NEEDS_BUILD=1
        echo -e "${YELLOW}Source files changed since last build:${NC}"
        echo "$NEWER_FILES" | while read f; do echo "  $(basename $f)"; done
        echo -e "${YELLOW}Rebuilding agent...${NC}"
    fi
fi

if [ $NEEDS_BUILD -eq 1 ]; then
    cd "$REPO_ROOT"
    invoke agent.build --build-exclude=systemd,python --exclude-rtloader
    echo -e "${GREEN}Agent built successfully${NC}"
else
    echo -e "${GREEN}Agent already built and up to date: $AGENT_BIN${NC}"
fi

# Step 2: Start agent in background
echo ""
echo -e "${CYAN}Starting agent...${NC}"
$AGENT_BIN run -c "$CONFIG_FILE" > "$LOG_FILE" 2>&1 &
AGENT_PID=$!

# Verify agent started
sleep 2
if ! kill -0 "$AGENT_PID" 2>/dev/null; then
    echo -e "${RED}ERROR: Agent failed to start. Check $LOG_FILE${NC}"
    exit 1
fi
echo -e "${GREEN}Agent running (PID $AGENT_PID)${NC}"

# Start background log tailer for anomalies (with highlighting)
tail -f "$LOG_FILE" 2>/dev/null | while IFS= read -r line; do
    if echo "$line" | grep -q "ANOMALY DETECTED"; then
        echo -e "${RED}$line${NC}"
    fi
done &
TAIL_PID=$!

# Step 3: Wait for baseline
echo ""
echo -e "${CYAN}Waiting ${BASELINE_WAIT}s for baseline metrics to accumulate...${NC}"
echo "(Need ~16 data points at 15s flush interval for sliding window)"
for i in $(seq $BASELINE_WAIT -30 1); do
    echo -ne "\r  ${i}s remaining...  "
    sleep 30
done
echo -e "\r  ${GREEN}Baseline complete${NC}                    "

# Step 4: Generate CPU spike
echo ""
echo -e "${CYAN}Generating CPU spike for ${SPIKE_DURATION}s...${NC}"
timeout $SPIKE_DURATION bash -c 'while true; do :; done' &
SPIKE_PID=$!
echo "Spike process PID: $SPIKE_PID"
wait $SPIKE_PID 2>/dev/null || true
SPIKE_PID=""
echo -e "${GREEN}CPU spike ended${NC}"

# Step 5: Wait for anomaly detection
echo ""
echo -e "${CYAN}Waiting for anomaly detection (timeout: ${ANOMALY_TIMEOUT}s)...${NC}"
ANOMALY_FOUND=0
WAIT_START=$(date +%s)

while true; do
    NOW=$(date +%s)
    ELAPSED=$((NOW - WAIT_START))

    if [ $ELAPSED -ge $ANOMALY_TIMEOUT ]; then
        echo -e "${YELLOW}Timeout waiting for anomaly. Check $LOG_FILE for details.${NC}"
        break
    fi

    # Check for anomaly in log (looking for any anomalies - bayesian or zscore)
    if grep -q "ANOMALY DETECTED" "$LOG_FILE" 2>/dev/null; then
        echo ""
        echo -e "${GREEN}Anomaly detected!${NC}"
        echo ""
        echo "=== Detected Anomalies ==="
        grep "ANOMALY DETECTED" "$LOG_FILE" | tail -10
        echo "=========================="
        ANOMALY_FOUND=1
        break
    fi

    echo -ne "\r  Waiting... ${ELAPSED}s / ${ANOMALY_TIMEOUT}s  "
    sleep 5
done

# Step 6: Wait a bit after anomaly, then exit
if [ $ANOMALY_FOUND -eq 1 ]; then
    echo ""
    echo -e "${CYAN}Waiting ${POST_ANOMALY_WAIT}s before stopping...${NC}"
    sleep $POST_ANOMALY_WAIT
fi

echo ""
echo -e "${GREEN}=== Demo Complete ===${NC}"
echo ""
echo "Results:"
ANOMALY_COUNT=$(grep -c "ANOMALY DETECTED" "$LOG_FILE" 2>/dev/null || echo "0")
echo "  Total anomalies detected: $ANOMALY_COUNT"
echo "  Log file: $LOG_FILE"
echo ""

# Show summary of what was detected
if [ "$ANOMALY_COUNT" -gt 0 ]; then
    echo "Anomaly summary:"
    grep "ANOMALY DETECTED" "$LOG_FILE" | sed 's/.*\[ANOMALY DETECTED\] /  /' | cut -d: -f1-2 | sort | uniq -c | sort -rn
fi
