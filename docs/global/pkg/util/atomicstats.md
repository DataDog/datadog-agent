# pkg/util/atomicstats

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/atomicstats`

## Purpose

`pkg/util/atomicstats` provides a reflection-based helper for converting a stats struct into a `map[string]interface{}` suitable for use with expvar, status pages, or JSON serialization. It is designed for structs that mix plain integer fields and `go.uber.org/atomic` pointer fields so that callers do not have to write boilerplate per-field marshalling code.

Fields are opt-in via a `stats:""` struct tag. Field names are automatically converted to `snake_case` in the output map. The reporter for each type is cached after the first call, so subsequent calls pay only a reflection lookup cost.

## Key Elements

### `Report(v interface{}) map[string]interface{}`

The single public function. Takes a **pointer to a struct** and returns a snapshot of all fields tagged with `stats:""`. Panics on programming errors (non-pointer argument, unsupported field type) rather than returning an error, because these are type-level mistakes that should be caught during development.

**Supported field types:**

| Category | Types |
|---|---|
| Plain integers | `int`, `int8`, `int16`, `int32`, `int64`, `uint`, `uint8`, `uint16`, `uint32`, `uint64`, `uintptr` |
| `go.uber.org/atomic` pointers | `*atomic.Bool`, `*atomic.Duration`, `*atomic.Error`, `*atomic.Float64`, `*atomic.Int32`, `*atomic.Int64`, `*atomic.String`, `*atomic.Time`, `*atomic.Uint32`, `*atomic.Uint64`, `*atomic.Uintptr`, `*atomic.UnsafePointer`, `*atomic.Value` |

Unexported fields are supported via an `unsafe.Pointer` trick, which is necessary because `reflect.Value.Interface()` panics on unexported fields.

### Struct tag

```go
type myStats struct {
    requestCount  int64         `stats:""`
    errorCount    *atomic.Int64 `stats:""`
    internalField int64         // not exported to stats map
}
```

The tag value is always empty — the presence of the key `stats` is the only signal.

### Name conversion

Field names are converted from `CamelCase` to `snake_case`:

| Struct field | Map key |
|---|---|
| `RequestCount` | `request_count` |
| `ErrorCount` | `error_count` |
| `barbaz` | `barbaz` |

## Usage

```go
import "github.com/DataDog/datadog-agent/pkg/util/atomicstats"

type myStats struct {
    integer       int64         `stats:""`
    atomicInteger *atomic.Int64 `stats:""`
    notStats      int64
}

stats := myStats{
    integer:       10,
    atomicInteger: atomic.NewInt64(20),
    notStats:      30,
}

m := atomicstats.Report(&stats)
// m == map[string]interface{}{"integer": 10, "atomic_integer": int64(20)}
```

The result is typically passed directly to an expvar `Map.Set` or marshalled to JSON for a status endpoint.

### Contrast with `pkg/telemetry`

`pkg/util/atomicstats` and `pkg/telemetry` both surface runtime statistics, but serve different consumers:

| Package | Output | Consumer |
|---------|--------|----------|
| `atomicstats` | `map[string]interface{}` snapshot | `expvar`, agent status pages, JSON APIs — read by humans or internal tooling |
| `pkg/telemetry` | Prometheus metrics (Counter, Gauge, Histogram) | `/telemetry` endpoint, `comp/core/agenttelemetry`, and ultimately Datadog |

Use `atomicstats` for internal status pages and expvar display. Use `pkg/telemetry` when you need metrics ingested by Datadog's agent-telemetry pipeline.

## Cross-references

| Topic | See also |
|-------|----------|
| `pkg/telemetry` — the Prometheus-backed alternative for metrics that flow to Datadog | [pkg/telemetry](../telemetry.md) |
| DogStatsD server — a typical consumer of expvar-based stats that could adopt this pattern | [comp/dogstatsd/server](../../comp/dogstatsd/server.md) |
