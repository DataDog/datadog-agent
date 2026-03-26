> **TL;DR:** `comp/core/tagger` is the central source of entity tags throughout the agent, deriving low/orchestrator/high-cardinality tag sets from workloadmeta events and serving them to metrics, logs, traces, and DogStatsD enrichment.

# Component `comp/core/tagger`

## Purpose

`comp/core/tagger` is the central source of truth for entity tags throughout the agent. It subscribes to `workloadmeta` to learn about running workloads (containers, pods, ECS tasks, processes, GPUs, …), extracts tags for each entity from multiple collector sources, and stores them in an in-memory tag store. Any agent component that needs to attach tags to a metric, log, or trace queries the tagger.

Tags are grouped by **cardinality**, which controls how many distinct tag values a tag can generate across the fleet:

| Cardinality | Typical use |
|---|---|
| `LowCardinality` | Host-level metrics, most pipelines |
| `OrchestratorCardinality` | Per-pod or per-task metrics |
| `HighCardinality` | Per-container or per-request metrics |

## Key Elements

### Key interfaces

#### Component interface (`comp/core/tagger/def`)

```go
type Component interface {
    Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error)
    TagWithCompleteness(entityID types.EntityID, cardinality types.TagCardinality) ([]string, bool, error)
    Standard(entityID types.EntityID) ([]string, error)
    GetEntity(entityID types.EntityID) (*types.Entity, error)
    List() types.TaggerListResponse
    Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error)
    GetEntityHash(entityID types.EntityID, cardinality types.TagCardinality) string
    AgentTags(cardinality types.TagCardinality) ([]string, error)
    GlobalTags(cardinality types.TagCardinality) ([]string, error)
    EnrichTags(tb tagset.TagsAccumulator, originInfo taggertypes.OriginInfo)
    GenerateContainerIDFromOriginInfo(originInfo origindetection.OriginInfo) (string, error)
}
```

**Key methods:**

| Method | Description |
|---|---|
| `Tag(entityID, cardinality)` | Returns all tags for an entity up to the given cardinality level. Tags accumulate: `HighCardinality` includes orchestrator and low tags too. |
| `TagWithCompleteness(...)` | Same as `Tag` but also returns a bool indicating whether all expected collectors have reported data for the entity. For K8s, this includes the pod's completeness. |
| `Standard(entityID)` | Returns only the standard tags: `env`, `version`, `service`. |
| `GetEntity(entityID)` | Returns the full `types.Entity` struct with tags split by cardinality. |
| `Subscribe(id, filter)` | Returns a `Subscription` whose `EventsChan()` delivers `[]EntityEvent` whenever a matching entity's tags change. |
| `AgentTags(cardinality)` | Tags for the agent container itself (if running in a container). |
| `GlobalTags(cardinality)` | Static host-level tags used when host tags are absent (e.g., serverless). |
| `EnrichTags(tb, originInfo)` | Resolves the origin of a DogStatsD packet to an entity and appends its tags to the accumulator. |

### Key types

#### EntityID (`comp/core/tagger/types`)

Entity IDs have the form `{prefix}://{id}`. The valid prefixes are:

| Prefix constant | String | Workload kind |
|---|---|---|
| `ContainerID` | `container_id` | `workloadmeta.KindContainer` |
| `KubernetesPodUID` | `kubernetes_pod_uid` | `workloadmeta.KindKubernetesPod` |
| `ECSTask` | `ecs_task` | `workloadmeta.KindECSTask` |
| `KubernetesDeployment` | `deployment` | `workloadmeta.KindKubernetesDeployment` |
| `KubernetesMetadata` | `kubernetes_metadata` | `workloadmeta.KindKubernetesMetadata` |
| `ContainerImageMetadata` | `container_image_metadata` | `workloadmeta.KindContainerImageMetadata` |
| `Process` | `process` | `workloadmeta.KindProcess` |
| `GPU` | `gpu` | `workloadmeta.KindGPU` |
| `Host` | `host` | — (host entity) |
| `InternalID` | `internal` | Internal entities (e.g., global tags) |

