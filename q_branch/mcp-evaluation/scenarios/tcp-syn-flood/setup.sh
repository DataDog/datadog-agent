#!/bin/bash
set -e

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIMA_VM="${1:-mcp-eval}"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Error: Lima VM '$LIMA_VM' not found"
    echo "Please run: q_branch/mcp-evaluation/scripts/start-vm.sh"
    exit 1
fi

echo "Deploying connection tester..."

limactl shell --workdir /tmp "$LIMA_VM" sudo mkdir -p /opt/connection_tester
limactl copy "$SCENARIO_DIR/workload.py" "$LIMA_VM:/tmp/service.py"

limactl shell --workdir /tmp "$LIMA_VM" bash <<'EOF'
sudo mv /tmp/service.py /opt/connection_tester/service.py
cd /opt/connection_tester
python3 service.py > /tmp/connection_tester.log 2>&1 &
echo $! > /tmp/connection_tester.pid
echo "Service started. PID: $(cat /tmp/connection_tester.pid)"
EOF

echo "Deployment complete."
