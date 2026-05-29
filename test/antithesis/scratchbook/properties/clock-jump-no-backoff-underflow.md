---
slug: clock-jump-no-backoff-underflow
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# clock-jump-no-backoff-underflow

## What Led to This

The HTTP destination's `sendAndRetry` loop computes the backoff deadline as:

```go
// comp/logs-library/client/http/destination.go:268-272
backoffDuration := d.backoff.GetBackoffDuration(nbErrors)
blockedUntil := time.Now().Add(backoffDuration)
if blockedUntil.After(time.Now()) {
    log.Warnf(...)
    d.waitForBackoff(blockedUntil)
```

`waitForBackoff` uses `context.WithDeadline`:

```go
// destination.go:553-557
func (d *Destination) waitForBackoff(blockedUntil time.Time) {
    ctx, cancel := context.WithDeadline(d.destinationsContext.Context(), blockedUntil)
    defer cancel()
    <-ctx.Done()
}
```

There are **two distinct clock-jump failure modes** here:

### Forward jump: bypasses the backoff sleep

If Antithesis jumps the clock forward by more than `backoffDuration` between the two
`time.Now()` calls (`blockedUntil` computation and the `After` check), the `After` check
returns `false` and the backoff sleep is **skipped entirely**. The destination immediately
retries, potentially hammering the intake with rapid retries at a rate that would normally
be gated by the exponential backoff. This is not dangerous to the agent itself but could
cause thundering-herd against the intake if multiple destinations behave this way
simultaneously.

### Forward jump: collapses the context deadline to now

If the clock jumps forward after `blockedUntil` is computed but before
`context.WithDeadline`, the deadline is in the past. `ctx.Done()` fires immediately,
`waitForBackoff` returns immediately, and the retry loop bypasses backoff. Same effect as
above but the mechanism differs.

### Forward jump: EWMA window resamples too frequently

The HTTP worker pool EWMA in `comp/logs-library/client/http/worker_pool.go:157`:

```go
if time.Since(l.virtualLatencyLastSample) >= l.ewmaSampleInterval {
    // ...
    l.virtualLatencyLastSample = time.Now()
```

`time.Since` uses `time.Now()` minus the stored timestamp. A forward jump makes
`time.Since(l.virtualLatencyLastSample)` huge, firing the EWMA update with potentially
zero samples in the window (`windowSum=0, samples=0` because no sends completed during
the zero-duration window). The branch at lines 167-168 (`if samples > 0`) guards the
avgLatency computation but not the `virtualLatencyLastSample` reset, which still fires.
Net effect: the virtual latency EWMA is reset to its previous value with a meaningless
sample interval, and the worker count may not adjust correctly, causing under- or
over-parallelism in the sender pool.

### Backward jump: never-ending backoff

If the clock jumps backward after `blockedUntil` is set but during `waitForBackoff`, the
`context.WithDeadline` deadline is now far in the future relative to the new "current
time". The send goroutine blocks until the deadline passes in real wall time — effectively
a backoff that is `|jump| + backoffDuration` instead of `backoffDuration`. During this
extended block, the pipeline queue fills and backpressure propagates to the tailer.
If the extended backoff coincides with a file rotation, the `closeTimeout` (60s) may
expire, causing byte loss.

## Files and Functions

- `comp/logs-library/client/http/destination.go:268-272` — backoff deadline computation
- `comp/logs-library/client/http/destination.go:553-557` — `waitForBackoff`
- `comp/logs-library/client/http/worker_pool.go:157-169` — EWMA window check and reset
- `pkg/util/backoff/backoff.go:64-79` — `GetBackoffDuration` (no clock dependency; pure math)
- `comp/logs-library/client/tcp/connection_manager.go:187-198` — TCP backoff also uses `rand.Intn` but delegates sleep to `context.WithTimeout`

## Why It Matters

The backoff policy is the system's primary defense against a wedged or unavailable intake.
If a clock jump can bypass backoff, the agent may retry at the maximum possible rate
against an already-stressed intake. Under Antithesis fault injection, it's easy to create
a scenario where:
- The intake is fault-injected to return 503s.
- A simultaneous clock forward jump bypasses backoff on the agent side.
- The agent hammers the intake, exhausting connection pools.

The EWMA worker-pool issue is subtler: over-parallelism under good conditions degrades
per-request latency and increases CPU load; under-parallelism under good conditions
leaves throughput on the table. Neither triggers an alarm — they manifest as unexplained
throughput variation.

## What the Assertion Checks

**Backoff bypass**: an `AlwaysOrUnreachable` assertion at the retry loop entry:
```go
// In sendAndRetry, before waitForBackoff:
assert.AlwaysOrUnreachable(backoffDuration <= 0 || blockedUntil.After(time.Now()),
    "backoff must not be bypassed due to clock skew",
    map[string]any{"backoffDuration": backoffDuration, "nbErrors": nbErrors})
```

**EWMA correctness**: a `Sometimes` assertion that the worker pool has been resized
at least once during a run (proving the EWMA path was exercised and didn't freeze):
```go
assert.Sometimes(l.inUseWorkers != l.minWorkers,
    "worker pool must scale up from minimum at least once under load",
    map[string]any{"inUseWorkers": l.inUseWorkers, "minWorkers": l.minWorkers})
```

## Open Questions

- Does Antithesis's clock fault affect `context.WithDeadline`'s internal timer? The Go
  runtime uses a monotonic clock for timers on Linux. If the Antithesis clock fault only
  adjusts the wall clock but not the monotonic clock, `context.WithDeadline` may be
  immune to the forward-jump bypass while `blockedUntil.After(time.Now())` (which does
  a wall-clock comparison when the `time.Time` value contains a wall clock reading) may
  still be affected. `(needs human input)`

### Investigation Log

#### Is the EWMA worker pool actually a production path (min != max workers by default)?

- Examined: `comp/logs-library/pipeline/provider.go:169-196` (the sender creation
  logic), `comp/logs-library/client/http/worker_pool.go:126-129` (early-exit guard),
  `pkg/config/setup/config.go:90-91` (`DefaultBatchMaxConcurrentSend = 0`).
- Found: In the non-legacy, non-serverless HTTP path (the default), the provider sets:
  - `minSenderConcurrency = numberOfPipelines` (e.g., 4 for 4 pipelines)
  - `maxSenderConcurrency = numberOfPipelines * maxConcurrencyPerPipeline` (= 4 × 10 = 40)
  Since `min != max`, the EWMA `resizeUnsafe()` path runs on every call — the early-exit
  guard at `worker_pool.go:126-129` (`if l.maxWorkers == l.minWorkers { return }`) is
  NOT triggered. The EWMA path is **live code in the default production configuration**.
  The only case where it is bypassed is when `BatchMaxConcurrentSend` is explicitly set
  (which forces `minSenderConcurrency == maxSenderConcurrency`), or for legacy/serverless
  mode.
- Conclusion: **resolved** — EWMA worker pool is the default production path when
  `batch_max_concurrent_send` is 0 (the default). The clock-jump EWMA scenario is
  reachable without any special topology configuration.
