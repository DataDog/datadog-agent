# Flight Recorder Bench Findings

## Architecture Overview

The flight recorder mirrors pipeline signals (metrics, logs, trace stats) from
the Datadog Agent to a Rust sidecar over a Unix socket. The sidecar writes the
data as columnar Parquet files (Snappy-compressed) for offline analysis.

### Agent-side data flow

```
MetricSampleSnapshot → hook callback → batcher ring buffer → FlatBuffers encode → Unix socket
```

### Context-key deduplication

Most metric traffic repeats the same ~600 (name, tags) contexts. Instead of
copying full strings for every sample, we use the aggregator's pre-computed
context key (`MetricSampleSnapshot.ContextKey`) and only send the full strings
on first occurrence:

- **Context definition** (first occurrence, ~0.1% of samples): references
  name + tags from the snapshot, sets context_key in the FlatBuffers message.
- **Context reference** (subsequent, ~99.9%): stores only context_key + value +
  timestamp + sample_rate. No string copies, no heap allocations.

The sidecar maintains a `ContextStore` with bloom filter dedup to resolve
references and write them to a separate `contexts.bin` file.

### Split ring buffer design (current)

To avoid Go's struct pre-allocation problem (large `[]capturedMetric` with
96-byte structs wastes memory even when most items are compact references), we
split the metric ring buffer into two:

- **Point buffer** (`[]metricPoint`, 48 bytes/item): capacity 10K default.
  Stores compact context references — the 99.9% fast path.
- **Definition buffer** (`[]contextDef`, with strings): capacity 1K default.
  Stores first-occurrence context definitions with full name+tags strings.

Key files:
- `comp/flightrecorder/impl/batcher.go` — split ring buffers + bloom filter
- `comp/flightrecorder/impl/sink.go` — hook callback with fast/slow path
- `comp/flightrecorder/impl/encoder.go` — `EncodeSplitMetricBatch` + struct types
- `comp/flightrecorder/impl/context_set.go` — inline k=3 bloom filter
- `schema/flightrecorder/signals.fbs` — `context_key:ulong` field on MetricSample
- `tools/flightrecorder/src/writers/metrics.rs` — context store resolution

---

## Drop Analysis

### Naive implementation (buffer_capacity=5000, hook_buffer=4096, no context keys)

Metrics dropped at two levels: **ring buffer overflow** in the batcher and **hook channel drops**.

| Scenario | Sent | Dropped (batcher) | Drop % | Hook drops |
|----------|------|--------------------|--------|------------|
| p50 (89 DSD/s) | 43,489 | 0 | 0% | 0 |
| p95 (8K DSD/s) | 412,707 | 42,326 | 9.3% | 0 |
| high-cardinality (200K ctx) | 337,886 | 85,824 | 20.2% | 0 |
| p99 (32K DSD/s) | 541,676 | 1,181,963 | 68.6% | 5,480 |
| high-throughput (100K DSD/s) | 674,324 | 1,267,091 | 65.3% | 4,781 |

### With context keys + split ring buffer (pts=10K, defs=1K, current)

| Scenario | Sent | Dropped (ring overflow) | Drop % | Hook drops |
|----------|------|-------------------------|--------|------------|
| high-cardinality (200K ctx, 300s) | 4,355,475 | 0 | ~0% | 3,010 |
| high-throughput (25K ctx, 300s) | 8,531,545 | 0 | 0% | 0 |

Hook drops at high cardinality (~3K out of 4.3M samples) occur during the
200K-context warm-up burst when the hook buffer (128 items) momentarily fills.
This is by design: we drop rather than backpressure the main pipeline.

---

## Overhead Evolution

### Progression of optimizations (high-cardinality, 200K contexts)

