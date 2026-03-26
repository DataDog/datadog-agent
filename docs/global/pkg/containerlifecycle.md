> **TL;DR:** `pkg/containerlifecycle` tracks container, pod, and ECS task deletion events from workloadmeta and streams them as protobuf payloads to the Datadog backend, enabling reliable close-out of metrics and traces for short-lived workloads.

# pkg/containerlifecycle

## Purpose

The container lifecycle feature tracks when containers, Kubernetes pods, and ECS tasks are deleted, and streams those deletion events to the Datadog backend. This enables Datadog to reliably close out metrics and traces for short-lived workloads, even when the agent's normal collection cycle misses the final data points.

The feature is split across two locations:

| Path | Role |
|---|---|
| `pkg/containerlifecycle/` | Shared constants (event names, object kinds, payload version) |
| `pkg/collector/corechecks/containerlifecycle/` | The actual `container_lifecycle` check implementation |

The check is a **long-running check** (interval 0); it subscribes to workloadmeta event streams for its entire lifetime.

---

## Key elements

### Key types

#### `pkg/containerlifecycle/types.go` — shared constants

```go
const (
    PayloadV1           = "v1"
    EventNameDelete     = "delete"
    ObjectKindContainer = "container"
    ObjectKindPod       = "pod"
    ObjectKindTask      = "task"
)
```

These are the only stable public symbols in the `pkg/containerlifecycle` package. They are referenced by both the check internals and any code that parses container lifecycle payloads.

---

### Key interfaces

#### `pkg/collector/corechecks/containerlifecycle/` — check implementation

#### `Check` and `Config` (`check.go`)

`Check` is the core check type. It is wrapped in `core.NewLongRunningCheckWrapper` so it runs as a persistent goroutine rather than on a periodic ticker.

```go
type Config struct {
    ChunkSize    int `yaml:"chunk_size"`            // default 100, max 100
    PollInterval int `yaml:"poll_interval_seconds"` // default 10
}
```

`Factory(store workloadmeta.Component)` returns a `check.Check` factory suitable for registration with the collector. The check is enabled only when `container_lifecycle.enabled: true` is set in `datadog.yaml`.

The `Run()` loop subscribes to three workloadmeta streams:
- `KindContainer` (source: `SourceRuntime`) — container start/stop events from the container runtime.
- `KindKubernetesPod` (source: `SourceNodeOrchestrator`) — pod deletion events.
- `KindECSTask` (source: `SourceNodeOrchestrator`) — ECS task deletion events (only when `ecs_task_collection_enabled: true`).

On Fargate sidecar shutdown, `sendFargateTaskEvent()` manually synthesizes a delete event for the current task before the check exits.

### Key functions

#### `processor` (`processor.go`)

Converts raw workloadmeta events into protobuf `contlcycle.EventsPayload` messages and flushes them via `sender.EventPlatformEvent` with type `eventplatform.EventTypeContainerLifecycle`.

Three independent `queue` instances are maintained — one each for containers, pods, and tasks. The processor goroutine polls them on the configured `PollInterval`.

Processing per entity kind:
- **Container:** sets exit code, exit timestamp, and owner reference (pod UID or task ARN) by querying workloadmeta.
- **Pod:** sets pod UID and exit timestamp from `KubernetesPod.FinishedAt`.
- **Task:** sets task ARN and uses `time.Now()` as the exit timestamp (ECS metadata v1 API does not provide it).

#### `event` interface and `eventTransformer` (`event.go`)


`event` is a builder interface. `newEvent()` returns a `*eventTransformer`. Callers chain `with*` methods then call `toPayloadModel()` (creates a new `contlcycle.EventsPayload` with host and cluster ID) or `toEventModel()` (adds to an existing payload).

The `toPayloadModel()` call resolves the hostname and cluster ID at event creation time. On Kubernetes, cluster ID comes from `clustername.GetClusterID()`; on ECS, from `ecsutil.GetClusterMeta()`.

#### `queue` (`queue.go`)


Thread-safe, chunked queue backed by `[]*contlcycle.EventsPayload`. Events are appended to the last payload until it reaches `chunkSize`, then a new payload entry is started. `flush()` atomically swaps the accumulated payloads out and returns them.

