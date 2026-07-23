---
slug: transport-switch-no-loss
focus: "8 — Lifecycle Transitions"
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# Property: transport-switch-no-loss

## What led to this property

The logs agent can perform a live transport switch from TCP to HTTP (and in
principle back). This is implemented in `agent_restart.go` via `restart()` and
`restartWithHTTPUpgrade()`. The restart mechanism:

1. `partialStop()` — stops launchers, pipelineProvider, destinationsCtx (in that
   order via `startstop.NewSerialStopper`). Calls `a.auditor.Flush()` after stop.
2. `setupAgentForRestart()` — rebuilds transient components: new `destinationsCtx`,
   new `pipelineProvider`, new `launchers`. **Persistent components preserved:**
   auditor, sources, tracker, schedulers.
3. `restartPipeline()` — starts only the transient components (not the auditor
   or schedulers).

The audit trail of a transport switch:
- Payloads in flight in the **old** pipelineProvider's channels when `partialStop`
  is called are lost (channel contents are not drained to the new pipeline).
- `auditor.Flush()` is called after the pipeline stops. This sends a flush request
  to the auditor's `run()` loop. The loop drains `len(inputChan)` items — but
  items that arrived at `inputChan` **after** the flush request was issued are
  not included. This is the H2 race from sut-analysis.md §3.
- After `partialStop`, the new launchers resume tailing from the auditor's
  registered offsets. If the Flush captured the right offsets, replay is minimal.
  If Flush missed some items, the new tailers re-read an earlier range.

The rollback path (`rollbackToPreviousTransport`) re-runs `setupAgentForRestart`
with the old endpoints if the new-transport restart fails. If this also fails, the
agent logs `CRITICAL: Failed to rollback` and returns an error — the pipeline is
left in an unknown state.

## Key code locations

- `comp/logs/agent/agentimpl/agent_restart.go:35-76` — `restart()` full sequence.
- `comp/logs/agent/agentimpl/agent_restart.go:164-183` — `partialStop()`: order
  of stop and flush.
- `comp/logs/agent/agentimpl/agent_restart.go:142-152` — `restartPipeline()`:
  starts only transient components.
- `comp/logs/agent/agentimpl/agent_restart.go:108-127` — rollback.
- `comp/logs/auditor/impl/auditor.go:313-331` — flush request handler: snapshots
  `len(inputChan)` then drains exactly that many items.

## What fault triggers it

**Network partition to intake** — causes the agent to fall back to TCP. When the
partition clears, `smartHTTPRestart` triggers `restartWithHTTPUpgrade`. Under
Antithesis, repeatedly injecting and lifting network partitions can trigger
multiple TCP↔HTTP switches, each of which creates a data-loss window.

**CPU throttling during `partialStop`** — widens the gap between `inputChan`
close and `auditor.Flush()`, increasing the chance that H2 (flush race) drops
items.

**Node termination during restart** — kills the agent mid-restart, potentially
leaving the auditor file in a state that reflects neither the old nor new
transport's committed offsets.

## Why it matters

Every transport switch creates a bounded data-loss window (payloads in flight
in the old pipeline's channels). For high-throughput pipelines, even a single
transport switch could drop thousands of log lines. The rollback path also has
a failure mode where the agent cannot restart at all, causing all log collection
to stop.

## Assertions needed (all net-new SUT instrumentation)

1. **`Reachable(transport switch initiated: TCP to HTTP)`** — SUT-side in
   `restartWithHTTPUpgrade()` or `restart()` when the new endpoints use HTTP:
   confirms the upgrade path is reached during the test.
2. **`Reachable(transport rollback initiated)`** — SUT-side in
   `rollbackToPreviousTransport()`: confirms the failure + rollback path is
   exercised.
3. **Workload `Always(line count at fakeintake monotonically increases through transport switch)`**
   — after a transport switch completes (the new transport is confirmed active),
   the total line count at fakeintake should not decrease. Any decrease indicates
   the new-transport's replay re-delivered lines that were already counted, which
   would be a double-counting scenario in the database.
4. **`Sometimes(auditor flush racing with inputChan items)`** — SUT-side in
   `auditor.go` flush handler at line 314: after `n := len(a.inputChan)`,
   if there are items in `inputChan` beyond position `n`, emit `Sometimes(true)`.
   This confirms the H2 race window is present during the test.

## Recovery window requirement

Fault-quiet window needed after transport switch completes: the new transport's
first successful send must complete before the workload counts lines, to give
time for the replay window to drain.

## Open questions

- Does `partialStop` drain the `pipelineProvider`'s internal channels before
  stopping?
  `(partial: confirmed that parallel pipeline stop means cross-pipeline in-flight
  payloads are not transferred; data loss window exists proportional to pipeline
  channel occupancy at stop time)`
- After a rollback fails critically, does the agent report a health check failure
  that would cause a process restart by the container orchestrator? No evidence
  found of a health check failure being triggered in the rollback failure path.
  `(needs human input)`

### Investigation Log

#### Does `partialStop` drain the `pipelineProvider`'s internal channels before stopping?

- Examined: `comp/logs/agent/agentimpl/agent_restart.go:164-183` (`partialStop`),
  `comp/logs-library/pipeline/provider.go:283-300` (`provider.Stop()`).
- Found: `partialStop` calls `stopComponents` which runs a `SerialStopper` on
  `[launchers, pipelineProvider, destinationsCtx]` in order. `provider.Stop()`
  closes router channels and waits for `forwarderWaitGroup` (forwarder goroutines
  drain), then calls `stopper.Stop()` on pipelines in parallel, then calls
  `p.sender.Stop()`. The parallel pipeline stop calls `processor.Stop()` (closes
  `inputChan`) and `strategy.Stop()` concurrently. There is no explicit drain of
  in-flight payloads between processor and strategy before close.
- Conclusion: the data-loss window is confirmed. Payloads in flight in router
  channels or processor-to-strategy channels at `partialStop` time are dropped.
  The `(partial:)` tag is confirmed. Full resolution requires knowing how many
  payloads are typically in-flight, which is workload-dependent.

#### Does `provider.Stop()` `forwarderWaitGroup.Wait()` have a timeout?

- Examined: `comp/logs-library/pipeline/provider.go:289`.
- Found: `forwarderWaitGroup.Wait()` is called without a timeout within
  `provider.Stop()`. The enclosing `stopComponents` wrapper imposes a 30s
  grace period — so provider.Stop() (including its Wait) is bounded at 30s
  by the outer layer only. The `forwardWithFailover` goroutines drain the router
  channel (which is closed before `Wait()`), so under normal conditions `Wait()`
  returns quickly. The hang scenario only arises if a forwarder goroutine is
  blocked on `InputChan` (backpressure fallback), which leaves the router channel
  range loop stuck and `Wait()` blocking indefinitely until the outer timeout.
