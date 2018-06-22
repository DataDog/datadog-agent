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

The agent proxy settings can be overridden with the standard `*_PROXY`
environment variables:

- `HTTP_PROXY`: an http URL to use as a proxy for `http` requests.
- `HTTPS_PROXY`: an http URL to use as a proxy for `https` requests.
- `NO_PROXY`: a comma-separated list of URLs for which no proxy should be used.

Notice: these variables don't use the `DD_` prefix.

For more information: https://docs.datadoghq.com/agent/proxy/#agent-v6

#### Optional collection agents

These features are disabled by default for security or performance reasons, you need to explicitly enable them:

- `DD_APM_ENABLED`: run the trace-agent along with the infrastructure agent, allowing the container to accept traces on 8126/tcp
- `DD_LOGS_ENABLED`: run the [log-agent](https://docs.datadoghq.com/logs/) along with the infrastructure agent. [See below for details](#log-collection)
- `DD_PROCESS_AGENT_ENABLED`: enable live process collection in the [process-agent](https://docs.datadoghq.com/graphing/infrastructure/process/). The Live Container View is already enabled by default if the Docker socket is available

#### Dogstatsd (custom metrics)

You can send custom metrics via [the statsd protocol](https://docs.datadoghq.com/developers/dogstatsd/):

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

You can either define them in your custom `datadog.yaml`, or set them as JSON maps in these envvars. The map key is the source (label/envvar) name, and the map value the datadog tag name.

```shell
DD_KUBERNETES_POD_LABELS_AS_TAGS='{"app":"kube_app","release":"helm_release"}'
DD_DOCKER_LABELS_AS_TAGS='{"com.docker.compose.service":"service_name"}'
```

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

#### Kubernetes integration

Please refer to the dedicated section about the [Kubernetes integration](#kubernetes) for more details.

- `DD_KUBERNETES_COLLECT_METADATA_TAGS`: configures the agent to collect Kubernetes metadata (service names) as tags.
- `DD_KUBERNETES_METADATA_TAG_UPDATE_FREQ`: set the collection frequency in seconds for the Kubernetes metadata (service names).
- `DD_COLLECT_KUBERNETES_EVENTS`: configures the agent to collect Kubernetes events. See [Event collection](#event-collection) for more details.
- `DD_LEADER_ELECTION`: activates the [leader election](#leader-election). Will be activated if the `DD_COLLECT_KUBERNETES_EVENTS` is set to true. The expected value is a bool: true/false.
- `DD_LEADER_LEASE_DURATION`: only used if the leader election is activated. See the details [here](#leader-election-lease). The expected value is a number of seconds.
- `DD_KUBE_RESOURCES_NAMESPACE`: can be used to configure the namespace where the configmaps of the Leader Election and the Event Collection live.

#### Others

- `DD_JMX_CUSTOM_JARS`: space-separated list of custom jars to load in jmxfetch (only for the `-jmx` variants)
- `DD_ENABLE_GOHAI`: enable or disable the system information collector [gohai](https://github.com/DataDog/gohai) (enabled by default if not set)
- `DD_EXPVAR_PORT`: change the port for fetching [expvar](https://golang.org/pkg/expvar/) public variables from the agent. (defaults to 5000, you may then also have to change the [agent_stat.yaml](https://github.com/DataDog/datadog-agent/blob/f41c924ee1348c5c755118663f0895c7e4da1a4d/cmd/agent/dist/conf.d/go_expvar.d/agent_stats.yaml.example#L40))

Some options are not yet available as environment variable bindings. To customize these, the agent supports mounting a custom `/etc/datadog-agent/datadog.yaml` configuration file (based on the [docker](https://github.com/DataDog/datadog-agent/blob/master/Dockerfiles/agent/datadog-docker.yaml) or [kubernetes](https://github.com/DataDog/datadog-agent/blob/master/Dockerfiles/agent/datadog-kubernetes.yaml) base configurations) for these options, and using environment variables for the rest.

### Optional volumes

To run custom checks and configurations without buidling your own image, you can mount additional files in these folders:

- `/checks.d/` : custom checks in this folder will be copied over and used, if a corresponding configuration is found
- `/conf.d/` : check configurations and Autodiscovery templates in this folder will be copied over in the agent's configuration folder. You can mount a host folder, kubernetes configmaps, or other volumes. **Note:** autodiscovery templates now are directly stored in the main `conf.d` folder, not in an `auto_conf` subfolder.

### Going further

For more information about the container's lifecycle, see [SUPERVISION.md](SUPERVISION.md).

## Kubernetes

<a name="kubernetes"></a>
To deploy the Agent in your Kubernetes cluster, you can use the manifest in `manifests`.
Firstly, make sure you have the correct [RBAC](#rbac) in place. You can use the files in manifests/rbac that contain the minimal requirements to run the Kubernetes Cluster level checks and perform the leader election.
`kubectl create -f manifests/rbac`

Then, you can then create the agents with:
`kubectl create -f manifests/agent.yaml`

The manifest for the agent has the `KUBERNETES` environment variable enabled, which will enable the event collection and the API server check described here.
If you want the event collection to be resilient, you can create a ConfigMap `datadogtoken` that agents will use to save and share a state reflecting which events where pulled last.

To create such a ConfigMap, you can use the following command:
`kubectl create -f manifests/datadog_configmap.yaml`
See details in [Event Collection](#event-collection).

### Event Collection

<a name="event-collection"></a>
Similarly to the Agent 5, the Agent 6 can collect events from the Kubernetes API server.
First and foremost, you need to set the `collect_kubernetes_events` variable to `true` in the datadog.yaml, this can be achieved via the environment variable `DD_COLLECT_KUBERNETES_EVENTS` that is resolved at start time.
You will need to give the agent some rights to activate this feature. See the [RBAC](#rbac) section.

A ConfigMap can be used to store the `event.tokenKey` and the `event.tokenTimestamp`. It has to be deployed in the `default` namespace and be named `datadogtoken`.
One can simply run `kubectl create configmap datadogtoken --from-literal="event.tokenKey"="0"` . You can also use the example in manifests/datadog_configmap.yaml.

When the ConfigMap is used, if the agent in charge (via the [Leader election](#leader-election)) of collecting the events dies, the next leader elected will use the ConfigMap to identify the last events pulled.
This is in order to avoid duplicate the events collected, as well as putting less stress on the API Server.

#### Leader Election

<a name="leader-election"></a>
The Datadog Agent6 supports built in leader election option for the Kubernetes event collector and the Kubernetes cluster related checks (i.e. Controle Plane service check).

This feature relies on Endpoints, you can enable it by setting the `DD_LEADER_ELECTION` environment variable to `true` the Datadog Agents will need to have a set of actions allowed prior to its deployment nevertheless.
See the [RBAC](#rbac) section for more details and keep in mind that these RBAC entities will need to be created before the option is set.

Agents coordinate by performing a leader election among members of the Datadog DaemonSet through kubernetes to ensure only one leader agent instance is gathering events at a given time.

This functionality is disabled by default, enabling the event collection will activate it (see [Event collection](#event-collection)) to avoid duplicating collecting events and stress on the API server.
<a name="leader-election-lease"></a>
The leaderLeaseDuration is the duration for which a leader stays elected. It should be > 30 seconds and is 60 seconds by default. The longer it is, the less frequently your agents hit the apiserver with requests, but it also means that if the leader dies (and under certain conditions), events can be missed until the lease expires and a new leader takes over.
It can be configured with the environment variable `DD_LEADER_LEASE_DURATION`.

#### RBAC

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

The Datadog Agent can collect logs from containers starting at the version 6. Two installations are possible:

- on the host: where the agent is external to the Docker environment
- or by deploying its containerized version in the Docker environment

### Setup

First let’s create two directories on the host that we will later mount on the containerized agent:

- `/opt/datadog-agent/run`: to make sure we do not lose any logs from containers during restarts or network issues we store on the host the last line that was collected for each container in this directory
- `/opt/datadog-agent/conf.d`: this is where you will provide your integration instructions. Any configuration file added there will automatically be picked up by the containerized agent when restarted. For more information about this check [here](https://github.com/DataDog/docker-dd-agent#enabling-integrations).

To  run a Docker container which embeds the Datadog Agent to monitor your host use the following command:

```shell
docker run -d --name dd-agent -h `hostname` -e DD_API_KEY=<YOUR_API_KEY> -e DD_LOGS_ENABLED=true -v /var/run/docker.sock:/var/run/docker.sock:ro -v /proc/:/host/proc/:ro -v /opt/datadog-agent/run:/opt/datadog-agent/run:rw -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro -v /opt/datadog-agent/conf.d:/conf.d:ro datadog/agent:latest
```

*Important notes*:

- The Docker integration is enabled by default, as well as [autodiscovery](https://docs.datadoghq.com/guides/servicediscovery/) in auto config mode ((remove the `listeners: -docker` section in `datadog.yaml` to disable it).

- You can find [here](https://hub.docker.com/r/datadog/agent/tags/) the list of available images for agent 6 and we encourage you to always pick the latest version.

The parameters specific to log collection are the following:

- `-e DD_LOGS_ENABLED=true`: this parameter enables the log collection when set to true. The agent now looks for log instructions in configuration files.
- `-v /opt/datadog-agent/run:/opt/datadog-agent/run:rw` : mount the directory we created to store pointer on each container logs to make sure we do not lose any.
- `-v /opt/datadog-agent/conf.d:/conf.d:ro` : mount the configuration directory we previously created to the container

### Configuration file example

Now that the agent is ready to collect logs, you need to define which containers you want to follow.
To start collecting logs for a given container filtered by image or label, you need to update the log section in an integration or custom .yaml file.
Add a new yaml file in the conf.d directory with the following parameters:

```yaml
init_config:

instances:
    [{}]

#Log section

logs:
   - type: docker
     image: my_image_name    #or label: mylabel
     service: my_application_name
     source: java #tells Datadog what integration it is
     sourcecategory: sourcecode
```

For more examples of configuration files or agent capabilities (such as filtering, redacting, multiline, …) read [this documentation](https://docs.datadoghq.com/logs/#filter-logs).

## How to build this image

### On debian-based systems

You can build your own debian package using `inv agent.omnibus-build`

Then you can call `inv agent.image-build` that will take the debian package generated above and use it to build the image

### On other systems

To build the image you'll need the agent debian package that can be found on this APT listing [here](https://s3.amazonaws.com/apt-agent6.datad0g.com).

You'll need to download one of the `datadog-agent*_amd64.deb` package in this directory, it will then be used by the `Dockerfile` and installed within the image.

You can then build the image using `docker build -t datadog/agent:master .`

To build the jmx variant, add `--build-arg WITH_JMX=true` to the build command
