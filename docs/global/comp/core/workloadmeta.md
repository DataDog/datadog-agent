> **TL;DR:** `comp/core/workloadmeta` is the central in-memory store for workload metadata (containers, pods, ECS tasks, processes, images, GPUs), merging data from multiple runtime collectors and distributing lifecycle events to downstream components via a publish-subscribe model.

# Component `comp/core/workloadmeta`

## Purpose

`comp/core/workloadmeta` is the central in-memory store for metadata about all workloads the agent observes. A "workload" is any unit of work tracked by the agent: a container, a Kubernetes pod, an ECS task, a process, a container image, a GPU device, and more. The component collects data from multiple sources (container runtimes, orchestrators, cloud provider APIs) and merges it into a single consistent view.

Other agent components subscribe to this store to react to workload lifecycle events, or query it directly to read current state. The tagger, autodiscovery, log pipeline, and most check implementations all depend on workloadmeta.

## Key Elements

### Key interfaces

#### Component interface (`comp/core/workloadmeta/def`)

The component is accessed via `workloadmeta.Component`:

```go
type Component interface {
    // Subscription
    Subscribe(name string, priority SubscriberPriority, filter *Filter) chan EventBundle
    Unsubscribe(ch chan EventBundle)

    // Containers
    GetContainer(id string) (*Container, error)
    ListContainers() []*Container
    ListContainersWithFilter(filter EntityFilterFunc[*Container]) []*Container

    // Kubernetes
    GetKubernetesPod(id string) (*KubernetesPod, error)
    GetKubernetesPodForContainer(containerID string) (*KubernetesPod, error)
    GetKubernetesPodByName(podName, podNamespace string) (*KubernetesPod, error)
    ListKubernetesPods() []*KubernetesPod
    GetKubernetesDeployment(id string) (*KubernetesDeployment, error)
    GetKubernetesMetadata(id KubeMetadataEntityID) (*KubernetesMetadata, error)
    ListKubernetesMetadata(filterFunc EntityFilterFunc[*KubernetesMetadata]) []*KubernetesMetadata
    GetKubeletMetrics() (*KubeletMetrics, error)
    GetKubeCapabilities() (*KubeCapabilities, error)

    // ECS
    GetECSTask(id string) (*ECSTask, error)
    ListECSTasks() []*ECSTask

    // Images
    GetImage(id string) (*ContainerImageMetadata, error)
    ListImages() []*ContainerImageMetadata

    // Processes
    GetProcess(pid int32) (*Process, error)
    ListProcesses() []*Process
    ListProcessesWithFilter(filterFunc EntityFilterFunc[*Process]) []*Process
    GetContainerForProcess(processID string) (*Container, error)

    // GPU
    GetGPU(id string) (*GPU, error)
    ListGPUs() []*GPU

    // Collector interface
    Notify(events []CollectorEvent)
    Push(source Source, events ...Event) error
    Reset(newEntities []Entity, source Source)
    ResetProcesses(newProcesses []Entity, source Source)

    // Diagnostics
    Dump(verbose bool) WorkloadDumpResponse
    DumpStructured() WorkloadDumpStructuredResponse
    IsInitialized() bool
}
```

### Key types

#### Entity kinds (`Kind`)

Each stored object belongs to a `Kind`:

| Constant | String value |
|---|---|
| `KindContainer` | `container` |
| `KindKubernetesPod` | `kubernetes_pod` |
| `KindKubernetesMetadata` | `kubernetes_metadata` |
| `KindKubernetesDeployment` | `kubernetes_deployment` |
| `KindKubeletMetrics` | `kubelet_metrics` |
| `KindKubeCapabilities` | `kubernetes_capabilities` |
| `KindECSTask` | `ecs_task` |
| `KindContainerImageMetadata` | `container_image_metadata` |
| `KindProcess` | `process` |
| `KindGPU` | `gpu` |
| `KindKubelet` | `kubelet` |
| `KindCRD` | `crd` |

