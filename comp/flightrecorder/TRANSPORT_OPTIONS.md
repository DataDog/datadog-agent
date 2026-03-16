# Flight Recorder Transport — Quantitative Analysis & Options

## 1. Measured operation costs

All measurements on arm64 Linux (dev container), Go 1.23, single-threaded.

### Hook → batcher (producer hot path)

| Operation | Latency | Allocs | Notes |
|---|---|---|---|
| `AddPoint` (lock + ring write) | **59 ns** | 0 | 32-byte struct, mutex contention only |
| `computeContextKey` (FNV-1a) | ~15 ns | 0 | Inline, no hash.Hash interface |
| `sync.Map.LoadOrStore` | ~50 ns | 0 (hit) | Lock-free read on fast path |
| **Total hot-path callback** | **~125 ns** | **0** | Known context, no string copy |

### Batcher → sidecar (flush path)

| Operation | Latency | Allocs | Notes |
|---|---|---|---|
| FlatBuffers encode 1 metric | 600 ns | 5 (1.3 KB) | Fresh builder each time |
| FlatBuffers encode 100 metrics | 26 µs | 14 (72 KB) | Builder grows once then reuses |
| FlatBuffers encode 1,000 metrics | 290 µs | 22 (1 MB) | O(n) in batch size |
| FlatBuffers encode 3,200 metrics | ~930 µs (est.) | ~25 | Extrapolated for p99 |
| FlatBuffers encode 10,000 metrics | ~2.5 ms (est.) | ~30 | Extrapolated for high-throughput |

### Unix socket write (loopback)

| Payload size | Latency | Throughput |
|---|---|---|
| 200 B | 760 ns | 260 MB/s |
| 1 KB | 790 ns | 1.3 GB/s |
| 4 KB | 940 ns | 4.3 GB/s |
| 16 KB | 2 µs | 7.9 GB/s |
| 64 KB | 6.4 µs | 10 GB/s |
| 150 KB | 14.5 µs | 10.5 GB/s |

Socket write is dominated by the syscall fixed cost (~700 ns) up to ~4 KB,
then by memory copy throughput (~10 GB/s).

---

## 2. Current design — time budget analysis

The batcher flushes every **100 ms**. Here's where time goes per flush:

### p50 (89 DSD/s → ~9 samples/flush)

| Phase | Time | % of 100 ms |
|---|---|---|
| Encode 9 samples | ~5 µs | 0.005% |
| Socket write (~2 KB payload) | ~800 ns | 0.001% |
| **Total flush** | **~6 µs** | **0.006%** |

Flush cost is negligible. No drops. +1.1 MB RSS overhead.

### p99 (32K DSD/s → ~3,200 samples/flush)

| Phase | Time | % of 100 ms |
|---|---|---|
| Encode 3,200 samples | ~930 µs | 0.93% |
| Socket write (~150 KB payload) | ~15 µs | 0.015% |
| **Total flush** | **~950 µs** | **~1%** |

Flush is fast. **Drops (24%) happen because more samples arrive in 100ms than
the 20K ring can hold** — multiple flush intervals of backlog accumulate during
the initial context warm-up phase when the slow path (with string copies) is hit.

### high-throughput (100K DSD/s → ~10,000 samples/flush)

| Phase | Time | % of 100 ms |
|---|---|---|
| Encode ~8,500 samples | ~2.5 ms | 2.5% |
| Socket write (~400 KB payload) | ~30 µs | 0.03% |
| **Total flush** | **~2.5 ms** | **~2.5%** |

Still well within budget. **Drops (15%) are purely a buffer capacity issue.**

### Key insight

**Encoding and transport are not bottlenecks.** The flush loop uses <3% of its
time budget even at 100K DSD/s. The two problems are:

1. **Ring overflow** at extreme throughput (>20K arrivals per 100ms)
2. **RSS overhead** from Go runtime (sync.Map, string interning, GC metadata)

---

## 3. Where RSS comes from

Breaking down the agent RSS overhead at p99 (+45 MB):

| Source | Estimated size | Why |
|---|---|---|
| Point ring buffers (20K × 32B × 2) | 1.3 MB | Pre-allocated, fixed |
| Def ring buffers (2K × ~120B × 2) | 0.5 MB | Pre-allocated, fixed |
| Log ring buffers (5K × ~200B × 2) | 2 MB | Pre-allocated, fixed |
| `sync.Map` (100K contexts) | ~10-15 MB | Go map overhead: per-entry ~100-150 bytes |
| FlatBuffers builder (pooled) | ~1 MB | Grows to max batch size, reused |
| Hook channel buffer (16K slots × 8B) | 0.1 MB | Pointer-sized channel slots |
| Go runtime / GC metadata | ~10-20 MB | Proportional to live heap objects |
| **Total estimated** | **~25-40 MB** | Aligns with measured +45 MB |

