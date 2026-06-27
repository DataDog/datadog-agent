---
slug: no-goroutine-leak-after-stop
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# no-goroutine-leak-after-stop — No Goroutine Leak After Component Stop

## What Led to This Property

The SUT analysis §3 identifies goroutine leaks as one of the top two bug
classes in the logs agent (alongside send-on-closed-channel). Bug history (§6)
documents concrete leaks: `86882e6e718` (file tailer leak), `90560d965b0`
(batch shutdown race leaving a goroutine), `f7cf97529ac` (destination sender
goroutine not joined). Antithesis is uniquely positioned to explore these paths
because it can pause goroutines at arbitrary scheduling points and verify the
total goroutine count after stop.

## Code Paths Involved

### Leak 1 — `noopDestinationsSink` goroutine in `worker.run()`

`comp/logs-library/sender/worker.go:213-221`:
```go
func noopDestinationsSink(bufferSize int) chan *message.Payload {
    sink := make(chan *message.Payload, bufferSize)
    go func() {
        for msg := range sink {
            _ = msg
        }
    }()
    return sink
}
```

`noopSink` is created inside `run()` (line 102). At worker shutdown
(line 208): `close(noopSink)` terminates the drain goroutine. But if the
worker exits via panic (or is interrupted before reaching line 208), the
drain goroutine leaks indefinitely waiting on a channel that has no
remaining writers.

### Leak 2 — `startRetryReader` goroutine in `DestinationSender`

`comp/logs-library/sender/destination_sender.go:55-69`:
```go
func (d *DestinationSender) startRetryReader() {
    go func() {
        for v := range d.retryReader {
            ...
        }
    }()
}
```

`Stop()` (line 72-76) calls `close(d.input)`, waits on `<-d.stopChan`, then
`close(d.retryReader)`. The goroutine terminates on `retryReader` close. Risk:
if `Stop()` panics between `close(d.input)` and `close(d.retryReader)`, the
retry goroutine leaks. Also: if `Stop()` is never called (destination abandoned
during shutdown), the goroutine leaks until process exit.

### Leak 3 — Container launcher `WrappedSource` goroutines

`pkg/logs/launchers/container/tailerfactory/tailers/source.go:34,42`:
```go
func (t *WrappedSource) Start() error {
    go t.Sources.AddSource(t.Source)   // fire-and-forget goroutine
    return nil
}
func (t *WrappedSource) Stop() {
    go t.Sources.RemoveSource(t.Source) // fire-and-forget goroutine
}
```

These goroutines are spawned to avoid the self-deadlock described in the
comment. They are unbounded: if `AddSource` or `RemoveSource` blocks (e.g.,
because a `Services.AddService` subscriber is slow — see
`no-services-store-deadlock`), these goroutines hang indefinitely. Each
container start/stop spawns one. Under container churn, hundreds of leaked
goroutines accumulate.

### Leak 4 — `readForever` goroutine when `forwardMessages` is stuck

If `forwardMessages` is stuck on `outputChan <- msg` with no consumer, and
a non-rotation `Stop()` never calls `stopForward()`, then `readForever` is
signalled via `t.stop` and exits — but `t.decoder.Stop()` is called from
`readForever`'s defer. The decoder's `OutputChan()` may not be fully drained
if `forwardMessages` is blocked mid-range. The range loop does not exit until
the channel is closed AND drained. So the `done` channel is never closed, and
the outer `<-t.done` in `Tailer.Stop()` hangs. The `forwardMessages` goroutine
effectively leaks for the duration of the hang.

## Triggering Scenarios

- Antithesis pauses the consumer of a DestinationSender, triggers shutdown,
  observes whether the retry goroutine exits.
- Antithesis triggers rapid container churn (many Start+Stop events) and then
  pauses `Sources.AddSource` to produce slow consumers; counts goroutines before
  and after to detect accumulation.
- Antithesis sends SIGTERM while the pipeline is under load and verifies that
  goroutine count returns to the idle baseline within T seconds.

## Why It Matters

Goroutine leaks cause memory growth proportional to leak rate, exhaust file
descriptors (goroutine stacks hold open file handles), and hide real bugs
behind apparent liveness. In long-running soak tests they surface as OOM or
resource exhaustion.

## SUT-Side Instrumentation (all missing)

- `Reachable("noop-sink-goroutine-cleanly-exited")` — at the drain goroutine's
  cleanup point (end of `for msg := range sink` loop).
- `Reachable("retry-reader-goroutine-cleanly-exited")` — at the end of
  `startRetryReader`'s loop.
- Workload: query `runtime.NumGoroutine()` before and after stop; assert the
  count returns to within N of baseline with `Always` at the workload layer.
- `Sometimes("goroutine-count-elevated-under-load")` — to confirm the test
  actually drove goroutine count above baseline (exercise coverage).

## Open Questions

- What is the expected baseline goroutine count after a clean stop? The SUT
  analysis estimates ~40+ goroutines at steady state, which all drain on stop.
  The exact post-stop floor depends on static global goroutines (telemetry,
  log package, etc.) that aren't part of the pipeline lifecycle. `(needs human input)`
- Does the `WrappedSource` goroutine approach (fire-and-forget to avoid
  deadlock) have any known bound? The comment says "long-term fix is that
  launchers should not be adding sources" — meaning this is a known technical
  debt, not an intended design. The leak surface is proportional to container
  churn rate.
- Do Antithesis goroutine-count checks work across the process boundary (i.e.,
  can the workload driver query goroutine counts from the SUT process), or does
  this require SUT-side instrumentation only? `(needs human input)`
- Is `pipeline_failover.enabled` exercised in the test topology? The H4 panic
  (forwardWithFailover blocked on InputChan send during shutdown) is specific to
  failover mode. `(needs human input)`