#### Sources (`Source`)

Data can come from multiple sources; workloadmeta merges them into one entity:

| Constant | Provided by |
|---|---|
| `SourceRuntime` | Docker, containerd, CRI-O, Podman, ECS Fargate |
| `SourceNodeOrchestrator` | Kubelet, ECS |
| `SourceClusterOrchestrator` | kube_metadata, CloudFoundry |
| `SourceNVML` | NVML GPU collector |
| `SourceKubeAPIServer` | Kubernetes API server |
| `SourceRemoteWorkloadmeta` | Remote workloadmeta (for process agent etc.) |
| `SourceHost` | Host-level tags |
| `SourceServiceDiscovery` | Service discovery for processes |

### Key functions

#### Events and subscriptions

Subscribers receive `EventBundle` values on a channel. Each bundle contains one or more `Event` values:

- `EventTypeSet` — an entity was added or its data was updated.
- `EventTypeUnset` — an entity was removed (all sources stopped reporting it).

The first bundle delivered to a new subscriber contains a `EventTypeSet` for every entity currently in the store, giving a snapshot of the current workload.

**Building a filter:**

```go
filter := workloadmeta.NewFilterBuilder().
    AddKind(workloadmeta.KindContainer).
    AddKind(workloadmeta.KindKubernetesPod).
    SetSource(workloadmeta.SourceAll).
    SetEventType(workloadmeta.EventTypeAll).
    Build()

ch := wmeta.Subscribe("my-component", workloadmeta.NormalSubscriberPriority, filter)
defer wmeta.Unsubscribe(ch)

for bundle := range ch {
    bundle.Acknowledge()
    for _, event := range bundle.Events {
        switch event.Type {
        case workloadmeta.EventTypeSet:
            // entity created or updated
        case workloadmeta.EventTypeUnset:
            // entity removed
        }
    }
}
```

A `nil` filter matches all events for all kinds from all sources.

#### Collectors (`Collector` interface)

Collectors are the source of data. Each collector implements:

```go
type Collector interface {
    Start(ctx context.Context, store Component) error
    Pull(ctx context.Context) error
    GetID() string
    GetTargetCatalog() AgentType
}
```

Collectors are registered via the fx value group `"workloadmeta"` using `CollectorProvider`:

```go
type CollectorProvider struct {
    fx.Out
    Collector Collector `group:"workloadmeta"`
}
```

Built-in collectors live under `comp/core/workloadmeta/collectors/internal/` and include: `containerd`, `docker`, `crio`, `podman`, `kubelet`, `kubeapiserver`, `ecs`, `nvml`, `process`, and `cloudfoundry`.

### Configuration and build flags

#### AgentType

`Params.AgentType` (a bitmask) controls which collectors are activated:

| Constant | Value |
|---|---|
| `NodeAgent` | `1 << 0` |
| `ClusterAgent` | `1 << 1` |
| `Remote` | `1 << 2` |

Each collector declares `GetTargetCatalog()` to indicate which agent types it should run in.

## fx Wiring

The standard (local store) module:

```go
// from comp/core/workloadmeta/fx
workloadmetafx.Module(workloadmeta.NewParams())
// or, when AgentType comes from a component config:
workloadmetafx.ModuleWithProvider(func(cfg myconfig.Component) workloadmeta.Params {
    return workloadmeta.Params{AgentType: workloadmeta.NodeAgent}
})
```

The module provides both `workloadmeta.Component` and `option.Option[workloadmeta.Component]`.

Additional modules:
- `comp/core/workloadmeta/fx-mock` — test mock
- Remote mode (process-agent, dogstatsd): use `comp/core/workloadmeta/impl` directly with a remote collector

## Relationship to other components

### Collectors that feed workloadmeta

Each built-in collector wraps a lower-level runtime client:

