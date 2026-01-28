#!/bin/bash
set -e

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIMA_VM="${1:-mcp-eval}"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Error: Lima VM '$LIMA_VM' not found"
    echo "Please run: q_branch/mcp-evaluation/scripts/start-vm.sh"
    exit 1
fi

echo "Deploying cache manager..."

limactl shell --workdir /tmp "$LIMA_VM" sudo mkdir -p /opt/cache_manager /tmp/cache_files
limactl copy "$SCENARIO_DIR/workload.py" "$LIMA_VM:/tmp/service.py"

limactl shell --workdir /tmp "$LIMA_VM" bash <<'EOF'
sudo mv /tmp/service.py /opt/cache_manager/service.py
cd /opt/cache_manager
python3 service.py > /tmp/cache_manager.log 2>&1 &
echo $! > /tmp/cache_manager.pid
echo "Service started. PID: $(cat /tmp/cache_manager.pid)"
EOF

echo "Deployment complete."
