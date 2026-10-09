#!/usr/bin/env bash
# Python custom-check distribution-sketch memory reproducer.
#
# Runs the histogram_lab check (checks.d/histogram_lab.py) which calls
# self.submit_histogram_bucket with a huge non-monotonic count (1.42e12),
# or a monotonic baseline+jump when --monotonic is set.
#
# Usage:
#   ./run_check_repro.sh [--monotonic] [CHECK_TIMES]
#
# Defaults: CHECK_TIMES=3
#
# Two modes:
#   1. One-shot (default): agent check ... --json --full-sketches
#      - nil serializer, no periodic flush, no expvar/heap
#      - fastest; shows sketch bins in stdout
#   2. Daemon: set DAEMON=1 env var
#      - full agent run with expvar/telemetry/heap capture
#
# Prerequisites:
#   - Built agent with Python/rtloader: dda inv agent.build --build-exclude=systemd
#   - datadog_checks.base available in the agent's embedded Python
set -euo pipefail

MONOTONIC=false
CHECK_TIMES=3
if [ "${1:-}" = "--monotonic" ]; then
  MONOTONIC=true
  shift
fi
CHECK_TIMES="${1:-$CHECK_TIMES}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LAB="$SCRIPT_DIR"
AGENT="${AGENT:-./bin/agent/agent}"
EXPVAR_PORT="${EXPVAR_PORT:-15000}"
ART="$LAB/artifacts"
mkdir -p "$ART"

CFG="$LAB/datadog.yaml"
if [ ! -f "$CFG" ]; then
  echo "Generating $CFG from template..."
  sed "s|__LAB__|$LAB|g" "$LAB/datadog.yaml.template" > "$CFG"
fi

# Flip the monotonic_jump flag in the check config.
CONF="$LAB/conf.d/histogram_lab.d/conf.yaml"
sed -i.bak "s/monotonic_jump: .*/monotonic_jump: $MONOTONIC/" "$CONF" && rm -f "$CONF.bak"

if [ "${DAEMON:-0}" = "1" ]; then
  echo "=== Daemon mode: check_times=$CHECK_TIMES monotonic=$MONOTONIC ==="
  env -i HOME="$HOME" PATH="$PATH" "$AGENT" -c "$CFG" run >"$ART/check_agent.log" 2>&1 &
  AGENT_PID=$!
  trap 'kill $AGENT_PID 2>/dev/null || true' EXIT

  echo "Waiting for expvar on :$EXPVAR_PORT..."
  for _ in $(seq 1 30); do
    if curl -sf "http://127.0.0.1:$EXPVAR_PORT/debug/vars" >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done

  BEFORE_DROP=$(curl -sf "http://127.0.0.1:$EXPVAR_PORT/debug/vars" 2>/dev/null \
    | jq -r '.sketch_series.ItemTooBig // 0' || echo 0)
  echo "Baseline sketch_too_big: $BEFORE_DROP"

  # Let the check run a few collection intervals.
  echo "Waiting $((CHECK_TIMES * 2))s for collections..."
  sleep $((CHECK_TIMES * 2))

  curl -sf "http://127.0.0.1:$EXPVAR_PORT/debug/pprof/heap" >"$ART/check_heap_after.pb.gz" || true
  AFTER_DROP=$(curl -sf "http://127.0.0.1:$EXPVAR_PORT/debug/vars" 2>/dev/null \
    | jq -r '.sketch_series.ItemTooBig // 0' || echo 0)
  echo "After sketch_too_big: $AFTER_DROP"
  echo "Dropped delta: $((AFTER_DROP - BEFORE_DROP))"
  echo "Agent log: $ART/check_agent.log"
  echo "Heap: $ART/check_heap_after.pb.gz"
else
  echo "=== One-shot mode: check_times=$CHECK_TIMES monotonic=$MONOTONIC ==="
  env -i HOME="$HOME" PATH="$PATH" "$AGENT" -c "$CFG" check histogram_lab \
    --check-times "$CHECK_TIMES" --pause 1000 --delay 500 \
    --json --full-sketches 2>&1 | tee "$ART/check_stdout.json"
  echo "Output: $ART/check_stdout.json"
  echo "Note: one-shot mode uses a nil serializer and does not reach the"
  echo "serializer drop path or expose expvar/heap. Use DAEMON=1 for those."
fi
