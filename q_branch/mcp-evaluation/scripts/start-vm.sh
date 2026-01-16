#!/bin/bash
set -euo pipefail

VM_NAME="${1:-mcp-eval}"
LIMA_CONFIG="${2:-lima.yaml}"
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
log_info "Checking if VM '$VM_NAME' exists..."
VM_COUNT=$(limactl list --format json | jq -s "map(select(.name == \"$VM_NAME\")) | length")
if [ "$VM_COUNT" -gt 0 ]; then
    log_info "VM '$VM_NAME' found, checking status..."
    VM_STATUS=$(limactl list --format json | jq -rs "map(select(.name == \"$VM_NAME\"))[0].status")

    if [ "$VM_STATUS" = "Running" ]; then
        log_info "VM '$VM_NAME' is already running"
        # Will refresh MCP directory below
    elif [ "$VM_STATUS" = "Stopped" ]; then
        log_info "VM '$VM_NAME' exists but is stopped. Starting..."
        limactl start -y "$VM_NAME"
    else
        log_warn "VM '$VM_NAME' is in state: $VM_STATUS"
        log_info "Attempting to start..."
        limactl start -y "$VM_NAME"
    fi
else
    log_info "Creating and starting new VM '$VM_NAME'..."
    limactl start -y --name="$VM_NAME" "$LIMA_CONFIG"
fi

# Wait a moment for VM to be fully ready
sleep 2

# Verify VM is running
if limactl list "$VM_NAME" | tail -n 1 | grep -q "Running"; then
    log_info "VM '$VM_NAME' is now running"

    # Copy MCP directory to VM for isolation
    MCP_SOURCE="$SCRIPT_DIR/../mcp"
    if [ -d "$MCP_SOURCE" ]; then
        log_info "Copying MCP directory to VM (for isolation)..."
        # Remove existing /mcp if it exists to ensure clean state
        limactl shell --workdir /tmp "$VM_NAME" bash -c "sudo rm -rf /mcp" 2>/dev/null || true
        limactl copy "$MCP_SOURCE" "$VM_NAME:/tmp/"
        limactl shell --workdir /tmp "$VM_NAME" bash -c "sudo mv /tmp/mcp /mcp && sudo chmod -R 755 /mcp"
        log_info "MCP directory copied to /mcp in VM"

        # Extract mode from VM name (e.g., mcp-eval-bash -> bash)
        if [[ "$VM_NAME" =~ mcp-eval-(.*) ]]; then
            MODE="${BASH_REMATCH[1]}"
        else
            MODE="bash"  # default
        fi

        log_info "Building MCP server and setting up service (mode: $MODE)..."
        limactl shell --workdir /tmp "$VM_NAME" bash <<EOF
# Build the MCP server
cd /mcp && make build

# Set SELinux to permissive mode (required for service to execute binary)
sudo setenforce 0

# Make SELinux permissive persistent across reboots
sudo sed -i 's/^SELINUX=enforcing$/SELINUX=permissive/' /etc/selinux/config

# Install and start systemd service
sudo sed "s/MODE_PLACEHOLDER/$MODE/" /mcp/mcp-server.service > /tmp/mcp-server.service
sudo mv /tmp/mcp-server.service /etc/systemd/system/mcp-server.service
sudo systemctl daemon-reload
sudo systemctl enable mcp-server
sudo systemctl restart mcp-server
sleep 3
sudo systemctl status mcp-server --no-pager || true
EOF
        log_info "MCP server service is running"
    else
        log_warn "MCP source directory not found at $MCP_SOURCE"
    fi
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
