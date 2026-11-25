#!/usr/bin/env bash
set -u

control_file=/tmp/anomalydetection-stable-e2e-control
trace_file=/tmp/anomalydetection-stable-e2e.trace
phase=baseline
wave_index=0
plateau_index=0

rm -f "$control_file" "$trace_file"
while true; do
  if [[ "$phase" == baseline && -f "$control_file" ]]; then
    phase=wave
    plateau_index=0
    echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) switch=wave" >> "$trace_file"
  fi

  if [[ "$phase" == baseline ]]; then
    small_value=100
    large_value=100
  else
    if (( plateau_index / 60 % 2 == 0 )); then
      small_value=104
      large_value=140
    else
      small_value=100
      large_value=100
    fi
  fi

  payload=$(printf '%s\n%s' \
    "e2e.anomalydetection.stable.small:${small_value}|g|@1" \
    "e2e.anomalydetection.stable.large:${large_value}|g|@1")
  printf '%s' "$payload" > /dev/udp/127.0.0.1/8125
  echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) phase=${phase} index=${wave_index} plateau=${plateau_index} small=${small_value} large=${large_value}" >> "$trace_file"

  wave_index=$((wave_index + 1))
  plateau_index=$((plateau_index + 1))
  sleep 1
done
