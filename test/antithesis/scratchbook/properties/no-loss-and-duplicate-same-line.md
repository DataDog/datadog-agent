---
slug: no-loss-and-duplicate-same-line
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# no-loss-and-duplicate-same-line

## The Frame Problem

Other analyst focuses treat "at-least-once" and "no-loss" as separate properties.
This property **questions the frame**: can a specific interleaving of faults produce
a line that is **both lost AND duplicated**? That sounds paradoxical, but it is
achievable through multiple simultaneous failure modes affecting different segments
of the same log file.

The scenario requires at least two overlapping failure mechanisms:

1. **Loss mechanism**: a rotation `closeTimeout` expiry that causes the old tailer to
   stop forward before draining all buffered messages.
2. **Duplication mechanism**: the auditor's 1-second flush window means the last-committed
   offset is stale; on restart, the new tailer replays from that stale offset.

These mechanisms do not affect the same byte range if they operate on non-overlapping
file regions. But if the old tailer loses bytes **at the tail** of the rotated file
(bytes in the `closeTimeout` buffer) while the registry records an offset in the
**body** of that file (the last successfully flushed offset, which is before the lost
bytes), then:

- Bytes in `[stale_offset, lost_byte_start)` will be **duplicated** (the new tailer
  replays them from the stale offset on restart).
- Bytes in `[lost_byte_start, file_end)` are **lost** (the close timeout discarded them).

The same logical restart event produces **both a loss and a duplicate** — for different
byte ranges of the same file. From the user's perspective, a single log session has a
gap (loss) surrounded by repeated content (duplicate).

## Second Scenario: 4xx + Restart Interleaving

A distinct "both" scenario arises from the 4xx permanent drop combined with a crash:

1. Sender sends payload P1 to intake.
2. Intake returns 400 (permanent error). The destination drops P1 and sends it to the
   auditor channel (`output <- payload` at destination.go:318, even on error).
3. Auditor advances the offset past P1. Registry now reflects "P1 was delivered."
4. **Before the auditor flushes to disk (within the 1-second window)**, the agent crashes.
5. Registry on disk still shows the pre-P1 offset.
6. On restart, the tailer replays P1.
7. Intake returns 200 for the replayed P1 → **P1 is now delivered even though the first
   attempt was permanently dropped**.

From the user's perspective: P1 was "lost" (dropped with 400) and then "found" (replayed
successfully). The logs that were permanently dropped appear after a restart. The
**permanent** in "permanent drop" is only permanent until the next crash.

This undermines the semantics of 4xx permanent drops — if users are told "400 means that
log is gone forever," the restart-replay behavior is surprising and may cause compliance
issues (e.g., if the 400 was due to a log containing PII that should have been rejected).

## Third Scenario: Multiline Flush × Rotation × Truncation

A multiline aggregation in progress during file rotation can produce:

1. The old tailer has a partial multiline buffer (lines L1, L2 accumulated, waiting for L3
   to complete the pattern). L3 arrives in the rotated file's tail.
2. `closeTimeout` expires before L3 is received. `stopForward()` is called.
3. The `MultiLineHandler.flush()` is called, emitting {L1, L2} as a truncated message.
4. The new tailer starts from byte 0 of the new file and correctly reads L3 as a
   standalone line.
5. At intake: the partial message {L1, L2 truncated} is received AND L3 as a separate
   line is received. L3 is neither lost nor duplicated in isolation, but the **logical
   event** that L1+L2+L3 was supposed to represent has been split: the partial is in one
   payload, L3 is in another, and the truncation flag on the partial tells users "this
   was cut," but there is no pointer to where the continuation (L3) lives in the intake.

This is a form of **logical duplication with loss**: the same logical event appears
twice (as a truncated fragment and as an orphaned continuation), while the complete
event is lost. Standard duplicate-detection at the intake (by content hash) will not
catch this because the two payloads have different content.

## Files and Functions

