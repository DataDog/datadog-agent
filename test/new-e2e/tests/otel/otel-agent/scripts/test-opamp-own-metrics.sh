#!/usr/bin/env bash
# test-opamp-own-metrics.sh — manual test for T022 (OTELCOL032)
#
# Verifies that when the OpAmp server pushes OtlpConnectionSettings for own
# metrics, the Collector exports its internal metrics to the specified endpoint.
#
# NOTE: This test requires the agent to declare the ReportsOwnMetrics capability
# (AgentCapabilities bit 64). The opampextension v0.147.0 does not implement
# this — the test will report SKIP until it is implemented.
#
# speky:OTELCOL#T022
#
# Prerequisites:
#   - otel-agent binary at $OTEL_AGENT (default: bin/otel-agent/otel-agent)
#   - opamp-server binary at $OPAMP_SERVER (default: /tmp/opamp-server/opamp-server)
#   - A second otel-agent instance running as local OTLP metrics sink on :5317/:5318
#   - Run from the datadog-agent repo root

set -euo pipefail

OTEL_AGENT="${OTEL_AGENT:-bin/otel-agent/otel-agent}"
OPAMP_SERVER="${OPAMP_SERVER:-/tmp/opamp-server/opamp-server}"
CONFIG="${TMPDIR:-/tmp}/otel-opamp-own-metrics-test.yaml"
SINK_CONFIG="${TMPDIR:-/tmp}/otel-sink.yaml"
SERVER_LOG="${TMPDIR:-/tmp}/opamp-server-own-metrics.log"
AGENT_LOG="${TMPDIR:-/tmp}/otel-agent-own-metrics.log"
SINK_LOG="${TMPDIR:-/tmp}/otel-sink.log"
PASS=0
FAIL=0
SKIP=0

pass() { echo "  PASS: $*"; PASS=$((PASS+1)); }
fail() { echo "  FAIL: $*"; FAIL=$((FAIL+1)); }
skip() { echo "  SKIP: $*"; SKIP=$((SKIP+1)); }
step() { echo; echo "==> $*"; }

cleanup() {
    kill "$AGENT_PID" "$SERVER_PID" "${SINK_PID:-}" 2>/dev/null || true
    wait "$AGENT_PID" "$SERVER_PID" "${SINK_PID:-}" 2>/dev/null || true
}
trap cleanup EXIT

# Main agent config with opamp.
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

# Sink config: accepts OTLP metrics and logs them.
cat > "$SINK_CONFIG" <<'EOF'
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:5317
exporters:
  debug:
    verbosity: detailed
service:
  pipelines:
    metrics:
      receivers: [otlp]
      exporters: [debug]
EOF

step "Starting OpAmp server"
"$OPAMP_SERVER" >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!
sleep 1
grep -q "listening" "$SERVER_LOG" && pass "server started" || { fail "server did not start"; exit 1; }

step "Starting OTLP metrics sink on :5317"
DD_OTELCOLLECTOR_ENABLED=true \
DD_API_KEY=test \
DD_CMD_PORT=0 \
DD_AGENT_IPC_PORT=-1 \
DD_AGENT_IPC_CONFIG_REFRESH_INTERVAL=0 \
DD_ENABLE_METADATA_COLLECTION=false \
DD_HOSTNAME=sink-host \
    "$OTEL_AGENT" --config "file:$SINK_CONFIG" >"$SINK_LOG" 2>&1 &
SINK_PID=$!
sleep 4

step "Starting main otel-agent"
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

step "Checking ReportsOwnMetrics capability (bit 64)"
if grep -q "ReportsOwnMetrics\|own.metrics" "$SERVER_LOG" 2>/dev/null; then
    pass "agent declares ReportsOwnMetrics capability"
else
    skip "agent does not declare ReportsOwnMetrics (opampextension v0.147.0 does not implement this)"
    echo
    echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
    exit 0
fi

step "Server pushes OtlpConnectionSettings for own_metrics -> localhost:5317"
# This step requires the opamp-server to support pushing OtlpConnectionSettings.
# Manual step: send via server API or CLI if supported.
echo "  (manual) Push: ConnectionSettingsOffers.own_metrics = { endpoint: http://localhost:5317 }"
sleep 10

step "Verifying sink received internal metrics from the agent"
if grep -q "otelcol_receiver_accepted\|otelcol_exporter_sent" "$SINK_LOG"; then
    pass "sink received internal metrics from the agent"
else
    fail "sink did not receive internal metrics within timeout"
    cat "$SINK_LOG"
fi

echo
echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
[ "$FAIL" -eq 0 ]
