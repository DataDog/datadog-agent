---
slug: clock-jump-no-extra-sampling
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# clock-jump-no-extra-sampling

## What Led to This

`AdaptiveSampler.Process` in `pkg/logs/internal/decoder/preprocessor/sampler.go`
uses `time.Now()` directly (via an injected `now func() time.Time`) to compute the
elapsed time since a pattern was last seen, and refills credits proportional to
elapsed time:

```go
// sampler.go:199-211
now := s.now()
// ...
elapsed := now.Sub(e.lastSeen).Seconds()
e.credits += elapsed * s.config.RateLimit
if e.credits > s.config.BurstSize {
    e.credits = s.config.BurstSize
}
e.lastSeen = now
```

The **upper bound** `e.credits > s.config.BurstSize` is applied, but there is **no
lower bound** on credits. If `now < e.lastSeen` (a backward clock jump delivered by
Antithesis), `elapsed` is negative, and `e.credits` is decremented by
`|elapsed| * RateLimit`. Credits can reach deeply negative values.

Once credits are negative, the entry will not emit any message until credits refill
back above 1.0. The refill rate is `RateLimit` credits per second, so a backward
jump of `D` seconds drives recovery time of `D + (initial_deficit / RateLimit)`
seconds — during which **every matching log is silently dropped** beyond what the
configured rate limit intends.

This is a **stealth correctness bug**: the system silently drops more logs than
configured and the only observable signal is the `logs_adaptive_sampler.dropped`
counter increasing without a corresponding traffic spike. The configuration guarantee
"drop at most `1 - RateLimit/incomingRate` fraction of logs" is violated.

## Files and Functions

- `pkg/logs/internal/decoder/preprocessor/sampler.go:199-211` — credit refill math
- `pkg/logs/internal/decoder/preprocessor/sampler.go:218` — `allow := e.credits >= 1.0` gate
- `pkg/logs/internal/decoder/preprocessor/sampler.go:125` — `now: time.Now` injection point

## Scenario

1. Pattern P is active and at steady-state (credits ≈ 0.3 at `RateLimit=2.0/s`).
2. Antithesis injects a backward clock jump of `-30s` (wall clock rewinds 30s).
3. Next log matching P: `elapsed = -30`, `credits += -30 * 2.0 = -60`, `credits → -59.7`.
4. At `RateLimit=2.0/s`, it takes 30 seconds of real time to recover to 0, then another
   0.5s to reach 1.0 — 30.5s of extra drops.
5. During that window, all P-matching logs are dropped, even if the configured rate limit
   is high (e.g. `RateLimit=100.0/s` means a 30s backward jump → 3000 credit deficit →
   30s of total blackout for pattern P).

## Why It Matters

The adaptive sampler is a user-configurable rate limiter. Users configure
`adaptive_sampling.rate_limit` to control the fraction of logs retained. A clock fault
silently over-drops logs without any user-visible error. Logs that were dropped cannot
be recovered — this is permanent data loss that violates the configured guarantee.

The bug is especially insidious because:
- It recovers on its own after real-time elapses.
- The recovery time is invisible; no metric tracks "credits below zero."
- It can appear to be a transient traffic anomaly rather than a system bug.

## What the Assertion Checks

An `Always` assertion placed at the credit refill point (or at the drop decision):

```go
// After credit refill:
assert.Always(e.credits >= -1.0,
    "adaptive sampler credits must never go deeply negative",
    map[string]any{"credits": e.credits, "elapsed": elapsed, "rateLimit": s.config.RateLimit})
```

A tighter bound (`-1.0` gives one extra drop slack for floating-point rounding) would
catch any backward clock jump that produces material extra drops.

Alternatively, a **workload-side** property: inject a backward clock jump, then measure
drop rate for pattern P over the following N seconds — it must not exceed the steady-state
configured rate by more than a small tolerance.

## Open Questions

- Does Antithesis's clock fault affect `time.Now()` inside the agent process, or only
  the system monotonic clock? Go's `time.Now()` on Linux uses a VDSO call that reads
  both wall time and a monotonic clock; if Antithesis manipulates the kernel clock (via
  `clock_adjtime` / `settimeofday` or container clock namespace), `time.Now()` will
  reflect the change. If Antithesis only injects faults at the hypervisor monotonic
  counter level without touching `CLOCK_REALTIME`, `time.Now()` wall subtraction may
  be unaffected. This is the critical unknown for whether the clock-jump scenario is
  reachable. `(needs human input)`

### Investigation Log

#### Is the `now func() time.Time` injection point in `AdaptiveSampler` overridable in production?

- Examined: `pkg/logs/internal/decoder/preprocessor/sampler.go:124` — the `now`
  field is declared as `now func() time.Time` inside `AdaptiveSampler` (unexported
  struct, exported field). `NewAdaptiveSampler` sets `now: time.Now` (line 134).
- Found: The field is not exported; the `sampler_test.go` tests override it directly
  via `s.now = func() time.Time { return t0 }` (e.g., line 114). This test-only
  pattern is idiomatic Go — the production code always uses `time.Now`, and there is
  no config or interface that allows runtime override.
- Conclusion: **resolved** — the injection point is test-only; production code always
  calls `time.Now()`. Antithesis must use OS-level clock faults to reproduce this,
  not the test injection mechanism.

#### Does `protect_important_logs=true` provide any protection against backward-jump extra drops?

- Examined: `sampler.go:189-198` — `Process()` checks `shouldSample()` first, then
  `isImportant()` check (early return before credit path), then the credit refill/check.
- Found: the `isImportant()` early return at line 194 exits *before* the credit refill
  code at line 199. For important logs, the credit bucket is never updated. Therefore,
  a backward clock jump that drives credits deeply negative does NOT affect important
  logs — they always pass through regardless.
- Conclusion: **resolved** (already stated in the file; confirmed by code reading).
  `protect_important_logs=true` is clock-invariant for important logs. Backward-jump
  damage is isolated to non-important patterns.
