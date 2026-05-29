---
slug: sampling-exact-count
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# sampling-exact-count — Adaptive Sampler Transmits At Most N Low-Value Logs Per Interval

## What Led to This Property

SUT guarantee S6 (from the owner's design doc): the adaptive sampler is a
credit-based rate limiter that should transmit at most `RateLimit` low-value
logs per second per pattern. Violation: more than `RateLimit * T` logs of a
pattern reaching the intake in any interval of length T.

The SUT analysis notes the "exact count not guaranteed for new/bursty patterns"
due to the initial `BurstSize` credit allocation — this is a known approximation,
not a bug.

## Code Paths Involved

**Credit-bucket logic** — `pkg/logs/internal/decoder/preprocessor/sampler.go:188-260`

`Process()` is called per message with the message and its tokenized pattern.
For each pattern entry (`samplerEntry`):
- Credits are refilled based on elapsed time: `e.credits += elapsed * s.config.RateLimit`
- Credits are capped at `BurstSize`.
- If `e.credits >= 1.0`: allow the message, decrement credits.
- If `e.credits < 1.0`: drop the message.

**New pattern handling** (line 244-258):
- First occurrence gets `BurstSize - 1` credits, not `BurstSize`.
- So the burst for a new pattern is bounded by `BurstSize`.

**Clock hazard** — The `now` function is `time.Now` (line 134). If the system
clock jumps backward (Antithesis clock fault), `elapsed` becomes negative →
`e.credits` decreases without any message arriving → rate is more restrictive
than configured. If the clock jumps forward, `e.credits` gets a windfall refill
and more messages than expected are allowed through.

**Pattern table sorting bug** (`7687b846b2a` from bug history):
- `entries` is sorted by descending `matchCount`. After a match, the entry
  bubbles toward the front (lines 230-233).
- The aliasing bug: after `s.entries[i-1], s.entries[i] = s.entries[i], s.entries[i-1]`
  swap, `e` (which was `&s.entries[i]`) now points to the entry at index `i`,
  which has changed value. The fix is reading from `s.entries[i]` after the
  swap, not from `e`. The current code (`i--` after the swap) correctly reindexes.
  However, under concurrent stress (multiple goroutines calling `Process`), the
  `entries` slice is not protected by a mutex — this is a data race risk if the
  same `AdaptiveSampler` is called from multiple goroutines.

## Failure Scenario

**Clock jump forward:**
1. System clock jumps forward by Δ seconds (Antithesis clock fault).
2. All pattern entries receive `Δ * RateLimit` bonus credits.
3. A burst of messages is allowed through, violating the rate limit.
4. Fakeintake sees more than `RateLimit * T` messages from the pattern.

**Pattern eviction and re-add:**
1. Pattern A is evicted (LRU when table is full).
2. Pattern A reappears and is treated as a new pattern with `BurstSize - 1` credits.
3. This is a deliberate design choice, not a bug — but it means a bursty pattern
   that causes frequent evictions can repeatedly exceed the per-interval rate.

**Concurrent Process() calls (potential data race):**
- If two decoder goroutines share one `AdaptiveSampler` instance (need to
  verify whether this occurs in the current architecture), the `entries` slice
  can be corrupted.

## Why It Matters

The adaptive sampler is a billing and data-quality control. If it allows
significantly more than the configured rate, customers pay for unexpected data
volume. If it drops significantly more (due to clock regression), customers
lose visibility into their services. The owner's design doc treats the
per-pattern rate as the key correctness invariant.

## Workload Instrumentation

- Workload sends log lines matching a known low-value pattern at a rate of
  10× `RateLimit` per second.
- After each interval T, the fakeintake counts received messages matching the
  pattern.
- Assertion: received count ≤ `ceil(RateLimit * T + BurstSize)` (allowing for
  the initial burst).
- SUT-side: a `Sometimes` assertion at the sampler's drop path confirming the
  drop counter increments when rate is exceeded — currently **missing**.

## Open Questions

- Is there a warmup period before sampling engages? If the sampler starts with
  an empty table, all new patterns get burst credits — the first `BurstSize` logs
  from each pattern are always allowed, regardless of rate. Is this the intended
  behavior per the owner's design doc? `(needs human input)`
- What is the `BurstSize` value in the test topology? If very large (e.g., 1000),
  the steady-state rate limit is effectively unenforced for short test runs.
  `(needs human input)`
- Does the Antithesis topology enable adaptive sampling? If `NoopSampler` is used
  (the default), this property is vacuously satisfied and has no value. The
  topology must configure `AdaptiveSampler`. `(needs human input)`

### Investigation Log

#### Is `AdaptiveSampler.Process()` called from a single goroutine per source, or can multiple goroutines invoke it concurrently?

- Examined: `pkg/logs/internal/decoder/preprocessor_line_handler.go`,
  `pkg/logs/internal/decoder/preprocessor/preprocessor.go`,
  `pkg/logs/internal/decoder/preprocessor/sampler.go`.
- Found: `preprocessorLineHandler.process()` is the only call site for
  `preprocessor.Preprocessor.Process()`, which in turn calls
  `s.sampler.Process()`. `preprocessorLineHandler` is wired into the decoder's
  `LineHandler` interface and invoked by `lineParser`'s single goroutine (the
  decoder reads lines sequentially). There is no fan-out path that calls
  `process()` concurrently. The `AdaptiveSampler` struct has no mutex — this is
  intentional: it is single-goroutine by design.
- Not found: any path that calls `Process()` from a second goroutine.
- Conclusion: **resolved** — `Process()` is single-goroutine per source. No
  concurrent access to `entries`. The data race concern is unfounded in the
  current architecture.
