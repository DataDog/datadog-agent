# leader-eventually-dispatches-after-warmup — A stable leader eventually dispatches after warmup

**Type:** Liveness · **Assertion:** `Sometimes` · **Priority:** P0 · **Intent:** invariant

**Provenance:** merged from 3 discovery agent(s): warmup-flap-dispatch-livelock, clusterchecks-leader-eventually-dispatches-after-warmup, warmup-reentry-starvation-under-flapping

## Property

Whenever a replica holds cluster-check leadership continuously for at least warmup_duration, the dispatcher becomes active (store.active=true) and begins dispatching; leadership churn shorter than warmup must not starve dispatch forever.


## Invariant / assertion

`assert.Sometimes(dispatcher_became_active_and_dispatched)`: across the run, store.active flips true and at least one config is dispatched after a stable-leadership interval >= warmup_duration. Sometimes fits — a progress condition that must be reached at least once; the failure mode is that under flapping it is NEVER reached.


## Antithesis angle

On every leadership acquisition the store is reset (active=false) and a warmup timer runs BEFORE dispatch (handler.go:118-141). During warmup, processNodeStatus tells all nodes 'up to date' without dispatching. A partition-induced flap at ~warmup period re-enters warmup each cycle → dispatch never starts (livelock/starvation). Flap leadership at ~warmup_duration and assert dispatch eventually occurs during a stable window.


## Why it matters

If dispatch never starts, cluster checks silently stop running cluster-wide while nodes believe they are current — the worst kind of outage (no error, no alert). Surfaced by 3 focus agents.


## Mechanism refinement (from open-question investigation)

Scope refinement (property remains VALID, not invalidated). Discovery agents assumed a partition blip makes GetLeaderIP resolve non-empty -> follower -> warmup abort. Primary evidence (handler.go:257-261 sends `follower` only when newIP!=''; leaderelection.go:262-266 + engine.go:164-165 return '' on OnStoppedLeading/self-leader) shows warmup is aborted ONLY when a DIFFERENT replica is observed as leader (non-empty IP). A single-pod lease flap (lose/regain with no successor) keeps GetLeaderIP='' and does NOT abort warmup, so dispatch is not starved by self-flap. The Sometimes(dispatcher_became_active) assertion stands, but the antagonist workload must (a) run >=2 replicas that actually alternate lease ownership, and (b) lower leader_lease_duration (default 60s bounds successor acquisition to ~lease-expiry >> the 30s warmup), otherwise sub-warmup starvation is unreachable.


## Fault dependencies

