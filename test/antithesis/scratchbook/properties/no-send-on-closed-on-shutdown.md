---
slug: no-send-on-closed-on-shutdown
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# no-send-on-closed-on-shutdown — No Send-on-Closed-Channel Panic During Shutdown

## What Led to This Property

The project's own review guidance names send-on-closed-channel as the top bug
class in the logs agent. The SUT analysis (§3, H4) identifies a concrete path
in `forwardWithFailover` and the broader sender shutdown sequence. This is the
most dangerous category of shutdown race: it panics the entire process.

## Code Paths Involved

### Path 1 — `forwardWithFailover` blocking on `InputChan` after `Stop()` closes it

`comp/logs-library/pipeline/provider.go:353-365`:
```go
func (p *provider) forwardWithFailover(routerIndex int) {
    defer p.forwarderWaitGroup.Done()
    for msg := range p.routerChannels[routerIndex] {
        if !p.trySendToPipeline(msg, primaryPipelineIndex) {
            // Blocks here waiting for a pipeline to accept
            p.pipelines[primaryPipelineIndex].InputChan <- msg  // line 361
            ...
        }
    }
}
```

`Stop()` (lines 283-299) closes `routerChannels` first, then calls
`forwarderWaitGroup.Wait()`. The `for msg := range ...` loop exits on channel
close. However: while `forwardWithFailover` is blocked at line 361 on
`InputChan <- msg`, the router channel close doesn't interrupt it. The goroutine
is stuck on `InputChan`, not on the router channel.

`Stop()` then proceeds to `stopper.Stop()` which closes `Pipeline.InputChan`
(via `processor.Stop()`). If the forwarder goroutine is simultaneously blocked
on `p.pipelines[i].InputChan <- msg`, the close of that channel triggers a
send-on-closed-channel **panic**.

