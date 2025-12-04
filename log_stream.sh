#!/bin/bash
# Script to continuously send logs during agent restart testing
# This helps verify that logs are not lost during the restart process
# Supports multiple log files to test offset flushing

NUM_FILES="${1:-3}"           # Number of log files (default 3)
LOG_DIR="${2:-/tmp}"           # Directory for log files (default /tmp)
DURATION="${3:-60}"            # Duration in seconds (default 60 to cover restart)
INTERVAL="${4:-0.1}"           # Interval between logs in seconds (default 0.1 for fast writes)

# Generate log file paths
LOG_FILES=()
for i in $(seq 1 $NUM_FILES); do
    LOG_FILES+=("$LOG_DIR/test_log_stream_$i.log")
done

# Clean up any existing log files
for log_file in "${LOG_FILES[@]}"; do
    > "$log_file"
done

echo "Starting log stream to $NUM_FILES files"
echo "Log files: ${LOG_FILES[*]}"
echo "Duration: ${DURATION}s, Interval: ${INTERVAL}s"
echo "Press Ctrl+C to stop early"
echo ""

echo "First, sleeping 5s to allow agent to start up"
sleep "5"

# Function to generate a log line with timestamp, sequence number, and file ID
generate_log() {
    local seq=$1
    local file_id=$2
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%S.000Z")
    echo "[$timestamp] [FILE:$file_id] [SEQ:$seq] [KOALA] Test log message during restart - sequence number $seq from file $file_id"
}

# Start time
start_time=$(date +%s)
end_time=$((start_time + DURATION))
sequence=1
file_index=0

# Write logs continuously, rotating through files
while [ $(date +%s) -lt $end_time ]; do
    # Rotate through files
    current_file="${LOG_FILES[$file_index]}"
    file_id=$((file_index + 1))
    
    generate_log $sequence $file_id >> "$current_file"
    
    # Print status every 10 logs to avoid spam
    if [ $((sequence % 10)) -eq 0 ]; then
        echo "Written log #$sequence to $current_file (file $file_id)"
    fi
    
    sequence=$((sequence + 1))
    file_index=$(((file_index + 1) % NUM_FILES))
    
    sleep "$INTERVAL"
done

echo ""
echo "Log stream completed. Total logs written: $((sequence - 1))"
echo "Log files:"
for log_file in "${LOG_FILES[@]}"; do
    if [ -f "$log_file" ]; then
        line_count=$(wc -l < "$log_file" 2>/dev/null || echo "0")
        echo "  $log_file: $line_count lines"
    fi
done