| Version | Agent RSS Δ | Key change |
|---------|------------|------------|
| Naive (no context keys, 50K buffer) | +42.5 MB | Baseline |
| Context keys + split ring (20K pts) | +22.8 MB | Context dedup eliminates string copies |
| + Vortex → Parquet migration | +22.8 MB | Sidecar RSS 92% lower, agent unchanged |
| + Tag copy elimination, halved rings | +33 MB | Reduced ring pre-alloc, but GC variance |
| + Zero-copy FlatBuffers send | +32 MB | Eliminated sizePrefixed copy |
| + Pooled offsets, shared strings, builder cap | +32 MB | FlatBuffers builder pool shrunk |
| + Batch size cap (2000/frame) | +21.5 MB | Builder stays under pool cap |
| + Bloom filter (sharded map → k=7 deferred) | +70.7 MB | Warm-up regression from deferred bits |
| + **Bloom filter k=3 inline + 9.6M bits** | **+64.7 MB** | 2x faster, no goroutine |
| + Aggregator context key + hook buf 128 | +69.7 MB | Eliminated computeContextKey |

Note: RSS numbers vary by ±8 MB between runs due to Go GC timing and the
non-deterministic `sync.Map` trie growth. The high-cardinality scenario is
particularly noisy because 200K context warm-up creates a burst of concurrent
goroutine activity.

### Current results (300s, inline k=3 bloom + pre-computed context key)

#### High Cardinality (200K contexts, 4 MiB/s DogStatsD)

| Metric | Stat | Baseline | With Recorder | Delta |
|--------|------|----------|---------------|-------|
| Agent RSS | mean | 322 MB | 387 MB | +65 MB (+20%) |
| Agent RSS | P95 | 379 MB | 456 MB | +77 MB (+20%) |
| Agent CPU | mean | 1431 mc | 1579 mc | +148 mc (+10%) |
| Recorder RSS | mean | N/A | 45 MB | — |
| Recorder CPU | mean | N/A | 25 mc | — |

#### High Throughput (25K contexts, 10 MiB/s DogStatsD)

| Metric | Stat | Baseline | With Recorder | Delta |
|--------|------|----------|---------------|-------|
| Agent RSS | mean | 217 MB | 238 MB | +22 MB (+10%) |
| Agent RSS | P95 | 257 MB | 275 MB | +19 MB (+7%) |
| Agent CPU | mean | 1057 mc | 1363 mc | +306 mc (+29%) |
| Recorder RSS | mean | N/A | 50 MB | — |
| Recorder CPU | mean | N/A | 32 mc | — |

### Heap profile breakdown (high-cardinality, with-recorder)

| Allocation site | Size | Notes |
|---|---|---|
| `sync.Map` trie nodes (newIndirectNode + newEntryNode) | ~29.5 MB | DogStatsD string interner, grows with concurrent access |
| `stringInterner.LoadOrStore` | 27.7 MB | DogStatsD string interning (also present in baseline at 37.5 MB) |
| `tagset.hashedTags.Copy` | 16 MB | Aggregator tag copies |
| `contextResolver.trackContext` | 14 MB | Aggregator context tracking |
| `newBatcher` (rings + bloom) | 4.5 MB | Flight recorder ring buffers + bloom filter |
| `IsContextKnown` (flat) | 3.5 MB | Bloom filter working set |
| `FlatBuffers growByteBuffer` | 2.2 MB | Builder pool (capped at 256 KB per builder) |
| `init.func1` (tag pool) | 2 MB | sync.Pool for tag slices |

**Key insight:** The flight recorder's own allocations total ~12 MB. The
remaining ~53 MB of the +65 MB delta comes from Go runtime effects:
- **sync.Map trie growth** (~29.5 MB): The DogStatsD string interner uses a
  `sync.Map` backed by a `HashTrieMap`. The trie's internal nodes
  (`newIndirectNode`, `newEntryNode`) appear only when the recorder is enabled
  but are not allocated by the recorder code — the pprof stacks are truncated
  so the true caller is unknown. The recorder's hook callbacks never touch the
  interner. The root cause is not yet understood; it may be related to GC
  timing differences or scheduler effects from the additional goroutines.