The sequence:
1. `p.routerChannels[i]` receives a message and the forwarder blocks at line 361
2. `provider.Stop()` is called
3. `close(p.routerChannels[i])` — does not unblock the goroutine (it's on InputChan)
4. `forwarderWaitGroup.Wait()` — hangs because the goroutine is blocked on InputChan
5. Deadlock (or timeout) → `stopper.Stop()` runs (pipeline InputChan closed)
6. Goroutine blocked at line 361 panics: send on closed channel

### Path 2 — Worker `stop()` vs in-flight send

`comp/logs-library/sender/worker.go:96-99`:
```go
func (s *worker) stop() {
    s.done <- struct{}{}
    <-s.finished
}
```

`Sender.Stop()` (sender.go:180-188):
```go
func (s *Sender) Stop() {
    for _, w := range s.workers {
        w.stop()          // signals done
    }
    for _, q := range s.queues {
        close(q)          // closes AFTER stop signals
    }
}
```

The worker `run()` loop reads from `inputChan` (the queue). After receiving
`done`, it cleans up destinations and sends `finished`. The queue close happens
after `stop()` returns. However, the queue has `workersPerQueue` buffer slots.
If a second goroutine sends to the queue while the worker is draining its
`done`, the close of the queue could race with a concurrent queue write from the
batch strategy. A second `Stop()` call (H6 from SUT analysis) would double-close
the queue channel → panic. Neither `Sender` nor `provider` has a `sync.Once`
guard on their `Stop()` methods.

### Path 3 — Auditor inputChan double-close

`comp/logs/auditor/impl/auditor.go:171-184`:
```go
func (a *registryAuditor) closeChannels() {
    a.chansMutex.Lock()
    defer a.chansMutex.Unlock()
    if a.inputChan != nil {
        close(a.inputChan)
    }
    ...
    a.inputChan = nil
}
```

The nil-guard protects against a double `Stop()` call on the auditor itself.
However, after `close(a.inputChan)` and before `a.inputChan = nil` is visible
to other goroutines (they read it via `Channel()` with their own lock acquire),
a destination goroutine that cached the channel pointer before the lock could
still send to the just-closed channel.

## Triggering Interleaving

For Path 1 under Antithesis CPU-pause faults:
1. `forwardWithFailover` goroutine is mid-send on `InputChan` (blocked)
2. Shutdown signal arrives; CPU pauses the forwarder goroutine before it can exit
3. `provider.Stop()` closes router channels, waits (times out or hangs), then
   closes pipeline InputChans
4. CPU resumes the forwarder goroutine → sends to closed InputChan → panic

## Why It Matters

A send-on-closed-channel panic terminates the agent process without graceful
shutdown. All in-flight data is lost, the auditor registry may not be flushed,
and the user sees an unexpected crash rather than a clean restart.

## SUT-Side Instrumentation (all missing)

- `Unreachable("send-on-closed-inputchan-from-forwarder")` — in a panic recovery
  wrapper around the `p.pipelines[i].InputChan <- msg` send in `forwardWithFailover`.
- `Unreachable("sender-queue-double-close")` — in a recover() at `Sender.Stop()`
  around the `close(q)` calls.
- `Reachable("forwarder-goroutine-exited-cleanly")` — at the `defer
  p.forwarderWaitGroup.Done()` point in `forwardWithFailover`, to confirm
  clean exit rather than panic-exit is the observed outcome.

## Open Questions

- Is `forwardWithFailover`'s block on `InputChan` interruptible? The current
  implementation has no select with a stop signal at line 361 — it's a plain
  send. If Antithesis cannot pause the goroutine at exactly that point, the
  window may be narrow. But since Antithesis controls scheduling, this is
  reachable.

### Investigation Log

#### Does `provider.Stop()` have a timeout on `forwarderWaitGroup.Wait()`?

- Examined: `comp/logs-library/pipeline/provider.go:283-300`,
  `comp/logs/agent/agentimpl/agent.go:340-380` (`stopComponents`),
  `pkg/config/setup/common_settings.go:1869`.
- Found: `provider.Stop()` calls `p.forwarderWaitGroup.Wait()` at line 289 with
  **no timeout** — it is an unbounded block. The timeout exists only in the outer
  `stopComponents` wrapper in `agent.go`: it runs `stopper.Stop()` in a goroutine,
  waits up to `logs_config.stop_grace_period` seconds (default 30s), then calls
  `forceClose` (which calls `destinationsCtx.Stop()`, cancelling the HTTP context),
  then gives 5 more seconds, then dumps goroutines and exits. So the hang is
  bounded externally at ~35s (30s + 5s), but the panic path described in Path 1
  remains real within that window.
- Conclusion: the hang is bounded by the grace period (not by `provider.Stop()`
  itself). This partially mitigates the "shutdown hangs forever" concern but does
  not prevent the panic if the scheduler gets past `Wait()` and closes `InputChan`
  while the forwarder goroutine is mid-send.

#### Does `Sender.Stop()` ever get called more than once in any real code path?

- Examined: all call sites of `*.sender.Stop()` across the codebase —
  `comp/logs-library/pipeline/provider.go:297` (one call from `provider.Stop()`),
  `comp/forwarder/eventplatform/impl/epforwarder.go:714`, and
  `cmd/system-probe/modules/network_tracer.go:236` (different Senders, not logs).
  Searched for `sync.Once` in `comp/logs-library/sender/` and
  `comp/logs-library/pipeline/` — none found.
- Found: in the logs pipeline, `p.sender.Stop()` is called exactly once per
  `provider.Stop()` invocation. A double-Stop of `provider` itself would call
  `Sender.Stop()` twice (since `p.routerChannels = nil` prevents the router-close
  panic but not the sender close panic). No `sync.Once` in `Sender`, none in
  `provider`. The double-Stop path requires `provider.Stop()` to be called twice.
- Not found: a concrete production code path that calls `provider.Stop()` twice
  under normal operation. The restart path (`agent_restart.go`) calls
  `partialStop()` which stops the provider once, then rebuilds it; it does not
  call Stop twice on the same instance.
- Conclusion: double-Stop remains a latent hazard but requires `provider.Stop()`
  called twice, which is not observed in current production code. Could be
  triggered by concurrent signal-handler + API-driven stop under Antithesis.
</content>
