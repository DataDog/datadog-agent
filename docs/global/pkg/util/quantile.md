# pkg/util/quantile

## Purpose

`pkg/util/quantile` implements a DDSketch-based quantile estimator used throughout the agent to
efficiently track distributions of arbitrary float64 values. Instead of storing every individual
observation, it maps values to logarithmically-spaced bins (keys) and tracks per-bin counts. This
gives relative-error guarantees for quantile queries at a fraction of the memory cost of exact
methods.

The package also contains:

- `summary/` — a standalone sub-package for computing exact incremental summary statistics
  (count, min, max, sum, average) alongside the sketch bins.
- `sketchtest/` — test helpers for verifying sketch behavior and quantile accuracy.

## Key elements

### Config

```go
type Config struct { ... }
```

Immutable configuration that controls the sketch's accuracy and capacity. All core operations
(`Insert`, `Merge`, `Quantile`) accept a `*Config` as a first argument.

| Symbol | Default | Meaning |
|---|---|---|
| `defaultEps` | `1/128 ≈ 0.0078` | Relative accuracy per bin (half the configured epsilon is used for gamma) |
| `defaultBinLimit` | `4096` | Maximum number of bins per sketch before trimming |
| `defaultMin` | `1e-9` | Smallest non-zero absolute value representable |

**`Default() *Config`** — returns the package-level default config. This is what production code
should use unless a custom accuracy/capacity is needed.

**`NewConfig(eps, min float64, binLimit int) (*Config, error)`** — creates a custom config.
Pass `0` for any parameter to use its default value.

**`(*Config).MaxCount() int`** — returns the maximum total count the sketch can hold (binLimit ×
65535).

### Key

```go
type Key int16
```

A quantized bucket index. Values are mapped to signed int16 keys via the logarithmic gamma
mapping: a positive value `v` gets key `k` such that `γ^k ≤ v < γ^(k+1)`.  Negative values map
to the corresponding negative key. Zero and values below `config.norm.min` map to key `0`.
`±Inf` are represented by the sentinel keys `uvinf` / `uvneginf`.

### Sketch

```go
type Sketch struct {
    sparseStore
    Basic summary.Summary `json:"summary"`
}
```

The core data structure. A `Sketch` holds a sparse sorted list of `(Key, count uint16)` bins and
a `summary.Summary` for exact aggregate statistics. JSON serialization includes only the summary
fields (not the bins).

Key methods:

| Method | Description |
|---|---|
| `Insert(c *Config, vals ...float64)` | Insert individual values (prefer `InsertMany` for batches) |
| `InsertMany(c *Config, values []float64)` | Batch insert; much more efficient than `Insert` |
| `Merge(c *Config, o *Sketch)` | Merge another sketch into this one without mutating `o` |
| `Quantile(c *Config, q float64) float64` | Estimate the q-th quantile (q in [0,1]); returns min for q≤0 and max for q≥1 |
| `Copy() *Sketch` | Deep copy |
| `CopyTo(dst *Sketch)` | Deep copy into an existing allocation |
| `Reset()` | Clear all bins and statistics |
| `Equals(o *Sketch) bool` | Exact structural equality check |
| `BasicStats() (cnt, min, max, sum, avg)` | Returns summary fields; satisfies `metrics.SketchData` |
| `MemSize() (used, allocated int)` | Returns memory use in bytes |

### Agent

```go
type Agent struct {
    Buf      []Key
    CountBuf []KeyCount
    Sketch   Sketch
}
```

An insert-optimized wrapper around `Sketch` designed for use in the aggregator hot path. It
accumulates incoming keys in an internal buffer (`Buf`, capacity 512) and flushes them to the
underlying `Sketch` in sorted batches, which is faster than inserting one at a time.

Key methods:

| Method | Description |
|---|---|
| `Insert(v float64, sampleRate float64)` | Insert a value, adjusting for sample rate; flushes buffer when full |
| `InsertInterpolate(lower, upper float64, count uint) error` | Distributes `count` observations linearly between two bucket boundaries (used to ingest pre-bucketed histogram data) |
| `Finish() *Sketch` | Flush pending inserts and return a deep copy of the sketch; returns `nil` if empty |
| `IsEmpty() bool` | True when no values have been inserted |
| `Reset()` | Clear the sketch and buffer |

The package-level `agentConfig` (`Default()`) is used implicitly by all `Agent` operations.

### DDSketch interop

**`ConvertDDSketchIntoSketch(inputSketch *ddsketch.DDSketch) (*Sketch, error)`**

