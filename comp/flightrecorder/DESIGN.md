# Flight Recorder — Design Document

## Purpose

The flight recorder mirrors pipeline signals (metrics, logs, trace stats) from
the Datadog Agent to a Rust sidecar process over a Unix socket. The sidecar
writes the data as columnar Parquet files (Snappy-compressed) for offline
analysis and debugging. The entire system is designed as a passive observer: it
must never backpressure the agent's main pipeline, and its resource overhead
must be negligible for the vast majority of the fleet.

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
│ Trace stats writer           │      │                            │
│          │                   │      │  Unix socket listener      │
│   hook callback (zero-alloc  │      │          │                 │
│   fast path for known ctx)   │      │  FlatBuffers decoder       │
│          │                   │      │          │                 │
│   batcher (ring buffers)     │      │  ContextStore (bloom+file) │
│          │                   │      │  MetricsWriter (Parquet)   │
│   FlatBuffers encoder        │      │  LogsWriter (Parquet)      │
│          │                   │      │  TraceStatsWriter (Parquet) │
│   Unix socket transport ─────┼──────┤          │                 │
│          (reconnect loop)    │      │  Parquet files on disk     │
└──────────────────────────────┘      └────────────────────────────┘
```

## Signals captured

| Signal | Source hook | Agent component |
|--------|-----------|-----------------|
| Metrics | `MetricSampleSnapshot` batch from `TimeSampler` | DogStatsD / aggregator |
| Logs | `LogView` from log pipeline | Log agent |
| Trace stats | `TraceStatsView` from stats writer | Trace agent |

## Agent side — key design decisions

### Context-key deduplication

**Problem:** Most metric traffic repeats the same (name, tags) contexts. At
100K samples/s, naively copying name + tags strings on every sample creates
millions of short-lived allocations per second, causing severe GC amplification
in Go. Benchmarks showed this adds **+97 MB RSS** at 200K contexts.

**Solution:** Use the aggregator's pre-computed context key — a 64-bit hash
already available on `MetricSampleSnapshot.ContextKey`. Samples are split into
two categories:

- **Context definition** (~0.1% of samples): first occurrence of a context key.
  References name + tags strings from the snapshot. Sent with `context_key` +
  full strings.
- **Context reference** (~99.9%): subsequent samples with a known context key.
  Stores only `context_key` + value + timestamp + sample_rate. **Zero string
  copies, zero heap allocations.**

The sidecar maintains a mapping from context keys to (name, tags) for resolution.

### Context tracking: lock-free bloom filter

**Problem:** The agent needs a concurrent set to track "which context keys have
already been sent."

**Evolution of the design:**

| Approach | Memory at 200K ctx | Hot path | Issue |
|----------|-------------------|----------|-------|
| `sync.Map` | ~24 MB | Lock-free reads | 120 bytes/entry overhead |
| Sharded map (64 shards) | ~5.6 MB | RLock per shard | Complex eviction logic |
| Bloom filter k=7 (deferred) | 586 KB | 7 atomic loads + channel send | Channel overhead on miss |
| **Bloom filter k=3 (inline)** | **1.2 MB** | **3 atomic loads / 3 CAS** | Current design |

**Current solution:** `contextSet` — a lock-free bloom filter with 9.6M bits
(~1.2 MB) and k=3 hash probes.

- **Hit path (known key, 99.9%):** 3 `atomic.Load` operations (~24 ns). No CAS,
  no locks.
- **Miss path (new key):** 3 `atomic.Load` + 3 `CompareAndSwap` (~26 ns). Bits
  are set inline — no background goroutine needed because 3 CAS is cheap enough.
- **FPR:** 0.01% at 100K contexts (doubled bit array compensates for fewer
  probes).
- **Fixed size:** 1.2 MB regardless of context count. No eviction, no cap check.
- **Trade-off:** Bloom filters don't support deletion. This is fine — context
  definitions that fall out of the ring are simply not re-sent. The sidecar
  handles unknown context_keys gracefully.

### Split ring buffer design

**Problem:** Go pre-allocates the full backing array for slices. A single ring
buffer of `[]capturedMetric` (96 bytes/item with name, tags, value, timestamp)
wastes memory because 99.9% of entries only need the compact 48-byte reference
fields.

**Why not a unified ring:** We benchmarked a unified ring where every entry
carries full name + tags strings. At 200K contexts with high throughput, the
per-sample string copying caused +97 MB RSS from Go GC amplification — 4x worse
than the split design. The fast path's zero-allocation property is critical.

**Solution:** Two separate ring buffers:

| Buffer | Item type | Size/item | Default cap | Purpose |
|--------|-----------|-----------|-------------|---------|
| Point buffer | `metricPoint` | 48 bytes | 10,000 | Compact context references (99.9% of traffic) |
| Definition buffer | `contextDef` | ~96 bytes + strings | 1,000 | First-occurrence context definitions |

Memory: 10K × 48B × 2 (double-buffer) = **~1 MB** for the point buffer.
The definition buffer is small (1K items) because new contexts arrive rarely
after warm-up.

Both buffers use a double-buffer swap pattern: the writer fills the "active"
buffer while the flusher drains the "drain" buffer. A single mutex guards the
swap. This avoids lock contention between the hot-path hook callbacks and the
flush goroutine.

**Adaptive flushing:** In addition to the fixed-interval ticker, an early flush
is triggered when any ring buffer exceeds 80% capacity. This eliminates drops
at extreme throughput without increasing baseline RSS.

### FlatBuffers encoding

**Why FlatBuffers:** Zero-copy deserialization on the Rust side. The Go encoder
builds the buffer, sends it over the Unix socket, and the Rust sidecar can read
fields without parsing. This minimizes sidecar CPU.

**Optimizations applied:**

- **Builder pool** with capacity cap: builders exceeding 256 KB backing capacity
  are discarded instead of returned to the pool. Prevents a single large batch
  from inflating the pool permanently.
- **Shared strings:** `CreateSharedString` deduplicates hostname, status, source,
  and service strings within each batch.
- **Pooled offset arrays:** FlatBuffers offset vectors are recycled via a
  separate `sync.Pool`, eliminating per-batch allocations.
- **Batch size cap** (2000 points/frame): keeps the builder under the pool cap
  even at 100K samples/s. At high throughput, multiple smaller frames are sent
  per flush instead of one large one.
- **Zero-copy framing:** `Finish()` (not `FinishSizePrefixed`) + transport-layer
  `net.Buffers` (writev) prepends the 4-byte length prefix without copying the
  encoded bytes.

### Transport: Unix socket with auto-reconnect

The agent writes length-prefixed FlatBuffers frames to a Unix domain socket.
A background goroutine handles reconnection with exponential backoff (100ms →
30s). On reconnect, the bloom filter is reset (the sidecar lost all state), so
all context definitions will be re-sent.

`Send()` uses `net.Buffers` (writev) to prepend the 4-byte LE length prefix
without copying the encoded bytes. If the socket is down, it returns an error
immediately. The batcher increments a counter and drops the batch.

### Hook subscription

The flight recorder subscribes to typed hooks from the DogStatsD pipeline,
log pipeline, and trace stats writer:

- **Metrics:** `Hook[[]MetricSampleSnapshot]` with `WithRecycle` — batches are
  delivered as value-type snapshots and recycled via a pool.
- **Logs:** `Hook[LogView]` — individual log entries.
- **Trace stats:** `Hook[TraceStatsView]` — trace statistics buckets.

Default hook buffer size is 128. The `WithRecycle` option on the metrics hook
eliminates per-batch allocations by reusing snapshot slices from a pool.

## Sidecar side — key design decisions

### Parquet storage (replaced Vortex)

**Why the migration:** Vortex files required complex write strategies, custom
compression tuning, and session management to control RSS. Parquet with Snappy
compression achieved **52% disk reduction** and **92% RSS reduction** compared
to Vortex, with simpler code.

**File rotation:** A single Parquet file is kept open per signal type, with a
new row group flushed every 60 seconds. This avoids the filesystem overhead of
creating 1000+ files/hour (which caused CPU growth over time with per-flush
files) while still allowing concurrent reads of completed row groups.

File layout:
```
/data/signals/
  contexts.bin              # append-only context_key → name, tags (binary)
  metrics-{timestamp}.parquet  # context_key, value, timestamp_ns, sample_rate, source
  logs-{timestamp}.parquet     # content, status, tags, hostname, timestamp_ns, source
  trace_stats-{timestamp}.parquet  # 18 columns: service, name, resource, hits, errors, etc.
