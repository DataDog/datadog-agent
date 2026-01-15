#!/bin/bash
set -e

LIMA_VM="mcp-eval"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping task coordinator..."

limactl shell "$LIMA_VM" bash <<'EOF'
if [ -f /tmp/task_coordinator.pid ]; then
    kill $(cat /tmp/task_coordinator.pid) 2>/dev/null || true
    rm /tmp/task_coordinator.pid
fi
rm -f /tmp/task_coordinator.log
sudo rm -rf /opt/task_coordinator
EOF

echo "Teardown complete."
