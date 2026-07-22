# Workloadmeta

-----

Workloadmeta ([`comp/core/workloadmeta`](<<<SRC>>>/comp/core/workloadmeta)) is the Agent's in-memory store of workload entities: containers, Kubernetes pods, ECS tasks, processes, container images, GPUs, and more. It exists so that every consumer that needs to know "what is running here" — the [tagger](tagger.md), [Autodiscovery](../checks/autodiscovery.md), the logs agent, checks, SBOM collection — reads from one consistent, merged view instead of each talking to Docker, containerd, the kubelet, or the ECS metadata endpoints on its own. Collectors feed the store; consumers query it or subscribe to a stream of set/unset events.

## Key packages

| Path | Purpose |
|---|---|
| [`comp/core/workloadmeta/def/types.go`](<<<SRC>>>/comp/core/workloadmeta/def/types.go) | `Kind`, `Source`, the `Entity` interface, all entity structs, `AgentType` |
| [`comp/core/workloadmeta/def/collectors.go`](<<<SRC>>>/comp/core/workloadmeta/def/collectors.go) | `Collector` interface and the `CollectorProvider` Fx value group |
| [`comp/core/workloadmeta/def/merge.go`](<<<SRC>>>/comp/core/workloadmeta/def/merge.go) | Generic entity merge used when several sources report the same entity |
| [`comp/core/workloadmeta/def/filter.go`](<<<SRC>>>/comp/core/workloadmeta/def/filter.go) | Subscription filters (kinds, source, event types) |
| [`comp/core/workloadmeta/impl/store.go`](<<<SRC>>>/comp/core/workloadmeta/impl/store.go) | The store: event loop, pull loop, collector startup/retry, Subscribe/Unsubscribe |
| [`comp/core/workloadmeta/impl/cached_entity.go`](<<<SRC>>>/comp/core/workloadmeta/impl/cached_entity.go) | Per-entity multi-source cache and merge |
| [`comp/core/workloadmeta/impl/completeness.go`](<<<SRC>>>/comp/core/workloadmeta/impl/completeness.go) | Tracks whether all expected sources have reported an entity |
| [`comp/core/workloadmeta/collectors/internal`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal) | All collector implementations |
| [`comp/core/workloadmeta/collectors/catalog-core`](<<<SRC>>>/comp/core/workloadmeta/collectors/catalog-core) | Collector set compiled into the core Agent (other catalogs sit alongside it) |
| [`comp/core/workloadmeta/server/server.go`](<<<SRC>>>/comp/core/workloadmeta/server/server.go) | Server side of the `WorkloadmetaStreamEntities` gRPC stream |
| [`comp/core/workloadmeta/telemetry/telemetry.go`](<<<SRC>>>/comp/core/workloadmeta/telemetry/telemetry.go) | Internal telemetry (stored entities, subscriber channel usage, pull durations) |

## Entity model

Every entity implements the `workloadmeta.Entity` interface: `GetID() EntityID`, `Merge(Entity)`, and `DeepCopy()`. An `EntityID` is a `{Kind, ID}` pair — inside workloadmeta, entities are always identified by kind plus ID, never by the URL-like string form the tagger uses. The kinds defined in [`def/types.go`](<<<SRC>>>/comp/core/workloadmeta/def/types.go):

| Kind | Reported by |
|---|---|
| `container` | Container runtimes, kubelet, ECS |
| `kubernetes_pod` | kubelet, kubemetadata |
| `kubernetes_metadata` | kubeapiserver (namespaces, nodes, and any resource enabled through `*_as_tags` configuration) |
| `kubernetes_deployment` | kubeapiserver (language detection) |
| `kubelet_metrics`, `kubernetes_capabilities`, `kubelet` | kubelet and cluster-level capability probing |
| `kubernetes_kueue_queue`, `kubernetes_kueue_resource_flavor`, `kubernetes_kueue_workload` | kubeapiserver (Kueue) |
| `ecs_task` | ECS collector |
| `container_image_metadata` | Runtimes and Trivy SBOM scanning |
| `process` | Process collector, service discovery, language detection |
| `gpu` | NVML collector |
| `crd` | kubeapiserver (arbitrary CRDs) |

## Sources and merging

Each event carries a `Source` (`runtime`, `node_orchestrator`, `cluster_orchestrator`, `trivy`, `nvml`, `remote_workloadmeta`, `process_collector`, `service_discovery`, and others). The same entity is frequently reported by several sources — on Kubernetes a container is reported both by the container runtime collector (`runtime`) and by the kubelet collector (`node_orchestrator`), each knowing different fields.

