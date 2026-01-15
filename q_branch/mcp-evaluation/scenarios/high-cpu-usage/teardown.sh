#!/bin/bash
set -e

LIMA_VM="mcp-eval"

# Check if Lima VM exists
if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping worker service..."

# Stop the service in VM
limactl shell "$LIMA_VM" bash <<'EOF'
if [ -f /tmp/worker_service.pid ]; then
    kill $(cat /tmp/worker_service.pid) 2>/dev/null || true
    rm /tmp/worker_service.pid
fi
rm -f /tmp/worker_service.log
sudo rm -rf /opt/worker_service
EOF

echo "Teardown complete."
