#!/bin/bash
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# ssl_ctx_race test orchestration script
#
# This script tests whether the ssl_ctx_by_pid_tgid race condition can cause
# practical misattribution of SSL connections.
#
# Prerequisites:
# - system-probe built and running with USM/TLS enabled
# - OpenSSL development libraries (for building the C client)
# - Python 3 with ssl module
# - curl (for querying debug endpoints)
#
# Usage: ./run_race_test.sh [iterations]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../../../.." && pwd)"
CERT_DIR="$REPO_ROOT/pkg/network/protocols/http/testutil/testdata"

PORT1=18001
PORT2=18002
ITERATIONS=${1:-1000}
SYSPROBE_SOCK="/opt/datadog-agent/run/sysprobe.sock"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

cleanup() {
    log_info "Cleaning up..."

    # Kill Python servers
    if [ -n "$SERVER1_PID" ] && kill -0 "$SERVER1_PID" 2>/dev/null; then
        kill "$SERVER1_PID" 2>/dev/null || true
    fi
    if [ -n "$SERVER2_PID" ] && kill -0 "$SERVER2_PID" 2>/dev/null; then
        kill "$SERVER2_PID" 2>/dev/null || true
    fi

    # Kill test client
    if [ -n "$CLIENT_PID" ] && kill -0 "$CLIENT_PID" 2>/dev/null; then
        kill "$CLIENT_PID" 2>/dev/null || true
    fi

    # Remove temp files
    rm -f "$SCRIPT_DIR/server1.py" "$SCRIPT_DIR/server2.py"
}

trap cleanup EXIT

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    if [ ! -f "$CERT_DIR/cert.pem.0" ]; then
        log_error "Certificate not found at $CERT_DIR/cert.pem.0"
        exit 1
    fi

    if [ ! -f "$CERT_DIR/server.key" ]; then
        log_error "Key not found at $CERT_DIR/server.key"
        exit 1
    fi

    if ! command -v python3 &> /dev/null; then
        log_error "python3 is required but not found"
        exit 1
    fi

    if ! command -v gcc &> /dev/null; then
        log_error "gcc is required but not found"
        exit 1
    fi

    if [ ! -S "$SYSPROBE_SOCK" ]; then
        log_warn "system-probe socket not found at $SYSPROBE_SOCK"
        log_warn "Make sure system-probe is running with USM/TLS enabled"
        log_warn "Continuing anyway - debug endpoint queries will fail"
    fi

    log_info "Prerequisites OK"
}

# Build the C test client
build_client() {
    log_info "Building ssl_ctx_race client..."
    cd "$SCRIPT_DIR"
    make clean all
    if [ ! -f "$SCRIPT_DIR/ssl_ctx_race" ]; then
        log_error "Failed to build ssl_ctx_race"
        exit 1
    fi
    log_info "Client built successfully"
}

