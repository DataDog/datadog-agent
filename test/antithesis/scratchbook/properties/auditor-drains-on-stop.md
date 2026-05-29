---
slug: auditor-drains-on-stop
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# auditor-drains-on-stop — Auditor Drains In-Flight Payloads Before Shutdown

## What Led to This Property

The SUT analysis §3 (H5) identifies that `auditor.Stop()` exits the run loop
without draining buffered payloads, causing the auditor to not advance past
offsets for recently-sent data. On restart, these payloads are re-read and
re-sent — the at-least-once delivery guarantee. This is also the root pattern
of the `62bf5e55c25` and `a5141ba432c` bug fixes (H2 — `Flush()` race). The
auditor's `inputChan` has a 100-message buffer (configurable via
`logs_config.message_channel_size`). Under load, all 100 slots may be occupied
at shutdown time.

## Code Paths Involved

### Current `Stop()` behavior

`comp/logs/auditor/impl/auditor.go:131-138`:
```go
func (a *registryAuditor) Stop() {
    a.closeChannels()     // closes inputChan, waits for run loop to exit
    a.cleanupRegistry()
    if err := a.flushRegistry(); err != nil {
        a.log.Warn(err)
    }
}
```

`closeChannels()` (lines 171-184):
```go
func (a *registryAuditor) closeChannels() {
    a.chansMutex.Lock()
    defer a.chansMutex.Unlock()
    if a.inputChan != nil {
        close(a.inputChan)    // signals run loop to exit
    }
    if a.done != nil {
        <-a.done              // waits for run loop goroutine to exit
        a.done = nil
    }
    a.inputChan = nil
    a.flushRequestChan = nil
}
```

The run loop (lines 283-291):
```go
case payload, isOpen := <-a.inputChan:
    if !isOpen {
        // inputChan has been closed, no need to update the registry anymore
        return
    }
```

**Key observation:** when `inputChan` is closed, the run loop returns
immediately, regardless of how many buffered payloads remain in `inputChan`.
A closed channel with buffered items still delivers remaining items before
returning the zero-value and `isOpen=false`. So the run loop *does* drain
buffered items — each `case payload, isOpen := <-a.inputChan` will receive
buffered items first, and only when all buffered items are consumed does the
`isOpen=false` arm fire.

However: the `select` statement in the run loop has multiple arms. When
`inputChan` is closed and has buffered items, the `flushTicker.C` or
`cleanUpTicker.C` cases can also be selected instead of the `inputChan` case.
Go's `select` with multiple ready cases chooses uniformly at random. So the
run loop may consume a tick event instead of a buffered payload, and then
immediately return on the next iteration when it receives `isOpen=false`.

**This is the H2/H5 race**: buffered payloads in `inputChan` after close are
not guaranteed to be drained before the loop exits if other select arms fire.

### The `Flush()` mechanism (partial mitigation)

`Flush()` (lines 146-161) sends a done-channel to `flushRequestChan` and waits
for the run loop to respond. The run loop handles this:
```go
case responseChan := <-a.flushRequestChan:
    n := len(a.inputChan)
    for i := 0; i < n; i++ {
        select {
        case payload := <-a.inputChan:
            ...
        default:
        }
    }
    ...
    close(responseChan)
```

The `n := len(a.inputChan)` snapshot is the H2 race: payloads arriving after
the snapshot are not processed by this `Flush()` call. However, `Flush()` is
called before `Stop()` in transport-restart scenarios (per bug fix `62bf5e55c25`).
The `Stop()` path does NOT call `Flush()` first.

### Post-Stop flush

After `closeChannels()`, `Stop()` calls `a.flushRegistry()` directly. But the
registry is only updated by `updateRegistry()` (called from the run loop). Any
payloads that were in `inputChan` at close time and not drained → offsets not
advanced → will replay on restart.

## Triggering Scenario

1. Agent receives a flush of log lines from a fast-writing application.
2. The sender successfully delivers 50 payloads and puts them in `inputChan`.
3. Shutdown is triggered before the run loop drains those 50 payloads.
4. `Stop()` closes `inputChan`; the run loop hits a ticker select arm at random
   and exits without processing the 50 buffered payloads.
5. `flushRegistry()` writes the old (stale) offsets to disk.
6. On restart: 50 payloads are re-read from the file and re-sent. Duplicates.

The interleaving is specifically: `close(inputChan)` followed by a non-payload
select arm firing in the run loop before all buffered payloads are consumed.

## Why It Matters

Duplicate log delivery is a correctness issue that:
- Inflates customer log ingestion volume (billing impact).
- Corrupts time-series analysis (inflated event counts).
- May trigger duplicate alert notifications.

The auditor's explicit purpose is to prevent this; a bug in its stop sequence
undermines the core guarantee.

## SUT-Side Instrumentation (all missing)

- `Reachable("auditor-drained-all-buffered-payloads-at-stop")` — placed in
  the run loop when `isOpen=false` AND `len(inputChan) == 0` at that moment.
  This is a meaningful outcome: confirms the happy path.
