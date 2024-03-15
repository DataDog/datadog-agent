# package `listeners`

This package is providing the `ServiceListener` concept to the agent. A `ServiceListener` listens for events related to services the agent should monitor.

## `Service`

`Service` represents an application/device we can run an integration against. It should be matched with a config template by the ConfigResolver.

Services can be:
- Docker containers
- Containerd containers
- Podman containers
- Kubelet containers
- Kubelet pods
- ECS containers
- Kubernetes Service objects
- Kubernetes Endpoints objects
- CloudFoundry containers
- Network devices

## `ServiceListener`

`ServiceListener` monitors events related to `Service` lifecycles. It then formats and transmits this data to `autoconfig`.

Note: It's important to enable only one `ServiceListener` per `Service` type, for example, in Kubernetes `ContainerListener` and `KubeletListener` must not run together because they watch the same containers.

### `ContainerListener`

`ContainerListener` first gets current running containers and send these to the `autoconfig`. Then it starts watching workloadmeta container events and pass by `Services` mentioned in start/stop events to the `autoconfig` through the corresponding channel.

### `ECSListener`

The `ECSListener` relies on the ECS metadata APIs available within the agent container. We're listening on changes on the container list exposed through the API to discover new `Services`. This listener is enabled on ECS Fargate only, on ECS EC2 we use the docker listener.

### `KubeletListener`

The `KubeletListener` relies on the Kubelet API. We're listening on changes on the container list exposed through the API (`/pods`) to discover new `Services`. `KubeletListener` creates `Services` for containers and pods.

### `KubeServiceListener`

The `KubeServiceListener` relies on the Kubernetes API server to watch service objects and creates the corresponding Autodiscovery `Services`. The Datadog Cluster Agent runs this `ServiceListener`.

### `KubeEndpointsListener`

The `KubeEndpointsListener` relies on the Kubernetes API server to watch endpoints and service objects, and creates corresponding Autodiscovery `Services`. The Datadog Cluster Agent runs this `ServiceListener`.

### `CloudFoundryListener`

The `CloudFoundryListener` relies on the Cloud Foundry BBS API to detect container changes, and creates corresponding Autodiscovery `Services`.

### `SNMPListener`

TODO

## Listeners & auto-discovery

### Template variable support

| Listener | AD identifiers | Host | Port | Tag | Pid | Env | Hostname
|---|---|---|---|---|---|---|---|
| Docker | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Containerd | ✅ | ✅ | ❌ | ✅ | ❌ | ✅ | ✅ |
| Podman | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| ECS | ✅ | ✅ | ❌ | ✅ | ❌ | ✅ | ❌ |
| Kubelet | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | ❌ |
| KubeService | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ | ❌ |
| KubeEndpoints | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | ❌ |
