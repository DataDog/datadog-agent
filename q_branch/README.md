# q_branch - Kubernetes Development Infrastructure

Development infrastructure for Kubernetes-based experiments. Supports both Lima VM mode (macOS, Linux with KVM) and direct Docker mode (Workspaces, containers).

## Project Structure

```
q_branch/
├── lib/                      # Shared Python library
│   ├── __init__.py
│   └── k8s_backend.py        # VM/Direct mode backend abstraction
├── dev.py                    # Environment status CLI (minimal)
├── gadget-k8s-host.lima.yaml # Lima VM configuration
└── fine-grained-monitor/     # FGM experiment
    ├── dev.py                # Full FGM development CLI
    └── scenario.py           # Scenario runner
```

## Execution Modes

The infrastructure automatically detects and uses the appropriate mode:

| Environment | Mode | Description |
|-------------|------|-------------|
| macOS | VM | Lima VM with nested Kind cluster |
| Linux + KVM | VM | Lima VM with nested Kind cluster |
| Linux (no KVM) | Direct | Kind cluster directly on host Docker |

Check current mode:
```bash
cd q_branch && ./dev.py status
```

## Quick Start

### For Fine-Grained Monitor Development

See `fine-grained-monitor/README.md` for full documentation.

```bash
cd q_branch/fine-grained-monitor
./dev.py cluster deploy    # Create cluster + deploy FGM
./dev.py cluster status    # Check pod status
```

### Using the Shared Library

Projects can import shared utilities:

```python
import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent))

from lib.k8s_backend import (
    Mode, Environment, detect_environment, create_backend,
    run_cmd, check_health, format_uptime,
)
import dev as q_branch_dev

# Auto-detect mode and create backend
env = detect_environment()
backend = create_backend(env, "gadget-k8s-host")

# Execute commands through backend (handles VM vs Direct)
returncode, stdout, stderr = backend.exec(["kind", "get", "clusters"])

# Use shared cluster utilities
worktree_id = q_branch_dev.get_worktree_id(PROJECT_ROOT)
cluster_name = q_branch_dev.get_cluster_name("myproject", worktree_id)
```

---

## Lima VM Setup (VM Mode Only)

For macOS or Linux with KVM, you need to set up the Lima VM first.

### One-Time Setup

```bash
# Create VM and Kind cluster
./setup-k8s-host.sh

# Install Datadog Operator
limactl shell gadget-k8s-host -- helm install datadog-operator datadog/datadog-operator
```

This creates:
- Lima VM: `gadget-k8s-host`
- Kind cluster: `gadget-dev` (1 control-plane + 2 workers)
- Kubeconfig merged into `~/.kube/config`
- Datadog Operator ready to deploy agents

## Dev Loop

```bash

# 0. Ensure docker desktop is running and the lima VM is started
limactl start gadget-k8s-host

# 1. Build and load image into Kind
dda inv omnibus.docker-build && docker save localhost/datadog-agent:local | limactl shell gadget-k8s-host -- docker load && limactl shell gadget-k8s-host -- kind load docker-image localhost/datadog-agent:local --name gadget-dev

# 2. Deploy agent (only necessary when test-cluster.yaml has changed)
kubectl apply -f test-cluster.yaml --context kind-gadget-dev

# 3. Restart agent to pick up new image
kubectl delete pods -l app.kubernetes.io/name=datadog-agent -n default --context kind-gadget-dev

# 4. Watch pods come up
kubectl get pods -w --context kind-gadget-dev
```

To redeploy after code changes, repeat steps 1 and 3.

## K8s MCP Integration

Enable Claude Code to interact with the Kind cluster via the [kubernetes-mcp-server](https://github.com/containers/kubernetes-mcp-server).

This creates a **dedicated kubeconfig isolated to only the gadget-dev cluster** - the MCP server has no access to any other Kubernetes clusters in your `~/.kube/config`.

### Setup (One-Time)

```bash
# 1. Create namespace and service account
kubectl --context kind-gadget-dev create namespace mcp
kubectl --context kind-gadget-dev create serviceaccount mcp-viewer -n mcp

# 2. Grant cluster-admin (full access for dev)
kubectl --context kind-gadget-dev create clusterrolebinding mcp-viewer-crb \
  --clusterrole=cluster-admin \
  --serviceaccount=mcp:mcp-viewer

# 3. Create dedicated kubeconfig with 1-year token
KUBECONFIG_FILE="$HOME/.kube/mcp-viewer.kubeconfig"
TOKEN="$(kubectl --context kind-gadget-dev create token mcp-viewer --duration=8760h -n mcp)"
API_SERVER="$(kubectl config view --context kind-gadget-dev --minify -o jsonpath='{.clusters[0].cluster.server}')"
CA_DATA="$(kubectl config view --context kind-gadget-dev --minify --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')"

cat > "$KUBECONFIG_FILE" << EOF
apiVersion: v1
kind: Config
clusters:
- name: mcp-viewer-cluster
  cluster:
    server: $API_SERVER
    certificate-authority-data: $CA_DATA
users:
- name: mcp-viewer
  user:
    token: $TOKEN
contexts:
- name: mcp-viewer-context
  context:
    cluster: mcp-viewer-cluster
    user: mcp-viewer
current-context: mcp-viewer-context
EOF

chmod 600 "$KUBECONFIG_FILE"

# 4. Verify kubeconfig works
kubectl --kubeconfig="$KUBECONFIG_FILE" get pods -A

# 5. Add MCP server to Claude Code (use full path, not $HOME - env vars don't expand in JSON)
claude mcp add-json kubernetes-mcp-server \
  "{\"command\":\"npx\",\"args\":[\"-y\",\"kubernetes-mcp-server@latest\"],\"env\":{\"KUBECONFIG\":\"$HOME/.kube/mcp-viewer.kubeconfig\"}}" \
  -s user
```

### Token Renewal

The token expires after 1 year. To renew:

```bash
TOKEN="$(kubectl --context kind-gadget-dev create token mcp-viewer --duration=8760h -n mcp)"
kubectl config --kubeconfig="$HOME/.kube/mcp-viewer.kubeconfig" set-credentials mcp-viewer --token="$TOKEN"
```

### Cleanup

```bash
kubectl --context kind-gadget-dev delete clusterrolebinding mcp-viewer-crb
kubectl --context kind-gadget-dev delete serviceaccount mcp-viewer -n mcp
kubectl --context kind-gadget-dev delete namespace mcp
rm "$HOME/.kube/mcp-viewer.kubeconfig"
claude mcp remove kubernetes-mcp-server -s user
```

## SSH into VM

```bash
limactl shell gadget-k8s-host
```

## Teardown

```bash
# Delete VM
limactl delete gadget-k8s-host --force

# Remove kubeconfig entries (context, cluster, and user)
kubectl config delete-context kind-gadget-dev
kubectl config delete-cluster kind-gadget-dev
kubectl config delete-user kind-gadget-dev
```
