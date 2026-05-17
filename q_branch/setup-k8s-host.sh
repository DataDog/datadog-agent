#!/bin/bash
# Setup script for gadget-k8s-host Lima VM (uses gadget-k8s-host.lima.yaml)
# Creates VM, Kind cluster, and configures kubectl access from macOS

set -euo pipefail

VM_NAME="gadget-k8s-host"
CLUSTER_NAME="gadget-dev"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KUBECONFIG_FILE="$HOME/.kube/gadget-k8s-host.yaml"
DEFAULT_KUBECONFIG="$HOME/.kube/config"

# Merge extracted kubeconfig into default config so kubectx/kubectl can find it
merge_kubeconfig() {
    # Remove any existing entries for this cluster (avoids stale certs)
    kubectl config delete-context "kind-$CLUSTER_NAME" 2>/dev/null || true
    kubectl config delete-cluster "kind-$CLUSTER_NAME" 2>/dev/null || true
    kubectl config delete-user "kind-$CLUSTER_NAME" 2>/dev/null || true

    if [[ -f "$DEFAULT_KUBECONFIG" ]]; then
        # Merge with existing config
        KUBECONFIG="$DEFAULT_KUBECONFIG:$KUBECONFIG_FILE" kubectl config view --flatten > "$DEFAULT_KUBECONFIG.tmp"
        mv "$DEFAULT_KUBECONFIG.tmp" "$DEFAULT_KUBECONFIG"
    else
        # No existing config, just copy
        cp "$KUBECONFIG_FILE" "$DEFAULT_KUBECONFIG"
    fi
    chmod 600 "$DEFAULT_KUBECONFIG"
    rm -f "$KUBECONFIG_FILE"
}

echo "==> Setting up $VM_NAME..."

# Check if VM exists
if limactl list --format '{{.Name}}' 2>/dev/null | grep -q "^${VM_NAME}$"; then
    STATUS=$(limactl list --format '{{.Name}} {{.Status}}' | grep "^${VM_NAME} " | awk '{print $2}')
    if [[ "$STATUS" == "Running" ]]; then
        echo "VM '$VM_NAME' is already running."

        # Check if cluster exists
        if limactl shell "$VM_NAME" -- kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
            echo "Kind cluster '$CLUSTER_NAME' exists."
            read -p "Recreate cluster? [y/N] " -n 1 -r
            echo
            if [[ $REPLY =~ ^[Yy]$ ]]; then
                echo "==> Deleting existing cluster..."
                limactl shell "$VM_NAME" -- kind delete cluster --name "$CLUSTER_NAME"
            else
                echo "Keeping existing cluster. Updating kubeconfig..."
                mkdir -p "$(dirname "$KUBECONFIG_FILE")"
                limactl shell "$VM_NAME" -- kind get kubeconfig --name "$CLUSTER_NAME" > "$KUBECONFIG_FILE"
                chmod 600 "$KUBECONFIG_FILE"
                merge_kubeconfig
                echo "Done. Context 'kind-$CLUSTER_NAME' is available in kubectl/kubectx."
                exit 0
            fi
        fi
    else
        echo "VM exists but is $STATUS. Starting..."
        limactl start "$VM_NAME"
    fi
else
    echo "==> Creating VM (this takes ~5 minutes)..."
    limactl create --name "$VM_NAME" "$SCRIPT_DIR/gadget-k8s-host.lima.yaml" --tty=false
    limactl start "$VM_NAME"
    # Restart to ensure lima user's docker group membership is active
    echo "==> Restarting VM to activate docker group..."
    limactl stop "$VM_NAME"
    limactl start "$VM_NAME"
fi

# Wait for Docker
echo "==> Waiting for Docker..."
for _i in {1..60}; do
    if limactl shell "$VM_NAME" -- docker info &>/dev/null; then
        break
    fi
    sleep 2
done

# Create Kind cluster
echo "==> Creating Kind cluster '$CLUSTER_NAME'..."
limactl shell "$VM_NAME" -- kind create cluster --name "$CLUSTER_NAME" --config /dev/stdin <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  apiServerAddress: "127.0.0.1"
  apiServerPort: 6443
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 30000
        hostPort: 30000
      - containerPort: 30080
        hostPort: 30080
      - containerPort: 30443
        hostPort: 30443
  - role: worker
EOF

# Copy kubeconfig to host and merge into default config
echo "==> Merging kubeconfig into ~/.kube/config..."
mkdir -p "$(dirname "$KUBECONFIG_FILE")"
limactl shell "$VM_NAME" -- kind get kubeconfig --name "$CLUSTER_NAME" > "$KUBECONFIG_FILE"
chmod 600 "$KUBECONFIG_FILE"
merge_kubeconfig

# Verify
echo ""
echo "==> Verifying..."
kubectl --context "kind-$CLUSTER_NAME" get nodes

echo ""
echo "======================================================="
echo "Ready!"
echo ""
echo "  kubectl --context kind-$CLUSTER_NAME get nodes"
echo "  kubectx kind-$CLUSTER_NAME"
echo ""
echo "SSH: limactl shell $VM_NAME"
echo "======================================================="
