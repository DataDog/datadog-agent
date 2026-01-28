#!/bin/bash
set -e

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIMA_VM="${1:-mcp-eval}"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Error: Lima VM '$LIMA_VM' not found"
    echo "Please run: q_branch/mcp-evaluation/scripts/start-vm.sh"
    exit 1
fi

echo "Deploying storage sync service..."

limactl shell --workdir /tmp "$LIMA_VM" sudo mkdir -p /opt/storage_sync
limactl copy "$SCENARIO_DIR/workload.py" "$LIMA_VM:/tmp/service.py"

limactl shell --workdir /tmp "$LIMA_VM" bash <<'EOF'
sudo mv /tmp/service.py /opt/storage_sync/service.py
cd /opt/storage_sync
python3 service.py > /tmp/storage_sync.log 2>&1 &
echo $! > /tmp/storage_sync.pid
echo "Service started. PID: $(cat /tmp/storage_sync.pid)"
EOF

echo "Deployment complete."
