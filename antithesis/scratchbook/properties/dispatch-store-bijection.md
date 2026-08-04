# dispatch-store-bijection — Config-digest ↔ node assignment is an exact bijection

**Type:** Safety · **Assertion:** `Always` · **Priority:** P0 · **Intent:** invariant

**Provenance:** merged from 4 discovery agent(s): clusterchecks-digest-to-node-bijection, cluster-check-dispatched-to-exactly-one-node, clustercheck-single-dispatch-location-invariant, clusterchecks-dispatch-consistency-after-leadership-recovery

## Property

At every quiescent point, each known cluster-check config digest is in exactly one of {assigned to exactly one existing node} XOR {held in danglingConfigs} — never both, never neither, never on two nodes, never mapped to a non-existent node.


## Invariant / assertion

`assert.Always` via a store validator run under d.store lock at the tail of addConfig/removeConfig/expireNodes/rebalance/deleteDangling: (1) every digest in digestToNode maps to a node present in nodes, and that node's digestToConfig holds it; (2) no digest appears in two nodes' maps; (3) every node-held digest maps back via digestToNode; (4) digestToConfig has an entry for every referenced digest; (5) a digest is not simultaneously dangling and assigned. Always fits — a global structural invariant that must hold on every store mutation.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: a node was expired while still holding configs, concurrent with a Schedule/reset (the fracture interleaving actually occurred).


## Antithesis angle