| Collector | Lower-level package | What it provides |
|---|---|---|
| `containerd` | [`pkg/util/containerd`](../../pkg/util/containerd.md) | `Container`, `ContainerImageMetadata` entities; subscribes to containerd events via `GetEvents()` |
| `docker` | [`pkg/util/docker`](../../pkg/util/docker.md) | `Container` and image entities; subscribes via `SubscribeToEvents` and polls with `LatestContainerEvents` |
| `kubelet` / `kubeapiserver` | [`pkg/util/kubernetes`](../../pkg/util/kubernetes.md) | `KubernetesPod`, `KubernetesDeployment`, `KubernetesMetadata` entities; uses `ParseDeploymentForReplicaSet` and `GetStandardTags` |
| `containerd` (images) | [`pkg/sbom`](../../pkg/sbom.md) | Triggers SBOM scans for new `ContainerImageMetadata` entities; scan results are stored back as `workloadmeta.SBOM` |

### Components that consume workloadmeta

| Consumer | How it uses workloadmeta |
|---|---|
| [`comp/core/tagger`](tagger.md) | Subscribes to all entity kinds to drive tag collection; each workloadmeta entity kind maps to a tagger `EntityID` prefix (e.g. `KindContainer` → `container_id://`) |
| [`comp/core/autodiscovery`](autodiscovery.md) | Listeners backed by workloadmeta generate services that are matched against config templates |
| Container checks (`pkg/collector/corechecks/containers/*`) | Subscribes to `KindContainer` events and queries `GetContainer` / `ListContainers` for metrics collection |
| [`pkg/sbom`](../../pkg/sbom.md) | Reads `ContainerImageMetadata` from the store to drive scans; writes `SBOM` results back via `Push` |
| [`pkg/util/containers`](../../pkg/util/containers.md) | `MetaCollector` implementations query workloadmeta to resolve container IDs from PIDs and inodes |

## Usage

### Subscribing to workload events (typical check pattern)

```go
filter := workloadmeta.NewFilterBuilder().AddKind(workloadmeta.KindContainer).Build()
ch := wm.Subscribe("my-check", workloadmeta.NormalSubscriberPriority, filter)
defer wm.Unsubscribe(ch)

for bundle := range ch {
    bundle.Acknowledge()
    for _, event := range bundle.Events {
        container := event.Entity.(*workloadmeta.Container)
        // ...
    }
}
```

The docker check (`pkg/collector/corechecks/containers/docker`), CRI check, kubelet check, and SBOM check all follow this pattern.

### Point-in-time queries

```go
container, err := wm.GetContainer(containerID)
pods := wm.ListKubernetesPods()
processes := wm.ListProcessesWithFilter(func(p *workloadmeta.Process) bool {
    return p.ContainerID != ""
})
```

### Looking up container metadata from a tagger entity ID

The tagger uses entity IDs of the form `container_id://<sha>`. To retrieve the corresponding workloadmeta entity:

```go
entityID := types.NewEntityID(types.ContainerID, containerID)
// strip the prefix to get the raw container ID, then:
container, err := wm.GetContainer(containerID)
```

This round-trip is common in checks that receive a tagger entity from `EnrichTags` and need to read the full container spec.

### Writing a new collector

1. Implement `workloadmeta.Collector`.
2. Call `store.Notify([]workloadmeta.CollectorEvent{...})` to push events.
3. Export a `CollectorProvider` via the `"workloadmeta"` fx value group.
4. Add the collector to the appropriate catalog (`comp/core/workloadmeta/collectors/catalog/`).

When implementing a collector backed by containerd, use `pkg/util/containerd.ContainerdItf`; for Docker use `pkg/util/docker.Client`. Both packages expose `MockedContainerdClient` / `client_mock.go` for unit-testing without a running runtime.

### Debugging

```bash
# Print the current store contents
agent workload-list
agent workload-list --verbose  # includes per-source detail
```

Telemetry metrics are defined in `comp/core/workloadmeta/telemetry/`.
