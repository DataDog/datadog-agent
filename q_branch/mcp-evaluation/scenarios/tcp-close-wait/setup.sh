#!/bin/bash
set -e

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIMA_VM="mcp-eval"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Error: Lima VM '$LIMA_VM' not found"
    echo "Please run: q_branch/mcp-evaluation/scripts/start-vm.sh"
    exit 1
fi

echo "Deploying HTTP service..."

limactl shell "$LIMA_VM" sudo mkdir -p /opt/http_service
limactl copy "$SCENARIO_DIR/server.py" "$LIMA_VM:/tmp/server.py"

limactl shell "$LIMA_VM" bash <<'EOF'
sudo mv /tmp/server.py /opt/http_service/server.py
cd /opt/http_service
python3 server.py > /tmp/http_service.log 2>&1 &
echo $! > /tmp/http_service.pid
echo "Service started. PID: $(cat /tmp/http_service.pid)"

# Generate some client traffic
sleep 2
for i in {1..50}; do curl -s http://localhost:9000/ >/dev/null 2>&1 & done
EOF

echo "Deployment complete."
