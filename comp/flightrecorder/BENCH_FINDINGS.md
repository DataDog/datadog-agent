# Flight Recorder Bench Findings — 2026-03-13

## Architecture Overview

The flight recorder mirrors pipeline signals (metrics, logs) from the Datadog Agent
to a Rust sidecar over a Unix socket. The sidecar writes the data as columnar Vortex
files for offline analysis.

### Agent-side data flow

```
MetricSample → hook callback → batcher ring buffer → FlatBuffers encode → Unix socket
```

### Context-key deduplication

Most metric traffic repeats the same ~600 (name, tags) contexts. Instead of copying
full strings for every sample, we compute a 64-bit FNV-1a hash of (name, tags) —
the **context key** — and only send the full strings on first occurrence:

- **Context definition** (first occurrence, ~0.1% of samples): copies name + tags,
  sets context_key in the FlatBuffers message.
- **Context reference** (subsequent, ~99.9%): stores only context_key + value +
  timestamp + sample_rate. No string copies, no heap allocations.

The sidecar maintains a `HashMap<u64, (name, tags)>` to resolve references.

### Split ring buffer design (current)

To avoid Go's struct pre-allocation problem (large `[]capturedMetric` with 96-byte
structs wastes memory even when most items are compact references), we split the
metric ring buffer into two:

- **Point buffer** (`[]metricPoint`, 32 bytes/item): large capacity (20K default).
  Stores compact context references — the 99.9% fast path.
- **Definition buffer** (`[]contextDef`, with strings): small capacity (2K default).
  Stores first-occurrence context definitions with full name+tags strings.

Memory savings: 20K points × 32 bytes × 2 double-buffers = **1.3 MB** vs
20K × 96 bytes × 2 = 3.8 MB (old single-buffer design).

Key files:
- `comp/flightrecorder/impl/context_key.go` — zero-alloc FNV-1a hash
- `comp/flightrecorder/impl/batcher.go` — split ring buffers + `seenContexts`
- `comp/flightrecorder/impl/sink.go` — hook callback with fast/slow path
- `comp/flightrecorder/impl/encoder.go` — `EncodeSplitMetricBatch` + struct types
- `schema/flightrecorder/signals.fbs` — `context_key:ulong` field on MetricSample
- `tools/flightrecorder/src/writers/metrics.rs` — context map resolution

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

Root cause: ring buffer (5000) saturates at ~50K+ DSD samples/s (>5K items per 100ms tick).

### Large buffers (buffer_capacity=50000, hook_buffer=16384, no context keys)

**Zero drops across all scenarios.** All metrics sent successfully.

| Scenario | Sent | Dropped | Drop % | Hook drops |
|----------|------|---------|--------|------------|
| p50 (89 DSD/s) | 43,489 | 0 | 0% | 0 |
| p95 (8K DSD/s) | 435,151 | 0 | 0% | 0 |
| high-cardinality (200K ctx) | 527,793 | 0 | 0% | 0 |
| p99 (32K DSD/s) | 1,934,667 | 0 | 0% | 0 |
| high-throughput (100K DSD/s) | 1,920,757 | 0 | 0% | 0 |

### With context keys + 20K single-buffer

| Scenario | Sent | Dropped (ring overflow) | Drop % | Hook drops |
|----------|------|-------------------------|--------|------------|
| p50 | 43,489 | 0 | 0% | 0 |
| p95 | 564,628 | 0 | 0% | 0 |
| high-cardinality | 564,650 | 0 | 0% | 0 |
| p99 | 1,592,566 | 516,810 | 24.5% | 0 |
| high-throughput | 1,889,706 | 307,046 | 14.0% | 0 |

### With context keys + split ring buffer (pts=20K, defs=2K)

| Scenario | Sent | Dropped (ring overflow) | Drop % | Hook drops |
|----------|------|-------------------------|--------|------------|
| p50 | 37,790 | 0 | 0% | 0 |
| p95 | 567,780 | 0 | 0% | 0 |
| high-cardinality | 612,074 | 57 | ~0% | 0 |
| p99 | 1,585,134 | 505,252 | 24.2% | 0 |
| high-throughput | 1,705,404 | 292,263 | 14.6% | 0 |

Drop rates at p99/high-throughput are inherent to the 20K point buffer capacity at
32K-100K DSD/s — these extreme scenarios generate more than 20K points per 100ms
flush interval. This is by design: we drop rather than backpressure the main pipeline.

---

## Overhead Comparison

### Naive implementation (small buffers, no context keys)

| Scenario | RSS mean overhead | CPU P50 overhead | Recorder RSS | Recorder CPU |
|----------|------------------|------------------|-------------|-------------|
| p50 | +5.7 MB (+3.9%) | -4 mc | 28 MB | 29 mc |
| p95 | +19.8 MB (+11.0%) | +32 mc | 45 MB | 49 mc |
| high-cardinality | +13.3 MB (+6.0%) | +30 mc | 52 MB | 52 mc |
| p99 | +27.3 MB (+11.9%) | +81 mc | 44 MB | 49 mc |
| high-throughput | +23.8 MB (+12.3%) | +95 mc | 45 MB | 97 mc |

