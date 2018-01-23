# Agent 6 docker image

This is how the official agent 6 image available [here](https://hub.docker.com/r/datadog/agent/) is built.

## How to run it

The following environment variables are supported:

- `DD_API_KEY`: your API key (**required**)
- `DD_HOSTNAME`: hostname to use for metrics
- `DD_TAGS`: host tags, separated by spaces. For example: `simple-tag-0 tag-key-1:tag-value-1`

- `DD_APM_ENABLED`: run the trace-agent along with the infrastructure agent, allowing the container to accept traces on 8126/tcp
- `DD_PROCESS_AGENT_ENABLED`: run the [process-agent](https://docs.datadoghq.com/graphing/infrastructure/process/) along with the infrastructure agent, feeding data to the Live Process View and Live Containers View
- `DD_LOG_ENABLED`: run the [log-agent](https://docs.datadoghq.com/logs/) along with the infrastructure agent. See below for details

Example usage: `docker run -e DD_API_KEY=your-api-key-here -it <image-name>`

For a more detailed usage please refer to the official [Docker Hub](https://hub.docker.com/r/datadog/agent/)

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
docker run -d --name dd-agent -h `hostname` -e DD_API_KEY=<YOUR_API_KEY> -e DD_LOG_ENABLED=true -v /var/run/docker.sock:/var/run/docker.sock:ro -v /proc/:/host/proc/:ro -v /opt/datadog-agent/run:/opt/datadog-agent/run:rw -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro -v /opt/datadog-agent/conf.d:/conf.d:ro datadog/agent:latest
```

*Important notes*:

- The Docker integration is enabled by default, as well as [autodiscovery](https://docs.datadoghq.com/guides/servicediscovery/) in auto config mode ((remove the `listeners: -docker` section in `datadog.yaml` to disable it).

- You can find [here](https://hub.docker.com/r/datadog/agent/tags/) the list of available images for agent 6 and we encourage you to always pick the latest version.

The parameters specific to log collection are the following:

- `-e DD_LOG_ENABLED=true`: this parameter enables the log collection when set to true. The agent now looks for log instructions in configuration files.
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
