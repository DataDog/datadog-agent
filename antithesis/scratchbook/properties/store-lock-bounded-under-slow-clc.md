# store-lock-bounded-under-slow-clc — Dispatch store write lock is never held across a CLC-runner HTTP call

**Type:** Safety · **Assertion:** `Always` · **Priority:** P0 · **Intent:** invariant

**Provenance:** merged from 2 discovery agent(s): store-write-lock-bounded-under-slow-clc-runners, store-write-lock-held-across-clc-runner-http

## Property

The dispatcher never holds the global clusterStore write lock across a blocking network call, so a slow/partitioned CLC runner cannot stall node heartbeats, config polls, and dispatch.


## Invariant / assertion

`assert.Always` (structural, no wall-clock bound): no outbound CLC-runner HTTP call (GetRunnerStats/GetRunnerWorkers) is ever in progress while d.store's write lock is held. Implement a `storeLockHeld` boolean set/cleared around Lock/Unlock and assert it is false at the HTTP call site in updateRunnersStats. A duration bound was rejected (3 evaluation lenses): Antithesis controls the scheduler, so wall-clock 'time under lock' is not a faithful production proxy and the threshold is arbitrary. The real invariant is structural — no blocking I/O under the lock.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: an HTTP call to a CLC runner was initiated from the stats path under contention.


## Antithesis angle

updateRunnersStats takes d.store.Lock() then makes synchronous HTTP calls (GetRunnerWorkers/GetRunnerStats) to every CLC runner while holding it (dispatcher_nodes.go:201-245). N slow runners serialize; every processNodeStatus (heartbeat), getClusterCheckConfigs (poll), and Schedule blocks for N×timeout. Inject latency/partition to CLC-runner IPs during a rebalance and assert node-agent poll latency and lock-hold time stay bounded.


## Why it matters

A single slow CLC runner stalls the entire dispatch control plane and trips the clusterchecks-dispatch liveness probe (dispatcher_main.go:398-399 self-acknowledges 'might hang'), causing a pod restart and needless leadership churn. Surfaced by 2 focus agents.


## Mechanism refinement (from open-question investigation)

Threshold calibration (not an invalidation): worst-case store-write-lock hold in updateRunnersStats ~= numNodes x 2 (GetRunnerWorkers+GetRunnerStats, since rebalance_with_utilization defaults true) x 2s per-call timeout. e.g. 8 nodes -> ~32s > 30s health timeout. Evidence: clcrunner.go:86, common_settings.go:581, dispatcher_nodes.go:209/219.


## Fault dependencies

- network latency/congestion on leader->CLC-runner HTTP (enabled by default)
- asymmetric partition of a subset of CLC runners
- requires leader_election + advanced_dispatching + CLC runners in the topology


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. SUT-side timing: record lock-acquire/release timestamps; `assert.Always(held < bound)`. The real fix is to snapshot node IPs, drop the lock, then do HTTP — the assertion documents the requirement.


## Open questions (post-investigation)

- Whether any production Helm/Operator deployment runs rebalance_period low enough (default 10m) to hit this outside a tuned harness; only rebalance() (rebalanceTicker) reaches updateRunnersStats, so triggering fast needs a lowered rebalance_period or forced rebalance. `(needs human input)`


### Investigation Log

#### Exact aggregate CLC runner client per-call timeout (dial+TLS+response-header).

Examined pkg/util/clusteragent/clcrunner.go:60-90. Found: init() builds http.Client with Transport{TLSClientConfig:...} only (no per-phase Dial/ResponseHeader timeouts) and sets c.clcRunnerAPIClient.Timeout = 2*time.Second (:86). Conclusion: the single 2s http.Client.Timeout is the total per-call deadline covering dial+TLS+response header+body; worst case per runner call = 2s. RESOLVED.

#### Whether rebalance_with_utilization is enabled by default.

Examined pkg/config/setup/common_settings.go:581 and config_test.go:1394. Found: BindEnvAndSetDefault(cluster_checks.rebalance_with_utilization, true); config_test asserts it true. Conclusion: default ON, so the GetRunnerWorkers branch (dispatcher_nodes.go:209) is also taken -> 2 HTTP calls/node under the write lock. RESOLVED.

#### Whether any production deployment runs rebalance_period low enough to hit this.

Examined common_settings.go:586 (rebalance_period default 10m) and dispatcher_main.go:388,415-417 (only rebalanceTicker fires rebalance -> updateRunnersStats). Conclusion: code path clear but whether any real deployment lowers rebalance_period is a deployment/intended-behavior call. NEEDS-HUMAN.

#### What is the CLCRunnerClient HTTP timeout (clcrunner.go)?

Same as Q1: clcrunner.go:86 http.Client.Timeout = 2*time.Second. Conclusion: 2s. Worst-case lock hold ~ N nodes x 2 calls x 2s. RESOLVED.


---

## Source discovery evidence (raw, per contributing agent)


### from `store-write-lock-bounded-under-slow-clc-runners`

## Property
The dispatcher must never hold the `clusterStore` write lock across an unbounded number of blocking network calls. Every locked critical section completes within a bounded wall-clock time.

## Where (code paths)
- `pkg/clusteragent/clusterchecks/dispatcher_nodes.go:201-245` `updateRunnersStats`:
  - `d.store.Lock()` at :201, `defer d.store.Unlock()` at :202.
  - Inside the `for name, node := range d.store.nodes` loop, **two synchronous HTTP calls per node** are made while the write lock is held: `d.clcRunnersClient.GetRunnerWorkers(ip)` (:209, only when `rebalance_with_utilization`) and `d.clcRunnersClient.GetRunnerStats(ip)` (:219).
