---
slug: multiline-not-split-across-pipelines
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# multiline-not-split-across-pipelines — Multiline Log Events Are Not Split at Rotation or Pipeline Boundaries

## What Led to This Property

Two documented bugs from the SUT analysis bug history:
- `046241bfc73`: timestamp lost through auto-multiline aggregation.
- `15b1c1c8ae2`: `DetectingAggregator TrimSpace` breaks anchored rules.

Plus the general risk from the SUT analysis §10: "Multi-line aggregation ×
truncation: stacktraces split or cut at the size boundary; auto-multiline can
lose timestamps."

A multiline event (e.g., a Java stack trace) spans multiple raw log lines.
The decoder aggregates these into a single `message.Message`. If the event
is split at a rotation boundary or at the pipeline's maximum message size,
the user receives a partial event — corrupted data.

## Code Paths Involved

**Multiline aggregation** — `pkg/logs/internal/decoder/multiline_handler.go`
and `preprocessor/` handlers. The decoder accumulates lines matching a
multiline pattern until a flush condition is met (regex mismatch, timeout,
max size, or EOF).

**Truncation at max size** — `pkg/logs/internal/decoder/single_line_handler.go`
or the multiline handler: when the accumulated message exceeds
`logs_config.message_max_bytes` (default 256KB), it is truncated and the
`IsTruncated` flag is set. Bug `60c521b9e7d` covers a specific truncation-at-
boundary bug.

**Rotation boundary:**
- Old tailer's `readForever` is stopped; `decoder.Stop()` is called.
- `decoder.Stop()` flushes any buffered multiline state via `Flush()`.
- But if `forwardContext` is already cancelled (due to `stopForward()` firing
  before the decoder is fully flushed), the flush messages are discarded.

**`closeTimeout` interaction with multiline:**
- If a stack trace is in the middle of being decoded when `closeTimeout` fires,
  the partially-accumulated event is either:
  (a) Flushed if `decoder.Stop()` fires before `stopForward()` cancels the context.
  (b) Discarded if the context is cancelled first.

**`IsTruncated` flag** — `processor.go:184-191`: truncated messages are counted
via `TlmTruncatedCount`. The `ExcludeTruncated` rule can filter them out. But
there is no mechanism to rejoin a split multiline event after truncation.

## Failure Scenario

1. Agent tails a Java application log.
2. An exception occurs; the application logs a stack trace spanning 200 lines
   (a multiline event).
3. The log file rotates in the middle of the stack trace (after line 100).
4. Old tailer: decoded lines 1-100, still accumulating.
5. `StopAfterFileRotation()` fires; `closeTimeout=60s` starts.
6. The stack trace continues being written to the new file (lines 101-200).
7. After 60s (or less if backpressure), old tailer's `stopForward()` fires.
8. Lines 1-100 (the partial event) are either:
   - Flushed as a partial stack trace (corrupted — the user sees half a trace).
   - Discarded (lost entirely).
9. New tailer picks up from the start of the new file and sees lines 101-200
   as a new event — context missing.

The fakeintake receives two malformed events instead of one complete stack trace.

## Why It Matters

Corrupted multiline events are a correctness failure for observability: the
user's log aggregation shows partial stack traces, missing the root cause of
an error. This is directly linked to the auto-multiline feature that was added
to improve user experience but introduced several bugs (captured in the bug
history).

## Workload Instrumentation

- Workload writes multiline events (simulated stack traces) with a known
  structure: `BEGIN_EVENT_N`, then K lines of content, then `END_EVENT_N`.
- Each event is identifiable by its N tag and its BEGIN/END markers.
- After a rotation event, the fakeintake checks:
  - Every event has exactly one BEGIN and one END marker.
  - No lines from event N appear in a message that also contains lines from
    event N+1.
- SUT-side: a `Sometimes` assertion confirming the multiline flush path is
  exercised during rotation — currently **missing**.

## Open Questions

- Does the auto-multiline detector (`legacy_auto_multiline_handler.go`) have
  different flush semantics than the configured multiline handler? If auto-
  multiline uses a timeout-based flush, the rotation race window is larger.
  `(partial: auto-multiline uses flushTimer like the configured handler; the
  detecting aggregator path adds a pattern-learning phase that could compound
  the bug — needs deeper investigation of DetectingAggregator.Flush())`
- What is the maximum number of lines in a single multiline event? Is there a
  hard limit based on `max_message_size_bytes` only, or a line-count limit?
  If the max is purely size-based (256KB), the property should be scoped to
  "events within size limit are not split."
- Does Antithesis's clock fault affect Go's monotonic clock (used by
  `time.Timer`)? This is the critical uncertainty for the timer-flush split.
  `(needs human input)`
- What is the default `flushTimeout` value for multiline handlers in production
  configurations? `(needs human input)`

### Investigation Log

