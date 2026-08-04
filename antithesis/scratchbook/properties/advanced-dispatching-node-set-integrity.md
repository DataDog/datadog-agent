# advanced-dispatching-node-set-integrity — Advanced dispatching operates only on a valid CLC-runner node set

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P1 · **Intent:** known-defect-reproducer

**Provenance:** merged from 3 discovery agent(s): advanced-dispatching-oneway-latch-stale, advanced-dispatching-nodetype-latch-invariant, empty-realip-node-poisons-utilization-rebalance

## Property

While utilization-based (advanced) dispatching is enabled, every node it rebalances over is a CLC runner with a resolvable IP; a plain node-agent heartbeat or an empty-clientIP node must not silently corrupt the utilization view — and the one-way disable latch must reflect current composition.


## Invariant / assertion

Two `assert.AlwaysOrUnreachable` sub-invariants (distinct messages): (a) while advancedDispatching==true, no node in the store has nodetype==NodeAgent; (b) no node with empty clientIP is treated as a reachable CLC runner for utilization stats. AlwaysOrUnreachable fits — advanced dispatching is optional, but whenever on, the node set must be valid.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: a NodeAgent-typed heartbeat and/or an empty-X-Real-Ip heartbeat was processed while advanced dispatching was enabled.


## Antithesis angle

disableAdvancedDispatching is a one-way CAS true→false (dispatcher_main.go:369-374) triggered by ANY NodeAgent-typed heartbeat (dispatcher_nodes.go:60-62); it never re-enables for the dispatcher's lifetime. Separately, a node with empty X-Real-Ip (legacy agent) gets DefaultNumWorkers substituted / stale busyness (dispatcher_nodes.go:209-223), poisoning rebalance weights. Inject one spurious NodeAgent heartbeat and one empty-clientIP heartbeat during warmup; assert the mode/latch and rebalance node-set stay valid.


## Why it matters

A single stray/transient heartbeat permanently degrades load distribution (and disables KSM sharding) cluster-wide until process restart — a durable downgrade from a momentary blip. Merged from 3 focus agents.


## Mechanism refinement (from open-question investigation)

Scope strengthened. Discovery assumed the disable latch resets to true on each leadership term ('fresh dispatcher term restores advancedDispatching=true'); code disproves this: reset()/store.reset() never touch d.advancedDispatching (dispatcher_main.go:294-304, stores.go:42-55) and the dispatcher is constructed once (handler.go:74). The latch is therefore PER-PROCESS — once disabled by any NodeAgent heartbeat it stays disabled across all subsequent leadership cycles until process restart. The 'once observed false, never true again' monotonicity assertion should be scoped to the whole process lifetime, and the stale-mode/degraded-load-distribution window persists across leadership recoveries, not just within one term.


## Fault dependencies

- none beyond default config (advanced_dispatching_enabled=true); needs a crafted NodeAgent-typed and an empty-X-Real-Ip heartbeat in the workload
- requires leader_election enabled


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. Two distinct assertions per the sub-invariants. A `Reachable` on the disable-latch CAS confirms the downgrade path is exercised.


## Open questions (post-investigation)

None — all resolved (see Investigation Log).


### Investigation Log

#### Can a CLC runner transiently report NodeType==NodeTypeNodeAgent during startup?

Examined comp/core/autodiscovery/providers/clusterchecks.go:42,57,73-78,130,229. Found: the client's nodeType is set once at provider construction — default NodeTypeCLCRunner (line 57), then set from the static IsCLCRunner config (lines 73-78) before the first heartbeat is sent, and emitted identically on every heartbeat (:130,:229). Conclusion: a correctly-configured CLC runner never transiently sends NodeAgent; the value is config-derived and fixed at startup. Resolved.

#### Is advancedDispatching re-initialized to true on every dispatcher construction, or persisted?

Examined dispatcher_main.go:111-122 and handler.go:74. Found: advancedDispatching.Store(true) runs only at construction, and only if GetCLCRunnerClient() succeeds. newDispatcher is called exactly once (NewHandler, handler.go:74). Conclusion: the true default is a per-process construction default, set once; not reloaded per leadership term. Resolved.

#### Does reset() create a fresh dispatcher (re-Store(true)) or reuse the atomic?

Examined dispatcher_main.go:294-304 (reset), stores.go:42-55 (store.reset), handler.go:180-195. Found: on leadership loss runDispatch calls d.dispatcher.reset(), which only clears ksmShardedConfigs and calls store.reset(); store.reset() rebuilds store maps but NEVER touches d.advancedDispatching. The same dispatcher struct (constructed once) is reused across leadership cycles. Conclusion: the disable latch is PER-PROCESS, not per-leadership-cycle. This DIRECTLY CONTRADICTS the scratchbook discovery claim that 'a fresh dispatcher term restores advancedDispatching=true.' Resolved.

#### Do any real legacy node-agent versions still omit node_type?

