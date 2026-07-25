# Orchestrator explorer

-----

The orchestrator explorer powers the Kubernetes Explorer in the Datadog app: a near-real-time inventory of every Kubernetes resource in a cluster — pods, deployments, nodes, CRDs — including full scrubbed YAML manifests. It is implemented as the `orchestrator` core check running in the [Cluster Agent](cluster-agent.md) (DCA) or a cluster-check runner, which watches resources through dedicated informers, converts them into protobuf payloads, and ships them through a **dedicated forwarder** to the process intake at `orchestrator.datadoghq.com` — a separate path from the metrics [forwarder](../pipelines/forwarder.md).

The check lives in [`pkg/collector/corechecks/cluster/orchestrator`](<<<SRC>>>/pkg/collector/corechecks/cluster/orchestrator); shared configuration and scrubbing live in [`pkg/orchestrator`](<<<SRC>>>/pkg/orchestrator) and [`pkg/redact`](<<<SRC>>>/pkg/redact).

## Key packages

| Path | Purpose |
|---|---|
| [`orchestrator.go`](<<<SRC>>>/pkg/collector/corechecks/cluster/orchestrator/orchestrator.go) | `OrchestratorCheck`: 10 s interval, leader gating, informer factories |
| [`collector_bundle.go`](<<<SRC>>>/pkg/collector/corechecks/cluster/orchestrator/collector_bundle.go) | `CollectorBundle`: assembles the collector list from config, discovery, and inventory |
| [`collectors/k8s`](<<<SRC>>>/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s) | One collector per resource type (deployments, nodes, unassigned pods, PV/PVC, RBAC, CRDs, ...) |
| [`collectors/inventory`](<<<SRC>>>/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory) | The catalog of built-in collectors and their stability markers |
| [`discovery/collector_discovery.go`](<<<SRC>>>/pkg/collector/corechecks/cluster/orchestrator/discovery/collector_discovery.go) | API-discovery-based selection of collectors supported by the cluster |
| [`processors`](<<<SRC>>>/pkg/collector/corechecks/cluster/orchestrator/processors) | Per-resource processing: extraction, scrubbing, tagging, chunking into payloads |
| [`manifest_buffer.go`](<<<SRC>>>/pkg/collector/corechecks/cluster/orchestrator/manifest_buffer.go) | Cross-collector buffering of YAML manifests before flush |
| [`terminated_resource_bundle.go`](<<<SRC>>>/pkg/collector/corechecks/cluster/orchestrator/terminated_resource_bundle.go) | Captures deleted resources from informer delete events (leader only) |
| [`pkg/orchestrator/config/config.go`](<<<SRC>>>/pkg/orchestrator/config/config.go) | `OrchestratorConfig`: endpoints, scrubber, manifest settings, message limits |
| [`pkg/redact`](<<<SRC>>>/pkg/redact) | `DataScrubber` for container commands/env and annotation/label redaction |
| [`comp/forwarder/orchestrator/impl/forwarder_orchestrator.go`](<<<SRC>>>/comp/forwarder/orchestrator/impl/forwarder_orchestrator.go) | The dedicated orchestrator forwarder component (build tag `orchestrator`) |
| [`pkg/collector/corechecks/orchestrator/pod`](<<<SRC>>>/pkg/collector/corechecks/orchestrator/pod) | Node-agent `orchestrator_pod` check: per-node pod collection from the kubelet |
| [`pkg/collector/corechecks/orchestrator/ecs`](<<<SRC>>>/pkg/collector/corechecks/orchestrator/ecs) | ECS variant: task collection for the ECS explorer |

## Who collects what

Orchestrator data comes from two cooperating sources, joined by the **cluster ID** (the UUID the DCA persists in the `datadog-cluster-id` ConfigMap and serves at `/api/v1/cluster/id`):

| Runner | Check | Scope |
|---|---|---|
| DCA leader, or a CLC runner via [cluster checks](cluster-checks.md) | `orchestrator` | All cluster-scoped and namespaced resources *except* assigned pods: deployments, replicasets, services, nodes, jobs, cronjobs, statefulsets, daemonsets, PV/PVC, RBAC, ingresses, network policies, HPA/VPA, namespaces, CRDs, custom resources — plus *unassigned* pods (no `spec.nodeName` yet) |
| Node agent (every node) | `orchestrator_pod` | Pods assigned to that node, sourced from the kubelet through [workloadmeta](workloadmeta.md); the node agent fetches the cluster ID from the DCA first |
| Node agent on ECS | `orchestrator_ecs` | ECS tasks |

