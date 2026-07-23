# Evidence: bounded-memory-under-backpressure

## Summary

Under sustained backpressure (intake unreachable), the logs agent should not
grow its memory without bound. Two distinct mechanisms threaten this:

1. **Channel-bounded in-flight data:** all pipeline channels are bounded (100 /
   100 / 1 / 10 / 100 per pipeline). Once full, backpressure propagates to the
   tailer. This is the *intended* bound.

2. **zstd C-heap leak (commit `0d9dfc76f46`):** when `resetBatch()` is called
   on an error path that never reached `sendMessages()`, a `ZSTD_CCtx` allocated
   on the C heap is orphaned. The Go GC cannot observe it. Under repeated
   encode errors, this accumulates to multi-GiB RSS. This bug was fixed by adding
   an explicit `b.compressor.Close()` at the top of `resetBatch()`.

## Key code

**`comp/logs-library/sender/batch.go:73-88`** — `resetBatch()` (fix in place):
```go
func (b *batch) resetBatch() {
    // Free C-side resources (e.g. ZSTD_CCtx) before replacing the compressor;
    // error paths skip sendMessages so Close() would never be called otherwise.
    if b.compressor != nil {
        b.compressor.Close()  // <-- the fix; prevents C-heap leak
    }
    b.buffer.Clear()
    ...
    compressor := b.compression.NewStreamCompressor(&encodedPayload)
    ...
}
```

**`comp/logs-library/sender/batch.go:90-122`** — `processMessage()`: calls
`resetBatch()` on encoding errors via two paths (lines 97 and 112). Both
correctly hit the fixed `resetBatch()`.

**`pkg/util/compression/impl-zstd/zstd_strategy.go:56`** — CGO zstd backend
uses `github.com/DataDog/zstd` (CGO). The `ZSTD_CCtx` is allocated on the C
heap. The `NewStreamCompressor` returns an `io.WriteCloser` — the `Close()`
call releases the C-side context.

**`pkg/util/compression/impl-zstd-nocgo/zstd_nocgo_strategy.go`** — pure-Go
zstd (`klauspost/compress/zstd`). The encoder is reused via `Reset()` not
`Close()`. The CGO-specific leak only applies to the CGO backend. In nocgo mode,
the fixed `Close()` call is a no-op (or may trigger an `Encoder.Close()` on the
klauspost encoder, which is safe).

## Backpressure channel bounds

All channels are bounded (sut-analysis.md §1):
- `Pipeline.InputChan` / processor input: 100
- strategy input: 100
- sender per-worker queue: 1 (HTTP mode)
- `DestinationSender.input`: 10
- auditor input: 100

Under full backpressure, the maximum number of log messages in flight per
pipeline is bounded by these constants. Memory from *messages* is bounded.
Memory from *compressor state* depends on `resetBatch()` correctness.

## Why it matters

- Agent OOM during prolonged outage is a user-visible failure (sut-analysis.md §12.3)
- The Go GC cannot detect C-heap growth; RSS bloat is invisible to GC pressure
- Antithesis soak tests with a sustained network partition are the right
  environment: repeated batch encode → reset → new compressor cycles, with no
  successful sends to trigger `sendMessages()`
- The fix (`compressor.Close()` in `resetBatch()`) is present in the current
  codebase; the regression target is: does *any new code path* bypass
  `resetBatch()` and orphan a compressor?

## Assertion design

