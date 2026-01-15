#!/bin/bash
set -euo pipefail

VM_NAME="mcp-eval"
LIMA_CONFIG="lima.yaml"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if limactl is installed
if ! command -v limactl &> /dev/null; then
    log_error "limactl is not installed"
    echo ""
    echo "Install Lima with:"
    echo "  brew install lima"
    exit 1
fi

# Check if lima.yaml exists
if [ ! -f "$LIMA_CONFIG" ]; then
    log_error "Lima configuration file not found: $LIMA_CONFIG"
    exit 1
fi

# Check if VM already exists
if limactl list | grep -q "^$VM_NAME"; then
    VM_STATUS=$(limactl list "$VM_NAME" | tail -n 1 | awk '{print $2}')

    if [ "$VM_STATUS" = "Running" ]; then
        log_info "VM '$VM_NAME' is already running"
        echo ""
        echo "To access the VM:"
        echo "  limactl shell $VM_NAME"
        echo ""
        echo "To stop the VM:"
        echo "  limactl stop $VM_NAME"
        exit 0
    elif [ "$VM_STATUS" = "Stopped" ]; then
        log_info "VM '$VM_NAME' exists but is stopped. Starting..."
        limactl start "$VM_NAME"
    else
        log_warn "VM '$VM_NAME' is in state: $VM_STATUS"
        log_info "Attempting to start..."
        limactl start "$VM_NAME"
    fi
else
    log_info "Creating and starting new VM '$VM_NAME'..."
    limactl start --name="$VM_NAME" "$LIMA_CONFIG"
fi

# Wait a moment for VM to be fully ready
sleep 2

# Verify VM is running
if limactl list "$VM_NAME" | tail -n 1 | grep -q "Running"; then
    log_info "VM '$VM_NAME' is now running"
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Quick commands:"
    echo "  Shell into VM:  limactl shell $VM_NAME"
    echo "  Stop VM:        limactl stop $VM_NAME"
    echo "  Delete VM:      limactl delete $VM_NAME"
    echo ""
    echo "Inside the VM:"
    echo "  cd /mcp"
    echo "  go build -o mcp-server ./cmd/mcp-server"
    echo "  ./mcp-server"
    echo ""
    echo "MCP server will be accessible at: http://localhost:8080/mcp"
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
else
    log_error "Failed to start VM '$VM_NAME'"
    exit 1
fi
