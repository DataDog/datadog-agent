---
slug: sampling-reachable-under-load
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# sampling-reachable-under-load — Adaptive Sampler Drop Path Is Exercised Under Load

## What Led to This Property

A reachability property complementing `sampling-exact-count`. The adaptive
sampler's drop path (`tlmAdaptiveSamplerDropped.Inc()`) must be exercised during
fault-injection testing to confirm that:
1. The sampler is actually configured and active (not silently defaulting to
   `NoopSampler`).
2. The rate-limiting logic is reached with a non-trivial pattern table.
3. Antithesis's exploration focuses on the sampler's state machine.

## Code Paths Involved

**Drop path** — `pkg/logs/internal/decoder/preprocessor/sampler.go:238-241`:
```go
tlmAdaptiveSamplerDropped.Inc(s.source)
tlmAdaptiveSamplerBytesDropped.Add(float64(msg.RawDataLen), s.source)
return nil
```
This path is reached when `e.credits < 1.0` for a matched pattern.

**New pattern path** — `sampler.go:244-258`:
The first occurrence of a new pattern always passes through (BurstSize-1 credits).
The drop path can only be reached for a pattern that has been seen at least once
and whose credits are exhausted.

**Pattern eviction path** — `sampler.go:246-249`:
When the pattern table is full, the least-frequently-matched entry is evicted.
This path is reachable only when `MaxPatterns` distinct patterns have been seen.

**Protected path** — `sampler.go:194-198`: always reached for important logs
when `ProtectImportantLogs=true`.

## Failure Scenario (Reachability Failure)

1. The test topology configures `AdaptiveSampler` but with a `RateLimit` so
   high that credits are never exhausted during the test.
2. `tlmAdaptiveSamplerDropped` never increments.
3. The test passes vacuously — but the sampler is not actually exercising its
   core function.
4. A bug in the credit-depletion path goes undetected.

Antithesis reachability assertions (`Reachable`) prevent this: if the drop path
is never hit during an entire Antithesis run, the run fails with "assertion
never reached."

## Why It Matters

Without confirming the drop path is exercised, the sampling property tests
have no value. This is the canary for the entire adaptive sampling test suite.

## Workload Instrumentation

- Workload sends log lines at 100× the configured `RateLimit` for a sustained
  period.
- Assert reachability: `tlmAdaptiveSamplerDropped` counter increments at least
  once during the run.
- Assert reachability: pattern eviction path is reached (table fills to `MaxPatterns`).
- SUT-side: `Reachable` assertion at `sampler.go:238` — currently **missing**.
- SUT-side: `Reachable` assertion at `sampler.go:246-249` (eviction path) —
  currently **missing**.

## Open Questions

- Does the test topology enable the `AdaptiveSampler`? If the default is
  `NoopSampler`, the entire adaptive sampling property suite is unreachable
  without explicit topology configuration. `(needs human input)`
- What are the expected `MaxPatterns`, `RateLimit`, and `BurstSize` values in
  the test topology? These determine how long the workload needs to run before
  the drop path is reachable. `(needs human input)`

### Investigation Log

#### Does the test topology enable the AdaptiveSampler or does it default to NoopSampler?

- Examined: `pkg/logs/internal/decoder/preprocessor/sampler.go` (the
  `NoopSampler` is the default), `pkg/logs/internal/decoder/preprocessor_line_handler.go`
  (`newPreprocessorHandler` receives a `sampler preprocessor.Sampler` argument),
  and the decoder construction path.
- Found: `NoopSampler` is the struct defined as "the default implementation used
  until adaptive sampling logic is added" (sampler.go line 31). The `Sampler`
  interface is passed in from the decoder factory — there is no code in the repo
  that automatically enables `AdaptiveSampler` based on config. The switch from
  noop to adaptive must be explicit at construction time.
- Not found: any config key or init path that enables `AdaptiveSampler`
  automatically. No Antithesis topology files in the repo configure it.
- Conclusion: tagged `(needs human input)` — the test topology must explicitly
  construct and pass an `AdaptiveSampler` instance; without this, all adaptive
  sampler properties are vacuously unreachable.