Build an `EntityID` with `types.NewEntityID(prefix, id)`, or parse one from a string with `types.ExtractPrefixAndID(str)`.

#### TagInfo and TagStore

Collectors (inside `taggerimpl`) produce `types.TagInfo` structs:

```go
type TagInfo struct {
    Source               string
    EntityID             EntityID
    HighCardTags         []string
    OrchestratorCardTags []string
    LowCardTags          []string
    StandardTags         []string
    DeleteEntity         bool
    ExpiryDate           time.Time
    IsComplete           bool
}
```

The **TagStore** (`comp/core/tagger/tagstore`) ingests these and serves queries. When multiple sources report tags for the same entity, they are stored separately per source and merged at query time according to `CollectorPriority` (`NodeRuntime < NodeOrchestrator < ClusterOrchestrator`).

#### Entity

```go
type Entity struct {
    ID                          EntityID
    HighCardinalityTags         []string
    OrchestratorCardinalityTags []string
    LowCardinalityTags          []string
    StandardTags                []string
    IsComplete                  bool
}
```

`entity.GetTags(cardinality)` flattens the appropriate tag slices into a single `[]string`.

#### Subscriptions

`Subscribe` returns a `types.Subscription` interface:

```go
type Subscription interface {
    EventsChan() chan []EntityEvent
    ID() string
    Unsubscribe()
}
```

Events have type `EventTypeAdded`, `EventTypeModified`, or `EventTypeDeleted`.

#### TagCardinality

```go
const (
    LowCardinality          TagCardinality = iota
    OrchestratorCardinality
    HighCardinality
    NoneCardinality
    ChecksConfigCardinality  // alias for the checks_tag_cardinality setting
)
```

Convert from string with `types.StringToTagCardinality(s)`.

## fx Wiring

The tagger has several fx modules, one per deployment scenario:

| Module path | Use case |
|---|---|
| `comp/core/tagger/fx` | Local (node agent): subscribes directly to workloadmeta |
| `comp/core/tagger/fx-remote` | Remote: streams tags from the node agent over gRPC. Requires `RemoteParams`. |
| `comp/core/tagger/fx-dual` | Dual: starts as local, falls back to remote based on a runtime flag. Requires `DualParams` + `RemoteParams`. |
| `comp/core/tagger/fx-optional-remote` | Optional remote: uses the noop tagger if the remote is disabled. Requires `OptionalRemoteParams`. |
| `comp/core/tagger/fx-noop` | No-op tagger that always returns empty tags. For lightweight binaries. |
| `comp/core/tagger/fx-mock` | Test mock. |

**Local tagger (node agent):**

```go
taggerfx.Module()
```

**Remote tagger (process agent, dogstatsd):**

```go
taggerfxremote.Module(tagger.NewRemoteParams(
    tagger.WithRemoteTarget(func(cfg config.Component) (string, error) {
        return fmt.Sprintf(":%d", cfg.GetInt("cmd_port")), nil
    }),
))
```

**Dual tagger (cluster agent running CLC):**

```go
taggerfxdual.Module(
    tagger.DualParams{UseRemote: func(cfg config.Component) bool { ... }},
    tagger.NewRemoteParams(...),
)
```

All modules provide both `tagger.Component` and `option.Option[tagger.Component]`.

## Usage

### Tagging a metric in a check

```go
tags, err := t.Tag(types.NewEntityID(types.ContainerID, containerID), types.LowCardinality)
if err != nil {
    return err
}
sender.Gauge("my.metric", value, "", tags)
```

### Enriching DogStatsD metrics

```go
var tb tagset.NewHashingTagsAccumulator()
taggerComp.EnrichTags(tb, originInfo)
// tb now contains all tags for the origin container
```

### Subscribing to tag changes

