#!/bin/bash
set -e

LIMA_VM="${1:-mcp-eval}"

# Check if Lima VM exists
if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping session cache service..."

# Stop the service in VM
limactl shell --workdir /tmp "$LIMA_VM" bash <<'EOF'
if [ -f /tmp/session_cache.pid ]; then
    kill $(cat /tmp/session_cache.pid) 2>/dev/null || true
    rm /tmp/session_cache.pid
fi
rm -f /tmp/session_cache.log
sudo rm -rf /opt/session_cache
EOF

echo "Teardown complete."