**Workload-side (`Always`):** Periodically sample agent process RSS (via
`/proc/<pid>/status` or the agent's own expvar endpoint). Assert
`rss < 2 * baseline_rss`. Baseline is measured in the first 60 seconds before
fault injection. This catches both the original leak and any future ones.

**SUT-side (`Reachable`):** At the top of `resetBatch()`, add a `Reachable`
assertion when `b.compressor != nil` and compression kind is "zstd". Confirms
the fix path is exercised during the run.

**SUT-side (`Sometimes`):** `Sometimes(compressor.Close() was called on error
path)` — a message-level assertion in `processMessage()` at the `resetBatch()`
call on the error path (lines 97-98) would confirm the error path is reached.

## Open Questions

- Does the current test topology use CGO zstd or nocgo zstd? The leak is
  CGO-specific. If nocgo is used, the OOM scenario cannot be triggered. `(needs human input)`
- Under what conditions does `addMessage` / `Serializer.Serialize()` return an
  error? Without a synthetic failure injector (e.g., a patched compressor that
  returns errors), the error path may be unreachable in normal operation.
  `(needs human input)` — what error scenarios were observed in production?
- Is there a way to inject compressor errors at the Antithesis level without
  modifying the compressor itself?

### Investigation Log

#### Are the 3 `resetBatch()` paths CGO-zstd-specific? Is the nocgo `klauspost` `Close()` idempotent?

- Examined: `comp/logs-library/sender/batch.go:73-192` (all three `resetBatch()` call sites), `pkg/util/compression/impl-zstd-nocgo/zstd_nocgo_strategy.go` (full file), `pkg/util/compression/compression.go` (`StreamCompressor` interface).
- Found: All three `resetBatch()` paths (encoding error in `addMessage`, encoding error on retry, and `defer resetBatch()` in `sendMessages()`) are independent of CGO vs nocgo — all compressor backends implement the same `StreamCompressor` interface (`io.WriteCloser`). The `resetBatch()` nil-guard fix (`if b.compressor != nil { b.compressor.Close() }`) applies to all backends. For nocgo (`impl-zstd-nocgo`), `NewStreamCompressor` returns a `*zstd.Encoder` (klauspost). `zstd.Encoder.Close()` is NOT a no-op — it writes the zstd end frame to the output buffer. It is NOT idempotent (calling twice would write two end frames, corrupting the stream). However, the double-close is prevented by the nil guard: in `sendMessages()`, `b.compressor = nil` is set immediately after `b.compressor.Close()` fires (line 167), so the `defer b.resetBatch()` at the top of `sendMessages()` sees `b.compressor == nil` and skips the `Close()`. For the error paths (lines 97 and 111-112), `b.compressor != nil` when `resetBatch()` is called, so `Close()` fires — this releases the klauspost encoder's pending state and flushes any buffered data to the encodedPayload buffer (the flushed data is discarded when `b.encodedPayload` is replaced). The CGO-specific behavior is only the severity of the leak — CGO allocates `ZSTD_CCtx` on the C heap (invisible to Go GC), whereas nocgo's encoder is GC-visible. The fix is necessary for both, but only the CGO case can lead to unbounded RSS growth invisible to the GC.
- Not found: any case where `Close()` is called twice on the same `*zstd.Encoder` instance; the nil guard prevents this throughout.
- Conclusion: resolved. All three `resetBatch()` paths apply to both CGO and nocgo. The nocgo `klauspost` `Close()` is NOT a no-op and NOT idempotent, but double-close is prevented by the nil guard in `sendMessages()`. The OOM scenario (unbounded C-heap growth) is CGO-specific. Test topology must use CGO zstd to exercise the original bug scenario; nocgo is safe from unbounded growth due to GC visibility.

## Merged-in evidence (from no-zstd-cctx-leak)

The secondary file focused specifically on the **zstd `ZSTD_CCtx` leak** as a
regression target and provided additional code detail on all three `resetBatch()`
call paths:

**Three code paths calling `resetBatch()`:**
1. `processMessage()` line 97 — encoding error in `addMessage`
2. `processMessage()` line 112 — encoding error on retry
3. `sendMessages()` via `defer b.resetBatch()` (normal path; `b.compressor` is
   nil'd before defer fires to prevent double-close)

The fix (`compressor.Close()` at top of `resetBatch()`) correctly handles all
three: in paths 1 and 2, `b.compressor != nil` so `Close()` is called; in path
3, `b.compressor` is nil'd before the defer fires, so the nil guard prevents
double-close.

**`sendMessages()` nil guard** (`batch.go:163-192`):
```go
func (b *batch) sendMessages(...) {
    defer b.resetBatch()
    err := b.compressor.Close()
    b.compressor = nil  // prevent double-free from defer resetBatch
    ...
}
```

**CGO vs. nocgo distinction:**
- CGO zstd (`impl-zstd/`): `ZSTD_CCtx` on C heap; leak invisible to Go GC;
  `Close()` calls `ZSTD_freeCCtx()`.
- nocgo zstd (`impl-zstd-nocgo/`): pure Go encoder, GC-visible; `Close()`
  releases the encoder (less severe if leaked, but still applies).

**Regression surface** (from secondary): any new error path that creates a
compressor via `NewStreamCompressor()` and then calls `resetBatch()` without
the nil guard would re-introduce the leak.

**Additional assertion (from secondary):** `Unreachable` for the orphan state
is not directly observable without a proxy; the `Reachable` positive assertion
at `b.compressor.Close()` inside `resetBatch()` is the practical instrumentation
approach.
