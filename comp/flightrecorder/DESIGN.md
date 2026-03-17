# Flight Recorder — Design Document

## Purpose

The flight recorder mirrors pipeline signals (metrics, logs) from the Datadog
Agent to a Rust sidecar process over a Unix socket. The sidecar writes the data
as columnar Vortex files for offline analysis and debugging. The entire system
is designed as a passive observer: it must never backpressure the agent's main
pipeline, and its resource overhead must be negligible for the vast majority of
the fleet.

## Constraints

1. **No backpressure.** A slow or unavailable sidecar must never block or slow
   the agent's metric/log processing. Data is dropped silently rather than
   queued indefinitely.
2. **Minimal memory overhead.** The median agent (p50: 2.5K contexts, 89
   DSD/s) must see <5 MB of additional RSS. Even at the extreme tail (200K
   contexts), the overhead must remain a fraction of the agent's own footprint.
3. **Minimal CPU overhead.** The hook callback runs on the DogStatsD hot path
   at up to 100K calls/s. It must be allocation-free in the common case.
4. **Crash-safe.** The sidecar process is independent — if it crashes, the
   agent continues normally and reconnects automatically.

## Architecture

```
Agent process                          Sidecar process (Rust)
┌──────────────────────────────┐      ┌────────────────────────────┐
│ DogStatsD / check pipeline   │      │ flightrecorder binary      │
│          │                   │      │                            │
│   hook callback (zero-alloc  │      │  Unix socket listener      │
│   fast path for known ctx)   │      │          │                 │
│          │                   │      │  FlatBuffers decoder       │
│   batcher (ring buffers)     │      │          │                 │
│          │                   │      │  ContextsWriter (bloom)    │
│   FlatBuffers encoder        │      │  MetricsWriter (columnar)  │
│          │                   │      │  LogsWriter (columnar)     │
│   Unix socket transport ─────┼──────┤          │                 │
│          (reconnect loop)    │      │  Vortex files on disk      │
└──────────────────────────────┘      └────────────────────────────┘
```

## Agent side — key design decisions

### Context-key deduplication

**Problem:** Most metric traffic repeats the same (name, tags) contexts. At
100K samples/s, naively copying name + tags strings on every sample creates
millions of short-lived allocations per second, causing severe GC amplification
in Go. Benchmarks showed this adds **+97 MB RSS** at 200K contexts.

**Solution:** Compute a 64-bit FNV-1a hash of (name, tags) — the *context key*
— and split samples into two categories:

- **Context definition** (~0.1% of samples): first occurrence of a context key.
  Copies name + tags strings. Sent with `context_key` + full strings.
- **Context reference** (~99.9%): subsequent samples with a known context key.
  Stores only `context_key` + value + timestamp + sample_rate. **Zero string
  copies, zero heap allocations.**

The hash is computed inline with zero allocations (`context_key.go`). The
sidecar maintains a mapping from context keys to (name, tags) for resolution.

**Why FNV-1a?** It's simple, fast (single pass, no allocations), and produces
well-distributed 64-bit hashes. Collision probability at 200K keys is ~1e-9.
Hash collisions are benign — the sidecar simply stores two entries for the same
logical context.

### Context tracking: sharded map, not sync.Map

**Problem:** The agent needs a concurrent set to track "which context keys have
already been sent." Go's `sync.Map` costs ~120 bytes per entry (boxed
interface{} key/value). At 200K contexts, that's ~24 MB.

**Why not sync.Map:** The 120 bytes/entry overhead is inherent to `sync.Map`'s
design (interface boxing, internal `entry` struct with atomic pointer, read/dirty
map duplication). We benchmarked it and confirmed 24 MB at 200K contexts.

**Why not a bloom filter on the agent side:** A bloom filter can't remove
entries on eviction (see "Ring buffer eviction" below). We need exact membership
with deletion.

**Solution:** `contextSet` — a custom sharded map using 64 shards with
`sync.RWMutex`. Each entry is just a `uint64` key in a native Go map (~16
bytes/entry). At 200K contexts: ~5.6 MB instead of ~24 MB.

- **Hot path (known key, 99.9%):** `RLock` on one shard, map lookup, `RUnlock`.
  No write contention.
