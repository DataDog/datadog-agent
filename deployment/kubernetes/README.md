# Kubernetes Setup

## AWS EKS provisioning

If you want to create a cluster using AWS EKS please follow [this Readme](aws-eks/tf-cluster/README.md) on how to spin that up with Terraform.

## Agent deployment

### Enable Kubernetes state

To gather your kube-state metrics:
* Download the [Kube-State manifests folder](https://github.com/kubernetes/kube-state-metrics/tree/master/kubernetes)
* Apply them to your Kubernetes cluster:


    kubectl apply -f <NAME_OF_THE_KUBE_STATE_MANIFESTS_FOLDER>

### Deploying with kustomize (kubectl 1.14+)

`kubectl kustomize` ( https://kustomize.io/ ) sub command is used to tune the customization.

Create dedicated configuration directory which contains necessary overrides over default variables, named `kustomization.yaml`
As a minimum, overrides include

* `<STS_API_KEY>` with your StackState backend URL
* `<STS_STS_URL>` with your StackState backend URL (including suffix /stsAgent)
* `<STS_CLUSTER_NAME>` with your Cluster name

See example in folder `deployment/kubernetes/agents/overlays/example/kustomization.yaml`,

command to take a look on modified definition: `kubectl kustomize <CUSTOMIZATION DIRECTORY>`

Example:

```sh
kubectl kustomize agents/overlays/example/
```

You can deploy final definition into kubernetes cluster `kubectl apply -k <CUSTOMIZATION DIRECTORY>`

```sh
kubectl apply -k agents/overlays/example/
```
