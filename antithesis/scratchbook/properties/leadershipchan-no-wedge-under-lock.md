# leadershipchan-no-wedge-under-lock — Leadership-state send never blocks while holding the handler mutex

**Type:** Safety · **Assertion:** `Always` · **Priority:** P0 · **Intent:** invariant

**Provenance:** merged from 3 discovery agent(s): leadership-transition-does-not-wedge-dispatcher, leadership-chan-buffered1-send-under-lock, leadershipchan-send-under-handler-mutex

## Property

A leadership-state transition send on the buffered(1) leadershipChan never blocks while h.m is held, because that mutex guards every node-agent-facing clusterchecks request handler.


## Invariant / assertion

`assert.Always`: the `h.leadershipChan <- newState` in updateLeaderIP (executed under h.m.Lock, handler.go:246-277) never blocks — i.e. the channel is never full at send time, or the send is moved out from under the lock. Always fits — a no-blocking-under-lock invariant on every transition.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: a second leadership transition arrived while Run was mid-warmup (leadershipChan full at send).


## Antithesis angle

leadershipChan is buffered size 1; updateLeaderIP holds h.m and sends. If the Run consumer is mid-warmup (up to 30s) during a back-to-back transition, the second send blocks under h.m → RejectOrForwardLeaderQuery/GetState/GetConfigs (all RLock h.m) stall → data plane wedged by control plane; leaderWatch stops draining its liveness probe → restart. Flap leadership near the lease boundary while Run is busy in warmup. Surfaced by 3 focus agents.


## Why it matters

A wedged handler stalls all node-agent cluster-check traffic and self-restarts the pod, cascading into more leadership churn — a self-reinforcing outage under exactly the flapping Antithesis induces.


## Mechanism refinement (from open-question investigation)

Scope narrowing (not invalidation): the property's central trigger premise is weaker than stated. (1) Run drains leadershipChan during warmup (handler.go:133), so the vulnerable no-read window is sub-second code stretches, not 30s. (2) Only real leader<->follower flips send (handler.go:257-277); self-transitions never send. (3) Handler polls GetLeaderIP, so notify() coalescing is irrelevant. Net: trigger probability much lower and requires two real flips landing in a narrow window (fault-dependent). A residual hazard window still exists (:143-152), so the invariant stands but is harder to violate.


## Fault dependencies

- network partition leader<->apiserver near lease boundary to flap leadership (enabled by default)
- clock jitter past renew (DISABLED by default)
- requires leader_election enabled + >=2 replicas


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. SUT-side: assert non-blocking send (select with default, or measure send latency) under h.m; `assert.Always`. Also a `Reachable` marker on the second-transition-during-warmup branch.


## Open questions (post-investigation)

- Refined: Run DOES read leadershipChan during warmup (handler.go:133), and buffer is drained on entry at :111, so the only windows where Run is not selecting on the channel are the brief non-select code stretches (:116-128 and :143-152, sub-second) — not the full 30s warmup. Open: measure whether two sends can land in that narrow window. `(partial)`
- updateLeaderIP sends only on a real leader<->follower transition (handler.go:257-277); self-transitions produce NO send. So a wedge needs TWO real flips (lose then regain) inside a sub-second no-read window at 60s lease / 30s RenewDeadline / 15s RetryPeriod — realistic only under clock-skew/partition fault; needs flap frequency measured under fault. `(needs human input)`


### Investigation Log

#### Whether leadershipChan's single buffer is always drained by Run before the next tick (1s poll vs 30s warmup).

Examined handler.go:106-176. Found: entry select :108-116 consumes the leader state (buffer=0 entering warmup); warmup select :129-141 ALSO reads leadershipChan at :133; steady select :158-164 reads it. Conclusion: the discovery premise 'consumer parked in time.After not draining for 30s' is INACCURATE — Run drains during warmup. True no-read window = brief code between selects (:116-128, :143-152). PARTIAL (refined).

#### Whether the observation assertion can be added without the select refactor that removes the hang.

Examined handler.go:246-277. Found: bare send at :276 under h.m held :246. Conclusion: YES — capture lockAcquired at :246, measure time.Since after :276 and assert.Always(elapsed<T); this is pure observation and leaves the blocking send intact, unlike a select-with-default which would itself remove the hang. RESOLVED.

#### Interaction with notify() coalescing dropping the intermediate loss edge.

Examined handler.go:240 (leaderStatusCallback), handler_test.go:59 (le.get), leaderelection_engine.go:227-239 (notify) and leaderelection.go:339 (Subscribe). Found: the clusterchecks Handler polls GetLeaderIP via leaderStatusCallback every 1s; it does NOT use Subscribe()/notify(). Conclusion: notify() coalescing only affects Subscribe consumers, not this handler's leadershipChan; irrelevant. The relevant coalescing is 1s poll sampling (two flips within one poll are invisible to the handler). RESOLVED.

