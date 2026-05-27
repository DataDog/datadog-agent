# Rust Tokenizer — Benchmark Progress

**Machine:** Apple M1 Max, darwin/arm64
**Branch:** `yoon/tokenizer-in-rust`

---

## Round 1: Initial Build (opt-level=z, workspace default)

**Date:** 2026-05-19
**Status:** All 21 tests pass (16 unit + 5 proptest)
**Problem:** Workspace `Cargo.toml` has `[profile.release] opt-level = "z"` (size-optimized), which `cargo bench` inherits. This cripples scan loop performance.

| Benchmark | Rust (ns) | Go (ns) | Ratio |
|---|---:|---:|---|
| Labeler 60B (corpus) | 694 | ~310 | 2.2x slower |
| Sampler 2048B (corpus) | 1,229 | ~513 | 2.4x slower |
| Short 6B | 89 | ~234 | 2.6x faster |
| Medium 78B | 698 | ~663 | ~same |
| Long 500B | 4,267 | ~2,042 | 2.1x slower |

**Verdict:** Misleading. The `opt-level=z` penalty is 2.5-3x on scan loops.

---

## Round 2: Profiling Variants (opt-level=z)

**Date:** 2026-05-19
**Goal:** Isolate whether AC or LUT scan is the bottleneck.

Three variants tested:
- `lut_only`: Pure LUT scan + run-length encoding, NO keyword matching
- `switch_cascade`: LUT scan + Go-style length-dispatched switch for keywords
- `ac_per_emit`: LUT scan + AC DFA `find()` on each letter-run emit (current design)

| Variant | 60B (ns) | 2048B (ns) |
|---|---:|---:|
| `lut_only` | 682 | 1,193 |
| `switch_cascade` | 601 | 1,214 |
| `ac_per_emit` | 724 | 1,290 |

**AC overhead at 2048B:** ~97ns (8.1% of total). Not the main bottleneck.
**Main bottleneck:** Entire Rust scan is 2x+ slower than Go due to `opt-level=z`.

---

## Round 3: With opt-level=3 (via RUSTFLAGS)

**Date:** 2026-05-19
**Fix:** `RUSTFLAGS="-C opt-level=3" cargo bench`

### Profiling Variants

| Variant | 60B (ns) | 2048B (ns) |
|---|---:|---:|
| `lut_only` | 217 | 335 |
| `switch_cascade` | 253 | 393 |
| `ac_per_emit` | 286 | 461 |

**Improvement from opt-level=z → opt-level=3:** ~2.5-3x across the board.

### AC Overhead Analysis (opt-level=3)

| Window | LUT-only | AC per-emit | AC overhead |
|---|---:|---:|---|
| 60B | 217ns | 286ns | +69ns (32%) |
| 2048B | 335ns | 461ns | +126ns (38%) |

**AC is adding ~30-38% overhead** vs LUT-only. The `ac.find()` call on each letter-run emit is expensive — it's doing a DFA search on a 2-9 byte substring, which has fixed setup cost that doesn't amortize.

### Switch vs AC

| Window | Switch | AC per-emit | Delta |
|---|---:|---:|---|
| 60B | 253ns | 286ns | AC is 13% slower |
| 2048B | 393ns | 461ns | AC is 17% slower |

**The Go-style switch cascade beats AC by 13-17%.** The switch's length-dispatch rejects most candidates in O(1) (wrong length = skip). AC's DFA processes all bytes of the candidate regardless.

### Production Window Benchmarks (opt-level=3, AC design)

| Benchmark | Rust (ns) | Go (ns) | Speedup |
|---|---:|---:|---|
| **Labeler 60B** | **284** | ~310 | **1.09x faster** |
| **Sampler 2048B** | **456** | ~513 | **1.12x faster** |
| **Unlimited** | **460** | ~547 | **1.19x faster** |

**Rust beats Go by 9-19%** even with AC overhead, once compiled with `opt-level=3`.

---

## Key Findings

1. **`opt-level=z` was the entire problem.** The workspace profile destroys scan loop performance. With `opt-level=3`, Rust is 9-19% faster than Go on all production windows.

2. **AC adds 30-38% overhead vs LUT-only.** Switching to Go-style switch cascade would save 13-17%, making Rust even faster:
   - Switch at 60B: 253ns (vs Go 310ns) = **1.23x faster**
   - Switch at 2048B: 393ns (vs Go 513ns) = **1.31x faster**

