> **TL;DR:** `comp/filterlist` maintains the metric and tag allow/deny lists used to block or strip metrics before forwarding, loaded from local config and updatable at runtime via Remote Config's `METRIC_CONTROL` product.

# comp/filterlist

**Team:** agent-metric-pipelines
**Package:** `github.com/DataDog/datadog-agent/comp/filterlist`

## Purpose

The `filterlist` component maintains the allow/deny lists used to drop metrics and strip tags from metrics before they are forwarded to Datadog. It supports two independent filter dimensions:

- **Metric filter list** – a set of metric names (exact or prefix-matched) that should be blocked entirely.
- **Tag filter list** – per-metric-name rules that either keep only a named set of tags (`include`) or remove a named set of tags (`exclude`).

Both lists are loaded from the local agent configuration on startup and can be updated at runtime via Remote Config (RC) using the `METRIC_CONTROL` product. When RC pushes an empty update, the component automatically falls back to the local configuration.

## Key Elements

### Key interfaces

`comp/filterlist/def/component.go`

```go
type Component interface {
    OnUpdateMetricFilterList(func(utilstrings.Matcher, utilstrings.Matcher))
    OnUpdateTagFilterList(func(TagMatcher))
    GetMetricFilterList() utilstrings.Matcher
    GetHistoFilterList() utilstrings.Matcher
    GetTagFilterList() TagMatcher
}
```

| Method | Description |
|---|---|
| `OnUpdateMetricFilterList(cb)` | Register a callback invoked whenever the metric filter list changes (passes full list and histogram-only subset) |
| `OnUpdateTagFilterList(cb)` | Register a callback invoked whenever the tag filter list changes |
| `GetMetricFilterList()` | Return the current `utilstrings.Matcher` for metric name blocking |
| `GetHistoFilterList()` | Return a `utilstrings.Matcher` restricted to histogram aggregate suffixes (used by DogStatsD workers) |
| `GetTagFilterList()` | Return the current `TagMatcher` |

**`TagMatcher` interface** — `comp/filterlist/def/tagmatcher.go`:

```go
type TagMatcher interface {
    ShouldStripTags(metricName string) (func(tag string) bool, bool)
}
```

`ShouldStripTags(name)` returns `(keepFn, configured)`. If `configured` is false the metric is not subject to tag filtering. If true, calling `keepFn(tag)` returns whether that tag should be retained on the metric. Tags are matched by name (the key part before `:`) using murmur3-hashed comparisons for efficiency.

### Key types

**`FilterList` struct** (`comp/filterlist/impl/filterlist.go`) — holds two independent RW-mutex-protected state pairs: one for the metric filter list and one for the tag filter list. On every update (local or RC), all registered callbacks are called synchronously under the read lock. Tags are stored as murmur3 hashes (`[]uint64`) to reduce memory usage and comparison cost.

**`Requires` / `Provides`**:

| Field | Description |
|---|---|
| `Requires.Cfg` | Agent configuration (reads `metric_filterlist`, `metric_filterlist_match_prefix`, `statsd_metric_blocklist`, `metric_tag_filterlist`) |
| `Requires.Telemetry` | Registers counters/gauges for filter list update events and sizes |
| `Provides.Comp` | The `filterlist.Component` |
| `Provides.RCListener` | Registers the RC callback for `state.ProductMetricControl` |

### Key functions

Consumers register callbacks at construction time so updates propagate automatically:
- `OnUpdateMetricFilterList(cb)` — called on each metric filter change; passes full list and histogram-only subset.
- `OnUpdateTagFilterList(cb)` — called on each tag filter change.

### Configuration and build flags

| Key | Description |
|---|---|
| `metric_filterlist` | List of metric names to block |
| `metric_filterlist_match_prefix` | If true, treat names as prefixes |
| `statsd_metric_blocklist` | Legacy alias for `metric_filterlist` |
| `statsd_metric_blocklist_match_prefix` | Legacy alias for `metric_filterlist_match_prefix` |
| `metric_tag_filterlist` | List of `{metric_name, action, tags}` objects |

## Usage

The component is wired into the agent binary in `cmd/agent/subcommands/run/command.go`:

```go
filterlist.Module()
```

Consumers register callbacks at construction time so that updates propagate automatically. The primary consumer is the aggregator's context resolver (`pkg/aggregator/context_resolver.go`), which calls `GetTagFilterList()` during metric sample tracking to strip tags before a context key is computed:

```go
func (cr *contextResolver) trackContext(
    metricSampleContext metrics.MetricSampleContext,
    timestamp int64,
    filterList filterlist.TagMatcher,
) ckey.ContextKey
```

DogStatsD workers subscribe via `OnUpdateMetricFilterList` to receive the full and histogram-only matchers so they can filter metrics in-flight without calling back into the component on the hot path.

For testing, `comp/filterlist/fx-mock/fx.go` provides a no-op mock module.

### Runtime update flow

```
Remote Config backend
  │  pushes METRIC_CONTROL product update
  ▼
comp/remote-config/rcclient
  │  invokes the RCListener callback registered by comp/filterlist
  ▼
comp/filterlist (Provides.RCListener)
  │  parses new metric/tag filter lists; falls back to local config on empty update
  │  updates internal state under RW mutex
  │  calls all registered OnUpdateMetricFilterList / OnUpdateTagFilterList callbacks
  ▼
Consumers (demultiplexer time samplers, DogStatsD workers)
  │  SetSamplersFilterList(metricMatcher, histoMatcher)
  │  ShouldStripTags(metricName) → keepFn
  ▼
Metric pipeline (metrics blocked or tags stripped before context key is computed)
```

When RC pushes an empty update the component falls back to the local `metric_filterlist` / `metric_tag_filterlist` configuration, so operators can always restore defaults without restarting the agent.

## Related components and packages

| Component / Package | Relationship |
|---|---|
| [`comp/aggregator/demultiplexer`](aggregator/demultiplexer.md) | Depends on `filterlist.Component` at construction time (`Requires.FilterList`). The demultiplexer calls `SetSamplersFilterList` on its time samplers when `OnUpdateMetricFilterList` fires, and passes the `TagMatcher` to the context resolver so tags are stripped before computing context keys. |
| [`comp/remote-config/rcclient`](remote-config/rcclient.md) | Delivers `METRIC_CONTROL` product updates to `comp/filterlist` via the fx group `rCListener` mechanism. `comp/filterlist` returns a `types.ListenerProvider` in its `Provides` struct; `rcclient` collects all group members and calls them on every RC update. |
