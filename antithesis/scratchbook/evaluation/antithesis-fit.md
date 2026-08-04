---
sut_path: /Users/jon.rosario/go/src/github.com/DataDog/datadog-agent
commit: f2da1471bb748fb5108f89f36f7b83cab305ca79
updated: 2026-07-21
external_references: []
---

# Antithesis-Fit Evaluation — DCA Property Catalog

Lens: for each property, does verifying it require exploring a state space a deterministic
unit test cannot reach (timing / concurrency / partial-failure / interleaving)? Flag properties
that are really unit/integration territory, assertion-type mismatches, under/over-estimated
Antithesis value, and check the Reachable/Sometimes/Always balance. Biased toward finding
problems with the catalog as a portfolio.

## Assertion-type census (27 properties)

- **Always: 10** — dispatch-implies-lease-holder, dispatch-store-bijection,
  forwarder-single-hop-loop-cap, liveness-probe-no-restart-loop, kubeactions-at-most-once,
  no-404-on-registered-cluster-check-routes, empty-token-never-authenticates,
  store-lock-bounded-under-slow-clc, leadershipchan-no-wedge-under-lock,
  lastconfigchange-monotonic-epoch
- **AlwaysOrUnreachable: 11** — forwarder-ip-proxy-consistency, forwarder-target-is-live-endpoint,
  reset-restores-store-and-gauges, dangling-redispatch-no-resurrect, ksm-shard-tracking-consistency,
  node-expiry-monotonic-clock, advanced-dispatching-node-set-integrity,
  extmetrics-configmap-no-lost-update, getconfigs-distinguishes-unknown-node,
  isexternalpath-classifier-consistency, admission-webhook-no-silent-nil-cert
- **Sometimes: 4** — new-leader-elected-after-loss, leader-eventually-dispatches-after-warmup,
  dangling-eventually-redispatched, graceful-shutdown-releases-lease-bounded
- **Reachable: 2** — extmetrics-backoff-cap-stays-serving, autoscaling-fatal-startup-crashloop

Safety (Always+AOU) = 21 of 27 (78%). Liveness/reachability witnesses = 6 of 27 (22%).

---

## CATALOG-WIDE FINDINGS

### CW-1. Safety-heavy skew with too few reachability witnesses → vacuous-pass risk

78% of the catalog is Always/AOU. Antithesis's core failure mode for a safety-heavy catalog is
the **silent vacuous pass**: an `Always`/`AlwaysOrUnreachable` stays green not because the
invariant is robust but because the adversarial interleaving/window that would break it was
never actually scheduled. AOU only proves the *assert statement* was reached, not that the
*specific race* it targets occurred.

The race-shaped safety properties each need a paired `Reachable`/`Sometimes` witness proving the
hazardous precondition was hit, and the catalog does not supply them. Concretely missing:

- forwarder-ip-proxy-consistency → `Reachable(two SetLeaderIP writers interleaved / GetLeaderIP=="" observed on a follower)`
- forwarder-single-hop-loop-cap → `Reachable(a request arrived already carrying X-DCA-Follower-Forwarded)` — without it, the 508 guard is never even exercised.
- dangling-redispatch-no-resurrect → `Reachable(Unschedule interleaved between retrieveDangling and addConfig)`
- ksm-shard-tracking-consistency → `Reachable(AD Schedule landed between reset() and RemoveScheduler)`
- dispatch-implies-lease-holder → `Sometimes(a follower observed GetLeaderIP()=="" while the lease was held elsewhere)` — proves the split-brain *window* opened, not just that one leader existed.

Without these, a fully-green catalog cannot be distinguished from "the harness never opened the
window." Recommend adding a witness assertion for every race-dependent safety property.

### CW-2. A cluster of properties is inert or untestable under default fault config

The catalog flags this, but from the antithesis-fit angle it is more severe than "vacuous
Always": for several properties the *disabled fault is the entire mechanism*, so they contribute
zero exploration value on a default tenant, not merely a trivially-true invariant.

- **Clock skew DISABLED**: node-expiry-monotonic-clock (catalog: "fully inert"),
  lastconfigchange-monotonic-epoch ("largely inert"). Both are fundamentally clock-skew
  demonstrations; with clock faults off they are dead weight (2 of 27 slots).
- **Node termination DISABLED**: kubeactions-at-most-once crash-replay variant,
  new-leader-elected-after-loss crash variant, forwarder-target-is-live-endpoint (its headline
  "same pod name, new IP" hazard needs a reschedule). new-leader has a partition fallback;
  kubeactions has a split-brain fallback; **forwarder-target-is-live-endpoint has no default-fault
  path to its core hazard** unless the workload simulates the IP change via the EndpointSlice it
  owns — which should be stated as a hard requirement, not an open question.

Recommend: gate these behind an explicit "requires fault X enabled" tier and, where a
workload-driven substitute exists (EndpointSlice mutation for the stale-IP case), make it the
primary path rather than relying on node termination.

### CW-3. 4–5 slots are deterministic input-domain / code-fact checks — unit-test territory

These verify a pure function of an input or a deterministic code branch; a fixed-input unit test
covers them more cheaply, reliably, and quickly than a fault-injection run, and Antithesis's
timing/concurrency strengths add nothing:

- **isexternalpath-classifier-consistency** — verified: `isExternalPath` (server.go:199-219) is a
  pure function of `path` with no state, no clock, no concurrency. The catalog itself says "Not
  fault-timing dependent" and its own open question admits the assertion may be "tautological."
  This is a table-driven unit test.
