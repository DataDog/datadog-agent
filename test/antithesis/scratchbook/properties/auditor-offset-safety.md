---
slug: auditor-offset-safety
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# auditor-offset-safety — Auditor Offset Only Advances Past Successfully-Sent Data

## What Led to This Property

SUT guarantee S2: the auditor registry offset for a source should only be
advanced after the corresponding payload has been confirmed delivered (HTTP 2xx
from intake). Two concrete caveats undermine this guarantee in practice:

1. **Permanent 4xx also advances the offset** — the agent treats 4xx as a
   permanent drop and still writes `output <- payload` (advancing the auditor)
   even though the log data was never stored.
2. **1-second persistence window** — the auditor flushes to disk every 1 second
   (`defaultFlushPeriod`). A crash in the window replays ≈1s of traffic.

## Code Paths Involved

**Offset advancement path:**
- `comp/logs-library/client/http/destination.go:317-318` — after
  `unconditionalSend()` returns (success or permanent error), `output <- payload`
  is called unconditionally. The `output` channel feeds the auditor input.
- `comp/logs/auditor/impl/auditor.go:285-297` — the auditor `run()` loop reads
  from `inputChan` and calls `updateRegistry()`.
- `updateRegistry()` (`auditor.go:374-398`) stores the new offset, subject to
  the `IngestionTimestamp` monotonicity check.

**4xx silent advance:**
- `destination.go:407-419` — `StatusBadRequest (400)`, `StatusUnauthorized (401)`,
  `StatusForbidden (403)` (without secrets refresh), and `StatusRequestEntityTooLarge (413)`
  return `errClient` (non-retryable). Then `destination.go:308-312`:
  if `d.shouldRetry` is false *or* `updateRetryState` returns false, the block
  falls through to `output <- payload` (line 318). The offset is advanced even
  though the data was never stored.

**Flush race:**
- `auditor.go:313-331` — the flush path (triggered by `Flush()` or
  `flushRequestChan`) uses `n := len(a.inputChan)` to snapshot how many items
  to drain. Payloads arriving *after* the snapshot are not included in the flush.
  This is the root cause of bug `62bf5e55c25` and is the well-known 1-second
  window.

**Stop() drops in-flight payloads:**
- `auditor.go:132-138` — `Stop()` calls `closeChannels()` which closes
  `inputChan`, then waits for the `run()` goroutine to exit. The `run()` loop at
  line 285 sees the close and returns *without draining the buffered messages
  still in `inputChan`*. Those payloads were sent to intake successfully but
  their offsets were never persisted.

## Failure Scenario

**Crash during flush window:**
1. Payload containing log lines 1000-1050 is sent successfully to intake.
2. `output <- payload` advances the auditor's in-memory registry.
3. The 1-second ticker has not fired yet; the registry file still records
   offset 999.
4. Process is killed.
5. On restart, the agent starts re-tailing from offset 1000 → lines 1000-1050
   are re-sent (duplicate delivery).

**4xx silent data loss:**
1. Payload is rejected with 400 Bad Request (e.g., oversized after a batch
   encoding bug inflates the size past `logs_config.batch_max_size`).
2. Auditor advances offset past those lines.
3. No retry; those lines are permanently lost.
4. No metric currently distinguishes "dropped due to 4xx" from "dropped due to
   payload limit" — the loss is invisible.

## Why It Matters

