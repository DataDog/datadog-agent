# pkg/util/size

## Purpose

`pkg/util/size` provides utilities for computing the in-memory size of Go data structures and for parsing human-readable byte-size strings (e.g. `"512MB"`, `"1GB"`). It is used wherever the agent needs to reason about memory consumption of its internal data, or to read size-valued configuration options.

---

## Key elements

### Interface

**`HasSizeInBytes`** — an interface that any object can implement to expose its own size accounting:

| Method | Returns |
|---|---|
| `SizeInBytes() int` | Size of the object's struct/header in bytes (excluding content) |
| `DataSizeInBytes() int` | Size of the object's content (payload) in bytes |

This split allows callers to separately account for structural overhead vs. actual data, which is useful for capacity estimates (e.g. tag stores, context resolvers).

### Size helpers

**`SizeOfStringSlice(s []string) int`** — returns the size of the slice header plus one `string` header per element (does *not* count the bytes of the string values themselves).

**`DataSizeOfStringSlice(v []string) int`** — returns the sum of `len(s)` for all strings in the slice (the actual string bytes).

Both functions use `unsafe.Sizeof` constants computed at package init time to avoid repeated reflection.

### Parser

**`ParseSizeInBytes(sizeStr string) uint`** — converts a human-readable string to a `uint` number of bytes. Accepted units (case-insensitive):

| Suffix | Multiplier |
|---|---|
| `b` / `B` | 1 |
| `kb` / `KB` | 1 024 |
| `mb` / `MB` | 1 048 576 |
| `gb` / `GB` | 1 073 741 824 |

Returns `0` on parse error or overflow (multiplication overflow is detected explicitly).

---

## Usage

### Memory accounting in aggregator structures

`pkg/aggregator/context_resolver.go` and `pkg/aggregator/internal/tags/store.go` implement `HasSizeInBytes` so the aggregator can report its own memory footprint through the agent's internal metrics:

```go
func (r *contextResolver) SizeInBytes() int {
    return int(unsafe.Sizeof(*r)) + size.SizeOfStringSlice(r.tags)
}

func (r *contextResolver) DataSizeInBytes() int {
    return size.DataSizeOfStringSlice(r.tags)
}
```

### DogStatsD packet buffers

`comp/dogstatsd/packets/types.go` uses `HasSizeInBytes` to account for packet pool memory so the server can enforce memory limits.

### Configuration parsing

`ParseSizeInBytes` mirrors the behaviour of Viper's own size parser and is used wherever an agent config value represents a byte size that must be interpreted in Go code (e.g. buffer limits, flush thresholds).

---

## Related packages and components

- **`pkg/aggregator`** — `pkg/aggregator/context_resolver.go` and `pkg/aggregator/internal/tags/store.go` are the primary consumers of `HasSizeInBytes`. They implement `SizeInBytes()` / `DataSizeInBytes()` so the aggregator can report its own memory footprint through internal metrics. See [pkg/aggregator docs](../aggregator/aggregator.md).
- **`comp/dogstatsd/packets`** — `Packet` and `Packets` both implement `HasSizeInBytes` (enforced with `var _ size.HasSizeInBytes = (*Packet)(nil)` compile-time assertions in `types.go`). This lets the DogStatsD server account for packet pool memory and enforce configurable memory limits. See [comp/dogstatsd/packets docs](../../comp/dogstatsd/packets.md).
- **`pkg/collector/corechecks/agentprofiling`** — calls `ParseSizeInBytes` to interpret a size-valued configuration key for profiling buffer limits.
