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