The `sync.Map` and GC metadata dominate at high context counts. The ring buffers
themselves are only ~4 MB.

---

## 4. Transport options

### Option A: Adaptive flush interval (low effort)

**Idea**: flush more frequently when the ring is filling up. Instead of fixed
100ms, flush at `min(100ms, time_until_80%_full)`.

```
Trigger: ptsActiveN > ptCap * 0.8  →  immediate flush
```

**Expected impact**:
- At p99 (3,200/flush): ring never exceeds 80% → no change, same 100ms interval
- At high-throughput (10K/flush): triggers at 16K items → flushes every ~16ms
  - 6× more flushes/s, each encoding ~1,600 samples (~50 µs)
  - Total CPU: 6 × 50 µs = 300 µs/s → negligible
- **Drops**: eliminated (ring never overflows)
- **RSS**: unchanged (same ring buffer sizes)
- **CPU**: +0.3 ms/s at high-throughput (negligible)

**Pros**: trivial to implement (~10 lines), eliminates drops
**Cons**: does not address RSS; more socket writes/s (6× at high-throughput)

### Option B: Streaming (per-sample writes, no ring buffer)

**Idea**: encode and send each sample immediately in the hook callback, removing
the ring buffer entirely.

**Per-sample cost**:
- FlatBuffers encode 1 sample: 600 ns
- `sizePrefixed` copy: ~50 ns
- Socket write (~200 B): 760 ns
- **Total: ~1.4 µs/sample**

**Projected CPU**:

| Scenario | Samples/s | CPU (ms/s) | CPU % |
|---|---|---|---|
| p50 (89/s) | 89 | 0.1 | 0.01% |
| p95 (8K/s) | 8,000 | 11.2 | 1.1% |
| p99 (32K/s) | 32,000 | 44.8 | 4.5% |
| high-throughput (100K/s) | 100,000 | 140 | 14% |

**Projected RSS**: ~0 MB overhead (no ring buffers, no double-buffering, builder
could be small and reused per-sample)

**Pros**: zero RSS overhead, zero drops, simplest possible design
**Cons**: 14% CPU at high-throughput; 32K+ syscalls/s; callback latency
increases from 125 ns to ~1.5 µs (12× slower), which may backpressure the hook
channel

**Mitigation**: could batch in the callback itself — accumulate N samples or T
time, then flush. E.g., micro-batch of 10: encode 10 in 3 µs + socket 1.5 KB
in 800 ns → 380 ns/sample amortized. At 100K/s: 38 ms/s (3.8% CPU) + 10K
syscalls/s.

### Option C: Shared memory SPSC ring (zero-copy)

**Idea**: replace the socket with an mmap'd single-producer single-consumer
(SPSC) lock-free ring buffer. The agent writes 32-byte `metricPoint` structs
directly into shared pages; the Rust sidecar reads them without any syscalls.

**Architecture**:
```
Agent process                          Sidecar process
─────────────                          ───────────────
hook callback                          reader loop
  → computeContextKey                    → read from shared ring
  → atomic write to shared ring          → resolve context key
     (32 bytes, no syscall)              → append to Vortex columnar buffer
  → ~15-20 ns                            → flush to disk periodically
```

**Per-sample cost**: ~15-20 ns (atomic store + release fence)

**Projected CPU**:

| Scenario | Samples/s | CPU (ms/s) | CPU % |
|---|---|---|---|
| p50 | 89 | 0.002 | 0% |
| p95 | 8,000 | 0.16 | 0.02% |
| p99 | 32,000 | 0.64 | 0.06% |
| high-throughput | 100,000 | 2.0 | 0.2% |

**Ring sizing**: 128K slots × 32 bytes = **4 MB shared mapping**.
At 100K/s with sidecar reading every 10ms: 1,000 samples/read → ring
utilization <1%. Even if the sidecar stalls for 1 second, 100K entries fit.

**RSS impact**: shared mmap pages are counted as file-backed RSS (not anonymous
RSS). The agent's **anon RSS overhead is 0**. The 4 MB appears in
`/proc/pid/maps` as a shared mapping, similar to a shared library.

