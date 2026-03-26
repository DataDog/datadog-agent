# comp/autoscaling/datadogclient

**Team:** container-integrations

## Purpose

`comp/autoscaling/datadogclient` provides an authenticated Datadog API client used by the Cluster Agent's external metrics provider to query time-series data. The external metrics provider is the bridge between Kubernetes HPA/WPA (Horizontal/Watermark Pod Autoscaler) and Datadog metrics: it fetches metric values from the Datadog backend so that the Kubernetes autoscaler can scale workloads based on Datadog-sourced data.

The component is **conditionally active**: when `external_metrics_provider.enabled` is `false`, the constructor returns a no-op implementation (all methods return empty results). This lets callers depend on the interface unconditionally.

Two client modes are supported transparently:

| Mode | Config key | Behavior |
|---|---|---|
| Single | `external_metrics_provider.endpoint` | Queries one Datadog endpoint |
| Fallback | `external_metrics_provider.endpoints` | Queries a primary endpoint; on failure, retries secondary endpoints with exponential back-off |

The component also registers a status information provider (visible in `datadog-cluster-agent status`) that reports endpoint URLs, last query success/failure, and retry intervals.

## Key Elements

### Interface (`comp/autoscaling/datadogclient/def/component.go`)

```go
type Component interface {
    QueryMetrics(from, to int64, query string) ([]datadog.Series, error)
    GetRateLimitStats() map[string]datadog.RateLimit
}
```

| Method | Purpose |
|---|---|
| `QueryMetrics(from, to, query)` | Fetches metric time-series for the given Unix-epoch window and Datadog query string |
| `GetRateLimitStats()` | Returns the current API rate-limit headers from the most-recently-used endpoint (thread-safe) |

The interface is directly satisfied by `*datadog.Client` from `gopkg.in/zorkian/go-datadog-api.v2`. The component wraps it in a `datadogClientWrapper` that holds an `sync.RWMutex` so the client pointer can be swapped at runtime if API/app keys change.

### Key implementation types (`comp/autoscaling/datadogclient/impl/`)

**`datadogClientWrapper`** — the active `Component` implementation when the provider is enabled. It delegates all calls to an inner `Component` and refreshes the inner pointer (via `OnUpdate` callbacks on `api_key` / `app_key` config changes) without restarting the process.

**`datadogFallbackClient`** — used when `external_metrics_provider.endpoints` is configured. It maintains a list of `datadogIndividualClient` entries and on each `QueryMetrics` call iterates through them in order, skipping any that have failed recently (exponential back-off between 30 s and 30 min). If all clients have failed recently it retries the skipped ones as a last resort.

**`datadogSingleClient`** — a thin wrapper around `datadog.NewClient` that reads keys from `external_metrics_provider.api_key` / `api_key` and derives the endpoint from `external_metrics_provider.endpoint`, `DATADOG_HOST`, or `DD_SITE` (in that priority order).

### Authentication and endpoint resolution

```
Priority for API key:  external_metrics_provider.api_key → api_key
Priority for app key:  external_metrics_provider.app_key → app_key
Priority for endpoint: external_metrics_provider.endpoint → DATADOG_HOST env → site-derived URL
```

Both the single and fallback clients set `User-Agent: Datadog-Cluster-Agent` and a 3-second retry timeout.

### Status provider

`impl/status.go` implements `comp/core/status.InformationProvider` under the section "External Metrics Endpoints". It exposes per-client URL, last success/failure timestamps, current retry interval, and overall status (`OK` / `Failed` / `Unknown`). The template lives at `impl/status_templates/externalmetrics.tmpl`.

### fx module (`comp/autoscaling/datadogclient/fx/fx.go`)

```go
func Module() fxutil.Module {
    return fxutil.Component(
        fxutil.ProvideComponentConstructor(datadogclientimpl.NewComponent),
    )
}
```

### Dependencies

```go
type Requires struct {
    Config configComponent.Component
    Log    logComp.Component
}
```

### Provides

