---
slug: no-services-store-deadlock
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# no-services-store-deadlock — Services Store Never Deadlocks Under Concurrent Add/Remove

## What Led to This Property

`pkg/logs/service/services.go` holds `s.mu` across unbuffered channel sends to
all registered subscriber channels (`AddService:42-44`, `RemoveService:62-64`).
If any subscriber goroutine stops consuming its channel while the lock is held,
every concurrent caller of `AddService` or `RemoveService` blocks, stalling the
entire service-discovery layer.

The contrast with `LogSources` is instructive: `sources.go:63` explicitly
releases the lock *before* iterating over subscribers and uses `select { case
stream.ch <- source: case <-stream.done: }` to skip stopped consumers.
`Services` does neither — it holds the lock during the sends and has no done-
channel escape hatch.

## Code Paths Involved

```
pkg/logs/service/services.go

func (s *Services) AddService(service *Service) {
    s.mu.Lock()          // line 36 — lock acquired
    defer s.mu.Unlock()  // released AFTER sends

    s.services = append(s.services, service)
    added := s.addedPerType[service.Type]
    for _, ch := range append(added, s.allAdded...) {
        ch <- service    // lines 42-44 — BLOCKING send while holding lock
    }
}
```

Same pattern in `RemoveService` (lines 62-64).

Subscribers receive channels from:
- `GetAddedServicesForType` → unbuffered `chan *Service`
- `GetAllAddedServices` → unbuffered `chan *Service`
- `GetRemovedServicesForType` → unbuffered `chan *Service`
- `GetAllRemovedServices` → unbuffered `chan *Service`

All returned channels are **unbuffered**.

## Triggering Interleaving

1. Subscriber goroutine (e.g., the container launcher) is draining its added-
   services channel.
2. CPU scheduling or a fault pauses the subscriber goroutine mid-drain.
3. Autodiscovery calls `AddService` on a second goroutine while the first is
   paused.
4. `AddService` acquires `s.mu` and blocks on `ch <- service` waiting for the
   paused subscriber.
5. Any other goroutine needing `s.mu` (e.g., a concurrent `RemoveService` or
   `GetAddedServicesForType` call) is now deadlocked until the subscriber wakes.

Under Antithesis thread-pause faults this is deterministically reachable.

## Why It Matters

A deadlock in `Services` blocks all service lifecycle operations. Launchers
waiting on service notifications freeze; no new containers are picked up; no
removed containers are cleaned up. The agent appears alive (health checks pass)
but silently stops picking up new log sources.

## SUT-Side Instrumentation (all missing)

- `Reachable("services-addservice-send-completed")` inside the send loop in
  `AddService` — confirms the happy path is reachable.
- `Unreachable("services-lock-held-during-blocked-send")` — harder to place
  directly; a timeout-based watchdog that fires if `AddService` takes > N ms
  while holding the lock would serve as a proxy.
- Workload-level: confirm that after every `AddService` call the subscriber
  eventually receives the event (liveness check via `Sometimes`).

## Open Questions

- Is there a deployment topology where `Services` is exercised at high rate
  (e.g., rapid container churn)? Low churn makes this race narrow but not
  impossible under Antithesis scheduling. `(needs human input)`
- Can Antithesis reproduce the network-faulted Docker API scenario needed to
  make the tailer-factory block? The Docker socket is a Unix domain socket —
  does Antithesis support faulting Unix socket connections, or only TCP?
  `(needs human input)`

### Investigation Log

#### Which launchers subscribe via `GetAllAddedServices` vs. `GetAddedServicesForType`?

- Examined: full codebase search for all call sites of `GetAllAddedServices`,
  `GetAddedServicesForType`, `GetRemovedServicesForType`, `GetAllRemovedServices` in
  all `.go` files outside `services_test.go`.
- Found: **zero production callers**. All four methods are defined in
  `pkg/logs/service/services.go` and called only from `services_test.go`. No
  launcher, scheduler, or other component subscribes to `Services` via these
  methods in the current codebase.
