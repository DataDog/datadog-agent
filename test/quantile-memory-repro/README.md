# Distribution-sketch memory reproducers

Minimized reproducers for excessive memory allocation in the Go agent's
distribution sketch (`pkg/util/quantile`) on `DataDog/datadog-agent`.

## Quickstart

### Unit tests (fastest, no build needed beyond Bazel)

```bash
# From the repo root. All repros are gated by DD_QUANTILE_REPRO=1.
export DD_QUANTILE_REPRO=1

# The headline case: 1.42e12 count → 21.7M bins / 1.41 GiB on main
dda inv test --module=pkg/util/quantile --targets=./... \
  --test-args='-test.run=^TestReproQuantileAllocation$/interpolate_huge -test.v -test.count=1' \
  --bazel-args='--test_env=DD_QUANTILE_REPRO=1 --test_output=streamed --nocache_test_results'

# Expected on main:
#   REPRO stage=insert ... bins=21667812 total_alloc=1513129608 mem_used=86671320
# Expected on fix (dd/quantile-bound-huge-counts):
#   REPRO stage=insert ... bins=331 total_alloc=210336 mem_used=2720
```

### DogStatsD one-liner (if you already have an agent with DSD running)

```bash
# One distribution sample at rate 1e-9 → weight 999,999,999 → 15,260 bins on main
echo "repro.dist:1|d|@1e-9" | nc -u -w1 127.0.0.1 8125

# Control: |h does NOT use sketches
echo "repro.histo:1|h|@1e-9" | nc -u -w1 127.0.0.1 8125
```

### Full integration repro (builds an isolated agent)

```bash
# 1. Build the agent (from repo root)
dda inv agent.build --build-exclude=systemd

# 2. Run the DogStatsD repro (auto-generates isolated config)
./test/quantile-memory-repro/run_dogstatsd_repro.sh 1e-9 100 15

# Expected on main:
#   Sending 100 distribution samples at rate 1e-9...
#   After sketch_too_big: 0  (or >0 if the accumulated sketch exceeds the drop boundary)
#   Peak RSS visible in /usr/bin/time output (should be elevated)

# 3. Run the Python check repro
./test/quantile-memory-repro/run_check_repro.sh
```

## Root cause (main)

`appendSafe` emits one `uint16` bin per 65,535 observations. `insertCounts`
allocates **all** of them before `trimLeft` runs, and `trimLeft` keeps the
overflow bins. Memory therefore scales with the **observation count**, not the
number of keys.

```
// pkg/util/quantile/bin.go (main)
func appendSafe(bins []bin, k Key, n int) []bin {
    if n <= maxBinWidth { return append(bins, bin{k: k, n: uint16(n)}) }
    r := uint16(n % maxBinWidth)
    if r != 0 { bins = append(bins, bin{k: k, n: r}) }
    for i := 0; i < n/maxBinWidth; i++ {        // ← one bin per 65,535 obs
        bins = append(bins, bin{k: k, n: maxBinWidth})
    }
    return bins
}

// pkg/util/quantile/store.go (main)
func (s *sparseStore) insertCounts(c *Config, kcs []KeyCount) {
    tmp := getBinList()
    // ... merge loop appends all bins via appendSafe ...
    tmp = trimLeft(tmp, c.binLimit)  // ← runs AFTER all bins are allocated
    s.bins = s.bins.ensureLen(len(tmp))
    copy(s.bins, tmp)
}
```

