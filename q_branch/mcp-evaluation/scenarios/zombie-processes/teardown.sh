#!/bin/bash
set -e

LIMA_VM="${1:-mcp-eval}"

# Check if Lima VM exists
if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping task manager..."

# Stop the service in VM
limactl shell --workdir /tmp "$LIMA_VM" bash <<'EOF'
if [ -f /tmp/task_manager.pid ]; then
    kill $(cat /tmp/task_manager.pid) 2>/dev/null || true
    rm /tmp/task_manager.pid
fi
rm -f /tmp/task_manager.log
sudo rm -rf /opt/task_manager
EOF

echo "Teardown complete."
