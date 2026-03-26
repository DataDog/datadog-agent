# pkg/orchestrator

## Purpose

`pkg/orchestrator` provides the shared types, cache, and configuration for the Orchestrator Explorer feature. The Orchestrator Explorer collects Kubernetes and ECS resource manifests and streams them to the Datadog backend, enabling a live topology view of an entire cluster.

This package is a shared foundation. The actual collection logic lives in `pkg/collector/corechecks/cluster/orchestrator/` (cluster-agent side) and `pkg/collector/corechecks/orchestrator/` (node-agent side). This package exposes the constants, resource-type taxonomy, in-memory deduplication cache, and configuration that those consumers share.

**Build flag:** most files in `pkg/orchestrator/config/` are guarded by the `orchestrator` build tag. The `model/` and root `cache.go` files are unconditional.

---

## Key elements

### `model/` — resource type taxonomy and stats

**`NodeType`** (`model/types.go`)

An `int` enum that identifies every supported resource kind. Values are fixed integers (not `iota`) because they are used in the agent-payload protobuf schema (`OrchestratorResource` enum). Current types include all standard Kubernetes objects (Pod, Deployment, DaemonSet, StatefulSet, ReplicaSet, Service, Node, Namespace, Ingress, PersistentVolume, PersistentVolumeClaim, ConfigMap, Role, RoleBinding, ClusterRole, ClusterRoleBinding, ServiceAccount, CRD, CR, VPA, HPA, NetworkPolicy, LimitRange, StorageClass, PodDisruptionBudget, EndpointSlice, KubeletConfig) and `ECSTask` (value 150).

Important methods on `NodeType`:
- `String() string` — human-readable resource name.
- `Orchestrator() string` — returns `"k8s"` or `"ecs"`, used for telemetry tags.
- `TelemetryTags() []string` — returns `[orchestrator, resource]` tag pair for Prometheus/DSD metrics.

`NodeTypes() []NodeType` returns all registered types as a slice for iteration.

**`CheckStats`** (`model/stats.go`)

Holds per-run cache hit / miss counts for a `NodeType`. Stored in `KubernetesResourceCache` under the key built by `BuildStatsKey(nodeType)`. The Cluster Agent status command reads these entries to report collection efficiency.

```go
type CheckStats struct {
    CacheHits int
    CacheMiss int
    NodeType
}
```

`SetCacheStats(resourceListLen, resourceMsgLen int, nodeType NodeType, ca *cache.Cache)` computes `CacheHits = listLen - msgLen` (items skipped due to cache hit) and stores the result.

---

### Root package — deduplication cache

**`KubernetesResourceCache`** (`cache.go`)

A global `*go-cache.Cache` (TTL 3 min, purge 30 s). Keys are the Kubernetes resource UID; values are the `resourceVersion` string last seen.

**`SkipKubernetesResource(uid, resourceVersion string, nodeType NodeType) bool`**

The primary deduplication function. Returns `true` (skip) when the UID is already in cache with the same `resourceVersion`. On a cache miss or version change it stores the new version and returns `false` (send). Hits and misses are tracked via `expvar` counters and telemetry (`orchestrator/cache_hits`, `orchestrator/cache_misses`).

`SetCacheStats` (alias in root, delegates to `model.SetCacheStats`) stores a `CheckStats` snapshot in `KubernetesResourceCache` after each check run.

`ClusterAgeCacheKey` (`"orchestratorClusterAge"`) — a secondary entry in the same cache that the orchestrator check uses to record when the cluster was first seen.

---

### `config/` — runtime configuration (`orchestrator` build tag)

**`OrchestratorConfig`** (`config/config.go`)

The top-level configuration struct. Populated from `datadog.yaml` under the `orchestrator_explorer` namespace (backward-compatible with the old `process_config` namespace).

Key fields:

| Field | yaml key | Description |
|---|---|---|
| `OrchestrationCollectionEnabled` | `orchestrator_explorer.enabled` | Master switch |
| `KubeClusterName` | (derived) | RFC-1123-compliant cluster name |
| `IsScrubbingEnabled` | `container_scrubbing.enabled` | Scrub sensitive env vars from manifests |
| `Scrubber` | `custom_sensitive_words` | `*redact.DataScrubber` instance |
| `OrchestratorEndpoints` | `orchestrator_dd_url` / `orchestrator_additional_endpoints` | Backend endpoints with API keys |
| `MaxPerMessage` | `max_per_message` | Max resources per network message (default 100) |
| `MaxWeightPerMessageBytes` | `max_message_bytes` | Max message size (default 10 MB) |
| `IsManifestCollectionEnabled` | `manifest_collection.enabled` | Collect raw YAML manifests |
| `BufferedManifestEnabled` | `manifest_collection.buffer_manifest` | Buffer manifests before sending |
| `ManifestBufferFlushInterval` | `manifest_collection.buffer_flush_interval` | Flush interval for buffered manifests |
| `KubeletConfigCheckEnabled` | `kubelet_config_check.enabled` | Enable kubelet config collection |

Constructor: `NewDefaultOrchestratorConfig(extraTags []string)` builds a struct with sane defaults. Call `(*OrchestratorConfig).Load()` to hydrate it from the agent configuration.

Helper functions:
- `IsOrchestratorEnabled() (bool, string)` — returns enabled flag and cluster name.
- `IsOrchestratorECSExplorerEnabled() bool` — returns true when running on ECS with task collection on.
- `OrchestratorNSKey(pieces ...string) string` — builds a dotted config key under `orchestrator_explorer`.

---

### `util/` — payload chunking

**`ChunkPayloadsBySizeAndWeight`** (`util/chunking.go`)

A generic function that splits a flat list of protobuf messages into chunks that respect both a maximum item count (`maxChunkSize`) and a maximum total byte weight (`maxChunkWeight`). Used by orchestrator processors before sending batches to the backend.

Key types:
- `PayloadList[T]` — wraps `Items []T` and a `WeightAt func(int) int` to compute per-item byte size.
- `ChunkAllocator[T, P]` — manages the growing slice of chunks; callers supply `AppendToChunk` and optional `OnAccept` callbacks.

Items that individually exceed `maxChunkWeight` are placed in a chunk of their own rather than being dropped.

---

## Usage

### Where the packages are used

| Consumer | Package | Description |
|---|---|---|
| Cluster-agent orchestrator check | `pkg/collector/corechecks/cluster/orchestrator/` | Runs per-resource collectors; calls `SkipKubernetesResource` to deduplicate; uses `OrchestratorConfig`; calls `ChunkPayloadsBySizeAndWeight` before forwarding |
| Node-agent pod/kubelet checks | `pkg/collector/corechecks/orchestrator/` | Collects pod-level and kubelet config data |
| ECS orchestrator check | `pkg/collector/corechecks/orchestrator/ecs/` | ECS task manifest collection |
| Processor context | `pkg/collector/corechecks/cluster/orchestrator/processors/` | `ProcessorContext.GetOrchestratorConfig()` passes config into per-resource-type processors. `BaseProcessorContext` calls `SkipKubernetesResource` directly, and uses `MaxPerMessage` / `MaxWeightPerMessageBytes` from config as arguments to `chunkOrchestratorPayloadsBySizeAndWeight`. |
| Collector helpers | `pkg/collector/corechecks/cluster/orchestrator/collectors/k8s/helpers.go` | Uses `NewDefaultOrchestratorConfig(nil)` to construct a baseline config before loading from the agent configuration. |

### Typical call flow

1. At startup, `OrchestratorConfig.Load()` reads the agent configuration. `IsOrchestratorEnabled()` is called first; if disabled, collection is skipped entirely.
2. Each check run: the collector lists resources from the Kubernetes informer cache (backed by `APIClient.InformerFactory` from `pkg/util/kubernetes/apiserver`).
3. For each resource, `SkipKubernetesResource(uid, resourceVersion, nodeType)` decides whether to include it in the next payload. Cache keys are the Kubernetes resource UID; values are the last-seen `resourceVersion` string (TTL 3 min).
4. The remaining resources are split into network messages via `ChunkPayloadsBySizeAndWeight`. Items that individually exceed `MaxWeightPerMessageBytes` are placed in a chunk of their own rather than being dropped.
5. Chunked protobuf payloads are passed to `pkg/serializer.SendOrchestratorMetadata` (resource objects) or `SendOrchestratorManifests` (raw YAML).
6. The serializer calls `comp/forwarder/orchestrator.Get()` and dispatches via `SubmitOrchestratorChecks` / `SubmitOrchestratorManifests`.
7. After the run, `SetCacheStats` records hit/miss totals for the status page (visible via `agent status`).

