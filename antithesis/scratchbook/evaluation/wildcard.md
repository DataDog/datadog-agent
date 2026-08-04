---
sut_path: /Users/jon.rosario/go/src/github.com/DataDog/datadog-agent
commit: f2da1471bb748fb5108f89f36f7b83cab305ca79
updated: 2026-07-21
external_references: []
---

# Wildcard Evaluation — DCA Antithesis Property Catalog

Lens: the framing itself. What did `sut-analysis.md` miss that changes which properties
matter; what failure modes exist only under a *combination* of conditions no single property
constructs; which properties are mislabeled safety/liveness; which usage patterns the workload
cannot represent; and which high-value-but-infeasible properties have a feasible reformulation.
Not scored: per-property Antithesis fit, coverage-balance census, per-property implementability
(the other three lenses own those). I read the full catalog, all four scratchbook docs, and
spot-read `dispatch-store-bijection.md` to confirm the headline gap.

---

## Headline: the catalog's own #1 hazard has no property that can catch it

`sut-analysis.md` §8 lists its strongest Antithesis targets in order. After split-brain, the
FIRST is:

> "**Expiration ≠ death → duplicate check execution** (§5): partition one node agent (alive,
> still reaching Datadog) from the leader for >30s; its checks are re-dispatched elsewhere while
> it keeps running them → duplicate metrics. **No fencing token.**"

This is also the load-bearing product guarantee in §10 ("Each cluster check is dispatched to
**exactly one node**") and in the README. It is a *design-level* finding (no fencing token
exists), which is exactly what Antithesis timing faults are for.

**No property asserts it.** `dispatch-store-bijection` is the closest, and its own evidence file
(`dispatch-store-bijection.md:107-108, 172`) explicitly concedes it does NOT:

> "Store invariant (D on exactly one node) still holds *inside the store*, but the real-world
> duplicate is that N1 still executes D. ... Cross-replica duplicate dispatch ... is a related
> but distinct property requiring an external witness; **this property is the intra-leader
> consistency check.**"

So the catalog verifies the store's internal bookkeeping is self-consistent, then punts the
actual guarantee ("a given check runs on exactly one live node") to a property that was never
written. The gap is structural, not incidental: the store is *correct by design* to reassign an
expired node's configs — the store bijection holds precisely while two agents run the same check.
The bug lives in the gap between "store is consistent" and "reality is consistent," and that gap
is observable ONLY from the workload (which owns the simulated node agents and their heartbeat
timing). The topology already gives the workload exactly this power (`deployment-topology.md`:
"full control over heartbeat timing ... duplicate dispatch"). The missing property: *a config
handed to node N (which the workload keeps "running" per the cached-check contract) is never
simultaneously handed to a different live node without N first being told to drop it* — i.e., a
fencing/generation-token property. Its absence means the single most-emphasized correctness
hazard in the analysis is invisible to the portfolio.

This also exposes a framing flaw the SUT analysis half-sees but does not resolve: it lists "each
check on exactly one node" as a claimed guarantee (§10) AND documents that there is no fencing
mechanism to enforce it (§8), yet the catalog encodes only the enforceable-in-store shadow. The
right move is a workload-witnessed property (feasible today) — not to treat the real guarantee as
untestable.

---

## Catalog-wide framing problems

### A. Several properties describe kubelet-mediated outcomes the topology has no kubelet to produce

The topology (`deployment-topology.md`) is bare containers: etcd, kube-apiserver, dca-1/2,
workload. **There is no kubelet.** Multiple properties are framed around behaviors that only a
kubelet produces:

- `liveness-probe-no-restart-loop` — its entire *Why It Matters* is "DCA is needlessly killed and
  thrown into leadership churn" → a restart→election→churn feedback loop. Nothing in the topology
  runs the liveness probe or restarts a container on probe failure. The workload can poll the
  health endpoint, so the *observable* property collapses to "health endpoint stays green under
  transient latency" — a far narrower claim than the stated feedback-loop hazard.
- `autoscaling-fatal-startup-crashloop` — asserts "pod crash-loops (CrashLoopBackOff)." A bare
  container just exits; crash-loop backoff is kubelet behavior. Open question 3 in that property
  ("does OneShot error yield CrashLoopBackOff") is unanswerable in this topology.
