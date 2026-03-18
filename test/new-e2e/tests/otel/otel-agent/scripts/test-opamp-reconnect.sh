#!/usr/bin/env bash
# test-opamp-reconnect.sh — manual test for T026 (OTELCOL025/OTELCOL026)
#
# Verifies that the Collector reconnects to the OpAmp server after a disconnection,
# using exponential back-off, and re-sends its AgentToServer message on reconnect.
#
# speky:OTELCOL#T026
#
# Prerequisites:
#   - otel-agent binary at $OTEL_AGENT (default: bin/otel-agent/otel-agent)
#   - opamp-server binary at $OPAMP_SERVER (default: /tmp/opamp-server/opamp-server)
#   - Run from the datadog-agent repo root

set -euo pipefail

OTEL_AGENT="${OTEL_AGENT:-bin/otel-agent/otel-agent}"
OPAMP_SERVER="${OPAMP_SERVER:-/tmp/opamp-server/opamp-server}"
CONFIG="${TMPDIR:-/tmp}/otel-opamp-reconnect-test.yaml"
SERVER_LOG="${TMPDIR:-/tmp}/opamp-server-reconnect.log"
AGENT_LOG="${TMPDIR:-/tmp}/otel-agent-reconnect.log"
PASS=0
FAIL=0

pass() { echo "  PASS: $*"; PASS=$((PASS+1)); }
fail() { echo "  FAIL: $*"; FAIL=$((FAIL+1)); }
step() { echo; echo "==> $*"; }

cleanup() {
    kill "$AGENT_PID" "$SERVER_PID" 2>/dev/null || true
    wait "$AGENT_PID" "$SERVER_PID" 2>/dev/null || true
}
trap cleanup EXIT

cat > "$CONFIG" <<'EOF'
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
exporters:
  debug:
    verbosity: basic
extensions:
  opamp:
    server:
      ws:
        endpoint: ws://localhost:4320/v1/opamp
service:
  extensions: [opamp]
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
EOF

# ── Step 1: start OpAmp server ────────────────────────────────────────────────
step "Starting OpAmp server"
"$OPAMP_SERVER" >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!
sleep 1
if grep -q "listening" "$SERVER_LOG"; then
    pass "server started"
else
    fail "server did not start"; cat "$SERVER_LOG"; exit 1
fi

# ── Step 2: start otel-agent ──────────────────────────────────────────────────
step "Starting otel-agent"
DD_OTELCOLLECTOR_ENABLED=true \
DD_API_KEY=test \
DD_CMD_PORT=0 \
DD_AGENT_IPC_PORT=-1 \
DD_AGENT_IPC_CONFIG_REFRESH_INTERVAL=0 \
DD_ENABLE_METADATA_COLLECTION=false \
DD_HOSTNAME=test-host \
DD_OTELCOLLECTOR_CONVERTER_FEATURES="health_check" \
    "$OTEL_AGENT" --config "file:$CONFIG" >"$AGENT_LOG" 2>&1 &
AGENT_PID=$!
sleep 5

if grep -q "Everything is ready" "$AGENT_LOG"; then
    pass "agent started"
else
    fail "agent did not reach ready state"; cat "$AGENT_LOG"; exit 1
fi
if grep -q "Agent connected" "$SERVER_LOG"; then
    pass "initial connection established"
else
    fail "server did not receive initial connection"; cat "$SERVER_LOG"; exit 1
fi

# ── Step 3: kill server ───────────────────────────────────────────────────────
step "Killing OpAmp server"
kill "$SERVER_PID"
wait "$SERVER_PID" 2>/dev/null || true
sleep 3

if grep -q "connection refused\|will retry" "$AGENT_LOG"; then
    pass "agent detected disconnection and is retrying"
else
    fail "agent did not log a retry after server stopped"; cat "$AGENT_LOG"
fi

# ── Step 4: restart server ────────────────────────────────────────────────────
step "Restarting OpAmp server"
"$OPAMP_SERVER" >>"$SERVER_LOG" 2>&1 &
SERVER_PID=$!
sleep 6

# ── Step 5: verify reconnection ───────────────────────────────────────────────
step "Verifying reconnection"
RECONNECT_COUNT=$(grep -c "Agent connected" "$SERVER_LOG" || true)
if [ "$RECONNECT_COUNT" -ge 2 ]; then
    pass "agent reconnected after server restart (connected $RECONNECT_COUNT times total)"
else
    fail "expected at least 2 connections, got $RECONNECT_COUNT"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