**Cache expiry note:** The global `KubernetesResourceCache` uses `go-cache` with a 3-minute TTL and 30-second purge interval. Resources that stop being reported by Kubernetes informers will be evicted naturally and re-sent on the next collection cycle.

### Adding a new resource type

1. Add a new constant in `pkg/orchestrator/model/types.go` with the value matching the `agent-payload` proto enum.
2. Add the new type to the `NodeTypes()` slice and the `String()` / `Orchestrator()` switches.
3. Implement a collector and processor under `pkg/collector/corechecks/cluster/orchestrator/collectors/` and `processors/`.

---

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/util/kubernetes/apiserver` | [`util/kubernetes-apiserver.md`](util/kubernetes-apiserver.md) | Provides the `APIClient` (typed + dynamic Kubernetes clients, informer factories) used by orchestrator collectors to list and watch every resource type. `SkipKubernetesResource` consumes the UID and `resourceVersion` values delivered by these informers. The `InformerFactory` and `DynamicInformerFactory` fields in `APIClient` cannot be stopped after `Start()` — orchestrator collectors use them for the process lifetime. Leader-scoped informers (e.g. manifest collection) should create a separate factory from `APIClient.InformerCl`. |
| `comp/forwarder/orchestrator` | [`comp/forwarder/orchestrator.md`](../comp/forwarder/orchestrator.md) | The dedicated forwarder component that routes orchestrator payloads to the Datadog Orchestrator Explorer intake. `pkg/serializer.Serializer.SendOrchestratorMetadata` and `SendOrchestratorManifests` call `Get()` on this component before forwarding. Enabled only when `orchestrator_explorer.enabled: true` and the `orchestrator` build tag is set. In binaries without the `orchestrator` tag, `Get()` always returns `(nil, false)`. The `AgentDemultiplexer` receives this component at construction time and passes it to the serializer via fx. |
| `pkg/serializer` | [`serializer.md`](serializer.md) | `MetricSerializer.SendOrchestratorMetadata` and `SendOrchestratorManifests` are the methods that accept the chunked protobuf payloads produced by `ChunkPayloadsBySizeAndWeight` and dispatch them via the orchestrator forwarder. The `types.ProcessMessageBody` interface and `ProcessPayloadEncoder` function in `pkg/serializer/types` are the protobuf wrappers used when encoding orchestrator messages. |

### Payload flow

```
pkg/util/kubernetes/apiserver.APIClient.InformerFactory
    |  Watch/list via typed or dynamic informer client
    v
Kubernetes informer cache
    |
    v
pkg/collector/corechecks/cluster/orchestrator/collectors/<resource>
    |
    +--> SkipKubernetesResource(uid, resourceVersion, nodeType)  [pkg/orchestrator cache]
    |         (TTL 3min; expvar counters: orchestrator/cache_hits, orchestrator/cache_misses)
    |
    +--> processors/BaseProcessorContext.ProcessManifestList
    |         └─> scrub sensitive env vars (OrchestratorConfig.Scrubber)
    |
    +--> ChunkPayloadsBySizeAndWeight(payloads, MaxPerMessage, MaxWeightPerMessageBytes)
    |                                                        [pkg/orchestrator/util]
    |
    v
pkg/serializer.SendOrchestratorMetadata / SendOrchestratorManifests
    |  (types.ProcessMessageBody encoded by types.ProcessPayloadEncoder)
    v
comp/forwarder/orchestrator.Get() → DefaultForwarder
    |  SubmitOrchestratorChecks / SubmitOrchestratorManifests
    v
Datadog Orchestrator Explorer intake
```