- **Cold path (new key):** Upgrades to write lock on one shard. Other shards
  remain unlocked.
- **Bounded:** Configurable capacity (default 500K). When exceeded, the set is
  cleared and all definitions are re-sent. This caps memory at a known bound.

### Split ring buffer design

**Problem:** Go pre-allocates the full backing array for slices. A single ring
buffer of `[]capturedMetric` (96 bytes/item with name, tags, value, timestamp)
wastes memory because 99.9% of entries only need the compact 32-byte reference
fields.

**Why not a unified ring:** We benchmarked a unified ring where every entry
carries full name + tags strings. At 200K contexts with high throughput, the
per-sample string copying caused +97 MB RSS from Go GC amplification — 4x worse
than the split design. The fast path's zero-allocation property is critical.

**Solution:** Two separate ring buffers:

| Buffer | Item type | Size/item | Default cap | Purpose |
|--------|-----------|-----------|-------------|---------|
| Point buffer | `metricPoint` | 32 bytes | 20,000 | Compact context references (99.9% of traffic) |
| Definition buffer | `contextDef` | ~96 bytes + strings | 2,000 | First-occurrence context definitions |

Memory: 20K × 32B × 2 (double-buffer) = **1.3 MB** for the point buffer.
The definition buffer is small (2K items) because new contexts arrive rarely
after warm-up.

Both buffers use a double-buffer swap pattern: the writer fills the "active"
buffer while the flusher drains the "drain" buffer. A single mutex guards the
swap. This avoids lock contention between the hot-path hook callbacks and the
flush goroutine.

### Ring buffer eviction and context re-send

When the definition ring overflows (more than 2K new contexts arrive between
flushes), the oldest entry is evicted. Its context key is **removed from the
context set**, so the next occurrence of that metric will be treated as a new
context and re-sent with full strings. This ensures all contexts eventually get
their definitions flushed even during warm-up bursts that exceed ring capacity.

### FlatBuffers encoding

**Why FlatBuffers:** Zero-copy deserialization on the Rust side. The Go encoder
builds the buffer, sends it over the Unix socket, and the Rust sidecar can read
fields without parsing. This minimizes sidecar CPU.

**Builder pool:** FlatBuffers builders are recycled via `sync.Pool`. After
`Reset()`, the builder retains its grown backing slice so subsequent encodes
skip the resize path. This trades ~4-8 MB of retained builder memory for
eliminating per-flush allocations.

**Encoding order:** Context definitions are encoded before data points in the
FlatBuffers vector, so the sidecar always sees a definition before any reference
to the same context key within the same batch.

### Transport: Unix socket with auto-reconnect

The agent writes length-prefixed FlatBuffers frames to a Unix domain socket.
A background goroutine handles reconnection with exponential backoff (100ms →
30s). On reconnect, the context set is reset (the sidecar lost all state), so
all context definitions will be re-sent.

`Send()` is non-blocking: if the socket is down, it returns an error
immediately. The batcher increments a counter and drops the batch.

## Sidecar side — key design decisions

### Context externalization (separate files)

**Problem:** Initially, the sidecar resolved context references in-memory using
a `HashMap<u64, (name, tags)>` and embedded the resolved name + tags strings
in every metrics file. At 200K contexts, the HashMap consumed ~50 MB RSS. After
the agent-side eviction fix (which re-sends context definitions), metrics files
ballooned from 45 MB to 190 MB because every file embedded fully resolved
strings.

**Solution:** Store context definitions in separate `contexts-*.vortex` files.
Metrics files reference contexts by `context_key` (u64) only — no name or tags
columns. A bloom filter (~120 KB) replaces the HashMap to track which context
keys have already been written to disk.

File layout:
```
/data/signals/
  contexts-{timestamp}.vortex   # context_key, name, tags
  metrics-{timestamp}.vortex    # context_key, value, timestamp_ns, sample_rate, source
  logs-{timestamp}.vortex       # content, status, tags, hostname, timestamp_ns, source
```

A bloom filter false positive just means we skip writing a redundant context
definition that's already on disk — safe because the same context_key always
maps to the same (name, tags). The filter is cleared on new agent connection
since the agent re-sends all definitions.