### Large buffers, no context keys

| Scenario | RSS mean overhead | CPU P50 overhead | Recorder RSS | Recorder CPU |
|----------|------------------|------------------|-------------|-------------|
| p50 | +17.6 MB (+12.0%) | ~0 mc | 29 MB | 45 mc |
| p95 | +45.6 MB (+25.3%) | -1 mc | 48 MB | 81 mc |
| high-cardinality | +42.5 MB (+18.7%) | +10 mc | 69 MB | 70 mc |
| p99 | +115.1 MB (+45.9%) | +69 mc | 90 MB | 90 mc |
| high-throughput | +150.9 MB (+77.3%) | +41 mc | 97 MB | 102 mc |

### With context keys + 20K single-buffer

| Scenario | RSS mean overhead | CPU P50 overhead | Recorder RSS | Recorder CPU |
|----------|------------------|------------------|-------------|-------------|
| p50 | +7.7 MB (+6.0%) | +1 mc | 27 MB | 30 mc |
| p95 | +18.4 MB (+11.0%) | +6 mc | 47 MB | 64 mc |
| high-cardinality | +22.9 MB (+10.8%) | +11 mc | 45 MB | 72 mc |
| p99 | +42.1 MB (+18.5%) | +55 mc | 51 MB | 57 mc |
| high-throughput | +23.8 MB (+13.2%) | +21 mc | 49 MB | 81 mc |

### With context keys + split ring buffer (current)

| Scenario | RSS mean overhead | CPU P50 overhead | Recorder RSS | Recorder CPU |
|----------|------------------|------------------|-------------|-------------|
| p50 | **+1.1 MB (+0.8%)** | -3 mc | 28 MB | 64 mc |
| p95 | +27.5 MB (+17.5%) | +24 mc | 47 MB | 68 mc |
| high-cardinality | +22.8 MB (+11.1%) | +36 mc | 45 MB | 62 mc |
| p99 | +44.9 MB (+20.4%) | +51 mc | 50 MB | 69 mc |
| high-throughput | +18.3 MB (+10.1%) | +10 mc | 49 MB | 66 mc |

### Analysis

The split ring buffer design significantly improves the **median production workload**
(p50): **+1.1 MB (+0.8%)**, down from +7.7 MB with the single 20K buffer.

For higher workloads (p95+), the RSS delta is dominated by Go runtime allocations
from the `sync.Map` seen-contexts tracker and string interning during context
definition encoding, not the ring buffer pre-allocation. The split buffer cannot
help with these.

Key takeaway: **for the ~50% of fleet agents at or below p50 load, the flight
recorder adds ~1 MB of memory overhead. CPU overhead is negligible.**

---

## Changes Summary

### Session 1: Telemetry + buffer sizing
1. **Telemetry granularity** — split `metrics_dropped_total` into `reason="ring_overflow"`
   and `reason="transport_error"` labels; added `batch_size` gauge per type.
2. **Buffer sizing experiments** — tested 5K, 20K, 50K capacities.
3. **Bench report** — updated generator to display granular drop counters.

### Session 2: Context-key deduplication
1. **FlatBuffers schema** — added `context_key:ulong` field to `MetricSample`.
2. **Context key computation** (`context_key.go`) — zero-allocation inline FNV-1a
   hash of (name, tags). Called on every hook callback (~100K/s).
3. **Seen-context tracking** (`batcher.go`) — `atomic.Pointer[sync.Map]` for
   lock-free reads from hook callbacks. Reset on transport reconnect.
4. **Hook callback** (`sink.go`) — fast path skips all string copies for known
   contexts; slow path copies strings for first-occurrence context definitions.
5. **Encoder** (`encoder.go`) — context references skip encoding name/tags/source
   strings entirely, reducing FlatBuffers payload size.
6. **Sidecar** (`metrics.rs`) — maintains `HashMap<u64, ContextDef>` to resolve
   context references. Cleared on new client connection.

### Session 3: Split ring buffer
1. **Split ring buffer** (`batcher.go`) — separate `[]metricPoint` (32 bytes, 20K cap)
   from `[]contextDef` (with strings, 2K cap) to avoid Go struct pre-allocation waste.
2. **Split encoder** (`encoder.go`) — `EncodeSplitMetricBatch` encodes from two ring
   buffers: definitions first (sidecar sees them before references), then data points.
3. **Config** (`config.go`) — replaced single `buffer_capacity` with three separate
   capacities: `point_buffer_capacity` (20K), `def_buffer_capacity` (2K),
   `log_buffer_capacity` (5K).
4. **Sink wiring** (`sink.go`) — updated hook callbacks to use `AddPoint`/`AddContextDef`
   methods and pass separate capacity params to `newBatcher`.

