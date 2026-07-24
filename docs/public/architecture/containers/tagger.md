# Tagger

-----

The tagger ([`comp/core/tagger`](<<<SRC>>>/comp/core/tagger)) is the client-side source of truth for entity tags. It subscribes to [workloadmeta](workloadmeta.md), extracts tags from every entity according to built-in rules and `*_as_tags` configuration, stores them in an in-memory TagStore, and answers `Tag(entityID, cardinality)` queries from every data path — metric aggregation, DogStatsD [origin detection](origin-detection.md), logs, APM, and checks. Processes that don't collect workload metadata themselves (process-agent, security-agent, system-probe, trace-agent) query a *remote tagger* that streams tags from the core Agent over gRPC instead of running their own extraction.

## Key packages

| Path | Purpose |
|---|---|
| [`comp/core/tagger/def/component.go`](<<<SRC>>>/comp/core/tagger/def/component.go) | Component interface: `Tag`, `TagWithCompleteness`, `Standard`, `GlobalTags`, `AgentTags`, `EnrichTags`, `GenerateContainerIDFromOriginInfo`, `List`, `Subscribe` |
| [`comp/core/tagger/impl/tagger.go`](<<<SRC>>>/comp/core/tagger/impl/tagger.go) | `localTagger`: the full implementation backed by a TagStore |
| [`comp/core/tagger/collectors/workloadmeta_main.go`](<<<SRC>>>/comp/core/tagger/collectors/workloadmeta_main.go) | `WorkloadMetaCollector`: subscription, `*_as_tags` config loading, static/global tags, source names and priorities |
| [`comp/core/tagger/collectors/workloadmeta_extract.go`](<<<SRC>>>/comp/core/tagger/collectors/workloadmeta_extract.go) | Tag extraction rules per entity kind |
| [`comp/core/tagger/tagstore/tagstore.go`](<<<SRC>>>/comp/core/tagger/tagstore/tagstore.go) | TagStore: per-entity per-source tag sets, prune loop, subscriptions |
| [`comp/core/tagger/tagstore/entity_tags.go`](<<<SRC>>>/comp/core/tagger/tagstore/entity_tags.go) | Merging tags across sources with collector priorities |
| [`comp/core/tagger/types/types.go`](<<<SRC>>>/comp/core/tagger/types/types.go) | `TagCardinality`, `TagInfo`, entity events |
| [`comp/core/tagger/types/entity_id.go`](<<<SRC>>>/comp/core/tagger/types/entity_id.go) | Entity ID prefixes and the global entity |
| [`comp/core/tagger/k8s_metadata/k8s_metadata.go`](<<<SRC>>>/comp/core/tagger/k8s_metadata/k8s_metadata.go) | `InitMetadataAsTags`: glob and multi-value handling for `*_as_tags` maps |
| [`comp/core/tagger/impl-remote/remote.go`](<<<SRC>>>/comp/core/tagger/impl-remote/remote.go) | Remote tagger gRPC client (stream, reconnect, resync) |
| [`comp/core/tagger/impl-dual/dual.go`](<<<SRC>>>/comp/core/tagger/impl-dual/dual.go) | Dual tagger: picks local or remote at startup |
| [`comp/core/tagger/server/server.go`](<<<SRC>>>/comp/core/tagger/server/server.go) | gRPC server streaming tag events to remote taggers |
| [`pkg/util/tags/static_tags.go`](<<<SRC>>>/pkg/util/tags/static_tags.go) | Static tags for Fargate, EKS Fargate, and the Cluster Agent |

## How tags get into the store

The local tagger owns a `WorkloadMetaCollector` that subscribes to workloadmeta with a nil filter (everything) at `TaggerPriority` — the highest subscriber priority, guaranteeing the tagger processes an entity event before any other component that might immediately query tags for that entity. For each event, the extraction code in [`workloadmeta_extract.go`](<<<SRC>>>/comp/core/tagger/collectors/workloadmeta_extract.go) produces a `types.TagInfo{Source, EntityID, LowCardTags, OrchestratorCardTags, HighCardTags, StandardTags, IsComplete}` and feeds it to `TagStore.ProcessTagInfo`.

```text
workloadmeta ──EventBundle──> WorkloadMetaCollector ──TagInfo──> TagStore
                                                                   |
        Tag() / TagWithCompleteness() / EnrichTags() <─────────────┤
        tagger gRPC server (remote taggers)        <───────────────┘
```