[`cached_entity.go`](<<<SRC>>>/comp/core/workloadmeta/impl/cached_entity.go) keeps a `map[Source]Entity` per entity plus a pre-computed merged view. The merge iterates sources in **alphabetical order of the source string** and fills in zero-valued fields via `Merge` (backed by `mergo` in [`def/merge.go`](<<<SRC>>>/comp/core/workloadmeta/def/merge.go)). The order is deterministic but not semantic: conflicting non-zero values between sources are not expected, and when they happen the winner is whichever source sorts first. Subscribers using `SourceAll` (the default) get the merged view; a subscription filter can pin a single source instead.

## Collectors

A collector implements the interface in [`def/collectors.go`](<<<SRC>>>/comp/core/workloadmeta/def/collectors.go):

1. `Start(ctx, store)` — connect to the external system; long-running collectors stream events from here (for example the Docker event stream).
1. `Pull(ctx)` — polled by the store every 5 seconds (`defaultPullCollectorInterval`); collectors can override the interval by implementing `PullCollectorWithCustomInterval` (the kubelet collector honors `kubelet_collector_pull_interval`).
1. `GetID()` and `GetTargetCatalog()` — identity and an `AgentType` bitmask (`NodeAgent | ClusterAgent | Remote`) checked against the store's `Params.AgentType` in [`impl/workloadmeta.go`](<<<SRC>>>/comp/core/workloadmeta/impl/workloadmeta.go), so a collector compiled into a binary can still opt out of running in a given flavor.

Collectors are Fx value-group members. Each binary imports a *catalog* package that decides which collectors are even compiled in:

| Catalog | Used by | Collectors |
|---|---|---|
| [`catalog-core`](<<<SRC>>>/comp/core/workloadmeta/collectors/catalog-core) | Core agent | cloudfoundry (container + vm), containerd, crio, docker, ecs, kubelet, kubemetadata, podman, nvml, process, remote-process-collector, remote-sbom-collector |
| [`catalog-clusteragent`](<<<SRC>>>/comp/core/workloadmeta/collectors/catalog-clusteragent) | Cluster Agent | kubeapiserver only |
| [`catalog-remote`](<<<SRC>>>/comp/core/workloadmeta/collectors/catalog-remote) | process-agent, security-agent, system-probe | remote-workloadmeta gRPC client only |
| [`catalog-dogstatsd`](<<<SRC>>>/comp/core/workloadmeta/collectors/catalog-dogstatsd) | Standalone dogstatsd | Local runtime collectors, no process/remote collectors |
| [`catalog-otel`](<<<SRC>>>/comp/core/workloadmeta/collectors/catalog-otel) | otel-agent | Runtime + kubelet + kubemetadata collectors for local tag enrichment |

At startup ([`impl/store.go`](<<<SRC>>>/comp/core/workloadmeta/impl/store.go)), candidate collectors are started on a dedicated goroutine with exponential-backoff retry (1s doubling up to 30s). A collector returning a retriable error stays a candidate forever; one returning `errors.NewDisabled` (for example the ECS collector when not on ECS) is removed silently. This is how one catalog serves every environment — on a plain Linux host all the container-runtime collectors disable themselves through environment-feature detection. The pull goroutine waits up to 30 seconds (`firstPullWaitTimeout`) for the first collector to start before its first pull; each `Pull` runs in its own goroutine with a 1-minute timeout, and overlapping pulls of the same collector are skipped.

### Collector reference

All collectors live under [`collectors/internal`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal). Highlights, with their gating:

