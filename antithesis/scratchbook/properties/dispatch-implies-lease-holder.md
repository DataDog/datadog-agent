# dispatch-implies-lease-holder — Active cluster-check dispatch implies this replica holds the lease

**Type:** Safety · **Assertion:** `Always` · **Priority:** P0 · **Intent:** invariant

**Provenance:** merged from 2 discovery agent(s): clusterchecks-dispatch-implies-lease-leader, clusterchecks-dispatch-only-while-lease-held

## Property

A replica whose clusterchecks Handler is actively dispatching (state==leader, store active) must simultaneously be the Kubernetes lease holder, so at most one replica dispatches cluster checks at any instant.


## Invariant / assertion

`assert.Always`: whenever the clusterchecks Handler state==leader and dispatcher.run is live (store.active true after warmup), `LeaderEngine.IsLeader()` (lease-derived) must be true for the same process. Cross-replica corollary asserted from the workload: at most one replica reports clusterchecks state==leader at a time. Always fits — the guarantee must hold on every evaluation; any divergence is the split-brain bug.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: the **ex-leader** observed GetLeaderIP()=="" while its clusterchecks state was still `leader` (lease-less dispatch after OnStoppedLeading) — NOT a follower self-promoting. Investigation confirmed client-go keeps the Lease holderIdentity until reacquired, so a follower retains the old leader's name; the reachable split-brain is the ex-leader continuing to dispatch until it observes a different non-empty leader IP post-heal.


## Antithesis angle

The clusterchecks Handler derives its role ONLY from whether `GetLeaderIP()` returns "" (handler.go:257-272, verified). `GetLeaderIP()` returns ("",nil) for TWO opposite conditions (leaderelection.go:262-266): "I am the leader" AND "no leader observed / leaderIdentity is empty." `OnStoppedLeading` clears leaderIdentity to "". So a follower that observes "" during a leaderless gap self-promotes and dispatches while the lease is held elsewhere. Inject an asymmetric partition leader<->apiserver for >= LeaseDuration (60s) so the old leader loses the lease while a follower acquires it; assert no two replicas dispatch, and in-process state agrees with the lease. Investigation refinement: the violating replica is the EX-LEADER (state==leader, store active, GetLeaderIP()=="", IsLeader()==false after OnStoppedLeading), which never re-enters warmup and keeps dispatching until it observes the new leader's IP (handler.go:258-260 never leaves state==leader on newIP==""). Evaluate the assertion on every replica; expect the failure on the ex-leader.


## Why it matters

This is the load-bearing 'exactly one leader' guarantee behind every leader-gated behavior. Two dispatchers → the same check scheduled from two control planes → duplicate metrics, conflicting node assignments cluster-wide. The single most important property in the catalog; surfaced independently by 4 focus agents.


## Mechanism refinement (from open-question investigation)

Mechanism refinement (invariant still valid, does not invalidate): the reachable split-brain is the EX-LEADER continuing to dispatch (state==leader, store active, newIP=="", IsLeader()==false after OnStoppedLeading), NOT a follower self-promoting on "". Evidence: handler.go:258-260 never leaves state==leader on newIP==""; leaderelection_engine.go:164-169 clears identity only on the ex-leader; client-go keeps the Lease holderIdentity until reacquired so followers retain the old leader's name. The assertion (!dispatching || IsLeader()) should be evaluated on every replica, but the violation fires on the ex-leader; the paired R1 witness 'follower observed GetLeaderIP()==""' should be reframed to 'ex-leader observed GetLeaderIP()=="" while state==leader'.


## Fault dependencies

- network partition leader<->apiserver >= leader_lease_duration (asymmetric; enabled by default)
- clock skew past renew deadline (DISABLED by default — amplifies the window)
- requires leader_election enabled + >=2 replicas


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING (zero SDK usage). Add `assert.Always` at the point where dispatcher.run transitions store.active=true, checking `IsLeader()`. Also a workload-side cross-replica check polling each replica's clusterchecks status. SUT-side instrumentation on `updateLeaderIP` state transitions gives Antithesis a replay anchor for the self-promotion branch.


## Open questions (post-investigation)

- The 60s-partition window magnitude is measured under fault; note warmup_duration does NOT protect the primary hazard (the ex-leader keeps dispatching, already past warmup), so warmup masks only newly-promoted replicas, not this path. `(partial)`