- `store-lock-bounded-under-slow-clc` and `leadershipchan-no-wedge-under-lock` both cite "trips
  the liveness probe → pod restart → leadership churn" as the payoff. Same gap: the restart actor
  is absent.

This doesn't kill these properties (the internal hazard — wedged handler, stalled dispatch — is
real and observable), but their stated *value* and severity narratives assume an actor the
harness lacks. Either the topology needs something that runs probes and restarts containers on
failure (a shim), or the properties must be reframed around the directly-observable internal
symptom and drop the churn-cascade justification. As written, a reader over-credits their impact.

### B. No AlwaysOrUnreachable property is paired with a Reachable witness → silent vacuous passes

Nine properties use `AlwaysOrUnreachable`, several gated behind deep precondition stacks
(`ksm-shard-tracking-consistency` needs leader_election + ≥2 replicas + ksm_sharding_enabled +
advanced_dispatching + a *shardable KSM check present*; `forwarder-*` need active follower
forwarding; `dangling-redispatch-no-resurrect` needs an Unschedule to interleave a sub-ms
window). `AlwaysOrUnreachable` passes GREEN when the guarded path never executes. With this many
gates, the dominant failure mode is not a caught violation but a path that never runs, reported as
a clean pass. **Not one property in the catalog pairs its AlwaysOrUnreachable with a companion
`assert.Reachable` proving the guarded code actually executed in the run.** Without that, a
green board is indistinguishable from "we never reached the interesting state." This is a
methodological hole across the whole portfolio, not one property.

### C. ~7 of 27 properties assert a statically-KNOWN defect ("expected to FAIL — that is the point")

