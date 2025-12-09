#!/bin/bash
# Metric History Cache Prototype Demo
#
# This script demonstrates the anomaly detection feature by:
# 1. Starting the agent with metric history enabled
# 2. Creating a CPU spike that will be detected as an anomaly
#
# Prerequisites:
# - Agent built: invoke agent.build --build-exclude=systemd,python --exclude-rtloader
# - This script run from repo root

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONFIG_FILE="$SCRIPT_DIR/metric-history-demo-config.yaml"
AGENT_BIN="$REPO_ROOT/bin/agent/agent"

echo "=== Metric History Cache Demo ==="
echo ""
echo "This demo shows anomaly detection in action."
echo "After ~2 minutes of stable metrics, we'll create a CPU spike."
echo "The agent should detect and log the anomaly."
echo ""

# Check agent is built
if [ ! -f "$AGENT_BIN" ]; then
    echo "ERROR: Agent not built. Run:"
    echo "  invoke agent.build --build-exclude=systemd,python --exclude-rtloader"
    exit 1
fi

echo "Starting agent with metric history cache enabled..."
echo "Press Ctrl+C to stop"
echo ""
echo "Watch for [ANOMALY DETECTED] in the output!"
echo "================================================"
echo ""

# Run agent
$AGENT_BIN run -c "$CONFIG_FILE" 2>&1 | while IFS= read -r line; do
    # Highlight anomaly lines
    if echo "$line" | grep -q "ANOMALY DETECTED"; then
        echo -e "\033[1;31m$line\033[0m"
    else
        echo "$line"
    fi
done
