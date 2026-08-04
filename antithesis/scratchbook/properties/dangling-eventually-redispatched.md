# dangling-eventually-redispatched — Dangling configs are eventually re-dispatched when a node is available

**Type:** Liveness · **Assertion:** `Sometimes` · **Priority:** P1 · **Intent:** invariant

**Provenance:** merged from 1 discovery agent(s): dangling-configs-gauge-leak-and-recovery

## Property

When at least one live node exists, configs in danglingConfigs are eventually re-dispatched (the dangling map drains toward zero), so a check orphaned by node loss resumes running.


## Invariant / assertion

`assert.Sometimes(dangling_map_drained_to_zero_with_live_nodes)`: during a quiet period after a node-loss event, with >=1 node registered, danglingConfigs reaches empty. Sometimes fits — a liveness/progress condition verified under a recovery window.


## Antithesis angle

The cleanup ticker (node_expiration_timeout/2 = 15s) calls shouldDispatchDangling (requires >=1 node) then reschedule→deleteDangling (dispatcher_main.go:400-411). With ZERO nodes, dangling is NOT drained — only a warning logs. Kill all node agents, then bring one back, and assert dangling flushes. Worst-case recovery ~node_expiration_timeout + cleanup period (~45s).


## Why it matters

If re-dispatch stalls, checks silently stop running (monitoring gap). The known corner case (zero nodes → no drain) means a full node-agent outage leaves every config stuck until a node returns.


## Fault dependencies

- network partition of node agents > node_expiration_timeout then heal (enabled by default)
- requires leader_election enabled + >=2 replicas (dispatch is leader-only)


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.Sometimes` in a workload liveness command after restoring a node under a quiet period.


## Open questions (post-investigation)

None — all resolved (see Investigation Log).


### Investigation Log

#### Does reset() zero lastConfigChange?

Examined stores.go:42-55/:120/:129-137, dispatcher_nodes.go:69. Found: reset() remakes s.nodes, nodeStore (and its lastConfigChange) discarded; re-registered node starts at 0; comparison is exact-equality. Conclusion: RESOLVED - zeroed; does not cause recovered nodes to be falsely IsUpToDate, so does not block dangling re-pull.

#### Is there a concrete Inc-without-Dec path today (affecting dangling drain)?

Examined danglingConfigs gauge sites: Inc addConfig(dispatcher_configs.go:140)/expireNodes(dispatcher_nodes.go:161), Dec removeConfig(:191)/deleteDangling(:229), Delete clearDangling(stores.go:110). Found: danglingConfigs is balanced. The only Inc-without-clear leak in the package is dispatchedEndpoints, which is unrelated to dangling drain liveness. shouldDispatchDangling (dispatcher_configs.go:208-213) requires len(nodes)>0; drain loop dispatcher_main.go:405-411 runs each cleanup tick (nodeExpirationSeconds/2). Conclusion: RESOLVED - no Inc-without-Dec affecting dangling; drain liveness path is intact once >=1 node exists.


---

## Source discovery evidence (raw, per contributing agent)


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
