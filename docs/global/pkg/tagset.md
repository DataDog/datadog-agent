# pkg/tagset

## Purpose

`pkg/tagset` provides efficient, immutable data structures for representing and
manipulating sets of metric tags. It is designed for the high-throughput path
inside the aggregator and DogStatsD where the same tag sets are reused across
thousands of metrics per second, and where redundant tag computation and GC
pressure must be minimised.

The package delivers three things:

1. **Immutable, pre-hashed tag sets** (`HashedTags`) — each tag string is stored
   alongside its MurmurHash3 fingerprint so subsequent identity checks are
   hash comparisons rather than full string comparisons.
2. **Mutable accumulators** for building tag sets incrementally, with two
   flavors depending on whether hashes are needed.
3. **A composite view** (`CompositeTags`) that merges two tag slices without
   copying, intended for the aggregator's per-metric context.

## Key Elements

### Interface

```go
type TagsAccumulator interface {
    Append(...string)
    AppendHashed(HashedTags)
}
```

Both accumulator types implement this interface, enabling callers to be
agnostic about whether hashing is performed.

### HashedTags

```go
type HashedTags struct { /* unexported */ }

func NewHashedTagsFromSlice(tags []string) HashedTags
func (h HashedTags) Get() []string          // mutable; avoid in new code
func (h HashedTags) Slice(i, j int) HashedTags
func (h HashedTags) Len() int
func (h HashedTags) Copy() []string
```

An **immutable** snapshot of a tag set. Internally it keeps a parallel
`[]uint64` of murmur3 hashes aligned with the `[]string` data slice. Once
created it must not be modified; the `Get()` accessor is kept for legacy
callers and should not be used in new code.

### HashingTagsAccumulator

```go
func NewHashingTagsAccumulator() *HashingTagsAccumulator
func NewHashingTagsAccumulatorWithTags(tags []string) *HashingTagsAccumulator

// Core mutations
func (h *HashingTagsAccumulator) Append(tags ...string)
func (h *HashingTagsAccumulator) AppendHashed(src HashedTags)
func (h *HashingTagsAccumulator) RetainFunc(keep func(string) bool) int
func (h *HashingTagsAccumulator) SortUniq()
func (h *HashingTagsAccumulator) Reset()
func (h *HashingTagsAccumulator) Truncate(n int)

// Output
func (h *HashingTagsAccumulator) Get() []string
func (h *HashingTagsAccumulator) Hashes() []uint64
func (h *HashingTagsAccumulator) Hash() uint64   // XOR of all tag hashes
func (h *HashingTagsAccumulator) Dup() *HashingTagsAccumulator
```

The primary accumulator used by the aggregator. Tags and their hashes are
appended together, keeping the two slices in sync at all times. The internal
buffer starts at capacity 128 and grows as needed; `Reset()` returns to length
zero without releasing memory, making the accumulator safe to pool or reuse in
a tight loop.

`SortUniq()` sorts both slices in tandem using the hash as the primary sort key,
then removes duplicates in a single pass — making it cheaper than sorting
strings alone.

### HashlessTagsAccumulator

```go
func NewHashlessTagsAccumulator() *HashlessTagsAccumulator
func NewHashlessTagsAccumulatorFromSlice(data []string) *HashlessTagsAccumulator

func (h *HashlessTagsAccumulator) Append(tags ...string)
func (h *HashlessTagsAccumulator) AppendHashed(src HashedTags)
func (h *HashlessTagsAccumulator) AppendHashlessAccumulator(src *HashlessTagsAccumulator)
func (h *HashlessTagsAccumulator) SortUniq()
func (h *HashlessTagsAccumulator) Get() []string
func (h *HashlessTagsAccumulator) Copy() []string
func (h *HashlessTagsAccumulator) Reset()
```

A lighter-weight accumulator for contexts where hashes are never needed (for
example when constructing a tag slice that will be passed directly to a
serializer without further deduplication). It skips all hash computation.

