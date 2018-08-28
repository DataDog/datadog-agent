# Kubernetes DaemonSet Setup

This setup will tell you how to install StackState Agent 6 using daemonset in Kubernetes Cluster.

## Configure RBAC permissions 

If your Kubernetes has role-based access control (RBAC) enabled, configure RBAC permissions for your StackState Agent service account.  

Create the appropriate ClusterRole, ServiceAccount, and ClusterRoleBinding:

```
kubectl create -f stackstate-serviceaccount.yaml
```

## Create manifest

Replace <YOUR_API_KEY> with your Stackstate API key or use Kubernetes secrets to set your API key as an environment variable inside the `stackstate-agent.yaml`.

Deploy the DaemonSet with the command:

```
kubectl create -f stackstate-agent.yaml
```

# Kubernetes Host Setup

Installing the Agent directly on your host (rather than having the Agent run in a Pod) provides additional visibility into your ecosystem, independent of Kubernetes.
To gather your kube-state metrics:
* Download the [Kube-State manifests folder](https://github.com/kubernetes/kube-state-metrics/tree/master/kubernetes)
* Apply them to your Kubernetes cluster:
  ```
  kubectl apply -f <NAME_OF_THE_KUBE_STATE_MANIFESTS_FOLDER>
  ```
