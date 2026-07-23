# AWS EKS

Provisions a managed EKS cluster with the Datadog Agent deployed as a
DaemonSet via Helm. Use this to test Kubernetes-specific agent behavior,
the Cluster Agent, or Kubernetes integrations.

/// admonition | Provisioning time
    type: note

EKS clusters take approximately 20 minutes to create.
///

## Prerequisites

EKS requires both an API key and an **app key** in `~/.test_infra_config.yaml`
(or via `E2E_APP_KEY`).

## Create

```bash
dda inv aws.create-eks
```

### Key options

| Flag | Default | Description |
|------|---------|-------------|
| `--stack-name` | `aws-eks` | Suffix for the Pulumi stack name |
| `--linux-node-group` | `true` | Include a Linux (x86_64) node group |
| `--linux-arm-node-group` | `false` | Include a Linux ARM node group |
| `--bottlerocket-node-group` | `true` | Include a Bottlerocket node group |
| `--windows-node-group` | `false` | Include a Windows node group |
| `--gpu-node-group` | `false` | Include a GPU node group (disables all other node groups) |
| `--instance-type` | auto | EC2 instance type for cluster nodes |
| `--agent-version` | latest | Container image tag (e.g. `7.58.0-rc.3`) |
| `--full-image-path` | — | Full registry path to a custom agent image |
| `--cluster-agent-full-image-path` | — | Full registry path to a custom Cluster Agent image |
| `--helm-config` | — | Path to a custom Helm values file to merge with defaults |
| `--local-chart-path` | — | Path to a local Helm chart |
| `--kube-version` | latest | Kubernetes version (e.g. `1.31`) |
| `--agent-flavor` | — | Agent flavor (e.g. `datadog-fips-agent`) |
| `--no-interactive` | — | Disable clipboard prompt and desktop notification |

### Examples

```bash
# EKS cluster with Windows nodes
dda inv aws.create-eks --windows-node-group=true

# EKS with a specific Kubernetes version and custom Helm values
dda inv aws.create-eks --kube-version=1.31 --helm-config=./my-values.yaml

# GPU-only cluster
dda inv aws.create-eks --gpu-node-group=true
```

## Connect

After the cluster is ready, `kubectl` is configured automatically.
The task prints the cluster context name to use:

```bash
kubectl config use-context <context-name>
kubectl get nodes
```

## Destroy

```bash
dda inv aws.destroy-eks
```

## Limitations

- GPU node groups are x86_64 only; enabling `--gpu-node-group` disables all
  other node groups automatically.
- FakeIntake is not supported for EKS.
