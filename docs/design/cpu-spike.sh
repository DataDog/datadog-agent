#!/bin/bash
# Simple CPU spike generator
# Run this in another terminal while the agent is running

echo "Generating CPU spike for 30 seconds..."
echo "This should trigger system.cpu.* anomaly detection"

# Use a busy loop to spike CPU
timeout 30 bash -c 'while true; do :; done' &
SPIKE_PID=$!

echo "Spike process PID: $SPIKE_PID"
echo "Wait ~1-2 minutes after spike ends to see anomaly in agent logs"

wait $SPIKE_PID 2>/dev/null
echo "CPU spike ended"
