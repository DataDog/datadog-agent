# Discovery probe retry — design

## Problem

The discoverer (`comp/core/autodiscovery/discoverer/`) probes a service via the
Python `discover()` classmethod once per AD event for the matched template. If
the probe fails because the application inside the container is not yet
listening (HTTP connection refused on the metrics port), `discover()` returns
`None`, which the discoverer caches as a no-match for 30 s.

`reconcileService` is the only entry into the discoverer, and it is only
called from four places (`processNewService`, `processDelService`,
`processNewConfig`, `processDelConfigs`). There is no periodic
re-reconciliation. So a probe that fails because the application started
after the container did is permanent for the lifetime of that container — the
30 s TTL evicts the cache entry but no event re-fires `discover()`.

Reproduced end-to-end: a krakend container with `entrypoint: sh -c "sleep 60
&& krakend run …"` produces a single `python discover: krakend returned 4
bytes` log line at `t ≈ 2 s`, no further attempts, and the krakend check is
never scheduled — even after the application is fully listening on `:9090`
and well past the cache TTL. See `/tmp/krakend-delayed/` for the harness
(referenced; not committed).

This problem is unique to discovery templates. A normal manual
`auto_conf.yaml` resolves and schedules at AD-event time without probing; the
collector then runs the check on its interval (default 15 s). Connection
refused on the first run surfaces as `[ERROR]`; once the application is up,
the next run succeeds. Manual configs recover naturally because the collector
loop *is* the retry mechanism. Discovery templates have no such loop because
`discover()` gates scheduling — until it returns a hit, no check exists for
the collector to re-run.

## Goal

A bounded retry budget for failed discovery probes, so a service whose
application becomes ready within ~4 minutes of the container being added is
discovered and scheduled, while a genuinely-mismatched service (mislabelled,
or labelled `krakend` but actually `nginx:alpine`) gives up at a fixed cost
ceiling.

## Non-goals

- Running discovery off the configmgr lock. The known limitation that
  `cm.discoverer.Discover` is called under `cm.m` is unchanged here. Retries
  make this slightly worse over time — more probes-while-locked — but each
  probe is still milliseconds. An async `resolveTemplateForService` is its
  own change.
- Telemetry counters (attempts, give-ups, retry-successes). Worth adding in a
  follow-up; not required to validate the fix.
- Detecting "service is now ready" via container healthcheck transitions.
  Workloadmeta does track `ContainerHealth`, but `WorkloadService.Equal`
  doesn't compare it, so health flips don't currently produce AD events. A
  health-driven retry trigger is a separate, larger change.

## Approach

The retry budget lives in the discoverer cache; configmgr re-runs the
existing reconcile path on a fixed-interval ticker over the set of services
that have at least one pending (not-yet-given-up) discovery entry.

### Cache: retry budget

`comp/core/autodiscovery/discoverer/cache.go` — `cacheEntry` carries retry
state for failure entries. Success entries are unchanged.

```go
type cacheEntry struct {
    success      bool
    result       Result            // success only

    // failure-only:
    attemptsMade int               // count of failures so far
    nextRetryAt  time.Time         // zero when givenUp
    givenUp      bool              // attemptsMade > len(schedule)
}
```

`putFailure(svcID, integ)` increments `attemptsMade`. While
`attemptsMade <= len(schedule)`, `nextRetryAt = now + schedule[attemptsMade-1]`.
On the next call after the schedule is exhausted, `givenUp = true`.

Default schedule: `[5s, 5s, 30s, 30s, 30s, 30s, 30s, 30s, 30s, 30s]` — 10
retries on top of the initial probe (11 attempts total), total wait
window 5+5+30×8 = 250 s ≈ 4 min 10 s. The first two slots are tight to handle
the common ~10-30 s container-startup case quickly; the remaining slots use
the existing 30 s TTL value to keep the steady-state probe rate equivalent
to today.

`get(svcID, integ)` returns one of three explicit states:

- `hit-success` — Result + true.
- `pending` — failure entry, not yet given up; carries `nextRetryAt`.
- `givenUp` — failure entry, schedule exhausted.

`Forget(svcID)` — drops all entries for one service. The current cache has
no eviction on service removal (only TTL); `Forget` is wired from
`processDelService`. Without it, a stopped-and-restarted container with a
new container id and svcID would start fresh anyway, but a container
restarted in place by orchestration (same svcID, fresh app inside) would
still see stale entries until lazy eviction.

### Discoverer: cache-aware probe / skip

`Discover()` consults the cache state and decides whether to probe:

```
state := cache.get(svcID, integ)
if state.hit-success:        return cached, true
if state.givenUp:            return _, false           // no probe
if state.pending:
    if now < nextRetryAt:    return _, false           // budget not due
    // fall through, probe
result, err := bridge.RunDiscover(...)
on miss/error:               cache.putFailure         // advances schedule
on hit:                      cache.putSuccess
```

`Discover()` stays synchronous and stateless about timing. All retry state
lives in the cache; no goroutine in this package.

### configmgr: tick-driven reconcile of pending services

