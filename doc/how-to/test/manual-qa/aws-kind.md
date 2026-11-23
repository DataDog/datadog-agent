# AWS KinD

Provisions an EC2 instance running KinD (Kubernetes in Docker) with the
Datadog Agent deployed via Helm. A lighter-weight alternative to EKS when you
need a Kubernetes environment but don't need a managed cluster.

## Prerequisites

KinD requires both an API key and an **app key** in `~/.test_infra_config.yaml`
(or via `E2E_APP_KEY`).

## Create

```bash
dda inv aws.create-kind
```

### Key options

| Flag | Default | Description |
|------|---------|-------------|
| `--stack-name` | `aws-kind` | Suffix for the Pulumi stack name |
| `--architecture` | `x86_64` | CPU architecture: `x86_64` or `arm64` |
| `--agent-version` | latest | Container image tag |
| `--full-image-path` | — | Full registry path to a custom agent image |
| `--cluster-agent-full-image-path` | — | Full registry path to a custom Cluster Agent image |
| `--install-agent-with-operator` | `false` | Deploy the agent via the Datadog Operator instead of Helm |
| `--helm-config` | — | Path to a custom Helm values file to merge with defaults |
| `--kube-version` | latest | Kubernetes version (e.g. `1.31`) |
| `--use-fakeintake` | `false` | Deploy a local mock intake alongside the agent |
| `--agent-flavor` | — | Agent flavor (e.g. `datadog-fips-agent`) |

### Examples

```bash
# KinD cluster on an ARM host
dda inv aws.create-kind --architecture=arm64

# KinD with a specific Kubernetes version
dda inv aws.create-kind --kube-version=1.31

# Deploy agent via Operator instead of Helm
dda inv aws.create-kind --install-agent-with-operator=true
```

## Connect

SSH connection details for the EC2 host are printed after creation. Once
connected, `kubectl` is available on the host and the KinD cluster context is
already configured:

```bash
# From inside the EC2 host
kubectl get nodes
kubectl get pods -n datadog
```

## Destroy

```bash
dda inv aws.destroy-kind
```

## Limitations

- Single-node cluster — not suitable for testing multi-node Kubernetes behavior.
- The EC2 instance type is fixed at `t3.xlarge`; this cannot be overridden.
