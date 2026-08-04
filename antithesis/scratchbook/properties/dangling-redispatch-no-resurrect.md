# dangling-redispatch-no-resurrect — Dangling re-dispatch never resurrects an unscheduled config

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P1 · **Intent:** invariant

**Provenance:** merged from 1 discovery agent(s): dangling-redispatch-resurrects-unscheduled-config

## Property

A cluster-check config that AutoDiscovery removed via Unschedule is never re-added to the dispatch store by the periodic dangling re-dispatch loop.


## Invariant / assertion

`assert.AlwaysOrUnreachable`: a digest removed by Unschedule/removeConfig never reappears in digestToConfig/digestToNode via reschedule. AlwaysOrUnreachable fits — the re-dispatch path is periodic/optional, but whenever it runs it must not resurrect a removed config.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: an Unschedule interleaved the retrieveDangling→reschedule→deleteDangling span.


## Antithesis angle

The dangling re-dispatch sequence retrieveDangling()→reschedule→deleteDangling is NOT under a single store lock across the whole span (dispatcher_main.go:405-411); a concurrent Unschedule can interleave. A config Unscheduled mid-re-dispatch could be re-added (resurrected) or a live config dropped. Interleave AD Unschedule with the 15s cleanup tick during node churn.


## Why it matters

A resurrected check keeps collecting metrics for a config the user deleted — a correctness and cost problem that is invisible until someone notices duplicate/zombie data.


## Fault dependencies

- node expiry to populate danglingConfigs (network partition; enabled by default)
- concurrent AutoDiscovery Unschedule
- requires leader_election enabled + >=2 replicas


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.AlwaysOrUnreachable` in reschedule checking the digest is still present in digestToConfig (not tombstoned) before re-adding; or a tombstone set asserted against.


## Open questions (post-investigation)

- Frequency/latency of AD Unschedule relative to the cleanup cadence is runtime/environment-dependent and not derivable from code. Mechanism confirmed: the resurrect window is the gap between retrieveDangling (RUnlock, dispatcher_configs.go:217-222) and the per-config addConfig inside reschedule->add (each takes d.store.Lock separately); AD Unschedule runs on the AutoConfig single worker goroutine (controller.go:151-215) contending only on d.store.Lock. Reaching it requires Antithesis interleaving control, not wall-clock luck. `(partial)`


### Investigation Log

#### Frequency/latency of AD Unschedule relative to the 15s cleanup cadence.

Examined dispatcher_main.go:385 (cleanupTicker=nodeExpirationSeconds/2), :400-411 (compound op drops store lock between retrieveDangling and reschedule->addConfig), dispatcher_configs.go:120-166 (addConfig never checks digest still in danglingConfigs/known), controller.go:151-215 (AD single-worker Schedule/Unschedule under ms.m). Found: mechanism/window fully confirmed; the actual Unschedule frequency is deployment-dependent and unknowable from code. Conclusion: PARTIAL - window mechanism resolved, timing needs runtime data / Antithesis interleaving.

#### Whether endpoints configs have an analogous resurrection path.

Examined dispatcher_endpoints_configs.go (addEndpointConfig/removeEndpointConfig only), dispatcher_main.go:196/:248 (called ONLY directly from Schedule/Unschedule), dispatcher_nodes.go:142-186 (expireNodes iterates node.digestToConfig, NOT endpointsConfigs), dispatcher_configs.go:216-235 (retrieveDangling/deleteDangling touch only danglingConfigs). Found: endpoints are never placed in danglingConfigs and have no reschedule path. Conclusion: RESOLVED - no analogous resurrection path exists for endpoints configs.


---

## Source discovery evidence (raw, per contributing agent)


### from `dangling-redispatch-resurrects-unscheduled-config`

## Mechanism (verified)

**Non-atomic compound op in the cleanup tick** (dispatcher_main.go:400-411):
```
case <-cleanupTicker.C:
    d.expireNodes()
    if d.shouldDispatchDangling() {
        danglingConfigs := d.retrieveDangling()      // RLock, snapshot, UNLOCK
        scheduledConfigIDs := d.reschedule(danglingConfigs) // per-config Lock/Unlock
        d.store.Lock(); d.deleteDangling(scheduledConfigIDs); d.store.Unlock()
    }
```
- `retrieveDangling` (dispatcher_configs.go:216-222) copies configs then releases the lock.
- `reschedule` (dispatcher_main.go:262-271) calls `d.add(c)` for each; `add`->`addConfig` (dispatcher_configs.go:120-166) each take `d.store.Lock()` separately and, for a config with a found target node, unconditionally do `d.store.digestToConfig[digest] = config` and `d.store.digestToNode[digest] = targetNodeName` and dispatch. **addConfig never checks that the digest is still in danglingConfigs / still a known config.**

**Concurrent remover** (dispatcher_configs.go:168-204): `removeConfig` (from `Unschedule`, dispatcher_main.go:217-259) deletes the digest from digestToConfig, digestToNode, idToDigest, and dangling, all under `d.store.Lock()`.

**Goroutine independence** (verified): `dispatcher.run` is launched as its own goroutine (`go h.runDispatch` -> `h.dispatcher.run(ctx)`, handler.go:152,185) while AutoDiscovery drives `Schedule`/`Unschedule` from the autoconfig scheduler goroutine (`AddScheduler(..., h.dispatcher, true)`, handler.go:182). They contend only through the store lock, which the cleanup compound op drops between steps.

## Failure scenario (interleaving)
| step | dispatcher.run | AD scheduler |
|---|---|---|
| 1 | expireNodes: node N gone, C -> danglingConfigs | |
| 2 | retrieveDangling -> [C]; RUnlock | |
| 3 | | Unschedule(C) -> removeConfig(C): C purged everywhere |
| 4 | reschedule([C]) -> addConfig(C, M): re-inserts C, dispatches to node M | |
| 5 | deleteDangling([C]): no-op | |

Result: C runs on node M indefinitely; digestToConfig[C] and digestToNode[C]=M exist though AD unscheduled C. The proposed assertion (digest present in danglingConfigs at addConfig-from-reschedule time) FAILS at step 4.

## Why it survives
After step 4 there is no further Unschedule for C (AD already emitted it once), so only a leadership loss / `reset()` (dispatcher_main.go:293-304) clears it.