- Not found: any call sites in `pkg/logs/launchers/`, `comp/logs/`, or elsewhere.
- Conclusion: The deadlock-under-blocked-send scenario requires a subscriber. With
  zero subscribers the `AddService`/`RemoveService` loops `for _, ch := range
  append(added, s.allAdded...)` iterate over empty slices — no sends occur, no
  lock-held blocking. The deadlock path described in the property body is
  **currently unreachable**. However the code structure (unbuffered channels,
  lock held during sends) remains a latent hazard if subscribers are added.
  Property scope narrows: the present property tests the Services *API contract*
  (would deadlock if subscribers were added), not a live production defect.

#### Does `LogSources.AddSource` release the mutex before sending, confirming `Services` differs?

- Examined: `pkg/logs/sources/sources.go:53-78`.
- Found: `LogSources.AddSource` captures `streams` and `streamsForType` under the
  lock, calls `s.mu.Unlock()` at line 63 *before* the send loops, and uses
  `select { case stream.ch <- source: case <-stream.done: }` for each. Correct
  pattern — lock released before blocking sends, and stopped subscribers are
  skipped via `stream.done`.
- Conclusion: confirmed that `Services` differs from `LogSources` in exactly the
  ways described. The `Services` bug is real but dormant (zero subscribers).

## Merged-in evidence (from services-store-no-progress-loss)

The secondary file covered the **liveness / untailed-logs facet**: a progress
stall without full deadlock that causes log lines to be produced but never
delivered. This canonical file now covers both the safety (deadlock) and liveness
(untailed logs) facets.

### Liveness facet: slow consumer induces service-event backpressure → silent log loss

1. Container launcher's `run()` goroutine is processing a slow tailer start (e.g.,
   tailer factory blocked on a network-faulted Docker API).
2. A burst of container churn events arrives: C1 added, C2 removed, C3 added, C4
   removed, all in quick succession.
3. `AddService(C1)` holds `s.mu` and blocks waiting for the launcher's channel.
4. `AddService(C3)` and both `RemoveService` calls queue up (need `s.mu`).
5. During the stall, C1–C4 are emitting logs to files that have no active tailer.
6. When the stall resolves, tailers start — but at "current position" (end of file
   per default `TailingMode=end`), **skipping all logs written during the stall**.
7. No metric or error is emitted for the skipped lines.

This is not a deadlock — it resolves eventually. But logs are silently lost.

### Duplicate-service-delivery via `GetAddedServicesForType` goroutine race

`services.go:70-87`: when a new subscriber calls `GetAddedServicesForType`, a
goroutine is started to replay existing services. This goroutine sends to `added`
without holding `s.mu`. If `AddService` is already blocked waiting to send on
`added`, the goroutine and `AddService` both try to send on the same channel.
The subscriber receives the service twice → two tailers started for the same
container → identifier collision (same failure mode as
`container-identifier-no-collision`, triggered by a software bug rather than
file rotation).

### Additional code paths (from secondary)

- `pkg/logs/service/services.go:70-87` — `GetAddedServicesForType` goroutine race
- `pkg/logs/launchers/container/launcher.go:112-138` — `run()` / `loop()` — the
  consumer side

### Additional assertions (from secondary)

**Progress invariant** (workload-side): after fault-inject window ends and agent
has 30s fault-free operation:
```
lines_at_fakeintake + lines_in_drop_metrics >= N_written
```

**Duplicate service delivery** (SUT-side, `Always`):
```go
assert.Always(
    !existingTailerFound || isReplacement,
    "cannot start a second tailer for an already-tracked source",
    map[string]any{"sourceID": source.Config.Identifier})
```

**Stall reachability** (SUT-side, `Sometimes`):
```go
assert.Sometimes(servicesStoreSendBlocked,
    "services store was observed blocking on channel send", nil)
```
</content>
