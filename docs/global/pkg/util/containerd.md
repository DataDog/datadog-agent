> **TL;DR:** Agent-specific wrapper around the containerd gRPC client that provides automatic retry, namespace-scoped container and image queries, OCI spec parsing, event subscription, and image mounting for SBOM scanning, along with a mock client for unit testing.

# pkg/util/containerd

## Purpose

`pkg/util/containerd` provides a thin, agent-specific wrapper around the upstream `github.com/containerd/containerd` gRPC client. It is used to:

- Connect to the containerd socket with automatic exponential-backoff retry.
- Enumerate and filter containerd namespaces (controlled by `containerd_namespaces` / `containerd_exclude_namespaces`).
- Query containers, images, tasks (processes), labels, OCI specs, and container status within any namespace.
- Subscribe to containerd events (container create/update/delete, image create/update/delete, task start/exit/oom).
- Mount images to host directories for SBOM (software bill of materials) scanning.
- Expose a `MockedContainerdClient` (in `fake/`) for unit testing without a real containerd daemon.

## Build tag

All files in the package require the `containerd` build tag. The package is not compiled into agent binaries that do not include containerd support.

## Key elements

### Key types

**`ContainerdUtil`** struct — the concrete implementation of `ContainerdItf`. Not instantiated directly; use `NewContainerdUtil()`.

**`MockedContainerdClient`** — implements `ContainerdItf` via function fields for unit testing. See `### Fake client` below.

**Constants**

| Constant | Value | Use |
|---|---|---|
| `DefaultAllowedSpecMaxSize` | `2 * 1024 * 1024` | Recommended `maxSize` argument to `Spec()` |

**Sentinel error**: `ErrSpecTooLarge` — returned when the OCI spec exceeds `maxSize`.

### Key interfaces

### `ContainerdItf` interface (`containerd_util.go`)

The central interface that every consumer programs against. It covers the full set of operations needed by the agent:

```go
type ContainerdItf interface {
    RawClient() *containerd.Client
    Close() error
    CheckConnectivity() *retry.Error

    // Container operations
    Container(namespace, id string) (containerd.Container, error)
    ContainerWithContext(ctx context.Context, namespace, id string) (containerd.Container, error)
    Containers(namespace string) ([]containerd.Container, error)
    Info(namespace string, ctn containerd.Container) (containers.Container, error)
    Labels(namespace string, ctn containerd.Container) (map[string]string, error)
    LabelsWithContext(ctx context.Context, namespace string, ctn containerd.Container) (map[string]string, error)
    Spec(namespace string, ctn containers.Container, maxSize int) (*oci.Spec, error)
    Status(namespace string, ctn containerd.Container) (containerd.ProcessStatus, error)
    IsSandbox(namespace string, ctn containerd.Container) (bool, error)

    // Task (process) operations
    TaskMetrics(namespace string, ctn containerd.Container) (*types.Metric, error)
    TaskPids(namespace string, ctn containerd.Container) ([]containerd.ProcessInfo, error)

    // Image operations
    ListImages(namespace string) ([]containerd.Image, error)
    ListImagesWithDigest(namespace, digest string) ([]containerd.Image, error)
    Image(namespace, name string) (containerd.Image, error)
    ImageOfContainer(namespace string, ctn containerd.Container) (containerd.Image, error)
    ImageSize(namespace string, ctn containerd.Container) (int64, error)
    MountImage(ctx context.Context, expiration time.Duration, namespace string, img containerd.Image, targetDir string) (func(context.Context) error, error)
    Mounts(ctx context.Context, expiration time.Duration, namespace string, img containerd.Image) ([]mount.Mount, func(context.Context) error, error)

    // Misc
    GetEvents() containerd.EventService
    Metadata() (containerd.Version, error)
    Namespaces(ctx context.Context) ([]string, error)
    CallWithClientContext(namespace string, f func(context.Context) error) error
}
```

Every method that takes a namespace prepends it to the context using `namespaces.WithNamespace` before forwarding to the underlying containerd client.

### `ContainerdUtil` struct and constructor

```go
type ContainerdUtil struct { /* private */ }

func NewContainerdUtil() (ContainerdItf, error)
```

`NewContainerdUtil` reads configuration (`cri_socket_path`, `cri_query_timeout`, `cri_connection_timeout`) and sets up a `retry.Retrier` with exponential backoff (1s initial, 5m max). It does **not** return a singleton — callers that need concurrent access from different namespaces (workloadmeta, checks, etc.) should create separate instances.

The default socket path is `/var/run/containerd/containerd.sock`. It can be overridden via `cri_socket_path`.

### Key functions

```go
func EnvVarsFromSpec(spec *oci.Spec, filter func(string) bool) (map[string]string, error)
```

