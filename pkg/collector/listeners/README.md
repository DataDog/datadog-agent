## package `listeners`

This package is providing the `ServiceListener` concept to the agent. A `ServiceListener` listens for events related to services the agent should monitor.


### `Service`

`Service` reprensents an application we can run a check against. It should be matched with a check template by the ConfigResolver.
Services can only be Docker containers for now.


### `ServiceListener` (there is only a `DockerListener` for now)

`ServiceListener` monitors events related to `Service` lifecycles. It then formats and transmits this data to `ConfigResolver`.


### `DockerListener`

`DockerListener` first gets current running containers and send these to `ConfigResolver`. Then it starts listening on the Docker event API for container activity and pass by `Services` mentioned in start/stop events to `ConfigResolver` through the corresponding channel.

**TODO**:
- `DockerListener` calls Docker directly. We need a caching layer there.
- support TLS
- getHosts, getPorts and getTags need to use a caching layer for docker **and** use the k8s api (also with caching)
