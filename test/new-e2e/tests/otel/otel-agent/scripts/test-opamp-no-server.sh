#!/usr/bin/env bash
# test-opamp-no-server.sh — manual test for T025 (OTELCOL025/OTELCOL026)
#
# Verifies that an unreachable OpAmp server does not prevent the Collector from
# starting: pipelines must reach the ready state and the gRPC port must be open.
#
# speky:OTELCOL#T025
#
# Prerequisites:
#   - otel-agent binary at $OTEL_AGENT (default: bin/otel-agent/otel-agent)
#   - Nothing listening on port 4320 (OpAmp server intentionally absent)
#   - Run from the datadog-agent repo root

set -euo pipefail

OTEL_AGENT="${OTEL_AGENT:-bin/otel-agent/otel-agent}"
CONFIG="${TMPDIR:-/tmp}/otel-opamp-no-server-test.yaml"
AGENT_LOG="${TMPDIR:-/tmp}/otel-agent-no-server.log"
PASS=0
FAIL=0

pass() { echo "  PASS: $*"; PASS=$((PASS+1)); }
fail() { echo "  FAIL: $*"; FAIL=$((FAIL+1)); }
step() { echo; echo "==> $*"; }

cleanup() { kill "$AGENT_PID" 2>/dev/null || true; wait "$AGENT_PID" 2>/dev/null || true; }
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

step "Confirming no OpAmp server is running on :4320"
if nc -z localhost 4320 2>/dev/null; then
    fail "something is already listening on port 4320 — stop it before running this test"
    exit 1
fi
pass "port 4320 is free"

step "Starting otel-agent (OpAmp server intentionally absent)"
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
sleep 6

step "Verifying agent reached ready state despite connection failure"
if grep -q "Everything is ready" "$AGENT_LOG"; then
    pass "agent reached ready state"
else
    fail "agent did not reach ready state"; cat "$AGENT_LOG"; exit 1
fi
if grep -q "connection refused\|will retry" "$AGENT_LOG"; then
    pass "agent logged connection failure and is retrying in background"
else
    fail "no retry log found"; cat "$AGENT_LOG"
fi

step "Verifying gRPC port is open and accepting connections"
if nc -z localhost 4317 2>/dev/null; then
    pass "gRPC port 4317 is open"
else
    fail "gRPC port 4317 is not open"
fi

echo
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
