#!/bin/bash
set -e

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIMA_VM="${1:-mcp-eval}"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Error: Lima VM '$LIMA_VM' not found"
    echo "Please run: q_branch/mcp-evaluation/scripts/start-vm.sh"
    exit 1
fi

echo "Deploying session cache service..."

limactl shell --workdir /tmp "$LIMA_VM" sudo mkdir -p /opt/session_cache
limactl copy "$SCENARIO_DIR/workload.py" "$LIMA_VM:/tmp/service.py"

limactl shell --workdir /tmp "$LIMA_VM" bash <<'EOF'
sudo mv /tmp/service.py /opt/session_cache/service.py
cd /opt/session_cache
python3 service.py > /tmp/session_cache.log 2>&1 &
echo $! > /tmp/session_cache.pid
echo "Service started. PID: $(cat /tmp/session_cache.pid)"
EOF

echo "Deployment complete."
