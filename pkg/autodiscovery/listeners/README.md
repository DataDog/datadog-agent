# package `listeners`

This package is providing the `ServiceListener` concept to the agent. A `ServiceListener` listens for events related to services the agent should monitor.

## `Service`

`Service` represents an application we can run an integration against. It should be matched with a config template by the ConfigResolver.
Services can only be Docker containers for now.

## `ServiceListener`

`ServiceListener` monitors events related to `Service` lifecycles. It then formats and transmits this data to `ConfigResolver`.

### `DockerListener`

`DockerListener` first gets current running containers and send these to the `AutoConfig`. Then it starts listening on the Docker event API for container activity and pass by `Services` mentioned in start/stop events to the `AutoConfig` through the corresponding channel.

### `ECSListener`

The `ECSListener` relies on the metadata APIs available within the agent container. We're listening on changes on the container list exposed through the API to discover new `Services`.

### `KubeletListener`

The `KubeletListener` relies on the Kubelet API. We're listening on changes on the container list exposed through the API (`/pods`) to discover new `Services`.

## Listeners & auto-discovery

### Template variable support

| Listener | AD identifiers | Host | Port | Tag | Pid | Env | Hostname
|---|---|---|---|---|---|---|
| Docker | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| ECS | ✅ | ✅ | ❌ | ✅ | ❌ | ✅ | ❌ |
| Kubelet | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | ❌ |
