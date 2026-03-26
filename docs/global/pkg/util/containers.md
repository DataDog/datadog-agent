> **TL;DR:** Shared container identity, filtering, and metrics collection subsystem that provides canonical entity naming, regexp-based include/exclude filtering, an environment-variable allowlist, and a pluggable multi-backend provider that collects per-container CPU, memory, I/O, and network statistics from cgroups, Docker, containerd, CRI, kubelet, or ECS Fargate.

# pkg/util/containers

## Purpose

`pkg/util/containers` provides shared container-related types, constants, and utilities used across the agent whenever container identity, filtering, or environment-variable selection is needed. It is also the parent package of the multi-backend container metrics subsystem under `pkg/util/containers/metrics`.

The root package focuses on:
- A canonical container entity name scheme (`container_id://<id>`) shared by the tagger, workload metadata, and autodiscovery.
- A regexp-based `Filter` that decides which containers to include or exclude from metrics, logs, or both, based on image name, container name, and Kubernetes namespace. The same filter type supports Kubernetes annotation-based exclusion.
- An environment-variable allowlist used when collecting container env vars for tagging.
- Helpers to identify pause/infrastructure containers.

The `metrics` sub-tree collects CPU, memory, I/O, network, and PID statistics for individual containers from whichever runtime is present on the host.

### Relationship to other packages

| Package / component | Role |
|---|---|
| [`pkg/util/cgroups`](cgroups.md) | Provides the raw Linux cgroup v1/v2 filesystem parser consumed by the `metrics/system` collector. The `system` collector is the highest-priority collector on Linux; its `ContainerFilter` and `Reader` types are driven directly from `pkg/util/cgroups`. |
| [`pkg/util/containerd`](containerd.md) | The `metrics/containerd` collector calls `ContainerdItf.TaskMetrics` and `TaskPids` to populate the generic `ContainerStats` types. The `comp/core/workloadmeta/collectors/internal/containerd` collector uses `EnvVarFilterFromConfig()` to decide which env vars to store. |
| [`pkg/util/docker`](docker.md) | The `metrics/docker` and `metrics/ecsfargate` collectors use `DockerUtil` for Docker-API-backed stats. The workloadmeta Docker collector uses `containers.EnvVarFilterFromConfig()`. |
| [`comp/core/workloadmeta`](../../../comp/core/workloadmeta.md) | WorkloadMeta consumers (tagger, checks) obtain container IDs from this store and then pass them to `metrics.GetProvider().GetCollector()`. The `MetaCollector` implementations also query workloadmeta to resolve container IDs from PIDs/inodes. |

## Sub-packages

| Package | Description |
|---|---|
| `pkg/util/containers` | Entity naming, filtering, env-var allowlist, pause-container helpers |
| `pkg/util/containers/image` | Image name parsing (`SplitImageName`) and host-path sanitization |
| `pkg/util/containers/metadata` | Fan-out metadata fetch from CRI, Docker, and kubelet |
| `pkg/util/containers/metrics` | Facade that re-exports all provider types and registers all collectors |
| `pkg/util/containers/metrics/provider` | Core provider/collector abstraction, registry, cache, and stat types |
| `pkg/util/containers/metrics/system` | Linux cgroup v1/v2 collector (highest priority on Linux) |
| `pkg/util/containers/metrics/docker` | Docker API collector |
| `pkg/util/containers/metrics/containerd` | containerd gRPC collector |
| `pkg/util/containers/metrics/cri` | Generic CRI collector |
| `pkg/util/containers/metrics/kubelet` | kubelet `/stats/summary` collector |
| `pkg/util/containers/metrics/ecsfargate` | ECS Fargate task-metadata endpoint collector |
| `pkg/util/containers/metrics/ecsmanagedinstances` | ECS managed instances collector |
| `pkg/util/containers/metrics/mock` | Test-only mock provider and sample data |

## Key elements

### Root package (`pkg/util/containers`)

#### Key types

**Container filtering types**

| Symbol | Description |
|---|---|
| `ContainerEntityName` | `"container_id"` — the canonical entity-name prefix |
| `ContainerEntityPrefix` | `"container_id://"` — ready-to-use prefix string |
| `EntitySeparator` | `"://"` |
| `BuildEntityName(runtime, id)` | Builds `"<runtime>://<id>"` |
| `SplitEntityName(name)` | Splits a full entity name back into runtime and ID |
| `ShortContainerID(id)` | Returns first 12 characters of a container ID |