`autodiscoveryimpl/configmgr.go` — new field
`pendingDiscovery map[svcID]struct{}` populated whenever
`resolveTemplateForService` returns false for a `tpl.Discovery != nil`
template AND the discoverer reports the entry as pending (not given up).
Discoverer exposes a small predicate `IsPending(svcID, integ) bool` for
this.

A goroutine started during `autoConfig.Start()` and stopped on `Stop()`:

```go
ticker := time.NewTicker(5 * time.Second)
defer ticker.Stop()
for {
    select {
    case <-ctx.Done():
        return
    case <-ticker.C:
        cm.m.Lock()
        for svcID := range cm.pendingDiscovery {
            cm.reconcileService(svcID)         // existing path
        }
        cm.pruneGivenUp()                       // drop entries the discoverer reports as givenUp
        cm.m.Unlock()
    }
}
```

The 5 s tick matches the fastest retry slot; coarser ticks would miss the 5 s
boundaries by up to one tick interval. When `pendingDiscovery` is empty, each
tick is a noop (single map check). When non-empty, each tick acquires
`cm.m`, walks O(pending), and calls `reconcileService` for those svcIDs —
the cache short-circuits anything not yet due.

### Lifecycle

- **Service deleted** — `processDelService` calls `discoverer.Forget(svcID)`
  and `delete(cm.pendingDiscovery, svcID)`. (The lock-protected configmgr
  state and the discoverer cache are cleaned up together.)
- **Container restarted in place** — workloadmeta emits Set; AddService
  treats as updated (Equal=false on any change) → del+add fires → fresh
  budget on the new entry.
- **New discovery template added** that matches an existing service —
  `processNewConfig` already calls `reconcileService(svcID)`; the new
  `(svcID, integration)` pair gets a fresh budget independent of any
  existing entries for that service.
- **Multiple discovery templates per service** — tracked per
  `(svcID, integration)`; one giving up doesn't affect the others.
- **Successful late discovery during retry** — `cache.putSuccess`;
  `reconcileService` schedules the check via the existing path; svcID
  removed from `cm.pendingDiscovery`.

## What this changes vs. today

| Today                                                                     | After                                                                          |
| ------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| Failure cached for 30 s TTL.                                              | Failure entry tracks attempts on a `[5s, 5s, 30s × 8]` schedule.               |
| Cache entry only evicted lazily via TTL.                                  | Cache entry evicted lazily via `nextRetryAt` (pending) or never (givenUp).     |
| No re-trigger of `discover()` after 30 s for the same `(svcID, integ)`.   | configmgr's 5 s ticker re-runs `reconcileService` for pending services.        |
| Service removed → entries lazily expire via TTL.                          | Service removed → `Forget(svcID)` clears entries immediately.                  |
| `discoverer.Discover` synchronous, stateless about timing.                | Same — all timing logic in the cache.                                          |

## Defaults

Hardcoded for this iteration:

- Retry schedule: `[5s, 5s, 30s, 30s, 30s, 30s, 30s, 30s, 30s, 30s]`
- Configmgr ticker interval: `5s`

If config knobs are wanted later (`ad_discovery_retry_schedule`,
`ad_discovery_retry_tick_interval`), they're easy to add. Adding them now
expands surface without adding signal — no caller is asking for non-default
behavior yet.

## Tests

- `cache_test.go` — schedule progression (`putFailure` advances
  `attemptsMade` and `nextRetryAt`), give-up boundary (after the 10th
  failure, state is `givenUp`), `Forget(svcID)` clears all entries for a
  service.
- `discoverer_test.go` — cache-pending-not-due → no probe; cache-pending-due
  → probe; cache-given-up → no probe; success after pending replaces the
  failure entry.
- `configmgr_test.go` — `pendingDiscovery` populated when a discovery
  resolution fails with a pending entry; entry pruned when the discoverer
  reports given-up; the ticker fires `reconcileService` only for pending
  services.
- E2E (manual smoke, captured in the existing
  `2026-05-06-discover-e2e-smoke.md`): the delayed-krakend setup. First
  probe at AD-event time fails; subsequent retries fire at the next 5 s
  tick after each `nextRetryAt` (so the schedule slips by up to one tick
  interval). Krakend starts listening at ~`t+60s`; the next retry that
  fires after that point succeeds and the krakend check goes `[OK]`.

## Files touched

- `comp/core/autodiscovery/discoverer/cache.go` — entry shape, schedule,
  `Forget`.
- `comp/core/autodiscovery/discoverer/types.go` — exported `IsPending`.
- `comp/core/autodiscovery/discoverer/discoverer.go` — cache-aware
  `Discover()`.
- `comp/core/autodiscovery/discoverer/cache_test.go`,
  `comp/core/autodiscovery/discoverer/discoverer_test.go` — unit tests.
- `comp/core/autodiscovery/autodiscoveryimpl/configmgr.go` —
  `pendingDiscovery`, `Forget` wiring on delete, ticker goroutine.
- `comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go` — start/stop the
  goroutine.
- `comp/core/autodiscovery/autodiscoveryimpl/configmgr_test.go` — pending
  set + ticker tests.
