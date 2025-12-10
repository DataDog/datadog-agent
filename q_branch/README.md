# Gadget Development Environment

Local Kubernetes development environment with auto-detection:
- **VM mode**: Lima VM + Kind cluster (macOS or Linux with KVM)
- **Direct mode**: Kind cluster on host Docker (Linux without KVM)

## Prerequisites

- **VM mode**: [Lima](https://lima-vm.io/) installed
- **Direct mode**: Docker installed and running
- Python 3.9+ (for `gadget-dev.py`)

## Quick Start

```bash
# Check status
./gadget-dev.py

# Create environment (automatically sets up Helm repo if Helm is installed)
./gadget-dev.py start

# Install Datadog Operator (VM mode)
limactl shell gadget-k8s-host -- helm install datadog-operator datadog/datadog-operator

# Install Datadog Operator (Direct mode)
helm install datadog-operator datadog/datadog-operator
```

This creates:
- Kind cluster: `gadget-dev` (1 control-plane + 2 workers)
- Kubeconfig merged into `~/.kube/config`
- Context: `kind-gadget-dev`
- Datadog Helm repository configured (if Helm installed)
- In VM mode: Lima VM named `gadget-k8s-host`

## Available Commands

```bash
./gadget-dev.py              # Show status (default)
./gadget-dev.py status       # Show environment status
./gadget-dev.py start        # Create/start environment
./gadget-dev.py start --recreate  # Force recreate cluster
./gadget-dev.py stop         # Stop environment (VM mode only)
./gadget-dev.py delete       # Delete environment
./gadget-dev.py load-image <image>  # Load Docker image into cluster
./gadget-dev.py deploy [yaml]       # Deploy manifest and restart pods
```

## Dev Loop

### Simple Workflow (using gadget-dev.py)

```bash
# 1. Build image
dda inv omnibus.docker-build

# 2. Load image into cluster
./gadget-dev.py load-image localhost/datadog-agent:local

# 3. Deploy and restart
./gadget-dev.py deploy

# 4. Watch pods come up
kubectl get pods -w --context kind-gadget-dev
```

### Command Comparison Table

| Task | Using gadget-dev.py | Direct Mode (Manual) | VM Mode (Manual) |
|------|---------------------|----------------------|------------------|
| **Setup** | `./gadget-dev.py start` | `kind create cluster --config ...` | `limactl start gadget-k8s-host` (after initial setup) |
| **Load image** | `./gadget-dev.py load-image localhost/datadog-agent:local` | `kind load docker-image localhost/datadog-agent:local --name gadget-dev` | `docker save localhost/datadog-agent:local \| limactl shell gadget-k8s-host -- docker load && limactl shell gadget-k8s-host -- kind load docker-image localhost/datadog-agent:local --name gadget-dev` |
| **Deploy + restart** | `./gadget-dev.py deploy` | `kubectl apply -f test-cluster.yaml --context kind-gadget-dev && kubectl delete pods -l app.kubernetes.io/name=datadog-agent-deployment -n default --context kind-gadget-dev` | Same as direct mode |
| **Check status** | `./gadget-dev.py status` | `kind get clusters && kubectl get nodes --context kind-gadget-dev` | `limactl list && kind get clusters` (inside VM) |
| **Teardown** | `./gadget-dev.py delete` | `kind delete cluster --name gadget-dev` | `limactl delete gadget-k8s-host --force` |

## SSH into VM

**VM mode only:**

```bash
limactl shell gadget-k8s-host
```

In direct mode, you're already on the host - no SSH needed!

## Lifecycle Management

```bash
# Check status
./gadget-dev.py status

# Stop (VM mode only - pauses VM, cluster remains)
./gadget-dev.py stop

# Start (resumes stopped environment or creates if needed)
./gadget-dev.py start

# Delete everything (removes VM + cluster or just cluster)
./gadget-dev.py delete

# Force delete without confirmation
./gadget-dev.py --force delete
```

## Troubleshooting

**Check what mode you're in:**
```bash
./gadget-dev.py status
```

**Recreate cluster without deleting VM:**
```bash
./gadget-dev.py start --recreate
```

**Switch contexts:**
```bash
kubectl config use-context kind-gadget-dev
# or
kubectx kind-gadget-dev
```
