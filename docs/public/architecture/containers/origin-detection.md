# Origin detection

-----

Origin detection answers one question: *which container sent this payload?* DogStatsD metrics arrive over a socket, traces arrive over HTTP, and neither carries container tags by itself. Origin detection identifies the emitting container (or pod), and the [tagger](tagger.md) then enriches the payload with that entity's tags at the configured cardinality. It spans several subsystems — the DogStatsD listeners, the trace-agent receiver, the Cluster Agent's admission controller, and the tagger's `EnrichTags` resolution ladder — all sharing one vocabulary defined in [`comp/core/tagger/origindetection`](<<<SRC>>>/comp/core/tagger/origindetection/origindetection.go).

## Shared vocabulary

`OriginInfo` is the envelope every intake builds and hands to the tagger:

| Type | Produced by | Contents |
|---|---|---|
| `LocalData` | The client library (or the transport) | Container ID (`ci-<id>`; legacy APM `cid-<id>`; legacy bare ID), cgroup v2 inode (`in-<inode>`), pod UID (from `DD_ENTITY_ID`), and the process ID from socket credentials |
| `ExternalData` | The Cluster Agent's admission controller, injected as the `DD_EXTERNAL_ENV` env var | `it-<bool>` (init container), `cn-<container-name>`, `pu-<pod-uid>` |
| `Cardinality` | Client override (`card:` field or `dd.internal.card:` tag) | `low` / `orchestrator` / `high` / `none` |
| `ProductOrigin` | The intake | `DogStatsDLegacy`, `DogStatsD`, `APM`, or `OTel` — selects the resolution ladder |

`LocalData` requires the client to *see* its own container identity (readable `/proc/self/cgroup`). `ExternalData` exists precisely for environments where it cannot — gVisor sandboxes, restricted pod security policies, sidecar-less setups — because the admission controller records the container name and pod UID from outside at pod-creation time. Parsers for both formats (`ParseLocalData`, `ParseExternalData`) live in [`origindetection.go`](<<<SRC>>>/comp/core/tagger/origindetection/origindetection.go).

## The three origin channels

```text
                         +--------------------------------------+
  1. transport           | UDS ancillary data: SO_PASSCRED pid  |──> pid -> container ID (cgroups)
                         +--------------------------------------+
                         +--------------------------------------+
  2. protocol            | DogStatsD fields |c: |e: |card:      |──> LocalData / ExternalData
                         | APM headers Datadog-Entity-ID, ...   |
                         +--------------------------------------+
                         +--------------------------------------+
  3. environment         | Admission-injected DD_ENTITY_ID and  |──> pod UID / ExternalData
     (Kubernetes)        | DD_EXTERNAL_ENV in the app container |    via channel 2
                         +--------------------------------------+
```

1. **Transport**: on Linux, Unix domain sockets can carry the sender's credentials. The kernel appends a `struct ucred` (PID, UID, GID) to each datagram, and the Agent maps the PID to a container ID by parsing cgroups. This requires no client cooperation but only works over UDS on Linux.
1. **Protocol**: clients embed identity in the payload itself — DogStatsD optional fields or APM HTTP headers. Works over any transport, but requires a recent client library and, for the container ID, a client that can read its own cgroup.
1. **Environment**: on Kubernetes, the [admission controller](admission-controller.md) injects `DD_ENTITY_ID` (the pod UID via the downward API) and `DD_EXTERNAL_ENV` into application containers; client libraries forward these through channel 2.

## DogStatsD path

The wire-level details of the DogStatsD server are covered in [DogStatsD internals](../dogstatsd/internals.md); this section covers only origin handling.

### UDS socket credentials

When `dogstatsd_origin_detection: true`, the UDS listeners enable `SO_PASSCRED` ([`comp/dogstatsd/listeners/uds_linux.go`](<<<SRC>>>/comp/dogstatsd/listeners/uds_linux.go)). Each `ReadMsgUnix` receives the sender's `Ucred`; `processUDSOrigin` resolves PID → container ID through the container-metrics meta-collector ([`pkg/util/containers/metrics/provider/metacollector.go`](<<<SRC>>>/pkg/util/containers/metrics/provider/metacollector.go), backed by workloadmeta and cgroup parsing), caching results for 1 minute. The result is stored on the packet as `Origin = container_id://<id>` and travels with every metric parsed from it.

Two special cases: a PID of 0 means the client is in a different PID namespace the Agent cannot see (the error suggests host PID mode), and a GID equal to the replay marker (`replay.GUID`) means the traffic is replayed capture data, in which case the original PID rides in the UID field and resolves through the [`pidmap`](<<<SRC>>>/comp/dogstatsd/pidmap/impl/state.go) component instead of procfs.

UDP carries no transport identity, and Windows named pipes have no credential passing — on those transports only channels 2 and 3 apply.

### Protocol fields and tags

