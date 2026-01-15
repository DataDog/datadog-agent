#!/bin/bash
set -e

LIMA_VM="mcp-eval"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping application service..."

limactl shell "$LIMA_VM" bash <<'EOF'
if [ -f /tmp/app_service.pid ]; then
    kill $(cat /tmp/app_service.pid) 2>/dev/null || true
    rm /tmp/app_service.pid
fi
rm -f /tmp/app_service.log
sudo rm -rf /opt/app_service /tmp/app_logs
EOF

echo "Teardown complete."
