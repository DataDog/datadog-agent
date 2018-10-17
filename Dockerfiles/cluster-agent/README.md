# Datadog Cluster Agent | Containerized environments

This is the technical documentation for the Datadog Cluster Agent image, available [here](https://hub.docker.com/r/datadog/cluster-agent/).

## Pre-requisites for the Datadog Cluster Agent

Review the RBAC files in [the manifests folder](../manifests/rbac) to get the full scope of the requirements.
These manifests create a Service Account, a Cluster Role with a restricted scope and actions detailed below and a Cluster Role Binding as well.

### API Server requirements

- `get`, `list` and `watch` of `Componenentstatuses` to produce the controle plane service checks.
- `get` and `update` of the `Configmaps` named `datadogtoken` to update and query the most up to date version token corresponding to the latest event stored in ETCD.
- `watch` the `Services` to perform the Autodiscovery based off of services activity.
- `get`, `list` and `watch` of the `Pods`.
- `get`, `list` and `watch`  of the `Nodes`.
- `get`, `list` and `watch`  of the `Services`.
- `get`, `list` and `watch`  of the `Events`.
- `get`, `list` and `watch`  of the `Endpoints` to run cluster level health checks.
- `watch` and `list` of the `horizontalpodautoscalers` to run the AutoscalersController and serve custom metrics.

To store the `event.tokenKey` and the `event.tokenTimestamp`, deploy your ConfigMap in the same namespace as the cluster agent with the name `datadogtoken`, namespace can be configured otherwise with `DD_KUBE_RESOURCES_NAMESPACE`.
For this, run `kubectl create configmap datadogtoken --from-literal="event.tokenKey"="0"` .
NB:Set any resversion here, make sure it's not set to a value superior to the actual current resversion.

If not present, set the `event.tokenTimestamp`, it is automatically set.

### Deploying the Datadog Cluster Agent

Run the Datadog Cluster Agent in Kubernetes using [this manifest](../manifests/cluster-agent/cluster-agent.yaml).
Refer to the [getting started guide](../../docs/cluster-agent/GETTING_STARTED.md) for more details.


### Communication with the Datadog Node Agent
<a name="communication-with-the-datadog-node-agent"></a>
For the Datadog Cluster Agent to communicate with the Node Agent, you need to share an authentication token between the two agents.
The token needs to be greater or equal to 32 characters and should only have upper case or lower case letters and numbers.
You can pass the token as an environment variable: `DD_CLUSTER_AGENT_AUTH_TOKEN`.
Besides this token, you need to set the `DD_CLUSTER_AGENT_ENABLED=true` in the manifest of the Datadog Node Agent.

## Running the Datadog Cluster Agent with Kubernetes

### Security premise
<a name="security-premise"></a>

We strongly recommend using a secret to authenticate communication between Agents with the Datadog Cluster Agent.
You must modify the value of the secret in [the dca-secret.yaml](/manifests/cluster-agent/dca-secret.yaml) then create it:

`kubectl create -f Dockerfiles/manifests/cluster-agent/dca-secret.yaml`

yields:

```
kubectl get secret datadog-auth-token
NAME                 TYPE      DATA      AGE
datadog-auth-token   Opaque    1         16s

```

## Migration path

If you are running the Datadog Node Agent 6.4.2+, to deploy the Datadog Cluster Agent you need to:
- Deploy [the datadog-cluster-agent_service.yaml](../../Dockerfiles/manifests/cluster-agent/datadog-cluster-agent_service.yaml)
- Configure and create [the dca-secret.yaml](../../Dockerfiles/manifests/cluster-agent/dca-secret.yaml)
- Configure (DD_API_KEY and other options) the [the cluster-agent.yaml](../../Dockerfiles/manifests/cluster-agent/cluster-agent.yaml) and deploy it
- Specify the [required options](#communication-with-the-datadog-node-agent) to ensure the communication between the Datadog Cluster Agent and the Datadog Node Agent.
- Apply the new configuration to the Datadog Node Agent DaemonSet and redeploy the DaemonSet.
- Once the Datadog Cluster Agent and the Datadog Node Agents are running, you can run the `agent status` command to confirm the successful communication.

Refer to the Datadog Cluster Agent [troubleshooting section](../../docs/cluster-agent/GETTING_STARTED.md#troubleshooting) for more information.

### Command line interface of the Datadog Cluster Agent

The available commands for the Datadog Cluster Agents are:
- `datadog-cluster-agent status`: Gives an overview of the components of the agent and their health.
- `datadog-cluster-agent metamap [nodeName]`: Queries the local cache of the mapping between the pods living on `nodeName`
    and the cluster level metadata it's associated with (endpoints ...).
    Not specifying the `nodeName` will run the mapper on all the nodes of the cluster.
- `datadog-cluster-agent flare [caseID]`: Similarly to the node agent, the cluster agent can aggregate the logs and the configurations used
    and forward an archive to the support team or be deflated and used locally.

## Enabling Features

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

Ensure the Node Agents and the Datadog Cluster Agent can properly communicate.
Create a [service](../../Dockerfiles/manifests/cluster-agent/datadog-cluster-agent_service.yaml) in front of the Datadog Cluster Agent.
Ensure an auth_token is properly shared between the agents.
Confirm the [RBAC rules](../../Dockerfiles/manifests/cluster-agent/rbac) are properly set.

In the Node Agent, set the env var `DD_CLUSTER_AGENT_ENABLED` to true.

The env var `DD_KUBERNETES_METADATA_TAG_UPDATE_FREQ` can be set to specify how often the Node Agents hit the Datadog Cluster Agent.

You can disable the Kubernetes metadata tag collection with `DD_KUBERNETES_COLLECT_METADATA_TAGS`.

#### Custom Metrics Server

The Datadog Cluster Agent implements the External Metrics Provider's interface (currently in beta).
Therefore it can serve Custom Metrics to Kubernetes for Horizontal Pod Autoscalers.
It is referred throughout the documentation as the Custom Metrics Server, per Kubernetes' terminology.

To enable the Custom Metrics Server:
- Set `DD_EXTERNAL_METRICS_PROVIDER_ENABLED` to `true` in the Deployment of the Datadog Cluster Agent.
- Configure the `<DD_APP_KEY>` as well as the `<DD_API_KEY>` in the Deployment of the Datadog Cluster Agent.
- Create a [service exposing the port 443](../../Dockerfiles/manifests/cluster-agent/hpa-example/cluster-agent-hpa-svc.yaml) and [register it as an APIService for External Metrics](../../Dockerfiles/manifests/cluster-agent/hpa-example/rbac-hpa.yaml).

Refer to [the dedicated guide](/docs/cluster-agent/CUSTOM_METRICS_SERVER.md) to configure the Custom Metrics Server and get more details about this feature.


## Options available

The following environment variables are supported:

- `DD_API_KEY` - **required** - your [Datadog API key](https://app.datadoghq.com/account/settings#api).
- `DD_HOSTNAME`: hostname to use for the Datadog Cluster Agent.
- `DD_CLUSTER_AGENT_CMD_PORT`: port for the Datadog Cluster Agent to serve, default is `5005`.
- `DD_USE_METADATA_MAPPER`: enables the cluster level metadata mapping, default is `true`.
- `DD_COLLECT_KUBERNETES_EVENTS` - configures the agent to collect Kubernetes events. Default to `false`. See the [Event collection section](#event-collection) for more details.
- `DD_LEADER_ELECTION`: activates the [leader election](../../Dockerfiles/agent#leader-election). You must set `DD_COLLECT_KUBERNETES_EVENTS` to `true` to activate this feature. Default value is `false`.
- `DD_LEADER_LEASE_DURATION`: used only if the leader election is activated. See the details [here](#leader-election-lease). Value in seconds, 60 by default.
- `DD_CLUSTER_AGENT_AUTH_TOKEN`: 32 characters long token that needs to be shared between the node agent and the Datadog Cluster Agent.
- `DD_KUBE_RESOURCES_NAMESPACE`: configures the namespace where the Cluster Agent creates the configmaps required for the Leader Election, the Event Collection (optional) and the Horizontal Pod Autoscaling.
- `DD_KUBERNETES_INFORMERS_RESYNC_PERIOD`: frequency in seconds to query the API Server to resync the local cache. The default is 5 minutes.
- `DD_KUBERNETES_INFORMERS_RESTCLIENT_TIMEOUT`: timeout in seconds of the client communicating with the API Server. Default is 60 seconds.
- `DD_OPENMETRICS_PORT`: change the port for exposing openmetrics metrics from the Datadog Cluster Agent. The default is port 5000.
- `DD_EXTERNAL_METRICS_BATCH_WINDOW`: time waited in seconds to process a batch of metrics from multiple Autoscalers. Default to 10 seconds.
- `DD_EXTERNAL_METRICS_MAX_AGE`: maximum age in seconds of a datapoint before considering it invalid to be served. Default to 90 seconds.
- `DD_EXTERNAL_METRICS_AGGREGATOR`: aggregator for the Datadog metrics. Applies to all Autoscalers processed. Chose among [sum/avg/max/min]
- `DD_EXTERNAL_METRICS_BUCKET_SIZE`: size of the window in seconds used to query metric from Datadog. Default to 300 seconds.
- `DD_EXTERNAL_METRICS_LOCAL_COPY_REFRESH_RATE`: rate to resync local cache of processed metrics with the global store. Useful when there are several replicas of the Cluster Agent.

## How to build it

### Containerized Agent

The Datadog Cluster Agent is designed to be used in a containerized ecosystem.

Start by creating the binary by running `inv -e cluster-agent.build` from the `datadog-agent` [package](../../../datadog-agent). This will add a binary in `./bin/datadog-cluster-agent/`
Then from the current folder, run `inv -e cluster-agent.image-build`.
