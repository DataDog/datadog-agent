# Kubernetes cluster creation

If you want to create a cluster using AWS EKS please follow the `manifests/aws/tf/Readme.md` on how to spin that up with Terraform.

# Kubernetes DaemonSet Setup

This setup will tell you how to install StackState Agent 6 using daemonset in Kubernetes Cluster.

## Configure RBAC permissions 

If your Kubernetes has role-based access control (RBAC) enabled, configure RBAC permissions for your StackState Agent service account.  

Create the appropriate ClusterRole, ServiceAccount, and ClusterRoleBinding:

```
kubectl create -f stackstate-serviceaccount.yaml
```

## Create manifest

Replace `<STACKSTATE_BACKEND_IP>` with your Stackstate backend IP inside the `stackstate-agent.yaml`.

Now you can deploy the DaemonSet with the following command:

```
kubectl create -f stackstate-agent.yaml
```

## Configuration SetUp

* To enable `process and containers`, set the env variable `DD_PROCESS_AGENT_ENABLED` to `true` and data will be sent to `/api/v1/collector` endpoint.
* To enable only `containers`, don't set the env variable `DD_PROCESS_AGENT_ENABLED` and the data will be sent to `/api/v1/container` endpoint.
* To enable `connections`, set the env variable `DD_CONNECTIONS_CHECK` to `true` and make sure `DD_PROCESS_AGENT_ENABLED` is `true` as well.
* To override the endpoint for `process-agent`, set the env variable `DD_PROCESS_AGENT_URL` with custom one.
* To override the endpoint for stackstate backend, set the env variable `DD_DD_URL` with custom one.

# Kubernetes Host Setup

Installing the Agent directly on your host (rather than having the Agent run in a Pod) provides additional visibility into your ecosystem, independent of Kubernetes.
To gather your kube-state metrics:
* Download the [Kube-State manifests folder](https://github.com/kubernetes/kube-state-metrics/tree/master/kubernetes)
* Apply them to your Kubernetes cluster:

```
kubectl apply -f <NAME_OF_THE_KUBE_STATE_MANIFESTS_FOLDER>
```