Splitting pods out to node agents keeps the DCA's memory bounded: a pod informer over a large cluster is by far the most expensive watch. The DCA only maintains the cheap unassigned-pods informer (field selector `spec.nodeName=""`) and a terminated-pods informer.

When the `orchestrator` check runs on the DCA itself it is leader-gated through `IsLeader()` (only the leader collects, followers skip silently); when dispatched to a CLC runner, the check instance must carry `skip_leader_election: true` because the [cluster-check dispatcher](cluster-checks.md) already guarantees single placement.

`kubernetes_state_core` ([`pkg/collector/corechecks/cluster/ksm`](<<<SRC>>>/pkg/collector/corechecks/cluster/ksm)) is a *different* check with a similar deployment story: it produces `kubernetes_state.*` metrics rather than explorer payloads, and its collector list is deliberately kept name-compatible with the orchestrator check's (a comment in `NewCollectorBundle` points at the upstream kube-state-metrics builder) so users can share collector configuration between the two.

## The collector bundle

`Configure` on the check builds an `OrchestratorInformerFactory` — six informer factories with a 300 s resync: general, CRD, dynamic (custom resources), VPA, unassigned-pods, and terminated-pods — and wraps everything in a `CollectorBundle`. `prepareCollectors` assembles the collector list with this precedence:

1. **CRD collectors from the check config** (`crd_collectors`, format `<group>/<version>/<resource>`), capped at 100 (`defaultMaximumCRDs`); entries that duplicate a built-in collector are ignored with a warning. If `collectors: []` is explicitly set, default collection is skipped entirely (CRD-only mode).
1. **Collectors from the check config** (`collectors`), verified against API discovery.
1. **Discovery** ([`discovery/`](<<<SRC>>>/pkg/collector/corechecks/cluster/orchestrator/discovery), when `orchestrator_explorer.collector_discovery.enabled`): query the apiserver's group/version/resource catalog and activate every supported stable collector — this is how the check adapts to the cluster's Kubernetes version (for example `cronjobs` v1 versus v1beta1).
1. **Inventory fallback**: all stable collectors at their default versions.
1. **Builtin custom-resource collectors** are always appended last: the Datadog, Argo, Flux, Karpenter, and EKS API groups declared at the top of [`collector_bundle.go`](<<<SRC>>>/pkg/collector/corechecks/cluster/orchestrator/collector_bundle.go) are collected automatically when present (`orchestrator_explorer.custom_resources.ootb.enabled`, default true), while the Gateway API, service-mesh (Istio and other vendors), and ingress-controller groups additionally require the `ootb.gateway_api` / `ootb.service_mesh` / `ootb.ingress_controllers` opt-ins (default false). The terminated-pod collector is appended in the same step.

Each collector owns one informer and, per run, lists from the informer cache and hands the resources to its processor.

## Processing and payloads

