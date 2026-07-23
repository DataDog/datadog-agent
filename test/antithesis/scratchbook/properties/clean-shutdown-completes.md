---
slug: clean-shutdown-completes
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# clean-shutdown-completes — Shutdown Always Completes Within Bounded Time

## What Led to This Property

The logs agent has a complex, multi-stage shutdown sequence with several known
hang paths. The SUT analysis §3 identifies H4 (`forwardWithFailover` hang),
H5 (auditor Stop drops in-flight), and historical deadlocks in the launcher
layer. The project review guidelines flag goroutine-leak/hang as a top bug
class. A shutdown that never returns is invisible to health checks and blocks
agent upgrade/restart.

## Shutdown Sequence and Hang Risks

The full shutdown order (inferred from `provider.Stop()` and component
lifecycle):

1. **Launcher `Stop()`** — closes `addedSourcesDone`/`removedSourcesDone`,
   sends to `stop` chan, waits on `done`. Risk: if a `FilesToTail` goroutine is
   blocked on a file-system call, `done` never closes.

2. **Tailer `Stop()`** — signals `readForever` via `stop` chan, waits on
   `done` (which `forwardMessages` closes). Risk: `forwardMessages` is blocked
   on `outputChan <- msg` if the downstream pipeline is saturated; `done` is
   not closed until `forwardMessages` returns; launcher `Stop()` hangs.

3. **Pipeline `Stop()`** → `processor.Stop()` closes `inputChan`, then
   `strategy.Stop()`. Risk: if `forwardWithFailover` is blocked on `inputChan`
   (path 1 in `no-send-on-closed-on-shutdown`), `processor.Stop()` panics
   rather than hanging.

4. **Auditor `Stop()`** — `closeChannels()` closes `inputChan`, waits on
   `done` chan (which the run loop signals via `a.done <- struct{}{}`). No
   timeout. Risk: if the run loop is stuck on a slow disk flush, `done` blocks
   indefinitely.

5. **Sender `Stop()`** — calls `worker.stop()` (blocks until each worker's
   run loop sees `done` and signals `finished`), then closes queues. Risk:
   if a worker is in the `time.Sleep(100ms)` busy-wait loop while all
   destinations are retrying (`worker.go:146`), it responds to `done` within
   at most 100ms — bounded. But if destinations hang on network I/O
   indefinitely, the worker's `finished` is never sent.

The most dangerous hang: tailer `forwardMessages` blocked on `outputChan`,
while the pipeline's processor goroutine is already stopped, so nothing drains
`outputChan`. The `forwardContext.Done()` escape path (`tailer.go:432-433`) is
the designed mitigation — `StopAfterFileRotation` calls `t.stopForward()`. But
for a normal `Stop()` (not rotation), `stopForward` is NOT called; the tailer
blocks until the pipeline drains its input.

## Code Paths Involved

`pkg/logs/tailers/file/tailer.go:430-435`:
```go
select {
case t.outputChan <- msg:
    t.CapacityMonitor.AddIngress(msg)
case <-t.forwardContext.Done():  // only cancelled by stopForward, not by Stop()
}
```

`tailer.go:295-303`:
```go
func (t *Tailer) Stop() {
    t.registry.SetTailed(t.Identifier(), false)
    select {
    case t.stop <- struct{}{}:
    default:
    }
    t.file.Source.RemoveInput(t.file.Path)
    <-t.done    // blocks until forwardMessages returns
}
```

`forwardMessages` returns only when `t.decoder.OutputChan()` is closed, which
happens when `readForever` returns and calls `t.decoder.Stop()`. `readForever`
exits on `<-t.stop`. So the intended drain path is:
1. `Stop()` → signals `stop` → `readForever` exits → `decoder.Stop()` → decoder
   output channel closes → `forwardMessages` exits its range loop → closes `done`
2. But `forwardMessages` may be blocked on `outputChan <- msg` *before* the range
   loop exits. The `done` signal from `decoder.OutputChan()` closing won't wake it.

