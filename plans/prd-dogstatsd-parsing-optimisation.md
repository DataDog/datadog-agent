# PRD: DogStatsD Parsing CPU Optimisation

## Introduction

DogStatsD is the hot path for metric ingestion in the Datadog Agent. Under high-throughput workloads,
the parser (`comp/dogstatsd/server/parse.go` and related files) is a significant consumer of CPU.
This PRD describes a focused programme of CPU-reduction work targeting the metric parsing hot path
and tag parsing/interning subsystem, starting with targeted micro-optimisations and escalating to
structural refactors only where benchmarks justify the additional complexity.

**Success target:** ≥20% reduction in CPU time per metric sample as measured by the existing
`go test -bench` suite in `comp/dogstatsd/server/`.

---

## Goals

- Reduce CPU usage during dogstatsd metric parsing by ≥20% (measured via benchmarks)
- Keep all existing fuzz tests and unit tests green
- Introduce no regressions in correctness or throughput
- Leave a reproducible benchmark baseline so future changes can be evaluated

---

## User Stories

### US-001: Establish Benchmark Baseline
**Description:** As a developer, I need a reproducible benchmark baseline so that I can measure
the impact of each subsequent optimisation and know when the 20% target is reached.

**Acceptance Criteria:**
- [ ] Run `go test -bench=. -benchmem -count=5 ./comp/dogstatsd/server/...` and save output to `plans/bench-baseline.txt`
- [ ] Run `go test -bench=. -benchmem -count=5 ./comp/dogstatsd/server/...` with CPU profiling enabled and save `plans/profile-baseline.pprof`
- [ ] Top CPU-consuming functions are identified and documented in `plans/bench-baseline.txt` (appended as a comment)
- [ ] All existing unit tests pass: `go test ./comp/dogstatsd/server/...`

---

### US-002: Replace `bytes.Split` with Index-Based Parsing in `parseTags`
**Description:** As a developer, I want tag parsing to avoid intermediate slice allocations so
that GC pressure and CPU time is reduced on the critical tag parsing path.

**Background:** `parseTags()` in `comp/dogstatsd/server/parse.go` currently calls
`bytes.Split(rawTags, commaSeparator)` which allocates a `[][]byte` header on every call.
At high message rates this is a significant source of short-lived heap allocations that drive
CPU time in the GC. Replacing with a manual `bytes.IndexByte` loop eliminates this allocation.

**Acceptance Criteria:**
- [ ] `parseTags()` no longer calls `bytes.Split`; uses `bytes.IndexByte` (or equivalent) loop instead
- [ ] Produces identical tag output for all existing test cases
- [ ] `BenchmarkExtractTagsMetadata` shows ≥0 ns/op improvement (no regression) and reduced `allocs/op`
- [ ] All unit tests pass: `go test ./comp/dogstatsd/server/...`
- [ ] Fuzz tests compile: `go test -fuzz=. -fuzztime=10s ./comp/dogstatsd/server/` (any fuzz target)

---

### US-003: Optimise Metric Type Detection to Use Byte Switch
**Description:** As a developer, I want metric type detection to use a byte-switch instead of
sequential `bytes.Equal` calls so that the common case (gauge/count) is resolved in O(1) with
no function call overhead.

**Background:** `parseMetricSampleMetricType()` in `comp/dogstatsd/server/parse.go` performs a
series of `bytes.Equal` comparisons (`g`, `c`, `h`, `d`, `s`, `ms`). Since the type symbol is
1–2 bytes, switching on `rawMetricType[0]` (and `len(rawMetricType)`) gives a direct branch
with no slice comparison overhead and no import of `bytes` for this function.

**Acceptance Criteria:**
- [ ] `parseMetricSampleMetricType()` uses a `switch` on `rawMetricType[0]` / `len(rawMetricType)` rather than `bytes.Equal`
- [ ] All six metric types (g, c, h, d, s, ms) and the error path are covered
- [ ] `BenchmarkParseMetric` shows no regression
- [ ] All unit tests pass: `go test ./comp/dogstatsd/server/...`

---

### US-004: Optimise Field-Separator Scanning in `parseMetricSample`
**Description:** As a developer, I want the optional-field parsing loop to use `bytes.IndexByte`
for scanning rather than `bytes.SplitN`/`bytes.Split`, so that we avoid per-field allocations
when processing the `|`-delimited suffix of a statsd packet.

**Background:** After parsing name and value, `parseMetricSample()` iterates over `|`-separated
optional fields (tags, sample rate, timestamp, origin). If this uses `bytes.Split`, it allocates
once per packet. Replacing with a cursor-based `bytes.IndexByte` loop processes fields in-place
without any allocation.

**Acceptance Criteria:**
- [ ] Optional-field iteration in `parseMetricSample()` uses a cursor/index approach rather than `bytes.Split`
- [ ] Handles all field types: `#tags`, `@sample_rate`, `Ttimestamp`, `c:container_id`, `e:external`, `card:cardinality`
- [ ] `BenchmarkParsePackets` and `BenchmarkParsePacketsMultiple` show no regression
- [ ] All unit tests pass: `go test ./comp/dogstatsd/server/...`
- [ ] Fuzz tests compile and run for 10 seconds without panics

---

### US-005: Improve String Interner Eviction Strategy
**Description:** As a developer, I want the string interner to use a generational eviction
strategy instead of a full-cache reset so that hot strings are never evicted while cold strings
are collected, reducing re-interning CPU cost at scale.

