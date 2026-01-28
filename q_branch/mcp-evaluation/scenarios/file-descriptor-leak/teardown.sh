#!/bin/bash
set -e

LIMA_VM="${1:-mcp-eval}"

if ! limactl list | grep -q "$LIMA_VM"; then
    echo "Lima VM '$LIMA_VM' does not exist. Nothing to tear down."
    exit 1
fi

echo "Stopping metrics collector..."

limactl shell --workdir /tmp "$LIMA_VM" bash <<'EOF'
if [ -f /tmp/metrics_collector.pid ]; then
    kill $(cat /tmp/metrics_collector.pid) 2>/dev/null || true
    rm /tmp/metrics_collector.pid
fi
rm -f /tmp/metrics_collector.log
sudo rm -rf /opt/metrics_collector
EOF

echo "Teardown complete."
