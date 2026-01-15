#!/bin/bash
set -e

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIMA_VM="mcp-eval"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Error: Lima VM '$LIMA_VM' not found"
    echo "Please run: q_branch/mcp-evaluation/scripts/start-vm.sh"
    exit 1
fi

echo "Deploying API services..."

limactl shell "$LIMA_VM" sudo mkdir -p /opt/api_service
limactl copy "$SCENARIO_DIR/server1.py" "$LIMA_VM:/tmp/primary.py"
limactl copy "$SCENARIO_DIR/server2.py" "$LIMA_VM:/tmp/backup.py"

limactl shell "$LIMA_VM" bash <<'EOF'
sudo mv /tmp/primary.py /opt/api_service/primary.py
sudo mv /tmp/backup.py /opt/api_service/backup.py
cd /opt/api_service
python3 primary.py > /tmp/api_primary.log 2>&1 &
echo $! > /tmp/api_primary.pid
sleep 2
python3 backup.py > /tmp/api_backup.log 2>&1 &
echo $! > /tmp/api_backup.pid
echo "Services started."
echo "Primary PID: $(cat /tmp/api_primary.pid)"
echo "Backup PID: $(cat /tmp/api_backup.pid)"
EOF

echo "Deployment complete."
