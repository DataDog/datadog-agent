# pkg/tagger

## Purpose

`pkg/tagger` is a thin boundary package that exposes shared types used for **origin detection** when the tagger is invoked by DogStatsD, APM, or the OTel collector. The bulk of the tagger implementation lives in `comp/core/tagger`; this package only contains the types that cross the boundary between the metrics pipeline and the tagger component without creating a heavy import dependency.

The tagger's job is to **enrich metrics, logs, and traces with infrastructure tags** — things like `container_id`, `kube_pod_name`, `env`, `service`, `version`, and dozens of Kubernetes/ECS labels — by resolving them from the entity that produced the telemetry.

---

## Key elements

### pkg/tagger (root)

A near-empty package (`docs.go`) with one purpose: provide the `OriginInfo` type to packages in `pkg/` that must not import the full tagger component.

| Symbol | Kind | Description |
|--------|------|-------------|
| `OriginInfo` | struct | Carries origin-detection data from a metrics/trace producer to the tagger for tag resolution. |

**File:** `pkg/tagger/types/types.go`

```go
type OriginInfo struct {
    ContainerIDFromSocket string                        // container ID resolved via Unix Domain Socket (UDS)
    LocalData             origindetection.LocalData     // container ID or inode number sent inline with the payload
    ExternalData          origindetection.ExternalData  // pod UID + container name sent in an env var (injected by the admission webhook)
    Cardinality           string                        // requested tag cardinality ("low", "orchestrator", "high")
    ProductOrigin         origindetection.ProductOrigin // which pipeline sent the data (DogStatsD, APM, OTel, …)
}
```

### comp/core/tagger — the full implementation

The tagger itself is a component (`comp/core/tagger/def`). Its public interface is what callers use; `pkg/tagger` only supplies the input type. Key parts of the full implementation:

#### Component interface (`comp/core/tagger/def/component.go`)

```go
type Component interface {
    Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error)
    TagWithCompleteness(entityID types.EntityID, cardinality types.TagCardinality) ([]string, bool, error)
    Standard(entityID types.EntityID) ([]string, error)
    EnrichTags(tb tagset.TagsAccumulator, originInfo taggertypes.OriginInfo)
    GlobalTags(cardinality types.TagCardinality) ([]string, error)
    AgentTags(cardinality types.TagCardinality) ([]string, error)
    Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error)
    GetEntity(entityID types.EntityID) (*types.Entity, error)
    List() types.TaggerListResponse
    GetEntityHash(entityID types.EntityID, cardinality types.TagCardinality) string
    GenerateContainerIDFromOriginInfo(originInfo origindetection.OriginInfo) (string, error)
}
```

The most-used methods are:

| Method | Use-case |
|--------|----------|
| `Tag(entityID, cardinality)` | Get tags for a known entity ID (e.g. to tag a metric sample). |
| `EnrichTags(tb, originInfo)` | Resolve origin from `OriginInfo`, then append all tags into `tb`. Used by DogStatsD and APM on every inbound payload. |
| `GlobalTags(cardinality)` | Get cluster-level static tags to attach to all telemetry when host tags are absent. |
| `Subscribe(id, filter)` | Receive a stream of tag-change events (used by the log agent and other components that need to track entity tag updates). |

#### Entity IDs

Tagger entities are identified by URN-style string IDs:

| Entity kind | ID format |
|-------------|-----------|
| Container | `container_id://<sha256>` |
| Kubernetes pod | `kubernetes_pod_uid://<uid>` |
| Kubernetes deployment | `deployment://<namespace>/<name>` |
| Kubernetes metadata (generic) | `kubernetes_metadata://<group>/<resourceType>/<namespace>/<name>` |
| ECS task | `ecs_task://<task-id>` |
| Container image | `container_image_metadata://<sha256>` |
| Process | `process://<pid>` |
| GPU | `gpu://<uuid>` |

#### Tag cardinality (`comp/core/tagger/types`)

```go
const (
    LowCardinality          TagCardinality = iota  // host-count scale (kube_deployment, env, …)
    OrchestratorCardinality                        // pod/task-count scale (pod_name, task_arn, …)
    HighCardinality                                // container/request-count scale (container_id, …)
    NoneCardinality                                // no tags
    ChecksConfigCardinality                        // follows checks_tag_cardinality config setting
)
```

#### TagInfo and TagStore

Collectors (workloadmeta-based) produce `types.TagInfo` structs containing separate slices for each cardinality level. The `TagStore` accumulates these by entity ID and by source. When a source sends a new `TagInfo` for an entity, it replaces all previously stored tags from that source. When `DeleteEntity` is `true`, the entity is pruned from the store.

#### Origin detection (`comp/core/tagger/origindetection`)

Defines the wire-level protocol used by DogStatsD and APM to communicate their origin to the tagger:

| Constant | Purpose |
|----------|---------|
| `LocalDataContainerIDPrefix` (`ci-`) | Prefix for container ID in the local data list. |
| `LocalDataInodePrefix` (`in-`) | Prefix for cgroup inode in the local data list. |
| `ExternalDataPodUIDPrefix` (`pu-`) | Prefix for pod UID in the external data env var. |
| `ExternalDataContainerNamePrefix` (`cn-`) | Prefix for container name in the external data env var. |
| `ProductOriginDogStatsD` / `ProductOriginAPM` / `ProductOriginOTel` | Identifies which pipeline submitted the origin. |

---

## Usage

### How callers use the tagger

**DogStatsD** (`comp/dogstatsd/server/enrich.go`) calls `EnrichTags` on every metric sample to attach container tags:

```go
// enrich.go (simplified)
tagger.EnrichTags(tb, taggertypes.OriginInfo{
    ContainerIDFromSocket: containerIDFromUDS,
    LocalData:             localData,
    ExternalData:          externalData,
    Cardinality:           "low",
    ProductOrigin:         origindetection.ProductOriginDogStatsD,
})
```

**The aggregator** (`pkg/aggregator/statsd.go`) and metrics types (`pkg/metrics/metric_sample.go`, `pkg/metrics/event/event.go`) import `pkg/tagger/types` to carry `OriginInfo` alongside metric samples without importing the full tagger component.

**The log agent** subscribes to tag changes via `tagger.Subscribe` so it can re-tag log entries when a pod's labels change.

**The cluster check dispatcher** (`pkg/clusteragent/clusterchecks/dispatcher_main.go`) calls `tagger.GlobalTags(types.LowCardinality)` to attach cluster-level tags to every dispatched check configuration.

### Module structure

`pkg/tagger/types` is its own Go module (`pkg/tagger/types/go.mod`). It depends only on `comp/core/tagger/origindetection`, keeping the dependency footprint small for packages that only need `OriginInfo`. The full tagger implementation in `comp/core/tagger` is wired via the component framework (`fx`) and is not imported directly.

### IPC mode

When a non-agent process (e.g. the security agent) needs tags, it uses the remote tagger implementation (`comp/core/tagger/impl-remote`), which connects to the cluster agent or node agent's gRPC tagger server and receives tag updates over a stream. This avoids running a second local tagger that would independently query workloadmeta.

---

## Cross-references

### How the tagger fits into the wider pipeline

```
comp/core/workloadmeta  ──subscribes──►  comp/core/tagger (TagStore)
                                               │
                              ┌────────────────┼─────────────────┐
                              ▼                ▼                 ▼
                      DogStatsD           pkg/metrics        log agent
                   (EnrichTags)         (MetricSample        (Subscribe)
                                         .OriginInfo)
                              │
                              ▼
                         pkg/tagset
                    (TagsAccumulator)
                              │
                              ▼
                    pkg/aggregator context resolver
```

### Related documentation

| Document | Relationship |
|----------|--------------|
| [`comp/core/tagger`](../../comp/core/tagger.md) | Full tagger implementation: fx wiring, tag store, collectors, subscriptions, and IPC remote mode. Use this when wiring the tagger into a binary or writing tagger-aware components. |
| [`comp/core/workloadmeta`](../../comp/core/workloadmeta.md) | Primary data source for the tagger. The tagger subscribes to every workloadmeta entity kind; each `Kind` maps to a tagger `EntityID` prefix (e.g. `KindContainer` → `container_id://`). |
| [`pkg/tagset`](tagset.md) | Low-level tag-set data structures consumed by `EnrichTags`. `tagger.EnrichTags` writes resolved tags into a `tagset.TagsAccumulator`; the aggregator then uses `HashGenerator.Dedup2` to merge tagger-sourced tags with metric-level tags. |
| [`pkg/util/containers`](util/containers.md) | Provides `ContainerEntityPrefix` (`container_id://`) and `BuildEntityName` used to construct tagger entity IDs from runtime container IDs. Also provides `MetaCollector.GetContainerIDForPID` used during origin detection. |

### Tag flow for a DogStatsD metric

1. DogStatsD receives a UDP packet; `comp/dogstatsd/server/enrich.go` extracts origin info (UDS peer credentials, inline client-reported IDs, external data env var).
2. It calls `tagger.EnrichTags(tb, OriginInfo{...})` — `tb` is a `tagset.HashingTagsAccumulator`.
3. The tagger resolves the container ID from `OriginInfo` (using `pkg/util/containers/metrics.MetaCollector` as a fallback), looks it up in the tag store, and appends tags into `tb`.
4. The resulting `MetricSample` (with `OriginInfo` embedded) enters the aggregator's context resolver in `pkg/aggregator/context_resolver.go`.
5. The context resolver calls `sample.GetTags(taggerBuffer, metricBuffer, tagger)`, which internally calls `EnrichTags` again at aggregation time to merge tagger tags with check-level tags.
6. `tagset.HashGenerator.Dedup2` removes cross-source duplicates, and `tagset.NewCompositeTags` assembles the final tag set stored in the `Context`.
7. At flush time the `Serie` carries a `tagset.CompositeTags`; the serializer serializes it and the forwarder sends it to the intake.
