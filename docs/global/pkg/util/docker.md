> **TL;DR:** Thin wrapper around the Docker daemon HTTP API providing a global singleton client with automatic retry, container and image inspection helpers, typed event streaming and polling, storage statistics, and Swarm metadata collection.

# pkg/util/docker

## Purpose

`pkg/util/docker` provides a thin wrapper around the Docker daemon HTTP API. It is
the agent's single point of contact with Docker: it negotiates the API version,
manages a global singleton client with automatic retry on startup, exposes
container/image inspection helpers, surfaces Docker daemon events as typed Go
structs, and collects storage and host-tag metadata.

The package is compiled only when the `docker` build tag is set. When Docker
support is not compiled in, stub functions in `global_nodocker.go` and
`metadata_no_docker.go` return `ErrDockerNotCompiled` so callers do not need
build-tag guards of their own.

## Key elements

### Key types

**`DockerUtil`** struct — the central type. Holds the underlying `*client.Client`, a query timeout, an image-name-by-SHA cache, and event-stream state. Not instantiated directly; use the global helpers.

**`Config`** struct — embedded in `DockerUtil`. Notable fields: `CacheDuration` (default 10 s), `CollectNetwork` (default `true`).

**`StorageStats`** — holds `Name`, `Free`, `Used`, and `Total` as nullable `*uint64` fields. `GetPercentUsed()` computes usage even when only two of the three values are present.

**`ContainerEvent`** — fields: `ContainerID`, `ContainerName`, `ImageName`, `Action`, `Timestamp`, `Attributes`.

**`ImageEvent`** — fields: `ImageID`, `Action` (`pull`, `delete`, `tag`, `untag`), `Timestamp`.

**Sentinel errors**

| Error | Meaning |
|---|---|
| `ErrDockerNotAvailable` | Docker daemon not reachable at init time |
| `ErrDockerNotCompiled` | `docker` build tag absent |
| `ErrNotImplemented` | Mirroring `gopsutil` internal sentinel |
| `ErrAlreadySubscribed` | Subscriber name already registered |
| `ErrNotSubscribed` | Subscriber name not found on unsubscribe |
| `ErrStorageStatsNotAvailable` | Storage stats not available for this driver |

### Key interfaces

### `Client` interface (`client.go`)

```go
type Client interface {
    RawClient() *client.Client
    RawContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
    ResolveImageName(ctx context.Context, image string) (string, error)
    Images(ctx context.Context, includeIntermediate bool) ([]image.Summary, error)
    GetPreferredImageName(imageID string, repoTags, repoDigests []string) string
    GetStorageStats(ctx context.Context) ([]*StorageStats, error)
    CountVolumes(ctx context.Context) (int, int, error)
    LatestContainerEvents(ctx context.Context, since time.Time, filter workloadfilter.FilterBundle) ([]*ContainerEvent, time.Time, error)
}
```

`DockerUtil` satisfies this interface. Tests use `client_mock.go`.

### Key functions

#### Global singleton (`global.go`)

| Function | Description |
|---|---|
| `GetDockerUtil() (*DockerUtil, error)` | Returns the shared singleton; blocks until init succeeds or returns last error |
| `GetDockerUtilWithRetrier() (*DockerUtil, *retry.Retrier)` | Same, but also returns the retrier for callers that want to introspect retry state |
| `EnableTestingMode()` | Installs a no-op retrier so unit tests can use the inspect cache without a real daemon |
| `GetHostname(ctx) (string, error)` | Package-level shortcut to `DockerUtil.GetHostname` |

The singleton is initialised lazily with an exponential back-off retrier
(initial 1 s, max 5 min). The underlying connection is established by
`ConnectToDocker`, which calls `client.Info` to verify the daemon is
reachable.

### Container operations (`docker_util.go`)

| Method | Description |
|---|---|
| `RawContainerList` | Thin wrapper around `ContainerList`; caller validates results |
| `RawContainerListWithFilter` | Like `RawContainerList` but applies a `workloadfilter.FilterBundle` |
| `Inspect(id, withSize)` | Returns `container.InspectResponse`; caches results for 10 s |
| `InspectNoCache(id, withSize)` | Always fetches from the daemon |
| `AllContainerLabels()` | Returns `map[containerID]map[label]value` for all running containers |
| `GetContainerStats(id)` | One-shot stats snapshot (`container.StatsResponse`) |
| `ContainerLogs(id, opts)` | Returns an `io.ReadCloser` for log streaming |
| `GetContainerPIDs(id)` | Returns the list of PIDs inside a container via `ContainerTop` |

### Image operations (`docker_util.go`)

| Method | Description |
|---|---|
| `Images(includeIntermediate)` | Lists all images |
| `ResolveImageName(image)` | Resolves a SHA256/repo-digest reference to a human-readable name; caches results |
| `ResolveImageNameFromContainer(co)` | Like `ResolveImageName`, first tries `Config.Image` from an inspect response |
| `GetPreferredImageName(id, repoTags, repoDigests)` | Deterministically picks the shortest tag or, if absent, the repo name from the digest |
| `ImageInspect(id)` | Full `image.InspectResponse` |
| `ImageHistory(id)` | Layer history |

### Storage stats (`storage.go`)

