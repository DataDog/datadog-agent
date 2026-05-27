# Rust Preprocessor Tokenizer — Design Document

**Date:** 2026-05-18
**Author:** Yoon Nguyen
**Status:** Draft
**Prereq:** [preprocessor-tokenizer-architecture.md](preprocessor-tokenizer-architecture.md)

---

## 1. Why Rust, Not "Same Algorithm Faster"

The Go tokenizer is already near-optimal for its approach: 256-byte LUT,
single-pass O(n), 2 allocs per call. Rewriting the same algorithm in Rust
gains nothing — CGo FFI overhead (~200-500ns) would eat the savings.

Rust wins on two axes Go cannot match:

### A. Faster Scan Loop — No GC Safepoints, No Bounds Checks

Go's hot loop has hidden overhead that doesn't show in the algorithm:
- **GC safepoint polls** inserted by the compiler in every loop iteration
- **Bounds checks** on every `input[i]`, `tokenLookup[char]`, `toUpperLookup[char]`
- **Escape analysis limitations** — `emitToken` takes and returns slices,
  defeating some optimizations

Rust's hot loop has none of these. The byte scan uses unchecked indexing
into a static LUT, no GC exists, and `emit()` is always inlined. These
add up to 10-20% on the scan itself.

**Note on allocations:** The Go tokenizer's 2 allocs per call (lines
218-224) exist because callers take ownership of the returned slices —
`sampler.go:251` stores `tokens` for future `IsMatch` comparisons.
The Rust bridge must also allocate result slices for the same reason.
The win is scan speed, not allocation elimination.

### B. Aho-Corasick DFA for Keyword Recognition

Go's keyword matching is a length-dispatched switch with ~60 string
comparisons (`getSpecialLongToken`). It checks on every letter-run emit.

Rust's `aho-corasick` crate compiles all 60 keywords into a single DFA
with case-insensitive matching baked into the state transitions. The
automaton state advances alongside the byte-classification loop — zero
extra work per byte, keyword detection falls out of the DFA state at
emit time.

This is not a marginal improvement. It's a fundamentally different data
structure: O(n) with a constant factor near zero for keyword matching,
versus O(n × k) where k is the number of keywords checked per emit.

---

## 2. Architecture: Fused Single-Pass Design

```
Input bytes ──┐
              ▼
         ┌─────────────────────────────────────────────────────┐
         │              Fused Scan Loop                        │
         │                                                     │
         │  prev_ac_state = ac.start_state()                   │
         │  ac_state = ac.start_state()                        │
         │                                                     │
         │  for each byte:                                     │
         │    1. class = CHAR_CLASS_LUT[byte]        (0.3ns)   │
         │    2. prev_ac_state = ac_state                      │
         │       ac_state = ac.next(ac_state, byte)  (1-2ns)   │
         │    3. if class != prev_class:                       │
         │         emit(prev_class, run_len, prev_ac_state)    │
         │         if class != Letter:                         │
         │           ac_state = ac.start_state()               │
         │         reset run                                   │
         │    4. else: run_len++ (cap at 10)                   │
         │                                                     │
         │  emit():                                            │
         │    if prev_class == Letter && run_len <= 9:         │
         │      if ac.has_match(prev_ac_state) &&              │
         │         match_len == run_len:                        │
         │           → write keyword token                     │
         │      else:                                          │
         │           → write C{run_len} token                  │
         │    elif prev_class == Digit:                        │
         │           → write D{run_len} token                  │
         │    else:                                            │
         │           → write punctuation/space token           │
         │                                                     │
         │    write start_index to indices buffer              │
         └─────────────────────────────────────────────────────┘
              │
              ▼
         tokens_out[], indices_out[]  (caller-owned)
```

**Why fused, not two-pass:**
- Two-pass (AC bulk scan, then LUT pass) requires merging AC match
  positions with letter-run boundaries. AC would match "WARN" inside
  "XWARNING" — you'd need post-filtering. Fused is correct by
  construction: the AC match length is checked against the run length
  at emit time, guaranteeing whole-token matching.