```go
sub, err := taggerComp.Subscribe("my-subscriber", types.NewMatchAllFilter())
if err != nil { ... }
defer sub.Unsubscribe()

for events := range sub.EventsChan() {
    for _, ev := range events {
        switch ev.EventType {
        case types.EventTypeAdded, types.EventTypeModified:
            // ev.Entity has updated tags
        case types.EventTypeDeleted:
            // ev.Entity was removed
        }
    }
}
```

### Accessing from Python checks

The tagger is available to Python checks via the `datadog_agent` module:

```python
tags = datadog_agent.get_tags("container_id://abc123", False)  # False = low cardinality
```

### Checking tag completeness

For Kubernetes containers, use `TagWithCompleteness` to delay sending metrics until all collectors have reported:

```go
tags, complete, err := t.TagWithCompleteness(entityID, types.LowCardinality)
if !complete {
    // data may be partial; decide whether to skip or use partial tags
}
```

## Relationship to other components

### Upstream: workloadmeta

The tagger's local implementation subscribes to [`comp/core/workloadmeta`](workloadmeta.md) to learn about every running workload. Each workloadmeta entity kind has a corresponding tagger `EntityID` prefix:

| workloadmeta kind | tagger prefix | Example entity ID |
|---|---|---|
| `KindContainer` | `container_id` | `container_id://abc123` |
| `KindKubernetesPod` | `kubernetes_pod_uid` | `kubernetes_pod_uid://uid` |
| `KindECSTask` | `ecs_task` | `ecs_task://task-id` |
| `KindKubernetesDeployment` | `deployment` | `deployment://ns/name` |
| `KindContainerImageMetadata` | `container_image_metadata` | `container_image_metadata://sha` |
| `KindProcess` | `process` | `process://1234` |
| `KindGPU` | `gpu` | `gpu://uuid` |

The workloadmeta → tagger data flow is unidirectional: workloadmeta produces entity events; the tagger derives tag sets from them and stores them in the tag store.

### Downstream: tag consumers

| Consumer | How it uses the tagger |
|---|---|
| DogStatsD (`comp/dogstatsd`) | Calls `EnrichTags(tb, originInfo)` on every inbound metric to append container tags. `OriginInfo` is defined in [`pkg/tagger`](../../pkg/tagger.md). |
| Aggregator (`pkg/aggregator`) and metrics types (`pkg/metrics`) | Carry `taggertypes.OriginInfo` alongside metric samples; the tag accumulator type comes from [`pkg/tagset`](../../pkg/tagset.md). |
| Container checks | Call `Tag(entityID, cardinality)` to attach infrastructure tags to each metric series. |
| Log agent | Subscribes via `Subscribe` to re-tag log entries when pod labels change. |
| Autodiscovery (`comp/core/autodiscovery`) | Reads `TaggerEntity` from `integration.Config` during template resolution to enrich configs with current tags. |

### Tag accumulation: pkg/tagset

The tagger stores resolved tag sets as `tagset.HashedTags` and passes them through `tagset.TagsAccumulator` to callers. See [`pkg/tagset`](../../pkg/tagset.md) for details on the `HashingTagsAccumulator`, `CompositeTags`, and `HashGenerator` types used in the aggregator's hot path.

### pkg/tagger boundary package

[`pkg/tagger`](../../pkg/tagger.md) exists solely to provide the `OriginInfo` type to packages inside `pkg/` that must not import the full tagger component. It contains no tagger logic.

### pkg/util/containers

[`pkg/util/containers`](../../pkg/util/containers.md) provides the `ContainerEntityPrefix` constant (`"container_id://"`) and `BuildEntityName` helper used when constructing entity IDs. The `MetaCollector` inside `pkg/util/containers/metrics` calls `GetContainerIDForPID` / `GetContainerIDForInode`, which the tagger uses for origin detection.

## IPC / remote tagger

When the process agent or dogstatsd needs tags, they connect to the node agent's tagger gRPC server instead of running a local tagger. The server is defined in `comp/core/tagger/server/` and streams `types.EntityEvent` messages. The remote tagger client (`comp/core/tagger/impl-remote/`) reconnects automatically.