Two-level lock (store then node) released between operations, plus reset() wiping the store mid-flight, is the fracture point. addConfig (dispatcher_configs.go:154-163) does check-then-act with a `foundCurrent && currentNode != targetNode` guard (PR #3023). Interleave AD Schedule/Unschedule with expireNodes (30s heartbeat timeout) and a leadership loss→reset→re-acquire cycle; assert the bijection after each. Node-agent partition drives expiry; backward clock skew amplifies mass expiry.


## Why it matters

This is the store-level shadow of the split-brain hazard and the concrete mechanism behind 'each check dispatched to exactly one node.' Orphaned digest → silent check drop (monitoring gap, no alert). Digest on two nodes → duplicate check execution → double-counted metrics. Surfaced by 4 focus agents.


## Mechanism refinement (from open-question investigation)

Scope clarifications (no invalidation): (1) invariant must be scoped to non-endpoint maps (digestToConfig/digestToNode/nodes/danglingConfigs) - endpointsConfigs (stores.go:31) is a disjoint node-pinned structure, confirming the carve-out. (2) The reset-asymmetry angle should name endpoint_checks.configs_dispatched (dispatchedEndpoints) as the concrete surviving Inc-without-clear gauge across resets - no Delete site exists anywhere, so it ratchets each leadership cycle on AD replay.


## Fault dependencies

- network partition (node-agent<->leader >30s to trigger expiry; enabled by default)
- node hang/throttle on a node agent
- clock skew backward (DISABLED by default — amplifies mass expiry)
- requires leader_election enabled + >=2 replicas to exercise reset/re-acquire


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. Add checkStoreConsistency() under d.store.Lock at the tail of every mutating op, wrapped in assert.Always. Endpoints configs (node-pinned 1:1) need a carve-out from the load-balanced shape.


## Open questions (post-investigation)

None — all resolved (see Investigation Log).


### Investigation Log

#### Does reset() zero nodeStore.lastConfigChange, or can a stale epoch survive a leadership cycle?

Examined stores.go:42-55 (reset) and :129-137 (newNodeStore). Found: reset() does `s.nodes = make(...)`, discarding every *nodeStore object entirely; lastConfigChange is a field OF nodeStore (stores.go:120), so it cannot survive a reset. A re-registered node gets a fresh newNodeStore with lastConfigChange=0. Conclusion: RESOLVED - no per-node epoch survives; there is no stale lastConfigChange in the rebuilt store.

#### Are endpoints configs (node-pinned 1:1) expected to violate the load-balanced shape and need a carve-out?

Examined stores.go:31 (endpointsConfigs map[nodeName][digest]), dispatcher_main.go:188-198 (Schedule routes NodeName!="" to addEndpointConfig), dispatcher_endpoints_configs.go:42-64. Found: endpoints live in a disjoint endpointsConfigs map that is NEVER touched by digestToNode/digestToConfig/danglingConfigs. Conclusion: RESOLVED - the bijection invariant scopes only to the cluster-check maps and naturally excludes endpoints; a carve-out is correct/required.

#### Exact set of store mutators that must carry the invariant check.

Examined dispatcher_configs.go, dispatcher_nodes.go, dispatcher_rebalance.go, stores.go. Found digest/node-touching mutators, all under d.store.Lock: addConfig (dispatcher_configs.go:120), removeConfig (:168), deleteDangling (:225, caller holds lock), expireNodes (dispatcher_nodes.go:142), moveConfig/rebalance (dispatcher_rebalance.go:168, lock at :175), store.reset (stores.go:42 via d.reset dispatcher_main.go:301), getOrCreateNodeStore (stores.go:86, inserts nodes; callers hold lock). Conclusion: RESOLVED - that 7-site set is the enumerated coverage.

#### Exact set of gauges still reset-asymmetric after #52876.

Examined metrics.go and all Inc/Dec/Set/Delete sites. reset() (stores.go:42-55) clears only dispatchedConfigs (per-node Delete loop), nodeAgents (Dec loop), danglingConfigs+unscheduledCheck (via clearDangling). NOT cleared by reset(): dispatchedEndpoints (Inc addEndpointConfig:49 / Dec removeEndpointConfig:63, NO Delete anywhere -> ratchets up every leadership cycle on AD replay; the strongest genuine surviving leak, same shape as the #52876 nodeAgents bug), configsInfo (Set/Delete only in addConfig/removeConfig/rebalance/expireNodes - self-heals only by overwrite if same check lands on same node), busyness (Set updateRunnersStats / Delete expireNodes only), predictedUtilization (Set-only dispatcher_rebalance.go:539, never Deleted). statsCollectionFails is a counter. Conclusion: RESOLVED - {dispatchedEndpoints, configsInfo, busyness, predictedUtilization} survive reset asymmetrically; dispatchedEndpoints is the clean Inc-without-clear leak.

#### No leader-generation epoch on lastConfigChange; after reset() the counter restarts (folded from R6) - can a node be told IsUpToDate when it is not?

Examined helpers.go:47 (lastConfigChange = time.Now().UnixNano()) and dispatcher_nodes.go:69 (comparison is exact equality node.lastConfigChange == status.LastChange). Found: comparison is equality, not ordering, on a nanosecond wall-clock value. After reset a reused node gets a fresh nodeStore (0, then a fresh nano on next addConfig). A false IsUpToDate requires node.lastConfigChange == status.LastChange EXACTLY; a nanosecond timestamp never coincidentally equals a node's stale cached value (edge case both==0 correctly means no configs on either side). Even under backward clock skew the values still differ -> node re-pulls (returns false). Conclusion: RESOLVED - missing epoch is not exploitable for false IsUpToDate given exact-equality nanosecond comparison; not a bijection defect.


---

## Source discovery evidence (raw, per contributing agent)


### from `clusterchecks-digest-to-node-bijection`

## Property
The cluster-check dispatch store maintains an exact bijection between config digests and the node running them.

## Where the state lives
`pkg/clusteragent/clusterchecks/stores.go`:
- `clusterStore.digestToNode map[string]string` (digest -> node name), `digestToConfig`, `nodes map[string]*nodeStore`.
- Each `nodeStore.digestToConfig map[string]integration.Config` is the per-node view; guarded by the node's own RWMutex (two-level lock, order store->node).

## Mutation sites (all must preserve the invariant)
- `addConfig` (`dispatcher_configs.go:118-166`): registers `digestToConfig[digest]`, then `currentNode,foundCurrent := getNodeStore(digestToNode[digest])`, `targetNode := getOrCreateNodeStore(targetNodeName)`, `targetNode.addConfig(config)`, `digestToNode[digest]=targetNodeName`, then removes from `currentNode` only `if foundCurrent && currentNode != targetNode`. Check-then-act across two node locks.
- `removeConfig` (`dispatcher_configs.go:169-210`): deletes `digestToNode[digest]`, `digestToConfig[digest]`, walks `idToDigest`, then `node.removeConfig(digest)`.
- `expireNodes` (dispatcher_nodes.go): deletes entries from `clusterStore.nodes` when heartbeat older than `node_expiration_timeout` (30s), re-dispatching their configs to `danglingConfigs`.
- `reset()` (`stores.go:42-56`): replaces every map with a fresh empty map.

## Failure scenario
1. Config `X` (digest `d`) dispatched to node `A`: `digestToNode[d]=A`, `A.digestToConfig[d]=X`.
2. Node `A`'s heartbeat lapses (partitioned node agent, alive but unreachable for >30s). `expireNodes` deletes `A` from `clusterStore.nodes` and moves `d` to `danglingConfigs`.
3. Interleave: a rebalance or re-dispatch runs concurrently and, due to the two-level lock being released between store and node operations, `digestToNode[d]` is left pointing at `A` while `nodes[A]` is gone, OR the config is placed on `B` without clearing the stale `A` reference.
4. Invariant broken: `digestToNode[d]=A` but `A` not in `nodes` (dangling), or both `A` and `B` claim `d` (duplicate).

## Assertion point (MISSING — net-new)
A validator run under `d.store` lock at the end of `addConfig`/`removeConfig`/`expireNodes`/`reset`/rebalance: iterate `digestToNode`, assert target node exists and holds the digest; iterate every node's `digestToConfig`, assert `digestToNode` maps back to it; assert no digest appears in two nodes. `assert.Always(consistent, "digestToNode bijection holds", details)`.

## Existing coverage gap
Unit tests (`dispatcher_test.go`, `dispatcher_dynamic_test.go`) drive these single-goroutine with hand-set heartbeats; the concurrent expiry + reset + rebalance interleaving is never exercised (analysis §9).


### from `cluster-check-dispatched-to-exactly-one-node`

## Mechanism (verified against source)

- `clusterStore` (`stores.go:24-33`): `digestToConfig`, `digestToNode` (each digest → exactly one node), `nodes` (each `nodeStore` has its own RWMutex — two-level lock, order store→node), `danglingConfigs`, `endpointsConfigs`, `active`.
- No persistence; `reset()` wipes it on leadership loss; rebuilt from AD replay + heartbeats.
- Redispatch path (`dispatcher_main.go:400-411`): `expireNodes()` moves configs of nodes whose heartbeat is older than `node_expiration_timeout` (30s) into dangling; `shouldDispatchDangling()` → `retrieveDangling()` → `reschedule()` → `deleteDangling()`.
- KSM silent-drop race (self-documented, `handler.go:187-191`):

```go
// RemoveScheduler must be called before reset() to close a race window: if autodiscovery
// fires a Schedule call between reset() clearing ksmShardedConfigs and RemoveScheduler
// stopping new calls, ksmShardedConfigs gets repopulated. On the next leadership cycle,
// isAlreadySharded returns true and the KSM check is silently dropped.
```

Note the *current* order (runDispatch, handler.go:184-194) calls `dispatcher.run` → then `RemoveScheduler` → then `reset()`, which is the fixed order; the property re-tests that the fix holds under churn Antithesis can interleave differently than the unit tests did.

## Failure scenario (duplicate)

1. Node N1 holds config digest D. N1 is partitioned from the leader but stays alive and keeps running D (README claims cached checks keep running — sut-analysis §11, node side out of scope but this is the duplicate premise).
2. After 30s, `expireNodes()` moves D to dangling; `reschedule()` assigns D to N2. Store now has D→N2. No fencing token was ever sent to N1.
3. Store invariant (D on exactly one node) still holds *inside the store*, but the real-world duplicate is that N1 still executes D. The store-level assertion catches the stricter failure: if a churn/interleaving leaves D listed both in N1's nodeStore.configs and N2's (index/list disagreement), VIOLATION.

## Failure scenario (silent drop)

Leadership flap concurrent with AD Schedule leaves a digest in `digestToConfig` but in neither `digestToNode` nor `danglingConfigs` (conservation violation) — the check is lost until the next full replay. This is the class the #52876/#52078/#50715 fixes chased.

## Where to assert (SUT instrumentation — MISSING)

- Add a `store.checkInvariant()` helper invoked under the store write lock at the end of `expireNodes`, `reschedule`, `deleteDangling`, `addConfig`/`removeConfig`, and `reset` (`stores.go`, `dispatcher_main.go`, `dispatcher_configs.go`). Wrap in `assert.Always(...)`.
- Condition (1) function/consistency: iterate `nodes`, collect digest→node from per-node config lists, compare to `digestToNode`; assert no digest appears under two nodes.
- Condition (2) conservation: for each digest in `digestToConfig`, assert it is in exactly one of `digestToNode`/`danglingConfigs`.
- Endpoints checks are intentionally node-pinned (1:1 with a pod's node, sut-analysis §11) — scope the assertion to non-endpoint cluster checks or account for `endpointsConfigs` separately to avoid false positives.

## Existing coverage gap

Node expiry is tested by hand-overwriting heartbeat fields, not real time; rebalance is called synchronously with stubbed failures (sut-analysis §9). Concurrent Schedule-during-reset and churn-driven redispatch interleavings are unexercised.


### from `clustercheck-single-dispatch-location-invariant`

## Mechanism / where it can break (verified structure)

**Two-level maps that must stay in sync** (stores.go:24-33): `digestToConfig`, `digestToNode` (store-level) and each `nodeStore.digestToConfig` (node-level, own RWMutex). addConfig maintains them together and removes from the previous node (dispatcher_configs.go:146-165), citing PR #3023 for the 'don't de-schedule what we just scheduled' guard — evidence this consistency is fragile and has regressed before.

**expireNodes moves node configs to dangling** under store.Lock (dispatcher_nodes.go:142-186): deletes digestToNode[d], adds to danglingConfigs, deletes the node. If any path deletes the node but leaves a digestToNode entry, or leaves the config in the node map, the invariant breaks.

**reset() symmetry** (stores.go:42-55, clearDangling :104-112): must clear every map AND decrement every per-node gauge. SUT §8 lists a *series* of fixes (#52876/#52078/#50715) for gauges/maps not reset symmetrically on leadership loss — 'strong signal more remain.'

**No persistence** (SUT §6): the store is rebuilt each leadership cycle from AD replay + heartbeats, so every flap re-exercises the rebuild and any asymmetry surfaces as drift.

**Warmup window** (dispatcher_nodes.go:73-79): during active=false, processNodeStatus returns IsUpToDate=true to all nodes; a flap shorter than warmup_duration (30s) can leave configs never dispatched — a drop the invariant would catch as 'digest in digestToConfig, in neither location.'

## Assertion (checkable in-store)
Under d.store.Lock at a quiescent point (e.g. end of each cleanup tick, dispatcher_main.go:411), walk digestToConfig and assert.Always the XOR-single-location predicate above, plus cross-check danglingConfigs gauge == len(danglingConfigs) and sum of node dispatchedConfigs == count of assigned digests.

## Failure scenario
Leader flaps A->follower->A within 20s while node N is expiring: reset() on loss clears maps; re-acquire rebuilds; if a stale digestToNode[d]=N survives an asymmetric reset (the regressed pattern) while N is gone, d is in digestToConfig but in no node and not dangling -> silent drop -> assertion FAILS.

## Existing coverage gap
Unit tests use fake clientset with no real lease timing and hand-set heartbeats (SUT §9); reset-under-flap concurrent with expiry is unexercised.


### from `clusterchecks-dispatch-consistency-after-leadership-recovery`

## State model (verified)
`clusterStore` (stores.go:24-33) is the single authoritative in-memory dispatch state: `digestToConfig` (all configs), `digestToNode` (digest→one node), `nodes` (each a `nodeStore` with its own `digestToConfig`), `danglingConfigs`, `idToDigest`. No persistence.

## The recovery path that must preserve the invariant
- **Loss:** `runDispatch` (handler.go:180-195) runs `dispatcher.run` until ctx cancel, then `RemoveScheduler`, then `reset()`. `dispatcher.reset()` (dispatcher_main.go:294-304) clears `ksmShardedConfigs` and calls `store.reset()` (stores.go:42-55) which decrements per-node gauges and re-makes every map. `store.active=false`.
- **Reacquire:** `Run` (handler.go:106-176) waits for `leader`, runs `warmupDuration`, then `runDispatch` re-registers the scheduler with `replayConfigs=true` (handler.go:182) → AutoDiscovery re-`Schedule`s every config → `add`/`addConfig` rebuild the maps (dispatcher_configs.go:120-166). Nodes re-register via heartbeats (`getOrCreateNodeStore`, stores.go:86-99).

## Where the invariant can break
- **Asymmetric reset (regression cluster):** §8 cites #52876/#52078/#50715 — gauges/maps not reset symmetrically. `clearDangling` (stores.go:104-112) and `reset` touch overlapping telemetry; `expireNodes` (dispatcher_nodes.go:142-186) mutates `digestToNode`, `danglingConfigs`, `idToDigest`, and gauges in a hand-rolled loop — easy to leave a digest in `digestToNode` pointing at a deleted node, or in both `danglingConfigs` and a node.
- **Dangling re-dispatch TOCTOU:** dispatcher_main.go:404-411 does `retrieveDangling()` (RLock) → `reschedule()` (per-config Lock via `add`) → `deleteDangling(scheduledConfigIDs)` (Lock). `add` returns false when no node is available and re-inserts into danglingConfigs (dispatcher_configs.go:137-144); interleaved `expireNodes`/`Schedule` between the RLock snapshot and the deleteDangling can leave a digest both dispatched and dangling.
- **KSM Schedule race:** self-documented at handler.go:187-191 — a `Schedule` between `reset()` and `RemoveScheduler` repopulates `ksmShardedConfigs`, causing a silent drop next cycle.

## Failure scenario (concrete)
1. Leader dispatches check C (digest d) to node N: `digestToNode[d]=N`, `N.digestToConfig[d]=C`.
2. Leadership flaps; `reset()` wipes maps. Reacquire; warmup; AD replays C. Concurrently node N's heartbeat lapses and `expireNodes` runs mid-rebuild.
3. Interleaving leaves `digestToNode[d]=N` but `N` already deleted from `nodes` (or `d` present in danglingConfigs too). Invariant (1)/(2) violated → check silently unscheduled while metrics claim it is dispatched.

## Instrumentation (MISSING — net-new)
- A `checkConsistency()` helper guarded by `d.store` lock, called at the end of each `dispatcher.run` cleanup tick and in `getState`, wrapping the four clauses in `assert.AlwaysOrUnreachable(...)`.
- `assert.Reachable("dispatch store rebuilt after leadership reacquire with >=1 config", ...)` right after warmup (handler.go:144) to confirm the harness reached the meaningful state.
- Cross-replica duplicate dispatch (two leaders) is a related but distinct property requiring an external witness; this property is the intra-leader consistency check.

## Why existing tests miss it
§9: unit tests are single-goroutine with fake clientset; node expiry tested by hand-overwriting heartbeat fields; leadership transitions deferred to E2E which never kills the leader. The reset→rebuild path under concurrent expiry/schedule is unexercised.