When `dogstatsd_origin_detection_client: true`, the parser ([`comp/dogstatsd/server/impl/parse.go`](<<<SRC>>>/comp/dogstatsd/server/impl/parse.go)) honors three optional datagram fields: `|c:<localdata>` (container ID `ci-` and/or inode `in-`), `|e:<externaldata>`, and `|card:<cardinality>`. Independently of that gate, enrichment ([`enrich.go`](<<<SRC>>>/comp/dogstatsd/server/impl/enrich.go)) extracts two special tags: `dd.internal.entity_id:<pod-uid>` (set by client libraries from `DD_ENTITY_ID`) becomes `LocalData.PodUID`, and `dd.internal.card:` sets the cardinality if the `card:` field didn't. The assembled `OriginInfo` (socket origin + LocalData + ExternalData + cardinality, `ProductOrigin: DogStatsD`) rides on each `MetricSample`; actual tag resolution is deferred to the aggregator's context resolver, which calls `tagger.EnrichTags`.

## Resolution in the tagger: EnrichTags

`EnrichTags` ([`comp/core/tagger/impl/tagger.go`](<<<SRC>>>/comp/core/tagger/impl/tagger.go)) is the single sink where origins become tags. Before any ladder runs, an inode-only `LocalData` is resolved to a container ID via the meta-collector. Then the ladder depends on the product origin.

**Legacy DogStatsD ladder** — used when `origin_detection_unified: false` (the default) and the sample came from DogStatsD:

1. If `dogstatsd_origin_optout_enabled: true` (default) and the client sent `card:none`, return with no origin tags at all.
1. Accumulate tags for the **UDS socket origin**, unless a `dd.internal.entity_id` pod UID is present *and* `dogstatsd_entity_id_precedence: true`.
1. Accumulate tags for the **client-provided origin**: the pod UID from `dd.internal.entity_id` if set (and not `none`), otherwise the container ID from the `c:` field. Note this is additive to step 2, not a replacement.
1. Accumulate the global entity tags (`internal://global-entity-id`).

**Unified ladder** — used for APM and OTel always, and for DogStatsD when `origin_detection_unified: true`. A cardinality of `none` disables enrichment unconditionally; otherwise the first rung that finds tags wins and resolution stops:

```text
1. container ID from UDS socket credentials
2. container ID from LocalData (ci- field, or resolved in- inode)
3. container ID generated from ExternalData (pod UID + container name -> workloadmeta)
4. pod UID from LocalData (dd.internal.entity_id)
5. pod UID from ExternalData
   (fallthrough: global entity tags)
```

Container-level rungs outrank pod-level ones deliberately: container tags include the pod's tags (the tagger propagates pod tags to containers during [extraction](tagger.md)), plus container-specific ones. In the unified ladder a successful rung returns immediately, so the trailing global-entity accumulation only runs when nothing matched; the legacy ladder always appends global tags. On Fargate this makes no practical difference because static tags are attached to the container entities themselves, not only to the global entity.

## APM path

The trace-agent resolves a container for incoming trace payloads in [`pkg/trace/api/container_linux.go`](<<<SRC>>>/pkg/trace/api/container_linux.go), using headers defined in [`headers.go`](<<<SRC>>>/pkg/trace/api/internal/header/headers.go):

1. `Datadog-Entity-ID` header — LocalData (`ci-<id>` returns immediately; `in-<inode>` needs resolution).
1. `Datadog-Container-ID` header — deprecated, kept for older libraries.
1. The peer PID: when the client used the receiver UDS socket (`apm_config.receiver_socket`), `connContext` captures `SO_PEERCRED` credentials into the request context.
1. `Datadog-External-Env` header — ExternalData.

Anything short of a literal container ID is resolved through `tagger.GenerateContainerIDFromOriginInfo`, which tries in order: known container ID, PID lookup, cgroup inode lookup, ExternalData lookup. On the trace-agent the tagger is the [remote tagger](tagger.md), so this call becomes the `TaggerGenerateContainerIDFromOriginInfo` RPC served by the core agent — the trace-agent itself never parses cgroups for other processes. The resulting container ID keys `container_id://<id>` tag lookups that decorate trace stats and spans. On Windows and macOS ([`container.go`](<<<SRC>>>/pkg/trace/api/container.go)) only the headers work; there are no socket credentials or cgroups. See the [trace pipeline](../pipelines/traces.md) for what happens downstream.

## Logs path

Logs need no per-message origin detection: the origin is intrinsic, because each tailer knows which container it tails. The logs agent attaches tags at send time via a per-source provider ([`pkg/logs/internal/tag/provider.go`](<<<SRC>>>/pkg/logs/internal/tag/provider.go)) that calls `tagger.Tag(container_id://<id>, HighCardinality)` on every batch, so tag updates during a container's life are picked up. `logs_config.tagger_warmup_duration` (default 0) is a legacy startup delay; the modern mechanism for "don't emit before tags are complete" is Autodiscovery's tag-completeness gating (`ad_tag_completeness_max_wait`), built on workloadmeta's [completeness tracking](workloadmeta.md). See the [logs pipeline](../pipelines/logs.md).

## OTLP path

