#!/bin/bash
# Run the misattribution reproduction test
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Kill any existing servers
echo "Stopping any existing servers..."
pkill -f "./server -port" 2>/dev/null || true
sleep 1

# Start servers
echo "Starting servers..."
./server -port 8443 &
./server -port 9443 &
sleep 2

# Run stress client
echo "Running stress test..."
./stress_client \
    -server1 "localhost:8443" \
    -server2 "localhost:9443" \
    -duration 15s \
    -concurrency 10 \
    -skip-close 0.1 \
    -rate 100ms

echo "Test complete!"