Tagger sources are named `workloadmeta-<kind>` (for example `workloadmeta-kubernetes_pod`), plus `workloadmeta-static` for static tags. When two sources report tags for the same entity — a container seen by both the runtime and the kubelet — `CollectorPriorities` ranks `NodeOrchestrator` (pod/task sources) above `NodeRuntime` (container/image sources) so higher-confidence values win during the merge in [`entity_tags.go`](<<<SRC>>>/comp/core/tagger/tagstore/entity_tags.go).

## Entity IDs

Tagger entities are identified as `<prefix>://<id>` ([`types/entity_id.go`](<<<SRC>>>/comp/core/tagger/types/entity_id.go)):

| Workloadmeta kind | Tagger entity ID |
|---|---|
| Container | `container_id://<sha>` |
| Kubernetes pod | `kubernetes_pod_uid://<uid>` |
| ECS task | `ecs_task://<task-id>` |
| Kubernetes deployment | `deployment://<namespace>/<name>` |
| Kubernetes metadata | `kubernetes_metadata://<group>/<resourceType>/<namespace>/<name>` (empty namespace for cluster-scoped objects) |
| Container image | `container_image_metadata://<sha>` |
| Process | `process://<pid>` |
| GPU | `gpu://<gpu-uuid>` |
| — | `internal://global-entity-id` |

The `internal://global-entity-id` pseudo-entity holds **global tags** that apply to everything the Agent emits — Fargate static tags, or the Cluster Agent's `kube_cluster_name`. `GlobalTags()` reads it directly and `EnrichTags` appends it during origin resolution.

## Cardinality

Every query specifies a `TagCardinality` ([`types/types.go`](<<<SRC>>>/comp/core/tagger/types/types.go)):

| Level | String | Contents | Example tags |
|---|---|---|---|
| Low | `low` | Stable, host-order cardinality | `image_name`, `kube_deployment`, `env` |
| Orchestrator | `orchestrator` (or `orch`) | Changes per pod/task | `pod_name`, `task_arn` |
| High | `high` | Changes per container or more | `container_name`, `container_id` |
| None | `none` | No tags at all | — |

Higher levels are supersets: a high-cardinality query returns low + orchestrator + high tags. Defaults are `checks_tag_cardinality: low` and `dogstatsd_tag_cardinality: low`; checks can override per instance, and the internal `ChecksConfigCardinality` sentinel resolves to the configured checks default at query time. Cardinality here controls *tag-set size per entity*, which directly drives timeseries cardinality (and cost) on the backend — that is why `high` is opt-in everywhere.

## Extraction rules

Per entity kind, the main rules in [`workloadmeta_extract.go`](<<<SRC>>>/comp/core/tagger/collectors/workloadmeta_extract.go):

