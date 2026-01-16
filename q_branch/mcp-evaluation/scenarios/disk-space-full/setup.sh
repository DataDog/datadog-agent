#!/bin/bash
set -e

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIMA_VM="${1:-mcp-eval}"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Error: Lima VM '$LIMA_VM' not found"
    echo "Please run: q_branch/mcp-evaluation/scripts/start-vm.sh"
    exit 1
fi

echo "Deploying archive manager..."

limactl shell --workdir /tmp "$LIMA_VM" sudo mkdir -p /opt/archive_manager
limactl shell --workdir /tmp "$LIMA_VM" mkdir -p /tmp/data_archives
limactl copy "$SCENARIO_DIR/workload.py" "$LIMA_VM:/tmp/service.py"

limactl shell --workdir /tmp "$LIMA_VM" bash <<'EOF'
sudo mv /tmp/service.py /opt/archive_manager/service.py
cd /opt/archive_manager
python3 service.py > /tmp/archive_manager.log 2>&1 &
echo $! > /tmp/archive_manager.pid
echo "Service started. PID: $(cat /tmp/archive_manager.pid)"
EOF

echo "Deployment complete."
