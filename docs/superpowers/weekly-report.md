# Weekly Report — Rust Preprocessor Tokenizer

**Week of:** 2026-05-18 – 2026-05-21  
**Author:** Yoon Nguyen

---

## What did I try?

Investigated whether rewriting the preprocessor tokenizer in Rust and calling it from Go via CGo FFI would produce a measurable CPU improvement on the adaptive sampling and auto-multiline detection pipeline.

The preprocessor tokenizer converts raw log bytes into a compact structural fingerprint (68 token types). It runs on every log line when adaptive sampling or auto-multiline detection is enabled. Based on CPU profiles, tokenization accounts for roughly 5-8% of the full pipeline CPU.

The work involved:

- **Profiling the existing Go tokenizer** to establish baselines at the two production window sizes: 60B (labeler) and 2048B (sampler)
- **Researching Rust keyword-matching libraries** — aho-corasick, phf, daachorse, regex-automata — to identify whether any could beat the Go switch cascade
- **Building a complete Rust crate** (`pkg/logs/internal/decoder/preprocessor/rust/`) with:
  - Full token type system mirroring Go's iota exactly (`#[repr(u8)]`)
  - LUT-based byte classification + length-dispatched switch cascade for keyword recognition
  - `extern "C"` API with `panic::catch_unwind` at every boundary, cbindgen-generated header
  - CGo bridge in Go behind `//go:build rust_preprocessor && cgo`
- **Correctness verification** via 21 unit tests + 5 proptest property-based tests running 50,000+ random byte sequences, confirming byte-identical output to Go across all inputs
- **Iterative benchmarking** through 6 rounds, including profiling variant isolation (LUT-only vs switch vs AC DFA) and FFI overhead measurement

AI agents were used throughout: parallel Explore agents to understand existing codebase patterns and FFI conventions (`pkg/discovery/module/rust/`), a Plan agent for architecture review before writing code, and an Advisor call to catch a correctness bug in the AC state boundary logic before it was implemented.

---

## What worked?

**The Rust scan is faster in isolation.** With `opt-level=3`, the Rust tokenizer (switch cascade design) runs at:

| Window | Go (ns) | Rust pure (ns) | Speedup |
|---|---:|---:|---|
| 60B | 310 | 249 | 1.24x faster |
| 2048B | 513 | 386 | 1.33x faster |

This matched expectations: no GC safepoint polls, no bounds checks on hot-path array accesses.

**The switch cascade beat aho-corasick.** The initial design used aho-corasick's DFA for keyword recognition, assuming a compiled automaton would outperform Go's switch statements. It didn't — AC was 13-17% slower than the switch cascade. The profiling variant benchmarks isolated this cleanly. LLVM assembly inspection confirmed the compiler already generates packed integer comparisons (`u32`/`u64` XOR) for the switch arms — the switch was already near-optimal code. The correct architecture was determined empirically, not by assumption.

**The `tokenize_into` optimization.** When the Rust FFI initially wrote results into a temporary `Vec` then copied to the caller's buffer, FFI overhead was 82ns. Switching to writing directly into caller-owned output buffers dropped FFI overhead to 50ns — a measurable improvement confirmed across 10 benchmark runs with sub-1ns variance.

**Proptest caught a correctness issue.** The property-based tests (`prop_parity`, `prop_case_insensitive`, `prop_truncation`) caught a discrepancy in the reference tokenizer during development before any Go integration existed.

---

## What failed?

**FFI overhead reverses the scan advantage.** With the 50ns CGo crossing cost, the end-to-end Rust+FFI numbers are worse than Go at both production window sizes (10 benchmark runs, -count=10):

| Benchmark | Go (ns) | Rust+FFI (ns) | Result |
|---|---:|---:|---|
| Labeler 60B | 201 | 294 | 32% slower |
| Sampler 2048B | 358 | 486 | 26% slower |

The Rust scan saves ~60-120ns per call. The FFI crossing costs 50ns. The remaining 10-70ns margin was not enough to overcome the second alloc that Rust+FFI still requires (the sampler stores returned token slices, so both paths need 2 allocations per call).

**Batched FFI was not implemented.** The design called for a batch API that would amortize the 50ns crossing across N messages. At batch=8, the projected Rust+FFI advantage at 2048B would be roughly 2x. This was not built because the single-call numbers didn't justify the pipeline complexity — by the time the end-to-end FFI overhead was measured, the original single-call design was already clearly slower than Go, and the batch design would require changes to the decoder pipeline (message accumulation before the preprocessor) that were out of scope.