1. **Containers**: image tags (`image_name`, `short_image`, `image_tag`, `image_id`), `container_name` and `container_id` (high cardinality), runtime, standard tags from `DD_ENV`/`DD_SERVICE`/`DD_VERSION` container env vars, OpenTelemetry resource-attribute env vars, plus the `container_labels_as_tags` / `container_env_as_tags` mappings (Docker-specific aliases `docker_labels_as_tags` / `docker_env_as_tags` are merged in). Orchestrator-specific labels (Docker Swarm, Rancher, ECS) get dedicated tags.
1. **Kubernetes pods**: `kube_namespace`, `pod_name`, `pod_phase`, owner tags (`kube_deployment`, `kube_replica_set`, `kube_job`, `kube_stateful_set`, ...), `kubernetes_pod_labels_as_tags` / `kubernetes_pod_annotations_as_tags`, the `ad.datadoghq.com/tags` annotation (JSON), per-container tags (`kube_container_name`), Kueue queue tags, and PVC tags when `kubernetes_persistent_volume_claims_as_tags` is set. Pod tags are propagated to the pod's containers.
1. **ECS tasks**: `task_name`, `task_family`, `task_version`, `task_arn`, `ecs_cluster_name`, launch type, availability zone, and EC2 resource tags when `ecs_collect_resource_tags_ec2` is enabled.
1. **Kubernetes metadata** (from the Cluster Agent's kubeapiserver collector): generic labels/annotations-as-tags for any resource via `kubernetes_resources_labels_as_tags` / `kubernetes_resources_annotations_as_tags` (keyed by `<resourceType>.<group>`, for example `deployments.apps`).
1. **Processes and GPUs**: service-discovery tags; `gpu_vendor`, `gpu_device`, `gpu_uuid`.

The `*_as_tags` maps support glob patterns on the metadata key, comma-separated lists to map one key to several tag names, and a `+` prefix on a tag name to make that tag high-cardinality — this handling lives in [`k8s_metadata.InitMetadataAsTags`](<<<SRC>>>/comp/core/tagger/k8s_metadata/k8s_metadata.go) and its `AddMetadataAsTags` companion. Map keys are lowercased, so YAML and environment-variable sources behave identically.

### Static and global tags

[`pkg/util/tags/static_tags.go`](<<<SRC>>>/pkg/util/tags/static_tags.go) computes *static tags* on environments without a host identity: on ECS Fargate and EKS Fargate, `DD_TAGS`/`DD_EXTRA_TAGS` (which normally become host tags) are instead attached to every container and task entity, plus `eks_fargate_node` and `kube_cluster_name` on EKS Fargate. The `WorkloadMetaCollector` also publishes these under the `workloadmeta-static` source on the global entity, and the Cluster Agent adds `kube_cluster_name` there so cluster-level data is tagged even without a hostname.

## TagStore and pruning

The [`TagStore`](<<<SRC>>>/comp/core/tagger/tagstore/tagstore.go) maps entity ID → `EntityTags` with per-source tag sets. Deletion is deliberately lazy: a `TagInfo` with `DeleteEntity` set marks the source with an expiry of now + 5 minutes (`deletedTTL`), and a prune ticker (1 minute) removes expired sources and empty entities, emitting `EntityEvent`s to tagger subscribers (chiefly the tagger gRPC server). The 5-minute grace period exists so that late data — logs or metrics flushed after a container exits — still resolves to the right tags. `LookupHashed` returns pre-hashed tag sets (`tagset.HashedTags`) so the aggregator can build context keys without re-hashing on every sample.

## Query surface

1. `Tag(entityID, cardinality)` / `TagWithCompleteness` — the hot path used by check senders, DogStatsD enrichment, and the logs tag provider.
1. `Standard(entityID)` — just the standard `env`/`service`/`version` tags.
1. `GlobalTags(cardinality)` / `AgentTags(cardinality)` — the global entity, and the Agent's own container tags (resolved via its own container ID).
1. `EnrichTags(accumulator, originInfo)` — the [origin detection](origin-detection.md) resolution ladder.
1. `Subscribe(id, filter)` — typed event stream, used by the tagger gRPC server and a few internal consumers.
1. `List` — powers `agent tagger-list` ([`cmd/agent/subcommands/taggerlist`](<<<SRC>>>/cmd/agent/subcommands/taggerlist/command.go)) and `GET /agent/tagger-list` on the command API; the output shows tags per entity *per source*, which is the fastest way to see which collector contributed a bad tag.
1. Python checks get the same data through the `tagger.tag()` rtloader module (`tagger.get_tags()` is its deprecated predecessor; see [Python checks](../checks/python.md)).

## Tagger flavors per process

All flavors provide the same `tagger.Component` interface; binaries choose via their Fx wiring:

| Fx module | Implementation | Used by |
|---|---|---|
| [`fx`](<<<SRC>>>/comp/core/tagger/fx/fx.go) | Local tagger | Cluster Agent, standalone dogstatsd, otel-agent, cluster-agent-cloudfoundry |
| [`fx-dual`](<<<SRC>>>/comp/core/tagger/fx-dual/fx.go) | Local *or* remote, decided at startup | Core agent (`agent run`, `agent jmx`, `agent diagnose`, ...) |
| [`fx-remote`](<<<SRC>>>/comp/core/tagger/fx-remote/fx.go) | Remote client only | process-agent, security-agent, system-probe, host-profiler |
| [`fx-optional-remote`](<<<SRC>>>/comp/core/tagger/fx-optional-remote/fx.go) | Remote unless disabled (noop fallback) | trace-agent (noop on Azure App Services), otel-agent (alongside its local tagger) |
| [`fx-noop`](<<<SRC>>>/comp/core/tagger/fx-noop/fx.go) | No-op | Metadata-free CLI subcommands (`agent snmp`, systray, `trace-agent info`) |

The dual tagger's decision function lives in [`cmd/agent/common/tagger_params.go`](<<<SRC>>>/cmd/agent/common/tagger_params.go): the core agent goes remote only when it runs as a **cluster-check runner** (`IsCLCRunner` plus `clc_runner_remote_tagger_enabled`), in which case the target is the *Cluster Agent's* gRPC endpoint and authentication uses the Cluster Agent auth token — not the local IPC token. The CLC runner's remote filter also excludes `kubernetes_pod_uid` entities, since cluster and endpoint checks only need cluster-scoped tags.

## Remote tagger protocol

The server ([`comp/core/tagger/server`](<<<SRC>>>/comp/core/tagger/server/server.go)) runs inside the core agent's `AgentSecure` gRPC service on `cmd_port` (5001) and inside the Cluster Agent's API server on port 5005 for CLC runners. On connect, it subscribes to the local TagStore with the stream's prefix filter and cardinality, sends an initial snapshot, then live events. Two protections matter at scale:

1. Messages are cut into chunks to stay under gRPC's 4 MB default limit ([`server/util.go`](<<<SRC>>>/comp/core/tagger/server/util.go)) — initial snapshots on large clusters are far bigger than one message.
1. A sync throttler ([`server/syncthrottler.go`](<<<SRC>>>/comp/core/tagger/server/syncthrottler.go)) caps concurrent initial syncs at `remote_tagger.max_concurrent_sync` (default 3), so a fleet of runners reconnecting at once cannot stampede the Cluster Agent.

The client ([`impl-remote/remote.go`](<<<SRC>>>/comp/core/tagger/impl-remote/remote.go)) authenticates with the IPC session token, maintains its own minimal [tagstore](<<<SRC>>>/comp/core/tagger/impl-remote/tagstore.go) holding only the streamed cardinality, and reconnects with exponential backoff; each (re)connection triggers a full server-side resync. Because filtering happens server-side, a remote tagger only ever sees the entity prefixes and cardinality it asked for.

## Configuration

| Setting | Effect |
|---|---|
| `checks_tag_cardinality`, `dogstatsd_tag_cardinality` | Default cardinality per data path |
| `container_labels_as_tags`, `container_env_as_tags` | Container metadata mappings (runtime-agnostic) |
| `docker_labels_as_tags`, `docker_env_as_tags` | Legacy Docker-specific equivalents, merged in |
| `kubernetes_pod_labels_as_tags`, `kubernetes_pod_annotations_as_tags` | Pod metadata mappings (glob, multi-tag, and `+` high-cardinality support) |
| `kubernetes_namespace_labels_as_tags`, `kubernetes_node_labels_as_tags` (and `_annotations_`) | Namespace/node metadata mappings |
| `kubernetes_resources_labels_as_tags`, `kubernetes_resources_annotations_as_tags` | Arbitrary resources, `{"deployments.apps": {"team": "team"}}` style; requires the Cluster Agent's kubeapiserver collector |
| `kubernetes_persistent_volume_claims_as_tags` | PVC tags on pods |
| `ecs_collect_resource_tags_ec2` | EC2 resource tags on ECS tasks |
| `tags` (`DD_TAGS`), `extra_tags` | Host tags normally; static container/global tags on Fargate and the Cluster Agent |
| `clc_runner_remote_tagger_enabled` | CLC runner uses the Cluster Agent's remote tagger |
| `remote_tagger.max_concurrent_sync` | Server-side concurrent initial-sync cap (default 3) |

## Gotchas

1. **Tags survive death by 5 minutes.** `deletedTTL` keeps a deleted entity queryable so late-flushed data is still tagged; anything asserting "container gone ⇒ tags gone" observes a 5-minute lag, and that is by design.
1. **Source priority beats arrival order.** A tag reported by the kubelet (`NodeOrchestrator`) overrides the same key from the runtime (`NodeRuntime`) regardless of which event arrived last.
1. **Remote tagger auth differs for CLC runners.** They present the *Cluster Agent* token over the cross-node TLS config (`OverrideAuthTokenGetter` / `OverrideTLSConfigGetter` in `RemoteParams`), not the local `auth_token` — the usual suspect when debugging 401s from a runner.
1. **Static tags can duplicate label tags on Fargate.** `DD_TAGS` values are applied to container entities *and* the global entity; setting the same key with a different value in pod labels produces two tags after dedup, not one winner.
1. **`agent tagger-list` and `agent workload-list` answer different questions.** The former shows extracted tags per source; the latter shows raw entities. A missing tag with a present entity means an extraction/config problem, not a collection problem.
