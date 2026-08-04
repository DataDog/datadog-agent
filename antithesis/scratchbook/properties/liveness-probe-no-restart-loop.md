# liveness-probe-no-restart-loop — Health probe stays drained under transient, recoverable delay

**Type:** Liveness · **Assertion:** `Always` · **Priority:** P1 · **Intent:** invariant

**Provenance:** merged from 1 discovery agent(s): internal-lock-contention-liveness-restart-loop

## Property

The clusterchecks liveness health-probe goroutines (clusterchecks-dispatch, clusterchecks-leadership) always drain their health channel within the probe period under transient apiserver/CLC-runner slowness, so the DCA is not needlessly killed and thrown into leadership churn.


## Invariant / assertion

Reframed to the directly-observable symptom: a transient (bounded, recoverable) apiserver/CLC-runner delay does not drive the clusterchecks-dispatch / clusterchecks-leadership health probe to unhealthy. Instrument the probe-drain and assert it stays drained for delays below a recovery bound. The bare-container topology has no kubelet, so the restart→election→churn cascade is NOT asserted here (add a probe/restart shim to the topology if that cascade itself is to be tested); the assertable core is 'probe accuracy under recoverable latency'.


## Antithesis angle

Both probes only drain healthProbe.C in a no-op select case in the same loop that does blocking work (dispatcher_main.go:398, handler.go:211) — comments self-acknowledge hang risk. A blocking GetLeaderIP (apiserver Get, transport-default timeout) or a store deadlock stops the drain → unhealthy after ~2 missed 15s pings → restart → new election → churn. Inject bounded apiserver latency and assert liveness does NOT flap for transient (recoverable) slowness.


## Why it matters

Needless restarts convert a transient dependency blip into leadership churn, which (via other properties) risks dispatch starvation and split-brain windows — the assertable core is that the probe does not report unhealthy for a delay the system recovers from on its own.


## Mechanism refinement (from open-question investigation)

Assertion calibration: restart cascade requires the probe to stay undrained for ~90s (failureThreshold 6 x periodSeconds 15), not merely one 30s health-timeout window; the internal health component reports unhealthy at 30s (health.go). Instrument the recovery bound against ~90s sustained-failure, per Dockerfiles/manifests/.../cluster-agent-deployment.yaml:200-206.


## Fault dependencies

- network latency/congestion on DCA->apiserver and DCA->CLC-runner (enabled by default)
- node hang/throttle
- requires leader_election enabled


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. Distinguish 'transient slowness' from 'genuine deadlock' — the assertion should fire only when a recoverable delay caused a restart. SUT-side probe-drain timing instrumentation.


## Open questions (post-investigation)

- Where is the boundary between a legitimate liveness failure (real deadlock) and a false positive (recoverable transient latency)? The assertion must fire only when a delay the system recovers from on its own caused a restart, without penalizing correct hang-detection — an intended-behavior judgment. `(needs human input)`


### Investigation Log

#### Exact k8s liveness probe timeout/failureThreshold in shipped manifests.

Examined Dockerfiles/manifests/cluster-checks-runners/cluster-agent-deployment.yaml:199-208 and pkg/status/health/{global.go:22,health.go:15,76}. Found: canonical repo DCA livenessProbe = failureThreshold 6, periodSeconds 15, timeoutSeconds 5, initialDelay 15; health component internal timeout 30s, ping every 15s. Conclusion: /live goes unhealthy after ~30s of no channel drain; pod restart requires ~6 failed probes ~= 90s of sustained failure. Caveat: shipped Helm/Operator chart (separate repo) may override these. RESOLVED (with caveat).

#### Whether updateRunnersStats (rebalance 10m) vs cleanup (15s) is the overlapping lock user.

Examined dispatcher_main.go:385-419. Found: cleanupTicker = nodeExpiration/2 = 15s -> expireNodes (store lock, NO network); rebalanceTicker = 10m -> rebalance -> updateRunnersStats (store lock + HTTP). Both run in the SAME select goroutine that drains healthProbe.C (:398), so any long rebalance() starves the probe regardless of lock. Conclusion: the >30s-hold risk is updateRunnersStats (network under lock) via rebalance, not the frequent-but-I/O-free cleanup. RESOLVED.

#### Boundary between a legitimate liveness failure and a false positive.

Examined dispatcher_main.go:398-399 and handler.go:211-212 self-acknowledged 'might hang' comments. Conclusion: code cannot define the recoverable-vs-genuine-deadlock threshold; it is a design/intent decision the property must encode. NEEDS-HUMAN.


---

## Source discovery evidence (raw, per contributing agent)


### from `internal-lock-contention-liveness-restart-loop`

## Mechanism (all verified)

### Health probe = liveness = restart
`pkg/status/health/health.go`:
- `register` seeds each component's `healthChan` full so it starts unhealthy (:74-77).
- `run` pings every `pingFrequency` (15s) by filling the channel (:86-99).
- `RegisterLiveness` default timeout 30s (global.go:22-23). A component whose buffer stays full is reported unhealthy → /live fails → kubelet restarts the pod.

### Two clusterchecks liveness goroutines that can be blocked
1. **dispatcher.run** (`dispatcher_main.go:382-419`): registers `clusterchecks-dispatch`, drains at the `case <-healthProbe.C` arm (:398) — comment: *'This goroutine might hang if the store is deadlocked during a cleanup.'* Its `cleanupTicker`→`expireNodes`/`reschedule` and `rebalanceTicker`→`rebalance` paths take the store lock.
2. **leaderWatch** (`handler.go:197-233`): registers `clusterchecks-leadership`, drains at `case <-healthProbe.C` (:211) — comment: *'This goroutine might hang if the leader election engine blocks.'*

### Two confirmed ways to block them past 30s
- **Store write lock across HTTP** — `updateRunnersStats` (`dispatcher_nodes.go:201-245`): `d.store.Lock(); defer d.store.Unlock()` wraps a loop that calls `d.clcRunnersClient.GetRunnerWorkers(ip)` (:209) and `GetRunnerStats(ip)` (:219) synchronously for every node. N unreachable/slow runners serialize; the write lock is held for N×(dial+timeout). Any store-lock user — including the dispatch health-drain path and every data-plane reader (GetState/GetConfigs) — stalls.
- **Channel send under handler lock** — `updateLeaderIP` (`handler.go:246-277`): takes `h.m.Lock()` (:246, defer unlock) then `h.leadershipChan <- newState` on a buffered(1) channel (:276). If the `Run` consumer is mid-warmup (handler.go:129-141, up to 30s) during back-to-back transitions, the buffer is full and this send blocks under `h.m.Lock()`, wedging leaderWatch's own drain and every handler-lock reader.

## Failure scenario
Inject latency on DCA→CLC-runner connections so GetRunnerStats blocks ~10s each; with ≥4 nodes the store write lock is held >30s; `clusterchecks-dispatch` never drains `healthProbe.C`; liveness fails; kubelet kills the leader; lease drops; a follower self-promotes and re-warms up; the new leader hits the same slow runners → restart loop + leadership churn.

## Assertions to add (MISSING)
- Wrap each health-drain arm to record last-drain time; assert `assert.Always(sinceLastDrain < livenessTimeout, "clusterchecks liveness channel drained in time", ...)` — one for dispatcher_main.go:398, one for handler.go:211.
- `assert.AlwaysOrUnreachable(!storeWriteLockHeld, "no store write lock held across CLC-runner HTTP call")` bracketing dispatcher_nodes.go:209/:219 (release lock around I/O; snapshot IPs first).
- `assert.Reachable("leadershipChan send blocked under handler lock")` at handler.go:276 to catch the wedge.