#### Is the multiline decoder's `Flush()` guaranteed to complete before `forwardContext` is cancelled?

- Examined: `pkg/logs/tailers/file/tailer.go`:
  - `StopAfterFileRotation()` (lines 308-339): fires `t.stopForward()` first
    (line 332), then sends to `t.stop` channel (line 334).
  - `readForever()` (lines 343-373): the deferred call at line 348 (`t.decoder.Stop()`)
    runs when `readForever` returns, which happens when `t.stop` is received.
  - `forwardMessages()` (lines 391-436): the select at line 430-434 selects between
    `t.outputChan <- msg` and `<-t.forwardContext.Done()`. After `stopForward()` is
    called, `t.forwardContext.Done()` is closed.
- Found: The ordering is:
  1. `stopForward()` cancels `forwardContext`.
  2. `t.stop` signal is sent to `readForever()`.
  3. `readForever()` returns → deferred `decoder.Stop()` runs.
  4. `decoder.Stop()` calls `Flush()` which pushes remaining messages to the
     decoder's `OutputChan`.
  5. `forwardMessages()` drains `decoder.OutputChan`, but at the select, if
     `forwardContext.Done()` fires first (which it will, since step 1 already cancelled
     it), the message is **discarded**.
- Conclusion: **resolved** — the bug is confirmed. `stopForward()` fires before
  `decoder.Stop()`, so any messages that `decoder.Stop()/Flush()` emits to the
  decoder's `OutputChan` are discarded by `forwardMessages()` (the `forwardContext`
  is already cancelled). Partial multiline events accumulated at rotation time are
  silently dropped. This confirms the failure scenario described in the evidence.
  The property assertion needs to account for this: the claim "events are not split"
  is violated at rotation when the closeTimeout fires with a partial multiline in progress.

## Merged-in evidence (from multiline-flush-timeout-no-split-events)

The secondary file covered a **distinct but related split path**: premature timer
flush caused by Antithesis clock faults rather than file rotation. Both paths
produce the same symptom (a split event at fakeintake), but via different
mechanisms. This canonical file now covers both.

### Timer-Driven Split (clock-jump path)

`MultiLineHandler` and the preprocessor both use `flushTimer` (`time.Timer`) to
flush partial multiline messages when no new input arrives within `flushTimeout`.

**Forward clock jump:** if Antithesis advances the monotonic clock past the timer
deadline, the `flushTimer` fires prematurely:
1. Multiline message in progress (L1, L2 buffered, L3 expected in 10ms).
2. Antithesis jumps clock forward 5 seconds (>> 1s default `flushTimeout`).
3. `flushTimer` fires; `flush()` emits {L1, L2} as a complete (incomplete) message.
4. L3 arrives and is emitted as a standalone line.
5. Intake receives two events instead of one — **without any `IsTruncated` flag**.
   This is invisible corruption: no truncation flag, no error, no metric.

**Backward clock jump (liveness):** a backward jump may make the monotonic clock
appear to rewind relative to the timer deadline. The `flushTimer` does not fire
until real monotonic time reaches the original deadline. Combined with file
rotation during the extended wait, `flushTimer.Stop()` can block on
`<-h.flushTimer.C` (multiline_handler.go:95-99) if the timer goroutine is CPU-
throttled, stalling the decoder's `Stop()` call and delaying the final flush.

**Auto-multiline (DetectingAggregator):** the same clock-jitter scenario applies.
A premature flush during pattern learning changes the baseline and can cause the
detector to compute an incorrect pattern threshold from a split sample.
Bug `046241bfc73` (timestamp lost through auto-multiline) already demonstrated a
real issue on this path.

### Key code locations (timer path)

- `pkg/logs/internal/decoder/multiline_handler.go:78-83` — `flushChan()`
- `pkg/logs/internal/decoder/multiline_handler.go:85-158` — `process()` with timer management
- `pkg/logs/internal/decoder/multiline_handler.go:95-98` — timer drain race
- `pkg/logs/internal/decoder/preprocessor/preprocessor.go:121-138` — `stopFlushTimerIfNeeded` / `startFlushTimerIfNeeded`
- `pkg/logs/internal/decoder/preprocessor/preprocessor.go:96-100` — `FlushChan`
- `pkg/logs/internal/decoder/legacy_auto_multiline_handler.go` — auto-multiline flush path

### Additional assertion (timer path)

In `MultiLineHandler.sendBuffer`, before emitting:
```go
assert.AlwaysOrUnreachable(
    h.isBufferTruncated || h.patternMatchedOnce || timerExpiredButNotTruncated,
    "multiline message flushed by timer must be flagged or explicitly expected",
    map[string]any{"reason": flushReason, "linesLen": h.linesLen})
```

Workload-side: if intake receives a stacktrace header line standalone (without a
preceding truncated event from the same source), this is evidence of an invisible
timer-driven split.
