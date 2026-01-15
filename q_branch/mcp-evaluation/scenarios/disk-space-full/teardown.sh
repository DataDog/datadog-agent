#!/bin/bash
set -e

LIMA_VM="mcp-eval"

# Check if Lima VM exists
if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping archive manager..."

# Stop the service in VM
limactl shell "$LIMA_VM" bash <<'EOF'
if [ -f /tmp/archive_manager.pid ]; then
    kill $(cat /tmp/archive_manager.pid) 2>/dev/null || true
    rm /tmp/archive_manager.pid
fi
rm -f /tmp/archive_manager.log
sudo rm -rf /opt/archive_manager
rm -rf /tmp/data_archives
EOF

echo "Teardown complete."
