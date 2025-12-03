# Gadget K8s Host

Lima VM with a Kind cluster for local Kubernetes development.

## Setup

```bash
./setup-k8s-host.sh
```

This creates:
- Lima VM: `gadget-k8s-host`
- Kind cluster: `gadget-dev` (1 control-plane + 2 workers)
- Kubeconfig: `~/.kube/gadget-k8s-host.yaml`

## Usage

```bash
export KUBECONFIG=~/.kube/gadget-k8s-host.yaml
kubectl get nodes
```

## SSH into VM

```bash
limactl shell gadget-k8s-host
```
