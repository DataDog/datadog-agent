# Kind

-----

[Kind](https://kind.sigs.k8s.io/) (Kubernetes in Docker) creates lightweight local Kubernetes clusters for development and testing.

## Prerequisites

- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [helm](https://helm.sh/docs/intro/install/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)

## Quick start

```bash
# Create cluster with agent
dda lab local kind --name dev --install-agent
```

## Create cluster

```bash
# Basic cluster (no agent)
dda lab local kind --name dev

# With agent installation
dda lab local kind --name dev --install-agent

# With specific Kubernetes version
dda lab local kind --name dev --k8s-version v1.30.0

# Recreate existing cluster
dda lab local kind --name dev --force
```

## Deploy agent

### From registry

```bash
# Default agent image
dda lab local kind --name dev --install-agent

# Custom image
dda lab local kind --name dev --install-agent --agent-image gcr.io/datadoghq/agent:7.50.0

# With custom Helm values
dda lab local kind --name dev --install-agent --helm-values ./values.yaml
```

### Build locally

Builds run inside a [developer environment](../env.md) to ensure proper build tooling.

/// note
The developer environment must be running before building. Start it with:
```bash
dda env dev start
```
///

```bash
# Build and deploy agent
dda lab local kind --name dev --build-agent

# Include additional components
dda lab local kind --name dev --build-agent \
    --with-process-agent \
    --with-trace-agent \
    --with-system-probe \
    --with-security-agent

# Use a specific developer environment
dda lab local kind --name dev --build-agent --devenv myenv
```

### Load existing image

```bash
# Load pre-built local image
dda lab local kind --name dev --load-image myagent:dev
```

## Working with the cluster

```bash
# Switch kubectl context
kubectl config use-context kind-dev

# Check agent pods
kubectl get pods -n datadog

# View agent status
kubectl exec -n datadog daemonset/datadog-agent -- agent status

# View agent logs
kubectl logs -n datadog -l app=datadog -f
```

## Options

| Option | Description |
|--------|-------------|
| `--name`, `-n` | Cluster name (required) |
| `--k8s-version` | Kubernetes version (default: v1.32.0) |
| `--install-agent` | Install Datadog Agent |
| `--agent-image` | Custom agent image |
| `--helm-values` | Path to Helm values.yaml |
| `--build-agent` | Build local agent image |
| `--load-image` | Load local Docker image |
| `--with-process-agent` | Include process-agent in build |
| `--with-trace-agent` | Include trace-agent in build |
| `--with-system-probe` | Include system-probe in build |
| `--with-security-agent` | Include security-agent in build |
| `--devenv` | Developer environment ID for building |
| `--force`, `-f` | Recreate cluster if exists |