Wait: `for output := range t.decoder.OutputChan()` — the range loop body is:
```go
case t.outputChan <- msg:
case <-t.forwardContext.Done():
```
If `forwardContext` is not cancelled, and `outputChan` is full, `forwardMessages`
is stuck until `outputChan` has room. The pipeline that drains `outputChan` is
already stopping → circular dependency.

## Triggering Scenario

1. Network partition causes pipeline sender to back up; `outputChan` fills (100-
   message buffer exhausted).
2. Shutdown is initiated (e.g., agent SIGTERM).
3. `Launcher.Stop()` calls `tailer.Stop()` which blocks at `<-t.done`.
4. `Pipeline.Stop()` is called concurrently (parallel stopper), closes
   `processor.inputChan`, stopping the processor — no longer draining
   `outputChan`.
5. `forwardMessages` is blocked on `outputChan <- msg` with no consumers.
6. `forwardContext.Done()` is not cancelled (only rotation calls `stopForward`).
7. `tailer.Stop()` hangs indefinitely.

## Why It Matters

A hung shutdown blocks agent upgrades, prevents the auditor from flushing
(losing offsets), and forces a `kill -9` which leaves the process in an
unclean state. This is one of the primary bug patterns in the agent's bug
history (§6: `94d7ccbfc35`, `7041f901670`).

## SUT-Side Instrumentation (all missing)

- `Reachable("tailer-stop-completed")` — at the point after `<-t.done` returns
  in `tailer.Stop()`, confirming clean exit. Compare with a workload timeout.
- `Reachable("provider-stop-completed")` — at the end of `provider.Stop()`.
- `Reachable("auditor-stop-completed")` — after `closeChannels()` returns in
  `auditor.Stop()`.
- The workload should issue a shutdown signal and then assert within T seconds
  that the pipeline is no longer processing messages (progress has stopped).

## Open Questions

- Is the tailer `Stop()` → `forwardMessages` hang scenario actually reachable
  with the current parallel stopper ordering? If `Pipeline.Stop()` runs before
  `Tailer.Stop()`, the pipeline is already closed and `outputChan` is drained.
  The ordering depends on the `startstop.ParallelStopper` invocation order in
  `launcher.cleanup()`. `(partial: code at launcher.go:196-207 shows tailers
  stopped in parallel, but pipeline.Stop() is called from provider.Stop() which
  runs after launcher.Stop() — so pipeline IS alive while tailers stop; the
  hang scenario requires the pipeline to be saturated, not stopped)`

### Investigation Log

#### What timeout (if any) does the agent's top-level shutdown impose?

- Examined: `comp/logs/agent/agentimpl/agent.go:340-380` (`stopComponents`),
  `pkg/config/setup/common_settings.go:1869`.
- Found: `stopComponents` runs the serial stopper in a goroutine, waits up to
  `logs_config.stop_grace_period` seconds (default 30, integer, cast to
  `time.Duration * time.Second`). On timeout it calls `forceClose` (which calls
  `a.destinationsCtx.Stop()`, cancelling HTTP contexts), then gives 5 more
  seconds for the stopper goroutine to finish, then dumps goroutines and exits.
  The grace period is 30s by default — confirmed externally bounded.
- Conclusion: hang is bounded at ~35s by the outer wrapper. Antithesis should
  verify shutdown completes **before** the 30s grace expires, testing that the
  clean path works without relying on the timeout. This is the correct framing
  for the Antithesis property.

#### Does `forwardContext` get cancelled on normal `Stop()`?

- Examined: `pkg/logs/tailers/file/tailer.go:118-198, 295-338`.
- Found: `forwardContext` is created with `context.WithCancel(context.Background())`
  at line 171; `stopForward` is stored as `t.stopForward` at line 198.
  `Stop()` (lines 295-304) does NOT call `t.stopForward()`. Only
  `StopAfterFileRotation()` calls it (line 332). The hang scenario described in
  the property body is therefore accurate: a non-rotation `Stop()` leaves
  `forwardContext` uncancelled, and `forwardMessages` can block indefinitely on
  `outputChan` if the pipeline is saturated.
- Conclusion: confirmed. The `(partial:)` note in the other open question is
  consistent — the hang requires pipeline saturation, not pipeline stop.
  `forwardContext` is not the fix in the normal Stop path.
</content>