### Session 4: Unified ring experiment (reverted)

**Goal**: Replace the two metric ring buffers + `sync.Map` context dedup with a
single ring of full `capturedMetric` entries (always carrying name+tags). The
hypothesis was that removing `sync.Map` (~24 MB at 200K contexts) and GC
amplification would save ~30-35 MB, outweighing the slightly larger ring entries.

**Changes made** (subsequently reverted):
1. Removed `metricPoint` and `contextDef` types; promoted `capturedMetric` as the
   single ring buffer element.
2. Merged `ptsActive/ptsDrain` + `defsActive/defsDrain` into one pair:
   `metricsActive/metricsDrain []capturedMetric`.
3. Removed `seenContexts sync.Map`, `IsContextKnown`, `ResetContexts`.
4. Replaced `AddPoint` + `AddContextDef` with single `AddMetric`.
5. Added `EncodeMetricBatchRing` (replacing `EncodeSplitMetricBatch`).
6. Hook callback always copies name+tags (no fast/slow path).

**Benchmark result** (dogstatsd-high-cardinality, 200K contexts, 120s):

| Metric | Split-buffer (before) | Unified ring (experiment) |
|--------|----------------------|--------------------------|
| Agent anon RSS mean delta | +22.8 MB | **+97.2 MB** |
| Agent anon RSS P50 delta | — | +112.8 MB |
| Agent anon RSS max delta | — | +130.8 MB |
| Agent CPU P50 delta | +36 mc | +44 mc |
| Metrics sent | 612,074 | 1,318,649 |
| Metrics dropped (overflow) | 57 | 123,952 |

**Why it failed**: Removing the fast path means every sample copies name+tags
strings via `payload.GetName()` and tag slice copy. At 200K contexts with high
throughput (~1.3M metrics/120s), the continuous string allocation pressure creates
far more GC amplification than the `sync.Map` it replaced. The split-buffer design
works because the compact 48-byte `metricPoint` fast path (no strings, no heap
allocations) handles 99.9% of samples after context warm-up, keeping GC pressure
minimal.

**Key insight**: The `sync.Map` costs ~24 MB at 200K contexts, but the alternative
(copying strings on every sample) costs ~75 MB more due to Go GC amplification from
millions of short-lived string allocations per flush interval. The dedup map is the
lesser evil by a wide margin.

### Session 5: Sharded contextSet + sidecar memory optimizations

**Agent side — replaced `sync.Map` with sharded `contextSet`:**
- 64-shard `RWMutex` + native `map[uint64]struct{}` — ~16 bytes/entry vs ~120 bytes.
- At 200K contexts: ~5.6 MB (down from ~24 MB with `sync.Map`).
- Bounded capacity (default 500K) — cleared when exceeded.
- Def ring eviction now removes the context key from the set, so the definition
  gets re-sent on next occurrence.

**Sidecar — context externalization and RSS optimization:**
- Context definitions written to separate `contexts-*.vortex` files. Metrics
  files store only `context_key` (u64) — no name/tags columns.
- Bloom filter (~120 KB) replaces in-memory `HashMap` (~50 MB) for context dedup.
- `tikv-jemallocator` as global allocator (prevents glibc malloc fragmentation).
- Fresh `VortexSession` per flush (prevents Vortex registry accumulation).
- Custom write strategy with compression concurrency=1, 512 KB buffer (reduces
  transient in-flight memory from ~32 MB to ~2 MB on 16-core machines).
- Context flush batches capped at 2K rows (smaller transient pipeline memory).

**Benchmark results (all scenarios, 120s):**

| Scenario | Agent RSS delta | Recorder RSS mean | Recorder RSS max | Disk |
|----------|----------------|-------------------|------------------|------|
| p50 (2.5K ctx) | +7.4 MB (+4.6%) | 33 MB | 41 MB | 1.6 MB |
| p95 (50K ctx) | +18.8 MB (+8.8%) | 47 MB | 58 MB | 28 MB |
| p99 (100K ctx) | +25.2 MB (+8.8%) | 47 MB | 61 MB | 81 MB |
| high-throughput (25K ctx, 100K/s) | +18.1 MB (+8.2%) | 46 MB | 58 MB | 66 MB |
| high-cardinality (200K ctx) | +26.5 MB (+9.0%) | 61 MB | 77 MB | 80 MB |

Agent-side heap breakdown (pprof, high-cardinality):
- FlatBuffers builder: ~7.7 MB (retained backing slice, scales with batch size)
- contextSet: ~5.6 MB (scales with distinct contexts)
- Batcher ring buffers: ~4.5 MB (pre-allocated at init)
- Init/config: ~2.5 MB (fixed)

### Remaining work
- Consider adaptive sampling for extreme throughput (>50K DSD/s).
- Explore shrinking the FlatBuffers builder after large batches (currently
  retains peak allocation via `sync.Pool`).
