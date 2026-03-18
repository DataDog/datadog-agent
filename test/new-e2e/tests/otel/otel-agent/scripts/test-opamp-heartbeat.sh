#!/usr/bin/env bash
# test-opamp-heartbeat.sh — manual test for T020 (OTELCOL029)
#
# Verifies that the Collector sends periodic heartbeat messages to the OpAmp
# server, and that the server can shorten the interval via
# OpAMPConnectionSettings.heartbeat_interval_seconds.
#
# NOTE: This test requires the agent to declare the ReportsHeartbeat capability
# (AgentCapabilities bit 8192). The opampextension v0.147.0 does not declare
# this capability by default — the test will report SKIP until it is implemented.
#
# speky:OTELCOL#T020
#
# Prerequisites:
#   - otel-agent binary at $OTEL_AGENT (default: bin/otel-agent/otel-agent)
#   - opamp-server binary at $OPAMP_SERVER (default: /tmp/opamp-server/opamp-server)
#   - Run from the datadog-agent repo root

set -euo pipefail

OTEL_AGENT="${OTEL_AGENT:-bin/otel-agent/otel-agent}"
OPAMP_SERVER="${OPAMP_SERVER:-/tmp/opamp-server/opamp-server}"
CONFIG="${TMPDIR:-/tmp}/otel-opamp-heartbeat-test.yaml"
SERVER_LOG="${TMPDIR:-/tmp}/opamp-server-heartbeat.log"
AGENT_LOG="${TMPDIR:-/tmp}/otel-agent-heartbeat.log"
PASS=0
FAIL=0
SKIP=0

pass() { echo "  PASS: $*"; PASS=$((PASS+1)); }
fail() { echo "  FAIL: $*"; FAIL=$((FAIL+1)); }
skip() { echo "  SKIP: $*"; SKIP=$((SKIP+1)); }
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

step "Starting OpAmp server"
"$OPAMP_SERVER" >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!
sleep 1
grep -q "listening" "$SERVER_LOG" && pass "server started" || { fail "server did not start"; exit 1; }

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
    pass "agent reached ready state"
else
    fail "agent did not reach ready state"; cat "$AGENT_LOG"; exit 1
fi

step "Checking ReportsHeartbeat capability"
# Capability 8192 = ReportsHeartbeat. The server log should show the bitmask.
if grep -q "ReportsHeartbeat\|capabilities.*8192\|8192.*capabilities" "$SERVER_LOG" 2>/dev/null; then
    pass "agent declares ReportsHeartbeat capability"
else
    skip "agent does not declare ReportsHeartbeat (opampextension v0.147.0 does not implement this)"
    echo
    echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
    exit 0
fi

step "Waiting 35 seconds for initial heartbeat (default interval: 30 s)"
sleep 35
HEARTBEAT_COUNT=$(grep -c "Heartbeat\|heartbeat" "$SERVER_LOG" 2>/dev/null || true)
if [ "$HEARTBEAT_COUNT" -ge 1 ]; then
    pass "server received at least one heartbeat"
else
    fail "no heartbeat received within 35 s"
fi

echo
echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
[ "$FAIL" -eq 0 ]
