# Autoscaling

-----

The [Cluster Agent](cluster-agent.md) (DCA) hosts two distinct autoscaling products. The **external metrics provider** makes Datadog metrics available to standard Kubernetes `HorizontalPodAutoscaler` objects by serving the `external.metrics.k8s.io` APIService — Kubernetes asks Datadog "what is the value of this metric" and scales on the answer. **Workload autoscaling** goes further: the `DatadogPodAutoscaler` CRD lets the Datadog backend compute both horizontal (replicas) and vertical (resources) recommendations, delivered through [remote config](../configuration/remote-config.md), which DCA controllers apply to workloads directly. Both are leader-only control loops; both live under [`pkg/clusteragent/autoscaling`](<<<SRC>>>/pkg/clusteragent/autoscaling).

## Key packages

| Path | Purpose |
|---|---|
| [`cmd/cluster-agent/custommetrics/server.go`](<<<SRC>>>/cmd/cluster-agent/custommetrics/server.go) | Runs the Kubernetes external-metrics APIService inside the DCA (port 8443) |
| [`pkg/clusteragent/autoscaling/custommetrics`](<<<SRC>>>/pkg/clusteragent/autoscaling/custommetrics) | Legacy ConfigMap-backed external-metrics store and provider |
| [`pkg/clusteragent/autoscaling/externalmetrics`](<<<SRC>>>/pkg/clusteragent/autoscaling/externalmetrics) | DatadogMetric-CRD-based implementation: provider, retriever, watcher, controller |
| [`pkg/util/kubernetes/apiserver/controllers/hpa_controller.go`](<<<SRC>>>/pkg/util/kubernetes/apiserver/controllers/hpa_controller.go), [`wpa_controller.go`](<<<SRC>>>/pkg/util/kubernetes/apiserver/controllers/wpa_controller.go) | Legacy-mode HPA/WPA watchers feeding the ConfigMap store |
| [`pkg/util/kubernetes/autoscalers/processor.go`](<<<SRC>>>/pkg/util/kubernetes/autoscalers/processor.go) | Batched Datadog metric queries shared by both backends |
| [`comp/autoscaling/datadogclient`](<<<SRC>>>/comp/autoscaling/datadogclient) | Component wrapping the Datadog query API (keys, endpoints, redundancy) |
| [`pkg/clusteragent/autoscaling/workload/provider/provider.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/provider/provider.go) | `StartWorkloadAutoscaling`: wires the whole workload-autoscaling stack |
| [`pkg/clusteragent/autoscaling/workload/controller.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/controller.go) | Main `DatadogPodAutoscaler` controller (dynamic informer, `datadoghq.com/v1alpha2`) |
| [`controller_horizontal.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/controller_horizontal.go), [`controller_vertical.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/controller_vertical.go) | Replica scaling via `/scale`; rollout-triggering for resource changes |
| [`config_retriever.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/config_retriever.go) | Remote-config subscriptions for autoscaler settings and values |
| [`pod_watcher.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/pod_watcher.go), [`pod_patcher.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/pod_patcher.go) | Workloadmeta pod tracking; resource injection consumed by the admission webhook |
| [`pkg/clusteragent/autoscaling/workload/loadstore`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/loadstore) | In-memory store of node-agent-pushed container load metrics |
| [`pkg/clusteragent/autoscaling/workload/local`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/local) | Local failover recommender (replica calculator on loadstore data) |
| [`pkg/clusteragent/autoscaling/workload/external`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/external) | Custom external recommender client (per-autoscaler annotation, optional mTLS) |
| [`pkg/clusteragent/autoscaling/workload/profile`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/profile) | `DatadogPodAutoscalerProfile` CRD support and builtin profiles |
| [`cmd/cluster-agent/api/v2/series`](<<<SRC>>>/cmd/cluster-agent/api/v2/series) | `POST /api/v2/series`: node agents push failover metrics to the leader |
| [`pkg/clusteragent/autoscaling/cluster`](<<<SRC>>>/pkg/clusteragent/autoscaling/cluster), [`cluster/spot`](<<<SRC>>>/pkg/clusteragent/autoscaling/cluster/spot) | Cluster autoscaling values (RC) and spot-node scheduling |
| [`pkg/clusteragent/autoscaling/autoscalinggate/gate.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/autoscalinggate/gate.go) | Delays expensive informers until the first autoscaler object exists |

## External metrics provider

With `external_metrics_provider.enabled: true`, the DCA runs a full Kubernetes API extension server (built on `sigs.k8s.io/custom-metrics-apiserver`) on `external_metrics_provider.port` (8443), registered in the cluster as the `external.metrics.k8s.io` APIService. The flow is: HPA controller in kube-controller-manager → kube-apiserver → APIService → DCA → answer from an in-memory or ConfigMap store that a leader-only loop keeps filled from the Datadog query API.

Two backends exist, selected by `external_metrics_provider.use_datadogmetric_crd`:

### Legacy ConfigMap store