```go
type FilterType string
const (
    GlobalFilter  FilterType = "GlobalFilter"
    MetricsFilter FilterType = "MetricsFilter"
    LogsFilter    FilterType = "LogsFilter"
)

type Filter struct {
    FilterType           FilterType
    Enabled              bool
    ImageIncludeList     []*regexp.Regexp
    NameIncludeList      []*regexp.Regexp
    NamespaceIncludeList []*regexp.Regexp
    ImageExcludeList     []*regexp.Regexp
    NameExcludeList      []*regexp.Regexp
    NamespaceExcludeList []*regexp.Regexp
    Errors               map[string]struct{}
}
```

Each filter entry is a string of the form `"image:<regex>"`, `"name:<regex>"`, or `"kube_namespace:<regex>"`. Include-list entries take precedence over exclude-list entries. Annotation-based exclusion (`ad.datadoghq.com/exclude`, `ad.datadoghq.com/<container>.metrics_exclude`, etc.) is checked first, independently of the lists.

`Filter.GetResult()` returns a `workloadfilter.Result` (`Included`, `Excluded`, or `Unknown`). `Filter.IsExcluded()` is a convenience wrapper.

**`EnvFilter`** — environment variable allowlist type. `EnvVarFilterFromConfig()` (singleton) merges `docker_env_as_tags` and `container_env_as_tags` config maps with a built-in allowlist. Default allowed variables include Datadog unified service tagging vars (`DD_ENV`, `DD_SERVICE`, `DD_VERSION`), OTEL vars, ECS/Fargate metadata URIs, Nomad job info, and Chronos job info.

#### Key functions

**Entity naming**

| Symbol | Description |
|---|---|
| `ContainerEntityName` | `"container_id"` — the canonical entity-name prefix |
| `ContainerEntityPrefix` | `"container_id://"` — ready-to-use prefix string |
| `EntitySeparator` | `"://"` |
| `BuildEntityName(runtime, id)` | Builds `"<runtime>://<id>"` |
| `SplitEntityName(name)` | Splits a full entity name back into runtime and ID |
| `ShortContainerID(id)` | Returns first 12 characters of a container ID |

**Filter constructors and helpers**

| Function | Description |
|---|---|
| `NewFilter(ft, includeList, excludeList)` | Creates a compiled `Filter` from string slices |
| `GetPauseContainerFilter()` | Returns a `GlobalFilter` that excludes all known pause-container images when `exclude_pause_container` is `true` |
| `GetPauseContainerExcludeList()` | Returns the built-in list of ~18 pause image patterns |
| `IsPauseContainer(labels)` | Label-based pause detection (Kubernetes/containerd compatible) |
| `IsExcludedByAnnotationInner(...)` | Low-level annotation check used by `Filter` |

**Environment variable allowlist**

```go
func EnvVarFilterFromConfig() EnvFilter
func (f EnvFilter) IsIncluded(envVarName string) bool
```

### Metrics sub-system (`pkg/util/containers/metrics`)

The `metrics` package is a thin facade. Import it to pull all collectors into the binary via side-effect imports; use `metrics.GetProvider(wmeta)` (or the `provider` package directly) for the actual API.

#### Key interfaces

**Provider and registry (`metrics/provider`)**

**`Provider` interface**

```go
type Provider interface {
    GetCollector(RuntimeMetadata) Collector
    GetMetaCollector() MetaCollector
}
```

`GetProvider(wmeta)` returns the process-wide singleton. Do not cache the `Collector` returned by `GetCollector` — the best collector for a runtime can change as collectors finish initialization.

**`Collector` interface** — per-runtime stats

```go
type Collector interface {
    GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerStats, error)
    GetContainerNetworkStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerNetworkStats, error)
    GetContainerOpenFilesCount(containerNS, containerID string, cacheValidity time.Duration) (*uint64, error)
    GetPIDs(containerNS, containerID string, cacheValidity time.Duration) ([]int, error)
}
```

**`MetaCollector` interface** — cross-runtime identity resolution