- **getconfigs-distinguishes-unknown-node** — verified: the unknown-node branch is a deterministic
  `return ... fmt.Errorf("node %s is unknown", nodeName)` (dispatcher_nodes.go:34). No fault needed
  to reach it (just don't register the node); and it is *aspirational* (code returns 500 today), so
  as an `AlwaysOrUnreachable` it will simply report the same constant verdict every evaluation — a
  design critique, not a runtime invariant an execution explores.
- **admission-webhook-no-silent-nil-cert** — verified: the callback logs and `return cert, nil`
  (admission/server.go), returning (nil,nil) deterministically whenever the lister errors.
  Unit-testable with a fake erroring lister; the partition-freezes-informer angle is a marginal way
  to trigger a deterministic branch.
- **autoscaling-fatal-startup-crashloop** — the catalog admits "near-deterministic ... Lower
  Antithesis value." A `Reachable` on a fatal startup return is essentially a startup unit/integration
  test.

Partial: **empty-token-never-authenticates** — the core claim (empty configured token must never
compare equal) is deterministic compare-logic, unit-testable directly. Only the startup ordering
race (server accepting before token populated) is Antithesis-shaped, and it is marginal.

These 4–5 properties occupy ~18% of the catalog without exploiting the tool. Recommend demoting to
a "covered by unit test" appendix or explicitly de-prioritizing.

---

## PROPERTY-SPECIFIC FINDINGS

### PS-1. extmetrics-backoff-cap-stays-serving — Reachable target likely unreachable within a run

`Reachable(backoff_reached_cap)` targets the 1800s (30-minute) exponential-backoff cap. The
property's own open question asks whether "the 1800s cap fits within a single Antithesis timeline."
If a typical run is shorter than 30 min of sustained backend partition, this Reachable **never
fires** and reports a permanent (silent) not-yet-reached — the worst outcome for a Reachable, since
it looks identical to "not tested." Requires shortening the backoff constants via config for the
harness, otherwise the primary assertion is inert. Flag as an assertion-target/duration mismatch.

### PS-2. liveness-probe-no-restart-loop — redundant + ill-specified Always

The catalog states this "is really a consequence property of the two lock-hazard properties"
(store-lock-bounded-under-slow-clc, leadershipchan-no-wedge-under-lock). It largely duplicates their
coverage. Worse, its own open question — "the property must not penalize correct hang-detection" —
means the `Always(probe_drained_within_period)` has no crisp boundary between a *legitimate*
liveness failure (real deadlock the probe is supposed to catch) and a *false* one (transient
latency). An Always whose predicate you cannot state precisely will either flake or be watered down
to vacuity. Consider dropping in favor of the two lock-hazard properties plus a Sometimes witness.

### PS-3. store-lock-bounded-under-slow-clc — timing-threshold Always is calibration-fragile

`Always(lock_hold_duration < bound)` is a latency SLA dressed as a safety invariant. Three of its
open questions admit the CLC-runner client timeout / bound T is unknown and "needs measuring."
An Always keyed to an uncalibrated numeric threshold fails both ways: T too large → passes
vacuously (never catches the stall); T too small → flakes on ordinary scheduling jitter in the
Antithesis environment. The underlying hazard is real and Antithesis-appropriate (blocking I/O
under a global lock), but the assertion should be framed as a logical state ("no network call
issued while holding d.store lock", instrumentable as a boolean) rather than a wall-clock bound.

### PS-4. node-expiry-monotonic-clock — inert + expected-to-fail code-fact demonstration

Depends entirely on clock skew (DISABLED by default). Without it the AOU passes vacuously every
evaluation. Even *with* clock skew, the finding it demonstrates is a static code fact (expiry uses
`time.Now().Unix()`, helpers.go), which a unit test injecting a fake clock proves deterministically.
Antithesis's added value here is only the realistic "backward NTP jump under load" scenario — real
but contingent on a disabled fault. Gate behind clock-fault availability; otherwise dead.

### PS-5. lastconfigchange-monotonic-epoch — largely inert; mostly a clock-skew property

Primary trigger is backward clock jitter (DISABLED). The catalog concedes it is "largely inert
without it or without a leadership flap." The int64-nanosecond-collision failure mode has near-zero
probability without skew. Under default faults this contributes little; its distinct value (missing
leader-epoch tag) overlaps with dispatch-store-bijection's open questions. Consider folding the
epoch concern into the store-bijection evidence rather than a standalone near-inert Always.

### PS-6. Pass — genuine Antithesis-fit properties (for balance)

These strongly require the state space Antithesis explores and are correctly typed:
dispatch-implies-lease-holder (cross-replica Always, needs partition+interleaving),
dispatch-store-bijection (Always, node-agent partition drives expiry — no disabled fault needed),
leader-eventually-dispatches-after-warmup (Sometimes, flap-at-warmup starvation),
leadershipchan-no-wedge-under-lock (Always, buffered-channel-send-under-lock interleaving),
new-leader-elected-after-loss (Sometimes, partition fallback works on defaults),
dangling-eventually-redispatched (Sometimes), reset-restores-store-and-gauges (AOU regression
cluster). These are the crown jewels and their types fit.

---

## UNCERTAINTIES

- **graceful-shutdown-releases-lease-bounded (P2, Sometimes)** may be *under*-rated: partition +
  SIGTERM + a ReleaseOnCancel network call that can hang is genuine partial-failure timing content,
  arguably P1. Left as uncertainty because impact (delayed failover during rolling upgrade) is
  bounded and it overlaps new-leader-elected-after-loss.
- Whether client-go at the default 60s lease can produce leader self-transitions fast enough to
  open the warmup-interruption and leadershipchan-wedge windows without clock-skew/node-termination
  faults is an unresolved open question across several properties; if it cannot, more of the Always
  set slides toward vacuous than this evaluation assumes.
- Did not independently verify the 1800s backoff constant or the CLC-runner client timeout in
  source (relied on catalog line refs) — PS-1 and PS-3 severity depends on those exact values.
