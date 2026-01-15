#!/bin/bash
set -e

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIMA_VM="mcp-eval"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Error: Lima VM '$LIMA_VM' not found"
    echo "Please run: q_branch/mcp-evaluation/scripts/start-vm.sh"
    exit 1
fi

echo "Deploying data collector service..."

limactl shell "$LIMA_VM" sudo mkdir -p /opt/data_collector
limactl copy "$SCENARIO_DIR/workload.py" "$LIMA_VM:/tmp/service.py"

limactl shell "$LIMA_VM" bash <<'EOF'
sudo mv /tmp/service.py /opt/data_collector/service.py
cd /opt/data_collector
python3 service.py > /tmp/data_collector.log 2>&1 &
echo $! > /tmp/data_collector.pid
echo "Service started. PID: $(cat /tmp/data_collector.pid)"
EOF

echo "Deployment complete."
