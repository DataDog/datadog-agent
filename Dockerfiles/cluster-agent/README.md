# Cluster Agent 6 docker image

This is how the official Datadog Cluster Agent 6 (aka DCA) image available [here](https://hub.docker.com/r/datadog/cluster-agent/) is built.

## How to run it

The following environment variables are supported:

- `DD_API_KEY`: your API key (**required**)
- `DD_HOSTNAME`: hostname to use for metrics
- 'DD_CMD_PORT': Port you want the DCA to serve

Example usage: `docker run -e DD_API_KEY=your-api-key-here -e CMD_PORT=1234 -it <image-name>`

For a more detailed usage please refer to the official [Docker Hub](https://hub.docker.com/r/datadog/cluster-agent/)

## How to build it

### On debian-based systems

You can build your own debian package using `inv cluster-agent.omnibus-build`

Then you can call `inv cluster-agent.image-build` that will take the debian package generated above and use it to build the image

### On other systems

To build the image you'll need the cluster-agent debian package that will soon be on our apt/yum repos. In the meantime, you can use the omnibus-build command listed above.

You'll need to download one of the `datadog-cluster-agent*_amd64.deb` package in this directory, it will then be used by the `Dockerfile` and installed within the image.

You can then build the image using `docker build -t datadog/cluster-agent:master .`

If you are on macOS, use the --skip-sign option on the omnibus-build.

## Running the DCA with Kubernetes

To run the DCA in Kubernetes, you can simply run `kubectl create -f dca_deploy.yaml` and use the following manifest

```
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: dca
spec:
  template:
    metadata:
      labels:
        app: dca
      name: dca
      namespace: default
    spec:
      serviceAccountName: dca
      containers:
      - image: datadog/cluster-agent
        imagePullPolicy: Always
        name: dca
        env:
          - name: DD_API_KEY
            value: XXXX
```
And use the RBAC below to get the best out of it.

## Pre-requisites for the DCA to interact with the API server.

For the DCA to produce events, service checks and run checks one needs to enable it to perform a few actions.
Please find the minimum RBAC below to get the full scope of features.
This manifest will create a Service Account, a Cluster Role with a restricted scope and actions detailed below and a Cluster Role Binding as well.

### The DCA needs:

- `get`, `list` and `watch` of `Componenentstatuses` to produce the controle plane service checks.
- `get` and `update` of the `Configmaps` named `eventtokendca` to update and query the most up to date version token corresponding to the latest event stored in ETCD.
- `watch` the `Services` to perform the Autodiscovery based off of services activity
- `get`, `list` and `watch` of the `Pods`
- `get`, `list` and `watch`  of the `Nodes`
- `get`, `list` and `watch`  of the `Endpoints` to run cluster level health checks.


```
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: datadog-dca
rules:
- apiGroups:
  - ""
  resources:
  - services
  - events
  - endpoints
  - pods
  - nodes
  - componentstatuses
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - configmaps
  resourceNames:
  - configmapdcatoken
  verbs:
  - get
  - update
---
kind: ServiceAccount
apiVersion: v1
metadata:
  name: datadog-dca
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: datadog-dca
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: datadog-dca
subjects:
- kind: ServiceAccount
  name: datadog-dca
  namespace: default
---
```

The ConfigMap to store the `event.tokenKey` and the `event.tokenTimestamp` has to be deployed in the `default` namespace and be named `configmapdcatoken`
One can simply run `kubectl create configmap configmapdcatoken --from-literal="event.tokenKey"="0"` .
NB: you can set any resversion here, make sure it's not set to a value superior to the actual curent resversion.

You can also set the `event.tokenTimestamp`, if not present, it will be automatically set.