---

### Configuration and build flags

No build tags. The check is disabled by default and requires `container_lifecycle.enabled: true` in `datadog.yaml`. ECS task events additionally require `ecs_task_collection_enabled: true`.

## Usage

### Registration

The check is registered via `Factory` in `pkg/commonchecks/corechecks.go`. It requires a `workloadmeta.Component` to be injected at construction time.

### Configuration

```yaml
# datadog.yaml
container_lifecycle:
  enabled: true           # required; check is disabled by default
  chunk_size: 100         # max events per network message
  poll_interval_seconds: 10
```

ECS task events additionally require:
```yaml
ecs_task_collection_enabled: true
```

### Data flow

```
workloadmeta event stream
        |
        v
Check.Run() (long-running goroutine)
        |
        v
processor.processEvents(eventBundle)
   - processContainer / processPod / processTask
   - builds eventTransformer
   - queue.add(event)
        |
        v (every poll_interval)
processor.flush()
   - queue.flush() → []*contlcycle.EventsPayload
   - proto.Marshal each payload
   - sender.EventPlatformEvent(bytes, EventTypeContainerLifecycle)
        |
        v
Event Platform forwarder → Datadog backend
```

### Testing

`queue_test.go` and `processor_test.go` contain unit tests. The check can be integration-tested via the E2E framework using fakeintake's container lifecycle event endpoint.

---

## Cross-references

| Topic | See also |
|-------|----------|
| Event Platform forwarder — routes `EventTypeContainerLifecycle` payloads to `contlcycle-intake.` | [comp/forwarder/eventplatform](../comp/forwarder/eventplatform.md) |
| workloadmeta — provides the `KindContainer`, `KindKubernetesPod`, and `KindECSTask` event streams the check subscribes to | [comp/core/workloadmeta](../comp/core/workloadmeta.md) |
| containerd client — the workloadmeta containerd collector that feeds container start/stop events into the store | [pkg/util/containerd](util/containerd.md) |

### Relationship to the Event Platform forwarder

The processor calls `sender.EventPlatformEvent(rawBytes, EventTypeContainerLifecycle)`.
This routes through `pkg/aggregator.BufferedAggregator` to
`comp/forwarder/eventplatform.Forwarder.SendEventPlatformEvent`, which dispatches the
payload over the `contlcycle-intake.` pipeline. The pipeline uses a `StreamStrategy`
(one protobuf message per HTTP request) rather than `BatchStrategy`. See
[comp/forwarder/eventplatform](../comp/forwarder/eventplatform.md) for the full
pipeline internals, endpoint resolution, and the `EventTypeContainerLifecycle` constant.

### Relationship to workloadmeta

The check subscribes to three workloadmeta streams. Understanding the source semantics
matters for correctness:

| Kind | Source | Notes |
|------|--------|-------|
| `KindContainer` | `SourceRuntime` | Events come from the container runtime (containerd, Docker, CRI-O). A container is deleted when all sources unset it. |
| `KindKubernetesPod` | `SourceNodeOrchestrator` | Events come from the kubelet collector. `KubernetesPod.FinishedAt` provides the exit timestamp. |
| `KindECSTask` | `SourceNodeOrchestrator` | Events come from the ECS metadata v1 collector. No exit timestamp is available from ECS; the processor uses `time.Now()`. |

The processor also queries workloadmeta point-in-time via `GetContainer` to resolve the
owner reference (pod UID or task ARN) and exit code for container events. If the container
entity has already been evicted from the store by the time the processor runs, the owner
reference is left empty. See [comp/core/workloadmeta](../comp/core/workloadmeta.md) for
the full `Subscribe`/`GetContainer` API.

### Relationship to the containerd collector

Container delete events ultimately originate from the containerd event stream, which is
polled by `comp/core/workloadmeta/collectors/internal/containerd` via
`pkg/util/containerd.ContainerdItf.GetEvents()`. The collector translates `task/exit`
and `container/delete` containerd events into workloadmeta `EventTypeUnset` events, which
are then received by this check's subscription. See [pkg/util/containerd](util/containerd.md)
for the `GetEvents()` API and namespace-scoped event filtering.