# Create Python HTTPS server script
create_server_script() {
    local port=$1
    local script_path=$2

    cat > "$script_path" << EOF
import http.server
import ssl
import sys

class RequestHandler(http.server.BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'

    def log_message(self, format, *args):
        # Suppress request logging
        pass

    def do_GET(self):
        # Extract status code from path like /200/marker
        try:
            parts = self.path.split('/')
            if len(parts) >= 2:
                status_code = int(parts[1])
            else:
                status_code = 200
        except:
            status_code = 200

        self.send_response(status_code)
        self.send_header('Content-type', 'text/plain')
        self.send_header('Content-Length', '2')
        self.send_header('Connection', 'keep-alive')
        self.end_headers()
        self.wfile.write(b'OK')

server_address = ('127.0.0.1', $port)
httpd = http.server.HTTPServer(server_address, RequestHandler)

context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.load_cert_chain(certfile='$CERT_DIR/cert.pem.0', keyfile='$CERT_DIR/server.key')
httpd.socket = context.wrap_socket(httpd.socket, server_side=True)

print(f"HTTPS server running on port $port", flush=True)
httpd.serve_forever()
EOF
}

# Start HTTPS servers
start_servers() {
    log_info "Starting HTTPS servers..."

    create_server_script $PORT1 "$SCRIPT_DIR/server1.py"
    create_server_script $PORT2 "$SCRIPT_DIR/server2.py"

    python3 "$SCRIPT_DIR/server1.py" &
    SERVER1_PID=$!

    python3 "$SCRIPT_DIR/server2.py" &
    SERVER2_PID=$!

    # Wait for servers to start
    sleep 1

    # Verify servers are running
    if ! kill -0 "$SERVER1_PID" 2>/dev/null; then
        log_error "Server 1 failed to start"
        exit 1
    fi
    if ! kill -0 "$SERVER2_PID" 2>/dev/null; then
        log_error "Server 2 failed to start"
        exit 1
    fi

    log_info "Servers started: port $PORT1 (PID $SERVER1_PID), port $PORT2 (PID $SERVER2_PID)"
}

# Query USM debug endpoint for HTTP stats
query_usm_stats() {
    local output_file=$1

    if [ ! -S "$SYSPROBE_SOCK" ]; then
        log_warn "Cannot query USM stats - system-probe not running"
        return 1
    fi

    # Query the debug endpoint
    curl -s --unix-socket "$SYSPROBE_SOCK" "http://unix/debug/http_monitoring" > "$output_file" 2>/dev/null || {
        log_warn "Failed to query USM debug endpoint"
        return 1
    }

    return 0
}

# Analyze USM stats for misattribution
analyze_misattribution() {
    local stats_file=$1
    local conn1_local=$2
    local conn2_local=$3

    log_info "Analyzing USM stats for misattribution..."

    if [ ! -f "$stats_file" ] || [ ! -s "$stats_file" ]; then
        log_warn "No stats file or empty stats"
        return
    fi

    # Look for requests with conn1 marker going to port2, or vice versa
    local misattributed=0

    # conn1 requests should go to PORT1, conn2 requests should go to PORT2
    # If we see conn1-iter* with dst_port=PORT2, that's misattribution

    # Count conn1 requests attributed to each port
    local conn1_to_port1=$(grep -c "conn1-iter.*:$PORT1" "$stats_file" 2>/dev/null || echo 0)
    local conn1_to_port2=$(grep -c "conn1-iter.*:$PORT2" "$stats_file" 2>/dev/null || echo 0)
    local conn2_to_port1=$(grep -c "conn2-iter.*:$PORT1" "$stats_file" 2>/dev/null || echo 0)
    local conn2_to_port2=$(grep -c "conn2-iter.*:$PORT2" "$stats_file" 2>/dev/null || echo 0)

    log_info "Attribution analysis:"
    log_info "  conn1 requests -> port $PORT1: $conn1_to_port1"
    log_info "  conn1 requests -> port $PORT2: $conn1_to_port2 (MISATTRIBUTED)"
    log_info "  conn2 requests -> port $PORT1: $conn2_to_port1 (MISATTRIBUTED)"
    log_info "  conn2 requests -> port $PORT2: $conn2_to_port2"

    misattributed=$((conn1_to_port2 + conn2_to_port1))

    if [ "$misattributed" -gt 0 ]; then
        log_error "RACE CONDITION CONFIRMED: $misattributed misattributed requests!"
        return 1
    else
        log_info "No misattribution detected"
        return 0
    fi
}

# Dump ssl_ctx_by_pid_tgid map to verify fallback is being used
dump_ssl_maps() {
    local output_file=$1

    if [ ! -S "$SYSPROBE_SOCK" ]; then
        log_warn "Cannot dump maps - system-probe not running"
        return 1
    fi

    # Try to get map contents via debug endpoint
    curl -s --unix-socket "$SYSPROBE_SOCK" "http://unix/debug/ebpf_maps?maps=ssl_ctx_by_pid_tgid,ssl_sock_by_ctx" > "$output_file" 2>/dev/null || {
        log_warn "Failed to dump eBPF maps"
        return 1
    }

    return 0
}

# Main test flow
main() {
    log_info "=== SSL Context Race Condition Test ==="
    log_info "Iterations: $ITERATIONS"
    log_info ""

    check_prerequisites
    build_client

    # Start servers FIRST (connections will be established to these)
    start_servers

    # Create output directory
    OUTPUT_DIR="$SCRIPT_DIR/test_output_$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$OUTPUT_DIR"

    # Capture initial USM stats
    log_info "Capturing initial USM stats..."
    query_usm_stats "$OUTPUT_DIR/stats_before.json" || true

    # Start the test client (it will establish connections and wait for signal)
    log_info "Starting test client..."
    "$SCRIPT_DIR/ssl_ctx_race" 127.0.0.1 $PORT1 127.0.0.1 $PORT2 $ITERATIONS > "$OUTPUT_DIR/client_output.txt" 2>&1 &
    CLIENT_PID=$!

    # Wait for client to signal ready
    log_info "Waiting for client to establish connections..."
    local timeout=10
    local ready=false
    while [ $timeout -gt 0 ]; do
        if grep -q "^READY:" "$OUTPUT_DIR/client_output.txt" 2>/dev/null; then
            ready=true
            break
        fi
        sleep 0.5
        timeout=$((timeout - 1))
    done

    if [ "$ready" != "true" ]; then
        log_error "Client failed to establish connections"
        cat "$OUTPUT_DIR/client_output.txt"
        exit 1
    fi

    # Parse connection info
    READY_LINE=$(grep "^READY:" "$OUTPUT_DIR/client_output.txt")
    CONN1_LOCAL=$(echo "$READY_LINE" | cut -d: -f2)
    CONN1_REMOTE=$(echo "$READY_LINE" | cut -d: -f3)
    CONN2_LOCAL=$(echo "$READY_LINE" | cut -d: -f4)
    CONN2_REMOTE=$(echo "$READY_LINE" | cut -d: -f5)

    log_info "Connections established:"
    log_info "  conn1: local=$CONN1_LOCAL -> remote=$CONN1_REMOTE"
    log_info "  conn2: local=$CONN2_LOCAL -> remote=$CONN2_REMOTE"

    # Dump SSL maps to verify fallback path is being used
    log_info "Checking eBPF maps..."
    dump_ssl_maps "$OUTPUT_DIR/ssl_maps_before.txt" || true

    # Now signal the client to start the rapid operations
    log_info "Signaling client to start test (SIGUSR1)..."
    kill -USR1 $CLIENT_PID

    # Wait for client to complete
    log_info "Waiting for test to complete..."
    wait $CLIENT_PID || true
    CLIENT_PID=""

    # Display client output
    log_info "Client output:"
    cat "$OUTPUT_DIR/client_output.txt"

    # Capture final USM stats
    log_info "Capturing final USM stats..."
    sleep 2  # Give USM time to process
    query_usm_stats "$OUTPUT_DIR/stats_after.json" || true
    dump_ssl_maps "$OUTPUT_DIR/ssl_maps_after.txt" || true

    # Analyze results
    log_info ""
    log_info "=== Results ==="

    if [ -f "$OUTPUT_DIR/stats_after.json" ]; then
        analyze_misattribution "$OUTPUT_DIR/stats_after.json" "$CONN1_LOCAL" "$CONN2_LOCAL"
        RESULT=$?
    else
        log_warn "Could not analyze USM stats (system-probe not running?)"
        log_info "Manual verification needed. Check:"
        log_info "  1. Run: curl --unix-socket $SYSPROBE_SOCK http://unix/debug/http_monitoring"
        log_info "  2. Look for requests with 'conn1' marker going to port $PORT2"
        log_info "  3. Look for requests with 'conn2' marker going to port $PORT1"
        RESULT=0
    fi

    log_info ""
    log_info "Output files saved to: $OUTPUT_DIR"
    log_info "  - client_output.txt: Test client stdout/stderr"
    log_info "  - stats_before.json: USM stats before test"
    log_info "  - stats_after.json: USM stats after test"
    log_info "  - ssl_maps_*.txt: eBPF map dumps"

    exit $RESULT
}

main "$@"