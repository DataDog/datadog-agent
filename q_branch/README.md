# Gadget K8s Host

Lima VM with a Kind cluster for local Kubernetes development.

## One-Time Setup

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
# 1. Build custom agent image
dda inv omnibus.build-image

# 2. Load image into Kind (macOS docker → Lima docker → Kind)
docker tag datadog-agent:local localhost/datadog-agent:local
docker save localhost/datadog-agent:local | limactl shell gadget-k8s-host -- docker load
limactl shell gadget-k8s-host -- kind load docker-image localhost/datadog-agent:local --name gadget-dev

# 3. Deploy agent with custom image
kubectl apply -f test-cluster.yaml --context kind-gadget-dev

# 4. Watch pods come up
kubectl get pods -w --context kind-gadget-dev
```

To redeploy after code changes, repeat steps 1-3.

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
