#!/bin/bash
set -e

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIMA_VM="mcp-eval"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Error: Lima VM '$LIMA_VM' not found"
    echo "Please run: q_branch/mcp-evaluation/scripts/start-vm.sh"
    exit 1
fi

echo "Deploying connection monitor..."

limactl shell "$LIMA_VM" sudo mkdir -p /opt/connection_monitor
limactl copy "$SCENARIO_DIR/workload.py" "$LIMA_VM:/tmp/service.py"
limactl copy "$SCENARIO_DIR/resolv.conf.broken" "$LIMA_VM:/tmp/resolv.conf.test"

limactl shell "$LIMA_VM" bash <<'EOF'
# Backup original resolv.conf
sudo cp /etc/resolv.conf /etc/resolv.conf.backup

# Replace with broken config
sudo mv /tmp/resolv.conf.test /etc/resolv.conf

# Move service file
sudo mv /tmp/service.py /opt/connection_monitor/service.py

# Start the service
cd /opt/connection_monitor
python3 service.py > /tmp/connection_monitor.log 2>&1 &
echo $! > /tmp/connection_monitor.pid
echo "Service started. PID: $(cat /tmp/connection_monitor.pid)"
EOF

echo "Deployment complete."