Examined git log for the node_type field: added by commit 3cee1cf97a3 on 2025-07-18 (~7.68). Found: any agent built before mid-2025 sends no node_type, which decodes to 0; 0 != NodeTypeNodeAgent so the latch never trips for such node agents. Conclusion: the omit-node_type gap is broadly reachable across a large fraction of deployed agents, not only via a hand-crafted client. Resolved.

#### Does a legacy node agent that omits X-Real-Ip also omit node_type (co-occur), or independent?

Examined pkg/util/clusteragent/clusteragent.go:44,150 (X-Real-Ip set to pod IP; per validateClientIP comment requires agent >=6.17, ~2020) vs node_type added 2025-07-18. Found: agents in the 6.17..~7.67 range send X-Real-Ip but omit node_type. Conclusion: the two gaps occur INDEPENDENTLY; empty-X-Real-Ip (pre-6.17) is far older/rarer than omit-node_type. They do not necessarily co-occur. Resolved.

#### What does GetRunnerWorkers('')/GetRunnerStats('') do — immediate error vs blocking dial?

Examined pkg/util/clusteragent/clcrunner.go:81-97,133-198. Found: with IP=='', runnerURL uses net.JoinHostPort('',port)=':port', http.NewRequest succeeds, and clcRunnerAPIClient.Do(req) attempts a real dial; the client has Timeout=2s (line 86). Conclusion: neither an immediate error nor an unbounded block — each call blocks up to ~2s while the store write lock is held (dispatcher_nodes.go:201-224), so N empty-IP nodes cost up to N*2s of lock-stall. Resolved.


---

## Source discovery evidence (raw, per contributing agent)


### from `advanced-dispatching-oneway-latch-stale`

## Mechanism (verified)

`pkg/clusteragent/clusterchecks/dispatcher_main.go`:
```go
func (d *dispatcher) disableAdvancedDispatching() {
    if d.advancedDispatching.CompareAndSwap(true, false) {   // one-way
        log.Info("Node agents detected in cluster check pool, disabling advanced dispatching")
    }
}
```
There is no `CompareAndSwap(false,true)` / `Store(true)` anywhere in the term. `UpdateAdvancedDispatchingMode` (dispatcher_main.go:352-366) scans nodes and, if `hasNodeAgent`, calls `disableAdvancedDispatching()` — it can only turn it OFF.

`pkg/clusteragent/clusterchecks/dispatcher_nodes.go:59-62`:
```go
if d.advancedDispatching.Load() && status.NodeType == types.NodeTypeNodeAgent {
    d.disableAdvancedDispatching()
}
```
A single heartbeat with `NodeType==NodeTypeNodeAgent` latches it.

The only reset is a leadership cycle: `runDispatch` → `dispatcher.run` returns on ctx cancel → `dispatcher.reset()` (handler.go:185-194) rebuilds the store; a fresh dispatcher term restores advancedDispatching=true (construction default).

## Failure scenario
1. Cluster is all CLC-runners; leader completes warmup with advanced dispatching = true.
2. One node agent (non-CLC-runner) POSTs a single heartbeat → NodeType=NodeTypeNodeAgent → latch flips false.
3. That node stops reporting and expires after node_expiration_timeout (30s) — it's gone from the store.
4. `advancedDispatching` stays false for the rest of the term. `getNodeToScheduleCheck` (dispatcher_nodes.go:98-104) now uses getNodeWithLessChecks, rebalanceTicker's `if d.advancedDispatching.Load()` (dispatcher_main.go:416) never rebalances by utilization. Mode is stale vs a now-all-CLC cluster.

## Is this intended? Partly — mixed clusters legitimately disable advanced mode. The bug is the *irreversibility from transient input*: the mode never re-evaluates against current composition within a term. The assertion targets the divergence between latched mode and live composition.

## Assertions to add (MISSING)
- `assert.Sometimes(advancedDispatching==false && countNodesOfType(NodeTypeNodeAgent)==0, "advanced dispatching disabled while no node agents present")` evaluated in the rebalance/cleanup tick (dispatcher_main.go:400-418).
- `assert.Always(!(prevAdvanced==false && curAdvanced==true), "advancedDispatching never re-enabled within a term")` around disableAdvancedDispatching to lock in and surface the one-way property.


### from `advanced-dispatching-nodetype-latch-invariant`

## Mechanism

`cluster_checks.advanced_dispatching_enabled` defaults to **true** (`pkg/config/setup/common_settings.go:580`). On dispatcher creation the flag is set true iff a CLC-runner client is built (`pkg/clusteragent/clusterchecks/dispatcher_main.go:111-122`).

The flag is **only ever cleared**, never re-set, via a one-way CAS:
```go
// dispatcher_main.go:369-374
func (d *dispatcher) disableAdvancedDispatching() {
    if d.advancedDispatching.CompareAndSwap(true, false) {
        log.Info("Node agents detected in cluster check pool, disabling advanced dispatching")
    }
}
```
Grep confirms the only writes are `Store(true)` at init (line 121) and the CAS(true,false); there is no re-enable path anywhere in the package.