Processors ([`processors/`](<<<SRC>>>/pkg/collector/corechecks/cluster/orchestrator/processors)) turn Kubernetes objects into `CollectorK8s*` protobuf messages from [`agent-payload`](https://github.com/DataDog/agent-payload):

- **Scrubbing**: container commands, args, and env values are scrubbed by the `DataScrubber` in [`pkg/redact`](<<<SRC>>>/pkg/redact) using default sensitive words plus `orchestrator_explorer.custom_sensitive_words`; annotations and labels matching `custom_sensitive_annotations_labels` are redacted too.
- **Tagging**: cluster-level tags (`kube_cluster_name`, check tags, DCA tagger global tags) are attached; on a CLC runner the extra tags arrive via the dispatched check config instead of the local tagger.
- **Chunking**: messages are split by count and byte weight (`orchestrator_explorer.max_per_message`, `max_message_bytes`) with a shared group ID per run so the backend can reassemble a consistent snapshot.
- **Manifests**: when `orchestrator_explorer.manifest_collection.enabled` (default true) collectors also emit the full YAML of each resource. Manifests from all collectors funnel into the shared `ManifestBuffer` ([`manifest_buffer.go`](<<<SRC>>>/pkg/collector/corechecks/cluster/orchestrator/manifest_buffer.go)), which flushes on a ticker (`manifest_collection.buffer_flush_interval`) or when full — batching small manifests across resource types into fewer payloads.
- **Terminated resources**: the `TerminatedResourceBundle` ([`terminated_resource_bundle.go`](<<<SRC>>>/pkg/collector/corechecks/cluster/orchestrator/terminated_resource_bundle.go)) hooks informer delete events so the explorer can show *why* a resource disappeared; it is enabled only while the check holds leadership (followers explicitly disable it to avoid buffering deletes they will never flush).

## The dedicated forwarder

Orchestrator payloads bypass the metrics serializer entirely. The `orchestrator` forwarder component ([`forwarder_orchestrator.go`](<<<SRC>>>/comp/forwarder/orchestrator/impl/forwarder_orchestrator.go), build tag `orchestrator`) instantiates a second `DefaultForwarder` with its own domain resolvers pointed at the orchestrator endpoints — `orchestrator_explorer.orchestrator_dd_url` (falling back to `process_config.orchestrator_dd_url`, default `https://orchestrator.datadoghq.com`) plus `orchestrator_additional_endpoints` — with API-key checking disabled. The component resolves to a real forwarder only when `orchestrator_explorer.enabled` is true **and** the environment is Kubernetes or ECS; otherwise it is a no-op option and the aggregator drops orchestrator payloads. Payloads route to `/api/v2/orch` (legacy `/api/v1/orchestrator`, see [`endpoints.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/endpoints/endpoints.go)) and reuse the process-intake message framing, sharing infrastructure with the [process pipeline](../pipelines/processes.md).

## Configuration

| Key | Default | Meaning |
|---|---|---|
| `orchestrator_explorer.enabled` | true | Master switch (still requires a K8s/ECS environment and, for collection, the check being scheduled) |
| `orchestrator_explorer.orchestrator_dd_url` | `https://orchestrator.datadoghq.com` | Intake endpoint; `process_config.orchestrator_dd_url` is the legacy fallback |
| `orchestrator_explorer.orchestrator_additional_endpoints` | — | Dual-shipping |
| `orchestrator_explorer.custom_sensitive_words` | — | Extra scrubber words for commands/env |
| `orchestrator_explorer.custom_sensitive_annotations_labels` | — | Annotations/labels to redact |
| `orchestrator_explorer.collector_discovery.enabled` | true | API-discovery-based collector selection |
| `orchestrator_explorer.manifest_collection.enabled` | true | Collect YAML manifests |
| `orchestrator_explorer.manifest_collection.buffer_flush_interval` | 20 s | Manifest buffer flush cadence |
| `orchestrator_explorer.max_per_message` / `max_message_bytes` | 100 / bounded | Payload chunking limits |
| `orchestrator_explorer.custom_resources.max_count` | 5000 (hard cap 10000) | Per-CR-collector item quota |
| Check instance `collectors` / `crd_collectors` / `skip_leader_election` | — | Collector overrides and CLC-runner gating |

## Gotchas

- **Two checks, one product.** Forgetting the node agents' `orchestrator_pod` check (or breaking their DCA connectivity for the cluster ID) yields an explorer with every resource *except* running pods; the DCA-side check alone only covers unassigned pods.
- **The check errors out without a cluster name** (`orchestrator check is configured but the cluster name is empty` in `Configure`) — cluster-name detection is a hard prerequisite, same as for [workload autoscaling](autoscaling.md).
- **Leader gating differs by placement.** On the DCA, `leader_election: true` is required or the check refuses to run; on CLC runners, `skip_leader_election: true` must be set in the check instance (the DCA dispatcher injects it automatically only for KSM shards) or the check refuses to run for the same reason.
- **Running the check both on the DCA and as a cluster check double-collects** the whole cluster inventory; pick one placement.
- **CRD collectors are quota-bound**: a custom-resource collector that lists more than `orchestrator_explorer.custom_resources.max_count` items (hard cap 10000) is skipped with a listing error rather than truncated, and at most 100 `crd_collectors` entries are honored.
- **Subresources cannot be collected** — discovery filters out names containing `/` (for example `nodepools/scale`) because informers on subresources never sync.
- **Scrubbing is CPU-visible.** Manifest collection plus custom sensitive words run regex-style scrubbing over every object on every resync; large clusters tune `extra_sync_timeout_seconds` and collector lists rather than disabling scrubbing.