**Background:** `stringInterner` in `comp/dogstatsd/server/intern.go` currently evicts the
**entire** cache when `maxSize` is reached. Under high-cardinality workloads this causes all
strings to be re-interned immediately after a reset, wasting CPU. A two-generation (young/old)
strategy keeps `old = young; young = {}` on eviction — hot strings survive one generation and
are promoted, while cold strings are dropped after two generations.

**Acceptance Criteria:**
- [ ] `stringInterner` implements two-generation eviction: on capacity, `old = current; current = new map`
- [ ] `LoadOrStore` checks `current` first, then `old`; if found in `old`, promotes to `current`
- [ ] Total memory is bounded: combined size of both generations ≤ `2 × maxSize` entries
- [ ] Telemetry counters (hits/misses/resets) remain accurate; add `promotions` counter
- [ ] `BenchmarkParseMetric` (1000 tags variant) shows ≤ cpu/op vs baseline (no regression)
- [ ] All unit tests in `intern_test.go` (if present) or new tests covering eviction pass
- [ ] All unit tests pass: `go test ./comp/dogstatsd/server/...`

---

### US-006: Fast-Path Tag Metadata Extraction for Common Tags
**Description:** As a developer, I want `extractTagsMetadata()` in `enrich.go` to exit early
after finding all expected special tags so that we don't scan the entire tag list unnecessarily
on every metric.

**Background:** `extractTagsMetadata()` iterates over all tags on every metric sample looking
for special prefixes (`dd.internal.entity_id`, `host`, etc.). For metrics with many tags, this
is O(n) in tag count even when all special tags appear near the beginning. Adding an early-exit
once all expected metadata fields are found reduces average scan length.

**Acceptance Criteria:**
- [ ] `extractTagsMetadata()` tracks how many metadata fields remain to be found and breaks the loop when count reaches zero
- [ ] Behaviour is identical to before for all existing test cases (no metadata dropped or mis-classified)
- [ ] `BenchmarkExtractTagsMetadata` (200-tag variant) shows ≤ ns/op vs baseline (no regression)
- [ ] All unit tests pass: `go test ./comp/dogstatsd/server/...`

---

### US-007: Validate Cumulative Improvement Against Baseline
**Description:** As a developer, I want to confirm the cumulative effect of all optimisations
meets the ≥20% CPU reduction target so that the work can be signed off.

**Acceptance Criteria:**
- [ ] Run `go test -bench=. -benchmem -count=5 ./comp/dogstatsd/server/...` and save output to `plans/bench-final.txt`
- [ ] `BenchmarkParsePackets` ns/op in `bench-final.txt` is ≥20% lower than in `bench-baseline.txt`
- [ ] `BenchmarkParseMetric` (any tag-count variant) ns/op is ≥20% lower than baseline
- [ ] Zero new test failures: `go test ./comp/dogstatsd/server/...`
- [ ] Summary diff between baseline and final is appended to `plans/bench-final.txt`

---

## Functional Requirements

- FR-1: All parsing changes must produce byte-for-byte identical metric samples to the current implementation for all inputs covered by existing unit and fuzz tests.
- FR-2: `parseTags()` must not allocate a `[][]byte` intermediate slice per call.
- FR-3: `parseMetricSampleMetricType()` must resolve metric type using a direct byte switch, not `bytes.Equal` comparisons.
- FR-4: Optional-field iteration in `parseMetricSample()` must not call `bytes.Split` on the remainder of the packet.
- FR-5: The string interner must use two-generation eviction; no single eviction event must clear more than half the cache.
- FR-6: `extractTagsMetadata()` must break early when all metadata fields have been found.
- FR-7: All existing benchmarks in `comp/dogstatsd/server/` must show no regression vs the baseline for stories not directly targeting them.
- FR-8: Benchmarks in `plans/bench-baseline.txt` must be captured before any code changes.

---

## Non-Goals

- No changes to the UDP/UDS listener layer or packet assembly
- No changes to the aggregation pipeline downstream of `batcher.appendSample()`
- No changes to metric wire format or API surface
- No introduction of SIMD or CGo-based parsing
- No changes to Windows named pipe listener
- No changes to event or service-check parsing (focus is metric samples only)
- No changes to the mapper/remapping subsystem

---

## Technical Considerations

- All work is in `comp/dogstatsd/server/` (`parse.go`, `enrich.go`, `intern.go`)
- Existing benchmarks: `server_bench_test.go`, `convert_bench_test.go`, `enrich_bench_test.go`
- The `unsafe` string cast in `parseFloat64()` is already present and intentional — do not remove
- Each worker has its own `parser` instance so no locking is needed inside the parser
- The string interner `maxSize` is configurable via `dogstatsd_string_interner_size`; the two-generation approach doubles effective working set to `2 × maxSize` which is acceptable
- Fuzz targets exist for metric, event, and service check parsing — all must compile and run clean

---

## Success Metrics

- `BenchmarkParsePackets` ns/op reduced by ≥20% vs baseline
- `BenchmarkParseMetric` (any tag-count variant) ns/op reduced by ≥20% vs baseline
- `allocs/op` reduced in tag-heavy benchmarks
- Zero regressions in unit or fuzz tests

---

## Open Questions

- Should the two-generation interner be gated behind a feature flag for safe rollout, or can it replace the current implementation directly?
- Are there additional allocation hot-spots visible in the baseline CPU profile that are not covered by US-002 to US-006? (To be answered after US-001.)
- Is `BenchmarkParseMetric` with 1000 tags representative of production workloads, or should a new benchmark with a realistic tag distribution be added?
