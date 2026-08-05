---
slug: adaptive-sampler-no-aliasing
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# adaptive-sampler-no-aliasing — Adaptive Sampler Credit Accounting Is Not Corrupted by Pattern Table Resort

## What Led to This Property

Bug `7687b846b2a` is cited in the SUT analysis §6 as "sampled_count aliasing on
pattern-table resort" — a real past bug. The fix is in
`pkg/logs/internal/decoder/preprocessor/sampler.go` with an explicit comment
warning about the aliasing hazard (lines 215-217). This makes it a regression
target: the fix is subtle and could be re-broken by a future refactor, or by
Antithesis finding a timing window the unit test didn't cover.

## Code Path Involved

`pkg/logs/internal/decoder/preprocessor/sampler.go:200-241`,
`AdaptiveSampler.Process()`:

```go
for i := range s.entries {
    e := &s.entries[i]            // e is a pointer into the slice
    if !IsMatch(e.tokens, tokens, s.config.MatchThreshold) {
        continue
    }
    // Refill credits, update timestamps
    elapsed := now.Sub(e.lastSeen).Seconds()
    e.credits += elapsed * s.config.RateLimit
    ...
    e.matchCount++

    // ALL mutations to e must complete before bubbling:
    // bubbling swaps entries by VALUE, so e (= &s.entries[i])
    // aliases a DIFFERENT entry after the first swap.    ← comment line 215
    allow := e.credits >= 1.0
    if allow {
        e.credits--
        if e.sampled > 0 {
            msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags,
                adaptiveSamplerSampledCountTag(e.sampled))
        }
        e.sampled = 0
    } else {
        e.sampled++
    }

    // Bubble the matched entry toward the front (by swapping values in the slice)
    for i > 0 && s.entries[i-1].matchCount < s.entries[i].matchCount {
        s.entries[i-1], s.entries[i] = s.entries[i], s.entries[i-1]
        i--
    }
    // After the first swap: s.entries[i] is now the OLD s.entries[i-1]
    // e still points to s.entries[original_i], which after swap is a DIFFERENT entry
    // Any write to e after this point would corrupt the wrong entry
    ...
}
```

The fix is explicit: all mutations (`credits`, `sampled`, `matchCount`) are done
before the bubble loop. If any mutation were accidentally moved below the bubble
loop (or if the bubble loop ran before all mutations), the write would corrupt a
different entry's data.

## Why This Is Still a Regression Target

The fix relies on the programmer maintaining the invariant "mutate before
bubble." This is not enforced by the type system or by any invariant check. A
future contributor who:
- Adds a new field to `samplerEntry` and forgets to update it before bubble
- Refactors the loop to check a condition after bubble and modifies e
- Adds a defer or a closure that captures `e` and modifies it asynchronously

...would silently break the invariant. The existing test (`sampler_test.go:264`)
covers the specific `sampled_count` field. A new field could alias in a new way.

The `AdaptiveSampler` is not concurrent (it runs in the single `forwardMessages`
goroutine per tailer), so this is not a goroutine-race bug. The aliasing is
intra-goroutine pointer aliasing through slice element swaps — a subtle value
semantics trap.

## Observable Effect of Aliasing

If `sampled_count` is written to the wrong entry after a bubble:
- Entry A (hot pattern, bubbled to front): loses its `sampled_count` reset
  → on next pass, `sampled_count` is incremented again → inflated drop metric
  attached to a future log line of a different pattern.
- Entry B (the entry that was at position `i-1`): gets `sampled_count = 0`
  incorrectly → drops that occurred for B are not reported → the
  `adaptive_sampler_sampled_count:<N>` tag on the next B log is wrong (0
  instead of real count).

From the user's perspective: the dropped-count tag on emitted logs is incorrect,
misleading observability about sampling behavior.

## Antithesis Angle