- Two-pass touches the input twice. For 2048-byte inputs in L1 cache
  this barely matters, but it doubles branch mispredictions.
- The Teddy SIMD prefilter (AC's acceleration for <100 patterns) only
  works in bulk-search mode, not per-byte walking. But at 60 bytes
  (labeler window) SIMD has no room to accelerate anyway.

---

## 3. Aho-Corasick Automaton Design

### 3.1 Pattern Set (60 keywords → Token mappings)

```
Severity (13 keywords, some with aliases):
  WARN, WARNING → Warn        FATAL → Fatal
  ERROR → Error                PANIC → Panic
  ALERT → Alert               SEVERE → Severe
  CRIT, CRITICAL → Critical   EMERG, EMERGENCY → Emergency
  EXCEPTION → Exception       CRASH, CRASHED → Crash
  FAILED, FAILURE → Failure   DEADLOCK → Deadlock
  TIMEOUT → Timeout

Calendar (31 keywords):
  JAN..DEC (12) → Month
  MON..SUN (7) → Day
  AM, PM (2) → Apm
  UTC, GMT, EST, EDT, CST, CDT, MST, MDT, PST, PDT,
  JST, KST, IST, MSK, CET, BST, HST, HDT, NST, NDT,
  CEST, NZST, NZDT, ACST, ACDT, AEST, AEDT, AWST, AWDT,
  AKST, AKDT, CHST, CHDT (33) → Zone

Single-char (handled separately, not in AC):
  T → T
  Z → Zone
```

### 3.2 Automaton Configuration

```rust
use aho_corasick::{AhoCorasick, AhoCorasickKind, MatchKind};

let ac = AhoCorasick::builder()
    .ascii_case_insensitive(true)   // fold at build time, not per-search
    .kind(Some(AhoCorasickKind::DFA)) // dense DFA for fastest next_state()
    .match_kind(MatchKind::Standard)  // leftmost-first, report all matches
    .build(PATTERNS)
    .unwrap();
```

**Memory estimate:** 60 patterns, 2-9 chars, alphabet compressed to ~30
equivalence classes. Approximately 200-400 states × 30 × 4 bytes =
**24-48 KB**. Fits in L1 cache. Built once at `tokenizer_new()`, amortized
to zero.

### 3.3 Keyword Boundary Correctness

The fused design guarantees whole-token matching without word-boundary
logic:

1. The LUT classifies each byte. Letters are `CharClass::Letter`.
2. A letter run accumulates until a non-letter byte.
3. **Critical:** the loop saves `prev_ac_state` before advancing with
   the current byte. When a non-letter byte triggers emit, the emit
   function checks `prev_ac_state` (the state after the last letter),
   not `ac_state` (which has already been advanced with the non-letter
   byte). After emit, `ac_state` is reset to `start_state()`.
4. At emit, if AC reports a match on `prev_ac_state` AND
   `match_len == run_len`, the keyword spans exactly one token →
   promote to keyword token.
5. If the match is shorter than the run (e.g., "WARNINGS" is 8 chars,
   "WARNING" match is 7), the length check fails → emit `C8`.

This is strictly correct because the tokenizer already defines token
boundaries by character class transitions. A keyword can only be
recognized when it IS the entire token.

---

## 4. C API / FFI Boundary

```c
// dd_preprocessor_tokenizer.h (generated by cbindgen)

#include <stdint.h>
#include <stddef.h>

// Opaque handle to the Rust tokenizer (holds the AC automaton).
typedef struct dd_tokenizer dd_tokenizer;

// Create a tokenizer. max_eval_bytes: 0 = unlimited.
// Returns NULL on allocation failure (should never happen).
dd_tokenizer* dd_tokenizer_new(size_t max_eval_bytes);

// Free a tokenizer.
void dd_tokenizer_free(dd_tokenizer* t);

// Tokenize input bytes. Writes tokens and indices into caller-owned buffers.
// Returns the number of tokens written, or -1 if capacity is insufficient.
//
// tokens_out:  caller-allocated buffer for token bytes
// indices_out: caller-allocated buffer for start indices (int32)
// capacity:    size of both buffers (in elements, not bytes)
//
// The caller should retry with a larger buffer if -1 is returned.
int32_t dd_tokenizer_tokenize(
    dd_tokenizer* t,
    const uint8_t* input,
    size_t input_len,
    uint8_t* tokens_out,
    int32_t* indices_out,
    size_t capacity
);
```

### 4.1 Design Decisions

**Why `staticlib` + cbindgen (not `cdylib`):**
Following the `dd_discovery` precedent — static linking avoids shared
library distribution headaches. cbindgen generates the C header from
Rust types.

**Why `int32_t` for indices (not `size_t`):**
Go's `int` is 64-bit but log lines are <64KB. `int32_t` halves the
memory bandwidth for the index buffer. Go converts with a simple cast.

**Why caller-owned buffers:**
This is the key performance win. The Go Tokenizer holds two
`[]byte`/`[]int32` slices sized to the max expected output. Each call
writes into them — zero allocation, zero GC pressure. The Rust side
never allocates during tokenization.

**Why opaque handle:**
The `dd_tokenizer` struct owns the compiled AC automaton (~30KB). It's
created once per decoder goroutine (matching Go's one-Tokenizer-per-
goroutine model) and reused for the lifetime of the source.

---

## 5. Go Integration

### 5.1 Build Tag

```go
//go:build rust_preprocessor && cgo
```

Separate from the existing `rust_patterns` tag (pattern clustering
tokenizer). Both can be enabled independently.

### 5.2 CGo Bridge

```go
package preprocessor

/*
#cgo CFLAGS:  -I${SRCDIR}/rust/include
#cgo LDFLAGS: -L${SRCDIR}/rust/target/release -ldd_preprocessor_tokenizer
#include "dd_preprocessor_tokenizer.h"
*/
import "C"

import "unsafe"

type RustTokenizer struct {
    handle    *C.dd_tokenizer
    maxEval   int
    tokensBuf []byte   // pre-allocated, reused across calls
    idxBuf    []int32  // pre-allocated, reused across calls
}

func NewRustTokenizer(maxEvalBytes int) *RustTokenizer {
    initCap := 256 // generous initial capacity
    return &RustTokenizer{
        handle:    C.dd_tokenizer_new(C.size_t(maxEvalBytes)),
        maxEval:   maxEvalBytes,
        tokensBuf: make([]byte, initCap),
        idxBuf:    make([]int32, initCap),
    }
}

func (t *RustTokenizer) Tokenize(input []byte) ([]Token, []int) {
    if len(input) == 0 {
        return nil, nil
    }
    for {
        n := C.dd_tokenizer_tokenize(
            t.handle,
            (*C.uint8_t)(unsafe.Pointer(&input[0])),
            C.size_t(len(input)),
            (*C.uint8_t)(unsafe.Pointer(&t.tokensBuf[0])),
            (*C.int32_t)(unsafe.Pointer(&t.idxBuf[0])),
            C.size_t(len(t.tokensBuf)),
        )
        if n >= 0 {
            // Convert in-place: tokensBuf bytes → Token slice
            tokens := make([]Token, n)
            for i := int32(0); i < n; i++ {
                tokens[i] = Token(t.tokensBuf[i])
            }
            indices := make([]int, n)
            for i := int32(0); i < n; i++ {
                indices[i] = int(t.idxBuf[i])
            }
            return tokens, indices
        }
        // Capacity insufficient — double and retry
        t.tokensBuf = make([]byte, len(t.tokensBuf)*2)
        t.idxBuf = make([]int32, len(t.idxBuf)*2)
    }
}

func (t *RustTokenizer) Close() {
    if t.handle != nil {
        C.dd_tokenizer_free(t.handle)
        t.handle = nil
    }
}
```

### 5.3 Why Not Zero-Copy?

The bridge copies from FFI buffers → Go slices. This is intentional:
`sampler.go:251` stores the returned `tokens` slice for future `IsMatch`
comparisons. If Tokenize returned a view into the reusable FFI buffer,
the next call would overwrite it. The 2 allocs per call are the same
cost as Go's tokenizer — the win is scan speed, not allocation count.

The `int32_t` indices are also copied with widening to Go's 64-bit `int`.
This is unavoidable without matching Go's `int` width in the FFI buffer,
which would double bandwidth for no benefit (log lines are <64KB).

---

## 6. Performance Analysis

### 6.1 Per-Byte Cost Comparison

| Operation | Go (ns/byte) | Rust (ns/byte) | Notes |
|---|---:|---:|---|
| LUT lookup | ~0.3 | ~0.3 | Identical — same 256-byte table |
| Run-length tracking | ~0.1 | ~0.1 | Same logic |
| Keyword check (on emit) | ~2-5 | ~0.3 | Go: switch cascade; Rust: AC DFA state check |
| GC safepoint polls | ~0.1-0.3 | 0 | Go inserts checks in loops |
| Bounds checks | ~0.05-0.1 | 0 | Go checks input[i], LUT[char] |
| Allocation (2× per call) | same | same | Both must copy — callers own the result |
| **Total per byte** | **~2.5-5.8** | **~0.7** | |

**Note on allocations:** Both Go and Rust paths require 2 allocs per
call. The sampler stores returned token slices for future IsMatch
comparisons (`sampler.go:251`), so the caller must own the memory.
The Rust bridge copies from its FFI buffer into Go-allocated slices.
Allocation cost is identical for both paths.

### 6.2 End-to-End Projections

| Scenario | Go (ns) | Rust (ns) | Speedup |
|---|---:|---:|---|
| **60B labeler window** | | | |
| Scan + overhead | ~210 | ~42 | |
| Keyword checks (~3 emits) | ~10 | ~1 | |
| Alloc (2×, identical) | ~90 | ~90 | |
| FFI overhead | 0 | ~250 | |
| **Total** | **~310** | **~383** | **0.81x (slower)** |
| **2048B sampler window** | | | |
| Scan + overhead | ~350 | ~140 | |
| Keyword checks (~30 emits) | ~90 | ~9 | |
| Alloc (2×, identical) | ~70 | ~70 | |
| FFI overhead | 0 | ~250 | |
| **Total** | **~513** | **~469** | **1.09x** |

**60-byte window:** Rust is slower. FFI overhead dominates — the input
is too small for scan speed gains to overcome the ~250ns crossing cost.
This is the labeler path (auto-multiline), which is not the hot path.

**2048-byte window:** Modest ~9% win. The AC DFA advantage over the
switch cascade compounds over more bytes, but FFI eats most of the gain.

### 6.3 When Does Rust Actually Win Big?

The projections above show Rust is marginal at best for the current
pipeline. But three factors change the calculus:

**A. Batched FFI — amortize the crossing cost.**
If we batch N log lines per FFI call (Section 6.4), the ~250ns overhead
is divided by N. At batch=8, FFI cost drops to ~31ns per line. The
2048-byte projection becomes ~250ns vs Go's ~513ns — a **2x speedup**.

**B. Longer inputs.**
The sampler window is configurable. At 4096 bytes, Rust scan is ~280ns
vs Go's ~700ns. With single-call FFI, total is ~530ns vs ~770ns (1.45x).

**C. Future pipeline changes.**
If the pipeline moves to process more bytes per message (e.g., full log
body tokenization for better clustering), the per-byte advantage
compounds and FFI overhead becomes negligible.

### 6.4 Batched FFI Design (Key Optimization)

Instead of one FFI call per log line, batch multiple lines:

```c
// Batch API: tokenize multiple inputs in one FFI crossing
int32_t dd_tokenizer_tokenize_batch(
    dd_tokenizer* t,
    const uint8_t** inputs,      // array of input pointers
    const size_t* input_lens,    // array of input lengths
    size_t count,                // number of inputs
    uint8_t* tokens_out,         // flat output buffer
    int32_t* indices_out,        // flat output buffer
    int32_t* offsets_out,        // per-input offsets into output
    size_t capacity
);
```

This amortizes FFI overhead across the batch. The Go side collects
messages until a batch threshold (count or time), then makes one call.

**Tradeoff:** Adds latency (must wait for batch to fill). For the
sampler path this is acceptable — messages are already queued. For the
labeler path (latency-sensitive), single-call is still used.

### 6.5 Hidden Rust Advantages (Hard to Quantify)

1. **No GC safepoints** — Go inserts cooperative preemption checks in
   loops. At ~0.1-0.3ns/byte, this is 200-600ns over 2048 bytes.
2. **No bounds checking** — `unsafe` byte access eliminates ~0.05-0.1ns/byte
3. **Better inlining** — `emit()` is always inlined; Go's escape
   analysis may prevent inlining when slices are returned
4. **Cache locality** — Rust struct is ~30KB (AC DFA) in a contiguous
   allocation; Go's Tokenizer has pointer indirection to heap slices

These are hard to measure in isolation but typically add 10-20%.

---

## 7. Rust Crate Structure

```
pkg/logs/internal/decoder/preprocessor/rust/
├── Cargo.toml
├── cbindgen.toml
├── include/
│   └── dd_preprocessor_tokenizer.h   (generated)
├── src/
│   ├── lib.rs          // C API: dd_tokenizer_new/free/tokenize
│   ├── tokenizer.rs    // Fused LUT + AC scan loop
│   ├── tokens.rs       // Token enum mirroring Go's tokens.go
│   └── keywords.rs     // AC pattern list + Token mappings
└── tests/
    └── parity_test.rs  // Property: Rust output == Go output for all inputs
```

### 7.1 Cargo.toml

```toml
[package]
name = "dd-preprocessor-tokenizer"
version = "0.1.0"
edition.workspace = true
license.workspace = true
rust-version.workspace = true

[lib]
name = "dd_preprocessor_tokenizer"
crate-type = ["staticlib"]

[dependencies]
aho-corasick = "1"

[dev-dependencies]
proptest = "1"
```

Minimal dependency: only `aho-corasick` (which pulls in `memchr`
transitively). No serde, no async, no allocator.

### 7.2 Token Enum (tokens.rs)

```rust
#[repr(u8)]
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Token {
    Space = 0,
    Colon = 1,
    Semicolon = 2,
    // ... exact same order as Go's iota
    End = 94,
}
```

The `#[repr(u8)]` guarantees byte layout matches Go's `Token byte`.
A compile-time assertion ensures the enum values stay in sync:

```rust
const _: () = assert!(Token::End as u8 == 67);
```

---

## 8. Build System Integration

Following the `dd_discovery` precedent:

1. Add `"pkg/logs/internal/decoder/preprocessor/rust"` to workspace
   `Cargo.toml` members
2. `cbindgen.toml` generates `include/dd_preprocessor_tokenizer.h`
3. Go files gated on `//go:build rust_preprocessor && cgo`
4. Build: `cargo build --release -p dd-preprocessor-tokenizer`
5. The `#cgo LDFLAGS` line points to `rust/target/release/`

### 8.1 CI

- Parity test: `cargo test -p dd-preprocessor-tokenizer` runs proptest
  comparing Rust output against a reference implementation
- Go integration: `dda inv test --targets=./pkg/logs/internal/decoder/preprocessor/
  --build-tags=rust_preprocessor` (requires Rust lib pre-built)

---

## 9. Correctness Strategy

### 9.1 Parity Property

The Rust tokenizer must produce **byte-identical output** to the Go
tokenizer for all inputs. This is verifiable:

```rust
// proptest: for any byte sequence, rust_tokenize(input) == go_reference(input)
proptest! {
    #[test]
    fn parity(input in proptest::collection::vec(any::<u8>(), 0..4096)) {
        let (rust_tokens, rust_indices) = rust_tokenize(&input);
        let (ref_tokens, ref_indices) = reference_tokenize(&input);
        assert_eq!(rust_tokens, ref_tokens);
        assert_eq!(rust_indices, ref_indices);
    }
}
```

The `reference_tokenize` is a pure-Rust reimplementation of Go's
algorithm (LUT + switch cascade, no AC) used only in tests.

### 9.2 Token Value Sync

A Go test verifies that Rust and Go Token values match:

```go
func TestTokenValues(t *testing.T) {
    // Call a Rust function that returns all token byte values
    // Compare against Go's iota values
}
```

---

## 10. Implementation Phases

### Phase 1: Rust Crate + Parity Tests (no Go integration)
- Implement `tokens.rs`, `keywords.rs`, `tokenizer.rs`
- Property-based parity tests against reference implementation
- Benchmark: `cargo bench` comparing Rust scan speed vs reference
- **Gate:** Rust scan must be <0.7ns/byte on 2048B inputs

### Phase 2: C API + Go Bridge
- Add `lib.rs` with `extern "C"` functions
- cbindgen setup, header generation
- Go bridge with `rust_preprocessor` build tag
- Go benchmark: compare `RustTokenizer` vs `Tokenizer`
- **Gate:** Rust pipeline must beat Go at 2048B (sampler window)

### Phase 3: Batched FFI (if Phase 2 is marginal)
- Implement `dd_tokenizer_tokenize_batch` (Section 6.4)
- Go side collects messages, calls once per batch
- Re-benchmark at batch sizes 4, 8, 16
- **Gate:** Batch=8 should show >1.5x speedup at 2048B

### Phase 4: Integration
- Wire `RustTokenizer` into preprocessor via config flag
- E2E test: verify auto-multiline and adaptive sampling produce
  identical results with both backends
- Merge behind `rust_preprocessor` build tag (off by default)

---

## 11. Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| FFI overhead eats all gains at 60B | Labeler path is no faster | Accept — labeler is not the hot path. Sampler (2048B) is. |
| AC automaton too large for L1 | Cache pressure slows scan | 60 patterns → ~30KB DFA, well within 64KB L1D. Monitor with `perf stat`. |
| Token value drift (Go adds new tokens) | Silent data corruption | Compile-time assertion + Go parity test in CI. |
| `aho-corasick` crate update breaks API | Build failure | Pin to `1.x`, `aho-corasick` has excellent semver discipline. |
| Cross-compilation (linux/arm64) | CI build failure | Rust cross targets are well-supported; `dd_discovery` already does this. |

---

## 12. Decision: Why Not the Alternatives

### Why not `phf` (perfect hash)?
Requires pre-extracted candidate strings. You'd need to identify word
boundaries first, extract substrings, allocate, then look up. AC finds
keywords inline during the scan — no extraction step, no allocation.

### Why not `daachorse` (double-array AC)?
Does not support case-insensitive matching. You'd need to lowercase the
entire input before matching, adding a pre-processing pass that defeats
the single-pass design.

### Why not `regex-automata` (raw DFA)?
Same `next_state()` interface as `aho-corasick` but without multi-pattern
matching built in. You'd need to build the composition yourself.
Engineering cost >> marginal performance gain.

### Why not `fst` (finite state transducers)?
Designed for millions of keys with shared prefixes. For 60 keywords,
construction overhead exceeds a simple AC automaton with no benefit.

### Why not move IsMatch to Rust too?
The sampler calls `IsMatch` up to 1000 times per message (linear scan).
Each call is 2-5ns with early exit. Moving this to Rust would require
passing all 1000 pattern entries (~1000 × ~50 bytes = 50KB) across the
FFI boundary per message. The FFI marshaling cost would dwarf the
computation. `IsMatch` stays in Go.