### Investigation Log

#### Is there a shutdown timeout anywhere in the agent's `Stop()` sequence?

- Examined: `comp/logs/agent/agentimpl/agent.go:340-380`,
  `pkg/config/setup/common_settings.go:1869`.
- Found: `stopComponents` wraps the serial stopper in a goroutine with a
  `logs_config.stop_grace_period` timeout (default 30s). On expiry it calls
  `destinationsCtx.Stop()` (context cancellation), waits another 5s, then
  dumps goroutines and exits. Shutdown is bounded at ~35s.
- Conclusion: goroutine leaks that outlive the 35s window are masked by process
  exit. Antithesis should test that goroutine count returns to baseline well
  within that window, not rely on the timeout as clean exit.

#### Does `http.Destination.run()` respect `destinationsContext.Context()` cancellation?

- Examined: `comp/logs-library/client/http/destination.go:228-320`,
  `comp/logs-library/client/http/destination.go:553-557` (`waitForBackoff`),
  `comp/logs-library/client/destinations_context.go`.
- Found: `destinationsContext.Stop()` calls `dc.cancel()`, cancelling the
  context returned by `Context()`. Three paths use this context:
  (1) `unconditionalSend` at line 328 gets `ctx` and passes it to the HTTP
  request via `req.WithContext(ctx)` — if the request is in-flight, it is
  cancelled by the transport; the error is `context.Canceled`, which causes
  `sendAndRetry` to exit at line 298.
  (2) `waitForBackoff` at line 554 uses `context.WithDeadline(d.destinationsContext.Context(), ...)` —
  cancellation unblocks the `<-ctx.Done()` immediately.
  (3) `run()` loop reads from `input` channel; context cancellation does not
  directly unblock a channel read, but `DestinationSender.Stop()` closes
  `d.input` separately.
- Conclusion: context cancellation **does** unblock retrying/backing-off
  destinations and in-flight HTTP sends. The `sendAndRetry` loop exits on
  `context.Canceled`. This means when `destinationsCtx.Stop()` is called
  (either at grace period timeout or in `partialStop`), the `wg.Wait()` in
  `destination.run()` resolves. Goroutine leak risk for HTTP destinations is
  lower than the evidence file suggested.

#### Does the `WrappedSource` fire-and-forget goroutine have any join mechanism?

- Examined: `pkg/logs/launchers/container/tailerfactory/tailers/source.go`.
- Found: `WrappedSource.Start()` spawns `go t.Sources.AddSource(t.Source)` with
  no wait group, no channel return, no tracking. `WrappedSource.Stop()` similarly
  spawns `go t.Sources.RemoveSource(t.Source)` fire-and-forget. No sync mechanism
  exists. The comment at line 27-33 explicitly acknowledges the goroutine is
  unbounded: "The long-term fix is that launchers should not be adding sources."
- Not found: any join or cancellation for these goroutines.
- Conclusion: confirmed — zero join mechanism. Goroutines block until
  `LogSources.AddSource`/`RemoveSource` returns. Since `LogSources` uses
  `select { case stream.ch <-: case <-stream.done: }`, sends are bounded by the
  subscriber's `done` channel. However if `stream.done` is not closed (e.g.,
  a subscriber goroutine that has stopped consuming but hasn't closed `done`),
  the goroutine leaks indefinitely.

## Merged-in evidence (from shutdown-no-goroutine-leak)

The secondary file provided a **complementary goroutine inventory** and named
additional high-risk shutdown paths not present in the canonical:

**Goroutine inventory** (per 4-pipeline HTTP agent):
- 1 auditor `run()` goroutine
- 4 processor `run()` goroutines (one per pipeline)
- 4 batch-strategy goroutines
- Sender worker goroutine(s)
- `noopDestinationsSink` drain goroutine
- `startRetryReader` goroutines (one per DestinationSender)
- `http.Destination.run` goroutines (one per destination)
- Up to `maxConcurrency` concurrent HTTP send goroutines (dynamic worker pool)
- 2 per file tailer (`readForever`, `forwardMessages`)
- Launcher scan goroutines
- `forwardWithFailover` goroutines (if failover enabled, one per pipeline)

**Additional high-risk shutdown paths (from secondary):**

**H4 — `forwardWithFailover` hang with send-on-closed-channel:**
`provider.go:353-366` — if the forwarder goroutine is blocked on
`p.pipelines[primaryPipelineIndex].InputChan <- msg` (the backpressure fallback
at line 361), closing `routerChannels[i]` does NOT unblock it. Then
`pipeline.Stop()` closes `InputChan`, causing a send-on-closed-channel panic at
line 361. `forwarderWaitGroup.Wait()` at `provider.go:289` hangs indefinitely if
neither the send completes nor a panic is caught.

**H6 — `DestinationSender.Stop()` double-close:**
`destination_sender.go:72-76` — `Stop()` closes `d.input` then `d.stopChan`.
No `sync.Once` guards the close. A second `Stop()` call panics on
close-of-closed-channel.

**H2 — `worker.stop()` blocking:**
`worker.go:96-99` — `stop()` sends to `s.done` and waits on `s.finished`. If any
destination goroutine hangs (e.g., blocked in `sendAndRetry` waiting for a context
that is never cancelled), `Worker.run()` also hangs at `wg.Wait()` (line 247).

**Additional workload instrumentation (from secondary):**
After each graceful shutdown cycle, check goroutine count via
`/debug/pprof/goroutine`. SUT-side: `Always` assertion in `provider.Stop()`
confirming `forwarderWaitGroup.Wait()` returns within a timeout.
</content>
