# pkg/clusteragent/autoscaling

## Purpose

Implements the autoscaling capabilities of the Datadog Cluster Agent. The package tree covers three distinct but related features:

1. **External / Custom Metrics for HPA** — serves Datadog metrics to the Kubernetes Horizontal Pod Autoscaler via the External Metrics API.
2. **Workload autoscaling (`DatadogPodAutoscaler`)** — a Datadog-native controller that drives both horizontal and vertical pod scaling based on recommendations from the Datadog backend.
3. **Cluster autoscaling** — manages node-pool scaling (Karpenter-based) using recommendations from the Datadog backend.

All sub-packages require the `kubeapiserver` build tag.

## Package Structure

```
autoscaling/
├── store.go          # Generic in-memory Store[T] with observer callbacks
├── controller.go     # Base controller (work-queue + reconcile loop)
├── processor.go      # Reconcile helper shared by sub-controllers
├── telemetry.go      # Shared telemetry helpers
├── utils.go          # Misc utilities
│
├── custommetrics/    # Legacy HPA custom/external metrics (ConfigMap-backed store)
├── externalmetrics/  # Modern HPA external metrics (DatadogMetric CRD-backed)
├── workload/         # DatadogPodAutoscaler controller (horizontal + vertical)
└── cluster/          # Cluster autoscaling controller (NodePool / Karpenter)
```

## Key Elements

### Base infrastructure (`autoscaling/`)

| Type | Description |
|------|-------------|
| `Store[T]` | Generic thread-safe in-memory map keyed by string. Notifies registered `Observer` callbacks (`SetFunc`, `DeleteFunc`) on changes. Used by every sub-controller to hold its internal state. |
| `SenderID` | String identifier that labels which component triggered a store change. |
| `Controller` | Base Kubernetes controller built on `client-go`'s work-queue. Sub-controllers embed it and implement `reconcile(key)`. |
| `Processor` | Helper for the reconcile loop — handles requeue-on-error, max-retry logic. |

### custommetrics — legacy HPA metrics

Stores external metric values for HPAs inside a **Kubernetes ConfigMap** (one per namespace). The Cluster Agent leader polls Datadog and writes fresh metric values; followers read from the ConfigMap.

| Type / Function | Description |
|-----------------|-------------|
| `ExternalMetricValue` | A single metric value with name, labels, timestamp, autoscaler reference, and a `Valid` flag. |
| `MetricsBundle` | Collection of `ExternalMetricValue` and deprecated HPA-only values. |
| `Store` interface | CRUD interface over the ConfigMap backend (`SetExternalMetricValues`, `ListAllExternalMetricValues`, …). |
| `NewDatadogProvider(...)` | Creates a `provider.ExternalMetricsProvider` that the Kubernetes API server calls when an HPA needs a metric value. Reads from the `Store` with exponential backoff. |

Config keys: `external_metrics_provider.*`, `external_metrics.aggregator`.

### externalmetrics — DatadogMetric CRD path

The modern replacement for `custommetrics`. Metric queries are expressed as `DatadogMetric` CRDs instead of HPA annotations. This allows non-leader Cluster Agents to serve metric values without reading the ConfigMap.

| Type | Description |
|------|-------------|
| `DatadogMetricController` | Watches `DatadogMetric` CRDs (via dynamic informer). Leader retrieves fresh values from the Datadog API and updates the CRD status; all replicas answer HPA queries from the in-memory store. |
| `datadogMetricProvider` | Implements `provider.ExternalMetricsProvider` on top of `DatadogMetricsInternalStore`. |
| `MetricsRetriever` | Batches Datadog metric queries and updates `DatadogMetric` CRD status. Supports split-batch-on-error backoff. |
| `AutoscalerWatcher` | Watches HPA objects and auto-generates `DatadogMetric` CRDs from HPA metric annotations when `enable_datadogmetric_autogen` is true. |

Config keys: `external_metrics_provider.refresh_period`, `external_metrics_provider.max_age`, `external_metrics_provider.enable_datadogmetric_autogen`, `external_metrics_provider.wpa_controller`.

### workload — DatadogPodAutoscaler

Implements horizontal and vertical pod autoscaling driven by a `DatadogPodAutoscaler` CRD (Datadog Operator API `datadoghq.com/v1alpha2`).

| Type | Description |
|------|-------------|
| `Controller` | Main reconcile loop. Watches `DatadogPodAutoscaler` objects and pods. Delegates to `horizontalController` and `verticalController`. |
| `horizontalController` | Applies replica-count changes by calling the Kubernetes Scale sub-resource. |
| `verticalController` | Applies CPU/memory resource changes by annotating the pod template with `autoscaling.datadoghq.com/scaling-hash`, triggering a rolling restart. Only deployments are currently supported. Identifies in-progress rollouts by checking that all pods are owned by the same ReplicaSet. |
| `PodWatcher` | Watches pods to detect rollout completion. |
| `PodPatcher` | Interface used by the admission webhook to apply per-pod resource patches before pod creation. |
| `ConfigRetriever` | Fetches autoscaling recommendations and settings from the Datadog backend via Remote Configuration. |
| `model.PodAutoscalerInternal` | Internal representation of a `DatadogPodAutoscaler` including its current recommendations and status. |

Stale-recommendation threshold: 30 minutes (`defaultStaleTimestampThreshold`). After this, the controller stops acting on recommendations until fresh ones arrive.

### cluster — Cluster autoscaler

Controls node-pool scaling by watching Karpenter `NodePool` and `EC2NodeClass` CRDs and reconciling them against recommendations received from the Datadog backend via Remote Configuration.

| Type | Description |
|------|-------------|
| `Controller` | Watches Karpenter `NodePool` objects; applies config updates from RC (`RcClient`). |
| `ConfigRetriever` | Fetches cluster autoscaling recommendations from the Datadog RC service. |
| `model.NodePoolInternal` | Internal representation of a Karpenter NodePool with Datadog recommendations. |

Currently supports AWS EC2 node classes only (`ec2nodeclasses` GVR).

## Usage

Each sub-controller is started from the Cluster Agent startup sequence:

```
cmd/cluster-agent
  └── starts externalmetrics.DatadogMetricController  (if DatadogMetric CRDs available)
  └── starts custommetrics.NewDatadogProvider          (legacy path, ConfigMap store)
  └── starts workload.Controller                       (DatadogPodAutoscaler)
  └── starts cluster.Controller                        (Karpenter NodePool)
  └── registers externalmetrics provider with k8s API aggregation layer
```

The `autoscaling.Store[T]` + `Observer` pattern decouples producers (controllers that write recommendations) from consumers (API handlers that read metric values or the admission webhook that patches pods). Controllers register observers on the store to trigger re-reconciliation when a related object changes.

For the workload autoscaler, the `PodPatcher` interface bridges the autoscaling package to the admission controller (`mutate/autoscaling` webhook), allowing resource recommendations to be applied at pod-creation time rather than via in-place updates.