- network partition leader<->apiserver to induce flapping (enabled by default — sufficient)
- clock jitter (DISABLED by default — sharper trigger)
- requires leader_election enabled + >=2 replicas


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.Sometimes` on the store.active=true transition plus a first-dispatch marker; a `Reachable` on runDispatch entry helps Antithesis steer toward the stable-leadership branch.


## Open questions (post-investigation)

- Is warmup_duration ever set below RenewDeadline (leader_lease_duration/2) in real deployments? Code shows both are independently tunable and equal by default (30s == 30s); a field survey is needed to know actual deployment configs. `(needs human input)`


### Investigation Log

#### Q1: Can client-go deliver leader->non-leader transitions faster than 30s repeatedly under the default 60s lease, or does it require lowering lease_duration?

Examined handler.go:239-277 (updateLeaderIP), leaderelection_engine.go:150-205 (callbacks + RenewDeadline/LeaseDuration), common_settings.go:278 (leader_lease_duration default 60). Found: the handler observes leadership only via GetLeaderIP polling at 1s; a demotion (`follower` send) requires newIP!='' i.e. a *different* replica observed as leader (handler.go:257-261). Another replica cannot acquire the lease until it expires (~LeaseDuration=60s after last renew); our own OnStoppedLeading fires only after RenewDeadline=30s. Concluded: RESOLVED — sub-30s repeated leader->non-leader transitions are NOT achievable under the default 60s lease; the harness must lower leader_lease_duration to make lease churn sub-warmup. Confirms the question's hypothesis.

#### Q2: Does a leadership-lost during warmup leak any goroutine/scheduler across cycles?

Examined handler.go:129-176. Found: runDispatch (which calls AddScheduler and dispatcher.run) and dispatchCtx/dispatchCancel are created only AFTER warmup completes (:151-152). On warmup abort the code hits `continue` at :136 — no dispatch goroutine started, no scheduler registered, nothing to cancel; the warmup span is closed via finishWarmupSpan('leadership_lost') (:134). Only residue: the abandoned time.After(warmupDuration) timer from :138 lingers until it fires (<=30s) then is GC'd — bounded, not a cross-cycle goroutine/scheduler leak. Concluded: RESOLVED — no leak.

#### Q3: Does flap-window magnitude depend on whether a follower sees a non-empty leader IP during the gap (empty holderIdentity -> self-promote)?

Examined handler.go:257-277 and leaderelection.go:250-294 + engine.go:151-169. Found: warmup abort (the `follower` continue) fires ONLY when GetLeaderIP returns a non-empty IP, which happens only when GetLeader() returns a DIFFERENT identity (via OnNewLeader). OnStoppedLeading sets leaderIdentity='' -> GetLeaderIP='' -> updateLeaderIP sends NO transition (newState stays unknown). Concluded: RESOLVED and property-narrowing — a pure gap where our pod loses the lease with no successor keeps GetLeaderIP='' and does NOT abort warmup; the warmup timer keeps running. Starvation requires a DIFFERENT replica to be repeatedly observed as leader within sub-warmup windows. (Self-promotion is moot here: the pod is already the aspiring leader and simply stays leader.)

#### Q4: Does a duplicate `leader` send restart warmup?

Examined handler.go:129-141 and 257-277. Found: updateLeaderIP sends only on genuine state transitions (leader<->follower); while state==leader it never re-sends `leader` (newIP=='' -> unknown -> no send; newIP!='' -> follower). Even structurally, the :135 guard `if newState != leader { continue }` means a `leader` value would NOT continue — it falls through and cuts warmup SHORT to dispatch, never restarting it. Concluded: RESOLVED — only follower edges abort warmup; a duplicate leader neither restarts warmup nor can it be emitted by the normal path. Confirms scratchbook.

#### Q5: Does leadershipChan coalescing (buffered(1)) deliver a spurious non-leader during a fast gain->lose->gain, interrupting warmup even when the lease was effectively held?

Examined handler.go:73 (make(chan state,1)), :274-277 (send under h.m.Lock()), :197-234 (leaderWatch, sole sender at 1s). Found: sends are serialized under the lock by a single sender; buffered(1) provides FIFO with backpressure (a full buffer blocks the next send under lock) — no coalescing/dropping of states. Combined with Q3: a self gain->lose->gain yields GetLeaderIP='' throughout, so NO sends occur; a `follower` is emitted only upon observing a distinct non-empty leader IP. Concluded: RESOLVED — coalescing cannot manufacture a spurious non-leader; every delivered follower reflects a real observation of a different leader.

#### Q6: Is warmup_duration ever set below RenewDeadline in real deployments?

Examined common_settings.go:278 (leader_lease_duration=60) & :576 (warmup=30); engine.go:201 (RenewDeadline=LeaseDuration/2=30). Found: defaults make warmup==RenewDeadline (30s==30s); the two are independent knobs. Would fall below RenewDeadline only if leader_lease_duration is raised (>60 -> RenewDeadline>30) or warmup lowered (<30). Concluded: PARTIAL/needs-human — code gives defaults and the mechanism, but actual field-deployment values require a human survey.


---

## Source discovery evidence (raw, per contributing agent)


### from `warmup-flap-dispatch-livelock`

## Mechanism

Leader FSM in `pkg/clusteragent/clusterchecks/handler.go` (`Run`, verified at lines 106-177):

1. Outer loop waits for `leader` on `leadershipChan` (handler.go:111-116).
2. On becoming leader it enters the **warmup select** (handler.go:129-141):
   ```go
   select {
   case <-ctx.Done(): ...return
   case newState := <-h.leadershipChan:
       finishWarmupSpan("leadership_lost")
       if newState != leader { continue }   // <-- back to top, dispatch NEVER started
   case <-time.After(h.warmupDuration):     // <-- only path that reaches dispatch
       break
   }
   ```
3. Only the timer arm falls through to `runDispatch` (handler.go:143-152) → `dispatcher.run` sets `store.active=true` (dispatcher_main.go:378-380).

`warmupDuration` defaults to 30s (`cluster_checks.warmup_duration`, handler.go:72).

## Failure scenario

Leader is repeatedly interrupted before 30s elapses (partition/latency/clock jitter causing lease loss then re-acquire). Each cycle: replica becomes leader → enters warmup → receives a `follower`/`unknown` transition before the 30s timer → `continue`s. `runDispatch` is never called, so `store.active` stays false and no config is dispatched. Meanwhile `processNodeStatus` (dispatcher_nodes.go:73-79) returns `IsUpToDate=true` to every node during warmup, so node agents keep running their **cached** check set and nothing surfaces as unhealthy.

## Why existing tests miss it

Per sut-analysis §9, leader-election unit tests defer transition testing to E2E and simulate loss via `ctx.cancel()`, not real lease churn; the E2E `testDCALeaderElection` caller passes `restartLeader=false`. No test exercises repeated sub-warmup flapping.

## Assertion to add (MISSING — zero SDK instrumentation exists)

- `assert.Sometimes(true, "cluster-check dispatch loop entered", ...)` at the top of `runDispatch` (handler.go:180) or immediately after `store.active=true` (dispatcher_main.go:379). Under a workload that flaps leadership near the warmup boundary, a run where this never becomes true is the bug.
- Optionally a paired `assert.Reachable("warmup completed via timer")` on the `case <-time.After` arm to distinguish 'never led long enough' from 'led but never dispatched'.

## Open question

Magnitude depends on how tightly Antithesis can pin lease-loss deliveries inside the 30s window; a floor on warmup vs lease-duration would bound it.


### from `clusterchecks-leader-eventually-dispatches-after-warmup`

## Warmup masking (verified)
`processNodeStatus` (dispatcher_nodes.go:44-84): when `!store.active` it sets `warmingUp=true` and returns `true` (IsUpToDate) to every node (lines 48-50, 73-79), regardless of actual config version, 'to keep their cached configurations running while we finish the warmup phase.'

## Where active is set (the liveness target)
`dispatcher.run` sets `store.active=true` as its very first action (dispatcher_main.go:378-380). `run` is only invoked by `runDispatch` (handler.go:185), which is only reached after the warmup `select` in `Handler.Run` falls through on the `time.After(h.warmupDuration)` branch (handler.go:138-141).

## The interruption path (verified)
The warmup `select` (handler.go:129-141) has three cases: ctx.Done → return; `newState := <-h.leadershipChan` → `finishWarmupSpan("leadership_lost")`, and if `newState != leader` → `continue` (back to the outer follower loop, warmup discarded); `time.After(warmupDuration)` → break (proceed to dispatch). So any leadership-lost notification arriving within the 30s warmup window aborts the transition to active. `warmupDuration = cluster_checks.warmup_duration * time.Second` (handler.go:72).

## leadershipChan hazards (§4)
`leadershipChan` is buffered(1) (handler.go:73). `updateLeaderIP` sends state transitions under `h.m.Lock()` (handler.go:274-277). The polling `leaderWatch` runs at `leaderStatusFreq=1s` (handler.go:71,206). Rapid leader/follower oscillation feeds alternating states; the warmup loop restarts on each follower edge.

## Failure scenario (concrete)
1. Replica wins lease; `Run` enters warmup (30s countdown).
2. At t=20s, a partition blip makes `GetLeaderIP()` briefly resolve a non-empty IP → `updateLeaderIP` sends `follower` (handler.go:259-261) → warmup select takes the leadership_lost case → `continue`.
3. Partition clears; replica wins again; warmup restarts at 30s.
4. Blips recur every ~25s. `store.active` never becomes true. All node polls return IsUpToDate=true. Cluster checks are frozen indefinitely with no error.

## Instrumentation (MISSING — net-new)
- `assert.Sometimes(true, "clusterchecks dispatcher became active after leadership", ...)` at dispatcher_main.go:379 — proves the good path is reachable; its absence over a flap-heavy run is the alarm.
- `assert.Reachable("warmup aborted by leadership loss", ...)` at handler.go:135 (the `continue`) to confirm the antagonist path fired.
- Optionally a bounded-liveness check: track wall time since last `active=true` while `state==leader` and assert it stays under a threshold.

## Why existing tests miss it
§9: no real lease/renew clock (fake clientset), leadership loss simulated by ctx.cancel not lease expiry, transitions deferred to E2E that never kills the leader. The warmup-abort-under-flap interleaving is unreachable in current tests.


### from `warmup-reentry-starvation-under-flapping`

## Mechanism (verified)

`pkg/clusteragent/clusterchecks/handler.go:106-176` main leadership loop:

- Outer `for` waits for `leadershipChan == leader` (`:106-116`).
- On gaining leadership it logs "Becoming leader, waiting %s" and enters the **warmup select** (`:118-141`):
  ```
  select {
  case <-ctx.Done():            finishWarmupSpan("context_done"); return
  case newState := <-h.leadershipChan: finishWarmupSpan("leadership_lost"); if newState != leader { continue }
  case <-time.After(h.warmupDuration): finishWarmupSpan(""); break
  }
  ```
- Only after the `time.After` branch fires does control fall through to `logWarmupSummary` (`:145`), `UpdateAdvancedDispatchingMode` (`:148`), and `go h.runDispatch(dispatchCtx)` (`:152`). `runDispatch` → `dispatcher.run` sets `store.active = true` (`dispatcher_main.go:379`), which ends warmup and begins real dispatch.
- If leadership is lost during warmup (`:133` branch, `newState != leader`), `continue` returns to the top of the outer loop. **runDispatch is never called for that cycle.**

`warmup_duration` default = 30s (`pkg/config/setup/common_settings.go:576`, seconds). `RenewDeadline` = `LeaseDuration/2` = 30s by default (`leaderelection_engine.go:200-203`). The two are equal, so the margin between "lease renew must succeed" and "warmup completes" is zero.

## Failure scenario

1. Replica A gains leadership, begins 30s warmup.
2. At ~t=25s an apiserver partition (or forward clock jump past RenewDeadline) causes A to lose the lease → warmup interrupted (`handler.go:133`, "leadership_lost"), loop restarts.
3. Partition heals; A (or B) reacquires and starts a **fresh** 30s warmup.
4. Oscillation with period <30s ⇒ `runDispatch` (`handler.go:152`) is never reached on any replica ⇒ `store.active` stays false ⇒ zero cluster checks dispatched, indefinitely.
5. Node agents keep running cached checks (warmup returns IsUpToDate=true, dispatcher_nodes.go:73-79), so metrics look alive while scheduling is dead.

## Key observations

- Warmup has no carry-over/credit: an interrupted 29s warmup contributes nothing to the next attempt. This is the starvation lever.
- Equal defaults (warmup=30s, RenewDeadline=30s) make single-miss interruption easy.

## Timing window

Starvation persists for as long as the leadership flap period stays below warmup_duration (30s default).