| Collector | Gating | Behavior |
|---|---|---|
| [`docker`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/docker/docker.go) | build tag `docker`, Docker socket detected | Subscribes to the Docker event stream plus an initial listing; also emits `container_image_metadata` (including SBOM trigger events) |
| [`containerd`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/containerd/containerd.go) | build tag `containerd`, containerd socket detected | containerd event subscription; namespace selection via `containerd_namespace(s)` / `containerd_exclude_namespaces`; pause containers excluded via `exclude_pause_container` |
| [`crio`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/crio/crio.go) | build tag `crio`, CRI socket detected | Polls the CRI API (`cri_socket_path`) |
| [`podman`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/podman/podman.go) | build tag `podman`, podman state DB present | Reads podman's BoltDB/SQLite state file directly from disk (`podman_db_path`) — no daemon API, so the file must be mounted and match podman's storage configuration |
| [`kubelet`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/kubelet/kubelet.go) | build tag `kubelet`, on Kubernetes | Polls the kubelet `/pods` API via [`pkg/util/kubernetes/kubelet`](<<<SRC>>>/pkg/util/kubernetes/kubelet/kubelet.go) (auto-detects 10250 TLS / 10255 read-only); emits pods and their containers as `node_orchestrator`; entities expire after not being seen for 15s |
| [`kubeapiserver`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/kubeapiserver/kubeapiserver.go) | build tag `kubeapiserver`, Cluster Agent only | Reflector-based informer stores for pods, deployments, namespaces/nodes/arbitrary resources (`kubernetes_resources_labels_as_tags` and friends), Kueue objects, and CRDs |
| [`kubemetadata`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/kubemetadata/kubemetadata.go) | build tags `kubeapiserver && kubelet`, node agent | Asks the Cluster Agent (or the API server directly when `cluster_agent.enabled: false`) for service tags and namespace labels/annotations to decorate pod entities |
| [`ecs`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/ecs/ecs.go) | build tag `docker`, on ECS | Daemon mode (EC2 / managed instances): ECS agent introspection v1 plus optional v4 task detail (`ecs_task_collection_enabled`, rate-limited). Sidecar mode (Fargate): task metadata endpoint v2/v4 |
| [`cloudfoundry`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/cloudfoundry/container/cf_container.go) | on Cloud Foundry | Garden containers and BBS VMs |
| [`nvml`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/nvml/nvml.go) | NVIDIA NVML available, GPU monitoring enabled | GPU devices as `gpu` entities |
| [`process`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/process/process_collector.go) | any of `process_config.process_collection.enabled`, `language_detection.enabled`, system-probe `discovery.enabled`, `gpu.enabled` | Unified local process collection; also carries language-detection and service-discovery sources |
| [`remote/workloadmeta`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/remote/workloadmeta/workloadmeta.go) | catalog-remote binaries | gRPC client of the core agent's store (see below) |
| [`remote/processcollector`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/remote/processcollector/process_collector.go) | core agent, Windows only | Reverse direction: the core agent subscribes to process entities streamed by the process-agent (no-op on other platforms) |
| [`remote/sbomcollector`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/remote/sbomcollector/sbom_collector.go) | core agent, build tag `trivy` | Reverse direction: SBOM entities streamed from system-probe's runtime-security (CWS) module |

## Event flow and subscription semantics

```text
collector.Start/Pull
      |
      | store.Notify([]CollectorEvent)
      v
  eventCh (buffered, 50)
      |
      v                     (single goroutine)
 handleEvents ── update cached_entity map
      |
      | filtered EventBundle per subscriber
      v
 subscriber channels ──> tagger, autodiscovery listeners, logs, checks, ...
```

`handleEvents` is the only writer of the cache, which makes the merge and fan-out race-free. Its non-obvious behaviors:

1. Set events that change nothing (deep-equal to the cached source state) are dropped, so subscribers never see no-op churn.
1. Unset events for entities that were never Set are dropped (collectors emit unsets for pause containers they filtered out on the way in).
1. When one source unsets an entity but **other sources still report it**, subscribers receive a **Set** event carrying the pre-delete merged state, not an unset. This prevents tag flicker when, say, containerd reports a container deletion moments before the kubelet does. An unset only reaches subscribers when the last source is removed.
1. Subscribers have priorities (`SubscriberPriority` in [`def/types.go`](<<<SRC>>>/comp/core/workloadmeta/def/types.go)); the [tagger](tagger.md) subscribes at `TaggerPriority`, which sorts it before every other consumer so that tags are updated before anything that might query them reacts to the same event.
1. The first bundle a new subscriber receives replays the entire store, so consumers need no special bootstrap path.
1. Each `EventBundle` must be acknowledged (`bundle.Acknowledge()`); the store waits up to 10 seconds to hand the bundle to a full subscriber channel, then up to 1 second (`eventBundleChTimeout`) for the acknowledgment, before moving on and recording a telemetry miss. A slow subscriber degrades itself, not the store.

/// warning
Do not treat an unset event as "the container is gone" in the general case — with multiple sources it is delivered as a Set with the surviving merged state, and only the final source removal produces a real unset. Consumers that cache entity state themselves must handle both.
///

## Completeness

