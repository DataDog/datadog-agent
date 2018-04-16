# Cluster Agent 6 docker image

This is how the official Datadog Cluster Agent (also known as `DCA`) image, available [here](https://hub.docker.com/r/datadog/cluster-agent/), is built.

## How to run it

The following environment variables are supported:

- `DD_API_KEY`: your API key (**required**)
- `DD_HOSTNAME`: hostname to use for the DCA.
- `DD_CMD_PORT`: Port you want the DCA to serve

For a more detailed usage please refer to the official [Docker Hub](https://hub.docker.com/r/datadog/cluster-agent/)

## How to build it

### Dockerized Agent

The Datadog Cluster Agent is designed to be used in a containerized ecosystem.
Therefore, you will need to have docker installed on your system.

Start by creating the binary by running `inv -e cluster-agent.build`. This will add a binary in `./bin/datadog-cluster-agent/`
Then from the current folder, run `inv -e cluster-agent.image-build`.


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
      serviceAccountName: datadog-dca
      containers:
      - image: datadog/cluster-agent
        imagePullPolicy: Always
        name: dca
        env:
          - name: DD_API_KEY
            value: XXXX
          - name: DD_CLUSTER_AGENT_AUTH_TOKEN
            value: <32 characters long string shared with node agent>
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

### Communication with the Datadog Node Agent.

For the DCA to communicate with the Node Agent, you need to share an authentication token between the two agents.
The Token needs to be longer than 32 characters and should only have upper case or lower case letters and numbers.
You can pass the token as an environment variable: `DD_CLUSTER_AGENT_AUTH_TOKEN`.

### Enabling Features

#### Event collection

In order to collect events, you need the following environment varibales:
```
          - name: DD_COLLECT_KUBERNETES_EVENTS
            value: "true"
          - name: DD_LEADER_ELECTION
            value: "true"
```
Enabling the leader election will ensure that only one agent collects the events.

#### Cluster metadata provider

You need to ensure the Node agents and the DCA can properly communicate.
Create a service in front of the DCA (see /manifests/datadog-cluster-agent_service.yaml)
Ensure an auth_token is properly shared between the agents.
Confirm the RBAC rules are properly set (see /manifests/rbac/).

In the Node Agent, make sure the `DD_CLUSTER_AGENT` env var is set to true.
The env var `DD_KUBERNETES_METADATA_TAG_UPDATE_FREQ` can be set to specify how often the node agents hit the DCA.
You can disable the kubernetes metadata tag collection with `DD_KUBERNETES_COLLECT_METADATA_TAGS`.