### Investigation Log

#### Q1/Q3: Can client-go surface an empty holderIdentity to a FOLLOWER during the lease gap (self-promotion), or does the follower retain the outgoing leader's name?

Examined leaderelection_engine.go:150-170 (callbacks) and leaderelection.go:250-266 (GetLeader/GetLeaderIP). Found: a follower's leaderIdentity is set ONLY via OnNewLeader(identity), which client-go invokes with the Lease record's spec.holderIdentity. client-go does not clear holderIdentity on lease expiry — it is overwritten only when a new candidate CASes the lease. OnStoppedLeading (which sets leaderIdentity="") fires ONLY on the process that WAS leading. Concluded: a follower retains the outgoing leader's non-empty name during the gap; GetLeaderIP() returns that leader's (possibly stale) IP, NOT "". A follower does NOT self-promote on "" (that empty-observation only happens at startup zero-value or on the ex-leader). RESOLVED.

#### Q2: Does warmup_duration (30s) mask short flaps enough that dispatching windows rarely overlap?

Examined handler.go:95-176 (Run) and 118-141 (warmup gate). Found: warmup applies only to a replica transitioning INTO state==leader before it starts runDispatch. The dominant split-brain hazard is the EX-leader that is already dispatching (past warmup) and never re-enters warmup. Concluded: warmup provides no protection for the ex-leader over-dispatch path; the 60s-partition overlap magnitude is a runtime timing question left to fault measurement. PARTIAL.

#### Q4: Does the old leader's dispatcher reliably observe loss and reset() before a healed partition lets both dispatch?

Examined handler.go:236-280 (updateLeaderIP state machine), 155-195 (Run loss handling + runDispatch reset), leaderelection.go:262-266. Found: the state machine transitions leader→follower ONLY when newIP != "" (handler.go:258-260). After OnStoppedLeading clears the ex-leader's identity to "", GetLeaderIP() returns ("",nil) locally (no network), so newIP=="" and NO transition occurs — the ex-leader stays state==leader and keeps dispatching. reset() (handler.go:194) runs only after dispatchCancel(), which fires only on receiving newState=follower, i.e. only once the ex-leader observes a DIFFERENT non-empty leader IP (post-heal). Concluded: the ex-leader does NOT reset on loss-of-lease; it dispatches lease-less until it observes the new leader's IP. RESOLVED.


---

## Source discovery evidence (raw, per contributing agent)


### from `clusterchecks-dispatch-implies-lease-leader`

## Property
Only the lease holder may dispatch cluster checks; the clusterchecks state machine must not diverge from the lease.

## The three independent notions (analysis §4, verified against source)
1. Lease: `LeaderEngine.IsLeader()` = `GetLeader()==HolderIdentity` (`leaderelection.go:328`), driven by client-go callbacks.
2. Service-endpoint IP resolution: `GetLeaderIP()` resolves leader pod name -> IP via Endpoints/EndpointSlices, cached 5 min (`leaderelection.go:262-325`).
3. The `""`-means-leader heuristic in the clusterchecks Handler (`handler.go:239-280`).

## The overload trap (verified)
`GetLeaderIP()` returns `("", nil)` both when this pod IS the leader and when NO leader has been observed (`leaderIdentity==""`, set by `OnStoppedLeading` at `leaderelection_engine.go:164-169`, and its zero value at startup).

`updateLeaderIP` (`handler.go:239-280`) then does:
```
case follower: if newIP == "" { newState = leader }
case unknown:  if newIP == "" { newState = leader }
case leader:   if newIP != "" { newState = follower }
```
A follower observing the empty window flips to `leader`, sends on `leadershipChan`, and `Run` (`handler.go:105-176`) starts `runDispatch` -> `dispatcher.run` after warmup.

## Failure scenario
1. Replica L holds the lease and dispatches. Replica F is follower, resolving L's IP.
2. Partition L <-> apiserver for >= `leader_lease_duration` (60s). client-go on L fires `OnStoppedLeading`; the lease will eventually be acquirable by F, but there is a leaderless gap.
3. During the gap F's 1s `leaderStatus` poll calls `GetLeaderIP()` -> `("",nil)` (no leader observed). F flips `state=leader` and, after warmup, begins dispatching.
4. If the partition heals such that L still believes it leads (or L reacquires slowly), both dispatch. Even absent that, F now dispatches while the lease record is stale.
5. Assertion on F: `state==leader && store.active` but `IsLeader()==false` -> VIOLATION.