### HashGenerator

```go
func NewHashGenerator() *HashGenerator
func (g *HashGenerator) Hash(tb *HashingTagsAccumulator) uint64
func (g *HashGenerator) Dedup2(l, r *HashingTagsAccumulator)
```

A **non-thread-safe**, reusable helper for computing a set-level hash from a
`HashingTagsAccumulator`. It uses three strategies depending on the number of
tags to avoid heap allocations:

- **n <= 4**: brute-force O(n²) loop using an on-stack buffer.
- **4 < n < 512**: open-addressed hashset backed by a fixed-size on-stack array
  (`hashSetSize = 512`).
- **n >= 512**: sort-then-dedup via `SortUniq()`.

`Dedup2` applies the same logic to two accumulators simultaneously, ensuring
each tag is present in at most one of the two (useful when splitting tags from
different sources before computing a combined context key).

`HashGenerator` should be kept as a field on long-lived structs (e.g., the
aggregator's `contextResolver`) so the scratch arrays are not re-allocated on
each call.

### CompositeTags

```go
func NewCompositeTags(tags1, tags2 []string) CompositeTags
func CompositeTagsFromSlice(tags []string) CompositeTags
func CombineCompositeTagsAndSlice(ct CompositeTags, tags []string) CompositeTags

func (t CompositeTags) ForEach(callback func(string))
func (t CompositeTags) ForEachErr(callback func(string) error) error
func (t CompositeTags) Find(callback func(string) bool) bool
func (t CompositeTags) Len() int
func (t CompositeTags) Join(sep string) string
func (t CompositeTags) MarshalJSON() ([]byte, error)
func (t CompositeTags) UnsafeToReadOnlySliceString() []string
```

A zero-copy view over two `[]string` slices. Used by `Context` in the
aggregator to combine tagger-sourced tags with metric-level tags without
allocating a merged slice on the hot path. Iteration via `ForEach` spans both
internal slices transparently. `UnsafeToReadOnlySliceString` returns an
allocation-free view if `tags2` is nil, otherwise it concatenates the two
slices.

## Usage

The package is consumed primarily by the aggregator and the tagger:

- **`pkg/aggregator/context_resolver.go`** — creates a `HashingTagsAccumulator`
  for tagger tags and another for metric tags; uses `HashGenerator.Dedup2` to
  eliminate cross-source duplicates, then passes a `CompositeTags` into each
  `Context`.
- **`comp/core/tagger`** — stores resolved tag sets as `HashedTags` in the tag
  store and passes them through `AppendHashed` to accumulate tags across
  multiple sources.
- **`pkg/metrics`** — `SketchSeries` and related types carry `CompositeTags`
  as the tag representation forwarded to the serializer.
- **`pkg/collector/python/loader.go`** — bridges Python check tags through the
  `TagsAccumulator` interface.

### Typical accumulator pattern

```go
gen := tagset.NewHashGenerator()           // reuse across calls
acc := tagset.NewHashingTagsAccumulator()  // or reuse with Reset()

acc.Append("env:prod", "service:web")
acc.AppendHashed(taggerTags)

hash := gen.Hash(acc)                      // deduplicates in-place, returns XOR hash
tags := acc.Get()                          // []string after dedup
```

### Splitting tagger and metric tags

```go
taggerAcc := tagset.NewHashingTagsAccumulator()
metricAcc  := tagset.NewHashingTagsAccumulator()

taggerAcc.Append(taggerTags...)
metricAcc.Append(metricTags...)

gen.Dedup2(taggerAcc, metricAcc)           // removes duplicates across both

ctx.Tags = tagset.NewCompositeTags(taggerAcc.Get(), metricAcc.Get())
```

### Context key generation with ckey

`pkg/aggregator/ckey.KeyGenerator` wraps a `HashGenerator` and uses it to chain the tag hash with metric name and hostname:

```go
// inside TimeSampler / CheckSampler hot path
keyGen := ckey.NewKeyGenerator()  // wraps tagset.NewHashGenerator()

// two-accumulator path (tagger tags + metric-level tags)
ck, taggerTagsKey, metricTagsKey := keyGen.GenerateWithTags2(
    sample.Name, sample.Host, taggerAcc, metricAcc,
)
// ck: ContextKey used as map key in ContextMetrics / sketchMap
// taggerTagsKey / metricTagsKey: TagsKey values stored in pkg/aggregator/internal/tags.Store
```

`GenerateWithTags2` calls `gen.Dedup2(l, r)` internally, so no separate dedup step is needed when going through `ckey`.

### Shared tag store (aggregator_use_tags_store)

When `aggregator_use_tags_store = true`, the aggregator's `pkg/aggregator/internal/tags.Store` uses `ckey.TagsKey` as the deduplication key to share a single tag snapshot across all contexts that carry the same tag set. The store is keyed by `TagsKey` and returns a pointer to the shared `*Entry`, which holds the canonical `[]string` slice. This avoids storing duplicate tag slices for metrics from many containers with identical tag sets.

---

## Cross-references

### How tagset fits into the wider pipeline

```
comp/core/tagger.EnrichTags
        │  writes into
        ▼
tagset.TagsAccumulator (HashingTagsAccumulator)
        │
        │  HashGenerator.Dedup2
        ▼
tagset.CompositeTags  ──stored in──►  pkg/metrics.Serie.Tags
                                             │
                                             ▼
                                    pkg/serializer (stream builder)
                                    serialises tags to JSON/protobuf
```

### Related documentation

| Document | Relationship |
|----------|--------------|
| [`comp/core/tagger`](../comp/core/tagger.md) | Calls `EnrichTags(tb TagsAccumulator, originInfo)` to append resolved container/pod tags into an accumulator. Internally stores resolved tag sets as `HashedTags` and delivers them through `AppendHashed` on the accumulator. |
| [`pkg/aggregator`](aggregator/aggregator.md) | `context_resolver.go` owns the `HashGenerator` and two `HashingTagsAccumulator`s per context. Uses `Dedup2` before constructing a `CompositeTags` that is stored in each `Context`. The `ckey` package hashes the deduplicated accumulator to produce the context key. |
| [`pkg/aggregator/ckey`](aggregator/ckey.md) | `KeyGenerator` wraps a `tagset.HashGenerator`. `GenerateWithTags` calls `HashGenerator.Hash` to compute the tag hash; `GenerateWithTags2` calls `HashGenerator.Dedup2` to deduplicate two separate accumulators before hashing. `TagsKey` (the tag-only hash) is reused as a map key in `pkg/aggregator/internal/tags.Store`. |
| [`pkg/metrics`](metrics/metrics.md) | `Serie.Tags` is a `tagset.CompositeTags`. `SketchSeries` also carries `CompositeTags`. The serializer iterates tags via `CompositeTags.ForEach` without requiring a flat `[]string` allocation. |

### Lifecycle within a flush cycle

1. For each incoming `MetricSample`, `context_resolver.go` calls `sample.GetTags(taggerAcc, metricAcc, tagger)`.
2. `tagger.EnrichTags` appends infrastructure tags (container, pod, host) into `taggerAcc`.
3. Inline sample tags (from the check or DogStatsD payload) are appended into `metricAcc`.
4. `HashGenerator.Dedup2(taggerAcc, metricAcc)` ensures no tag appears in both slices.
5. `ckey.KeyGenerator.GenerateWithTags(name, host, taggerAcc, metricAcc)` produces the `ContextKey` for map lookup.
6. At flush, `ContextMetrics.Flush()` produces `[]*metrics.Serie`; each `Serie.Tags` is set to `NewCompositeTags(taggerAcc.Get(), metricAcc.Get())`.
7. The serializer's stream builder iterates `CompositeTags.ForEach` and writes each tag string into the JSON/protobuf payload without ever materialising a merged `[]string`.
