# PRD: DogStatsD-to-Forwarder Pipeline Memory Optimisations

## Introduction

The full DogStatsD metric pipeline — from UDP/UDS reception through aggregation, serialisation, and HTTP forwarding — suffers from excessive memory allocation pressure across several hot paths. Under high-throughput conditions (tens of thousands of metrics per second), repeated allocation of short-lived objects (tag slices, Serie/Point structs, serialisation buffers, forwarder transaction objects) creates significant GC pressure that degrades CPU efficiency, increases p99 latency, and reduces overall throughput.

This PRD defines a set of targeted, moderate-refactoring optimisations to reduce `allocs/op` across the entire pipeline. Work is structured as data-driven iterations: profiling comes first, then hotspot-targeted optimisations, validated by benchmarks throughout.

The pipeline in scope:
```
DogStatsD Listener → Packet Parser → Demultiplexer → TimeSampler
    → Aggregation Flush → Serialiser → DefaultForwarder → Intake API
```

---

## Goals

- Establish a reproducible CPU and memory allocation baseline via pprof profiling
- Reduce `allocs/op` in the aggregator hot path (sample + flush) by ≥30%
- Reduce `allocs/op` in the serialisation path by ≥20%
- Reduce `allocs/op` in the forwarder transaction path by ≥15%
- Ensure all optimisations are validated by existing or new benchmarks
- Introduce no regressions in correctness or existing benchmark throughput

---

## User Stories

### US-001: Establish CPU and Memory Allocation Baseline
**Description:** As a developer, I want a reproducible profiling baseline for the full pipeline so that I can measure the impact of every subsequent optimisation against real data.

**Acceptance Criteria:**
- [ ] A benchmark or integration harness exists that exercises the full pipeline end-to-end at realistic load (≥10K metrics/sec, mix of gauge/counter/histogram/distribution)
- [ ] `go test -bench=. -memprofile=mem.out -cpuprofile=cpu.out` produces valid pprof profiles for: `comp/dogstatsd/server`, `pkg/aggregator`, `pkg/serializer`, `comp/forwarder/defaultforwarder`
- [ ] A `scripts/profile_pipeline.sh` script documents the exact commands to reproduce the baseline profiles
- [ ] Top 10 allocation sites by cumulative bytes are documented in `plans/profiling-baseline.md`
- [ ] Top 10 CPU hotspots are documented in `plans/profiling-baseline.md`

---

### US-002: Pool MetricSample Structs Across the Parsing→Aggregator Boundary
**Description:** As a developer, I want `MetricSample` structs to be pooled and reused rather than heap-allocated per metric so that GC pressure in the high-frequency parsing path is reduced.

**Background:** `MetricSample` (pkg/metrics/metric_sample.go:95-109) is a 200+ byte struct allocated for every incoming metric. At 50K metrics/sec this is ~10MB/sec of short-lived heap churn. The existing `MetricSamplePool` (metric_sample_pool.go) pools the *batch slice* but not the individual structs.

**Acceptance Criteria:**
- [ ] `MetricSample` structs are obtained from a `sync.Pool` in the parsing worker (comp/dogstatsd/server/server_worker.go)
- [ ] Structs are returned to the pool after the TimeSampler has consumed them (after `sample()` completes)
- [ ] Tags (`[]string`) field reuse is handled safely — tags are copied or interned before the struct is returned to pool
- [ ] `BenchmarkTimeSamplerSampleTagCount` reports ≥20% reduction in `allocs/op` vs baseline
- [ ] No data race detected under `go test -race`

---

### US-003: Reduce Tag Slice Allocations in Context Key Generation
**Description:** As a developer, I want tag slice handling in `contextResolver.trackContext()` to allocate fewer intermediate slices so that context key generation is cheaper per sample.

**Background:** `contextResolver.trackContext()` (context_resolver.go:140+) calls `keyGenerator.GenerateWithTags2(name, host, taggerBuffer, metricBuffer)` which internally creates sorted tag copies for hashing. At high cardinality this creates many short-lived `[]string` allocations per sample call.

**Acceptance Criteria:**
- [ ] `HashingTagsAccumulator` reuses a pre-allocated backing buffer per worker goroutine (using `sync.Pool` or per-worker state), avoiding a fresh allocation per `trackContext()` call
- [ ] The sort used in key generation operates on the in-place buffer, not a copy, where possible
- [ ] `BenchmarkContextResolver1000` reports ≥25% reduction in `allocs/op` vs baseline
- [ ] `BenchmarkTimeSamplerSampleHighCardinality` (100K contexts) shows no regression in ns/op

---

### US-004: Pool Serie and Point Slices in TimeSampler Flush
**Description:** As a developer, I want `Serie` structs and their `[]Point` backing arrays to be pooled across flush cycles so that the flush path allocates less per cycle.