```go
type MetaCollector interface {
    GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error)
    GetContainerIDForInode(inode uint64, cacheValidity time.Duration) (string, error)
    GetSelfContainerID() (string, error)
    ContainerIDForPodUIDAndContName(podUID, contName string, initCont bool, cacheValidity time.Duration) (string, error)
}
```

`MetaCollector` fans out to all registered collectors in priority order and returns the first non-empty result.

**Stat types** (all numeric fields are `*float64`; `nil` = not available)

| Type | Key fields |
|---|---|
| `ContainerStats` | `CPU`, `Memory`, `IO`, `PID`, `Timestamp` |
| `ContainerCPUStats` | `Total`, `User`, `System`, `Limit`, `ThrottledPeriods`, `ThrottledTime`, `PartialStallTime` |
| `ContainerMemStats` | `UsageTotal`, `WorkingSet`, `Cache`, `RSS`, `Limit`, `OOMEvents`, `PartialStallTime` |
| `ContainerIOStats` | `ReadBytes`, `WriteBytes`, `ReadOperations`, `WriteOperations`, per-device `Devices` |
| `ContainerNetworkStats` | `BytesSent`, `BytesRcvd`, per-interface `Interfaces` |
| `ContainerPIDStats` | `ThreadCount`, `ThreadLimit` |

**Runtime constants**

```go
const (
    RuntimeNameDocker              Runtime = "docker"
    RuntimeNameContainerd          Runtime = "containerd"
    RuntimeNameCRIO                Runtime = "cri-o"
    RuntimeNameGarden              Runtime = "garden"
    RuntimeNamePodman              Runtime = "podman"
    RuntimeNameECSFargate          Runtime = "ecsfargate"
    RuntimeNameECSManagedInstances Runtime = "ecsmanagedinstances"
    RuntimeNameCRINonstandard      Runtime = "cri-nonstandard"
)
```

`RuntimeFlavorKata` (`"kata"`) is the only non-default flavor today; it selects a collector variant for Kata Containers.

**Registering a new collector**

A collector registers itself in its package `init()`:

```go
func init() {
    provider.RegisterCollector(provider.CollectorFactory{
        ID: "my-collector",
        Constructor: func(cache *provider.Cache, wmeta option.Option[workloadmeta.Component]) (provider.CollectorMetadata, error) {
            // Return ErrPermaFail if this runtime is never available.
            // Return ErrNothingYet (wraps retry.FailWillRetry) to be retried.
            return provider.CollectorMetadata{
                ID: "my-collector",
                Collectors: provider.CollectorCatalog{
                    provider.NewRuntimeMetadata("my-runtime", ""): &provider.Collectors{
                        Stats:   provider.MakeRef[provider.ContainerStatsGetter](myImpl, priority),
                        Network: provider.MakeRef[provider.ContainerNetworkStatsGetter](myImpl, priority),
                    },
                },
            }, nil
        },
    })
}
```

The registry retries all pending factories every 2 seconds until all succeed or permanently fail.

**`Collectors` struct** — capabilities advertised by a collector implementation

```go
type Collectors struct {
    Stats             CollectorRef[ContainerStatsGetter]
    Network           CollectorRef[ContainerNetworkStatsGetter]
    OpenFilesCount    CollectorRef[ContainerOpenFilesCountGetter]
    PIDs              CollectorRef[ContainerPIDsGetter]
    // Used by MetaCollector:
    ContainerIDForPID               CollectorRef[ContainerIDForPIDRetriever]
    ContainerIDForInode             CollectorRef[ContainerIDForInodeRetriever]
    SelfContainerID                 CollectorRef[SelfContainerIDRetriever]
    ContainerIDForPodUIDAndContName CollectorRef[ContainerIDForPodUIDAndContNameRetriever]
}
```

Lower `Priority` value = higher precedence when multiple collectors support the same capability (0 beats 1 beats 10, etc.).

**`Cache` / `CacheWithKey`** — staleness-tolerant result cache

```go
func NewCache(gcInterval time.Duration) *Cache
func (c *Cache) Get(now time.Time, key string, cacheValidity time.Duration) (interface{}, bool, error)
func (c *Cache) Store(now time.Time, key string, value interface{}, err error)
```

Pass `cacheValidity = 0` to bypass the cache. The cache is GC'd (map replaced) every `gcInterval`.