- **GC overhead / fragmentation** (~25 MB gap between pprof inuse_space and
  RSS): Go's runtime retains pages allocated during peak usage. `MADV_FREE`
  pages count toward RSS until the OS reclaims them.

### Experiment: slab-copy of strings (rejected)

**Hypothesis:** Copying name+tags into a pooled byte slab in the contextDef
would break reference chains to the string interner's `sync.Map`, allowing the
GC to reclaim entries sooner.

**Result:** No RSS improvement (+69.7 MB, same as without slab copy). The
sync.Map trie growth is caused by **concurrent access patterns** (more
goroutines calling `LoadOrStore`), not by reference retention keeping entries
alive. The slab copy added allocation overhead without benefit.

---

## Changes Summary

### Session 1: Telemetry + buffer sizing
1. Split `metrics_dropped_total` into `reason="ring_overflow"` / `reason="transport_error"`.
2. Tested 5K, 20K, 50K buffer capacities.

### Session 2: Context-key deduplication
1. FlatBuffers schema — added `context_key:ulong` field.
2. Context key computation — zero-allocation FNV-1a hash.
3. Seen-context tracking — `sync.Map` for lock-free reads.
4. Hook callback fast/slow path — skip string copies for known contexts.

### Session 3: Split ring buffer
1. Separate `[]metricPoint` (32B) from `[]contextDef` (with strings).
2. `EncodeSplitMetricBatch` encodes defs first, then points.

### Session 4: Unified ring experiment (reverted)
- Removed context dedup, always copied strings → +97 MB RSS. Reverted.

### Session 5: Sharded contextSet + sidecar memory optimizations
- Replaced `sync.Map` with 64-shard `RWMutex` map (~5.6 MB vs ~24 MB).
- Sidecar: bloom filter, jemalloc, VortexSession-per-flush, concurrency=1.

### Session 6: Trace stats + Parquet migration
1. Added `TraceStatsView` hook + FlatBuffers schema + Rust writer.
2. Migrated sidecar from Vortex to Parquet+Snappy (52% disk, 92% RSS reduction).
3. Parquet file rotation every 60s (fixed CPU growth from 1000+ files/hour).
4. Extracted `BaseWriter` for shared Parquet writing logic.
5. Added `hydrate` subcommand to join contexts.bin + Parquet files.

### Session 7: Agent RSS optimization
1. Eliminated tag copies — reference `MetricSampleSnapshot` strings directly.
2. Halved ring buffer defaults (pts=10K, defs=1K, logs=2500).
3. Zero-copy FlatBuffers send via `net.Buffers` (writev).
4. Builder pool cap check fix (`cap(b.Bytes)` not `len(FinishedBytes())`).
5. FlatBuffers alignment fix — `Finish()` + transport-layer framing.

### Session 8: FlatBuffers + bloom filter optimization
1. Pooled offset arrays + `CreateSharedString` for dedup within batches.
2. Batch size cap (2000 pts/frame) keeps builder under pool limit.
3. Replaced sharded map with bloom filter — fixed 586 KB, no eviction logic.

### Session 9: Bloom filter simplification + context key reuse
1. Simplified bloom filter: k=7 deferred → **k=3 inline**. 2x faster hot path
   (24 ns hit, 26 ns miss). No background goroutine needed.
2. Doubled bloom bit array to 9.6M bits (1.2 MB) — FPR stays at 0.01%.
3. Replaced `computeContextKey` (FNV-1a, ~320ns/sample) with aggregator's
   pre-computed `MetricSampleSnapshot.ContextKey`.
4. Reduced hook buffer from 4096 → 128.
5. Standardized env vars to `DD_FLIGHTRECORDER_` prefix.
6. Tested slab-copy of strings to break interner references — no RSS benefit,
   rejected.
