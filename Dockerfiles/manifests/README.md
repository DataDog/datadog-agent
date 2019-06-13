# Kubernetes Setup

### Cluster creation

If you want to create a cluster using AWS EKS please follow [this Readme](aws-eks/tf-cluster/README.md) on how to spin that up with Terraform.

### Configure RBAC 

If your Kubernetes has role-based access control (RBAC) enabled, configure RBAC permissions for your StackState Agent service account.  

Create the appropriate ClusterRole, ServiceAccount, and ClusterRoleBinding:

```
kubectl apply -f stackstate-serviceaccount.yaml
```

### Enable Kubernetes state

To gather your kube-state metrics:
* Download the [Kube-State manifests folder](https://github.com/kubernetes/kube-state-metrics/tree/master/kubernetes)
* Apply them to your Kubernetes cluster:

```
kubectl apply -f <NAME_OF_THE_KUBE_STATE_MANIFESTS_FOLDER>
```

## Deploy the DaemonSet

Before deploying the agent there are few configuration settings to take care of, open the `stackstate-agent.yaml` and:

* replace `<STACKSTATE_BACKEND_URL>` with your StackState backend URL

Now you can deploy the DaemonSet with the following command:

```
kubectl apply -f stackstate-agent.yaml
```