### Memory optimization: jemalloc + custom Vortex strategy

**Problem:** The Vortex file writing pipeline creates large temporary buffers
during serialization. With glibc's allocator, these pages were never returned
to the OS (malloc fragmentation), causing RSS to grow monotonically to ~115 MB.

**Root causes identified:**
1. **glibc malloc fragmentation:** Large mmap'd pages retained after freeing.
2. **VortexSession registry accumulation:** DashMap-based registries grow
   across writes and are never cleaned.
3. **CompressingStrategy concurrency:** Defaults to `available_parallelism()`
   (16 cores), holding 16 chunks (~32 MB) in flight simultaneously.

**Solutions:**
1. **jemalloc** as global allocator — returns pages to the OS aggressively.
2. **Fresh VortexSession per flush** — prevents registry leak across writes.
3. **Custom write strategy** (`low_memory_strategy`) — replicates the default
   Vortex pipeline but pins compression concurrency to 1 and reduces the buffer
   from 2 MB to 512 KB. This reduces transient in-flight memory from ~32 MB to
   ~2 MB.
4. **Smaller context flush batches** — capped at 2K rows (context rows carry
   long strings, so smaller batches reduce peak transient memory).
5. **write_buf replaced after flush** — releases the serialization buffer back
   to the allocator instead of retaining it.

### Minimal strategy for context files

Context files are a simple lookup table (context_key → name, tags). They don't
need the full Vortex analytical pipeline (zone maps, dictionary encoding,
BtrBlocks compression). A `minimal_strategy` is available that uses only flat
layout + repartitioning. This reduces transient memory from ~15 MB to ~2 MB per
context flush, at the cost of larger files on disk. Currently not used (the
`low_memory_strategy` with compression provides a better size/memory tradeoff),
but available as an escape hatch if RSS needs to be reduced further.

## Overhead budget

Benchmark results across all fleet tiers (120s, 200K-context high-cardinality
worst case):

| Component | p50 (2.5K ctx) | p95 (50K ctx) | p99 (100K ctx) | High-cardinality (200K ctx) |
|-----------|---------------|---------------|----------------|----------------------------|
| Agent RSS delta | +7.4 MB | +18.8 MB | +25.2 MB | +26.5 MB |
| Agent CPU delta | ~0 | +65 mc | ~0 | ~0 |
| Recorder RSS | 33 MB | 47 MB | 47 MB | 61 MB |
| Recorder CPU | 36 mc | 63 mc | 92 mc | 85 mc |
| Disk (2 min) | 1.6 MB | 28 MB | 81 MB | 80 MB |

Agent-side heap breakdown (from pprof):

| Allocation | What | Scales with |
|---|---|---|
| FlatBuffers builder | ~4-8 MB | Throughput (batch size) |
| contextSet | ~1.5-5.7 MB | Distinct contexts |
| Batcher ring buffers | ~2-5 MB | Config (point/def capacity) |
| Init ring buffers | ~2-4 MB | Config (fixed at startup) |

## Alternatives considered

| Alternative | Why rejected |
|---|---|
| **Shared memory (Iceoryx2)** | Adds deployment complexity. Unix socket is simple, fast enough (<1% CPU), and the Transport interface allows swapping later. |
| **Agent-embedded writer (no sidecar)** | Vortex is a Rust library. CGo bridge adds build complexity and crash risk in the agent process. Separate process isolates failures. |
| **Protobuf instead of FlatBuffers** | Protobuf requires deserialization on the Rust side. FlatBuffers is zero-copy — the sidecar reads fields directly from the buffer. |
| **sync.Map for context tracking** | 120 bytes/entry → 24 MB at 200K contexts. Sharded map is 5.6 MB. |
| **Unified ring buffer (always copy strings)** | Benchmarked: +97 MB RSS from Go GC amplification. The fast path's zero-allocation property is essential. |
| **In-memory HashMap in sidecar** | 50 MB at 200K contexts. Bloom filter + separate context files uses ~120 KB. |
| **Compact encodings (zstd + pco)** | Better compression ratio but heavier on transient memory during serialization. Default BtrBlocks encoders with concurrency=1 provide a better tradeoff. |
