#!/bin/bash
set -e

LIMA_VM="mcp-eval"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping data collector service..."

limactl shell "$LIMA_VM" bash <<'EOF'
if [ -f /tmp/data_collector.pid ]; then
    kill $(cat /tmp/data_collector.pid) 2>/dev/null || true
    rm /tmp/data_collector.pid
fi
rm -f /tmp/data_collector.log
sudo rm -rf /opt/data_collector
EOF

echo "Teardown complete."