For OTLP telemetry, origin comes from resource attributes rather than transport: the `infraattributesprocessor` ([`comp/otelcol/otlp/components/processor/infraattributesprocessor`](<<<SRC>>>/comp/otelcol/otlp/components/processor/infraattributesprocessor)) maps `container.id` and `k8s.pod.uid` resource attributes to tagger entity IDs and appends infrastructure tags (`ProductOriginOTel`, unified ladder). This runs in both [OTLP ingest](../otel/otlp-ingest.md) and the [DDOT collector](../otel/ddot.md).

## Admission controller injection

On Kubernetes, the Cluster Agent's mutating webhook ([`pkg/clusteragent/admission/mutate/config/mutator.go`](<<<SRC>>>/pkg/clusteragent/admission/mutate/config/mutator.go)) makes channels 2 and 3 work without any user configuration in the application pod:

1. `DD_ENTITY_ID` — the pod UID via the downward API (`metadata.uid`); client libraries turn it into the `dd.internal.entity_id` tag.
1. `DD_EXTERNAL_ENV` — built per container as `it-<init>,cn-<container-name>,pu-$(DD_INTERNAL_POD_UID)`; client libraries forward it as the `e:` field / `Datadog-External-Env` header.
1. Agent connectivity itself — `DD_AGENT_HOST` or a mounted DogStatsD/APM socket, controlled by `admission_controller.inject_config.mode` (`hostip` / `service` / `socket` / `csi`).

See [Admission controller](admission-controller.md) for the webhook machinery itself.

## Configuration matrix

| Setting | Default | Effect |
|---|---|---|
| `dogstatsd_origin_detection` | `false` | Enable UDS `SO_PASSCRED` credentials (Linux, socket traffic only) |
| `dogstatsd_origin_detection_client` | `false` | Parse client-sent `c:` / `e:` / `card:` fields |
| `origin_detection_unified` | `false` | Use the unified ladder for DogStatsD (APM/OTel always use it) |
| `dogstatsd_entity_id_precedence` | `false` | Legacy ladder: presence of `dd.internal.entity_id` suppresses the UDS origin |
| `dogstatsd_origin_optout_enabled` | `true` | Legacy ladder: `card:none` opts the client out of all origin enrichment |
| `dogstatsd_tag_cardinality` | `low` | Default cardinality when the client specifies none |
| `dogstatsd_socket` | `/var/run/datadog/dsd.socket` (Linux) | UDS datagram socket path |
| `apm_config.receiver_socket` | platform-dependent | Trace receiver UDS, enables peer-credential capture |
| `admission_controller.inject_config.enabled` | `true` (on the Cluster Agent) | Inject `DD_ENTITY_ID`, `DD_EXTERNAL_ENV`, and connectivity config |

Client-side, the relevant env vars are `DD_ENTITY_ID` and `DD_EXTERNAL_ENV` (both admission-injected) — client libraries read them automatically.

## Per-deployment guidance

1. **Linux host**: UDS + `dogstatsd_origin_detection` gives transport-level attribution with zero client requirements; add `dogstatsd_origin_detection_client` for containerized clients hitting UDP.
1. **Kubernetes DaemonSet**: the standard setup is the host-path DogStatsD socket plus admission-controller injection; UDS credentials cover most cases, `DD_ENTITY_ID` covers UDP senders, and `DD_EXTERNAL_ENV` covers workloads that cannot read their own cgroup.
1. **Sandboxed / restricted workloads** (gVisor, locked-down `/proc`): only ExternalData works — the client cannot produce `ci-`/`in-` values, and there is no usable PID mapping.
1. **ECS Fargate**: the Agent is a sidecar inside the task; there is no host UDS socket, so clients use UDP over localhost. Task-level tags are applied to all data as static tags anyway; container-level attribution requires client-provided LocalData (`c:` field).
1. **Windows**: named pipes and UDP carry no credentials; rely on protocol fields entirely.

## Gotchas

1. **`origin_detection_unified` defaults to `false`**, so DogStatsD still runs the legacy ladder where the `dd.internal.entity_id` pod UID is *additive* to the UDS container origin, and where `dogstatsd_entity_id_precedence` can suppress the UDS origin entirely. APM and OTel are unaffected by all three legacy knobs.
1. **`card:none` semantics differ by ladder**: legacy honors it only when `dogstatsd_origin_optout_enabled: true`; unified honors it unconditionally.
1. **Origin detection is not free**: PID and inode resolutions hit the container-metrics meta-collector with short caches; per-origin telemetry (`telemetry.dogstatsd_origin`) is deliberately capped to avoid cardinality explosions.
1. **The UDS origin cache is 1 minute per PID** — a PID recycled across containers within that window (rare but possible under churn) briefly resolves to the old container.
1. **Cardinality is resolved per sample, not per connection**: the `card:` field overrides `dd.internal.card:`, which overrides `dogstatsd_tag_cardinality`. A single misbehaving client sending `card:high` can multiply context cardinality in the [aggregator](../pipelines/metrics/aggregation.md).
1. **PID 0 in UDS credentials** means the Agent cannot see the sender's PID namespace — on Kubernetes this usually means the Agent pod lacks `hostPID: true` and per-PID resolution silently degrades to the other channels.