This is not a classic concurrency race — it is a pure correctness property of
a sequential algorithm. Antithesis adds value by:
1. Exploring many interleaving patterns (even within a single goroutine, via
   CPU scheduling and branch exploration) to drive the sampler through many
   diverse pattern table configurations.
2. Specifically driving states where: the matched entry is at position 2+
   (needs multiple bubble swaps), `sampled_count > 0` at match time, and
   the entry's `matchCount` is just barely less than the entry above it (one
   increment causes a swap).
3. Verifying the `sampled_count` tag on emitted log lines exactly matches the
   count of dropped messages since the last emission for that specific pattern.

## SUT-Side Instrumentation (all missing)

- `Always("sampler-entry-matchcount-monotonically-increasing")` — after each
  `Process()` call, verify that for the matched entry, `matchCount` equals its
  value before the call plus 1. This catches a write to the wrong entry that
  would leave the matched entry's count unchanged.
- `Always("sampler-sampled-count-resets-on-emit")` — after an `allow=true`
  path, verify `e.sampled == 0` for the matched entry (not the aliased one).
  Requires reading the entry's state after the bubble loop.
- Workload: emit patterns A and B alternately in a ratio that causes frequent
   bubbling; after every emitted log containing `adaptive_sampler_sampled_count`,
   verify the count equals the number of drops since the previous emission for
   that pattern.

## Open Questions

- Is the test at `sampler_test.go:264` sufficient to catch all forms of
  aliasing, or only the specific `sampled_count` case? `(partial: test
  confirmed to cover sampled field explicitly; credits and matchCount fields
  are only indirectly verified via behavior; a new field added after the bubble
  loop would not be caught)`
- What is the `BurstSize` default? With a small burst, pattern matches
  quickly hit rate-limit and the sampled_count accumulates faster.
  `(partial: no repo-wide default; config is caller-defined per
  AdaptiveSamplerConfig — needs topology/config documentation)`

### Investigation Log

#### Is `AdaptiveSampler` ever called concurrently?

- Examined: `pkg/logs/internal/decoder/preprocessor_line_handler.go` (the
  `preprocessorLineHandler` adapter), `pkg/logs/internal/decoder/preprocessor/preprocessor.go`
  (the `Preprocessor.Process()` method), and `pkg/logs/internal/decoder/preprocessor/sampler.go`.
- Found: `preprocessorLineHandler.process()` is the sole entry point into
  `Preprocessor.Process()`, which calls `s.sampler.Process()`. The
  `preprocessorLineHandler` is invoked by the decoder's `LineHandler` interface
  from a single goroutine per decoder. There is no mutex on `AdaptiveSampler`
  fields, and none is needed — the single-goroutine design is intentional.
  `PatternTable` (`pattern_table.go`) is a separate struct with its own mutex,
  used for auto-multiline detection; it is not the same code as `AdaptiveSampler`.
- Conclusion: **resolved** — `Process()` is single-goroutine. Concurrent access
  to `entries` cannot occur in the current architecture. The partial tag is upgraded:
  this question is fully answered.

#### Is the test at `sampler_test.go:264` sufficient to catch all aliasing forms?

- Examined: `TestAdaptiveSampler_BubblingAliasesSampledCount` (line 270-303).
- Found: The test creates two patterns with equal `matchCount`, drops one to
  make it bubble, then verifies that the `adaptive_sampler_sampled_count` tag
  on the next emitted message matches the drop count. This specifically tests
  the `sampled` field. The test does NOT independently verify that `credits`
  and `matchCount` of the correctly-identified entry (post-bubble) are
  consistent — it only observes the tag value on the next emission. If a new
  field were added to `samplerEntry` and its mutation were accidentally placed
  after the bubble loop, the existing test would not catch it.
- Not found: any test that explicitly reads back `credits` or `matchCount` of
  the correct entry after a bubble event.
- Conclusion: tagged `(partial: ...)` — the `sampled` aliasing case is covered;
  other fields are not independently verified after bubble.
</content>
