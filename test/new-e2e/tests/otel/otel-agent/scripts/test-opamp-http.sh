#!/usr/bin/env bash
# test-opamp-http.sh — manual test for T017 (OTELCOL025/OTELCOL027)
#
# Verifies that the Collector can connect to the OpAmp server using the HTTP
# transport (POST to /v1/opamp), and that the server receives the initial
# AgentToServer message.
#
# speky:OTELCOL#T017
#
# Prerequisites:
#   - otel-agent binary at $OTEL_AGENT (default: bin/otel-agent/otel-agent)
#   - opamp-server binary at $OPAMP_SERVER (default: /tmp/opamp-server/opamp-server)
#   - Run from the datadog-agent repo root

set -euo pipefail

OTEL_AGENT="${OTEL_AGENT:-bin/otel-agent/otel-agent}"
OPAMP_SERVER="${OPAMP_SERVER:-/tmp/opamp-server/opamp-server}"
CONFIG="${TMPDIR:-/tmp}/otel-opamp-http-test.yaml"
SERVER_LOG="${TMPDIR:-/tmp}/opamp-server-http.log"
AGENT_LOG="${TMPDIR:-/tmp}/otel-agent-http.log"
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
      http:
        endpoint: http://localhost:4320/v1/opamp
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

step "Starting otel-agent with HTTP OpAmp transport"
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

step "Verifying agent started"
if grep -q "Everything is ready" "$AGENT_LOG"; then
    pass "agent reached ready state"
else
    fail "agent did not reach ready state"; cat "$AGENT_LOG"; exit 1
fi

step "Verifying server received AgentToServer message via HTTP"
if grep -q "Agent connected" "$SERVER_LOG"; then
    pass "server received connection"
else
    fail "server did not receive connection"; cat "$SERVER_LOG"; exit 1
fi
if grep -q "service.name.*otel-agent" "$SERVER_LOG"; then
    pass "AgentDescription contains service.name=otel-agent"
else
    fail "AgentDescription missing service.name"; cat "$SERVER_LOG"
fi

echo
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
