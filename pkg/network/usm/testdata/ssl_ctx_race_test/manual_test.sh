#!/bin/bash
# Manual test helper for ssl_ctx_race
#
# This script provides step-by-step instructions for manually testing
# the ssl_ctx_by_pid_tgid race condition.
#
# Usage: ./manual_test.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../../../.." && pwd)"
CERT_DIR="$REPO_ROOT/pkg/network/protocols/http/testutil/testdata"
SYSPROBE_SOCK="/opt/datadog-agent/run/sysprobe.sock"

cat << 'EOF'
=============================================================
SSL Context Race Condition Manual Test
=============================================================

This test verifies if the ssl_ctx_by_pid_tgid race can cause
practical misattribution of SSL connections.

THE BUG:
1. Thread calls SSL_read(conn1) -> fallback stores ctx1 in map
2. Thread calls SSL_read(conn2) BEFORE tcp_sendmsg for conn1
   -> OVERWRITES map with ctx2
3. tcp_sendmsg for conn1 reads ctx2 -> MISATTRIBUTION!

The fallback only triggers when ssl_sock_by_ctx lookup misses,
which happens when connections exist BEFORE system-probe starts.

=============================================================
EOF

echo ""
echo "Step 1: Build the test client"
echo "------------------------------"
echo "cd $SCRIPT_DIR && make"
echo ""

echo "Step 2: Start two HTTPS servers (in separate terminals)"
echo "--------------------------------------------------------"
echo ""
echo "Terminal 1 - Server on port 18001:"
cat << EOF
python3 -c "
import http.server, ssl
class H(http.server.BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-Length', '2')
        self.send_header('Connection', 'keep-alive')
        self.end_headers()
        self.wfile.write(b'OK')
s = http.server.HTTPServer(('127.0.0.1', 18001), H)
c = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
c.load_cert_chain('$CERT_DIR/cert.pem.0', '$CERT_DIR/server.key')
s.socket = c.wrap_socket(s.socket, server_side=True)
print('Server 1 on port 18001')
s.serve_forever()
"
EOF
echo ""
echo "Terminal 2 - Server on port 18002:"
cat << EOF
python3 -c "
import http.server, ssl
class H(http.server.BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-Length', '2')
        self.send_header('Connection', 'keep-alive')
        self.end_headers()
        self.wfile.write(b'OK')
s = http.server.HTTPServer(('127.0.0.1', 18002), H)
c = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
c.load_cert_chain('$CERT_DIR/cert.pem.0', '$CERT_DIR/server.key')
s.socket = c.wrap_socket(s.socket, server_side=True)
print('Server 2 on port 18002')
s.serve_forever()
"
EOF
echo ""

echo "Step 3: Start the test client (establishes connections)"
echo "--------------------------------------------------------"
echo "$SCRIPT_DIR/ssl_ctx_race 127.0.0.1 18001 127.0.0.1 18002 1000"
echo ""
echo "The client will print READY and wait for SIGUSR1."
echo "Note the client PID for step 5."
echo ""

echo "Step 4: Start/restart system-probe with USM enabled"
echo "----------------------------------------------------"
echo "This ensures the connections exist BEFORE monitoring starts,"
echo "forcing the fallback path in tup_from_ssl_ctx()."
echo ""
echo "Edit /etc/datadog-agent/system-probe.yaml:"
echo "  service_monitoring_config:"
echo "    enabled: true"
echo "    tls:"
echo "      native:"
echo "        enabled: true"
echo ""
echo "sudo systemctl restart datadog-agent-sysprobe"
echo ""

echo "Step 5: Signal the test client to start"
echo "----------------------------------------"
echo "kill -USR1 <CLIENT_PID>"
echo ""

echo "Step 6: Check USM stats for misattribution"
echo "-------------------------------------------"
echo "curl --unix-socket $SYSPROBE_SOCK http://unix/debug/http_monitoring | jq ."
echo ""
echo "Look for:"
echo "  - Requests with 'conn1' marker going to port 18002 (WRONG)"
echo "  - Requests with 'conn2' marker going to port 18001 (WRONG)"
echo ""

echo "Step 7: Check eBPF maps (optional debugging)"
echo "---------------------------------------------"
echo "# Check if ssl_ctx_by_pid_tgid has entries (fallback being used)"
echo "sudo bpftool map dump name ssl_ctx_by_pi"
echo ""
echo "# Check ssl_sock_by_ctx entries"
echo "sudo bpftool map dump name ssl_sock_by_ct"
echo ""

echo "=============================================================
EXPECTED OUTCOMES:

1. If misattribution detected:
   -> RACE CONDITION IS PRACTICAL
   -> Bug can cause real issues in production

2. If no misattribution:
   -> Race window may be too tight
   -> Try increasing iterations
   -> Try on systems with slower I/O

3. If connections use ssl_sock_by_ctx (not fallback):
   -> Test setup issue - monitor started before connections
   -> Restart system-probe AFTER client shows READY
============================================================="