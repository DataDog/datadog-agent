# node-expiry-monotonic-clock — Node expiry uses elapsed (monotonic) time, not wall clock

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P1 · **Intent:** known-defect-reproducer

**Provenance:** merged from 2 discovery agent(s): clock-skew-mass-node-expiry, clusterchecks-node-expiry-monotonic-not-wallclock

**⚠ Requires fault (disabled by default):** clock skew — inert unless the tenant enables it.

## Property

A node is expired (checks moved to dangling) only when real elapsed time since its last heartbeat exceeds node_expiration_timeout — a backward wall-clock jump must never mass-expire all nodes.


## Invariant / assertion

`assert.AlwaysOrUnreachable`: expireNodes removes a node only if monotonic elapsed since heartbeat > timeout. AlwaysOrUnreachable fits — expiry is periodic/optional, but any expiry decision must be based on real elapsed time. Today expiry uses time.Now().Unix() (helpers.go:52-53), so the property is currently expected to FAIL under backward skew — that is the point.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: a backward clock jump was applied while nodes had recent heartbeats.


## Antithesis angle

Heartbeat and cutoff use wall-clock Unix seconds (dispatcher_nodes.go:143-152). A backward NTP/clock jump makes heartbeat < cutoff fire for all nodes at once → 'No nodes reporting, cluster checks will not run' → every config dumped to dangling. Inject backward clock jitter and assert no spurious mass expiry.


## Why it matters

A single backward clock jump silently halts ALL cluster checks cluster-wide until nodes re-register — a severe, hard-to-diagnose outage triggered by an ordinary NTP correction. Merged from 2 focus agents.


## Mechanism refinement (from open-question investigation)

No change. Code confirms the property premise verbatim: expiry uses wall-clock seconds with no monotonic guard (helpers.go:52-53, dispatcher_nodes.go:143,152) and the leader stamps heartbeats on receipt, so the AlwaysOrUnreachable invariant and its expected-FAIL-under-skew framing stand as written.


## Fault dependencies

- clock jitter forward/backward (DISABLED BY DEFAULT) — the property is INERT unless the tenant enables clock faults
- requires leader_election enabled + >=2 replicas


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. The fix is to use a monotonic clock for expiry; the assertion (`AlwaysOrUnreachable`) checks expiry decisions against elapsed monotonic time. Flag to user: needs clock faults enabled.


## Open questions (post-investigation)

- Does the Antithesis harness/tenant actually enable clock-skew faults? Without them the property is inert. No harness config exists in the repo (only antithesis/scratchbook), so this is a tenant fault-config decision that cannot be settled from code. `(needs human input)`
- Do the KSM shard map and per-node gauges recover cleanly after a mass-expiry+redispatch at runtime? Code shows the ksmShardedConfigs map is untouched by the expiry/redispatch path, but full gauge recovery overlaps clusterchecks-dispatch-consistency-after-leadership-recovery and warrants a runtime check. `(partial)`


### Investigation Log

#### Heartbeat stamped on receipt (leader clock only) vs node-supplied timestamp?

Examined dispatcher_nodes.go:44-57 and types/types.go:33-37. Found: node.heartbeat = timestampNow() is set by the LEADER on receipt (dispatcher_nodes.go:56); the wire type NodeStatus carries only LastChange (a config-version counter, not wall-clock time) and NodeType. No node-supplied timestamp is read or trusted. Conclusion: only the leader's clock governs expiry; question resolved.

#### Does Go's time.Now() monotonic component shield the subtraction? (.Unix() strips it)

Examined helpers.go:47-54. Found: timestampNow() returns time.Now().Unix(); .Unix() returns wall-clock seconds and discards the monotonic reading. expireNodes (dispatcher_nodes.go:143) subtracts two such wall-clock values. Conclusion: monotonic clock does NOT protect this path; code is genuinely exposed to backward/forward wall-clock jumps. Resolved.

#### Does the harness enable clock skew (disabled by default)?

Examined antithesis/ directory (only scratchbook present) and grepped for clock/skew/jitter fault config. Not found: no harness/compose/fault config checked in. Conclusion: cannot resolve from repo; depends on tenant fault configuration. Kept as needs-human.

#### Do KSM shard map and gauges recover cleanly after mass-expiry+redispatch?

Examined dispatcher_nodes.go:142-186 (expireNodes), dispatcher_main.go:404-411 (redispatch), dispatcher_ksm.go:93-139, stores.go:42-55. Found: expireNodes moves per-node (already-sharded child) configs to danglingConfigs and deletes per-node gauges; reschedule() re-adds them to surviving nodes. Neither expireNodes nor reschedule touches ksmShardedConfigs (parent->shard map) — it is only cleared on leadership loss via reset() (dispatcher_main.go:297-299). Conclusion: shard map persists intact through expiry/redispatch; gauge/full-recovery behavior needs runtime confirmation and overlaps another property. Kept partial.


---

