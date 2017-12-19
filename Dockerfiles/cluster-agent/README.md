# Cluster Agent 6 docker image

This is how the official cluster-agent (aka DCA) 6 image available [here](https://hub.docker.com/r/datadog/agent/) is built.

## How to run it

The following environment variables are supported:

- `DD_API_KEY`: your API key (**required**)
- `DD_HOSTNAME`: hostname to use for metrics
- 'CMD_PORT': Port you want the DCA to serve

Example usage: `docker run -e DD_API_KEY=your-api-key-here -e CMD_PORT=1234 -it <image-name>`

For a more detailed usage please refer to the official [Docker Hub](https://hub.docker.com/r/datadog/agent/)

## How to build it

### On debian-based systems

You can build your own binary using `inv cluster-agent.build`

Then you can call `inv cluster-agent.image-build` that will take the binary generated above and use it to build the image
