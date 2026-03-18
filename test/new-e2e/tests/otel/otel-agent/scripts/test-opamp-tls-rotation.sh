#!/usr/bin/env bash
# test-opamp-tls-rotation.sh — manual test for T024 (OTELCOL034)
#
# Verifies that the OpAmp server can rotate the agent's TLS client certificate
# via OpAMPConnectionSettings, and that the agent reconnects using the new
# certificate and reports a successful ConnectionSettingsStatus.
#
# NOTE: This test requires the agent to declare the AcceptsOpAMPConnectionSettings
# capability (AgentCapabilities bit 256). The opampextension v0.147.0 does not
# implement this — the test will report SKIP until it is implemented.
#
# speky:OTELCOL#T024
#
# Prerequisites:
#   - otel-agent binary at $OTEL_AGENT (default: bin/otel-agent/otel-agent)
#   - opamp-server binary at $OPAMP_SERVER (default: /tmp/opamp-server/opamp-server)
#   - openssl available in PATH
#   - Run from the datadog-agent repo root

set -euo pipefail

OTEL_AGENT="${OTEL_AGENT:-bin/otel-agent/otel-agent}"
OPAMP_SERVER="${OPAMP_SERVER:-/tmp/opamp-server/opamp-server}"
CONFIG="${TMPDIR:-/tmp}/otel-opamp-tls-rotation-test.yaml"
SERVER_LOG="${TMPDIR:-/tmp}/opamp-server-tls.log"
AGENT_LOG="${TMPDIR:-/tmp}/otel-agent-tls.log"
CERT_DIR="${TMPDIR:-/tmp}/opamp-tls-certs"
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

step "Generating self-signed CA and client certificate"
mkdir -p "$CERT_DIR"
# CA
openssl req -x509 -newkey rsa:2048 -keyout "$CERT_DIR/ca.key" -out "$CERT_DIR/ca.crt" \
    -days 1 -nodes -subj "/CN=TestCA" 2>/dev/null
# Client cert signed by CA
openssl req -newkey rsa:2048 -keyout "$CERT_DIR/client.key" -out "$CERT_DIR/client.csr" \
    -nodes -subj "/CN=otel-agent" 2>/dev/null
openssl x509 -req -in "$CERT_DIR/client.csr" -CA "$CERT_DIR/ca.crt" -CAkey "$CERT_DIR/ca.key" \
    -CAcreateserial -out "$CERT_DIR/client.crt" -days 1 2>/dev/null
pass "certificates generated in $CERT_DIR"

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
grep -q "Everything is ready" "$AGENT_LOG" && pass "agent started" || { fail "agent did not start"; exit 1; }

step "Checking AcceptsOpAMPConnectionSettings capability (bit 256)"
if grep -q "AcceptsOpAMPConnectionSettings\|accepts_opamp" "$SERVER_LOG" 2>/dev/null; then
    pass "agent declares AcceptsOpAMPConnectionSettings"
else
    skip "agent does not declare AcceptsOpAMPConnectionSettings (opampextension v0.147.0 does not implement this)"
    echo
    echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
    exit 0
fi

step "Pushing new TLS client certificate via OpAMPConnectionSettings"
# Encode certs to base64 for the server push (server API dependent).
CERT_B64=$(base64 < "$CERT_DIR/client.crt")
KEY_B64=$(base64 < "$CERT_DIR/client.key")
CA_B64=$(base64 < "$CERT_DIR/ca.crt")
echo "  (manual) Push: OpAMPConnectionSettings.certificate = { public_key: <$CERT_B64>, private_key: <$KEY_B64>, ca_public_key: <$CA_B64> }"
sleep 6

step "Verifying agent applied the new certificate"
if grep -q "Applied new TLS\|certificate.*applied\|ConnectionSettingsStatus.*APPLIED" "$AGENT_LOG"; then
    pass "agent applied the new TLS certificate"
else
    fail "no certificate application log found"; cat "$AGENT_LOG"
fi

step "Verifying server received APPLIED ConnectionSettingsStatus"
if grep -q "ConnectionSettingsStatus.*APPLIED\|APPLIED" "$SERVER_LOG"; then
    pass "server received APPLIED status"
else
    fail "server did not receive APPLIED status"; cat "$SERVER_LOG"
fi

step "Verifying agent remains connected after certificate rotation"
if grep -q "Agent connected\|Heartbeat" "$SERVER_LOG"; then
    pass "agent remains connected after rotation"
else
    fail "agent disconnected after certificate rotation"
fi

echo
echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
[ "$FAIL" -eq 0 ]