#### Whether client-go can emit two self-transitions within one warmup window at 60s lease.

Examined handler.go:257-277 and leaderelection_engine.go:200-202 (LeaseDuration 60s default, RenewDeadline=LD/2=30s, RetryPeriod=LD/4=15s; leaderelection.go:46,90). Found: the state switch only sets newState (and sends) on an actual leader<->follower change; staying-leader/staying-follower yields newState=unknown -> no send. Conclusion: self-transitions are a red herring — the wedge requires two REAL transitions; feasibility at 60s lease is fault-dependent. NEEDS-HUMAN (refined).

#### Whether any h.m reader is on the node-agent hot path so the stall is externally observable.

Examined handler_api.go:22-24 (RejectOrForwardLeaderQuery takes h.m.RLock) and cmd/cluster-agent/api/v1/clusterchecks.go:44,76,98,127 + endpointschecks.go:30. Found: RejectOrForwardLeaderQuery runs on EVERY node-agent cluster-check request (GET /configs, POST /status, endpoints). Conclusion: a writer stalled on h.m blocks all these RLocks -> externally observable as node-agent request timeouts. RESOLVED.


---

## Source discovery evidence (raw, per contributing agent)


### from `leadership-transition-does-not-wedge-dispatcher`

## Mechanism (verified against source)

`updateLeaderIP()` (`handler.go:239-280`):

```go
h.m.Lock()
defer h.m.Unlock()
... compute newState ...
if newState != unknown {
    h.state = newState
    h.leadershipChan <- newState   // buffered(1) send WHILE holding h.m
}
```

`leadershipChan` is buffered(1). The consumer is the Handler's `Run` loop, which during warmup (`warmup_duration` default 30s, dispatcher active=false) may not be draining promptly. Two transitions within one buffer slot → second send blocks → `h.m` held indefinitely.

`leaderWatch` (`handler.go:197-234`) selects on `healthProbe.C` (the liveness probe) and the watch ticker; its ticker branch also takes `h.m.Lock()` (handler.go:215). If h.m is held by a blocked updateLeaderIP, leaderWatch cannot process and the `clusterchecks-leadership` liveness probe (registered handler.go:203) is not served → pod restart.

Self-acknowledgement:
```go
// handler.go:211-212
case <-healthProbe.C:
    // This goroutine might hang if the leader election engine blocks
```
```go
// dispatcher_main.go:398-399
case <-healthProbe.C:
    // This goroutine might hang if the store is deadlocked during a cleanup
```

## Failure scenario

1. Replica gains leadership → updateLeaderIP sends `leader` (buffer now full, 1 item). Run consumer is in 30s warmup, hasn't drained.
2. Rapid flap: partition heals/re-breaks; within warmup the replica loses then regains, updateLeaderIP tries to send a second state → blocks on full buffer while holding h.m.
3. A node-agent POST /status arrives → processNodeStatus / RejectOrForwardLeaderQuery takes h.m → blocks. Data plane wedged.
4. leaderWatch ticker branch blocks on h.m → liveness probe unserved → ~30s later pod restart → new leadership churn. VIOLATION (data plane stalled / probe starved).

## Where to assert (SUT instrumentation — MISSING)

- Wrap the `h.leadershipChan <- newState` at handler.go:276 in a `select { case h.leadershipChan <- newState: case <-time.After(bound): assert.Unreachable("leadershipChan send blocked under handler lock", details) }` (bound << warmup_duration). Note: changing to a select also *fixes* the bug, so for pure observation, instead record lock-acquire time and assert.Always(hold < bound) at Unlock.
- Alternatively assert.Always that the clusterchecks-leadership and clusterchecks-dispatch health probes are drained within their tick interval (requires exposing probe-served timestamps).

## Coverage gap

No unit test drives back-to-back transitions against a live Run consumer mid-warmup; transitions are simulated via ctx.cancel (sut-analysis §9). notify() coalescing (buffered(1), skip-if-full, leaderelection_engine.go:227-239) compounds this by dropping intermediate edges, changing which flaps reach the handler.


### from `leadership-chan-buffered1-send-under-lock`

## Property
A leadership-state transition send on `leadershipChan` (capacity 1) must never block while `h.m` is held, so the single lock that serializes all data-plane reads is never pinned by control-plane backpressure.

