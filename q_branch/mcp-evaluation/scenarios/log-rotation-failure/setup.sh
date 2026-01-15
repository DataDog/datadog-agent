#!/bin/bash
set -e

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIMA_VM="mcp-eval"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Error: Lima VM '$LIMA_VM' not found"
    echo "Please run: q_branch/mcp-evaluation/scripts/start-vm.sh"
    exit 1
fi

echo "Deploying application service..."

limactl shell "$LIMA_VM" sudo mkdir -p /opt/app_service /tmp/app_logs
limactl copy "$SCENARIO_DIR/workload.py" "$LIMA_VM:/tmp/service.py"

limactl shell "$LIMA_VM" bash <<'EOF'
sudo mv /tmp/service.py /opt/app_service/service.py
cd /opt/app_service
python3 service.py > /tmp/app_service.log 2>&1 &
echo $! > /tmp/app_service.pid
echo "Service started. PID: $(cat /tmp/app_service.pid)"
EOF

echo "Deployment complete."
