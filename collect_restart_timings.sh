#!/bin/bash
# Script to run the agent multiple times and collect restart timing data
# Usage: ./collect_restart_timings.sh [num_runs] [config_file] [output_file]

NUM_RUNS="${1:-10}"           # Number of runs (default 10)
CONFIG_FILE="${2:-/workspaces/datadog-agent/dev/dist/datadog.yaml}"
OUTPUT_FILE="${3:-restart_timings.txt}"
AGENT_BIN="${4:-./bin/agent/agent}"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo "=========================================="
echo "Collecting restart timing data"
echo "=========================================="
echo "Runs: $NUM_RUNS"
echo "Config: $CONFIG_FILE"
echo "Output: $OUTPUT_FILE"
echo ""

# Clear output file
> "$OUTPUT_FILE"

# Function to extract timing data from logs
extract_timings() {
    local run_num=$1
    local start_time=$2
    local log_file="/var/log/datadog/agent.log"
    
    echo "" >> "$OUTPUT_FILE"
    echo "=== Run #$run_num (started at $start_time) ===" >> "$OUTPUT_FILE"
    
    # Since we clear the log file before each run, all logs in the file are from this run
    # Extract comparison data
    if grep -q "\[COMPARISON\]" "$log_file" 2>/dev/null; then
        grep "\[COMPARISON\]" "$log_file" >> "$OUTPUT_FILE" 2>/dev/null
    else
        echo "No comparison data found" >> "$OUTPUT_FILE"
    fi
}

# Function to wait for agent to complete restart
wait_for_restart() {
    local timeout=60  # seconds
    local elapsed=0
    local check_interval=1
    
    while [ $elapsed -lt $timeout ]; do
        if grep -q "Successfully restarted pipeline with HTTP" /var/log/datadog/agent.log 2>/dev/null; then
            # Wait a bit more for comparison logs
            sleep 2
            return 0
        fi
        sleep $check_interval
        elapsed=$((elapsed + check_interval))
    done
    
    echo -e "${YELLOW}Warning: Timeout waiting for restart completion${NC}"
    return 1
}

# Check if agent binary exists
if [ ! -f "$AGENT_BIN" ]; then
    echo -e "${RED}Error: Agent binary not found at $AGENT_BIN${NC}"
    exit 1
fi

# Check if config file exists
if [ ! -f "$CONFIG_FILE" ]; then
    echo -e "${RED}Error: Config file not found at $CONFIG_FILE${NC}"
    exit 1
fi

successful_runs=0
failed_runs=0

for i in $(seq 1 $NUM_RUNS); do
    echo -e "${GREEN}[$i/$NUM_RUNS] Starting agent run...${NC}"
    
    # Record start time for this run
    RUN_START_TIME=$(date -u +"%Y-%m-%d %H:%M:%S")
    
    # Clear previous agent log to ensure clean extraction
    > /var/log/datadog/agent.log
    
    # Start agent in background
    DD_TEST_FORCE_TCP_AND_RESTART=1 "$AGENT_BIN" run -c "$CONFIG_FILE" > /dev/null 2>&1 &
    AGENT_PID=$!
    
    # Wait for restart to complete
    if wait_for_restart; then
        echo -e "${GREEN}[$i/$NUM_RUNS] Restart completed, extracting data...${NC}"
        extract_timings $i "$RUN_START_TIME"
        successful_runs=$((successful_runs + 1))
    else
        echo -e "${RED}[$i/$NUM_RUNS] Failed to complete restart${NC}"
        # Still try to extract what we can
        extract_timings $i "$RUN_START_TIME"
        failed_runs=$((failed_runs + 1))
    fi
    
    # Stop the agent
    kill $AGENT_PID 2>/dev/null
    wait $AGENT_PID 2>/dev/null
    
    # Wait a bit between runs
    if [ $i -lt $NUM_RUNS ]; then
        echo "Waiting 3 seconds before next run..."
        sleep 3
    fi
done

echo ""
echo "=========================================="
echo "Collection complete!"
echo "=========================================="
echo -e "${GREEN}Successful runs: $successful_runs${NC}"
echo -e "${RED}Failed runs: $failed_runs${NC}"
echo "Data saved to: $OUTPUT_FILE"
echo ""
echo "To view comparison data:"
echo "  grep '\[COMPARISON\]' $OUTPUT_FILE"
echo ""
echo "To view all timing data:"
echo "  cat $OUTPUT_FILE"

