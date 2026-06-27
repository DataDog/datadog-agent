---
slug: idempotent-stop
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# idempotent-stop — Stop() Is Safe to Call More Than Once

## What Led to This Property

The SUT analysis §9 (Unproven Assumption 5) flags "Sender.Stop() called exactly
once" as a dangerous implicit assumption. The project review guidelines identify
double-stop/close-of-closed as a top bug class. Several components lack
`sync.Once` protection on their `Stop()` methods. A double-Stop is a plausible
event during error-recovery paths, test teardown, or when shutdown is initiated
concurrently from multiple places (e.g., an API call and a signal handler).

## Code Paths Involved

### `Sender.Stop()` — no `sync.Once`

`comp/logs-library/sender/sender.go:179-188`:
```go
func (s *Sender) Stop() {
    log.Debug("sender mux stopping")
    for _, s := range s.workers {
        s.stop()      // sends to s.done chan
    }
    for _, q := range s.queues {
        close(q)      // PANICS if q is already closed
    }
}
```

A second call to `Stop()` attempts `close(q)` on already-closed channels →
panic. The `Sender` has no `stopped` flag, no `sync.Once`, no `select { default
}` guard.

### `provider.Stop()` — no `sync.Once`

`comp/logs-library/pipeline/provider.go:283-299`:
```go
func (p *provider) Stop() {
    stopper := startstop.NewParallelStopper()
    for _, ch := range p.routerChannels {
        close(ch)     // PANICS on second call if routerChannels not nil
    }
    ...
    p.routerChannels = nil  // nil-out after use
}
```

After `p.routerChannels = nil`, a second call would not panic on the `close`
loop (range over nil slice is a no-op), but it would call `stopper.Stop()` on
already-stopped pipelines, whose `InputChan` channels are already closed. This
propagates to `processor.Stop()` which would close `inputChan` again → panic.

### `Launcher.Stop()` — protected

The file launcher uses `stopOnce sync.Once` (`launcher.go:70`, called at
`launcher.go:141-149`). This is the correct pattern. The auditor
`closeChannels()` uses a nil-guard on `a.inputChan` (lines 173-177), providing
protection for the auditor. The worker `stop()` uses a synchronous send (not a
channel close), which is safe to call twice if the goroutine is still running
(blocks), but panics if the worker goroutine has already exited and `finished`
is closed.

### `DestinationSender.Stop()` — partial guard

`comp/logs-library/sender/destination_sender.go:72-76`:
```go
func (d *DestinationSender) Stop() {
    close(d.input)      // PANICS if already closed
    <-d.stopChan
    close(d.retryReader) // PANICS if already closed
}
```

No nil-guard, no `sync.Once`.

## Triggering Interleaving

- Signal handler and API handler both call `agent.Stop()` concurrently under
  Antithesis thread scheduling.
- Error recovery in a failed-start scenario calls `Stop()` on a partially-started
  pipeline, then `Stop()` is called again from the parent.
- Test teardown runs `Stop()` twice to verify idempotency (currently this would
  panic for `Sender`).

## Why It Matters

A double-Stop panic terminates the process during shutdown — the worst possible
moment because the auditor registry may not be flushed. The panic trace is
misleading (appears as a channel operation, not a logic error). Under Antithesis
fault injection, concurrent shutdown signals are routine.

## SUT-Side Instrumentation (all missing)

- `Unreachable("sender-stop-double-close-panic")` — wrap `Sender.Stop()` in
  a recover() and fire this assertion if panic is caught.
- `Unreachable("destination-sender-stop-double-close-panic")` — same for
  `DestinationSender.Stop()`.
- `Reachable("sender-stop-completed-idempotently")` — placed after a second
  `Stop()` call in a controlled test path to confirm no panic.
- Workload: issue two concurrent `Stop()` calls on the pipeline provider; assert
  no panic is observed (workload-level `Always` on "process stayed alive").

## Open Questions

- Is there any real production code path that calls `Sender.Stop()` twice? The
  SUT analysis tags this as an "unproven assumption" (H6) but does not cite a
  specific triggering call site.
  `(partial: no concurrent call site found in production code; double-Stop requires provider.Stop() called twice, which is not observed in normal operation — see Investigation Log)`
- The file launcher's `stopOnce` pattern is the obvious fix for all of these.
  Is there a project-wide policy requiring `sync.Once` on `Stop()` methods? If
  yes, the missing instances are straightforward bugs to file; if not, this is
  a design decision to escalate. `(needs human input)`

### Investigation Log

#### Is there any real production code path that calls `Sender.Stop()` twice?

- Examined: all call sites of `sender.Stop()` in the logs pipeline —
  `comp/logs-library/pipeline/provider.go:297` (sole caller for the logs
  `Sender`). Checked `provider.Stop()` for `sync.Once` — none. Checked the
  restart path in `agent_restart.go`: `partialStop()` stops the provider once
  and then `rebuildTransientComponents()` creates a new `pipelineProvider` and
  new `Sender`. The old sender is stopped once; the new sender is a different
  object. No double-Stop observed.
- Found: the container launcher's `Stop()` uses `sync.Once` (line 69, 98-108).
  `Sender.Stop()`, `provider.Stop()`, and `DestinationSender.Stop()` have no
  `sync.Once` or nil-guard.
- Not found: any production path that calls `Sender.Stop()` or
  `DestinationSender.Stop()` twice.
- Conclusion: the double-Stop risk is a latent bug triggered by concurrent
  stop events (e.g., OS signal + API call both reaching `agent.Stop()`
  concurrently). Updated from "unproven assumption" to
  `(partial: no production double-call observed; concurrency-triggered path remains possible)`.

#### Does `startstop.ParallelStopper` protect against being called twice?

- Examined: `pkg/util/startstop/parallel_stopper.go`.
- Found: `parallelStopper.Stop()` iterates over `g.components`, calling each
  component's `Stop()` in a goroutine. No `sync.Once`, no nil-guard on
  `components`. A second call to `parallelStopper.Stop()` re-invokes all
  component `Stop()` methods. Since a new `parallelStopper` is constructed
  inside each `provider.Stop()` call, the stopper itself is not reused — but
  the pipeline objects it wraps are the same. A second `provider.Stop()` call
  would create a second stopper wrapping the same pipeline objects and call
  their `Stop()` again.
- Conclusion: the fresh-stopper construction provides no protection against
  double-stopping the underlying pipeline components. Existing partial tag
  confirmed correct.
</content>
