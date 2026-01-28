#!/bin/bash
set -e

LIMA_VM="${1:-mcp-eval}"

# Check if Lima VM exists
if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping API services..."

# Stop the services in VM
limactl shell --workdir /tmp "$LIMA_VM" bash <<'EOF'
for pidfile in /tmp/api_primary.pid /tmp/api_backup.pid; do
    if [ -f "$pidfile" ]; then
        kill $(cat "$pidfile") 2>/dev/null || true
        rm "$pidfile"
    fi
done
rm -f /tmp/api_primary.log /tmp/api_backup.log
sudo rm -rf /opt/api_service
EOF

echo "Teardown complete."
