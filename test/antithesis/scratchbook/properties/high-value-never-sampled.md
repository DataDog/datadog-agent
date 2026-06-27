---
slug: high-value-never-sampled
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# high-value-never-sampled — High-Value Logs Are Never Dropped by the Adaptive Sampler

## What Led to This Property

SUT guarantee S4: when `ProtectImportantLogs=true`, log lines identified as
"high-value" (containing severity keywords like FATAL, ERROR, PANIC, etc.) must
bypass the credit-bucket rate limiter and always be delivered. This is the
strongest safety property of the adaptive sampler.

The owner's design doc (external reference #1) treats this as a hard invariant,
not an approximation.

## Code Paths Involved

**Protection check** — `pkg/logs/internal/decoder/preprocessor/sampler.go:194-198`
```go
if s.config.ProtectImportantLogs && isImportant(tokens) {
    tlmAdaptiveSamplerKept.Inc(s.source)
    tlmAdaptiveSamplerProtected.Inc(s.source)
    return msg
}
```
This fires *before* any credit check, so important logs never consume or check
credits.

**`isImportant()` token check** (lines 140-149):
```go
func isImportant(tokens []Token) bool {
    for _, t := range tokens {
        switch t {
        case Fatal, Error, Panic, Alert, Severe, Critical, Emergency, Warn,
            Exception, Crash, Failure, Deadlock, Timeout:
            return true
        }
    }
    return false
}
```
The protection depends entirely on the tokenizer correctly identifying severity
keywords as the relevant `Token` values.

**Tokenizer path** — `pkg/logs/internal/decoder/preprocessor/tokenizer.go`
(not read in detail). The tokenizer converts raw log content to a `[]Token`
slice. If the tokenizer fails to recognize a severity keyword (e.g., due to
encoding, case mismatch, or unexpected log format), `isImportant()` returns
false and the log is subject to sampling.

**Guard ordering:**
- `shouldSample()` (line 177-185) is called *before* `isImportant()`.
- If the message is excluded via `Exclude` filters, `shouldSample` returns
  false → message bypasses sampling entirely (passes through without rate
  limiting), which is also "protected."
- If the message matches an `Include` filter only, it goes through the credit
  check regardless of severity.

**The critical ordering:** `shouldSample()` is evaluated first. If false
(excluded), the message passes. If true (included), then `isImportant()` is
checked. So `ProtectImportantLogs` applies to all included messages, including
those that would otherwise be rate-limited.

**Condition on config:**
- `ProtectImportantLogs` defaults to false. **The property is only exercised
  when this flag is enabled in the test topology.**

## Failure Scenario

**Tokenizer misclassification under CPU fault:**
1. CPU is throttled by Antithesis.
2. Tokenizer allocates tokens but a concurrent GC pause corrupts (hypothetically)
   the token slice — not a realistic Go GC scenario, but thread-pause faults can
   expose aliasing bugs.
3. `isImportant()` scans a malformed token slice and misses the severity keyword.
4. A high-value log containing "FATAL" is dropped by the credit bucket.

**More realistic scenario — pattern-table eviction race:**
1. An important log has a new pattern. It is correctly passed through on first
   occurrence (new-pattern path, line 251-258, always allows first instance).
2. On second occurrence, the pattern *is* in the table. If credits are exhausted
   AND `isImportant()` returns false (tokenizer miss), the log is dropped.

**Clock regression:**
1. Clock jumps backward by Δ seconds.
2. `elapsed = now.Sub(e.lastSeen).Seconds()` is negative.
3. `e.credits` decreases without any dropped messages.
4. Next important log arrives, `isImportant()` returns true → still protected.
   Clock regression does not break this property because the `isImportant()`
   check precedes the credit check.

The clock regression scenario actually confirms the protection is clock-invariant
for important logs. The risk is exclusively in the tokenizer path.

## Why It Matters

If high-value logs (FATAL, ERROR, PANIC) are silently dropped, the user loses
visibility into exactly the incidents they most need to detect. This is the
adaptive sampler's primary safety contract and its failure would make the feature
unusable in production.

## Workload Instrumentation

- Workload sends a mix of:
  - High-rate "noise" log lines matching a pattern (low-value, no severity words).
  - High-value log lines with "FATAL: service crashed" at any rate.
- The sampler should be configured to rate-limit noise but protect important logs.
- Fakeintake assertion: every high-value log line sent by the workload (identified
  by sequence number embedded in the FATAL line) is received exactly at least once.
- SUT-side: a `Sometimes` assertion at `tlmAdaptiveSamplerProtected.Inc()` confirms
  the protection path is reached at least once per run — currently **missing**.

## Open Questions

- Does `ProtectImportantLogs` apply when the sampler's `Include`/`Exclude` filters
  are also configured? The code says yes for `Include`-matched messages, but
  messages matching an `Exclude` filter bypass even the protection check (they go
  straight to allow via `shouldSample` returning false). Confirm this is the
  intended priority ordering — exclude-matched messages still pass, so there is no
  data loss, but the protection telemetry counter (`tlmAdaptiveSamplerProtected`)
  does not increment for them. `(needs human input)`

### Investigation Log

#### Is the tokenizer case-sensitive?

- Examined: `pkg/logs/internal/decoder/preprocessor/tokenizer.go` — specifically
  `makeToUpperLookup()` (lines 30-38), `tokenize()` loop (line 204:
  `t.strBuf[t.strLen] = toUpperLookup[char]`), and `getSpecialLongToken()`.
- Found: The tokenizer normalizes all letter characters to uppercase before
  buffering them in `strBuf` for special-token matching. `toUpperLookup` maps
  `[a-z]` → `[A-Z]` and is identity for everything else. `getSpecialLongToken`
  only matches uppercase strings (e.g., `"FATAL"`, `"ERROR"`). Because input is
  uppercased before the comparison, the tokenizer is effectively **case-insensitive**
  for severity keywords. A log containing `"fatal"` or `"Fatal"` or `"FATAL"` all
  produce the `Fatal` token and `isImportant()` returns true.
- Not found: any code path that skips the uppercase normalization for letter runs.
- Conclusion: **resolved** — tokenizer is case-insensitive for severity keywords.
  JSON-formatted logs like `"level":"fatal"` are correctly identified as important
  as long as the word `fatal` appears as a contiguous letter run (no intervening
  non-letter bytes from JSON framing affect the run classification of `fatal` itself).

#### Is there a test for `ProtectImportantLogs=true` + zero remaining credits?

- Examined: `pkg/logs/internal/decoder/preprocessor/sampler_test.go` —
  `TestAdaptiveSampler_ImportantLogBypassesRateLimit` (line 326).
- Found: The test explicitly configures `burst=1.0, rate=0` (no credit refill),
  sends the first message (consumes burst), then sends additional important-token
  messages that would normally be dropped. All pass through. This directly covers
  the "zero remaining credits + important tokens" case.
- Conclusion: **resolved** — there is a test. The behavior is confirmed correct.