The default (`use_datadogmetric_crd: false`, kept for compatibility). The leader's HPA controller ([`hpa_controller.go`](<<<SRC>>>/pkg/util/kubernetes/apiserver/controllers/hpa_controller.go)) watches HPA objects (and `WatermarkPodAutoscaler` CRs when `external_metrics_provider.wpa_controller` is set), extracts the external metric references, queries Datadog through the shared `Processor` ([`processor.go`](<<<SRC>>>/pkg/util/kubernetes/autoscalers/processor.go), batching `external_metrics_provider.chunk_size` = 35 queries per call), and persists values into the ConfigMap `datadog-custom-metrics`. Any replica answers `GetExternalMetric` by reading the ConfigMap. There is no consistency guarantee between successive reads across replicas, and metric semantics are limited to what an HPA metric name plus label selector can express.

### DatadogMetric CRD

The recommended mode. A `DatadogMetric` CR (`datadoghq.com/v1alpha1`) carries an arbitrary Datadog query; HPAs reference it as `datadogmetric@<namespace>:<name>`. Three cooperating pieces in [`pkg/clusteragent/autoscaling/externalmetrics`](<<<SRC>>>/pkg/clusteragent/autoscaling/externalmetrics):

1. `DatadogMetricController` ([`datadogmetric_controller.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/externalmetrics/datadogmetric_controller.go)) syncs CRs into an in-memory store on every replica, and (leader only) writes status — value, freshness, errors — back to the CR so `kubectl get datadogmetric` is debuggable.
1. `MetricsRetriever` ([`metrics_retriever.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/externalmetrics/metrics_retriever.go), leader only) refreshes *active* metrics from the Datadog API every `external_metrics_provider.refresh_period` (30 s), splitting failed batches to isolate bad queries.
1. `AutoscalerWatcher` ([`autoscaler_watcher.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/externalmetrics/autoscaler_watcher.go), leader only) marks DatadogMetrics active or inactive based on whether any HPA/WPA references them, and — with `enable_datadogmetric_autogen` (default true) — autogenerates DatadogMetric CRs for HPAs that still use raw query-style metric names, expiring them after a few hours without references.

The provider ([`provider.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/externalmetrics/provider.go)) answers `GetExternalMetric` from the local store. Staleness is governed by `external_metrics_provider.max_age` (120 s) and `query_validity_period`: when Datadog is unreachable, the DCA **keeps serving the last values, flagged invalid**, and the APIService stays ready — degrading HPA freshness rather than breaking the Kubernetes metrics pipeline. Credentials come from the [`datadogclient`](<<<SRC>>>/comp/autoscaling/datadogclient) component (`external_metrics_provider.api_key/app_key/endpoint`, defaulting to the main agent keys; a redundant `endpoints` list is supported).

## Workload autoscaling: DatadogPodAutoscaler

Enabled by `autoscaling.workload.enabled`, wired by `StartWorkloadAutoscaling` in [`provider/provider.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/provider/provider.go). It hard-requires the remote-config client and a detected cluster name (startup fails otherwise), and effectively requires the [admission controller](admission-controller.md) — vertical scaling is applied to new pods by the `autoscaling` mutating webhook, and without it the vertical path silently degrades to an error log.

```text
 Datadog app (autoscaling product)
      |  remote config
      v
 CONTAINER_AUTOSCALING_SETTINGS ----> ConfigRetriever ----> PodAutoscalerInternal store
 CONTAINER_AUTOSCALING_VALUES  ---/        |                     |
                                           v                     v
                              DatadogPodAutoscaler CRs      Controller workers
                              (created/synced in cluster)    /            \
                                                   horizontal              vertical
                                                   /scale subresource      pod-template annotation patch
                                                                           -> rollout -> admission webhook
                                                                              injects resources (PodPatcher)
```

The moving parts:

- **Store and config retriever.** `PodAutoscalerInternal` objects live in a size-bounded store (`autoscaling.workload.limit`, default 1000, hash-based eviction). The `ConfigRetriever` ([`config_retriever.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/config_retriever.go)) subscribes to two RC products: `CONTAINER_AUTOSCALING_SETTINGS` — autoscaler specs created in the Datadog app, which the controller materializes as `DatadogPodAutoscaler` CRs in the cluster — and `CONTAINER_AUTOSCALING_VALUES` — the recommended replica counts and container resources computed by the backend.
- **Main controller** ([`controller.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/controller.go)): a dynamic-informer controller on `datadoghq.com/v1alpha2 DatadogPodAutoscaler` with `autoscaling.workload.num_workers` workers, reconciling cluster objects against the store in both directions (remote-created autoscalers are written to the cluster; user-created ones are ingested).
- **Horizontal scaling** ([`controller_horizontal.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/controller_horizontal.go)) applies replica recommendations through the target's `/scale` subresource, enforcing the autoscaler's policies (bounds, stabilization windows, scaling velocity).
- **Vertical scaling** ([`controller_vertical.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/controller_vertical.go)) cannot mutate running pods, so it patches the target's pod template annotations (`autoscaling.datadoghq.com/rolloutAt`, `autoscaling.datadoghq.com/rec-id`) through the shared [`pkg/clusteragent/patcher`](<<<SRC>>>/pkg/clusteragent/patcher), triggering a rollout; Deployments, Argo Rollouts, and StatefulSets are supported. As replacement pods are created, the `autoscaling` admission webhook asks the `PodPatcher` ([`pod_patcher.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/pod_patcher.go)) for the recommended resources and injects them into the pod spec.
- **Pod watcher** ([`pod_watcher.go`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/pod_watcher.go)) tracks pods per owner through [workloadmeta](workloadmeta.md), giving the controllers live replica counts and rollout state. An `autoscalinggate.Gate` delays it until the first `DatadogPodAutoscaler` exists, so clusters that never use the product do not pay the pod-informer cost.
- **Profiles** ([`profile/`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/profile)): the `DatadogPodAutoscalerProfile` CRD plus builtin profiles apply shared scaling policies across workloads.

