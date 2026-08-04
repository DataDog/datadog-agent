# new-leader-elected-after-loss — A new leader is eventually elected after the current one is lost

**Type:** Liveness · **Assertion:** `Sometimes` · **Priority:** P1 · **Intent:** invariant

**Provenance:** merged from 1 discovery agent(s): new-leader-elected-after-loss

## Property

After the replica holding the lease loses it (partition, termination, or graceful step-down), some surviving replica acquires the lease and resumes leader-only work within a bounded time.


## Invariant / assertion

`assert.Sometimes(a_new_distinct_leader_acquired_after_loss)`: during a quiet period (ANTITHESIS_STOP_FAULTS) following a forced leadership loss, a replica different from the previous holder becomes `IsLeader()==true` and its dispatcher becomes active. Sometimes is correct — it is a progress/liveness condition that must become true at least once per run, verified under a recovery window, not on every evaluation.


## Antithesis angle

client-go renews at RenewDeadline=LeaseDuration/2 (30s) and re-acquires; runLeaderElection loops re-running Run after loss (leaderelection.go:236-248). Kill or partition the current leader, pause faults, and assert a new leader appears within ~LeaseDuration. With graceful shutdown, ReleaseOnCancel shortens the lease to 1s for fast failover.


## Why it matters

If no replica takes over, all leader-only work halts cluster-wide: dispatch stops, HPA metrics stop refreshing, controllers stall. The core availability guarantee of a leader-elected singleton.


## Fault dependencies

- network partition leader<->apiserver >= LeaseDuration (works with defaults)
- node termination (DISABLED by default — needed for the crash-failover variant)
- requires leader_election enabled + >=2 replicas


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.Sometimes` in a workload liveness command run under a quiet period. SUT-side `Reachable` markers on OnStartedLeading would confirm the acquisition path is hit.


## Open questions (post-investigation)

- Code gives the acquire cadence (LeaseDuration 60s, RenewDeadline 30s, RetryPeriod 15s, leaderelection_engine.go:200-202); a hard recovery SLA still needs measurement under injected latency, so keep Sometimes rather than a deadline assertion. `(partial)`
- ReleaseOnCancel performs a blocking Lease Update network call on shutdown (leaderelection_engine.go:196-198); under partition it blocks until the k8s client/dial timeout (not unbounded), delaying handoff — exact bound is measured under fault. `(partial)`


### Investigation Log

#### Q1: Can recovery time be bounded as a hard SLA?

Examined leaderelection_engine.go:195-204. Found: LeaseDuration=leader_lease_duration (default 60s), RenewDeadline=LeaseDuration/2, RetryPeriod=LeaseDuration/4. Concluded: static cadence is derivable (~LeaseDuration + RetryPeriod after loss under no added latency), but converting to a hard deadline assertion requires confirming client-go acquire behavior under injected apiserver latency — measurement-under-fault. PARTIAL (stays Sometimes).

#### Q2: Can ReleaseOnCancel's shutdown network call hang under partition and delay handoff?

Examined leaderelection_engine.go:196-205 (ReleaseOnCancel gated by leader_election_release_on_shutdown, sets lease to 1s via a network call on ctx cancel). Found: client-go's release is a synchronous Lock.Update; under partition it blocks until the k8s client's dial/TLS timeout rather than hanging forever. Concluded: it CAN delay graceful handoff by the client timeout window; precise bound needs fault measurement. PARTIAL.


---

## Source discovery evidence (raw, per contributing agent)


### from `new-leader-elected-after-loss`

## Mechanism (verified)

- client-go `leaderelection.LeaderElector` on a `coordination.k8s.io` Lease; timings derived in `leaderelection_engine.go:195-204`: LeaseDuration=`leader_lease_duration` (default 60s), RenewDeadline=LeaseDuration/2 (30s), RetryPeriod=LeaseDuration/4 (15s).
- Callbacks (`leaderelection_engine.go:150-170`): OnStartedLeading → updateLeaderIdentity(self)+notify; OnStoppedLeading → updateLeaderIdentity("")+notify; OnNewLeader → updateLeaderIdentity(identity). Note OnNewLeader does NOT notify subscribers (sut-analysis §4) — subscribers learn only self-transitions.
- `ReleaseOnCancel` (`leader_election_release_on_shutdown`) shortens the lease to 1s on graceful shutdown for faster handoff (a network call on shutdown).

## Failure scenario the property guards against

After the leader is partitioned: it stops renewing, client-go fires its OnStoppedLeading at RenewDeadline; the lease object expires at LeaseDuration; a follower's LeaderElector acquire loop (RetryPeriod cadence) should CAS the lease and fire OnStartedLeading. The property confirms this actually happens under fault. Non-occurrence (Sometimes never satisfied in partition timelines) would indicate a wedged acquire loop or a lease left un-acquirable.

## Where to assert (SUT instrumentation — MISSING)

- Assert at `OnStartedLeading` (`leaderelection_engine.go:156-161`): a `assert.Sometimes(...)` for 're-election after loss', with run-level context distinguishing initial election from post-loss re-election (e.g., a package-level flag set when any OnStoppedLeading fired).
- Complement with a workload-side check that during the partition window exactly the expected replica set is eligible.
- Optionally a `Reachable` at the clusterchecks Handler reaching state==leader for the newly-elected replica to confirm the data plane follows the control plane.

## Coverage gap

The real failover path is dead code in the E2E suite (only caller passes false). No test exercises lease expiry under a real renew clock.
