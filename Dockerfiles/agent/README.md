# Agent 6 docker image

This is how the official agent 6 image available [here](https://hub.docker.com/r/datadog/agent/) is built.

## How to run it

The following environment variables are supported:

- `DD_API_KEY`: your API key (**required**)
- `DD_HOSTNAME`: hostname to use for metrics

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