[`impl/completeness.go`](<<<SRC>>>/comp/core/workloadmeta/impl/completeness.go) tracks whether all *expected* sources have reported an entity, based on detected environment features: on Kubernetes a pod is complete once both `node_orchestrator` (kubelet) and `cluster_orchestrator` (kubemetadata) have reported it, and a container once the kubelet and (if a runtime socket is accessible) the runtime have; on ECS EC2 the equivalent applies to the ECS collector plus the runtime. `Event.IsComplete` propagates to the tagger, which uses it to tell Autodiscovery and the logs agent whether an entity's tags are final (`ad_tag_completeness_max_wait` gates check scheduling on it). The expected-sources map is static: if an expected collector permanently fails, its entities never become complete.

## Remote workloadmeta

Processes that must not (or cannot) watch runtimes themselves — the process-agent, security-agent, and system-probe — use `catalog-remote`, whose only collector is a gRPC client of the core agent's store. The server side ([`server/server.go`](<<<SRC>>>/comp/core/workloadmeta/server/server.go)) implements `WorkloadmetaStreamEntities` on the core agent's `AgentSecure` gRPC service ([`pkg/proto/datadog/api/v1/api.proto`](<<<SRC>>>/pkg/proto/datadog/api/v1/api.proto)), registered in [`comp/api/grpcserver/impl-agent/grpc.go`](<<<SRC>>>/comp/api/grpcserver/impl-agent/grpc.go) and served on `localhost:cmd_port` (default 5001) with IPC auth — see [Inter-process communication](../processes/ipc.md).

The client ([`remote/workloadmeta`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/remote/workloadmeta/workloadmeta.go)) subscribes with a kind filter (containers, pods, ECS tasks, processes, container images). On any stream error it reconnects with backoff; the server then replays a full snapshot, and the client diffs it against its local state and unsets entities that disappeared while it was disconnected. All entities arrive under the single source `remote_workloadmeta`, so no multi-source merging happens on the remote side.

## Configuration

| Setting | Effect |
|---|---|
| `containerd_namespace(s)`, `containerd_exclude_namespaces` | containerd namespace selection |
| `exclude_pause_container` | Drop Kubernetes pause containers (default true) |
| `cri_socket_path`, `podman_db_path` | Runtime endpoints for CRI-O and podman |
| `kubernetes_kubelet_host`, `kubelet_tls_verify`, `kubernetes_http(s)_kubelet_port` | Kubelet connectivity |
| `kubelet_collector_pull_interval` | Override the kubelet collector's 5s pull cadence |
| `ecs_deployment_mode` | `auto` / `daemon` / `sidecar`; auto-corrects to sidecar on Fargate |
| `ecs_task_collection_enabled`, `ecs_task_collection_rate/burst`, `ecs_task_cache_ttl` | v4 per-task detail collection in daemon mode |
| `ecs_collect_resource_tags_ec2` | Collect ECS resource tags on EC2 |
| `process_config.process_collection.enabled`, `language_detection.enabled`, `gpu.enabled` | Gates for the process collector |
| `cluster_agent.collect_kubernetes_tags` | Makes the Cluster Agent's kubeapiserver collector watch pods |

## Deployment modes

1. **Host install**: catalog-core is compiled in, but every container collector self-disables via environment-feature detection; typically only `process` (if enabled) and `nvml` (GPU hosts) run.
1. **Kubernetes DaemonSet**: kubelet plus one runtime collector both report each container (two sources per entity — this is what completeness tracking and merged unsets exist for); `kubemetadata` augments pods with Cluster-Agent-served namespace and service metadata.
1. **Cluster Agent**: `Params.AgentType` is `ClusterAgent`, so only the `kubeapiserver` collector runs, entirely informer-based.
1. **ECS EC2**: docker collector plus the ECS collector in daemon mode.
1. **ECS Fargate**: the ECS collector runs in sidecar mode against the task metadata endpoint; it is the only source, so entities are always complete.
1. **process-agent / security-agent / system-probe**: catalog-remote; everything is streamed from the core agent, nothing collected locally.

## Debugging

1. `agent workload-list` ([`cmd/agent/subcommands/workloadlist`](<<<SRC>>>/cmd/agent/subcommands/workloadlist/command.go)) prints the store of a running agent; `agent workload-list -v` includes full entity dumps. It queries `GET /agent/workload-list` on the agent command API.
1. A [flare](../operations/flare.md) contains `workload-list.log` with the same output.
1. Internal telemetry counters (entity counts by kind and source, subscriber channel saturation, pull latencies) are defined in [`telemetry/telemetry.go`](<<<SRC>>>/comp/core/workloadmeta/telemetry/telemetry.go).