#### Configuration and build flags

| Collector package | Build tag |
|---|---|
| `metrics/system` | `linux` (no extra tag; always compiled on Linux) |
| `metrics/docker` | `docker && (linux \|\| windows)` |
| `metrics/containerd` | `containerd && (linux \|\| windows)` |
| `metrics/cri` | `cri` |
| `metrics/kubelet` | `kubelet` |
| `metrics/ecsfargate` | `docker` |
| `metrics/ecsmanagedinstances` | `docker` |
| `metrics/mock` | `test` |

## Usage

### Filtering containers

```go
import "github.com/DataDog/datadog-agent/pkg/util/containers"

filter, err := containers.NewFilter(
    containers.MetricsFilter,
    pkgconfigsetup.Datadog().GetStringSlice("container_include_metrics"),
    pkgconfigsetup.Datadog().GetStringSlice("container_exclude_metrics"),
)
if filter.IsExcluded(pod.Annotations, ctrName, imageName, namespace) {
    continue
}
```

The `comp/core/workloadfilter` component wraps this into an injectable component for use within the FX dependency graph. Direct use of `containers.NewFilter` is typical in non-FX code such as security checks and admission webhooks.

### Collecting container metrics (standard path)

Import the facade to register all collectors, then call `GetProvider`:

```go
import (
    "github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
    "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
)

p := metrics.GetProvider(option.New(wmetaComponent))

collector := p.GetCollector(provider.NewRuntimeMetadata(
    string(container.Runtime),
    string(container.RuntimeFlavor),
))
if collector == nil {
    // runtime not yet available; retry next check run
    return
}

stats, err := collector.GetContainerStats(container.Namespace, container.ID, 2*time.Second)
```

Do not cache the `Collector` value across check runs — the provider can swap in a better collector as they come online.

### Resolving a container ID from a PID

```go
mc := p.GetMetaCollector()
cid, err := mc.GetContainerIDForPID(pid, 30*time.Second)
```

This is used by dogstatsd (origin detection), system-probe, and the process check.

### Who imports these packages

- **Container checks** (`pkg/collector/corechecks/containers/generic`, `docker`, `containerd`, `cri`) use `metrics.GetProvider` directly to collect per-container stats.
- **Workload metadata collectors** (`comp/core/workloadmeta/collectors/internal/*`) use `containers.EnvVarFilterFromConfig()` to decide which env vars to retain in container metadata. See [`comp/core/workloadmeta`](../../../comp/core/workloadmeta.md) for the full collector list.
- **Tagger** (`comp/core/tagger`) uses `containers.BuildEntityName` and `metrics.GetMetaCollector()`.
- **Workload filter** (`comp/core/workloadfilter`) wraps `containers.NewFilter` and `containers.IsExcludedByAnnotationInner`.
- **Security agent** (`pkg/security`) creates `containers.Filter` instances for process/container scoping.
- **dogstatsd** (`comp/dogstatsd`) uses `MetaCollector.GetContainerIDForPID` for origin detection on UDS.
- **Process check** (`pkg/process`) uses `MetaCollector.GetContainerIDForInode` and `GetContainerIDForPID`.
- **Log launchers** (`pkg/logs`) use `containers.Filter` to decide which containers to tail.

### Collector implementation details

The `metrics/system` collector (highest priority on Linux) creates a `cgroups.Reader` with `cgroups.ContainerFilter` to track per-container cgroup paths; see [`pkg/util/cgroups`](cgroups.md) for the cgroup filesystem API. On Windows or when cgroups are unavailable, the provider falls back to the `metrics/docker` or `metrics/containerd` collector.

The `metrics/containerd` collector wraps [`pkg/util/containerd`](containerd.md)'s `ContainerdItf.TaskMetrics` call. The `metrics/docker` and `metrics/ecsfargate` collectors wrap [`pkg/util/docker`](docker.md)'s `DockerUtil` client.

The `metrics/kubelet` collector fetches stats from the kubelet `/stats/summary` endpoint, sharing the connection managed by `pkg/util/kubernetes/kubelet`.

The `metrics/mock` package (build tag `test`) provides a `MockMetricsProvider` and `GetProvider` stub used in unit tests that exercise code depending on container stats without running a real runtime.
