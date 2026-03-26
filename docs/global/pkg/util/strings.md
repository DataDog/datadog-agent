> **TL;DR:** Provides a binary-search-based string matcher for efficient exact/prefix lookups against a fixed set of names and a byte-budget UTF-8 truncation function.

# pkg/util/strings

Import path: `github.com/DataDog/datadog-agent/pkg/util/strings`

## Purpose

Small set of string helpers used across the agent where the standard library falls short.
It provides two focused utilities:

- **`Matcher`** — an efficient, sorted-list matcher for testing whether a string equals (or is prefixed by) a known set of strings. Used in hot paths such as metric flushing where the same list of names is tested repeatedly.
- **`TruncateUTF8`** — a byte-budget truncation function that guarantees the output is valid UTF-8, even when the input contains multi-byte code points.

## Key elements

### Key types

#### `Matcher` (struct)

```go
type Matcher struct { ... }

func NewMatcher(data []string, matchPrefix bool) Matcher
func (m *Matcher) Test(name string) bool
```

`NewMatcher` sorts and deduplicates the input slice, then (when `matchPrefix` is `true`) collapses entries that share a common prefix so that only the shortest unique prefix is retained. The resulting invariant lets `Test` perform a single binary search (`sort.SearchStrings`) to answer either exact-match or prefix-match queries in O(log n).

Key behaviours:
- A `nil` receiver for `Test` is safe and returns `false`.
- When `matchPrefix` is `false`, `Test` is a pure exact-match lookup.
- When `matchPrefix` is `true`, `Test` returns `true` if the probed string starts with any entry in the matcher.

### Key functions

#### `TruncateUTF8(s string, limit int) string`

Truncates `s` to at most `limit` bytes while preserving valid UTF-8. If the cut falls inside a multi-byte sequence, bytes are dropped one at a time from the end until the suffix is again valid. A string that is already within the limit is returned unchanged.

> Note: grapheme clusters composed of multiple code points (e.g. flag emoji built from two Regional Indicator letters) may be split; the result is still valid UTF-8 but may render differently.

## Usage

### `Matcher` — selective metric flushing

The aggregator uses `Matcher` to decide which metrics to flush on demand. A `Matcher` built from a configured filter list is sent over a channel and checked during `flushSeries` / `flushSketches`:

```go
// build once (example from pkg/aggregator)
filterList := utilstrings.NewMatcher(metricNames, false) // exact match

// check per-metric (hot path)
if filterList.Test(metricName) {
    // include in flush
}
```

The `comp/filterlist` component also wraps `Matcher` to expose a named filter-list that DogStatsD and the aggregator share.

### `TruncateUTF8` — safe byte-bounded payloads

Used wherever a string field must fit within a protocol or storage limit, for example when constructing log payloads or metadata values that must not exceed a fixed byte count.

```go
safeValue := utilstrings.TruncateUTF8(rawValue, 200)
```

## Cross-references

### `pkg/aggregator` — `Matcher` in the flush path

`pkg/aggregator` is the most import-heavy consumer of `pkg/util/strings`. The `utilstrings` alias appears in:

- `aggregator.go`, `demultiplexer_agent.go`, `demultiplexer_serverless.go` — store and propagate a `Matcher` received from `comp/filterlist` to decide which metrics to flush or drop.
- `time_sampler.go`, `time_sampler_worker.go` — `Matcher.Test` is called per-sample on the DogStatsD hot path to apply the histogram-only filter list.
- `check_sampler.go` — checks the metric filter list before committing a sample from a check.

The `Matcher` lifecycle: `NewMatcher` is called once in `comp/filterlist` when the filter list is loaded or updated from Remote Config. The resulting `Matcher` is then passed to the aggregator's channels; samplers hold a local copy and call `Test` per metric on the flush path.

See: [`pkg/aggregator`](../aggregator/aggregator.md)

### `comp/filterlist` — `Matcher` ownership

`comp/filterlist` is the canonical owner of the `Matcher` instances. It wraps `NewMatcher` to build both the full metric filter list and the histogram-only subset, then distributes them via `OnUpdateMetricFilterList` callbacks to the aggregator and DogStatsD workers. Callers receive a `utilstrings.Matcher` value (not a pointer) so a stale copy cannot be updated in place.

See: [`comp/filterlist`](../../comp/filterlist.md)

### Related utility packages

| Package | Relationship |
|---------|--------------|
| [`pkg/util/sort`](sort.md) | `NewMatcher` internally sorts and deduplicates the input list — conceptually the same operation as `pkg/util/sort.UniqInPlace`, but using `sort.Strings` (not insertion sort) because filter lists are set up once, not on the hot path. |
| [`pkg/util/maps`](maps.md) | Orthogonal: `pkg/util/maps` transforms maps; `pkg/util/strings` matches against sorted string lists. No direct dependency between the two. |