3. **For FFI deployment**, the question is whether the ~250ns CGo overhead eats the gains:
   - AC + FFI at 60B: 284 + 250 = 534ns vs Go 310ns → **1.72x slower**
   - AC + FFI at 2048B: 456 + 250 = 706ns vs Go 513ns → **1.38x slower**
   - Switch + FFI at 60B: 253 + 250 = 503ns vs Go 310ns → **1.62x slower**
   - Switch + FFI at 2048B: 393 + 250 = 643ns vs Go 513ns → **1.25x slower**

4. **FFI overhead still dominates.** Even with the fastest Rust variant (switch, opt-level=3), single-call FFI makes Rust slower than Go at both window sizes. The batched FFI design from the plan is essential.

---

## Round 4: Switch Cascade (AC removed, opt-level=3)

**Date:** 2026-05-19
**Change:** Replaced `aho-corasick` DFA with Go-style length-dispatched switch cascade.
The `aho-corasick` crate is kept as a dev-dependency for profiling benchmarks only;
the production tokenizer has zero external dependencies.

### Production Window Benchmarks

| Benchmark | Rust (ns) | Go (ns) | Speedup |
|---|---:|---:|---|
| **Labeler 60B** | **249** | ~310 | **1.24x faster** |
| **Sampler 2048B** | **386** | ~513 | **1.33x faster** |
| **Unlimited** | **386** | ~547 | **1.42x faster** |

### Improvement from AC → Switch

| Window | AC (ns) | Switch (ns) | Delta |
|---|---:|---:|---|
| 60B | 284 | 249 | -12% |
| 2048B | 456 | 386 | -15% |

### Summary: Pure Rust tokenizer is 24-42% faster than Go

**This is the best achievable without FFI overhead.**

### FFI Projections (single-call, assuming ~250ns overhead)

| Window | Rust + FFI (ns) | Go (ns) | Result |
|---|---:|---:|---|
| 60B | 249 + 250 = 499 | 310 | 1.61x slower |
| 2048B | 386 + 250 = 636 | 513 | 1.24x slower |

**Single-call FFI still loses.** Batched FFI or an alternative integration
strategy is required.

---

## Cumulative Progress

| Round | Change | 60B (ns) | 2048B (ns) |
|---|---|---:|---:|
| 1 | Initial (AC, opt=z) | 694 | 1,229 |
| 3 | AC, opt=3 | 284 | 456 |
| 4 | **Switch, opt=3** | **249** | **386** |
| Go baseline | — | ~310 | ~513 |

---

## Round 5: Assembly Verification

**Date:** 2026-05-19

Verified LLVM codegen for `emit_token` / `keyword_lookup` via `objdump`:

| Length | Strategy LLVM chose | Detail |
|---|---|---|
| 9-byte | Packed u64 + XOR + branch | Loads 8 bytes as x64, XORs against constants |
| 5-byte | Packed u32 + byte comparisons | Loads 4 bytes + 1 byte, decision tree |
| 4-byte | Binary search on first byte | Cascading `cmp`/`b.gt`/`b.eq` |
| 3-byte | Decision tree (39 entries) | Binary search on first byte, then sub-branches |
| 2-byte | Direct byte comparison | Check first byte A/P, then M |

**Conclusion:** LLVM already generates optimal code for the switch cascade.
No manual u32/u64 packing needed — the compiler does it automatically.
PHF would be slower (11% slower than HashMap in benchmarks, always pays hash cost).

**The switch cascade is the optimal approach for this workload.**

---

---

## Round 6: Phase 2 — C API + Go FFI Bridge

**Date:** 2026-05-19

### Initial FFI (tokenize → Vec → copy to FFI buf → Go copy)

| Benchmark | Go (ns) | Rust+FFI (ns) | Ratio |
|---|---:|---:|---|
| Labeler 60B | 255 | 395 | 1.55x slower |
| Sampler 2048B | 452 | 590 | 1.31x slower |
| FFI overhead (1-byte) | — | 82 | — |

**Problem:** Double copy — Rust allocates Vec internally, copies to FFI buffer,
then Go copies from FFI buffer to Go slices. The Vec allocation is wasted.

### Optimized FFI with tokenize_into (zero-copy scan → FFI buffer)

Refactored Rust tokenizer to write directly into caller-owned raw buffers
via `tokenize_into()`, eliminating the intermediate Vec allocation inside Rust.