The two triggers both key on `== NodeTypeNodeAgent`:
```go
// dispatcher_nodes.go:59-62 (per heartbeat)
if d.advancedDispatching.Load() && status.NodeType == types.NodeTypeNodeAgent {
    d.disableAdvancedDispatching()
}
// dispatcher_main.go:353-366 (pool scan)
if nodetype == cctypes.NodeTypeNodeAgent { hasNodeAgent = true; break }
```

## The version-compatibility gap

`NodeType` is `uint8` with only two defined values and `omitempty` JSON:
```go
// types/types.go:24-37 — comment: "distinguish between CLC runners, node agents, and unknown types"
NodeTypeCLCRunner NodeType = 1
NodeTypeNodeAgent NodeType = 2
type NodeStatus struct { LastChange int64; NodeType NodeType `json:"node_type,omitempty"` }
```
Modern clients set it explicitly (`comp/core/autodiscovery/providers/clusterchecks.go:75-77`). A **legacy node agent predating the `node_type` field sends no field → decodes to 0**. `0 != NodeTypeNodeAgent`, so neither trigger fires and advanced dispatching stays enabled with a non-runner node registered in `d.store.nodes`.

## Failure scenario

1. Leader elected, advanced dispatching = true.
2. Legacy node agent heartbeats with `node_type` omitted → node registered, nodetype=0.
3. Latch never fires; `getNodeToScheduleCheck()` (dispatcher_nodes.go:98-104) returns `getRandomNode()` and may pick the 0-typed node.
4. `updateRunnersStats` calls `GetRunnerWorkers(ip)`/`GetRunnerStats(ip)` against a client with no runner API → errors, DefaultNumWorkers substituted, busyness skewed; checks dispatched there may not execute.

## Suggested assertion (MISSING — net-new)

At dispatch/rebalance decision points assert:
```go
assert.AlwaysOrUnreachable(
  !d.advancedDispatching.Load() || node.nodetype == types.NodeTypeCLCRunner,
  "advanced dispatching implies node is a CLC runner",
  map[string]any{"node": node.name, "nodetype": node.nodetype})
```
Also a monotonicity assertion: once observed false, never observed true within one dispatcher instance.

No Antithesis SDK exists in the tree yet (see existing-assertions.md); this is entirely net-new instrumentation.


### from `empty-realip-node-poisons-utilization-rebalance`

## Path

```go
// cmd/cluster-agent/api/v1/clusterchecks.go:58
clientIP, err := validateClientIP(r.Header.Get(dcautil.RealIPHeader), sc.ClusterCheckHandler.IsAdvancedDispatchingEnabled())
```
`dcautil.RealIPHeader == "X-Real-Ip"` (`pkg/util/clusteragent/clusteragent.go:44`); modern clients set it to their pod IP (clusteragent.go:150).

```go
// clusterchecks.go:186-197
func validateClientIP(addr string, advancedDispatchingActive bool) (string, error) {
    if addr != "" && net.ParseIP(addr) == nil { return "", fmt.Errorf(...) }
    if addr == "" && advancedDispatchingActive {
        log.Warn("...cannot get runner IP from http headers. advanced_dispatching_enabled requires agent 6.17 or above.")
    }
    return addr, nil   // <-- returns ("", nil): empty IP tolerated
}
```
The empty IP flows into `PostStatus` → `processNodeStatus` and is stored as `node.clientIP` (`dispatcher_nodes.go:44-57`).

## Consumption under advanced dispatching (defaults on)

`cluster_checks.advanced_dispatching_enabled` and `cluster_checks.rebalance_with_utilization` both default **true** (`pkg/config/setup/common_settings.go:580-581`).

```go
// dispatcher_nodes.go:201-216 — full store write lock held across these HTTP calls
d.store.Lock(); defer d.store.Unlock()
for name, node := range d.store.nodes {
    ip := node.clientIP            // may be ""
    if ...GetBool("cluster_checks.rebalance_with_utilization") {
        workers, err := d.clcRunnersClient.GetRunnerWorkers(ip)   // GetRunnerWorkers("")
        if err != nil { node.workers = pkgconfigsetup.DefaultNumWorkers }  // silent substitution
    }
    stats, err := d.clcRunnersClient.GetRunnerStats(ip)          // GetRunnerStats("")
    if err != nil { statsCollectionFails.Inc(...); continue }
}
```

## Failure scenario

1. Legacy node agent heartbeats with no X-Real-Ip → node stored with clientIP=="".
2. Advanced dispatching still on (empty IP alone does not disable it).
3. Rebalance loop calls runner-stats API with empty IP → all calls fail → busyness computed from DefaultNumWorkers + zero stats → skewed rebalancing.
4. Under latency, the failing calls serialize while the store write lock is held → dispatch stall.

## Suggested assertion (MISSING — net-new)

```go
assert.AlwaysOrUnreachable(
  !d.advancedDispatching.Load() || node.clientIP != "",
  "advanced dispatching implies node has a resolvable client IP",
  map[string]any{"node": name})
```
Optionally assert.Reachable at the `addr=="" && advancedDispatchingActive` branch in validateClientIP to confirm the legacy path is exercised.