## Source discovery evidence (raw, per contributing agent)


### from `clock-skew-mass-node-expiry`

## Mechanism (verified)

`pkg/clusteragent/clusterchecks/helpers.go:47-54`:
```go
func timestampNowNano() int64 { return time.Now().UnixNano() }
func timestampNow() int64 { return time.Now().Unix() }   // wall clock, no monotonic
```

`pkg/clusteragent/clusterchecks/dispatcher_nodes.go`:
- heartbeat write: `node.heartbeat = timestampNow()` (:56).
- expiry:
```go
func (d *dispatcher) expireNodes() {
    cutoffTimestamp := timestampNow() - d.nodeExpirationSeconds   // :143
    ...
    if node.heartbeat < cutoffTimestamp {                        // :152
        // expire, move configs to dangling
    }
}
```

`nodeExpirationSeconds` = `cluster_checks.node_expiration_timeout` (default 30s, dispatcher_main.go:55). cleanupTicker fires every `node_expiration_timeout/2` = 15s (dispatcher_main.go:385, :400-402).

## Failure scenarios
- **Forward jump > 30s:** cutoff = (now+Δ) − 30 leaps past every stored heartbeat → all nodes fail `heartbeat < cutoff` → all expire in one tick → all configs to `danglingConfigs` → `shouldDispatchDangling`→`reschedule` re-dispatches the entire set (dispatcher_main.go:404-411). Metric 'nodes_reporting' → 0.
- **Backward jump > 30s:** cutoff becomes tiny/negative → no node ever expires; a genuinely dead node keeps its check assignments and those checks are never redispatched (liveness violation of the 'dead node → reassigned' guarantee, sut-analysis §10).

## Existing tests miss it
sut-analysis §9: node expiry is tested by hand-overwriting `heartbeat` fields, never against a real or skewed clock.

## Assertions to add (MISSING)
- In `expireNodes`, capture `before := len(d.store.nodes)` and `expired := <count>`; `assert.AlwaysOrUnreachable(before<=1 || expired < before, "expireNodes did not wipe a populated node pool in one pass", ...)` (dispatcher_nodes.go:142-160).
- Migrate freshness to a monotonic source and `assert.Always(cutoffTimestamp monotonic non-decreasing across ticks, "node-expiry cutoff never moves backward")`.
- `assert.Reachable("all nodes expired simultaneously")` to confirm the fault reproduces the state under clock jitter.


### from `clusterchecks-node-expiry-monotonic-not-wallclock`

## Verified wall-clock dependence
`timestampNow()` returns `time.Now().Unix()` (helpers.go:51-54) — monotonic component stripped. `nodeStore.heartbeat` is set to `timestampNow()` on each status (dispatcher_nodes.go:56). `expireNodes` computes `cutoffTimestamp := timestampNow() - d.nodeExpirationSeconds` and deletes any node with `node.heartbeat < cutoffTimestamp` (dispatcher_nodes.go:143-172). `nodeExpirationSeconds = cluster_checks.node_expiration_timeout` (dispatcher_main.go:55, default 30). Cleanup runs every `nodeExpirationSeconds/2` = 15s (dispatcher_main.go:385).

## Forward-jump failure scenario
1. 5 CLC runners healthy, heartbeats at wall-time T.
2. Antithesis steps the clock forward by 120s.
3. Next cleanup tick: `cutoffTimestamp = (T+120) - 30 = T+90`. Every node's heartbeat (~T) < T+90 → all 5 expired in one loop iteration. Log: 'No nodes reporting.' All dispatched configs → danglingConfigs (dispatcher_nodes.go:157-161).
4. Next tick `shouldDispatchDangling` true → full re-dispatch (dispatcher_main.go:404-411). Every cluster check bounces nodes → duplicate/gap metrics during the window.

## Backward-jump failure scenario
A dead node's last heartbeat is at T. Clock steps back to T-120. `cutoffTimestamp = (T-120)-30`. Dead node's heartbeat T is NOT < cutoff → never expired → its checks are never moved to dangling → the 'dead node reassigned' guarantee stalls until the clock recovers.

## Contrast with kubeactions
Kubeactions at least buffers 10s for clock skew in `ValidateTimestamp` (action_store.go:92-95); the clusterchecks expiry path has no such guard and no monotonic clock.

## Instrumentation (MISSING — net-new)
- Record a monotonic timestamp (`time.Now()` retained, or a `time.Duration` since a monotonic base) alongside `heartbeat`; in `expireNodes` assert `AlwaysOrUnreachable(monotonicElapsed(node) >= nodeExpirationSeconds, "node expired only after real elapsed timeout", ...)` before deleting.
- `assert.Reachable("all nodes expired in a single cleanup pass", ...)` at dispatcher_nodes.go:183 to confirm the mass-expiry state was exercised.

## Why existing tests miss it
§9: node expiry is tested by hand-overwriting `heartbeat` fields, never against a real or skewed clock; clock jitter is not modeled at all.
