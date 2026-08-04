# ksm-shard-tracking-consistency — KSM shard tracking never diverges from the dispatch store

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P1 · **Intent:** invariant

**Provenance:** merged from 1 discovery agent(s): ksm-shard-tracking-store-divergence

## Property

The ksmShardedConfigs tracking map and the dispatch store never diverge such that a KSM check is marked 'already sharded' while it has no shards in the store — which would silently drop the check on the next leadership cycle.


## Invariant / assertion

`assert.AlwaysOrUnreachable`: for every digest in ksmShardedConfigs, its shard digests exist in the store; and no KSM source config is marked sharded without live shards. AlwaysOrUnreachable fits — KSM sharding is an optional feature, but whenever active the tracking must match the store.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: an AD Schedule of a KSM check landed between reset() and RemoveScheduler.


## Antithesis angle

Self-documented race (handler.go:187-191): RemoveScheduler must precede reset() or an AD Schedule between reset() clearing ksmShardedConfigs and RemoveScheduler repopulates it → isAlreadySharded returns true → KSM check silently dropped next cycle. ksmShardingMutex and the store lock are taken separately (never together), so the 'is sharded' bit and store shards can disagree under interleaving. Flap leadership concurrent with AD config replay of a KSM check.


## Why it matters

A silently dropped KSM check means Kubernetes State Metrics stop flowing for that resource — a large, invisible monitoring gap. This is a claimed fix (ordering-dependent); Antithesis is the right tool to confirm it holds under interleaving.


## Mechanism refinement (from open-question investigation)

Scope note (no invalidation): the self-documented KSM Schedule-during-reset race appears defended in depth - RemoveScheduler/Deregister serializes with Schedule via the controller's shared ms.m (controller.go:131 vs :191-208) under a single worker goroutine, and runs before reset() (handler.go:191-194), so the specific interleaving may be UNREACHABLE through the AutoConfig path. The invariant must still hold, so the property remains valid as a regression guard, but the paired Reachable witness (a KSM Schedule landing between reset() and RemoveScheduler) may be unschedulable given current controller locking.


## Fault dependencies

- network partition to flap leadership concurrent with AD config replay (enabled by default)
- concurrency (ksmShardingMutex vs store lock split)
- requires leader_election enabled + >=2 replicas + ksm_sharding_enabled + advanced_dispatching


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.AlwaysOrUnreachable` reconciling ksmShardedConfigs against store shards after Schedule/reset. Confirm the documented fix ordering from the diff.


## Open questions (post-investigation)

None — all resolved (see Investigation Log).


### Investigation Log

#### Whether AutoConfig RemoveScheduler synchronously drains in-flight Schedule calls.

Examined autoconfig.go:665-668 (RemoveScheduler->Controller.Deregister), controller.go:131-139 (Deregister takes ms.m), :163-215 (processNextWorkItem holds ms.m across the ENTIRE scheduler.Schedule() call section :191-208), :87 (single worker goroutine). Found: an in-flight Schedule holds ms.m, so Deregister BLOCKS until it completes; after Deregister returns the dispatcher is removed from activeSchedulers so no subsequent Schedule reaches it. runDispatch (handler.go:191) calls RemoveScheduler BEFORE reset() (:194). Conclusion: RESOLVED - yes, it synchronously drains; the self-documented KSM race window (handler.go:187-191) is effectively closed at the AutoConfig layer by the shared ms.m plus the single-worker model.

#### Whether reset() zeroing under two locks can interleave with a concurrent Schedule seeing half-cleared state.

Examined dispatcher_main.go:294-304 (reset takes ksmShardingMutex then store.Lock sequentially), dispatcher_ksm.go:38-40/:96-110 (isAlreadySharded/markAsSharded read/write ksmShardedConfigs only under ksmShardingMutex, called only from Schedule). Found: because RemoveScheduler (serialized with Schedule via ms.m) precedes reset(), no AD-driven Schedule can be concurrent with reset(); the only reader of ksmShardedConfigs is scheduleKSMCheck via Schedule. Conclusion: RESOLVED - no concurrent observer of the half-cleared state exists once the scheduler is deregistered.

#### Requires a shardable KSM check config present for the branch to be reachable.

Examined dispatcher_main.go:111-141 (KSM sharding enabled only when advanced_dispatching_enabled AND ksm_sharding_enabled) and Schedule->scheduleKSMCheck (:201). Found: the isAlreadySharded/markAsSharded branch is reachable only with both config flags set plus an actual KSM check config. Conclusion: RESOLVED - harness precondition confirmed: leader_election + >=2 replicas + advanced_dispatching_enabled + ksm_sharding_enabled + a KSM check config.


---

## Source discovery evidence (raw, per contributing agent)


### from `ksm-shard-tracking-store-divergence`

## Claim
The KSM shard-tracking map (`ksmShardedConfigs`, guarded by `ksmShardingMutex`) can diverge from the dispatch store (guarded by the store lock): a config marked sharded whose shards are absent from the store causes `isAlreadySharded` to short-circuit scheduling and silently drop the check.

## Code path (verified)
`pkg/clusteragent/clusterchecks/dispatcher_ksm.go`
```go
if d.isAlreadySharded(config) {          // :38
    return true                          // :40  prevents original from scheduling
}
...
d.markAsSharded(config, shardedDigests)  // :88 -> :105-110 under ksmShardingMutex
```
`isAlreadySharded`/`markAsSharded` lock only `ksmShardingMutex` (`:96,:106`) - **not** the store lock. The store is a separate structure under `d.store`.

## The divergence window (verified structure)
`reset()` clears the two structures under two different locks, sequentially:
```go
func (d *dispatcher) reset() {
    d.ksmShardingMutex.Lock(); d.ksmShardedConfigs = make(...); d.ksmShardingMutex.Unlock() // :297-299
    d.store.Lock(); defer d.store.Unlock(); d.store.reset()                                  // :301-303
}
```
`runDispatch` orders teardown as run -> `RemoveScheduler` -> `reset` (`handler.go:185-194`) with a comment (`:187-191`) explaining that a `Schedule` between clearing `ksmShardedConfigs` and stopping the scheduler repopulates the map and drops the check next cycle. **`RemoveScheduler` unregisters future calls but does not wait for an in-flight `Schedule`.** A `Schedule` running concurrently can execute `markAsSharded` after `reset` cleared the map, leaving `ksmShardedConfigs[orig]` populated while the store has no corresponding shards.

## Consequence on next leadership cycle
The store is rebuilt from AD replay, but for the stale-tracked config `isAlreadySharded` returns true at `:38` and `scheduleKSMCheck` returns true at `:40` without calling `d.add`, so neither the original nor shards land in the store -> no dispatch, no dangling entry, no error.

## Suggested SUT instrumentation (MISSING - net new)
After store rebuild (end of warmup) or on each Schedule, iterate `ksmShardedConfigs` under both locks and `assert.AlwaysOrUnreachable(shardPresentInStore, "ksm shard tracking matches store", details)`. Also `assert.Reachable` at the `isAlreadySharded==true` short-circuit (`:40`) to confirm coverage of that branch.
