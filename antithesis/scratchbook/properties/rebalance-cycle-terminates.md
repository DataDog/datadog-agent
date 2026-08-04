# rebalance-cycle-terminates — Each rebalance cycle terminates

**Type:** Liveness · **Assertion:** `Sometimes` · **Priority:** P1 · **Intent:** invariant

**Provenance:** merged from 1 discovery agent(s): rebalance-cycle-terminates

## Property

Every invocation of the leader's rebalance cycle (rebalanceUsingBusyness / rebalanceUsingUtilization, fired by the 10-min rebalanceTicker) runs a bounded number of moveConfig operations and returns, even when a CLC runner is unreachable, stats are missing, or an AutoDiscovery Unschedule concurrently removes a config mid-cycle. It never spins retrying the same failing move.


## Invariant / assertion

assert.Sometimes(rebalance_cycle_completed_after_a_moveconfig_failure): across the run there is at least one rebalanceUsingBusyness invocation that (a) had >=1 moveConfig return an error during the cycle AND (b) still returned to its caller. The hang regression (pre-#52884 `continue`) would loop forever on the failing move, so the Sometimes never fires. Complementary SUT-side hard guard: assert.Always(inner_loop_moveConfig_attempts_per_cycle <= numNodes*numConfigsAtCycleStart) — a static upper bound; exceeding it is the infinite-loop regression.


## Antithesis angle

rebalanceUsingBusyness (dispatcher_rebalance.go:281-325) has an inner loop `for diffMap[source] > 0` whose only progress is a successful moveConfig (which shrinks the source node's cluster-check stat set and recomputes diffMap via updateDiff). #52884 (84f11df1f18) fixed a `continue`->`break` bug: on moveConfig failure the loop used to retry the SAME digest forever because the store was unchanged, so pickConfigToMove returned it again — an infinite loop that also spiked rebalancing_decisions. The store is NOT held across the whole cycle (calculateAvg/getDiffAndWeights/pickConfigToMove take RLock, moveConfig takes Lock, each released between calls), so Antithesis can interleave a concurrent AD Unschedule/removeConfig or an expireNodes between pickConfigToMove and moveConfig to make moveConfig fail with 'no config registered for digest' / 'node not found' — the REAL failure modes the synthetic unit test (TestRebalanceUsingBusyness_BreaksOnMoveConfigFailure) only fakes. Additionally an unreachable CLC runner makes GetRunnerStats fail in updateRunnersStats (dispatcher_nodes.go:219-224, `continue` keeps stale stats), and moveConfig's per-instance GetRunnerStats can then return no movable instances (movedAny=false -> error -> break). Inject latency/partition dca-leader->clc-runner concurrent with AD config churn, force a rebalance, and assert the cycle returns.


## Why it matters

The rebalance loop runs under the dispatcher goroutine that also drains the clusterchecks-dispatch liveness probe (dispatcher_main.go:398-399, self-acknowledged hang risk) and holds/contends the global store lock. A non-terminating cycle wedges dispatch cluster-wide, trips the liveness probe -> pod restart -> leadership churn -> more instability. The class of bug (loop that only makes progress on the success path, retries the failure path) is exactly what #52884 was; Antithesis confirms it stays fixed under real concurrent failure interleavings the unit test cannot produce.


## Fault dependencies

- network latency/partition dca-leader -> clc-runner to make GetRunnerStats fail/stale (ENABLED by default)
- concurrency: AD Unschedule/removeConfig or expireNodes interleaved between pickConfigToMove and moveConfig (always-on thread interleaving)
- requires leader_election enabled + advanced_dispatching + >=1 CLC runner in topology; node termination and clock skew NOT required


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

Add the Antithesis Go SDK to the root module and instrument dispatcher_rebalance.go. (1) LIVENESS WITNESS: in rebalanceUsingBusyness, track a per-cycle bool `sawMoveConfigError`; on the moveConfig error break set it; after the outer loop returns call assert.Sometimes(true, 'rebalance cycle returned after a moveConfig failure', details{cycle,moves,errors}). This fires only if the cycle both saw a failure and reached the return — a hang makes it never fire. (2) HARD SAFETY GUARD: maintain a per-cycle counter of moveConfig attempts and assert.Always(attempts <= numNodesAtStart*numConfigsAtStart, 'rebalance inner-loop bounded', ...) at cycle end. (3) REACHABILITY of the hazardous precondition: assert.Reachable at the moveConfig-error break site with details recording the error string, so triage can confirm the real (not synthetic) failure path was scheduled. Workload side: run >=2 CLC-runner stubs, seed >1 cluster-check config, drive AD Unschedule churn while a partition to one runner is active, and lower rebalance_period so cycles fire within a timeline.


## Open questions (post-investigation)

None — all resolved (see Investigation Log).


### Investigation Log

#### Can two source nodes each hit a moveConfig failure with the outer loop still completing, or a pathological blow-up?

Examined dispatcher_rebalance.go:281-325 and moveConfig:168-238. Found: outer loop is fixed range over weights (len=#nodes); inner loop exits on first failure via break (:288 pick fail, :305 move fail, :322 toleration fail); only a successful moveConfig iterates, and it shrinks the source's stats (removeConfig/RemoveRunnerStats :211,:223). Conclusion: each node contributes <= (initial stat count)+1 attempts; two failing nodes just break independently and the outer loop completes. The static bound numNodes x numConfigsAtStart is safe and loose (true bound ~ total stats + #nodes). No blow-up. RESOLVED.

#### Does applyDistribution continue-on-failure re-propose the same failing move next cycle?

Examined rebalanceUsingUtilization:378-443, applyDistribution:502-535, currentDistribution:445-500. Found: proposedDistribution is rebuilt each cycle from currentDistribution(), which enumerates only checks present in clcRunnerStats with a registered digest (:459-470). A config that failed to move (Unscheduled/removed, or stats absent) is excluded from currentDistribution next cycle. Conclusion: applyDistribution's continue-on-failure does NOT perpetually re-propose the same failing move — the failing config drops out of the input. RESOLVED.

#### CLC-runner client timeout: a slow GetRunnerStats prolongs updateRunnersStats under the store write lock.

Examined clcrunner.go:86 (2s per-call timeout). Conclusion: a slow runner adds up to 2s/call to updateRunnersStats (bounded, returns after all nodes time out) and prolongs the store-write-lock hold (that is the store-lock-bounded-under-slow-clc property's concern) but does NOT break termination of the rebalance cycle. RESOLVED (cross-referenced).


---

## Source discovery evidence (raw, per contributing agent)


### from `rebalance-cycle-terminates`

## Mechanism (verified, commit f2da1471bb7)

`dispatcher.rebalance(force)` (`dispatcher_rebalance.go:240-257`) dispatches to one of two algorithms based on `cluster_checks.rebalance_with_utilization`. It is invoked only by the leader's dispatcher loop on `rebalanceTicker.C` (`dispatcher_main.go:415-418`, gated on `d.advancedDispatching.Load()`), period `rebalance_period` (default 10m).

### Busyness path — the loop with the historical hang
`rebalanceUsingBusyness` (`dispatcher_rebalance.go:261-328`):
```
for _, nodeWeight := range weights {              // outer: fixed len = #nodes
    for diffMap[nodeWeight.nodeName] > 0 {        // inner
        digest, configWeight, err := d.pickConfigToMove(source)
        if err != nil { break }
        ... toleration check ...
        if destDiff+configWeight < int(float64(sourceDiff)*tolerationMargin) {
            err = d.moveConfig(source, dest, digest)
            if err != nil { break }   // <-- #52884 changed this from `continue`
            diffMap = d.updateDiff(totalAvg)
        } else { break }
    }
}
```
Termination argument (post-fix): every path out of the inner loop is a `break` **except** a successful `moveConfig`, which (`moveConfig`, `:168-238`) calls `sourceNode.removeConfig` + `RemoveRunnerStats` for the moved digest's instances, strictly shrinking the source node's cluster-check stat set that `pickConfigToMove` (`:105-143`) draws from. So successful moves are bounded by the initial stat count, and each recomputed `diffMap[source]` drops by `configWeight`; the loop provably ends. Pre-#52884 the failure path used `continue`, leaving the store unchanged so `pickConfigToMove` returned the same digest forever.

### #52884 fix (84f11df1f18)
> "When moveConfig fails, the inner loop would **continue** without updating diffMap, causing pickConfigToMove to return the same digest repeatedly ... run the new unit tests on the old version ... the rebal algo infinitely cycling."

Existing regression test `TestRebalanceUsingBusyness_BreaksOnMoveConfigFailure` (`dispatcher_rebalance_test.go:2064`) fakes the failure by omitting a `digestToConfig` entry and uses a 5s watchdog. It is single-goroutine and synthetic.

### Utilization path — terminates by construction
`rebalanceUsingUtilization` (`:378-443`) does a single greedy pass: place pinned configs, then `configsSortedByWorkersNeeded()` once through `addToLeastBusy`, then `applyDistribution` iterates `proposedDistribution.Configs` once (`:502-535`). All loops are bounded by #configs/#runners; no retry-on-failure loop. `applyDistribution`'s moveConfig failure path is `continue` but the loop range is fixed, so it still terminates.

## Why Antithesis (not a unit test)
The real `moveConfig` failure modes require concurrency the unit test does not exercise: the store lock is released between `pickConfigToMove` (RLock) and `moveConfig` (Lock), so a concurrent AD `Unschedule`->`removeConfig` (deletes `digestToConfig[digest]` -> 'no config registered', `:192-194`) or `expireNodes` (deletes the node -> 'node not found', `:188-191`) makes moveConfig fail for genuine reasons mid-cycle. An unreachable CLC runner makes `updateRunnersStats` skip the node (`dispatcher_nodes.go:219-224`) leaving stale stats, and per-instance `GetRunnerStats` inside moveConfig can then yield `movedAny==false` -> error. These interleavings are Antithesis's domain.
