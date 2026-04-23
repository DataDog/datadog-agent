# Gadget K8s Host

Lima VM with a Kind cluster for local Kubernetes development.

## One-Time Setup

```bash
# Create VM and Kind cluster
./setup-k8s-host.sh

# Install Datadog Operator
limactl shell gadget-k8s-host -- helm install datadog-operator datadog/datadog-operator

# Create secret with API keys (set DD_API_KEY and DD_APP_KEY env vars first)
kubectl create secret generic datadog-secret \
  --from-literal=api-key="$DD_API_KEY" \
  --from-literal=app-key="$DD_APP_KEY" \
  --context kind-gadget-dev
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
