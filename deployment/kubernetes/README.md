# Kubernetes Setup

## Cluster provisioning

If you want to create a cluster using AWS EKS please follow [this Readme](aws-eks/tf-cluster/README.md) on how to spin that up with Terraform.

## Agent deployment

### Configure RBAC

If your Kubernetes has role-based access control (RBAC) enabled, configure RBAC permissions for your StackState Agent service account.

Create the appropriate ClusterRole, ServiceAccount, and ClusterRoleBinding:

    kubectl apply -f rbac/agent.yaml

### Enable Kubernetes state

To gather your kube-state metrics:
* Download the [Kube-State manifests folder](https://github.com/kubernetes/kube-state-metrics/tree/master/kubernetes)
* Apply them to your Kubernetes cluster:


    kubectl apply -f <NAME_OF_THE_KUBE_STATE_MANIFESTS_FOLDER>

### Creating configmap

```
kubectl create configmap sts-agent-config --from-file=agents-config-map.yaml
```

List current setup

```
kubectl describe configmaps sts-agent-config
```

If you ever needed to delete existing configmap

```
kubectl delete configmap sts-agent-config
```

### Deploying with parameters customization using kubernetes native "kustomize" (kubectl 1.14+)

`kubectl kustomize` ( https://kustomize.io/ ) sub command is used to tune the customization.

Create dedicated configuration directory which contains necessary overrides over default variables, named `kustomization.yaml`
As a minimum, overrides include

* `<STS_API_KEY>` with your StackState backend URL
* `<STS_STS_URL>` with your StackState backend URL (including suffix /stsAgent)
* `<STS_CLUSTER_NAME>` with your Cluster name

See example in folder `deployment/kubernetes/k8s/overlays/example/kustomization.yaml`,

command to take a look on modified definition: `kubectl kustomize <CUSTOMIZATION DIRECTORY>`

Example:

```sh
kubectl kustomize k8s/overlays/example/
```

You can deploy final definition into kubernetes cluster `kubectl apply -k <CUSTOMIZATION DIRECTORY>`

```sh
kubectl apply -k k8s/overlays/example/
```

### Deploy the DaemonSet

Before deploying the agent there are few configuration settings to take care of, open the `agent.yaml` and:

* replace `<STS_API_KEY>` with your StackState backend URL
* replace `<STACKSTATE_BACKEND_URL>` with your StackState backend URL

Now you can deploy the DaemonSet with the following command:


    kubectl apply -f agent.yaml

## Cluster Agent deployment

To enable collection of cluster level information deploy the Cluster Agent

Create the appropriate ClusterRole, ServiceAccount, and ClusterRoleBinding:

    kubectl apply -f rbac/cluster-agent.yaml

Create the shared token to allow Agent -> Cluster Agent communication:

    kubectl apply -f cluster-auth-token.yaml

Before deploying the cluster agent there are few configuration settings to take care of, open the `cluster-agent.yaml` and:

* replace `<STS_API_KEY>` with your StackState backend URL
* replace `<STACKSTATE_BACKEND_URL>` with your StackState backend URL
* replace `<STS_CLUSTER_NAME>` with your Cluster name

Uncomment the `STS_CLUSTER_AGENT_ENABLED` and `STS_CLUSTER_AGENT_AUTH_TOKEN` variables in `agent.yaml` and re-deploy the [main agent](#agent-deployment)
