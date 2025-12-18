# Kind

-----

[Kind](https://kind.sigs.k8s.io/) (Kubernetes in Docker) creates lightweight local Kubernetes clusters for development and testing.

## Prerequisites

- Docker (or an equivalent container runtime)
- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [helm](https://helm.sh/docs/intro/install/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)

## Quick start

```bash
# Create a local Kind lab environment (installs the Agent by default)
dda lab local kind --id dev
```

## Create cluster

```bash
# Create a cluster without installing the Agent
dda lab local kind --id dev --no-agent

# With specific Kubernetes version
dda lab local kind --id dev --k8s-version v1.30.0

# Recreate existing cluster
dda lab local kind --id dev --force
```

## Deploy agent

By default, `dda lab local kind` installs the Datadog Agent via Helm. Provide an API key with `E2E_API_KEY` or `~/.test_infra_config.yaml` (see [Lab environments](index.md#configuration)).

### From registry

```bash
# Default agent image (Agent installation is enabled by default)
dda lab local kind --id dev

# Custom image
dda lab local kind --id dev --agent-image gcr.io/datadoghq/agent:7.50.0

# With custom Helm values
dda lab local kind --id dev --helm-values ./values.yaml
```

### Build locally (custom build command)

If you want to build a local Agent image using a custom command, pass `--build-command`.

/// note
The build runs inside a developer environment (see [Using developer environments](../env.md)). Ensure it is started first:

```bash
dda env dev start
```
///

```bash
# Example: build an image tagged datadog/agent-dev:local, then load+install it
dda lab local kind --id dev \
  --build-command "dda inv agent.hacky-dev-image-build --target-image datadog/agent-dev:local"
```

### Load existing image

```bash
# Load pre-built local image
dda lab local kind --id dev --load-image myagent:dev
```

## Working with the cluster

```bash
# Switch kubectl context
kubectl config use-context kind-<id>

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
| `--id`, `-i` | Environment id |
| `--k8s-version` | Kubernetes version (default: v1.32.0) |
| `--no-agent` | Do not install the Datadog Agent |
| `--agent-image` | Custom agent image |
| `--load-image` | Load existing local docker image into the cluster |
| `--helm-values` | Path to custom Helm values.yaml file |
| `--build-command` | Command to build the agent image (must output an image tagged `datadog/agent-dev:local`) |
| `--devenv` | Developer environment ID (see `dda env dev`) |
| `--force`, `-f` | Recreate cluster if exists |
| `--nodes-count` | Number of nodes in the cluster (default: 2) |
