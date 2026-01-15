#!/bin/bash
set -e

LIMA_VM="mcp-eval"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping storage sync service..."

limactl shell "$LIMA_VM" bash <<'EOF'
if [ -f /tmp/storage_sync.pid ]; then
    kill $(cat /tmp/storage_sync.pid) 2>/dev/null || true
    rm /tmp/storage_sync.pid
fi
rm -f /tmp/storage_sync.log
sudo rm -rf /opt/storage_sync
rm -f /tmp/io_test_*.dat
EOF

echo "Teardown complete."