`kubeactions-at-most-once`, `extmetrics-configmap-no-lost-update`, `node-expiry-monotonic-clock`,
`getconfigs-distinguishes-unknown-node`, `lastconfigchange-monotonic-epoch`,
`admission-webhook-no-silent-nil-cert`, and `isexternalpath-classifier-consistency` are all
described as things the code is already read to violate ("today it returns 500," "does plain
Update() with no retry," "expected to expose the silent-nil path," "claim-to-improve"). These are
bug reports / feature requests wearing assertion clothing. Encoding a known-red invariant as a
perpetual property has two costs: (1) it can never regress-detect (it starts red), and (2) it
creates a permanently-failing board that masks *newly* red true-invariants (alert fatigue). The
portfolio mixes three different intents — true invariants we believe hold, aspirational
"should-improve" assertions, and known-defect reproducers — without distinguishing them. For the
known-defect set, the deliverable Antithesis uniquely provides is a *minimized reproducing trace*,
after which the assertion should be retired or flipped once fixed; it is not an ongoing property.
This distinction is absent and will confuse whoever reads the results.

### D. An unrecognized "premature-Serve() startup window" property family

The API server starts accepting connections at `command.go:368` *before* routes, tokens, and the
apiserver connection are ready (sut-analysis §2, §3). The catalog contains two members of this
family — `no-404-on-registered-cluster-check-routes` (routes not yet installed) and
`empty-token-never-authenticates` (token not yet loaded) — but treats them as unrelated one-offs.
It misses a third, which the SUT analysis hands to it directly (§6): the **concurrent-ConfigMap
create race**. dca-1 and dca-2 boot simultaneously (the exact topology), *before* any leader is
elected, and both run `GetOrCreateClusterID` / DCA-token read-then-create with **no
resourceVersion / conflict guard** (§6: "concurrent-create hazard ... first-boot create race").
Two replicas can create divergent cluster IDs or tokens, or one wins and the other proceeds on a
stale/empty value → node agents authenticate against one replica but are rejected by the other.
This needs **no special faults** — only concurrent startup, Antithesis's core competency — yet has
no property. Recognizing the family also suggests a cleaner joint framing: "once the listener
accepts a connection, every prerequisite that gates a correct response is either ready or the
response is an honest retryable 503 (never 404, never auth-against-empty, never a divergent
token)."

### E. Root-cause hazard `informer_client_timeout=0` (silent informer freeze) has no direct home

sut-analysis §7 flags prominently: with `informer_client_timeout=0`, a partition that drops a
watch *without RST* freezes the informer cache with **no error surfaced**. This single root cause
underlies at least three catalog properties as downstream symptoms (stale endpoints →
`forwarder-target-is-live-endpoint`; stale secret/cert → `admission-webhook-no-silent-nil-cert`;
stale leader-IP resolution). No property asserts the shared root behavior: *the DCA acts on
silently-frozen informer data and never detects staleness.* A partition-drops-watch fault (a
timing fault, in-sweet-spot) directly exercises it. As-is, the catalog tests three symptoms
independently and misses the cheaper, higher-leverage assertion at the source.

### F. Rebalance convergence/termination — SUT-surfaced, no property

sut-analysis §8 lists "**Rebalance non-convergence / infinite loop** — freshly-landed complex
algorithm (merged→reverted→reapplied; `continue`→`break` fix #52884) acting on stale busyness
when a runner is unreachable." Churny recent bug history on a scheduling loop is exactly the
signal that drove the (well-justified) `reset-restores-store-and-gauges` regression property — yet
rebalance has no convergence/termination property at all. `store-lock-bounded-under-slow-clc`
touches rebalance's *locking* but says nothing about whether rebalance *terminates* or *converges*
(doesn't oscillate configs between runners) when a CLC runner reports stale/zero busyness. A
liveness (rebalance terminates each cycle) + safety (a config isn't moved every cycle forever)
pair is missing on freshly-landed, fault-sensitive code.

---

## Safety-vs-liveness mislabels and assertion-shape inversions

- **`store-lock-bounded-under-slow-clc` (labeled Safety, `Always(lock_hold_duration < bound)`)** is
  really a *real-time / liveness* bound dressed as safety, and its chosen shape is Antithesis-
  hostile: Antithesis controls the scheduler, so a wall-clock "duration under the lock" is not a
  faithful proxy for production latency, and the bound `T` is arbitrary. The actual invariant the
  property text already names — "**no blocking I/O under the lock**" — IS a structural safety
  property observable without any timing (assert the lock is not held across the CLC-runner HTTP
  call). Reformulate to the structural predicate; drop the duration bound.

- **`liveness-probe-no-restart-loop` (labeled Liveness, `Always`)** is internally inconsistent: an
  `Always` assertion is a safety shape, and the invariant ("transient latency does not flip
  liveness unhealthy") is a safety predicate. Combined with finding A (no kubelet), the "Liveness"
  label and the restart-loop narrative both overreach.

- **`graceful-shutdown-releases-lease-bounded` (Liveness, `Sometimes(released within bound)`)** is
  an assertion-shape inversion: the *documented bug* is that `ReleaseOnCancel` can HANG under
  partition, delaying failover a full 60s. `Sometimes(good outcome)` witnesses only that the happy
  path exists — it can never fire on "sometimes it hangs." The interesting failure is undetectable
  by the chosen assertion. To catch the hang you need an `Always(shutdown completes within bound)`
  or a bounded-time liveness assertion, not a success-witness.

- **`extmetrics-backoff-cap-stays-serving` (labeled Reachability)** bundles a `Reachable` and a
  `Sometimes` — two different intents under one type label — and its own open question admits the
  1800s cap may not fit in one Antithesis timeline without shortening constants, so the `Reachable`
  may simply never confirm (a permanent un-witnessed "not reached," not a clean result).

---

## Usage patterns the workload cannot faithfully represent

- **Node-agent client semantics are the crux of several properties, not a mere severity caveat.**
  The catalog repeatedly files "node-agent retry/backoff is out of SUT scope" as a footnote about
  *severity*. But for `getconfigs-distinguishes-unknown-node` the *entire Antithesis value*
  depends on modeling client backoff: without a workload that actually backs off on a 500 vs a
  4xx, the property collapses to "DCA returns 500 for an unknown node" — a fixed-input fact a unit
  test already covers. Likewise the duplicate-execution headline gap depends entirely on the
  workload faithfully modeling "node keeps running cached checks while partitioned" (the README
  claim). The catalog never commits the workload to these behaviors, so whether these properties
  are Antithesis-worthy at all is left undetermined. This should be an explicit workload
  requirement, not a scope disclaimer.

- **Pod-IP reuse (the sharp edge of `forwarder-target-is-live-endpoint`)** — the severe variant
  ("stale IP reused by an unrelated live pod that reads the bearer token") needs a real IP handed
  from a dead pod to a new one. Static containers can't easily produce IP reuse; the workload can
  lag the EndpointSlice (feasible) to show "forwards to a stale/dead IP," but not "delivered to a
  wrong *live* pod." The property's most alarming justification is not reproducible in-topology.

---

## Cross-cut: high-value-but-infeasible → feasible reformulations

- **Node-termination-gated properties** (`new-leader-elected-after-loss` crash variant,
  `kubeactions-at-most-once` crash-replay, `forwarder-target-is-live-endpoint`) are marked inert
  because "node termination is DISABLED by default." Worth surfacing an ambiguity the catalog
  glosses: Antithesis's *node-level* termination fault (its infra) is distinct from simply
  stopping/restarting a SUT **container** in a compose harness, which is generally available. If
  the crash paths are reachable via container restart, three P1-ish properties de-risk
  substantially. The catalog conflates the two under one "disabled by default" label and never
  checks whether container-restart suffices. (Uncertainty — confirm against the tenant's compose
  fault levers.)

- **`kubeactions-at-most-once` without node termination** still has a feasible core: the
  split-brain re-execution path (two leaders each with their own in-memory ActionStore) needs only
  the partition fault the catalog already relies on for `dispatch-implies-lease-holder`. The
  at-most-once property could be reformulated to lead with the two-leader duplication path (no
  container kill needed) and treat crash-replay as the enhancement, rather than presenting node
  termination as a hard dependency.

---

## Priority sanity (framing, not a full census — that's coverage-balance's job)

- **`node-expiry-monotonic-clock` at P1** is "fully inert without clock skew," and clock skew is
  disabled by default AND cannot be induced by the workload (only the leader's own clock stamps
  heartbeats, per the property's own open question). A P1 that passes vacuously in the default
  configuration is mis-priced: either it is gated as "requires clock-fault-enabled tenant" and
  deprioritized, or it moves to a static/unit check of the `time.Now().Unix()` code path (which is
  a fixed-input fact, not Antithesis-shaped). Flagging the priority/feasibility tension; the
  cross-lens resolution is that a known-defect on an un-inducible fault is not a P1.

---

## What looks right (so this isn't all negative)

- The leadership-core cluster (`dispatch-implies-lease-holder`, `leadershipchan-no-wedge-under-lock`,
  `leader-eventually-dispatches-after-warmup`) is correctly centered on the genuinely hardest,
  timing-dependent hazard (the three-notions-of-leader divergence) and is well-matched to the
  partition fault that IS enabled by default.
- The `reset-restores-store-and-gauges` regression framing (repeated historical fixes → probe
  adjacent state under churn) is a sound use of bug-history as a coverage signal.
- The catalog is honest about fault-availability caveats per property, and the topology doc
  correctly identifies that the workload must OWN the EndpointSlice/heartbeat timing to turn
  propagation lag and node expiry into controllable inputs.

---

## Uncertainties

- Whether the compose harness can stop/restart a SUT container as a routine fault (would revive
  the node-termination-gated properties) vs. relying on Antithesis's node-level termination fault.
  I did not inspect tenant fault config.
- Whether the workload spec (owned by `antithesis-workload`, not yet written) will model
  node-agent backoff and cached-check-keeps-running behavior — the crux of the duplicate-execution
  gap and of `getconfigs-distinguishes-unknown-node`.
- Whether some symptom properties (stale cert, stale endpoint) are cheaper to subsume under a
  single informer-freeze root-cause property, or whether the symptoms diverge enough to warrant
  separate assertions. I did not read the forwarder/admission evidence files in depth.
- Exact crypto/tls behavior for `GetCertificate → (nil,nil)` (handshake fail vs fallback) is an
  open question the admission property already flags; it governs whether that property is
  observable at all.