- `pkg/logs/tailers/file/tailer.go:306-339` — `StopAfterFileRotation` / `closeTimeout`
- `comp/logs/auditor/impl/auditor.go:313-331` — Flush snapshot race (1-second drift)
- `comp/logs-library/client/http/destination.go:309-318` — 4xx permanent drop still
  advances auditor offset (`output <- payload` unconditional)
- `pkg/logs/internal/decoder/multiline_handler.go:85-158` — `flush` / `sendBuffer` on timeout
- `pkg/logs/internal/decoder/multiline_handler.go:160-208` — `sendBuffer` truncation flag

## Why It Matters

The "both loss AND duplicate" outcome is the hardest scenario to detect and the most
confusing for users:
- Duplicate detection in downstream systems (Datadog dedup, splunk dedup) operates on
  content hash or message ID. A duplicated log that precedes a gap is not caught by dedup
  because the surrounding context differs.
- The 4xx-then-restart scenario violates the semantics communicated to users: they are
  told permanent drops are permanent. In compliance or audit contexts this matters: the
  log they were told was rejected may re-appear days later after an agent restart.
- The multiline split is invisible at the intake — neither half carries a reference to
  the other, and the truncation flag on the first half is the only hint.

## What the Assertion Checks

This property requires **workload-side correlation** rather than a single SUT-side
assertion. The workload must:

1. Write a known sequence of numbered log lines to a file.
2. Collect all lines delivered to fakeintake.
3. After a fault-inject + crash + restart + recovery window, verify:
   - No line number appears at intake more times than allowed by at-least-once semantics
     (e.g., at most 2× for crash replay, but the second copy must be a replay, not a
     new loss-then-duplicate in the same session).
   - Every line number that fakeintake records as "dropped with 4xx" does NOT also appear
     in subsequent deliveries (because a re-delivered 4xx-dropped line indicates the
     "permanent" drop semantics are not durable).

`Always` assertion (workload-side, on each intake receipt event):
```
If line N was previously received with a permanent-drop response code:
  assert line N is not received again in a new delivery
```

`AlwaysOrUnreachable` (SUT-side, in destination's permanent-drop path):
```go
assert.AlwaysOrUnreachable(
    !registry.HasBeenFlushed(payload.Origin.Identifier, payload.Origin.Offset),
    "permanent-drop must not advance a flushed registry offset",
    map[string]any{...})
```
(This would be a no-op when the auditor hasn't flushed yet, but fires on the combination
of flush + subsequent crash + replay.)

## Open Questions

- Does fakeintake track which payloads were responded to with 4xx vs 2xx? If not, the
  workload cannot distinguish "first time we saw this log" from "replay of previously
  rejected log." The workload may need to use a content-addressable log scheme (embedding
  monotonically increasing sequence numbers in log content) to detect the 4xx+restart
  scenario. `(needs human input)`
- Can the multiline handler be made to carry a "continuation pointer" (e.g., source file
  offset of the start of the continuation fragment) in the truncation metadata? This is a
  design question for the product, not a property question. `(needs human input)`

### Investigation Log

#### Is there a flush-before-drop path in the auditor?

- Examined: `comp/logs/auditor/impl/auditor.go:285-297` (run loop inputChan arm), `auditor.go:301-312` (flushTicker arm).
- Found: The run loop processes `inputChan` receives and `flushTicker.C` ticks in the same `select`. There is no explicit flush triggered by a 4xx drop event. The sequence is: HTTP destination calls `output <- payload` (regardless of 4xx), the auditor's run loop eventually receives it and calls `updateRegistry()`, then on the next 1-second tick calls `flushRegistry()`. There is always a sub-second window between `updateRegistry()` and `flushRegistry()`. No flush-before-drop path exists.
- Conclusion: resolved. The 1-second crash window exists for every `output <- payload` call, including 4xx drops. The "permanent in permanent drop" is only permanent if the agent stays alive for at least one flush tick. Removed from Open Questions.
