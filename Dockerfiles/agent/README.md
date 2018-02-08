# Agent 6 docker image

This is how the official agent 6 image available [here](https://hub.docker.com/r/datadog/agent/) is built.

## How to run it

The following environment variables are supported:

- `DD_API_KEY`: your API key (**required**)
- `DD_HOSTNAME`: hostname to use for metrics
- `DD_TAGS`: host tags, separated by spaces. For example: `simple-tag-0 tag-key-1:tag-value-1`

- `DD_DOGSTATSD_NON_LOCAL_TRAFFIC`: listen to dogstatsd packets from other containers, required to send custom metrics
- `DD_APM_ENABLED`: run the trace-agent along with the infrastructure agent, allowing the container to accept traces on 8126/tcp
- `DD_PROCESS_AGENT_ENABLED`: run the [process-agent](https://docs.datadoghq.com/graphing/infrastructure/process/) along with the infrastructure agent, feeding data to the Live Process View and Live Containers View
- `DD_LOGS_ENABLED`: run the [log-agent](https://docs.datadoghq.com/logs/) along with the infrastructure agent. See below for details
- `DD_JMX_CUSTOM_JARS`: space-separated list of custom jars to load in jmxfetch (only for the `-jmx` variants)

- `DD_KUBERNETES_COLLECT_SERVICE_TAGS`: Configures the agent to collect Kubernetes service names as tags.
- `DD_KUBERNETES_SERVICE_TAG_UPDATE_FREQ`: Set the collection frequency in seconds for the Kubernetes service names.
- `DD_COLLECT_KUBERNETES_EVENTS`: Configures the agent to collect Kubernetes events. See [Event collection](#event-collection) for more details.
- `DD_LEADER_ELECTION`: Activates the [leader election](#leader-election). Will be activated if the `DD_COLLECT_KUBERNETES_EVENTS` is set to true. The expected value is a bool: true/false.
- `DD_LEADER_LEASE_DURATION`: Only used if the leader election is activated. See the details [here](#leader-election-lease). The expected value is a number of seconds.

Example usage: `docker run -e DD_API_KEY=your-api-key-here -it <image-name>`

For more information about the container's lifecycle, see [SUPERVISION.md](SUPERVISION.md).

## How to build it

### On debian-based systems

You can build your own debian package using `inv agent.omnibus-build`

Then you can call `inv agent.image-build` that will take the debian package generated above and use it to build the image

### On other systems

To build the image you'll need the agent debian package that can be found on this APT listing [here](https://s3.amazonaws.com/apt-agent6.datad0g.com).

You'll need to download one of the `datadog-agent*_amd64.deb` package in this directory, it will then be used by the `Dockerfile` and installed within the image.

You can then build the image using `docker build -t datadog/agent:master .`

To build the jmx variant, add `--build-arg WITH_JMX=true` to the build command

## How to activate log collection

The Datadog Agent can collect logs from containers starting at the version 6. Two installations are possible:

- on the host: where the agent is external to the Docker environment
- or by deploying its containerized version in the Docker environment

### Setup

First let’s create two directories on the host that we will later mount on the containerized agent:

- `/opt/datadog-agent/run`: to make sure we do not lose any logs from containers during restarts or network issues we store on the host the last line that was collected for each container in this directory
- `/opt/datadog-agent/conf.d`: this is where you will provide your integration instructions. Any configuration file added there will automatically be picked up by the containerized agent when restarted. For more information about this check [here](https://github.com/DataDog/docker-dd-agent#enabling-integrations).

To  run a Docker container which embeds the Datadog Agent to monitor your host use the following command:

```
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

```
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

### Kubernetes

To deploy the Agent in your Kubernetes cluster, you can use the manifest in `manifests`.
Firstly, make sure you have the correct [RBAC](#rbac) in place. You can use the files in manifests/rbac that contain the minimal requirements to run the Kubernetes Cluster level checks and perform the leader election.
`kubectl create -f manifest/rbac`

Then, you can then create the agents with:
`kubectl create -f manifest/agent.yaml`

The manifest for the agent has the `KUBERNETES` environment variable enabled, which will enable the event collection and the API server check described here.
If you want the event collection to be resilient, you can create a ConfigMap `datadogtoken` that agents will use to save and share a state reflecting which events where pulled last.

To create such a ConfigMap, you can use the following command:
`kubectl create -f manifest/datadog_configmap.yaml`
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

You can find the templates in manifests/rbac.
This will create the Service Account in the default namespace, a Cluster Role with the above rights and the Cluster Role Binding.