**Background:** `TimeSampler.flush()` (time_sampler.go:190+) iterates `metricsByTimestamp` and calls `Metric.flush()` for each context, which creates a new `Serie` and allocates a `[]Point` slice. For 10K active contexts flushing every 15 seconds, this is ~10K Serie + ~10K Point slice allocations per flush.

**Acceptance Criteria:**
- [ ] A `SeriePool` (`sync.Pool`) is introduced in `pkg/metrics` or `pkg/aggregator` to recycle `Serie` structs
- [ ] Point slices are pre-allocated to the expected capacity (number of flush buckets in the Serie's window) to avoid append-driven growth
- [ ] Pooled `Serie` structs are reset (zeroed) before reuse to prevent stale field leakage
- [ ] `BenchmarkTimeSamplerFlushCardinality` (10K contexts) reports ≥30% reduction in `allocs/op` vs baseline
- [ ] `BenchmarkTimeSamplerFlushMetricTypes` shows no regression

---

### US-005: Reduce Allocation in SketchMap Insertion and Flush
**Description:** As a developer, I want sketch (distribution metric) insertion and flush to avoid creating intermediate map entries and slice copies so that distribution metrics have lower allocation cost.

**Background:** `sketchMap` (sketch_map.go:16) is `map[int64]map[ckey.ContextKey]*quantile.Agent` — a two-level map. Each new (timestamp, context) pair allocates a new inner map entry. On flush, `flushBefore()` creates a `SketchSeries` with a `[]SketchPoint` per context.

**Acceptance Criteria:**
- [ ] Inner map initialisation in `sketchMap` uses a pre-sized `make(map[...], hint)` based on previous flush cardinality (tracked as a rolling average) to reduce rehashing
- [ ] `SketchPoint` slices are pre-allocated to expected capacity (number of timestamp buckets)
- [ ] `BenchmarkTimeSamplerFlushMetricTypes` for distribution type reports ≥20% reduction in `allocs/op` vs baseline
- [ ] No correctness regression in sketch quantile accuracy (existing accuracy tests pass)

---

### US-006: Reduce Buffer Allocations in Serialisation Streaming Path
**Description:** As a developer, I want the `IterableSeries` and `IterableSketches` streaming paths to reuse encode buffers between payload chunks so that serialisation allocates fewer intermediate byte slices.

**Background:** `Serializer.SendIterableSeries()` splits series into size-limited payloads. Each chunk triggers a new `bytes.Buffer` or byte slice allocation for JSON/protobuf encoding. With 4000-item buffers (`aggregator_flush_metrics_and_serialize_in_parallel_buffer_size`), this can be dozens of allocations per flush.

**Acceptance Criteria:**
- [ ] Encoding buffers in the serialiser are obtained from a `sync.Pool` and returned after each payload is submitted to the forwarder
- [ ] The JSON and protobuf encoding paths in `pkg/serializer/internal/metrics/` use the pooled buffer rather than allocating fresh per chunk
- [ ] `pkg/serializer` benchmark (`json_payload_builder_throughput_benchmark_test.go`) reports ≥20% reduction in `allocs/op` vs baseline
- [ ] Payload content is identical to pre-optimisation output (validated by existing serialiser correctness tests)

---

### US-007: Pool Forwarder Transaction Objects
**Description:** As a developer, I want HTTP transaction objects in the forwarder to be pooled and reused so that high-frequency metric submissions allocate fewer objects on the heap.

**Background:** Each call to `DefaultForwarder.SubmitV1Series()` / `SubmitSketch()` creates a new `transaction.HTTPTransaction` struct containing URL, payload, headers, and retry state. At 15-second flush intervals with many domains, these allocations add up and generate GC work between flushes.

**Acceptance Criteria:**
- [ ] `transaction.HTTPTransaction` structs are obtained from a `sync.Pool` in `comp/forwarder/defaultforwarder`
- [ ] Transaction structs are reset before reuse (headers cleared, payload nilled) to prevent accidental data leakage
- [ ] Transactions are returned to the pool after the HTTP response is processed (success or terminal failure)
- [ ] `forwarder_bench_test.go` reports ≥15% reduction in `allocs/op` vs baseline
- [ ] Retry logic correctly handles pooled transactions (no double-return to pool on retry)

---

### US-008: Validate Cumulative Allocation Reduction Against Baseline
**Description:** As a developer, I want a final benchmark comparison between the optimised pipeline and the baseline profiles to confirm that the cumulative `allocs/op` reduction meets targets and no regressions have been introduced.

**Acceptance Criteria:**
- [ ] Re-run the baseline profiling script from US-001 against the optimised code
- [ ] Total `allocs/op` across the full pipeline benchmark is ≥25% lower than baseline
- [ ] CPU time per flush cycle (from `BenchmarkTimeSamplerSampleThenFlushCycle`) is not worse than baseline
- [ ] All existing benchmark tests pass with no regression >5% in throughput or ns/op
- [ ] Updated profile summary documented in `plans/profiling-baseline.md` alongside baseline for comparison
- [ ] `go test -race ./comp/dogstatsd/... ./pkg/aggregator/... ./pkg/serializer/... ./comp/forwarder/...` passes

---

## Functional Requirements

- FR-1: Introduce `sync.Pool`-based recycling for `MetricSample` structs in the parsing worker
- FR-2: Introduce per-worker or pooled `HashingTagsAccumulator` buffers in `contextResolver.trackContext()`
- FR-3: Introduce `SeriePool` in the flush path for `Serie` struct and `Points` slice reuse
- FR-4: Pre-size inner maps in `sketchMap` using rolling cardinality estimates
- FR-5: Pool serialisation encode buffers (`bytes.Buffer` / byte slices) via `sync.Pool` in `pkg/serializer`
- FR-6: Pool `transaction.HTTPTransaction` objects in `comp/forwarder/defaultforwarder`
- FR-7: All pooled structs must be fully reset before reuse
- FR-8: All optimisations must be guarded by or validated with benchmarks that report `allocs/op`

---

## Non-Goals

- No changes to the DogStatsD wire protocol or packet parsing logic (that was covered in the prior parsing optimisation PRD)
- No architectural changes to introduce lock-free queues or zero-copy memory-mapped buffers
- No changes to the quantile sketch algorithm or DDSketch implementation
- No changes to the forwarder retry policy, transaction timeout, or domain routing logic
- No changes to the HTTP client or TLS configuration
- No changes to config parameter defaults or configuration schema
- No changes to the no-aggregation (passthrough) pipeline

---

## Technical Considerations

- **Pool safety**: `sync.Pool` objects can be collected by GC between GC cycles. All code that obtains from a pool must assume it may receive a zero-value and initialise accordingly.
- **Concurrency**: The TimeSampler runs in a single goroutine per worker, so per-worker state (e.g. `HashingTagsAccumulator`) is safe without locks.
- **Tag ownership**: When pooling `MetricSample` structs, the `Tags []string` slice must be fully consumed (interned or copied) before the struct is returned to the pool. The string interner in `comp/dogstatsd/server/intern.go` already handles tag string reuse — pooling must not break this.
- **Flush pipeline**: `IterableSeries` uses a buffered channel. Pooled encode buffers must not be returned to the pool while a channel consumer still holds a reference to the encoded bytes.
- **Benchmark tooling**: Use `benchstat` for comparing before/after results. Use `go tool pprof -alloc_objects` for allocation profiling (not just `-alloc_space`) to count object counts, not just bytes.
- **Key files**:
  - `comp/dogstatsd/server/server_worker.go` — parsing worker, MetricSample creation
  - `pkg/aggregator/context_resolver.go` — `trackContext()` hot path
  - `pkg/aggregator/time_sampler.go` — `sample()` and `flush()` hot paths
  - `pkg/aggregator/sketch_map.go` — distribution sketch storage
  - `pkg/metrics/series.go` — `Serie` struct and `Point` type
  - `pkg/serializer/serializer.go` — `SendIterableSeries()` / `SendSketch()`
  - `pkg/serializer/internal/metrics/` — JSON/proto encoding
  - `comp/forwarder/defaultforwarder/default_forwarder.go` — transaction submission

---

## Success Metrics

- `BenchmarkTimeSamplerSampleTagCount` `allocs/op`: ≥20% reduction
- `BenchmarkContextResolver1000` `allocs/op`: ≥25% reduction
- `BenchmarkTimeSamplerFlushCardinality` (10K contexts) `allocs/op`: ≥30% reduction
- `BenchmarkTimeSamplerFlushMetricTypes` (distribution) `allocs/op`: ≥20% reduction
- `json_payload_builder_throughput_benchmark_test.go` `allocs/op`: ≥20% reduction
- `forwarder_bench_test.go` `allocs/op`: ≥15% reduction
- Full-pipeline benchmark cumulative `allocs/op`: ≥25% reduction
- Zero regressions in existing benchmark throughput (ns/op within 5% of baseline)
- All `go test -race` checks pass

---

## Open Questions

1. Should pooled `MetricSample` structs have a fixed `Tags` backing array (pre-allocated to e.g. 32 tags) to avoid reallocation when tags are appended, or is the string interner sufficient?
2. Is there value in replacing the two-level `sketchMap` (`map[int64]map[ContextKey]*Agent`) with a flat `map[sketchKey]*Agent` (where `sketchKey = struct{ts int64; ctx ContextKey}`) to reduce inner-map allocation overhead?
3. For the forwarder transaction pool, should transactions be returned on HTTP 4xx (terminal, no retry) but kept alive on 5xx (retry path)? Need to verify retry state ownership.
4. Are there any downstream consumers of `Serie` or `SketchSeries` that hold references after serialisation completes, which would prevent safe pool return?
