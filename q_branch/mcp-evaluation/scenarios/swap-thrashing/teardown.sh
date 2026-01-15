#!/bin/bash
set -e

LIMA_VM="mcp-eval"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping data processor..."

limactl shell "$LIMA_VM" bash <<'EOF'
if [ -f /tmp/data_processor.pid ]; then
    kill $(cat /tmp/data_processor.pid) 2>/dev/null || true
    rm /tmp/data_processor.pid
fi
rm -f /tmp/data_processor.log
sudo rm -rf /opt/data_processor
EOF

echo "Teardown complete."
