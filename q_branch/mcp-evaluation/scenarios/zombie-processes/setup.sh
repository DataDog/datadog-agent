#!/bin/bash
set -e

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIMA_VM="mcp-eval"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Error: Lima VM '$LIMA_VM' not found"
    echo "Please run: q_branch/mcp-evaluation/scripts/start-vm.sh"
    exit 1
fi

echo "Deploying task manager..."

limactl shell "$LIMA_VM" sudo mkdir -p /opt/task_manager
limactl copy "$SCENARIO_DIR/workload.py" "$LIMA_VM:/tmp/service.py"

limactl shell "$LIMA_VM" bash <<'EOF'
sudo mv /tmp/service.py /opt/task_manager/service.py
cd /opt/task_manager
python3 service.py > /tmp/task_manager.log 2>&1 &
echo $! > /tmp/task_manager.pid
echo "Service started. PID: $(cat /tmp/task_manager.pid)"
EOF

echo "Deployment complete."
