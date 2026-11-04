#!/usr/bin/env bash
# DogStatsD distribution-sketch memory reproducer.
#
# Sends N distribution samples at a tiny sample rate to a running isolated
# agent, then captures peak RSS, heap profile, and sketch_too_big telemetry.
#
# Usage:
#   ./run_dogstatsd_repro.sh [RATE] [N] [DURATION_S]
#
# Defaults: RATE=1e-9, N=100, DURATION_S=15
#
# Prerequisites:
#   - Built agent: dda inv agent.build --build-exclude=systemd
#   - nc (netcat) and jq installed
#   - Copy datadog.yaml.template to datadog.yaml, substitute __LAB__
set -euo pipefail

RATE="${1:-1e-9}"
N="${2:-100}"
DURATION_S="${3:-15}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LAB="$SCRIPT_DIR"
AGENT="${AGENT:-./bin/agent/agent}"
EXPVAR_PORT="${EXPVAR_PORT:-15000}"
DSD_PORT="${DSD_PORT:-8125}"
ART="$LAB/artifacts"
mkdir -p "$ART"

CFG="$LAB/datadog.yaml"
if [ ! -f "$CFG" ]; then
  echo "Generating $CFG from template..."
  sed "s|__LAB__|$LAB|g" "$LAB/datadog.yaml.template" > "$CFG"
fi

echo "=== DogStatsD repro: rate=$RATE N=$N duration=${DURATION_S}s ==="

# Start agent under /usr/bin/time for peak RSS.
TIME_CMD=(/usr/bin/time)
if uname | grep -qi darwin; then
  TIME_CMD+=(-l)
else
  TIME_CMD+=(-v)
fi

# Clean environment to avoid inherited DD_* overrides.
env -i HOME="$HOME" PATH="$PATH" "${TIME_CMD[@]}" \
  "$AGENT" -c "$CFG" run >"$ART/dsd_agent.log" 2>&1 &
AGENT_PID=$!
trap 'kill $AGENT_PID 2>/dev/null || true' EXIT

# Wait for expvar to come up.
echo "Waiting for expvar on :$EXPVAR_PORT..."
for _ in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:$EXPVAR_PORT/debug/vars" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

# Baseline heap + telemetry.
curl -sf "http://127.0.0.1:$EXPVAR_PORT/debug/pprof/heap" >"$ART/dsd_heap_before.pb.gz" || true
BEFORE_DROP=$(curl -sf "http://127.0.0.1:$EXPVAR_PORT/debug/vars" 2>/dev/null \
  | jq -r '.sketch_series.ItemTooBig // 0' || echo 0)
echo "Baseline sketch_too_big: $BEFORE_DROP"

# Send N distribution samples at the given rate.
echo "Sending $N distribution samples at rate $RATE..."
for _ in $(seq 1 "$N"); do
  echo "repro.dist:1|d|@${RATE}" | nc -u -w1 127.0.0.1 "$DSD_PORT"
done

# Control: |h does NOT use sketches.
echo "repro.histo:1|h|@${RATE}" | nc -u -w1 127.0.0.1 "$DSD_PORT"

# Let the agent aggregate and flush.
echo "Waiting ${DURATION_S}s for aggregation/flush..."
sleep "$DURATION_S"

# Peak heap + telemetry after.
curl -sf "http://127.0.0.1:$EXPVAR_PORT/debug/pprof/heap" >"$ART/dsd_heap_after.pb.gz" || true
AFTER_DROP=$(curl -sf "http://127.0.0.1:$EXPVAR_PORT/debug/vars" 2>/dev/null \
  | jq -r '.sketch_series.ItemTooBig // 0' || echo 0)
echo "After sketch_too_big: $AFTER_DROP"
echo "Dropped delta: $((AFTER_DROP - BEFORE_DROP))"

# Peak RSS from /usr/bin/time output (macOS: -l prints max RSS in bytes;
# Linux: -v prints "Maximum resident set size" in kbytes).
echo "Agent log: $ART/dsd_agent.log"
echo "Heap profiles: $ART/dsd_heap_{before,after}.pb.gz"
echo "Inspect: go tool pprof -top $ART/dsd_heap_after.pb.gz"

# Leave agent running for manual inspection; kill on script exit via trap.
echo "Done. Agent PID $AGENT_PID will be killed on exit."
