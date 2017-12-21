# Agent 6 docker image

This is how the official agent 6 image available [here](https://hub.docker.com/r/datadog/agent/) is built.

## How to run it

The following environment variables are supported:

- `DD_API_KEY`: your API key (**required**)
- `DD_HOSTNAME`: hostname to use for metrics

Example usage: `docker run -e DD_API_KEY=your-api-key-here -it <image-name>`

For a more detailed usage please refer to the official [Docker Hub](https://hub.docker.com/r/datadog/agent/)

### Log collection parameters

For containerized installation, here are the command related to log collection:

* `-v /opt/datadog-agent/run:/opt/datadog-agent/run:rw:` Store on disk where to pick log file or container stdout when we restart
* `-v /var/run/docker.sock:/var/run/docker.sock:ro:` Give access to docker api to collect container stdout and stderr
* `-v /my/path/to/conf.d:/conf.d:ro:` mount configuration repository
* `-v /my/file/to/tail:/tail.log:ro:` Foreach log file that should be tailed by the agent (not required if you only want to collect container stdout or stderr)
* `-e DD_API_KEY=<YOUR_API_KEY>:` Set the api key
* `DD_LOG_ENABLED=true:` Activate log collection (disable by default)

To start collecting logs for a given container filtered by image or label, you need to update the log section in an integration or custom yaml file as explained in our [documentation](https://docs.datadoghq.com/logs/#docker-log-collection).

## How to build it

### On debian-based systems

You can build your own debian package using `inv agent.omnibus-build`

Then you can call `inv agent.image-build` that will take the debian package generated above and use it to build the image

### On other systems

To build the image you'll need the agent debian package that can be found on this APT listing [here](https://s3.amazonaws.com/apt-agent6.datad0g.com).

You'll need to download one of the `datadog-agent*_amd64.deb` package in this directory, it will then be used by the `Dockerfile` and installed within the image.

You can then build the image using `docker build -t datadog/agent:master .`

To build the jmx variant, add `--build-arg WITH_JMX=true` to the build command