This is the primary mechanism for "duplicate logs after restart" — a documented
user complaint (product context item #2 in the SUT analysis). The 4xx branch
also creates silent data loss (item #4). The auditor is the only persistence
layer in the logs pipeline; incorrect offsets lead to either data loss or
duplicate delivery, both of which users can detect but cannot remediate.

Antithesis node-termination faults (`kill -9`) exercise the crash window
directly. No existing test rotates the agent process while payloads are in-flight.

## Workload Instrumentation

- Each log line carries a sequence number embedded in its content.
- After a simulated crash-and-restart, the fakeintake checks:
  - No sequence numbers are missing (no data loss beyond the known 4xx drop path).
  - Duplicate sequence numbers exist (expected; at-least-once semantics).
- SUT-side: a `Sometimes` assertion at the auditor's `updateRegistry` site
  confirming that the offset being stored is >= the offset stored just before
  the crash — currently **missing**.

## Open Questions

- Is the "4xx advances offset" behavior intentional design (permanent drops are
  considered acknowledged)? If intentional, the property should be qualified:
  "offset only advances past successfully sent OR permanently-rejected data."
  `(needs human input)`
- Should permanent TCP drops advance the offset? The current HTTP/TCP divergence
  is real and confirmed (TCP does NOT advance on permanent drop; HTTP does).
  `(needs human input)` — is this asymmetry intentional?
- Under dual-shipping, does the auditor's `IngestionTimestamp` guard ensure
  idempotent offset updates, or can a race between the two destination outputs
  produce non-monotonic updates in the registry? `(partial: updateRegistry has
  timestamp guard at auditor.go:385, but the guard is "newer wins", not "highest
  byte offset wins" — for file tailers these should be equivalent, but for
  journald cursor offsets this may not hold)`
- Does the fakeintake provide per-payload sequence numbers or offsets that can be
  correlated back to file byte offsets? Without this, the workload-side `Always`
  check is approximate (line count based, not byte offset based).

## Merged-in evidence (from no-offset-ahead-of-durable-send)

The secondary file focused on the **"offset ahead of durable send"** direction
(data loss, not duplication) and the dual-shipping `IngestionTimestamp` race.
Key additional detail:

- `metrics.LogsSent` is incremented (destination.go:314) *before* `output <-
  payload` (line 318), creating a brief window where a metric signals success
  but the auditor has not yet been notified.
- Under CPU throttle, the window between `output <- payload` and the auditor's
  consumption of `inputChan` can be wide. Combined with the 1-second flush
  ticker, the maximum unacknowledged window is bounded by `inputChan` depth
  (100 payloads default) × average payload size.
- Additional needed assertion: `Sometimes(auditor inputChan non-empty at
  flushTicker tick)` — SUT-side probe to confirm the race window exists.

## Merged-in evidence (from offset-not-advanced-on-permanent-drop)

The secondary file detailed the **TCP vs. HTTP divergence** in permanent-drop
offset behaviour, which the canonical file does not cover:

**TCP destination** (`comp/logs-library/client/tcp/destination.go:81-146`):
on permanent drop (`shouldRetry=false`), `output <- payload` is **NOT** called.
The TCP auditor does not advance the offset on permanent drop. Under restart
after a TCP permanent drop, the payload is re-read and potentially re-sent
(duplicate rather than HTTP's permanent loss). This diverges from HTTP behavior
and is an inconsistency worth flagging.

For **retryable errors** on both transports: `continue` is called before
`output <- payload`, so the auditor is never notified until the payload either
succeeds or is permanently dropped — the offset invariant holds.

Additional assertion needed:
- **`AlwaysOrUnreachable` (on permanent drop path):** when `err != nil` and not
  retrying (HTTP permanent drop), assert that `output <- payload` will be called
  exactly once for this payload. Documents intent and catches refactors.

### Investigation Log

#### Does the Stop() path call Flush() before closeChannels(), covering buffered payloads?

- Examined: `comp/logs/auditor/impl/auditor.go:131-138` (`Stop()`), `comp/logs/agent/agentimpl/agent.go:308-333` (`stop()`), `startPipeline()` (lines 279–293).
- Found: `Stop()` is called via `startstop.NewSerialStopper` in `stopComponents()`. Stop order: `schedulers → launchers → pipelineProvider → auditor → destinationsCtx → diagnosticMessageReceiver`. `Stop()` body: `closeChannels()` then `cleanupRegistry()` then `flushRegistry()`. There is NO `Flush()` call before `closeChannels()`. `Flush()` is only called on transport restarts (HTTP↔TCP switch), not during agent shutdown.
- Not found: any caller of `a.Flush()` in the graceful shutdown sequence.
- Conclusion: `Flush()` is NOT called before `Stop()` during shutdown. The buffered-payload window exists for graceful shutdown too. Resolved; removed from Open Questions.

#### Is the HTTP/TCP asymmetry on permanent drop real?

- Examined: `comp/logs-library/client/http/destination.go:262-320` (`sendAndRetry`), `comp/logs-library/client/tcp/destination.go:81-146` (`sendAndRetry`).
- Found: HTTP — when `shouldRetry=false` or `updateRetryState` returns false, execution falls through to `output <- payload` (line 318) unconditionally whether `err` is nil or not. TCP — on write error with `shouldRetry=false`: increments errors, calls `updateRetryState(nil, ...)`, then **returns** without calling `output <- payload`. TCP only calls `output <- payload` on success (line 137).
- Conclusion: asymmetry is real and confirmed. HTTP advances auditor on permanent drop; TCP does not. Whether this is intentional is a design question. Tagged `(needs human input)`.

#### Are MRF/unreliable destinations in NonBlockingSend a separate advance path?

- Examined: `comp/logs-library/client/http/destination.go`, `comp/logs-library/client/destination.go`.
- Found: `NonBlockingSend` drops the payload silently (no `output <- payload`). MRF destinations share the same `sendAndRetry` path as primary HTTP destinations — same offset-advance semantics apply.
- Not found: a `noopSink` distinct from the auditor channel; the unreliable path simply discards the payload without touching the auditor.
- Conclusion: no additional offset-advance paths beyond HTTP and TCP. Removed from Open Questions.