```

### Context externalization (ContextStore)

Context definitions are stored in a separate append-only `contexts.bin` binary
file. Metrics Parquet files store only `context_key` (u64) — no name or tags
columns, saving significant disk space.

A bloom filter (~120 KB) deduplicates context writes. False positives just mean
we skip writing a redundant entry — safe because the same context_key always
maps to the same (name, tags). The filter is cleared on new agent connection.

The `hydrate` subcommand joins `contexts.bin` + Parquet files into single
queryable Parquet files with inline name/tags columns.

### Memory optimization: jemalloc + DogStatsD telemetry

**jemalloc** as global allocator — returns pages to the OS aggressively,
preventing the RSS growth seen with glibc's malloc fragmentation.

The sidecar exports telemetry via DogStatsD (using the `cadence` crate),
reporting memory usage, flush timing, connection state, and per-writer row
counts to the agent's DogStatsD server.

## Overhead budget

Benchmark results (300s runs, high-cardinality 200K contexts / high-throughput
25K contexts at 100K DSD/s):

| Metric | High-cardinality (200K ctx) | High-throughput (25K ctx) |
|--------|----------------------------|--------------------------|
| Agent RSS mean delta | +65 MB (+20%) | +22 MB (+10%) |
| Agent CPU mean delta | ~0 mc | +36 mc |
| Hook drops | ~1,000 | 0 |
| Recorder RSS mean | 45 MB | 50 MB |
| Recorder CPU mean | 25 mc | 32 mc |

Agent-side heap breakdown (from pprof, high-cardinality):

| Allocation | Size | Scales with |
|---|---|---|
| Bloom filter | 1.2 MB | Fixed |
| Batcher ring buffers | ~4.5 MB | Config (point/def/log capacity) |
| FlatBuffers builder pool | ~2 MB | Throughput (batch size) |
| Init/config/pools | ~2 MB | Fixed |
| **sync.Map trie growth** (indirect) | ~20 MB | Root cause unknown — appears only with recorder, not allocated by recorder code |

The largest single contributor to the RSS delta is not the flight recorder's own
allocations (~10 MB) but the growth of the DogStatsD string interner's
`sync.Map` internal trie nodes when additional goroutines (hook callbacks)
access it concurrently. This is a Go runtime effect, not directly controllable.

## Environment variables

All configuration uses the `DD_FLIGHTRECORDER_` prefix:

| Variable | Default | Description |
|----------|---------|-------------|
| `DD_FLIGHTRECORDER_SOCKET_PATH` | `/var/run/flightrecorder/pipeline.sock` | Unix socket path (activates automatically when socket is present) |
| `DD_FLIGHTRECORDER_DATA_DIR` | `/data/signals` | Sidecar output directory |
| `DD_FLIGHTRECORDER_MAX_DISK_MB` | `5000` | Max disk usage before janitor cleanup |
| `DD_FLIGHTRECORDER_RETENTION_HOURS` | `4` | File retention period |

## Alternatives considered

| Alternative | Why rejected |
|---|---|
| **Shared memory (Iceoryx2)** | Adds deployment complexity. Unix socket is simple, fast enough (<1% CPU), and the Transport interface allows swapping later. |
| **Agent-embedded writer (no sidecar)** | Vortex/Parquet are Rust libraries. CGo bridge adds build complexity and crash risk in the agent process. Separate process isolates failures. |
| **Protobuf instead of FlatBuffers** | Protobuf requires deserialization on the Rust side. FlatBuffers is zero-copy — the sidecar reads fields directly from the buffer. |
| **sync.Map for context tracking** | 120 bytes/entry → 24 MB at 200K contexts. Bloom filter is 1.2 MB fixed. |
| **Sharded map for context tracking** | 16 bytes/entry, but requires eviction logic and RWMutex per shard. Bloom filter is simpler and faster. |
| **Unified ring buffer (always copy strings)** | Benchmarked: +97 MB RSS from Go GC amplification. The fast path's zero-allocation property is essential. |
| **In-memory HashMap in sidecar** | 50 MB at 200K contexts. Bloom filter + separate context files uses ~120 KB. |
| **Vortex columnar storage** | Higher RSS and disk usage. Parquet + Snappy achieved 52% disk and 92% RSS reduction. |
| **FNV-1a context key computation** | Replaced by reusing the aggregator's pre-computed context key. Saves ~320ns/sample on the hot path. |
| **Slab-copy of strings in contextDef** | Benchmarked: no RSS improvement. The sync.Map growth is caused by concurrent access patterns, not string reference retention. |