| Benchmark | Go (ns) | Rust+FFI (ns) | Ratio |
|---|---:|---:|---|
| **Labeler 60B** | **256** | **283** | **1.10x slower** |
| **Sampler 2048B** | **451** | **458** | **1.02x slower (parity!)** |
| **Unlimited** | **449** | **468** | **1.04x slower** |
| FFI overhead (1-byte) | — | **50** | — |

### Analysis

The `tokenize_into` optimization was transformative:
- **FFI overhead dropped from 82ns to 50ns** (the 1-byte benchmark improved
  because there's no Vec allocation on the Rust side anymore)
- **60B improved from 395 → 283ns** (28% reduction)
- **2048B improved from 590 → 458ns** (22% reduction), now at **virtual parity with Go**

### Cost Breakdown (2048B)

| Component | Before tokenize_into | After tokenize_into |
|---|---:|---:|
| Rust scan | ~250ns | ~250ns |
| Rust Vec alloc + copy to FFI buf | ~100ns | **0** |
| CGo crossing | ~50ns | ~50ns |
| Go copy loop (FFI buf → Go slices) | ~50ns | ~50ns |
| Go slice alloc | ~100ns | ~100ns |
| **Total** | **~590ns** | **~458ns** |

### Verdict

**Rust tokenizer via FFI is now at practical parity with Go** on the sampler
path (2048B). The 60B labeler path is 10% slower — acceptable since the labeler
is not the hot path.

**Phase 3 (batched FFI) is no longer needed** — the single-call performance
is already competitive.

---

## Cumulative Progress

| Round | Change | 60B (ns) | 2048B (ns) | vs Go |
|---|---|---:|---:|---|
| 1 | Initial (AC, opt=z) | 694 | 1,229 | 2.4x slower |
| 3 | AC, opt=3 | 284 | 456 | 1.12x faster* |
| 4 | Switch cascade, opt=3 | 249 | 386 | 1.33x faster* |
| 6a | FFI (Vec copy) | 395 | 590 | 1.31x slower |
| **6b** | **FFI (tokenize_into)** | **283** | **458** | **1.02x slower (parity)** |
| Go | — | 256 | 451 | baseline |

*Pure Rust, no FFI overhead

---

---

## Round 7: Go Pipeline Optimization 1 — JSON Fast-Reject

**Date:** 2026-05-19
**Change:** Added `looksLikeJSON()` fast-reject in `json_aggregator.go`.
Non-JSON messages (first non-whitespace byte not `{`/`[`) now return
immediately without calling `json.Valid()` or entering the decoder path.

### Results

| Benchmark | Before (ns) | After (ns) | Speedup | Allocs |
|---|---:|---:|---|---|
| **NonJSON** | ~990 | **99** | **10x faster** | 20 → 3 |
| **MixedCorpus** | ~1,235 | **173** | **7.1x faster** | 24 → 4 |
| SingleLineJSON | 520 | 517 | no change | 3 → 3 |
| InvalidJSON (starts with `{`) | 1,400 | 1,400 | no change | 27 → 27 |
| ComplexNestedJSON | 972 | 978 | no change | 3 → 3 |

**Non-JSON messages: 10x faster (990ns → 99ns).** The entire json aggregator
path is bypassed. This is the biggest single optimization in the project.

**Mixed corpus: 7.1x faster (1,235ns → 173ns).** The corpus is ~75% non-JSON,
so the aggregate improvement reflects the production mix.

**JSON messages: unchanged.** The `looksLikeJSON` check adds ~1ns overhead,
invisible in benchmarks.

### Impact on Full Pipeline

The JSON aggregator was 31-35% of processMessage CPU. For non-JSON messages,
we've eliminated virtually all of that cost. Expected full pipeline savings:
**~25-28% CPU reduction** on mixed workloads.

---

## Next Steps

- [x] Replace AC with switch cascade
- [x] Verify LLVM codegen
- [x] Phase 2: C API + Go FFI bridge
- [x] Optimize FFI with tokenize_into (zero-copy)
- [x] **Optimization 1: JSON fast-reject (10x on non-JSON)**
- [x] Optimization 2: IsMatch length pre-filter (~4% on sampler scan)
- [x] **Optimization 3: Tokenizer index zero-copy (24-25% faster, 1 alloc eliminated)**
- [ ] Investigate opt-level=3 for Rust production build
- [ ] SMP regression test with adaptive sampling under load

---

## SMP Regression Test Plan

### What to Test

The tokenizer optimizations that matter for adaptive sampling:
- **Index zero-copy** (Opt 3): 24% faster tokenizer, 1 fewer alloc/call → 83% faster at 16 goroutines
- **IsMatch length pre-filter** (Opt 2): ~4% faster sampler scan
- **JSON fast-reject** (Opt 1): Only applies if auto-multiline + JSON aggregation enabled

### Test Case Structure

```
test/regression/cases/adaptive_sampling_tokenizer/
├── experiment.yaml
├── lading/
│   └── lading.yaml
└── datadog-agent/
    └── datadog.yaml
```

### lading.yaml — High-Volume Mixed Log Load

```yaml
generator:
  - file_gen:
      logrotate_fs:
        seed: [2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37, 41, 43, 47, 53,
               59, 61, 67, 71, 73, 79, 83, 89, 97, 101, 103, 107, 109, 113, 127, 131]
        load_profile:
          constant: 10 MiB
        concurrent_logs: 4          # 4 files = 4 decoder goroutines
        maximum_bytes_per_log: 500 MiB
        total_rotations: 5
        max_depth: 0
        variant: "apache_common"    # Non-JSON, heavy tokenization
        maximum_prebuild_cache_size_bytes: 300 MiB
        mount_point: /smp-shared

blackhole:
  - http:
      binding_addr: "127.0.0.1:9091"
  - http:
      binding_addr: "127.0.0.1:9092"
      response_delay_millis: 0
  - http:
      binding_addr: "127.0.0.1:9093"

target_metrics:
  - prometheus:
      uri: "http://127.0.0.1:5000/telemetry"
      tags:
        sub_agent: "core"
```

### datadog.yaml — Enable Adaptive Sampling

```yaml
auth_token_file_path: /tmp/agent-auth-token
cloud_provider_metadata: []

dd_url: http://127.0.0.1:9091

logs_enabled: true
logs_config:
  logs_dd_url: 127.0.0.1:9092
  file_scan_period: 1
  logs_no_ssl: true
  force_use_http: true

  # Enable adaptive sampling (the feature that uses the tokenizer)
  experimental_adaptive_sampling:
    enabled: true
    tokenizer_max_input_bytes: 2048
    max_patterns: 1000
    match_threshold: 0.9
    protect_important_logs: true

  # Enable auto-multiline (activates tokenizer + labeler)
  auto_multi_line_detection: true

process_config.process_dd_url: http://localhost:9093
telemetry.enabled: true
telemetry.checks: '*'
```

### experiment.yaml

```yaml
optimization_goal: cpu
erratic: false

target:
  name: datadog-agent
  cpu_allotment: 4
  memory_allotment: 2GiB

  environment:
    DD_API_KEY: a0000001
    DD_HOSTNAME: smp-regression

  profiling_environment:
    DD_INTERNAL_PROFILING_BLOCK_PROFILE_RATE: 10000
    DD_INTERNAL_PROFILING_CPU_DURATION: 1m
    DD_INTERNAL_PROFILING_DELTA_PROFILES: true
    DD_INTERNAL_PROFILING_ENABLED: true
    DD_INTERNAL_PROFILING_ENABLE_GOROUTINE_STACKTRACES: true
    DD_INTERNAL_PROFILING_MUTEX_PROFILE_FRACTION: 10
    DD_INTERNAL_PROFILING_PERIOD: 1m
    DD_INTERNAL_PROFILING_UNIX_SOCKET: /smp-host/apm.socket
    DD_PROFILING_EXECUTION_TRACE_ENABLED: true
    DD_PROFILING_EXECUTION_TRACE_PERIOD: 1m
    DD_PROFILING_WAIT_PROFILE: true

checks:
  - name: memory_usage
    description: "Pattern table + tokenizer memory"
    bounds:
      series: total_pss_bytes
      upper_bound: 1.5GiB

  - name: missed_bytes
    description: "Log data not consumed — tokenizer too slow"
    bounds:
      series: missed_bytes
      upper_bound: 0KiB
```

### SMP Results — Run 87c5507c (2026-05-21)

Baseline: 7.77.3, Comparison: 7.78.2, 1 replica, 50 MiB/s

| Experiment | CPU Δ mean % | CPU Δ CI | Memory | Verdict |
|---|---|---|---|---|
| `tokenizer_zerocopy_50mbs_0ms` | +7.02% | [-11.74, +25.79] | 0.12 GiB (pass) | ➖ No significant change |
| `tokenizer_couldmatch_50mbs_0ms` | +0.12% | [-17.69, +17.94] | 0.12 GiB (pass) | ➖ No significant change |
| `tokenizer_json_50mbs_0ms` | — | — | — | ❌ Crashed (incompatible with static_timestamped) |

**Analysis:** Both working cases show no measurable CPU change. This is expected —
the optimizations (index zero-copy, couldMatch pre-filter) are in OUR code changes
that aren't in either the 7.77.3 baseline or the 7.78.2 comparison. Both images
are stock releases without our optimizations. The SMP run validates that the
infrastructure works, but to measure our actual changes we need to build agent
images from our branch.

**JSON case removed** — `static_timestamped` prepends timestamps to all lines,
making `looksLikeJSON()` always return false. Incompatible with testing a
feature that detects `{` as the first byte.

### Key Metrics to Compare (baseline vs optimized)

| Metric | Source | What it shows |
|---|---|---|
| CPU time in `Tokenize` | pprof flame graph | Direct tokenizer speedup |
| CPU time in `IsMatch` | pprof flame graph | Sampler scan speedup |
| `total_pss_bytes` | SMP bounds check | Memory stability |
| `missed_bytes` | SMP bounds check | Throughput — 0 = agent keeps up |
| `logs_adaptive_sampler.new_patterns` | telemetry | Pattern churn rate |
| GC pause time | pprof / runtime metrics | Alloc reduction impact |

### How to Run

```bash
# Submit SMP job comparing baseline (main) vs optimized (this branch)
smp job submit \
  --baseline-image datadog/agent-dev:nightly-main-<main-sha>-py3 \
  --comparison-image datadog/agent-dev:nightly-main-<branch-sha>-py3 \
  --target-config-dir test/regression/ \
  --case adaptive_sampling_tokenizer
```

---

## Round 9: Go Pipeline Optimization 3 — Tokenizer Index Zero-Copy

**Date:** 2026-05-19
**Change:** Eliminated one of two allocations in `tokenizer.go`. Token indices
are now returned as a view into the internal buffer (callers only read them in
`limitTokensToBytes`, never store). Token slice still allocated (sampler stores it).

### Results

| Benchmark | Before (ns) | Before B/op | After (ns) | After B/op | Speedup |
|---|---:|---:|---:|---:|---|
| **Labeler 60B** | 255 | 265 (2 allocs) | **194** | 34 (1 alloc) | **24% faster** |
| **Sampler 2048B** | 451 | 476 (2 allocs) | **343** | 58 (1 alloc) | **24% faster** |

**The tokenizer is now 24% faster** with 1 allocation eliminated and 78-87%
less memory allocated per call. This is a bigger win than expected — the
index allocation was significant because `[]int` on 64-bit is 8 bytes per
element vs `[]Token` (1 byte per element).

### Cumulative Go Tokenizer Progress

| State | 60B (ns) | 2048B (ns) |
|---|---:|---:|
| Original Go | 310 | 513 |
| After Opt 3 (index zero-copy) | **194** | **343** |
| **Total improvement** | **37% faster** | **33% faster** |

The Go tokenizer is now significantly faster than the Rust+FFI path (283ns/458ns).

---

## Round 8: Go Pipeline Optimization 2 — IsMatch Length Pre-Filter

**Date:** 2026-05-19
**Change:** Added `couldMatch()` length-based pre-filter in `sampler.go:203`.
Rejects entries where token count ratio falls below threshold before running
the full token-by-token `IsMatch` comparison.

### Results

| Benchmark | Before (ns) | After (ns) | Improvement |
|---|---:|---:|---|
| SteadyState P10 | 672 | 639 | 4.9% |
| SteadyState P100 | 667 | 639 | 4.2% |
| SteadyState P500 | 662 | 635 | 4.1% |
| SteadyState P1000 | 669 | 639 | 4.5% |
| FullScan (all sizes) | ~89 | ~90 | no change |

**Modest ~4% improvement** on the sampler scan path. The benchmarks find matches
early (hot patterns bubble to front), so most entries aren't scanned. The
pre-filter helps more in worst-case scenarios (new pattern, full table scan).
