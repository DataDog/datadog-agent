# reset-restores-store-and-gauges — Leadership loss resets store and telemetry gauges to ground truth

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P1 · **Intent:** invariant

**Provenance:** merged from 2 discovery agent(s): clusterchecks-reset-clears-store, dangling-configs-gauge-leak-and-recovery

## Property

On losing leadership, dispatcher.reset() returns the in-memory store AND its exported gauges (nodes_reporting, dangling, unscheduled, KSM shard map) to empty/ground truth, so a later re-acquisition starts clean with no leaked counts.


## Invariant / assertion

`assert.AlwaysOrUnreachable`: after reset() completes, all store maps are empty AND each gauge equals its ground-truth value (0 / len(map)). AlwaysOrUnreachable fits — reset only runs on a leadership loss (optional path), but whenever it runs the post-condition must hold.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: reset() ran with a non-empty store and non-zero gauges (a real leadership loss with live state).


## Antithesis angle

A series of past fixes addressed reset asymmetry: #52876 (nodeAgents.Dec missing → nodes_reporting drifted up every cycle), #52078 (dangling/unscheduled gauges), #50715 (KSM shard map). Strong signal more remain. Flap leadership repeatedly (partition→heal) and assert gauges return to ground truth each cycle. This is a regression cluster — confirm each fixed mechanism still holds and probe adjacent gauges/maps.


## Why it matters

Gauge drift misleads operators (false 'N nodes reporting') and, for the KSM shard map, causes a check to be silently dropped next cycle. The repeated fix history makes this a high-value regression target.


## Mechanism refinement (from open-question investigation)

Strengthen the assertion set: the reset() post-condition check must cover endpoint_checks.configs_dispatched (dispatchedEndpoints) plus configsInfo/busyness/predictedUtilization, not just nodes_reporting/configs_dangling/unscheduled - dispatchedEndpoints is a confirmed Inc-without-clear that ratchets across leadership cycles (primary evidence: no Delete site in code; reset() at stores.go:42-55 never touches it).


## Fault dependencies

