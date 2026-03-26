# pkg/util/prometheus

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/prometheus`

## Purpose

`pkg/util/prometheus` provides utilities for parsing Prometheus text-format metric payloads into a simple, agent-friendly data model. It is used by checks that scrape Prometheus endpoints (kubelet, containerd, OTel collector status) and need to iterate over metric families and samples without pulling in the full Prometheus SDK.

The parser handles all standard Prometheus metric types (COUNTER, GAUGE, HISTOGRAM, SUMMARY, UNTYPED), correctly grouping histogram and summary sub-series (`_bucket`, `_sum`, `_count`) back under their parent family name. It also supports optional pre-filtering to skip lines matching caller-specified strings before parsing.

## Key Elements

### Types

**`Metric`** (`map[string]string`)

A set of label name→value pairs for a single sample. Includes the `__name__` label.

**`Sample`**

A single data point:
```go
type Sample struct {
    Metric    Metric
    Value     float64
    Timestamp int64  // milliseconds since epoch; 0 if not present in the payload
}
```

**`MetricFamily`**

A named group of samples sharing the same metric name:
```go
type MetricFamily struct {
    Name    string
    Type    string    // "COUNTER", "GAUGE", "HISTOGRAM", "SUMMARY", or "UNTYPED"
    Samples []Sample
}
```

### Functions

**`ParseMetrics(data []byte) ([]MetricFamily, error)`**

Parses a raw Prometheus text-format payload. Returns one `MetricFamily` per unique metric name found, preserving declaration order. Families with no samples are discarded.

**`ParseMetricsWithFilter(data []byte, filter []string) ([]MetricFamily, error)`**

Same as `ParseMetrics`, but first strips any line containing one of the `filter` strings. Useful for dropping known-bad or irrelevant series before parsing to avoid parse errors or noise.

### Internal helpers

- `preprocessData` — normalises whitespace/line endings and applies the filter pass.
- `trimHistogramSuffix` / `trimSummarySuffix` — strip `_bucket`/`_sum`/`_count` from raw series names so they are matched to their declared `HISTOGRAM`/`SUMMARY` family.

## Usage

### Kubelet check (`pkg/collector/corechecks/containers/kubelet/`)

Multiple providers import the package as `prom` and call `ParseMetrics` (or `ParseMetricsWithFilter`) on the raw HTTP response body from a kubelet endpoint:

```go
import prom "github.com/DataDog/datadog-agent/pkg/util/prometheus"

families, err := prom.ParseMetrics(rawData)
for _, family := range families {
    for _, sample := range family.Samples {
        // Map family.Name + sample.Metric labels to a Datadog metric
    }
}
```

### Containerd check (`pkg/collector/corechecks/containers/containerd/`)

Uses `ParseMetricsWithFilter` to exclude lines whose metric names are not in an allow-list before iterating over the result.

### OTel collector status (`comp/otelcol/status/impl/`)

Parses the OTel collector's `/metrics` endpoint to extract Prometheus metrics for the agent's own status page.

### Kubernetes API server feature gates (`pkg/util/kubernetes/apiserver/common/`)

Calls `ParseMetrics` against the API server's metrics endpoint to read feature gate states.

## Cross-references

| Topic | See also |
|-------|----------|
| Agent-internal metric registration (Counter, Gauge, Histogram backed by Prometheus) | [`pkg/telemetry`](../telemetry.md) |
| fx-injected telemetry component; `RegisterCollector` for custom Prometheus collectors; `/telemetry` HTTP endpoint | [`comp/core/telemetry`](../../comp/core/telemetry.md) |

### Relationship to `pkg/telemetry` and `comp/core/telemetry`

`pkg/util/prometheus` and `pkg/telemetry` both use Prometheus under the hood, but serve opposite purposes:

- `pkg/util/prometheus` **parses** Prometheus text-format payloads produced by *external* endpoints (kubelet, containerd, OTel collector, Kubernetes API server). It is a read-only parser; it never writes to any Prometheus registry.
- `pkg/telemetry` / `comp/core/telemetry` **produce** Prometheus metrics that describe the agent's own internal health. They register instruments into a shared `prometheus.Registry` and serve them at the `/telemetry` HTTP endpoint.

The two packages share only the Prometheus text-format as a wire format. A check that scrapes an external `/metrics` endpoint with this package and wants to forward the results to Datadog does so by mapping `MetricFamily`/`Sample` values onto Datadog metric types — it does not re-register them with `pkg/telemetry`.

If you need to expose a custom `prometheus.Collector` in the agent's own telemetry, use `comp/core/telemetry`'s `RegisterCollector` instead.