- `Sometimes("auditor-run-loop-exited-with-buffered-payloads-remaining")` —
  placed at the `return` inside `if !isOpen`, checking `len(inputChan) > 0`.
  This would fire when the race condition is actually triggered, helping
  Antithesis identify it as interesting.
- Workload: write N log lines, wait for `LogsSent == N`, issue stop; on
  restart, verify intake received exactly N (not N + duplicate).

## Open Questions

- What is the typical `inputChan` fill level at shutdown in production? If
  `inputChan` is usually near-empty at shutdown time, the duplicate window is
  small (1–2 messages max). If the system is under load at shutdown, it could
  be 100 messages. `(needs human input)`
- Is there a `sync.Once` guard preventing double `Stop()` of the auditor? If
  `Stop()` is called twice, `close(a.inputChan)` would panic on
  close-of-closed-channel.
- Under the stop timeout path: if `stop_grace_period` expires,
  `destinationsCtx.Stop()` force-cancels all in-flight HTTP connections. Can
  this cause partially-transmitted batches to be retried or permanently lost if
  the context is not re-created on restart?

## Merged-in evidence (from in-flight-payloads-on-stop)

The secondary file covered the **full stop-order sequence** and identified
additional fault triggers and assertions not present in the canonical:

**Stop order** (`agent.go:308-332`):
`schedulers → launchers → pipelineProvider → auditor → destinationsCtx →
diagnosticMessageReceiver`

The `pipelineProvider.Stop()` drain path *should* work: batchStrategy drains →
sender workers drain → destination input drains → all sent. The H5 bug breaks the
last step: `auditor.Stop()` / `closeChannels()` closes `inputChan`, and the run
loop returns on `!isOpen` **without draining remaining buffered items**.

**Grace period path** — `stopComponents()` (`agent.go:340-380`): if graceful stop
exceeds `logs_config.stop_grace_period`, `destinationsCtx.Stop()` is force-called,
cancelling all in-flight HTTP connections. Partially-transmitted batches may be
retried on next start (at-least-once) or permanently lost if the context is not
re-created.

**Additional fault trigger (from secondary):**
CPU throttle between `pipelineProvider.Stop()` return and `auditor.Stop()` call
widens the window during which payloads accumulate in `auditor.inputChan` before
it is closed.

**Additional assertions (from secondary):**
3. `Reachable(grace period timeout triggered in stopComponents)` — SUT-side in
   `stopComponents()` when the `case <-time.After(timeout)` branch fires.
4. `Unreachable(send-on-closed-channel panic in auditor)` — verify that
   destination goroutines do not attempt to send to the auditor channel after
   `pipelineProvider.Stop()` returns.

### Investigation Log

#### Is the H2 race still present after `62bf5e55c25`? Does Stop() drain buffered payloads?

- Examined: `comp/logs/auditor/impl/auditor.go`, full `run()` loop (lines 270–334), `Stop()` (131–138), `closeChannels()` (171–184).
- Found: `Stop()` calls `closeChannels()` which does `close(a.inputChan)` then blocks on `<-a.done`. The `run()` loop is a `for { select { ... } }`. When `inputChan` is closed the `case payload, isOpen := <-a.inputChan` arm fires; if `!isOpen` it returns immediately. Critically, `select` chooses uniformly at random among ready cases, so a ready `flushTicker.C` or `cleanUpTicker.C` arm can fire instead of the `inputChan` arm, consuming one loop iteration. If enough iterations are consumed by ticker arms, the loop reaches the `!isOpen` branch before exhausting all buffered items. This is the race. `Stop()` does NOT call `Flush()` before `closeChannels()`.
- Not found: any drain loop around `inputChan` in the stop path; any `sync.Once` guard on `Stop()`. Commit `1d1b05d054b` could not be inspected directly (no git blame traversal), but the current code does not contain a drain — the race is live.
- Conclusion: both sub-questions resolved. H2 race is present in the Stop() path. The `select`-not-`range` pattern is the root cause. Removed from Open Questions; findings already captured in the "Key observation" block above.

#### Does `flushRegistry()` in Stop() write the stale (pre-drain-gap) registry?

- Examined: `Stop()` body (lines 131–138). After `closeChannels()` returns (run loop has exited), `cleanupRegistry()` and `flushRegistry()` are called. `flushRegistry()` calls `readOnlyRegistryCopy()` which reads `a.registry`. The run loop only calls `updateRegistry()` for items it actually dequeues; items that remained buffered in `inputChan` when `!isOpen` fired were never processed. Therefore `a.registry` does not include those payloads.
- Found: confirmed — `flushRegistry()` after `Stop()` writes the stale registry (missing the unprocessed payloads).
- Conclusion: resolved. Removed from Open Questions.

#### Does Go's closed-channel `range` semantics apply here?

- Examined: `run()` loop structure. Uses `select`, not `range`.
- Found: for `range ch`, Go drains all buffered items before the loop exits. For `select { case v, ok := <-ch: if !ok { return } }`, a `!ok` fires as soon as the channel is drained OR (under select) when the channel is closed and other ready cases exist. The race is real because buffered items and channel-closed-signal are both ready simultaneously, and Go selects among ready cases randomly.
- Conclusion: confirmed the bug mechanism. Removed from Open Questions.
