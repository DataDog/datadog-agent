#!/bin/bash
set -e

LIMA_VM="mcp-eval"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping cache manager..."

limactl shell "$LIMA_VM" bash <<'EOF'
if [ -f /tmp/cache_manager.pid ]; then
    kill $(cat /tmp/cache_manager.pid) 2>/dev/null || true
    rm /tmp/cache_manager.pid
fi
rm -f /tmp/cache_manager.log
sudo rm -rf /opt/cache_manager
# This may take time if many files were created
rm -rf /tmp/cache_files
EOF

echo "Teardown complete."
