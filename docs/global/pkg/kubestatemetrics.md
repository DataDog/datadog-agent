> **TL;DR:** `pkg/kubestatemetrics` embeds kube-state-metrics in-process inside the Datadog Cluster Agent, adding a memory-efficient custom `MetricsStore`, optional workloadmeta-backed pod collection, and event-callback hooks for resource lifecycle reactions.

# pkg/kubestatemetrics

## Purpose

`pkg/kubestatemetrics` embeds the upstream
[kube-state-metrics](https://github.com/kubernetes/kube-state-metrics) (KSM)
library directly inside the Datadog Cluster Agent. Instead of running KSM as a
separate deployment, the Cluster Agent hosts it in-process and drives the
`kubernetes_state_core` cluster check.

The package extends upstream KSM in three ways:

1. **Custom `MetricsStore`** — stores only the Datadog-relevant subset of
   metrics (labels + float64 value) rather than full Kubernetes objects,
   reducing memory footprint.
2. **Workloadmeta-backed pod collection** — pods can be sourced from the
   in-process workloadmeta store (populated by the Kubelet collector) instead
   of the Kubernetes API server, cutting API-server load on large clusters.
3. **Event callbacks** — callers can register hooks (`StoreEventCallback`) that
   fire whenever an object is added, updated, or deleted in a store. This is
   used by the Cluster Agent to react to resource lifecycle events without an
   additional informer.

**Build constraint:** all files in this package require the `kubeapiserver`
build tag. The workloadmeta-backed pod collector additionally requires `kubelet`.

---

## Key elements

### Key types

#### `builder/` — store factory

| Symbol | Description |
|--------|-------------|
| `Builder` | Central builder struct. Wraps the upstream `ksmtypes.BuilderInterface` and adds Datadog-specific extensions. Created via `New()`. |
| `Builder.Build()` | Returns an upstream `metricsstore.MetricsWriterList` (Prometheus-compatible). Used when the agent exposes a `/metrics` endpoint. |
| `Builder.BuildStores()` | Returns `[][]cache.Store` — the `MetricsStore` instances that the cluster check polls via `Push()`. Also starts the workloadmeta reflector if configured. |
| `GenerateStores[T]()` | Generic free function. Applies the allow/deny filter, composes metric-generation functions, creates `MetricsStore` instances per namespace, and starts the appropriate reflector (API-server list/watch or workloadmeta). |
| `Builder.WithPodCollectionFromWorkloadmeta(store)` | Switches pod collection to workloadmeta. When set, `GenerateStores` creates a `workloadmetaReflector` instead of a Kubernetes reflector for pod resources. |
| `Builder.WithUnassignedPodsCollection()` | Adds a `spec.nodeName=` field selector so only unscheduled pods are collected (used by the Cluster Agent when each node agent handles its own pods). |
| `Builder.WithCallbacksForResources([]string)` | Marks resource types for which event callbacks should be enabled on the created `MetricsStore`. |
| `Builder.RegisterStoreEventCallback(resourceType, eventType, cb)` | Registers a `StoreEventCallback` for a `(resourceType, eventType)` pair. Thread-safe. |
| `Builder.NotifyStoreEvent(eventType, resourceType, obj)` | Called by a `MetricsStore` when an object changes. Looks up and calls the registered callback. |
| `cacheEnabledListerWatcher` | Wraps an upstream `ListerWatcher`; uses `ResourceVersionMatch=NotOlderThan` on List calls to read from the API-server cache rather than etcd (reduces load). |
| `workloadmetaReflector` | Bridges workloadmeta pod events into `cache.Store.Add/Update/Delete` calls. Requires `kubeapiserver` + `kubelet` build tags. |

### Key interfaces

#### `store/` — custom metrics store

| Symbol | Description |
|--------|-------------|
| `MetricsStore` | Implements `cache.Store` (client-go). Stores `[]DDMetricsFam` per Kubernetes object UID. Thread-safe via `sync.RWMutex`. |
| `DDMetricsFam` | A metric family: `{ Type, Name string; ListMetrics []DDMetric }`. |
| `DDMetric` | A single metric sample: `{ Labels map[string]string; Val float64 }`. |
| `NewMetricsStore(generateFunc, metricsType)` | Constructor. `generateFunc` is the composed KSM metric generator. |
| `MetricsStore.Add/Update/Delete` | Standard `cache.Store` methods. On `Add`/`Update`, calls `generateFunc(obj)` and stores the result keyed by UID. Fires callbacks if enabled. |
| `MetricsStore.Push(familyFilter, metricFilter)` | Primary consumer API. Returns `map[string][]DDMetricsFam` filtered by the caller-provided predicates. Used by the cluster check to extract metrics at each tick. |
| `MetricsStore.EnableCallbacks(notifier)` | Arms the store to call `notifier.NotifyStoreEvent` on every state change. |
| `StoreEventType` | String enum: `EventAdd`, `EventUpdate`, `EventDelete`. |
| `StoreEventCallback` | `func(eventType StoreEventType, resourceType, namespace, name string, obj interface{})`. |
| `EventNotifier` | Interface with `NotifyStoreEvent`; implemented by `Builder` to decouple the store from the builder import. |
| `FamilyAllow` / `MetricAllow` | Filter function types passed to `Push`. Pre-built `GetAllFamilies` and `GetAllMetrics` pass everything through. |
| `ExtractNamespaceAndName(obj)` | Helper that extracts namespace and name from any Kubernetes object (type-switches common types, falls back to `metav1.Object`). |

### Configuration and build flags

All files require the `kubeapiserver` build tag. The workloadmeta-backed pod collector additionally requires `kubelet`. There are no dedicated `datadog.yaml` keys for this package; behavior is controlled by builder options and the `kubernetes_state_core` check configuration.

---

## Usage

### Cluster check setup

`pkg/collector/corechecks/cluster/ksm/kubernetes_state.go` is the primary
consumer. At check initialisation it:

1. Calls `builder.New()` and configures it with `WithKubeClient`, `WithContext`,
   `WithNamespaces`, `WithFamilyGeneratorFilter`, etc.
2. Optionally calls `WithPodCollectionFromWorkloadmeta` to avoid hitting the API
   server for pods.
3. Calls `BuildStores()` to get `[][]cache.Store`, which are `*MetricsStore`
   instances backed by live reflectors.
4. At each check run, calls `store.Push(familyFilter, metricFilter)` to pull
   the latest metrics and forwards them to the Datadog intake.

### Filtering metrics

```go
// Accept only gauge families whose name starts with "kube_pod_"
familyFilter := func(f store.DDMetricsFam) bool {
    return strings.HasPrefix(f.Name, "kube_pod_")
}
metrics := metricsStore.Push(familyFilter, store.GetAllMetrics)
```

### Registering event callbacks

```go
b := builder.New()
b.WithCallbacksForResources([]string{"*v1.Deployment"})
b.RegisterStoreEventCallback("*v1.Deployment", store.EventAdd, func(
    eventType store.StoreEventType,
    resourceType, namespace, name string,
    obj interface{},
) {
    // React to new Deployment objects
})
```

Callbacks fire synchronously from within `MetricsStore.Add/Update/Delete`, so
implementations should be fast or hand off to a goroutine.

### Importers

| Path | Role |
|------|------|
| `pkg/collector/corechecks/cluster/ksm/kubernetes_state.go` | Main cluster check; consumes `builder.Builder` and `store.MetricsStore`. |
| `pkg/collector/corechecks/cluster/ksm/kubernetes_state_*.go` | Transformers, aggregators, label-join helpers — all operate on `store.DDMetricsFam` / `store.DDMetric`. |
| `test/benchmarks/kubernetes_state/main.go` | Standalone benchmark that exercises the store directly. |

---

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`pkg/util/kubernetes/apiserver`](util/kubernetes-apiserver.md) | `builder.GenerateStores` calls `apiserver.GetAPIClient()` to obtain `APIClient.Cl` (for node/component-status queries during builder configuration) and passes `APIClient.InformerCl` / `APIClient.DynamicInformerCl` to the upstream KSM reflectors. The `cacheEnabledListerWatcher` wrapper applies `ResourceVersionMatch=NotOlderThan` so the API-server cache is hit instead of etcd — this relies on the long-timeout `InformerCl` provided by `APIClient`. Leader election state from `leaderelection.LeaderEngine` (in the same `apiserver` sub-package) gates when the KSM builder starts its reflectors. |
| [`comp/core/workloadmeta`](../../comp/core/workloadmeta.md) | `Builder.WithPodCollectionFromWorkloadmeta(store)` switches pod collection from the Kubernetes API server to the in-process workloadmeta store. The `workloadmetaReflector` inside `builder/` subscribes to `KindKubernetesPod` events from workloadmeta (populated by the kubelet collector) and translates them into `cache.Store.Add/Update/Delete` calls. This avoids a second list/watch against the API server for pods, reducing load on large clusters. |
| [`pkg/clusteragent`](clusteragent/clusteragent.md) | The `kubernetes_state_core` cluster check is scheduled by the Cluster Agent's `clusterchecks.Handler`. `pkg/clusteragent/clusterchecks` dispatches the check config to CLC runners or node agents; on the Cluster Agent pod itself, `pkg/collector/corechecks/cluster/ksm` hosts the running check. The `StoreEventCallback` mechanism in `pkg/kubestatemetrics/builder` is used by the Cluster Agent to react to resource lifecycle events (e.g. new `Deployment` objects) without a separate informer, feeding into autoscaling and admission-webhook logic. |
| [`pkg/collector`](collector/collector.md) | The KSM cluster check is a `check.Check` implementation scheduled through the standard `pkg/collector` machinery (`CheckScheduler.Schedule` → `collector.Component.RunCheck`). At each tick the worker calls `check.Run()`, which invokes `store.Push(familyFilter, metricFilter)` to pull metrics from `MetricsStore` and forwards them to the Datadog intake via the sender manager. |

### Architecture note: in-process KSM vs. standalone deployment

Running KSM inside the Cluster Agent (this package) versus deploying standalone `kube-state-metrics` has the following trade-offs:

| Aspect | In-process (`pkg/kubestatemetrics`) | Standalone KSM |
|--------|-------------------------------------|----------------|
| API-server load | Lower — single informer shared with the Cluster Agent | Higher — separate watch connections |
| Memory | Custom `MetricsStore` stores only label+value, not full objects | Full Kubernetes objects in memory |
| Operational complexity | No extra deployment | Separate Deployment, RBAC, and service account |
| Feature flags | Controlled by `kubernetes_state_core.enabled` | Independent |

The workloadmeta-backed pod collection (`WithPodCollectionFromWorkloadmeta`) further reduces API-server load by reusing pod data already present in the node agent's kubelet collector, rather than watching pods from the Cluster Agent.
