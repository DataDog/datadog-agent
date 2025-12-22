#!/bin/bash

echo "Starting baseline CPU load..."

# Function to create sustained spike CPU load (more intensive)
# Tighter loop with more iterations for sustained higher CPU
spike_load() {
    while true; do
        : $((result = 0))
        for ((i=0; i<500000; i++)); do
            : $((result = result + i * i * i))
        done
    done
}

# Wait for 3 minutes (180 seconds)
echo "Waiting 3 minutes before CPU spike..."
# sleep 180

echo "STARTING CPU SPIKE NOW - 2.5x increase!"

# Start spike load processes
for i in {1..10}; do
    spike_load &
    SPIKE_PIDS="$SPIKE_PIDS $!"
done

echo "CPU spike active with processes: $SPIKE_PIDS"
echo "Press Ctrl+C to stop..."

# Keep the script running
wait
