# pkg/util/sort

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/sort`

## Purpose

`pkg/util/sort` provides allocation-free sorting and deduplication helpers tuned for the agent's hot paths. The standard library `sort.Strings` uses an interface internally that trips the Go escape analyser and causes a heap allocation even for in-place sorts. On high-frequency code paths (e.g., sorting tag slices before hashing every metric sample), those allocations add up. This package avoids them for short slices.

The design is grounded in benchmark data (see `pkg/util/sort/sort_benchmarks_note.md`): for slices up to ~40 elements, an insertion sort is faster than `sort.Strings` and allocates nothing. Above 40 elements `sort.Strings` wins and the allocation cost becomes negligible relative to the work done, so the stdlib is used automatically.

## Key Elements

### `InsertionSortThreshold = 40` (const)

The crossover point. Below this length, `InsertionSort` is preferred; at or above it, `UniqInPlace` delegates to `sort.Strings`.

### `InsertionSort(elements []string)`

In-place insertion sort. Zero allocations. Best for slices whose length is known to be small (tag counts, label lists). For slices larger than `InsertionSortThreshold` consider `sort.Strings` directly if allocation is acceptable.

### `UniqInPlace(elements []string) []string`

Sorts `elements` and removes duplicates, returning a sub-slice of the original backing array. No additional memory is allocated for small slices (uses `InsertionSort`). The input slice is modified in place; the returned slice is the authoritative deduplicated view. Slices with fewer than 2 elements are returned unchanged.

## Usage

### Sorting tag slices before context key generation

The aggregator's context key computation must sort tags to produce canonical keys regardless of the order they arrive. `UniqInPlace` is used here to both sort and deduplicate in a single allocation-free pass:

```go
import utilsort "github.com/DataDog/datadog-agent/pkg/util/sort"

tags = utilsort.UniqInPlace(tags)
// tags is now sorted and deduplicated, ready for hashing
```

Real-world callers include:
- `pkg/aggregator/aggregator.go` — tag normalization before metric aggregation
- `comp/dogstatsd/server/server.go` — DogStatsD tag processing
- `pkg/tagset/hashless_tags_accumulator.go` — tag accumulator deduplication
- `pkg/util/cloudproviders/cloudproviders.go` — host tag list deduplication
- `comp/metadata/host/hostimpl/hosttags/tags.go` — host tag merging

### Using `InsertionSort` directly

When you know the slice is already unique (or you don't need deduplication), call `InsertionSort` directly to avoid the extra pass:

```go
utilsort.InsertionSort(myTags)
```

### Performance note

From the benchmarks in `sort_benchmarks_note.md`, for 40-element slices the times are roughly equal between `InsertionSort` and `sort.Strings` (~21 µs). The real advantage of `InsertionSort` at that size is the absence of allocations, not raw speed. For 15-element slices, insertion sort is ~7 % faster and completely allocation-free.

## Cross-references

### How `pkg/util/sort` fits into the tag pipeline

`pkg/util/sort` is a leaf dependency: it is imported by higher-level packages but itself has no agent dependencies.

```
pkg/util/sort.UniqInPlace
    │
    ├── pkg/tagset.HashlessTagsAccumulator.SortUniq()
    │       └── called by aggregator context resolver, DogStatsD tag processing
    │
    └── pkg/aggregator (direct import)
            └── tag normalization before context key computation (ckey.md)
```

### `pkg/tagset` — primary consumer

`pkg/tagset/hashless_tags_accumulator.go` imports `pkg/util/sort` directly. Its `SortUniq()` method delegates to `sort.UniqInPlace` to sort and deduplicate the internal tag slice in a single zero-allocation pass. The heavier `HashingTagsAccumulator` uses a hash-based dedup strategy instead (see `tagset.HashGenerator`), but `HashlessTagsAccumulator.SortUniq()` is the direct caller in paths where hashes are not needed.

See: [`pkg/tagset`](../tagset.md)

### `pkg/aggregator` — context key normalization

The aggregator calls `UniqInPlace` on raw tag slices before passing them to `ckey.KeyGenerator.GenerateWithTags`. Canonical, sorted, deduplicated tags are required to produce a stable `ContextKey` hash regardless of emission order.

See: [`pkg/aggregator`](../aggregator/aggregator.md), [`pkg/aggregator/ckey`](../aggregator/ckey.md)

### Related utility packages

| Package | Relationship |
|---------|--------------|
| [`pkg/util/slices`](slices.md) | Provides `Map` for slice projection; `pkg/util/sort` is complementary (sorting/dedup rather than transformation). |
| [`pkg/util/strings`](strings.md) | `Matcher.NewMatcher` also sorts and deduplicates its input list, but for a different purpose (binary-search matching) and over a one-time setup cost rather than a hot path. |