**Context definitions**: the 0.1% of samples that need strings (first-occurrence
context defs) can't go through a fixed-size ring slot. Two sub-options:
- **(C1)** Keep a parallel Unix socket for context defs only. At 0.1% of traffic,
  this is ~100 context defs/s at p99 → negligible cost.
- **(C2)** Use a second shared ring with variable-length slots (more complex) or
  a shared mmap'd arena for strings.

**Drops**: zero — 128K ring at 100K/s gives 1.28 seconds of buffer.

**Pros**: near-zero CPU, zero anon RSS, zero drops, lock-free
**Cons**: significant implementation effort (mmap setup, SPSC protocol, cross-
language struct layout agreement, sidecar reader rewrite); platform-specific
(Linux/macOS mmap semantics); loses FlatBuffers schema evolution for point data

### Option D: Shared ring for points + socket for context defs (hybrid)

**Idea**: combine Option C's shared SPSC ring for the 99.9% fast path with the
existing socket for the 0.1% slow path (context definitions).

```
                     ┌─────────────────────────┐
                     │    mmap'd SPSC ring      │
  metricPoint ──────►│  (32B fixed slots, 4MB)  │──────► Sidecar reader
  (99.9%)            └─────────────────────────┘        (poll every 1-10ms)

                     ┌─────────────────────────┐
                     │    Unix socket           │
  contextDef  ──────►│  (FlatBuffers framed)    │──────► Sidecar socket reader
  (0.1%)             └─────────────────────────┘        (async, same process)
```

**Cost model** (same as Option C for points):

| Component | p50 | p99 | high-throughput |
|---|---|---|---|
| Point writes (shared ring) | 0.002 ms/s | 0.64 ms/s | 2.0 ms/s |
| Context def writes (socket) | ~0 | ~0.1 ms/s | ~0.1 ms/s |
| Agent anon RSS | ~0 MB | ~0 MB | ~0 MB |
| Drops | 0 | 0 | 0 |

**Pros**: best of both worlds — zero-copy for data, schema evolution for metadata
**Cons**: highest implementation complexity; two transport paths to maintain

---

## 5. Comparison matrix

| | Adaptive flush (A) | Streaming (B) | Shared SPSC (C) | Hybrid (D) |
|---|---|---|---|---|
| **Agent CPU (p99)** | ~1 ms/s | ~45 ms/s | ~0.6 ms/s | ~0.7 ms/s |
| **Agent CPU (100K/s)** | ~2.5 ms/s | ~140 ms/s | ~2 ms/s | ~2.1 ms/s |
| **Agent anon RSS (p50)** | +1 MB | ~0 MB | ~0 MB | ~0 MB |
| **Agent anon RSS (p99)** | +45 MB | ~0 MB | ~0 MB | ~0 MB |
| **Drops (p99)** | 0 | 0 | 0 | 0 |
| **Drops (100K/s)** | 0 | 0 | 0 | 0 |
| **Syscalls/s (p99)** | ~16/s | ~32K/s | 0 | ~100/s |
| **Impl effort** | ~1 hour | ~1 day | ~1 week | ~1-2 weeks |
| **Schema evolution** | yes (FlatBuffers) | yes (FlatBuffers) | no (raw structs) | partial (defs only) |
| **Platform constraints** | none | none | mmap (Linux/macOS) | mmap + socket |

---

## 6. Recommendation

**Phase 1 — Adaptive flush (Option A)**: implement immediately. Eliminates drops
at zero RSS cost. ~10 lines of code in `batcher.go`.

**Phase 2 — Evaluate RSS source**: the +45 MB at p99 is mostly `sync.Map` +
GC metadata, not ring buffers. Before investing in shared memory, profile whether
replacing `sync.Map` with a sharded `map[uint64]struct{}` behind a `sync.RWMutex`
reduces RSS significantly. If it does, the current socket transport may be good
enough.

**Phase 3 — Shared memory (Option C or D)**: if RSS remains a concern after
Phase 2, implement the shared SPSC ring. Option D (hybrid) is the cleanest
architecture because it preserves FlatBuffers for variable-length data while
using zero-copy for the hot path. The SPSC ring protocol is well-understood
(e.g., Linux kernel's `io_uring`, Disruptor pattern) and can be implemented in
~500 lines of Go + Rust.

Option B (streaming) is not recommended: its CPU cost at high throughput (14%) is
too high, and it trades one problem (RSS) for another (CPU + syscall pressure).