```go
type Provides struct {
    Comp           datadogclient.Component
    StatusProvider status.InformationProvider
}
```

## Usage

### Wiring the component

```go
import datadogclientfx "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/fx"

// In the cluster-agent fx app:
datadogclientfx.Module()
```

### Querying metrics

```go
type MyDeps struct {
    fx.In
    DDClient datadogclient.Component
}

func (d MyDeps) fetchValue(query string) (float64, error) {
    now := time.Now().Unix()
    series, err := d.DDClient.QueryMetrics(now-300, now, query)
    if err != nil {
        return 0, err
    }
    // process series...
}
```

### Multi-endpoint fallback configuration

```yaml
external_metrics_provider:
  enabled: true
  endpoints:
    - site: datadoghq.com
      api_key: <primary-api-key>
      app_key: <primary-app-key>
    - url: https://api.datadoghq.eu
      api_key: <secondary-api-key>
      app_key: <secondary-app-key>
```

When this key is present, `createDatadogClient` builds a `datadogFallbackClient`. The primary endpoint (from `external_metrics_provider.api_key` / `external_metrics_provider.endpoint`) is prepended to the list automatically.

### Where it is used

- `cmd/cluster-agent/subcommands/start/command.go` — wires the component into the cluster-agent
- `cmd/cluster-agent/custommetrics/server.go` — passes the component to the custom metrics server
- `pkg/clusteragent/autoscaling/externalmetrics/provider.go` — `NewDatadogMetricProvider` uses it to back Kubernetes HPA queries
- `pkg/util/kubernetes/autoscalers/processor.go` — the HPA processor calls `QueryMetrics` to evaluate autoscaling metrics
- `pkg/util/kubernetes/apiserver/controllers/` — HPA and WPA controllers pass the client to the processor

## Related packages

| Package | Relationship |
|---------|-------------|
| [`pkg/clusteragent/autoscaling`](../../pkg/clusteragent/autoscaling.md) | The principal consumer of this component. `externalmetrics.DatadogMetricController` and the `MetricsRetriever` call `QueryMetrics` to fetch time-series values from the Datadog backend, which are then written into `DatadogMetric` CRD status fields and served to the Kubernetes HPA via the external metrics API. The `custommetrics.NewDatadogProvider` (legacy ConfigMap path) also uses `QueryMetrics` through `pkg/util/kubernetes/autoscalers/processor.go`. |
| [`comp/remote-config/rcclient`](../remote-config/rcclient.md) | Used by the workload autoscaler sub-package (`pkg/clusteragent/autoscaling/workload`). The `ConfigRetriever` inside `workload` subscribes to RC products (autoscaling recommendations) via `rcclient`. This is separate from `datadogclient`, which queries the Datadog *metrics API*; RC delivers autoscaling *recommendations* while `datadogclient` delivers *metric values*. |

### How the component fits in the autoscaling pipeline

```
Kubernetes HPA / WPA
        │  ExternalMetrics API call
        ▼
pkg/clusteragent/autoscaling/externalmetrics.datadogMetricProvider
        │  reads from DatadogMetricsInternalStore (in-memory)
        │
        │  (leader only, periodic refresh)
        ▼
externalmetrics.MetricsRetriever
        │  QueryMetrics(from, to, query)
        ▼
comp/autoscaling/datadogclient            ← this component
        │  datadogFallbackClient or datadogSingleClient
        ▼
Datadog metrics API (POST /api/v1/query)
```

For the workload autoscaler (`DatadogPodAutoscaler`), scaling *recommendations* (target replica counts, resource requests) come from the Datadog backend via Remote Configuration (`comp/remote-config/rcclient`) rather than through this component.

### Runtime key refresh

`datadogClientWrapper` subscribes to `OnUpdate` callbacks for the `api_key` and `app_key` config paths. When either key changes at runtime (e.g. via RC `AGENT_CONFIG`), the inner `*datadog.Client` pointer is swapped atomically under a write lock without restarting the process or dropping in-flight requests.
