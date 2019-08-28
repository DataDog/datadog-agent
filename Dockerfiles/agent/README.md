# Agent 6 docker image

This is how the official agent 6 image available [here](https://hub.docker.com/r/datadog/agent/) is built.

## How to run it

Head over to [datadoghq.com](https://app.datadoghq.com/account/settings#agent/docker) to get the official installation guide.

For a simple docker run, you can quickly get started with:

```shell
docker run -d -v /var/run/docker.sock:/var/run/docker.sock:ro \
              -v /proc/:/host/proc/:ro \
              -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro \
              -e DD_API_KEY=<YOUR_API_KEY> \
              datadog/agent:latest
```

### Environment variables

The agent is highly customizable, here are the most used environment variables:

#### Global options

- `DD_API_KEY`: your API key (**required**)
- `DD_HOSTNAME`: hostname to use for metrics (if autodetection fails)
- `DD_TAGS`: host tags, separated by spaces. For example: `simple-tag-0 tag-key-1:tag-value-1`
- `DD_CHECK_RUNNERS`: the agent runs all checks in sequence by default (default value = `1` runner). If you need to run a high number of checks (or slow checks) the `collector-queue` component might fall behind and fail the healthcheck. You can increase the number of runners to run checks in parallel

#### Proxies

Starting with Agent v6.4.0, the agent proxy settings can be overridden with the following
environment variables:

- `DD_PROXY_HTTP`: an http URL to use as a proxy for `http` requests.
- `DD_PROXY_HTTPS`: an http URL to use as a proxy for `https` requests.
- `DD_PROXY_NO_PROXY`: a space-separated list of URLs for which no proxy should be used.

Note: at the moment, the trace agent only supports the above proxy environment variables starting from version 6.5.0

For more information: https://docs.datadoghq.com/agent/proxy/#agent-v6

#### Optional collection agents

These features are disabled by default for security or performance reasons, you need to explicitly enable them:

- `DD_APM_ENABLED`: run the trace-agent along with the infrastructure agent, allowing the container to accept traces on 8126/tcp
- `DD_LOGS_ENABLED`: run the [log-agent](https://docs.datadoghq.com/logs/) along with the infrastructure agent. [See below for details](#log-collection)
- `DD_PROCESS_AGENT_ENABLED`: enable live process collection in the [process-agent](https://docs.datadoghq.com/graphing/infrastructure/process/). The Live Container View is already enabled by default if the Docker socket is available

#### Dogstatsd (custom metrics)

Send custom metrics via [the statsd protocol](https://docs.datadoghq.com/developers/dogstatsd/):

- `DD_DOGSTATSD_NON_LOCAL_TRAFFIC`: listen to dogstatsd packets from other containers, required to send custom metrics
- `DD_HISTOGRAM_PERCENTILES`: histogram percentiles to compute, separated by spaces. The default is "0.95"
- `DD_HISTOGRAM_AGGREGATES`: histogram aggregates to compute, separated by spaces. The default is "max median avg count"
- `DD_DOGSTATSD_SOCKET`: path to the unix socket to listen to. Must be in a `rw` mounted volume.
- `DD_DOGSTATSD_ORIGIN_DETECTION`: enable container detection and tagging for unix socket metrics. Running in host PID mode (e.g. with --pid=host) is required.

#### Tagging

We automatically collect common tags from [Docker](https://github.com/DataDog/datadog-agent/blob/master/pkg/tagger/collectors/docker_extract.go), [Kubernetes](https://github.com/DataDog/datadog-agent/blob/master/pkg/tagger/collectors/kubelet_extract.go), [ECS](https://github.com/DataDog/datadog-agent/blob/master/pkg/tagger/collectors/ecs_extract.go), [Swarm, Mesos, Nomad and Rancher](https://github.com/DataDog/datadog-agent/blob/master/pkg/tagger/collectors/docker_extract.go), and allow you to extract even more tags with the following options:

- `DD_DOCKER_LABELS_AS_TAGS` : extract docker container labels
- `DD_DOCKER_ENV_AS_TAGS` : extract docker container environment variables
- `DD_KUBERNETES_POD_LABELS_AS_TAGS` : extract pod labels
- `DD_KUBERNETES_POD_ANNOTATIONS_AS_TAGS` : extract pod annotations

You can either define them in your custom `datadog.yaml`, or set them as JSON maps in these envvars. The map key is the source (label/envvar) name, and the map value the Datadog tag name.

```shell
DD_KUBERNETES_POD_LABELS_AS_TAGS='{"app":"kube_app","release":"helm_release"}'
DD_DOCKER_LABELS_AS_TAGS='{"com.docker.compose.service":"service_name"}'
```

You can use shell patterns in label names to define simple rules for mapping labels to Datadog tag names using the same simple template system used by Autodiscovery. This is only supported by `DD_KUBERNETES_POD_LABELS_AS_TAGS`.

To add all pod labels as tags to your metrics where tags names are prefixed by `kube_`, you can use the following:

```shell
DD_KUBERNETES_POD_LABELS_AS_TAGS='{"*":"kube_%%label%%"}'
```

To add only pod labels as tags to your metrics that start with `app`, you can use the following:

```shell
DD_KUBERNETES_POD_LABELS_AS_TAGS='{"app*":"kube_%%label%%"}'
```

#### Using secret files (BETA)

Integration credentials can be stored in Docker / Kubernetes secrets and used in Autodiscovery templates. See the [setup instructions for the helper script](secrets-helper/README.md) and the [agent documentation](https://github.com/DataDog/datadog-agent/blob/6.4.x/docs/agent/secrets.md) for more information.

#### Ignore containers

You can exclude containers from the metrics collection and autodiscovery, if these are not useful for you. We already exclude Kubernetes and OpenShift `pause` containers by default. See the `datadog.yaml.example` file for more documentation, and examples.
- `DD_AC_INCLUDE`: whitelist of containers to always include
- `DD_AC_EXCLUDE`: blacklist of containers to exclude

**The format for these option is space-separated strings**. For example, if you only want to monitor two images, and exclude the rest, specify:

```
DD_AC_EXCLUDE = "image:.*"
DD_AC_INCLUDE = "image:cp-kafka image:k8szk"
```

Please note that the `docker.containers.running`, `.stopped`, `.running.total` and `.stopped.total` metrics are not affected by these settings and always count all containers. This does not affect your per-container billing.

### Additional Autodiscovery sources

You can add extra listeners and config providers via the `DD_EXTRA_LISTENERS` and `DD_EXTRA_CONFIG_PROVIDERS` enviroment variables. They will be added on top of the ones defined in the `listeners` and `config_providers` section of the datadog.yaml configuration file.

### Datadog Cluster Agent

The DCA is a **beta** feature, if you are facing any issues please reach out to our [support team](http://docs.datadoghq.com/help)
Starting with Agent v6.3.2, you can use the [Datadog Cluster Agent](#https://github.com/DataDog/datadog-agent/blob/master/docs/cluster-agent/README.md).

Cluster level features are now handled by the cluster agent, and you will find a `[DCA]` notation next to the affected features. Please refer to the below user documentation as well as the technical documentation here for further details on the instrumentation.

#### Kubernetes integration

Please refer to the dedicated section about the [Kubernetes integration](#kubernetes) for more details.

- `DD_KUBERNETES_COLLECT_METADATA_TAGS`: configures the agent to collect Kubernetes metadata (service names) as tags.
- `DD_KUBERNETES_METADATA_TAG_UPDATE_FREQ`: set the collection frequency in seconds for the Kubernetes metadata (service names) from the API Server (or the Datadog Cluster Agent if enabled).
- `DD_COLLECT_KUBERNETES_EVENTS` [DCA]: configures the cluster agent to collect Kubernetes events. See [Event collection](#event-collection) for more details.
- `DD_COLLECT_KUBERNETES_METRICS` [DCA]: configures the cluster agent to collect Kubernetes metrics. See [Metric collection](#metric-collection) for more details.
- `DD_COLLECT_KUBERNETES_TOPOLOGY` [DCA]: configures the cluster agent to collect Kubernetes topology. See [Topology collection](#topology-collection) for more details.
- `DD_LEADER_ELECTION` [DCA]: activates the [leader election](#leader-election). Will be activated if the `DD_COLLECT_KUBERNETES_EVENTS` is set to `true`. The expected value is a bool: true/false.
- `DD_LEADER_LEASE_DURATION` [DCA]: only used if the leader election is activated. See the details [here](#leader-election-lease). The expected value is a number of seconds.
- `DD_KUBE_RESOURCES_NAMESPACE` [DCA]: configures the namespace where the Cluster Agent creates the configmaps required for the Leader Election, the Event Collection (optional) and the Horizontal Pod Autoscaling.

#### Others

- `DD_JMX_CUSTOM_JARS`: space-separated list of custom jars to load in jmxfetch (only for the `-jmx` variants)
- `DD_ENABLE_GOHAI`: enable or disable the system information collector [gohai](https://github.com/DataDog/gohai) (enabled by default if not set)
- `DD_EXPVAR_PORT`: change the port for fetching [expvar](https://golang.org/pkg/expvar/) public variables from the agent. (defaults to 5000, you may then also have to change the [agent_stat.yaml](https://github.com/DataDog/datadog-agent/blob/f41c924ee1348c5c755118663f0895c7e4da1a4d/cmd/agent/dist/conf.d/go_expvar.d/agent_stats.yaml.example#L40))

Some options are not yet available as environment variable bindings. To customize these, the agent supports mounting a custom `/etc/datadog-agent/datadog.yaml` configuration file (based on the [docker](https://github.com/DataDog/datadog-agent/blob/master/Dockerfiles/agent/datadog-docker.yaml) or [kubernetes](https://github.com/DataDog/datadog-agent/blob/master/Dockerfiles/agent/datadog-kubernetes.yaml) base configurations) for these options, and using environment variables for the rest.

### Optional volumes

To run custom checks and configurations without building your own image, you can mount additional files in these folders:

- `/checks.d/` : custom checks in this folder will be copied over and used, if a corresponding configuration is found
- `/conf.d/` : check configurations and Autodiscovery templates in this folder will be copied over in the agent's configuration folder. You can mount a host folder, kubernetes configmaps, or other volumes. **Note:** autodiscovery templates now are directly stored in the main `conf.d` folder, not in an `auto_conf` subfolder.

### Going further

For more information about the container's lifecycle, see [SUPERVISION.md](SUPERVISION.md).

## Kubernetes

#### Without the DCA
**This sub-section is only valid for the agent versions < 6.3.2 or when not using the Datadog Cluster Agent.**

<a name="kubernetes"></a>
To deploy the Agent in your Kubernetes cluster, you can use the manifest in [manifests](../manifests/cluster-agent/cluster-agent.yaml). Firstly, make sure you have the correct [RBAC](#rbac) in place. You can use the files in manifests/rbac that contain the minimal requirements to run the Kubernetes Cluster level checks and perform the leader election.
`kubectl create -f manifests/rbac`

Please note that with the above RBAC, every agent will have access to the API Server, to list the pods, services ...
These accesses vanish when using the Datadog Cluster Agent.
Indeed, the agents will only have access to the local kubelet and only the Cluster Agent will be able to access cluster level insight (nodes, services...).

Once the RBAC is in place, you can then create the agents with:
`kubectl create -f manifests/agent.yaml`

The manifest for the agent has the `KUBERNETES` environment variable enabled, which will enable the collection of local kubernetes metrics via the kubelet's API. For the event collection and the API server check please read below.
If you want the event collection to be resilient, you can create a ConfigMap `datadogtoken` that agents will use to save and share a state reflecting which events where pulled last.

To create such a ConfigMap, you can use the following command:
`kubectl create -f manifests/datadog_configmap.yaml`
See details in [Event Collection](#event-collection).

#### With the DCA

**This sub-section is only valid for agent versions > 6.3.2 and when using the Datadog Cluster Agent.**

Event collection is handled by the cluster agent and the RBAC for the agent is slimmed down to the kubelet's API access. There is now a dedicated Clusterrole for the agent which should be as follows:

```
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: datadog-agent
rules:
- apiGroups:  # Kubelet connectivity
  - ""
  resources:
  - nodes/metrics
  - nodes/spec
  - nodes/proxy
  verbs:
  - get

```

It goes along the ClusterRoleBinding and the Service Account, dedicated to the datadog-agents.

### Event Collection [DCA]

<a name="event-collection"></a>
Similarly to Agent 5, Agent 6 collects events from the Kubernetes API server.

1/ Set the `collect_kubernetes_events` variable to `true` in the `datadog.yaml` file, you can use the environment variable `DD_COLLECT_KUBERNETES_EVENTS` for this.
2/ Give the agents proper RBACs to activate this feature. See the [RBAC](#rbac) section.
3/ A ConfigMap can be used to store the `event.tokenKey` and the `event.tokenTimestamp`. It has to be deployed in the `default` namespace and be named `datadogtoken`.
   Run `kubectl create configmap datadogtoken --from-literal="event.tokenKey"="0"` .
   You can also use the example in [manifests/datadog_configmap.yaml][https://github.com/DataDog/datadog-agent/blob/master/Dockerfiles/manifests/datadog_configmap.yaml].

Note: When the ConfigMap is used, if the agent in charge (via the [Leader election](#leader-election)) of collecting the events dies, the next leader elected will use the ConfigMap to identify the last events pulled.
This is in order to avoid duplicate the events collected, as well as putting less stress on the API Server.

#### Leader Election [DCA]

<a name="leader-election"></a>
Datadog Agent 6 supports built in leader election option for the Kubernetes event collector and the Kubernetes cluster related checks (i.e. Controle Plane service check).

This feature relies on Endpoints, you can enable it by setting the `DD_LEADER_ELECTION` environment variable to `true` the Datadog Agents will need to have a set of actions allowed prior to its deployment nevertheless.
See the [RBAC](#rbac) section for more details and keep in mind that these RBAC entities will need to be created before the option is set.

Agents coordinate by performing a leader election among members of the Datadog DaemonSet through kubernetes to ensure only one leader agent instance is gathering events at a given time.

This functionality is disabled by default, enabling the event collection will activate it (see [Event collection](#event-collection)) to avoid duplicating collecting events and stress on the API server.
<a name="leader-election-lease"></a>
The leaderLeaseDuration is the duration for which a leader stays elected. It should be > 30 seconds and is 60 seconds by default. The longer it is, the less frequently your agents hit the apiserver with requests, but it also means that if the leader dies (and under certain conditions), events can be missed until the lease expires and a new leader takes over.
It can be configured with the environment variable `DD_LEADER_LEASE_DURATION`.

#### RBAC

If you are using the DCA, find all the RBAC for the agent as well as the Cluster agent [here](https://github.com/DataDog/datadog-agent/tree/master/Dockerfiles/manifests/cluster-agent)

<a name="rbac"></a>
In the context of using the Kubernetes integration, and when deploying agents in a Kubernetes cluster, a set of rights are required for the agent to integrate seamlessly.

You will need to allow the agent to be allowed to perform a few actions:

- `get` and `update` of the `Configmaps` named `datadogtoken` to update and query the most up to date version token corresponding to the latest event stored in ETCD.
- `list` and `watch` of the `Events` to pull the events from the API Server, format and submit them.
- `get`, `update` and `create` for the `Endpoint`. The Endpoint used by the agent for the [Leader election](#leader-election) feature is named `datadog-leader-election`.
- `list` the `componentstatuses` resource, in order to submit service checks for the Controle Plane's components status.

You can find the templates in manifests/rbac [here](https://github.com/DataDog/datadog-agent/tree/master/Dockerfiles/manifests/rbac).
This will create the Service Account in the default namespace, a Cluster Role with the above rights and the Cluster Role Binding.

### Node label collection

The agent can collect node labels from the APIserver and report them as host tags. This feature is disabled by default, as it is usually redundant with cloud provider host tags. If you need to do so, you can provide a node label -> host tag mapping in the `DD_KUBERNETES_NODE_LABELS_AS_TAGS` environment variable. The format is the inline JSON described in the [tagging section](#Tagging).

### Kubernetes node name as aliases

By default, the agent is using the kubernetes _node name_ as an alias that can be used to forward metrics and events. This allows to submit events and metrics from remote hosts.
However, if you have several clusters where some nodes could have similar node names, some host alias collisions could occur. To prevent those, the agent supports the use of a cluster-unique identifier (such as the actual cluster name), through the environment variable `DD_CLUSTER_NAME`. That identifier will be added to the node name as a host alias, and avoid collision issues altogether.

### Legacy Kubernetes Versions

Our default configuration targets Kubernetes 1.7.6 and later, as we rely on features and endpoints introduced in this version. More installation steps are required for older versions:

- [RBAC objects](https://kubernetes.io/docs/admin/authorization/rbac/) (`ClusterRoles` and `ClusterRoleBindings`) are available since Kubernetes 1.6 and OpenShift 1.3, but are available under different `apiVersion` prefixes:
  * `rbac.authorization.k8s.io/v1` in Kubernetes 1.8+ (and OpenShift 3.9+), the default apiVersion we target
  * `rbac.authorization.k8s.io/v1beta1` in Kubernetes 1.5 to 1.7 (and OpenShift 3.7)
  * `v1` in Openshift 1.3 to 3.6

You can apply our yaml manifests with the following `sed` invocations:
```
sed "s%authorization.k8s.io/v1%authorization.k8s.io/v1beta1%" clusterrole.yaml | kubectl apply -f -
sed "s%authorization.k8s.io/v1%authorization.k8s.io/v1beta1%" clusterrolebinding.yaml | kubectl apply -f -
```
or for Openshift 1.3 to 3.6:
```
sed "s%rbac.authorization.k8s.io/v1%v1%" clusterrole.yaml | oc apply -f -
sed "s%rbac.authorization.k8s.io/v1%v1%" clusterrolebinding.yaml | oc apply -f -
```

- The `kubelet` check retrieves metrics from the Kubernetes 1.7.6+ (OpenShift 3.7.0+) prometheus endpoint. You need to [enable cAdvisor port mode](https://github.com/DataDog/integrations-core/blob/41cb3c5164b4eebd01e250a0f322896493233813/kubelet/README.md#compatibility) for older versions.

- Our default daemonset makes use of the [downward API](https://kubernetes.io/docs/tasks/inject-data-application/environment-variable-expose-pod-information/) to pass the kubelet's IP to the agent. This only works on versions 1.7 and up. For older versions, here are other ways to enable kubelet connectivity:

  * On versions 1.6, use `fieldPath: spec.nodeName` and make sure your node name is resolvable and reachable from the pod
  * If `DD_KUBERNETES_KUBELET_HOST` is unset, the agent will retrieve the node hostname from docker and try to connect there. See `docker info | grep "Name:"` and make sure the name is resolvable and reachable
  * If the IP of the docker default gateway is constant across your cluster, you can directly pass that IP in the `DD_KUBERNETES_KUBELET_HOST` envvar. You can retrieve the IP with the `ip addr show | grep docker0` command.

- Our default configuration relies on [bearer token authentication](https://kubernetes.io/docs/admin/authentication/#service-account-tokens) to the APIserver and kubelet. On 1.3, the kubelet does not support bearer token auth, you will need to setup client certificates for the `datadog-agent` serviceaccount and pass them to the agent.

## Log collection

The Datadog Agent can collect logs from containers starting at the **version 6**. Two installations are possible:

- on the host: where the agent is external to the Docker environment
- or by deploying its containerized version in the Docker environment

### Setup

To  run a Docker container which embeds the Datadog Agent to monitor your host use the following command:

```
docker run -d --name datadog-agent \
           -e DD_API_KEY=<YOUR_API_KEY> \
           -e DD_LOGS_ENABLED=true \
           -e DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL=true \
           -e DD_AC_EXCLUDE="name:datadog-agent" \
           -v /var/run/docker.sock:/var/run/docker.sock:ro \
           -v /proc/:/host/proc/:ro \
           -v /opt/datadog-agent/run:/opt/datadog-agent/run:rw \
           -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro \
           datadog/agent:latest
```

The commands related to log collection are the following:

* `-e DD_LOGS_ENABLED=true`: this parameter enables log collection when set to `true`. The Agent looks for log instructions in configuration files.
* `-e DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL=true`: this parameter adds a log configuration that enables log collection for all containers (see `Option 1` below)
* `-v /opt/datadog-agent/run:/opt/datadog-agent/run:rw`: to make sure you do not lose any logs from containers during restarts or network issues, the last line that was collected for each container in this directory is stored on the host.
* `-e DD_AC_EXCLUDE="name:datadog-agent"`: to prevent the Datadog Agent from collecting and sending its own logs. Remove this parameter if you want to collect the Datadog Agent logs.

**Important notes**: Integration Pipelines and Processors will not be installed automatically, as the source and service are set to the `docker` generic value.
The source and service values can be overriden thanks to Autodiscovery as described below; it automatically installs integration Pipelines that parse your logs and extract all the relevant information from them.

### Activate Log Integrations

The second step is to use Autodiscovery to customize the `source` and `service` value. This allows Datadog to identify the log source for each container.

Since version 6.2 of the Datadog Agent, you can [configure log collection directly in the container labels](https://docs.datadoghq.com/logs/log_collection/docker/?tab=dockerfile#activate-log-integrations).
Pod annotations are also supported for Kubernetes environment, see the [Kubernetes Autodiscovery documentation][https://docs.datadoghq.com/agent/autodiscovery/#template-source-kubernetes-pod-annotations].

## How to build this image

### On debian-based systems

You can build your own debian package using `inv agent.omnibus-build`

Then you can call `inv agent.image-build` that will take the debian package generated above and use it to build the image

### On other systems

To build the image you'll need the agent debian package that can be found on this APT listing [here](https://s3.amazonaws.com/apt-agent6.datad0g.com).

You'll need to download one of the `datadog-agent*_amd64.deb` package in this directory, it will then be used by the `Dockerfile` and installed within the image.

You can then build the image using `docker build -t datadog/agent:master .`

To build the jmx variant, add `--build-arg WITH_JMX=true` to the build command
