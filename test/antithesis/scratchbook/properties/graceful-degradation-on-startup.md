---
slug: graceful-degradation-on-startup
focus: "8 — Lifecycle Transitions"
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# Property: graceful-degradation-on-startup

## What led to this property

sut-analysis.md §9 (unproven assumption 4): "`DestinationsContext.Start()` runs
before any `Send`" — if this ordering is violated, `Context()` returns nil, which
would cause a nil-pointer dereference in the HTTP destination when it calls
`d.destinationsContext.Context()` (http/destination.go:328).

More broadly, several startup dependencies can fail or be unavailable:
1. **Intake unreachable at startup** — `buildEndpoints()` (agent_core_init.go:60)
   performs an HTTP connectivity check. If it fails, it falls back to TCP. If both
   fail, the agent returns an error from `start()` and does not start the pipeline.
   This is documented behavior but the recovery path (smart HTTP retry) needs
   fault-injection validation.
2. **journald unavailable at startup** — `journald.Launcher` calls `setup()`, which
   opens the journald socket. If the socket isn't available, the source is flagged
   with an error and the tailer is not started. No retry or reconnect mechanism
   exists for mid-session journald failure.
3. **Docker API socket unavailable at startup** — container launcher may fail to
   initialize its tailer factory. Graceful fallback exists to file tailing.
4. **Auditor directory not writable** — `flushRegistry()` will log a warning and
   continue; the agent starts but registry writes silently fail. No health check
   fails.

The key startup ordering in `startPipeline()` (agent.go:279-292):
```
startstop.NewStarter(
    a.destinationsCtx,    // Start() creates context
    a.auditor,            // Start() creates channels, recovers registry, starts run()
    a.pipelineProvider,   // Start() creates pipelines and senders
    a.diagnosticMessageReceiver,
    a.launchers,          // Start() starts tailers
)
```
The `startstop.Starter` starts components sequentially in this order. So
`destinationsCtx.Start()` is called before `pipelineProvider.Start()`, which is
before `launchers.Start()`. This ordering prevents the nil context panic — but
only if `startstop.NewStarter` is actually sequential.

## Key code locations

- `comp/logs/agent/agentimpl/agent.go:279-292` — `startPipeline()` and the
  Starter ordering.
- `comp/logs-library/client/destinations_context.go:27-31` — `Start()` creates
  context.
- `comp/logs-library/client/http/destination.go:328` — `d.destinationsContext.Context()`
  called during send; nil would panic.
- `comp/logs/agent/agentimpl/agent_core_init.go:60-73` — `buildEndpoints()`:
  connectivity check → fallback to TCP.
- `pkg/logs/launchers/journald/launcher.go` — journald setup (no reconnect).

## What fault triggers it

**Dependency unavailable at startup** — network partition to intake at startup
time tests the TCP fallback path. Journald socket removed at startup tests the
graceful journald skip. Docker socket removed mid-session tests the container
launcher degradation.

**CPU throttling during startup** can expose ordering races if `startstop.Starter`
is not strictly sequential — worth verifying.

## Why it matters

An agent that panics or deadlocks during startup (or fails to start at all when
a dependency is unavailable) does not collect any logs. Graceful degradation —
starting with reduced capability and recovering when dependencies come back — is
critical for reliability.

## Assertions needed (all net-new SUT instrumentation)

1. **`Unreachable(nil context dereference in HTTP destination)`** — the existing
   nil check in `destinations_context.go:Context()` returns nil if `Start()` was
   not called. An `Unreachable` assertion at this nil return point would catch any
   ordering violation. This is a "must never be hit" assertion.
2. **`Reachable(TCP fallback taken at startup)`** — SUT-side in `buildEndpoints()`
   when `httpConnectivity == HTTPConnectivityFailure` and TCP endpoints are returned.
   Confirms the fallback path is exercised.
3. **`Sometimes(agent started with degraded capabilities: journald unavailable)`** —
   SUT-side in journald launcher when it skips a source due to setup failure: a
   `Sometimes` assertion that this degraded-but-running state was reached.
4. **Workload: `Always(agent health endpoint responds within T seconds of startup)`**
   — after startup with faulted dependencies, the agent's health endpoint should
   still respond (degraded, not crashed). This tests graceful degradation rather
   than availability.

## Recovery window requirement

Fault-quiet window needed after dependency recovery to confirm logs resume
flowing. For journald, this may require an agent restart since no reconnect exists.

## Open questions

- When HTTP connectivity check fails and TCP fallback is taken, does the
  `smartHTTPRestart` goroutine start immediately? If so, it will attempt HTTP
  connectivity checks concurrently with the TCP pipeline's operation — this
  concurrent check adds goroutine load at startup.
  `(partial: confirmed smartHTTPRestart starts immediately in agent.go:223, spawning a background httpRetryLoop goroutine with exponential backoff; goroutine leaks if agent stops before HTTP upgrade completes)`

### Investigation Log

#### Is `startstop.Starter` strictly sequential?

- Examined: `pkg/util/startstop/starter.go`.
- Found: `NewStarter` returns a `*starter` with `Start()` implementing a plain
  sequential for-loop: `for _, c := range s.components { c.Start() }`. No
  goroutines, no concurrency. Components are started one-by-one in the order
  they were added.
- Conclusion: **confirmed strictly sequential**. The `startPipeline()` order in
  `agent.go:279-292` — `destinationsCtx`, `auditor`, `pipelineProvider`,
  `diagnosticMessageReceiver`, `launchers` — is guaranteed by `NewStarter`.
  `DestinationsContext.Start()` runs and returns before `pipelineProvider.Start()`
  is called. The nil-context panic in `http.Destination.unconditionalSend`
  (line 328) is not reachable via a startup ordering race under normal conditions.
  Remove the `(partial:)` tag — this question is resolved. Property confidence
  in the ordering guarantee is now high; the nil-context path can only be reached
  if `Start()` is skipped entirely or if `DestinationsContext` is replaced mid-run.
