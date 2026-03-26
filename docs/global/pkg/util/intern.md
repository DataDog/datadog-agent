> **TL;DR:** Implements a GC-aware string interning pool that deduplicates logically equal strings to a single shared pointer, reducing memory footprint in high-frequency network tracer paths that process millions of events with repeated string values.

# pkg/util/intern

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/intern`

## Purpose

`pkg/util/intern` implements a **string interning pool**: a cache that maps logically equal strings to a single shared `*StringValue` pointer. Two calls with the same string content always return the same pointer, so equality checks between interned values reduce to a cheap pointer comparison instead of a byte-by-byte string comparison.

The primary benefit in the agent is **memory reduction**. Hot paths in the network tracer (NPM) process millions of events that carry the same container IDs, protocol names, or topic names. Storing each as a distinct `string` would duplicate the underlying bytes for every event; with interning, only one copy of the bytes exists in memory regardless of how many events reference it.

The implementation is adapted from [go4.org/intern](https://pkg.go.dev/go4.org/intern) (Brad Fitzpatrick). Interned values are held only through weak references (stored as `uintptr` in the pool map) backed by a finalizer, so the GC can reclaim strings that are no longer referenced outside the pool, preventing unbounded growth.

## Key Elements

### Key types

#### `StringValue` (struct)

The handle to an interned string. Intentionally not comparable as a value type (contains a `[0]func()` field to break struct equality), so callers must use it as a pointer (`*StringValue`).

**`(*StringValue).Get() string`** — returns the underlying string.

#### `StringInterner` (struct)

The pool itself. Safe for concurrent use (internally mutex-guarded).

### Key functions

**`NewStringInterner() *StringInterner`** — allocates a new, empty pool. Each logical domain (container IDs, topic names, etc.) should use its own interner.

**`(*StringInterner).GetString(k string) *StringValue`** — returns the canonical `*StringValue` for `k`. Creates one if it does not exist yet. The returned pointer is stable for the lifetime of the value.

**`(*StringInterner).Get(k []byte) *StringValue`** — same as `GetString` but accepts a `[]byte`. The compiler optimizes the internal map lookup to avoid allocating a `string` from the slice, making this the preferred entry point when data arrives as raw bytes.

### GC interaction

Interned values are stored in the map as raw `uintptr` (not `*StringValue`) so the GC does not count the map entry as a reference. A finalizer on each `*StringValue` removes it from the map when no live pointers remain. A resurrection guard (`resurrected` flag) handles the race between finalization and a concurrent `GetString` call.

## Usage

### Interning protocol/application labels in the network tracer

`pkg/network/protocols/kafka/statkeeper.go` uses an interner to deduplicate Kafka topic names across all tracked transactions:

```go
type StatKeeper struct {
    topicNames *intern.StringInterner
    // ...
}

func NewStatKeeper(...) *StatKeeper {
    return &StatKeeper{
        topicNames: intern.NewStringInterner(),
    }
}

func (s *StatKeeper) extractTopicName(tx *KafkaTransaction) *intern.StringValue {
    return s.topicNames.GetString(tx.TopicName)
}
```

### Interning container and connection identifiers

`pkg/network/event_common.go` uses `*intern.StringValue` (aliased as `intern.Value` from `go4.org/intern`) as the type for container IDs and connection source/destination fields:

```go
type ConnectionTuple struct {
    Source, Dest *intern.Value
}
```

### Byte-slice path (zero alloc)

When parsing binary protocol frames, prefer `Get([]byte)` over `GetString(string(buf))` to avoid an intermediate allocation:

```go
value := interner.Get(rawBytes) // no string allocation
```

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/network` | [../network/network.md](../network/network.md) | `ConnectionTuple` and related types in `event_common.go` use `*intern.StringValue` (via `go4.org/intern`) for source/destination container IDs. `pkg/network/protocols/kafka/statkeeper.go`, `stats.go`, `pkg/network/protocols/http/stats.go`, `pkg/network/protocols/http2/`, and `pkg/network/protocols/redis/stats.go` each maintain their own `StringInterner` instances to deduplicate high-cardinality strings (topic names, paths, field values) across millions of tracked transactions. `pkg/network/dns/types.go` also uses an interner for DNS hostname strings. |
| `pkg/tagset` | [../tagset.md](../tagset.md) | `pkg/tagset` uses MurmurHash3-based deduplication for tag strings at flush time, a complementary approach for the tag accumulation path. While `pkg/util/intern` handles identity deduplication for long-lived strings in the network tracer, `pkg/tagset` handles per-flush tag-set deduplication in the aggregator. The two packages target different hot paths and are not interchangeable. |
