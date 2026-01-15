#!/bin/bash
set -e

LIMA_VM="mcp-eval"

# Check if Lima VM exists
if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping connection monitor..."

# Stop the service and restore resolv.conf in VM
limactl shell "$LIMA_VM" bash <<'EOF'
if [ -f /tmp/connection_monitor.pid ]; then
    kill $(cat /tmp/connection_monitor.pid) 2>/dev/null || true
    rm /tmp/connection_monitor.pid
fi
rm -f /tmp/connection_monitor.log
sudo rm -rf /opt/connection_monitor

# Restore original resolv.conf
if [ -f /etc/resolv.conf.backup ]; then
    sudo mv /etc/resolv.conf.backup /etc/resolv.conf
fi
EOF

echo "Teardown complete."