**The workspace `opt-level=z` profile caused 6 rounds of misleading benchmarks.** The Cargo workspace defaults to `opt-level=z` (optimize for binary size). Cargo bench inherits this profile, so the first round of Rust benchmarks showed 2-2.4x slower than Go — making the entire approach look like a failure before the profile issue was identified. The fix required passing `RUSTFLAGS="-C opt-level=3"` explicitly, which also doesn't appear in cargo build verbose output, making it easy to misread whether the flag was applied. This added roughly a day of debugging.

**aho-corasick was the wrong library for this use case.** The hypothesis was that a compiled multi-pattern DFA would outperform sequential string comparisons for 72 keywords. The assumption was wrong because the problem shape was wrong — the tokenizer checks keywords on isolated letter runs (2-9 bytes), not in a continuous byte stream. At that scale, the DFA's fixed setup cost doesn't amortize. This was learned from benchmarks, not from first principles.

---

## What is reusable?

**The Rust crate itself.** `pkg/logs/internal/decoder/preprocessor/rust/` is fully functional: zero-allocation tokenizer, verified correct, production-ready FFI. It's gated behind `//go:build rust_preprocessor && cgo` and does not affect any existing builds. If the pipeline ever needs to push more work into the FFI call (e.g., running IsMatch inside Rust alongside tokenization), the plumbing already exists.

**The FFI pattern for this codebase.** The crate follows the exact same patterns as `pkg/discovery/module/rust/`: `#[unsafe(no_mangle)]`, `panic::catch_unwind(AssertUnwindSafe(...))` at every boundary, cbindgen-generated header, `crate-type = ["rlib", "staticlib"]`, caller-owned output buffers. Any future Rust FFI work can copy this structure directly.

**The SMP experiment infrastructure for adaptive sampling.** `smp-playground/yoon/tokenizer-optimizations` branch has two working SMP experiment cases (`tokenizer_zerocopy_50mbs_0ms`, `tokenizer_couldmatch_50mbs_0ms`) validated end-to-end — 50 MiB/s load, correct lading version, agent config with adaptive sampling enabled. These can be adapted as a template for future sampling pipeline experiments.

**The SMP skill learnings.** Six specific failure modes documented in `.claude/skills/smp-logs-feature/SKILL.md`:
- Lading version compatibility (`logrotate` vs `logrotate_fs`, which SHA supports what)
- `static_timestamped` required fields and incompatibility with first-byte detection features
- Git worktree mount requirements for Docker-based builds
- dda Python dependency installation in the build image
- NUM_REPLICAS=1 for iterative development

**The `tokenize_into` pattern.** For any Go ↔ Rust FFI where the Rust function produces variable-length output: passing caller-owned output buffers and writing directly into them eliminates one allocation inside Rust. This dropped FFI overhead from 82ns to 50ns here and applies to any similar API.

---

## Impact / Learnings

**Time spent:** ~3 days across design, implementation, benchmarking, and SMP setup.

**Direct outcome:** The Rust tokenizer was not deployed. Single-call FFI is 26-32% slower than the current Go tokenizer at production window sizes, with no batching implemented to close the gap.

**Indirect outcome:** The benchmarking process produced a concrete understanding of where the tokenizer's time actually goes — scan loop vs keyword lookup vs allocation vs FFI crossing — with nanosecond-level measurements. This ruled out Rust as a path forward for the current workload without relying on estimates.

**Key learning:** For CGo FFI to produce a net improvement, the computation inside the FFI boundary needs to be substantially larger than the crossing cost (~50ns). At the current tokenizer window sizes, the scan work (250-400ns) is only 5-8x the crossing cost, which is insufficient margin once allocations and Go-side copy overhead are included. A rule of thumb: FFI makes sense when the work inside the boundary is at least 20-30x the crossing cost, or when the boundary is crossed infrequently relative to the work done (e.g., batch APIs, long-running computations).

**What was validated:** The Go tokenizer is already well-optimized for its problem shape. A LUT + length-dispatched switch cascade on isolated letter runs is a near-optimal approach — LLVM generates packed integer comparisons for the switch arms, and the 256-byte LUT fits in one cache line. There is no library that improves on this for the specific sub-problem of classifying known-length candidates.