### Recommenders and failover

Recommendations normally come from the Datadog backend via RC, but two alternative sources exist:

1. **Local failover recommender** ([`local/`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/local), active when `autoscaling.failover.enabled`): node agents continuously POST raw container-load series to the DCA at `POST /api/v2/series` ([`series.go`](<<<SRC>>>/cmd/cluster-agent/api/v2/series/series.go), leader-proxied by the `LeaderForwarder`). The series land in the [`loadstore`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/loadstore), and when connectivity to Datadog is lost, the local replica calculator keeps producing horizontal recommendations from that in-cluster data — autoscaling survives an intake outage.
1. **External custom recommenders** ([`external/`](<<<SRC>>>/pkg/clusteragent/autoscaling/workload/external)): a per-autoscaler annotation points at a user-operated recommendation service, called over HTTP(S) with optional mTLS (`autoscaling.workload.external_recommender.tls.*`).

### Cluster autoscaling and spot

Two sibling leader-only products under [`pkg/clusteragent/autoscaling/cluster`](<<<SRC>>>/pkg/clusteragent/autoscaling/cluster): `autoscaling.cluster.enabled` consumes the `CLUSTER_AUTOSCALING_VALUES` RC product for node-level scaling decisions, and `autoscaling.cluster.spot.enabled` ([`cluster/spot`](<<<SRC>>>/pkg/clusteragent/autoscaling/cluster/spot)) manages spot-node scheduling — workload patching, pod eviction and tracking — paired with the `spot` mutating webhook in the admission controller.

## Configuration

| Key | Default | Meaning |
|---|---|---|
| `external_metrics_provider.enabled` | false | Serve the external-metrics APIService |
| `external_metrics_provider.port` | 8443 | APIService port |
| `external_metrics_provider.use_datadogmetric_crd` | false | DatadogMetric CRD backend instead of the ConfigMap store |
| `external_metrics_provider.refresh_period` | 30 s | Datadog query refresh interval (CRD mode) |
| `external_metrics_provider.max_age` | 120 s | Staleness bound before a value is flagged invalid |
| `external_metrics_provider.chunk_size` | 35 | Queries batched per Datadog API call |
| `external_metrics_provider.enable_datadogmetric_autogen` | true | Autogenerate DatadogMetrics for raw-query HPAs |
| `external_metrics_provider.api_key` / `app_key` / `endpoint` | main agent keys | Datadog query credentials |
| `external_metrics_provider.wpa_controller` | false | Also watch WatermarkPodAutoscaler CRs (legacy mode) |
| `autoscaling.workload.enabled` | false | DatadogPodAutoscaler controllers |
| `autoscaling.workload.limit` | 1000 | Max autoscaler objects tracked |
| `autoscaling.workload.num_workers` | 2 | Controller worker goroutines |
| `autoscaling.failover.enabled` | false | Local failover recommender + `/api/v2/series` ingestion |
| `autoscaling.cluster.enabled` / `autoscaling.cluster.spot.enabled` | false | Cluster autoscaling and spot scheduling |

## Gotchas

- **The external metrics server never reports unready because Datadog is down.** Serving stale-but-flagged values keeps kube-apiserver's APIService healthy; killing readiness would break *all* external-metric HPAs in the cluster, including unrelated ones.
- **An app key is required** for the external metrics provider — the metrics *query* API needs it, unlike everything else the agent ships, which only needs the API key.
- **The ConfigMap backend gives no cross-replica read consistency**; two `GetExternalMetric` calls hitting different DCA replicas can observe different generations of `datadog-custom-metrics`.
- **Workload autoscaling fails startup without remote config or a cluster name**, but the missing-admission-controller case for vertical scaling is only an error log — replicas scale while resources silently never change.
- **HPA and DatadogPodAutoscaler are independent paths.** The external metrics provider serves the standard Kubernetes HPA machinery; workload autoscaling bypasses HPA entirely and writes `/scale` itself. Pointing both at one workload creates a control-loop fight.
- **Autogen DatadogMetrics are garbage-collected** a few hours after the last HPA reference disappears; pinning dashboards or alerts to autogenerated CR names is fragile.
