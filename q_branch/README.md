# Gadget K8s Host

Lima VM with a Kind cluster for local Kubernetes development.

## Setup

```bash
./setup-k8s-host.sh
```

This creates:
- Lima VM: `gadget-k8s-host`
- Kind cluster: `gadget-dev` (1 control-plane + 2 workers)
- Kubeconfig merged into `~/.kube/config`

## Usage

```bash
kubectx kind-gadget-dev
kubectl get nodes
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