## Where (code paths)
- Channel capacity: `pkg/clusteragent/clusterchecks/handler.go:73` `leadershipChan: make(chan state, 1)` (buffer 1).
- Blocking send under lock: `handler.go:239-280` `updateLeaderIP`: `h.m.Lock()` at :246 (deferred unlock :247), state computed :257-272, then `h.leadershipChan <- newState` at :276 **inside the locked region**.
- Consumer: `handler.go:106-176` `Run`. After receiving `leader` it enters the warmup select (:129-141) blocking on `time.After(h.warmupDuration)` (:138). During warmup it can receive at most one more state; a second transition arriving while the buffer already holds one fills the buffer and blocks the sender.
- The same `h.m` guards data-plane methods (RejectOrForwardLeaderQuery / GetState / GetConfigs and updateLeaderIP itself), so a sender blocked under `h.m` stalls all of them.
- Poll cadence: `leaderWatch` ticks every `leaderStatusFreq = 1s` (handler.go:71, :206) calling `updateLeaderIP`; warmup default is 30s.

## Failure scenario
1. DCA transitions follower->leader; `updateLeaderIP` sends `leader` (buffer now full or just consumed), Run enters 30s warmup select.
2. Antithesis flaps leadership: apiserver partition causes lease loss then re-acquire within the warmup window, producing follower then leader states on successive 1s polls.
3. First state fills the buffer (Run is parked in time.After). The next `updateLeaderIP` poll computes a new state and blocks at :276 on the full channel **while holding h.m**.
4. All readers needing `h.m` block; `leaderWatch` cannot return to drain `healthProbe.C` (:211) -> clusterchecks-leadership liveness probe fails -> pod restart -> more churn.

## Assertion (net-new)
Replace the bare send with a bounded send under instrumentation: capture `t0` before `h.leadershipChan <- newState`; assert `assert.Always(time.Since(t0) < T)` (or use a select with a timeout channel and assert the timeout branch is Unreachable). Alternatively assert Always that `updateLeaderIP` total lock-hold time < T.

## Key observations
- notify() on the engine side uses buffered(1) too and *skips* full subscribers (leaderelection_engine.go:227-239); the Handler instead avoids drops by polling but pays with a blocking send under the lock.
- The hazard is a capacity/backpressure mismatch: sustained transition rate (up to 1/s) vs a 30s consumer stall vs a 1-slot buffer.

## Timing window
Window = warmup_duration (default 30s) during which the consumer cannot accept a second state; any two additional transitions in that window trigger the block.


### from `leadershipchan-send-under-handler-mutex`

## Claim
`updateLeaderIP` sends the new leadership state on a **buffered(1)** channel **while holding `h.m.Lock()`**; if the consumer is not draining, the second of two rapid transitions blocks the send under the lock, stalling all `h.m` users and the leadership liveness probe.

## Code path (verified)
`pkg/clusteragent/clusterchecks/handler.go`
```go
leadershipChan: make(chan state, 1),   // :73  buffer size 1
...
func (h *Handler) updateLeaderIP() error {
    newIP, err := h.leaderStatusCallback()
    ...
    h.m.Lock()          // :246
    defer h.m.Unlock()  // :247
    ...
    if newState != unknown {
        h.state = newState
        h.leadershipChan <- newState   // :276  send UNDER h.m
    }
    return nil
}
```
Called every `leaderStatusFreq` = 1s from `leaderWatch` (`:213-214`).

## Why the send can block
`leadershipChan` capacity is 1. The consumer is the `Run` goroutine, which reads it only at three select points: `:111` (follower wait), `:133` (during warmup), `:162` (steady leader). Between consuming a `leader` state at `:111` and reaching the next read, `Run` executes warmup (`time.After(h.warmupDuration)`, up to 30s at `:138`), `logWarmupSummary`, `UpdateAdvancedDispatchingMode`, and launches `go h.runDispatch` (`:152`). During that stretch it is **not** reading `leadershipChan`. Two transitions within that window: first send fills the buffer, second send blocks -> `updateLeaderIP` holds `h.m` indefinitely.

## Contended lock users (block on the stalled h.m)
Every method that locks `h.m` for reads/writes - `updateLeaderIP` itself on the next tick, and the API handlers reading `h.state`/`h.leaderIP` (RejectOrForwardLeaderQuery, GetState/GetConfigs paths). `leaderWatch`'s own loop (`:209-233`) can no longer reach `case <-healthProbe.C:` (`:211`) because it is blocked inside `updateLeaderIP` -> `clusterchecks-leadership` liveness probe stops being answered.

## Self-documented risk
`handler.go:211-212` ("This goroutine might hang if the leader election engine blocks") and `dispatcher_main.go:398-399` acknowledge hang exposure.

## Suggested SUT instrumentation (MISSING - net new)
At `handler.go:246`, capture `lockAcquired := time.Now()`; wrap the send in a select with a short timeout or record `time.Since(lockAcquired)` after the send and `assert.Always(elapsed < h.leaderStatusFreq, "leadershipChan send under h.m is non-blocking", ...)`. Add `assert.Reachable` at the point the buffer is observed full to confirm the fault reaches the contended state.
