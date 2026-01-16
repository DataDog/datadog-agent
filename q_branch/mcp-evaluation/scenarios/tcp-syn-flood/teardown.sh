#!/bin/bash
set -e

LIMA_VM="${1:-mcp-eval}"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping connection tester..."

limactl shell --workdir /tmp "$LIMA_VM" bash <<'EOF'
if [ -f /tmp/connection_tester.pid ]; then
    kill $(cat /tmp/connection_tester.pid) 2>/dev/null || true
    rm /tmp/connection_tester.pid
fi
rm -f /tmp/connection_tester.log
sudo rm -rf /opt/connection_tester
EOF

echo "Teardown complete."
