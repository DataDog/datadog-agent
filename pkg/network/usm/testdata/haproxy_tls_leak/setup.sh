#!/bin/bash
# Setup script for HAProxy TLS leak reproduction.
#
# Usage:
#   ./setup.sh          - Generate certs and start containers
#   ./setup.sh teardown - Stop containers and clean up
#
# After setup, use reproduce.sh to run the full reproduction test.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

if [ "${1}" = "teardown" ]; then
    echo "Stopping containers..."
    docker compose down -v 2>/dev/null || docker-compose down -v 2>/dev/null
    echo "Done."
    exit 0
fi

# Generate TLS certificates if not present
if [ ! -f certs/server.pem ]; then
    echo "Generating TLS certificates..."
    mkdir -p certs
    openssl req -x509 -newkey rsa:2048 -keyout certs/server.key \
        -out certs/server.crt -days 365 -nodes \
        -subj "/CN=haproxy.local" 2>/dev/null
    cat certs/server.crt certs/server.key > certs/server.pem
    rm -f certs/server.key certs/server.crt
    echo "Certificates generated."
fi

# Start containers
echo "Starting containers..."
docker compose up -d 2>/dev/null || docker-compose up -d 2>/dev/null

echo "Waiting for services to be ready..."
sleep 3

# Verify services
echo "Checking services..."
for name in haproxy backend-api backend-blackbox traffic-generator; do
    if docker ps --format '{{.Names}}' | grep -q "^${name}$"; then
        echo "  $name: running"
    else
        echo "  $name: NOT RUNNING"
    fi
done

echo ""
echo "Setup complete. Containers are running."
echo "To reproduce the bug:"
echo "  1. Start system-probe (observe: 0% misattribution)"
echo "  2. Stop system-probe"
echo "  3. Wait 15s (traffic continues on persistent connections)"
echo "  4. Restart system-probe"
echo "  5. Query: sudo curl -s --unix-socket /opt/datadog-agent/run/sysprobe.sock http://unix/network_tracer/debug/http_monitoring > /tmp/http_debug.json"
echo "  6. Analyze: python3 analyze.py /tmp/http_debug.json"