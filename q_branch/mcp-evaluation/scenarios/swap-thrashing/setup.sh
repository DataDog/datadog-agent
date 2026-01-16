#!/bin/bash
set -e

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIMA_VM="${1:-mcp-eval}"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Error: Lima VM '$LIMA_VM' not found"
    echo "Please run: q_branch/mcp-evaluation/scripts/start-vm.sh"
    exit 1
fi

echo "Deploying data processor..."

limactl shell --workdir /tmp "$LIMA_VM" sudo mkdir -p /opt/data_processor
limactl copy "$SCENARIO_DIR/workload.py" "$LIMA_VM:/tmp/service.py"

limactl shell --workdir /tmp "$LIMA_VM" bash <<'EOF'
sudo mv /tmp/service.py /opt/data_processor/service.py
cd /opt/data_processor
python3 service.py > /tmp/data_processor.log 2>&1 &
echo $! > /tmp/data_processor.pid
echo "Service started. PID: $(cat /tmp/data_processor.pid)"
EOF

echo "Deployment complete."