The fix (PR #57762) ports Saluki's `appendTrimmed`, which walks the runs
top-down and lays out only the bins `trimLeft` keeps. Allocation no longer
scales with observation count.

## Conceptual reproducers

Each reproducer targets the same root cause through a different entry point.
The core logic is repeated here so the idea is crystal clear without reading
the test code.

### 1. `InsertInterpolate` with a huge count (unit test)

**Entry point:** `pkg/util/quantile/agent.go` — `Agent.InsertInterpolate(lower, upper, count)`

**Core logic:** `InsertInterpolate(0, 0, 1.42e12)` with equal bounds produces
one key. All 1.42e12 observations land on that key. `insertCounts` calls
`appendSafe(key=0, n=1.42e12)`, which emits `ceil(1.42e12 / 65,535) = 21,667,812`
bins before `trimLeft` runs. Each bin is 4 bytes (int16 key + uint16 count),
so the transient allocation is ~83 MiB retained / ~1.41 GiB total alloc.

**Smallest input:** one call, one key, count ≥ 65,536 (first overflow bin).

### 2. `Insert` at a tiny sample rate (unit test)

**Entry point:** `pkg/util/quantile/agent.go` — `Agent.Insert(v, sampleRate)`

**Core logic:** `Insert(1, 1e-9)` computes `n = 1/sampleRate = 999,999,999`
and calls `insertCounts` with one `KeyCount{k: key(1), n: 999,999,999}`.
Same path as above: `appendSafe` emits `ceil(999,999,999 / 65,535) = 15,260`
bins. The Go agent has **no DogStatsD sample-rate floor**; any rate in (0,1] is
accepted.

**Smallest input:** one call, one key, rate < 1/65,535 (first overflow bin).

### 3. Repeated sampled inserts accumulating (unit test)

**Entry point:** same as #2, but multiple `Insert` calls into one `Agent`.

**Core logic:** each `Insert(1, 1e-9)` appends one `KeyCount{n: 999,999,999}`
to `CountBuf`. On flush, `insertCounts` processes them all. Two calls produce
`n = 1,999,999,998`, yielding `ceil(1,999,999,998 / 65,535) = 30,519` bins.
Bins accumulate linearly with call count.

**Smallest input:** N calls, one key, rate < 1/65,535, N large enough to cross
the target threshold.

### 4. CheckSampler monotonic histogram jump (unit test)

**Entry point:** `pkg/aggregator/check_sampler.go` — `addBucket` + `commit`

**Core logic:** a monotonic `HistogramBucket` with equal bounds (one key) and
raw value jumping by 1.42e12 between two commits. The first commit establishes
a baseline; the second computes `delta = 1.42e12` and calls `insertInterp`
with that delta. Same `insertCounts` → `appendSafe` path as #1.

**Smallest input:** two commits, one key, delta ≥ 65,536.

### 5. TimeSampler distribution at a tiny rate (unit test)

**Entry point:** `pkg/aggregator/time_sampler.go` — `sample` + `flush`

**Core logic:** a `MetricSample{Mtype: DistributionType, SampleRate: 1e-9}`
is inflated by `1/SampleRate = 1e9` and fed to the sketch. Same single-key,
huge-count path as #2.

**Smallest input:** one sample, one key, rate < 1/65,535.

### 6. Serializer drop boundary (unit test)

**Entry point:** `pkg/serializer/internal/metrics/sketch_series_list.go` —
`MarshalSplitCompressPipelines`

**Core logic:** the serializer checks each sketch against a worst-case
compression bound. For a single zero-key sketch under default zstd
(`serializer_max_payload_size = 2,621,440`), the drop boundary is count
42,780,854,791 (652,795 bins, 2,611,238 raw bytes, 2,621,438 compress bound).
At count-1 (652,794 bins) it's accepted; at count it's dropped with
`sketch_series.sketch_too_big += 1`. The drop is **silent** — only the
telemetry counter shows it.

**Smallest input:** one sketch, one key, count at the boundary.

### 7. DogStatsD distribution (integration)

**Entry point:** DogStatsD UDP listener → aggregator → sketch

**Core logic:** `echo "repro.dist:1|d|@1e-9" | nc -u -w1 127.0.0.1 8125` sends
one distribution sample at rate 1e-9. The agent inflates it to weight
999,999,999 and feeds it to the sketch. N packets in one flush window
accumulate. This is the same path as #2, just over the network.

**Smallest input:** one packet, one key, rate < 1/65,535. N packets to
accumulate past a threshold.

### 8. Python custom check (integration)

**Entry point:** `self.submit_histogram_bucket(name, count, lower, upper, monotonic, ...)`
→ `CheckSampler.addBucket` → `commit` → `insertInterp`

**Core logic:** a Python check calls `submit_histogram_bucket` with a huge
non-monotonic count (1.42e12) and equal bounds (one key). This goes through
`CheckSampler` → `insertInterp` → `insertCounts` → `appendSafe`, the same
path as #4. A monotonic variant uses two runs: baseline=1, then jump to
1 + 1.42e12, exercising the delta path.

**Prerequisite:** the agent's embedded Python must have `datadog_checks.base`
installed. A fresh `dda inv agent.build` provides the rtloader/CPython runtime
but **not** the integration wheels. Point `PYTHONPATH` at an installed agent's
site-packages, or `pip install datadog-checks-base` into the embedded Python.

**Smallest input:** one `submit_histogram_bucket` call, one key, count ≥ 65,536.

### 9. OTLP — NOT a memory reproducer

**Entry point:** `pkg/opentelemetry-mapping-go/otlp/metrics/validation.go`

**Core logic:** OTLP drops points whose total bucket count exceeds
`quantile.Default().MaxCount()` (= 4096 × 65,536 = 268,431,360) **before**
sketch conversion. PR #57002 added this guard. The `insertCounts` →
`appendSafe` overflow path is never reached via OTLP. `run_otlp_rejection.sh`
is a rejection control (confirms a too-large count is dropped), not a memory
reproducer.

## What's here

```
test/quantile-memory-repro/
├── README.md                  ← this file
├── datadog.yaml.template       ← isolated agent config (auto-substituted by scripts)
├── checks.d/
│   └── histogram_lab.py        ← Python check calling submit_histogram_bucket
├── conf.d/
│   └── histogram_lab.d/
│       └── conf.yaml           ← check config
├── run_dogstatsd_repro.sh      ← DogStatsD runner (auto-generates config, captures RSS/heap/telemetry)
├── run_check_repro.sh          ← Python check runner (one-shot or daemon)
├── run_otlp_rejection.sh       ← OTLP MaxCount rejection control (NOT a memory repro)
├── .gitignore                  ← excludes artifacts/, *.pb.gz, *.log, datadog.yaml
└── artifacts/
    └── results.md              ← measured main-vs-fix table and boundary table
```

## Prerequisites

- **Go agent repo** at the branch you want to test (main or fix)
- **Bazel** (`dda inv test` wrapper) for unit tests
- **Built agent** (`dda inv agent.build --build-exclude=systemd`) for integration repros
- **nc** (netcat), **jq**, **curl** for integration scripts
- **/usr/bin/time** for peak RSS capture
- For the Python check: `datadog_checks.base` in the agent's embedded Python
  (see §8 above)

## Measuring

```bash
# Peak RSS (Linux)
/usr/bin/time -v ./bin/agent/agent -c "$LAB/datadog.yaml" run

# Peak RSS (macOS)
/usr/bin/time -l ./bin/agent/agent -c "$LAB/datadog.yaml" run

# Heap profile from expvar port
curl -s http://127.0.0.1:15000/debug/pprof/heap > "$LAB/artifacts/heap.pb.gz"
go tool pprof -top "$LAB/artifacts/heap.pb.gz"

# sketch_too_big telemetry
curl -s http://127.0.0.1:15000/debug/vars | jq '.sketch_series.ItemTooBig'
```

**Caveat:** sampled heap profiles and `ps` peaks capture retained memory, not
transient allocations. `runtime.MemStats.TotalAlloc` (unit tests) measures
cumulative allocation. Do not equate the two.

## Main vs fix branch

The fix branch is `dd/quantile-bound-huge-counts` (PR #57762), which ports
Saluki's `u32` bins and `trim_left` collapse. Bin width changes from 4 bytes
(main: int16+uint16) to 8 bytes (fix: int16+uint32), but bin **count** drops
from millions to hundreds for the same input.

See `artifacts/results.md` for the full measured table. Summary:

| Input | Main bins | Main TotalAlloc | Fix bins | Fix TotalAlloc |
|---|---|---|---|---|
| `InsertInterpolate(0,0,1.42e12)` | 21,667,812 | 1.41 GiB | 331 | 205 KiB |
| `Insert(1,1e-9)` | 15,260 | 609 KiB | 1 | 2.8 KiB |
| `Insert(1,1e-12)` | 15,259,022 | 922 MiB | 233 | 201 KiB |
| Serializer @ 42.78B count | 652,795 → dropped | — | 10 → accepted | — |

## Notes

- Only distributions (`|d`) use sketches; `|h` does not.
- Go DogStatsD has no sample-rate floor; ADP clamps to 3.845e-9, but the DSD
  server itself accepts any rate in (0,1].
- OTLP already drops points past `MaxCount` (#57002) — not a repro path.
- Rates ≤ ~1.08e-19 corrupt counts (implementation-defined float→int). Excluded
  from memory repros.
