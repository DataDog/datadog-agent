#!/bin/bash
# Stop and delete the MCP evaluation Lima VM

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

VM_NAME="mcp-eval"

# Helper functions
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
    log_error "limactl is not installed. Install it with: brew install lima"
    exit 1
fi

# Check if VM exists
if ! limactl list | grep -q "^$VM_NAME"; then
    log_warn "VM '$VM_NAME' does not exist. Nothing to do."

    # Still clean up the Lima directory if it exists (orphaned state)
    LIMA_DIR="$HOME/.lima/$VM_NAME"
    if [ -d "$LIMA_DIR" ]; then
        log_info "Found orphaned Lima directory, cleaning up: $LIMA_DIR"
        rm -rf "$LIMA_DIR"
    fi

    exit 0
fi

# Get VM status
VM_STATUS=$(limactl list "$VM_NAME" 2>/dev/null | tail -n 1 | awk '{print $2}')

log_info "Found VM '$VM_NAME' with status: $VM_STATUS"

# Stop the VM if it's running
if [ "$VM_STATUS" = "Running" ]; then
    log_info "Stopping VM '$VM_NAME'..."
    limactl stop "$VM_NAME"
    log_info "VM stopped successfully"
fi

# Delete the VM
log_info "Deleting VM '$VM_NAME'..."
limactl delete "$VM_NAME"

# Clean up any remaining Lima cache/state for this VM
LIMA_DIR="$HOME/.lima/$VM_NAME"
if [ -d "$LIMA_DIR" ]; then
    log_info "Cleaning up remaining Lima directory: $LIMA_DIR"
    rm -rf "$LIMA_DIR"
fi

log_info "VM '$VM_NAME' has been completely removed"
log_info "To recreate it, run: ./scripts/start-vm.sh"
