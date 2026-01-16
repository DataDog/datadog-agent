#!/bin/bash
set -e

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIMA_VM="${1:-mcp-eval}"

# Check if Lima VM exists
if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Error: Lima VM '$LIMA_VM' not found"
    echo "Please run: q_branch/mcp-evaluation/scripts/start-vm.sh"
    exit 1
fi

echo "Deploying worker service..."

# Create service directory in VM
limactl shell --workdir /tmp "$LIMA_VM" sudo mkdir -p /opt/worker_service

# Copy workload to VM temp location first
limactl copy "$SCENARIO_DIR/workload.py" "$LIMA_VM:/tmp/service.py"

# Move to /opt with sudo and start service
limactl shell --workdir /tmp "$LIMA_VM" bash <<'EOF'
sudo mv /tmp/service.py /opt/worker_service/service.py
cd /opt/worker_service
python3 service.py > /tmp/worker_service.log 2>&1 &
echo $! > /tmp/worker_service.pid
echo "Service started. PID: $(cat /tmp/worker_service.pid)"
EOF

echo "Deployment complete."
