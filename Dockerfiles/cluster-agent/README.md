# Datadog Cluster Agent | Containerized environments

This is how the official Datadog Cluster Agent (also known as `DCA`) image, available [here](https://hub.docker.com/r/datadog/cluster-agent/), is built.

## How to run it

The following environment variables are supported:

- `DD_API_KEY` - **required** - your [Datadog API key](https://app.datadoghq.com/account/settings#api).
- `DD_HOSTNAME`: hostname to use for the DCA.
- `DD_CLUSTER_AGENT_CMD_PORT`: port for the DCA to serve, default is `5005`.
- `DD_USE_METADATA_MAPPER`: enables the cluster level metadata mapping, default is `true`.
- `DD_COLLECT_KUBERNETES_EVENTS` - configures the agent to collect Kubernetes events. Default to `false`. See the [Event collection section](#event-collection) for more details.
- `DD_LEADER_ELECTION`: activates the [leader election](#leader-election). You must set `DD_COLLECT_KUBERNETES_EVENTS` to `true` to activate this feature. Default value is `false`.
- `DD_LEADER_LEASE_DURATION`: used only if the leader election is activated. See the details [here](#leader-election-lease). The expected value is a number of seconds, is 60 by default.
- `DD_CLUSTER_AGENT_AUTH_TOKEN`: 32 characters long token that needs to be shared between the node agent and the DCA.
- `DD_KUBE_RESOURCES_NAMESPACE`: configures the namespace where the Cluster Agent creates the configmaps required for the Leader Election, the Event Collection (optional) and the Horizontal Pod Autoscaling.
- `DD_KUBERNETES_INFORMERS_RESYNC_PERIOD`: frequency in seconds to query the API Server to reprocess the cluster metadata. The default is 5 minutes.
- `DD_EXPVAR_PORT`: change the port for fetching [expvar](https://golang.org/pkg/expvar/) public variables from the Datadog Cluster Agent. The default is port 5000.

For a more detailed usage please [refer to the official Docker Hub](https://hub.docker.com/r/datadog/cluster-agent/) documentation.

## How to build it

### Dockerized Agent

The Datadog Cluster Agent is designed to be used in a containerized ecosystem.
Therefore, you need to have docker installed on your system.

Start by creating the binary by running `inv -e cluster-agent.build`. This will add a binary in `./bin/datadog-cluster-agent/`
Then from the current folder, run `inv -e cluster-agent.image-build`.

## Running the DCA with Kubernetes

### Security premise
<a name="security-premise"></a>

You should create the secret that is be used for your Agents to communicate with the DCA.
You must modify the value in [the dca-secret.yaml](/manifests/cluster-agent/dca-secret.yaml) then create it:

`kubectl create -f manifests/cluster-agent/dca-secret.yaml`

Will yield:

```
kubectl get secret datadog-auth-token
NAME                 TYPE      DATA      AGE
datadog-auth-token   Opaque    1         16s

```

### Spin up the DCA
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
          - name: DD_COLLECT_KUBERNETES_EVENTS
            value: "true"
```
And use the RBAC below to get the best out of it.

## Pre-requisites for the DCA to interact with the API server.

For the DCA to produce events, service checks and run checks one needs to enable it to perform a few actions.
Please find the minimum RBAC listed in [the manifests](/manifests/rbac) to get the full scope of features.
These manifests will create a Service Account, a Cluster Role with a restricted scope and actions detailed below and a Cluster Role Binding as well.

### The DCA needs:

- `get`, `list` and `watch` of `Componenentstatuses` to produce the controle plane service checks.
- `get` and `update` of the `Configmaps` named `eventtokendca` to update and query the most up to date version token corresponding to the latest event stored in ETCD.
- `watch` the `Services` to perform the Autodiscovery based off of services activity
- `get`, `list` and `watch` of the `Pods`
- `get`, `list` and `watch`  of the `Nodes`
- `get`, `list` and `watch`  of the `Endpoints` to run cluster level health checks.

The ConfigMap to store the `event.tokenKey` and the `event.tokenTimestamp` has to be deployed in the `default` namespace and be named `configmapdcatoken`
One can simply run `kubectl create configmap configmapdcatoken --from-literal="event.tokenKey"="0"` .
NB: you can set any resversion here, make sure it's not set to a value superior to the actual curent resversion.

You can also set the `event.tokenTimestamp`, if not present, it will be automatically set.

### Command line interface of the Cluster Agent

The available commands for the Cluster Agents are:
- `datadog-cluster-agent status`: This will give you an overview of the components of the agent and their health.
- `datadog-cluster-agent metamap [nodeName]`: Will query the local cache of the mapping between the pods living on `nodeName`
    and the cluster level metadata it's associated with (endpoints ...).
    One can also not specify the `nodeName` to run the mapper on all the nodes of the cluster.
- `datadog-cluster-agent flare [caseID]`: Similarly to the node agent, the cluster agent can aggregate the logs and the configurations used
    and forward an archive to the support team or be deflated and used locally.


### Communication with the Datadog Node Agent.

For the DCA to communicate with the Node Agent, you need to share an authentication token between the two agents.
The Token needs to be longer than 32 characters and should only have upper case or lower case letters and numbers.
You can pass the token as an environment variable: `DD_CLUSTER_AGENT_AUTH_TOKEN`.

### Enabling Features

#### Event collection

In order to collect events, you need the following environment variables:
```
          - name: DD_COLLECT_KUBERNETES_EVENTS
            value: "true"
          - name: DD_LEADER_ELECTION
            value: "true"
```
Enabling the leader election will ensure that only one agent collects the events.

#### Cluster metadata provider

You need to ensure the Node Agents and the DCA can properly communicate.
Create a service in front of the DCA (see /manifests/datadog-cluster-agent_service.yaml)
Ensure an auth_token is properly shared between the agents.
Confirm the RBAC rules are properly set (see /manifests/rbac/).

In the Node Agent, make sure the `DD_CLUSTER_AGENT` env var is set to true.
The env var `DD_KUBERNETES_METADATA_TAG_UPDATE_FREQ` can be set to specify how often the Node Agents hit the DCA.
You can disable the kubernetes metadata tag collection with `DD_KUBERNETES_COLLECT_METADATA_TAGS`.

#### HPA

To enable the HPA:
- Set `DD_EXTERNAL_METRICS_PROVIDER_ENABLED` to `true` in the Deployment of the DCA.
- Configure the `<DD_APP_KEY>` as well as the `<DD_API_KEY>` in the Deployment of the DCA.
- Create a service exposing the port 443 and register it as an APIService for External Metrics.

Refer to [the dedicated guide](/docs/cluster-agent/HORIZONTAL_POD_AUTOSCALING.md) to configure the HPA and get more details about this feature.
