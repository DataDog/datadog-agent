#!/bin/bash
set -e

LIMA_VM="${1:-mcp-eval}"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping HTTP service..."

limactl shell --workdir /tmp "$LIMA_VM" bash <<'EOF'
if [ -f /tmp/http_service.pid ]; then
    kill $(cat /tmp/http_service.pid) 2>/dev/null || true
    rm /tmp/http_service.pid
fi
rm -f /tmp/http_service.log
sudo rm -rf /opt/http_service
EOF

echo "Teardown complete."