`StorageStats` holds `Name`, `Free`, `Used`, and `Total` as nullable `*uint64`
fields. `GetPercentUsed()` computes usage even when only two of the three
values are present. `GetStorageStats` parses the `DriverStatus` field from
`docker info`; currently supports DeviceMapper only.

Sentinel errors: `ErrStorageStatsNotAvailable`.

### Event streaming (`event_stream.go`, `event_types.go`)

Events are pushed to named subscribers via buffered channels.

```go
containerCh, imageCh, err := du.SubscribeToEvents("my-listener", filterBundle)
// ...
du.UnsubscribeFromContainerEvents("my-listener")
```

**`ContainerEvent`** fields: `ContainerID`, `ContainerName`, `ImageName`,
`Action` (one of `start`, `die`, `rename`, `health_status`, `died`),
`Timestamp`, `Attributes`.

**`ImageEvent`** fields: `ImageID`, `Action` (`pull`, `delete`, `tag`,
`untag`), `Timestamp`.

Image events are only streamed when `container_image.enabled` is `true`.

The stream reconnects automatically on EOF or error (10 s back-off).

Sentinel errors: `ErrAlreadySubscribed`, `ErrNotSubscribed`.

### Event polling (`event_pull.go`)

`LatestContainerEvents(since, filter)` performs a one-shot poll instead of
streaming. Used by workloadmeta to catch up on missed events.

### Host tags and metadata

`GetTags(ctx)` returns `docker_swarm_node_role:{worker|manager}` when the
host is part of a Swarm cluster.

`GetMetadata()` returns `docker_version` and `docker_swarm` (active/inactive),
used by the inventory-host metadata payload.

### Misc helpers

`ContainerHosts(networkIPs, labels, hostname)` builds a map of hostname
aliases to IP addresses, preferring network IPs, then Rancher label IP,
then the container hostname.

Network mode constants: `DefaultNetworkMode`, `HostNetworkMode`,
`BridgeNetworkMode`, `NoneNetworkMode`, `AwsvpcNetworkMode`,
`UnknownNetworkMode`.

### Configuration and build flags

All non-stub files carry `//go:build docker`. The stubs always compile and return sentinel errors, keeping callers tag-free.

| Config key | Description |
|---|---|
| `docker_query_timeout` | Timeout in seconds for Docker API calls |
| `container_image.enabled` | Enables image event streaming |
| `collect_ec2_tags` | Propagated to image events (via the docker workloadmeta collector) |

## Usage

### Fetching container labels

```go
// comp/core/workloadmeta/collectors/internal/docker/docker.go
du, err := docker.GetDockerUtil()
if err != nil {
    return err
}
labels, err := du.AllContainerLabels(ctx)
```

### Inspecting a container

```go
inspect, err := du.Inspect(ctx, containerID, false)
if errors.Is(err, dderrors.ErrNotFound) {
    // container gone
}
```

### Subscribing to events

```go
containerCh, imageCh, err := du.SubscribeToEvents("workloadmeta", nil)
if err != nil {
    return err
}
defer du.UnsubscribeFromContainerEvents("workloadmeta")

for event := range containerCh {
    switch event.Action {
    case docker.ActionDied, events.ActionDie:
        handleStop(event.ContainerID)
    case events.ActionStart:
        handleStart(event.ContainerID)
    }
}
```

### Resolving image names (Docker check)

```go
// pkg/collector/corechecks/containers/docker/check.go
name, err := du.ResolveImageName(ctx, container.Image)
```

### Storage stats (Docker check)

```go
storageStats, err := du.GetStorageStats(ctx)
for _, s := range storageStats {
    pct := s.GetPercentUsed()
}
```

### Testing

Call `docker.EnableTestingMode()` in `TestMain` to avoid connecting to a real
daemon. For full mock control, instantiate a type that satisfies `docker.Client`
using the helpers in `client_mock.go` (generated by mockery).

---

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `comp/core/workloadmeta` | [comp/core/workloadmeta.md](../../comp/core/workloadmeta.md) | The `docker` workloadmeta collector (`comp/core/workloadmeta/collectors/internal/docker`) uses `DockerUtil` as its primary runtime client. It calls `AllContainerLabels`, `Inspect`, `Images`, and subscribes to events via `SubscribeToEvents` / `LatestContainerEvents`. The collector translates Docker API responses into `workloadmeta.Container` and `workloadmeta.ContainerImageMetadata` entities pushed to the store. |
| `pkg/util/containers` | [pkg/util/containers.md](containers.md) | The `metrics/docker` collector (inside `pkg/util/containers/metrics/docker`) wraps `DockerUtil.GetContainerStats` to expose Docker-API-backed CPU, memory, and I/O stats through the generic `Collector` interface. `EnvVarFilterFromConfig()` is applied when the workloadmeta Docker collector reads container environment variables. |
| `pkg/util/containerd` | [pkg/util/containerd.md](containerd.md) | Docker and containerd are parallel runtime backends in the agent. Both implement the same workloadmeta collector pattern and expose their runtime data through the `metrics/provider` abstraction. On hosts running both runtimes, the `system` cgroup collector (backed by `pkg/util/cgroups`) takes priority over both for container stats. |