Converts an upstream `github.com/DataDog/sketches-go` DDSketch into the agent's `Sketch` format.
This is used when receiving sketch data from OpenTelemetry or other sources that produce
DDSketches. The conversion re-maps bin indexes to match the agent's gamma/bias convention and
round-converts float bin counts to unsigned integers while preserving the total count.

### summary.Summary

```go
// pkg/util/quantile/summary
type Summary struct {
    Min, Max, Sum, Avg float64
    Cnt                int64
}
```

A standalone accumulator for exact statistics. Supports `Insert(v)`, `InsertN(v, n)`, and
`Merge(o Summary)`. Used as the `Basic` field of every `Sketch`.

## Usage

### Aggregator (DogStatsD distribution metrics)

The aggregator keeps a `sketchMap` — a `map[timestamp]map[contextKey]*quantile.Agent` — that
receives metric samples on the hot path:

```go
// pkg/aggregator/sketch_map.go
type sketchMap map[int64]map[ckey.ContextKey]*quantile.Agent

func (m sketchMap) insert(ts int64, ck ckey.ContextKey, v float64, sampleRate float64) bool {
    m.getOrCreate(ts, ck).Insert(v, sampleRate)
    return true
}
```

At flush time, `Agent.Finish()` is called to obtain a `*Sketch` that is then serialized and
forwarded to the Datadog backend.

### OpenTelemetry metrics pipeline

The OTel serializer exporter (`comp/otelcol/otlp/components/exporter/serializerexporter`) and the
OTel metrics consumer (`pkg/opentelemetry-mapping-go/otlp/metrics`) both use
`ConvertDDSketchIntoSketch` to translate incoming DDSketches into the agent's format before
serialization.

### Check sampler (agent checks)

Checks that emit distribution metrics call `Sketch.Insert` directly via the check sampler:

```go
expSketch := &quantile.Sketch{}
expSketch.Insert(quantile.Default(), 10.0, 15.0)
```

### Testing

`pkg/util/quantile/sketchtest` provides a `Quantile` helper that generates test vectors and a
`DDSketchWithExactSummaryStore` for creating DDSketches in tests without the approximation layer.

## Notes

- Always use `Default()` unless you have a specific accuracy requirement. Custom configs risk
  serialization incompatibility with the Datadog backend.
- `Agent` is not safe for concurrent use. Each goroutine (or DogStatsD worker) should own its
  own `Agent`.
- Bins are silently trimmed from the low end when the bin count exceeds `binLimit`. This trades
  accuracy at the extreme lower tail for bounded memory.
- The `Sketch` JSON representation omits bins; only the `summary` field is marshaled. The bins
  are serialized separately (e.g. via protobuf in the sketch series payload).
- The package maintains several `sync.Pool`s (`binListPool`, `keyListPool`, `overflowListPool`,
  `keyCountPool`) to reuse internal slice allocations across insert and merge operations, reducing
  GC pressure on the hot path.

## Cross-references

### How pkg/util/quantile fits into the wider pipeline

```
DogStatsD / Checks
      │  MetricSample (DistributionType)
      ▼
pkg/aggregator TimeSampler / CheckSampler
      │  sketchMap: map[ts]map[ckey.ContextKey]*quantile.Agent
      │  Agent.Insert(v, sampleRate) on every sample
      │  Agent.Finish() → *Sketch at flush time
      ▼
pkg/metrics SketchSeries / SketchSeriesList
      │  SketchPoint.Sketch satisfies SketchData via *Sketch
      │  Cols() + BasicStats() used by the serializer
      ▼
pkg/serializer internal/metrics/SketchSeriesList
      │  serializes via protobuf (sketch series payload)
      ▼
comp/forwarder/defaultforwarder → Datadog backend
```

### Related documentation

| Document | Relationship |
|----------|--------------|
| [`pkg/aggregator`](../aggregator/aggregator.md) | `TimeSampler` and `CheckSampler` maintain a `sketchMap` (keyed by `ckey.ContextKey`) of `*quantile.Agent` instances. At each flush tick, `Agent.Finish()` drains the internal key buffer and returns a `*Sketch` that is wrapped in a `metrics.SketchPoint` and appended to a `SketchSeriesList`. |
| [`pkg/metrics`](metrics/metrics.md) | Defines `SketchData` (the interface satisfied by `*quantile.Sketch`), `SketchSeries`, and `SketchSeriesList`. The `Cols()` and `BasicStats()` methods on `Sketch` are the serializer's access points for bin data and summary statistics. |
| [`pkg/serializer`](../serializer.md) | `internal/metrics/SketchSeriesList` serializes `SketchData` values to protobuf. The serializer calls `Cols()` (bin keys and counts) and `BasicStats()` (count, min, max, sum, avg) to populate the sketch series wire payload sent to the Datadog intake. |