## Assertion point (MISSING — net-new)
In `dispatcher.run` per tick, and at the moment `store.active` is set true post-warmup: `assert.Always(!dispatching || leaderEngine.IsLeader(), "clusterchecks dispatch implies lease ownership", {holderIdentity, getLeaderIP, state})`. Optionally a cross-replica Sometimes(count_of_dispatchers>1) via a shared marker to confirm the harness reaches split-brain.

## Existing coverage gap
Leader-election unit tests defer transition testing to E2E and simulate loss via `ctx.cancel()`, not lease expiry (`leaderelection_test.go:90-92`); `fake.NewSimpleClientset` has no lease-expiry/renew clock (analysis §9). The E2E failover path passes `restartLeader=false` (dead code). This divergence is never tested.


### from `clusterchecks-dispatch-only-while-lease-held`

## Mechanism (verified against source)

Three independent notions of leadership diverge under fault:

1. **Lease** — `LeaderEngine.IsLeader()` = `GetLeader() == HolderIdentity` (`leaderelection.go:327-330`). Drives the generic `LeaderProxyHandler` (`leader_handler.go:108`) and every leader-gated controller.
2. **`""`-means-leader heuristic** — the clusterchecks `Handler` derives its own leader/follower state *purely* from whether `GetLeaderIP()` returns `""`:

```go
// handler.go:257-272 (VERIFIED)
switch h.state {
case leader:
    if newIP != "" { newState = follower }
case follower:
    if newIP == "" { newState = leader }   // <-- self-promotion
case unknown:
    if newIP == "" { newState = leader } else { newState = follower }
}
```

3. **The overload (VERIFIED, `leaderelection.go:262-266`):**

```go
func (le *LeaderEngine) GetLeaderIP() (string, error) {
    leaderName := le.GetLeader()
    if leaderName == "" || leaderName == le.HolderIdentity {
        return "", nil   // "" means BOTH "I'm leader" AND "no leader observed"
    }
    ...
}
```

`OnStoppedLeading` sets `leaderIdentity=""` (`leaderelection_engine.go:164-169`), and it starts empty. So a follower that momentarily sees `GetLeader()==""` reads `newIP==""` and promotes itself.

## Failure scenario

1. Replica A holds the lease, dispatching. Replica B is follower (state==follower, sees A's IP).
2. Partition A from the apiserver. A cannot renew; after RenewDeadline (30s) client-go calls A's `OnStoppedLeading` → A's `leaderIdentity=""`. The lease will expire after LeaseDuration (60s), after which B can acquire.
3. During the gap (and depending on B's observed `GetLeader()`), B's 1s `leaderWatch` poll (`handler.go:206`, `leaderStatusFreq`) can observe `GetLeaderIP()==""` and execute `case follower → newState=leader`, starting `dispatcher.run`.
4. If A's partition heals or A's dispatcher goroutine is still live before it observes loss, both A and B dispatch. VIOLATION.

## Where to assert (SUT instrumentation — MISSING, net-new)

- Primary in-process point: end of `Handler.updateLeaderIP()` (`handler.go:274-277`) after `h.state` is set, and at the top of each `dispatcher.run` loop iteration (`dispatcher_main.go:394`). Assert `assert.Always(h.state != leader || h.leaderEngine.IsLeader(), "clusterchecks dispatch requires lease", details)`.
- `leaderEngine.IsLeader()` must be callable from the handler; the handler already holds `leaderStatusCallback`/engine reference.
- Global framing: because client-go's Lease guarantees at-most-one holder (itself under test via lease-expiry timing, which fake clientsets never exercise — sut-analysis §9), this per-process invariant on every replica is sufficient to catch global split-brain dispatch: a violation fires on the non-holder that is dispatching.

## Existing coverage gap

Leader-election unit tests simulate loss via `ctx.cancel()`, not lease expiry (`leaderelection_test.go:90-92`), and defer transition testing to E2E. The E2E `testDCALeaderElection(restartLeader)` only ever passes `false` (sut-analysis §9) — the failover path is dead code in the suite. This invariant is entirely unexercised.