- network partition to force lease loss then heal (leadership flap; enabled by default)
- requires leader_election enabled + >=2 replicas


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.AlwaysOrUnreachable` at the end of reset() checking every map empty and each gauge==ground-truth. Confirm each historical fix (#52876/#52078/#50715) mechanism from the diff before treating it as a regression anchor. R11: the GAUGE half (nodes_reporting/dangling/unscheduled) is Prometheus telemetry on the unauth'd metrics_port — the workload can scrape /metrics and assert ground-truth across a flap with NO instrumentation; reserve SUT-side asserts for the in-memory-map-empty half.


## Open questions (post-investigation)

- Should the narrow in-flight heartbeat race be explicitly closed? A PostStatus that passed RejectOrForwardLeaderQuery reading state==leader just before h.state flips to follower can have its getOrCreateNodeStore land after reset() (both serialize on d.store.Lock), registering a phantom node + nodeAgents.Inc post-reset. It self-heals via expireNodes within node_expiration_timeout next term, so bounded/transient - but whether processNodeStatus should re-check active/leadership is a design call. `(needs human input)`


### Investigation Log

#### Should a heartbeat arriving during teardown be rejected once dispatchCancel fires, or is auto-registration after reset a genuine leak?

Examined cmd/cluster-agent/api/v1/clusterchecks.go:44 (PostStatus gated by RejectOrForwardLeaderQuery), handler_api.go:22-42 (gate reads h.state), handler.go:257-277 (updateLeaderIP sets h.state=follower at :275 BEFORE the leadershipChan send at :276 that triggers dispatchCancel at :172 and then reset() at :194). Found: the gate is h.state, which flips to follower strictly before reset() begins, so new heartbeats are forwarded not registered. processNodeStatus (dispatcher_nodes.go:44-52) has NO active/leadership re-check; it unconditionally getOrCreateNodeStore + nodeAgents.Inc. A request that passed the gate before the flip can register a phantom post-reset (narrow race, serialized on d.store.Lock). Conclusion: PARTIAL - gate exists and closes the common case; residual narrow race is self-healing (expireNodes next term). Whether to harden is intended-behavior -> needs-human.

#### Does reset() zero nodeStore.lastConfigChange so a reused node isn't falsely IsUpToDate on the next term?

Examined stores.go:42-55, :120, :129-137, dispatcher_nodes.go:69. Found: reset() remakes s.nodes, discarding all nodeStore objects (lastConfigChange is a nodeStore field); re-registered node starts at 0. Comparison is exact-equality on nanosecond timestamp. Conclusion: RESOLVED - lastConfigChange is effectively zeroed (nodeStore recreated); no false IsUpToDate (see dispatch-store-bijection R6 analysis).

#### Does reset() zero lastConfigChange (affects recovered nodes told IsUpToDate / dangling never re-pulled)?

Same evidence as above (stores.go reset remakes nodes map; equality comparison dispatcher_nodes.go:69). Conclusion: RESOLVED - duplicate of prior question; zeroed, not exploitable.

#### Is there a concrete Inc-without-Dec path today, or is the current code balanced?

Examined all gauge sites. Found: dispatchedEndpoints (dispatcher_endpoints_configs.go:49 Inc / :63 Dec) has NO Delete and is NOT cleared by reset() (stores.go:42-55 remakes endpointsConfigs map without decrementing) -> on AD replay after each leadership loss addEndpointConfig Inc's again, ratcheting the endpoint_checks.configs_dispatched gauge up every cycle. Also the narrow nodeAgents.Inc post-reset race (above). Balanced gauges: dispatchedConfigs, nodeAgents (both cleared in reset loop), danglingConfigs/unscheduledCheck (clearDangling). Conclusion: RESOLVED - yes, dispatchedEndpoints is a concrete Inc-without-clear path surviving resets.


---

## Source discovery evidence (raw, per contributing agent)


### from `clusterchecks-reset-clears-store`

## Property
reset() on leadership loss leaves no stale in-memory state or telemetry.

## Reset path
`handler.go:180-196` `runDispatch`: on `dispatcher.run` returning (leadership lost, `dispatchCancel()`), calls `RemoveScheduler` THEN `dispatcher.reset()`.
`dispatcher.reset()` (`dispatcher_main.go:294-304`): clears `ksmShardedConfigs` under `ksmShardingMutex`, then under `store.Lock()` calls `store.reset()`.
`store.reset()` (`stores.go:42-56`):
```
for _, node := range s.nodes { dispatchedConfigs.Delete(node.name, ...); nodeAgents.Dec(...) }
s.active = false
s.digestToConfig = make(...); s.digestToNode = make(...); s.nodes = make(...)
s.clearDangling(); s.endpointsConfigs = make(...); s.idToDigest = make(...)
```
`clearDangling()` (`stores.go:99-108`) deletes the `danglingConfigs` gauge but leaves `unscheduledCheck` series sticky by design.

## Known-asymmetry evidence
- Explicit sticky comment on unscheduledCheck (stores.go).
- Historical fixes #52876, #52078, #50715 all addressed gauges/maps not reset symmetrically (analysis §8).
- KSM ordering race self-documented at handler.go:187-191.

## Failure scenario
1. Leader dispatches configs to nodes A,B,C; gauges dispatchedConfigs{A},{B},{C}=n, nodeAgents=3.
2. Leadership lost (partition). Concurrently, before dispatchCancel fully stops processNodeStatus, a heartbeat from a new node D auto-registers (getOrCreateNodeStore increments nodeAgents) AFTER store.reset()'s loop snapshot but the run loop is tearing down.
3. reset()'s `for _, node := range s.nodes` iterated the map as it was; the racing insert leaves a gauge series (nodeAgents / dispatchedConfigs{D}) that reset never decremented.
4. Next term: gauges report a phantom node; nodes_reporting != ground truth. Or unscheduledCheck stays >0 for a config no longer present.
5. Assertion after reset: store maps non-empty or gauge > 0 -> VIOLATION.

## Assertion point (MISSING — net-new)
At the tail of `dispatcher.reset()` (after `store.reset()` returns, still under intent of quiescence): `assert.AlwaysOrUnreachable(all maps empty && active==false && ksmShardedConfigs empty && dispatchedConfigs/nodeAgents/danglingConfigs gauges == 0, "reset clears dispatch store and telemetry", details)`. Because processNodeStatus can race, the assertion should be evaluated once the run loop is confirmed stopped (post dispatchCancel join) to distinguish a true asymmetry from an in-flight insert — the race itself is a finding.

## Existing coverage gap
No test flaps leadership against a real lease; the E2E leader-restart path is dead code (restartLeader=false), and unit tests never exercise reset under concurrent heartbeat/Schedule (analysis §9).


### from `dangling-configs-gauge-leak-and-recovery`

## Property
(1) `danglingConfigs` gauge == `len(store.danglingConfigs)` at every quiescent point. (2) Once >=1 live node exists and warmup has completed, `len(store.danglingConfigs)` drains toward 0 (dangling redispatch works).

## Where (code paths)
- Map: `pkg/clusteragent/clusterchecks/stores.go:30` `danglingConfigs map[string]*danglingConfigWrapper`, keyed by config digest (so map size is bounded by distinct configs, not unbounded).
- Gauge Inc sites: `dispatcher_configs.go:140` (addConfig, new dangling), `dispatcher_nodes.go:161` (expireNodes moves node configs to dangling).
- Gauge Dec sites: `dispatcher_configs.go:191` (removeConfig dangling branch), `dispatcher_configs.go:229` (deleteDangling), `stores.go:110` (clearDangling deletes the whole gauge label).
- reset(): `stores.go:42-55` calls `clearDangling()` (:52) which does `danglingConfigs.Delete(...)` and reallocates the map; `dispatcher.reset()` (dispatcher_main.go:294-304) wraps it under the store lock. Called on every leadership loss via runDispatch (handler.go:194).
- Redispatch loop: `dispatcher_main.go:400-411` on cleanupTicker: `shouldDispatchDangling()` requires `len(danglingConfigs)>0 && len(nodes)>0` (dispatcher_configs.go:212), then reschedule + deleteDangling.

## Bound analysis (correcting the 'unbounded growth' lead)
Because `danglingConfigs` is keyed by digest and `addConfig` guards with `if _, found := ...; !found` (dispatcher_configs.go:139), the map cannot exceed the number of distinct configs even with all nodes down. The genuine resource hazards are: (a) the gauge Inc/Dec sites drifting out of sync with the map (telemetry leak), and (b) failure to drain after recovery.

## Failure scenario (leak)
1. Repeated leadership flaps run reset()->clearDangling(), which zeroes the gauge label; concurrently expireNodes/addConfig Inc the gauge.
2. If a redispatch path deletes a dangling entry from the map without the matching `danglingConfigs.Dec` (or double-Inc's the same digest via a race between the cleanupTicker reschedule at :407 and a concurrent Schedule), the gauge value and `len(map)` diverge and the gauge ratchets across cycles.

## Failure scenario (non-recovery)
1. All node agents partitioned > node_expiration_timeout=30s -> expireNodes moves every config to dangling, logs 'No nodes reporting' (dispatcher_nodes.go:184).
2. A node returns. `shouldDispatchDangling` should fire on the next cleanupTicker (15s). If warmup/active flags or a reset race leave `active=false` or nodes empty at the check instant, dangling never drains -> checks stop running.

## Assertion (net-new)
At each cleanupTicker tick (dispatcher_main.go:400) under the store lock: `assert.Always(gaugeValue(danglingConfigs) == len(d.store.danglingConfigs))`. Plus a liveness/`Sometimes` marker that `len(danglingConfigs)==0` is reached after a node-recovery cycle.

## Key observations
- The gauge is a single-label gauge (`le.JoinLeaderValue`), so cross-cycle carryover is plausible if any path skips the Dec.
- `clearDangling` comment (stores.go:101-103) is truncated/garbled, hinting the reset bookkeeping here has been patched before.