- `pkg/clusteragent/clusterchecks/dispatcher_nodes.go:142-186` `expireNodes`: holds `d.store.Lock()` across a nested loop over `idToDigest` per expired node (no network, but same global lock).
- Called from `pkg/clusteragent/clusterchecks/dispatcher_rebalance.go` on the `rebalanceTicker` (default `rebalance_period` = 10m) and during rebalance, while advanced dispatching is on.
- Every store reader contends on the same lock: `getState` (dispatcher_configs.go:39), `getClusterCheckConfigs` (dispatcher_nodes.go:29), `processNodeStatus` (dispatcher_nodes.go:47), `retrieveDangling` (dispatcher_configs.go:217), `expireNodes` (dispatcher_nodes.go:145).

## CLC runner client timeouts
The CLC runner HTTP client (`pkg/util/clusteragent`) uses per-call timeouts; the aggregate under the lock is roughly `N x (dial + response-header timeout)`. There is no per-cycle deadline bounding the whole locked loop.

## Failure scenario
1. DCA is leader, advanced dispatching enabled, N CLC runners registered.
2. Antithesis injects latency/congestion (or a partial partition) so each runner's stats endpoint hangs until its timeout.
3. `rebalanceTicker` (or utilization refresh) fires `updateRunnersStats`; the write lock is taken and held while all N calls time out sequentially.
4. During this window every node-agent `GET /configs` and `POST /status` request blocks (data plane wedged); the `clusterchecks-dispatch` liveness probe cannot make progress.
5. Hold duration exceeds threshold T -> assertion fails; in production this trips the liveness probe (~30s) -> pod restart -> leadership churn.

## Assertion (net-new; no SDK instrumentation exists today)
At the top of `updateRunnersStats` capture `start := time.Now()` after `d.store.Lock()`; in the deferred unlock path assert `assert.Always(time.Since(start) < T, "store write lock held bounded time", details{node_count, elapsed})`. Same pattern for `expireNodes`.

## Key observations
- The write lock spans the network calls, not just the map mutation; a copy-IPs-then-release-then-fetch refactor would fix it, exactly the kind of change this property protects.
- `getState` was already refactored (dispatcher_configs.go:26-67) to copy under lock and scrub after release, showing the maintainers know this class of stall exists; `updateRunnersStats` was not given the same treatment.

## Timing window
Window width scales with registered runner count N and injected per-call latency; with N=20 and 3s response-header timeout the lock can be held ~60s, well past the liveness budget.


### from `store-write-lock-held-across-clc-runner-http`

## Claim
In `updateRunnersStats` the full store **write** lock is held across synchronous HTTP calls to every registered CLC runner, so one slow/unreachable runner stalls all dispatch.

## Code path (verified)
`pkg/clusteragent/clusterchecks/dispatcher_nodes.go:190-246`
```go
func (d *dispatcher) updateRunnersStats() {
    ...
    d.store.Lock()          // :201  full store WRITE lock
    defer d.store.Unlock()  // :202
    for name, node := range d.store.nodes {   // :203
        ...
        workers, err := d.clcRunnersClient.GetRunnerWorkers(ip)  // :209  blocking HTTPS
        ...
        stats, err := d.clcRunnersClient.GetRunnerStats(ip)      // :219  blocking HTTPS
        ...
    }
}
```
Both `GetRunnerWorkers` and `GetRunnerStats` are synchronous per-runner HTTPS round-trips executed *inside* the loop while the store write lock is held. There is no per-call goroutine, no snapshot-then-unlock, no timeout shorter than the runner client's own.

## Contended lock users (all block for the whole hold)
- `processNodeStatus` acquires `d.store.Lock()` at `dispatcher_nodes.go:47` on **every node-agent heartbeat**.
- `getClusterCheckConfigs` acquires `d.store.RLock()` at `dispatcher_nodes.go:29` on **every node config poll** (`GET /configs/{id}`).
- `expireNodes` (`:145`), `scanUnscheduledChecks` (`:309`), `reset` (`:301`), the dangling redispatch write (`dispatcher_main.go:408`) all need the store lock.
Go's `sync.RWMutex`: a pending writer (updateRunnersStats) also blocks new RLock holders, so reads and writes both stall.

## Cascade (verified mechanisms)
- `node_expiration_timeout` default 30s; cleanup ticker fires every 15s (`dispatcher_main.go:385`). If the hold exceeds ~30s, heartbeats can't refresh and `expireNodes` mass-expires nodes -> `"No nodes reporting, cluster checks will not run"` (`dispatcher_nodes.go:183-185`).
- The dispatch loop's liveness probe channel `healthProbe.C` (`dispatcher_main.go:398`) is only serviced between ticker events; a long `updateRunnersStats` (called from `rebalance`) delays servicing -> liveness fail -> pod restart -> leadership churn.

## Trigger frequency caveat
`updateRunnersStats` is reached via `rebalance`, gated by `rebalanceTicker` = `cluster_checks.rebalance_period` (default **10m**). For Antithesis to exercise this quickly the harness should set `rebalance_period` low and enable `advanced_dispatching_enabled` with `rebalance_with_utilization` so the `GetRunnerWorkers` branch (:208-217) is also taken.

## Suggested SUT instrumentation (MISSING - net new)
Around `dispatcher_nodes.go:201-202`, record `start := time.Now()` before `d.store.Lock()` completes, and on unlock `assert.Always(time.Since(start) < nodeExpirationTimeout, "updateRunnersStats store-lock hold bounded", ...)`. Optionally an `assert.Sometimes(time.Since(start) > 5*time.Second, ...)` to confirm the fault actually reaches the stall.
