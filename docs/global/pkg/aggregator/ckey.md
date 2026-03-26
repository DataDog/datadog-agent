# pkg/aggregator/ckey

## Purpose

`ckey` provides deterministic, allocation-free hashing of a metric *context* — the triple of (metric name, hostname, tag set) that uniquely identifies a time series. The resulting `ContextKey` is a `uint64` used as a map key throughout the aggregator and metrics pipeline to group samples belonging to the same context without repeated string comparisons or heap allocations.

The design choices are intentional:
- A 64-bit integer key exploits Go runtime fast-paths (`mapassign_fast64`, `mapaccess2_fast64`), making map lookups significantly faster than string-keyed maps.
- All hashing is performed in-place on pre-allocated buffers (`tagset.HashingTagsAccumulator`) so no heap allocation occurs on the hot path.
- Tags are deduplicated and order-normalised before hashing, so `{"env:prod","region:us"}` and `{"region:us","env:prod"}` produce the same key.
- The hash algorithm is Murmur3 (128-bit internally, upper 64 bits returned). Benchmarks against FNV-1a and xxhash showed no benefit; Murmur3 was the best fit.

A separate `TagsKey` (`uint64`) hashes only the tag portion, used when callers need to compare tag sets independently of the metric name or hostname (e.g. the `GenerateWithTags2` two-accumulator variant).

## Key elements

| Symbol | Kind | Description |
|--------|------|-------------|
| `ContextKey` | `type uint64` | Hash of (name, hostname, tags). Used as map key in the aggregator. |
| `TagsKey` | `type uint64` | Hash of the tag set only. |
| `KeyGenerator` | `struct` | Stateful generator wrapping a `tagset.HashGenerator`. Not safe for concurrent use; each goroutine/sampler should own one. |
| `NewKeyGenerator() *KeyGenerator` | constructor | Allocates a new generator (cheap; allocates one `tagset.HashGenerator`). |
| `(*KeyGenerator).Generate(name, hostname string, tagsBuf *tagset.HashingTagsAccumulator) ContextKey` | method | Returns a `ContextKey`. Re-arranges `tagsBuf` in place to remove duplicates. |
| `(*KeyGenerator).GenerateWithTags(...) (ContextKey, TagsKey)` | method | Same as `Generate` but also returns the `TagsKey`. |
| `(*KeyGenerator).GenerateWithTags2(name, hostname string, l, r *tagset.HashingTagsAccumulator) (ContextKey, TagsKey, TagsKey)` | method | Combines two tag accumulators (e.g. check tags + host tags) without merging them into a single buffer. Returns a key for the combined set plus separate keys for each half. |
| `Equals(a, b ContextKey) bool` | function | Convenience equality check. |
| `(ContextKey).IsZero() bool` | method | Returns `true` for the zero value (unset key). |

### Hash construction

```
tagsHash  = murmur3(tags)          // from tagset.HashGenerator
combined  = murmur3_seed(tagsHash, name)
           then murmur3_seed(combined, hostname)
ContextKey = upper 64 bits of the 128-bit result
```

Name and hostname are not XOR'd with the tag hash (which could cause cancellation) but instead chained as seeded Murmur3 inputs.

## Usage

### In the aggregator

`TimeSampler` and `CheckSampler` hold a `*ckey.KeyGenerator` and call `Generate`/`GenerateWithTags` on every incoming metric sample. The resulting `ContextKey` is stored in `ContextMetrics` (a `map[ckey.ContextKey]...`) to accumulate values for the same time series into a single bucket.

```go
// pkg/aggregator/time_sampler.go (simplified)
key, tagsKey := sampler.keyGenerator.GenerateWithTags(name, hostname, tagsBuf)
sampler.metricsByTimestamp[ts][key].AddSample(sample)
```

The `tags.Store` (in `pkg/aggregator/internal/tags`) also uses `TagsKey` to deduplicate tag-set storage across contexts.

### In metrics serialization

`pkg/metrics` uses `ContextKey` in `Series`, `SketchSeries`, and `ContextMetrics` to associate flushed metrics with their originating context, and in `CheckMetrics` to deduplicate check-level metrics.

### Go module

`ckey` lives in its own `go.mod` (`pkg/aggregator/ckey/go.mod`) and is imported by the aggregator, metrics, and serializer packages via the standard module path `github.com/DataDog/datadog-agent/pkg/aggregator/ckey`.

## Cross-references

### How ckey fits into the wider pipeline

```
pkg/tagset.HashingTagsAccumulator  (tagger tags + metric tags, post-Dedup2)
      │
      │  KeyGenerator.GenerateWithTags(name, host, tagsBuf)
      │  or GenerateWithTags2(name, host, taggerAcc, metricAcc)
      ▼
ckey.ContextKey  ─── map key in ───►  pkg/metrics.ContextMetrics
                                       pkg/metrics.CheckMetrics
                                       pkg/aggregator.sketchMap
                                       pkg/aggregator/internal/tags.Store (via TagsKey)
```

### Related documentation

| Document | Relationship |
|----------|--------------|
| [`pkg/aggregator`](aggregator.md) | `TimeSampler` and `CheckSampler` each own a `*ckey.KeyGenerator` and call `GenerateWithTags` / `GenerateWithTags2` on every incoming `MetricSample`. The resulting `ContextKey` is the primary map key in `ContextMetrics`, `CheckMetrics`, and `sketchMap`. `GenerateWithTags2` is used specifically when tagger tags and metric-level tags are kept in two separate `HashingTagsAccumulator`s. |
| [`pkg/tagset`](../tagset.md) | `KeyGenerator` wraps a `tagset.HashGenerator`. The `Generate*` methods call `HashGenerator.Hash` (or `Dedup2` for the two-accumulator variant) to produce the deduplicated tag hash that is then chained with name and hostname via seeded Murmur3. `TagsKey` is also used by `pkg/aggregator/internal/tags.Store` as the deduplication key for shared tag storage. |
| [`pkg/metrics`](../metrics/metrics.md) | `ContextMetrics` and `CheckMetrics` are `map[ckey.ContextKey]Metric`. `SketchSeries` and `Serie` both carry the originating `ContextKey` field so the serializer can correlate a flushed series back to its context. |