Parses `spec.Process.Env` into a map. The optional `filter` function receives the variable name and returns `true` to include it. Useful for selectively extracting environment variables without exposing the full env to callers.

### Sandbox detection

```go
func (c *ContainerdUtil) IsSandbox(namespace string, ctn containerd.Container) (bool, error)
```

Returns `true` when the `io.cri-containerd.kind` label equals `"sandbox"` (identifies Kubernetes pause containers).

### Image mounting (`Mounts`, `MountImage`)

`Mounts` creates a read-only overlay snapshot of an image's top filesystem layer with a lease to prevent garbage collection during the scan. It handles both the `nydus` snapshotter and the `containerd.DefaultSnapshotter`, and sanitises host paths when the agent itself runs containerised (prefixing paths with the container host root). The returned cleanup function must be called to remove the snapshot and release the lease.

`MountImage` calls `Mounts` then calls `mount.All` to bind the mounts to `targetDir`. The returned cleanup function unmounts the directory and releases the lease.

### Namespace helpers (`namespaces.go`)

```go
func NamespacesToWatch(ctx context.Context, containerdClient ContainerdItf) ([]string, error)
func FiltersWithNamespaces(filters []string) []string
```

`NamespacesToWatch` returns the configured `containerd_namespaces` slice, or all namespaces minus `containerd_exclude_namespaces` when no explicit allowlist is set.

`FiltersWithNamespaces` rewrites containerd event filter expressions (e.g. `topic=="/container/create"`) to add `namespace=="ns1"` (allowlist) or `namespace!="ns1"` (denylist) clauses, enabling namespace-scoped event subscriptions.

### Configuration and build flags

All files in the package require the `containerd` build tag.

| Key | Description |
|---|---|
| `cri_socket_path` | Path to the containerd socket |
| `cri_query_timeout` | Timeout in seconds for individual API calls |
| `cri_connection_timeout` | Timeout in seconds for the initial connection |
| `containerd_namespaces` | Explicit list of namespaces to watch |
| `containerd_exclude_namespaces` | Namespaces to exclude when watching all |

### Fake client (`fake/containerd_util.go`)

```go
type MockedContainerdClient struct {
    MockContainers func(namespace string) ([]containerd.Container, error)
    MockLabels     func(namespace string, ctn containerd.Container) (map[string]string, error)
    // ... one field per ContainerdItf method
}
```

`MockedContainerdClient` implements `ContainerdItf` via function fields. Tests assign only the fields they need; unset fields panic when called (making missing mock setup immediately visible).

## Usage

`pkg/util/containerd` is used by the agent's container runtime integration:

- **`comp/core/workloadmeta/collectors/internal/containerd`**: the containerd workloadmeta collector creates a `ContainerdUtil`, subscribes to events via `GetEvents()`, and uses `Containers`, `Info`, `Labels`, `Spec`, `Status`, `TaskMetrics`, `TaskPids`, `IsSandbox`, `ImageOfContainer`, and `ImageSize` to build and maintain `workloadmeta.Container` and `workloadmeta.ContainerImage` entities.
- **`pkg/collector/corechecks/containers/containerd`**: the containerd core check uses `NewContainerdUtil` directly to collect container CPU, memory, and I/O metrics; it also subscribes to task lifecycle events for the `containerd.event` check.
- **`pkg/util/containers/metrics/containerd`**: the containerd metrics collector calls `TaskMetrics` and `TaskPids` to populate the generic container metrics interface used by the container check.
- **`pkg/sbom/collectors/containerd`** and **`pkg/util/trivy/containerd`**: SBOM and vulnerability scanning use `ListImages`, `MountImage`, and `Mounts` to access image layers.
- **`comp/core/workloadmeta/collectors/internal/containerd/container_builder`**: uses `EnvVarsFromSpec` to extract environment variables from OCI specs for autodiscovery and tagging.

Typical usage pattern:

```go
import cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"

client, err := cutil.NewContainerdUtil()
if err != nil {
    return err
}
defer client.Close()

namespaces, err := cutil.NamespacesToWatch(ctx, client)
for _, ns := range namespaces {
    ctns, err := client.Containers(ns)
    for _, ctn := range ctns {
        info, err := client.Info(ns, ctn)
        spec, err := client.Spec(ns, info, cutil.DefaultAllowedSpecMaxSize)
        envs, err := cutil.EnvVarsFromSpec(spec, nil)
    }
}
```

In tests, substitute `ContainerdItf` with `fake.MockedContainerdClient`:

```go
import "github.com/DataDog/datadog-agent/pkg/util/containerd/fake"

client := &fake.MockedContainerdClient{
    MockContainers: func(ns string) ([]containerd.Container, error) {
        return []containerd.Container{...}, nil
    },
}
```
