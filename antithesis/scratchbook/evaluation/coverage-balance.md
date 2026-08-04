---
sut_path: /Users/jon.rosario/go/src/github.com/DataDog/datadog-agent
commit: f2da1471bb748fb5108f89f36f7b83cab305ca79
updated: 2026-07-21
external_references: []
---

# Coverage-Balance Evaluation — DCA Antithesis Property Catalog (27 properties)

Lens: read `sut-analysis.md` section by section; for each high-risk area ask whether a
proportionate property exists. Find risk areas with NO property (gaps), areas that are
over-weighted, missing property types, and component blind spots vs the deployable topology.
Bias: skeptic. I confirmed the 27 evidence files match the catalog 1:1 (no hidden coverage)
and grounded each gap against primary source.

## Method

- Enumerated the risk claims in sut-analysis §2–§10 and mapped each to catalog slugs.
- Confirmed `properties/` contains exactly the 27 slugs in the catalog — nothing extra.
- Grepped the SUT to confirm the flagged-but-uncovered surfaces are real, live DCA code
  (not out-of-scope or dead): leader-forwarder RequestURI restoration, DCA gRPC
  tagger/kube-metadata streaming, DatadogMetric CRD autoscaling store, rebalance algorithm.

## Where the 27 properties land (weight map)

| SUT area (sut-analysis §) | Properties | Count |
|---|---|---|
| Leadership split-brain / self-promotion (§4) | dispatch-implies-lease-holder, new-leader-elected-after-loss | 2 |
| Leader forwarder (§4) | forwarder-ip-proxy-consistency, forwarder-single-hop-loop-cap, forwarder-target-is-live-endpoint | 3 |
| Dispatch store integrity (§5) | dispatch-store-bijection, reset-restores-store-and-gauges, dangling-eventually-redispatched, dangling-redispatch-no-resurrect, ksm-shard-tracking-consistency, lastconfigchange-monotonic-epoch | 6 |
| Dispatch liveness / warmup (§5) | leader-eventually-dispatches-after-warmup | 1 |
| Concurrency / lock hazards (§4,§5) | store-lock-bounded-under-slow-clc, leadershipchan-no-wedge-under-lock, liveness-probe-no-restart-loop | 3 |
| Node liveness / dispatch mode (§5) | node-expiry-monotonic-clock, advanced-dispatching-node-set-integrity | 2 |
| Idempotency (§6) | kubeactions-at-most-once | 1 |
| External metrics / HPA (§6,§7) | extmetrics-configmap-no-lost-update, extmetrics-backoff-cap-stays-serving | 2 |
| Protocol / auth (§3) | no-404-on-registered-cluster-check-routes, getconfigs-distinguishes-unknown-node, isexternalpath-classifier-consistency, empty-token-never-authenticates | 4 |
| Lifecycle (§2,§7) | graceful-shutdown-releases-lease-bounded, admission-webhook-no-silent-nil-cert, autoscaling-fatal-startup-crashloop | 3 |

**Headline imbalance:** ~15 of 27 properties (dispatch store 6 + leadership 2 + forwarder 3 +
concurrency 3 + dispatch liveness 1) concentrate on the cluster-check dispatch/leadership
control loop. That is the correct crown-jewel focus — but it has crowded out entire DCA
subsystems that the topology can deploy and fault. Admission has ONE property (nil-cert only);
autoscaling has ~2.5 (extmetrics x2 + autoscaling-fatal); the **gRPC tagger/kube-metadata data
plane has ZERO**; **orchestrator collection has ZERO**; the **DatadogMetric CRD path has ZERO**
(the legacy ConfigMap path got the one autoscaling-store property).

## Property-type balance

- Safety ~17, Liveness 5 (new-leader, dangling-eventually, leader-eventually-dispatches,
  liveness-probe, graceful-shutdown), Reachability 2 (extmetrics-backoff, autoscaling-fatal).
- No pure `Unreachable` and no explicit `Sometimes`-only reachability beyond the two Reachable
  ones — acceptable. The type mix is reasonable *within the covered areas*; the problem is
  breadth, not the safety/liveness split.

---

## GAPS — real, live DCA risk with no property

### G1. gRPC tagger + kube-metadata streaming data plane — ZERO coverage (component blind spot)
sut-analysis §3 lists the main API port as carrying "IPC, gRPC tagger/kube-metadata streams,"
and the topology (§ container inventory) names them as a node-agent-facing surface. Confirmed
live: `cmd/cluster-agent/api/server.go:130` registers `AgentSecureServer`; `grpc.go:34`
implements `StreamKubeMetadata`; `comp/core/tagger/server` is wired in. These are long-lived
streaming subscriptions consumed by every node agent. The known unsubscribe race on node
reconnect (issue #48026) is exactly a timing/interleaving hazard — Antithesis's strength —
yet there is not one property. A leaked/duplicated subscription or a stream that never
re-establishes after a leadership flip is a cluster-wide metadata/tagging outage. This is the
single largest component blind spot: an entire node-agent-facing data plane on the same port
as the (heavily-covered) HTTP cluster-check plane.

### G2. DatadogMetric CRD leader/follower store divergence — ZERO coverage
sut-analysis §6 records the DatadogMetric CRD status source-of-truth as "local in-memory store
only for the leader" (persistence "mixed"). `pkg/clusteragent/autoscaling/externalmetrics/store`
is a whole subtree. The ONE external-metrics property (extmetrics-configmap-no-lost-update)
covers only the *legacy* ConfigMap store, and its own open questions concede "Do real
deployments predominantly use the DatadogMetric CRD path… If the CRD path dominates, this
property should pivot." So the catalog invests in the path it itself suspects is legacy and
leaves the modern path — where a follower's in-memory store can diverge from CRD status across
a leadership flip — entirely uncovered. Under split brain or churn, HPAs read divergent metric
values. This is a proportionate-risk gap the catalog is aware of but did not fill.

### G3. Token / cluster-ID ConfigMap concurrent-create race — ZERO coverage
sut-analysis §6 explicitly flags the DCA-token and cluster-ID ConfigMaps as "read-then-
create/update **with no conflict guard**," and §2 step 6 puts `GetOrCreateClusterID` on the
critical startup path. First-boot with two replicas racing to create the same ConfigMap, or a
split-brain re-create, can produce a lost write or a cluster-ID flip — poisoning every payload
tagged with cluster ID cluster-wide. empty-token-never-authenticates touches the token *value*
but not the concurrent-create race. This is a classic Antithesis interleaving target with no
property.

### G4. Leader-forwarder path restoration from RequestURI — ZERO coverage
sut-analysis §10 lists "Forwarder path restoration from RequestURI after StripPrefix is
correct" as a claimed-but-untested guarantee. Confirmed live at
`pkg/clusteragent/api/leader_forwarder.go:123-131`: the Director re-parses `req.RequestURI`
(because StripPrefix mutated `URL.Path`) to rebuild the proxied path. A parse failure or a
prefix/escaping edge case silently forwards to the wrong path → node-agent request mis-routed
to a different DCA handler. The catalog has FOUR forwarder properties (ip-proxy, single-hop,
target-live, plus dispatch-implies touches it) but none assert the forwarded request actually
reaches the intended path. Over-investment in the forwarder's *IP/loop* correctness, under-
investment in its *path* correctness.

### G5. Rebalance convergence / termination — ZERO coverage
sut-analysis §8 flags rebalance as a "freshly-landed complex algorithm (merged→reverted→
reapplied; `continue`→`break` fix #52884) acting on stale busyness when a runner is
unreachable" — the highest churn-risk code in dispatch. Confirmed live:
`pkg/clusteragent/clusterchecks/dispatcher_rebalance.go`. store-lock-bounded-under-slow-clc
covers the *lock hold* during stats collection, but nothing asserts the rebalance loop
terminates / converges / does not thrash-move a check every cycle under stale or partial
utilization data. A non-converging rebalance re-moves checks indefinitely (dispatch churn,
duplicate windows). Proportionate-risk gap in the most-recently-broken area.

### G6. Informer cache freeze under partition (informer_client_timeout=0) — thin coverage
sut-analysis §7 flags `informer_client_timeout = 0` → a partition that drops a watch without
RST "freezes the informer cache with no error surfaced," and adds `connect()` can "succeed"
against a half-broken apiserver. admission-webhook-no-silent-nil-cert touches one frozen
informer (the secret lister) and forwarder-target-is-live-endpoint touches EndpointSlice lag,
but there is no property for the general hazard: the DCA serving stale kube-metadata/tags (over
the G1 gRPC streams) or making leader/dispatch decisions on a frozen cache while reporting
healthy. Given informers back tagging, metadata, endpoints, secrets, and CRDs, one property
per consumer is not expected — but the *class* (frozen-informer staleness surfaced as
authoritative) is under-represented relative to how central §7 makes it.

### G7. Admission failurePolicy=Fail fail-closed blast radius — thin/indirect coverage
sut-analysis §7 item 5: with `failure_policy: Fail`, DCA-down → "**all pod creation blocked
cluster-wide**." admission-webhook-no-silent-nil-cert is about a nil cert, and lists the
failurePolicy default as an open question rather than asserting behavior. Whether the webhook
stays available (or fails open per its configured policy) under DCA leadership churn / apiserver
partition is the actual cluster-wide risk, and no property targets it. This is arguably partly
out-of-DCA-scope (apiserver enforces the policy), but DCA-side webhook availability under fault
is in scope and unasserted.

### G8. Orchestrator resource collection — ZERO coverage (and absent from the SUT model)
The task lens names orchestrator collection as a risk area. Neither sut-analysis nor the
catalog mentions the orchestrator explorer (k8s resource collection → Datadog) at all. If it
is intentionally out of scope that should be stated; as it stands it is an unexplained
component blind spot. Lower confidence than G1–G5 because the SUT model itself omitted it —
flagging as an uncertainty to resolve, not a confirmed proportionate gap.

---

## OVER-WEIGHTING

### O1. Dispatch-store integrity is 6 properties with heavy internal overlap
dispatch-store-bijection, reset-restores-store-and-gauges, dangling-eventually-redispatched,
dangling-redispatch-no-resurrect, ksm-shard-tracking-consistency, and lastconfigchange-
monotonic-epoch all assert flavors of "the in-memory store stays consistent across reset/
churn." They share the same fault driver (leadership flap + node expiry) and the same
instrumentation (store-lock validator). The `reset()`-zeroes-`lastConfigChange` open question
is literally repeated verbatim across FOUR of them (bijection lists it three times in its own
Open Questions). This is real signal — dense bug-fix history — but six near-adjacent P1/P2
safety properties on one data structure, versus zero on the gRPC data plane and CRD store, is
disproportionate. Several could merge (bijection already subsumes much of reset and dangling-
no-resurrect) to free budget for G1–G3.

### O2. Forwarder is 3–4 properties, all about IP/loop, none about payload/path
See G4. The forwarder gets ip-proxy-consistency + single-hop-loop + target-live-endpoint (+
dispatch-implies touches its state), yet the path-restoration and header-preservation
(Authorization forwarding is only an open question in forwarder-target) correctness — what
actually determines whether a forwarded node-agent request succeeds — is not asserted.
Breadth-within-forwarder is skewed to the connection target and away from the request content.

---

## PORTFOLIO-LEVEL FAULT-AVAILABILITY CONCENTRATION

Beyond the per-property warnings the catalog already carries, as a *portfolio* a meaningful
tail leans on faults disabled by default:

- **Fully inert without clock skew:** node-expiry-monotonic-clock (catalog admits "INERT
  unless the tenant enables clock faults").
- **Largely inert / weak without clock skew or node termination:** lastconfigchange-monotonic-
  epoch (P2), forwarder-target-is-live-endpoint (strong exploit needs pod-IP change on
  reschedule = node termination), kubeactions-at-most-once (crash-replay needs node termination;
  otherwise passes only if it also rides a split-brain violation — high vacuous-pass risk),
  autoscaling-fatal-startup-crashloop (restart needs node termination).

That is ~5/27 properties (all outside the partition-driven crown jewels) whose primary trigger
is a commonly-disabled fault. If the tenant runs defaults, these pass vacuously and inflate the
apparent coverage of the autoscaling/idempotency/node-liveness areas that are *already* thin.
The crown-jewel set is correctly partition-driven and robust; the tail is fragile. This
compounds the imbalance: the under-covered areas are disproportionately the ones that also risk
vacuous passes.

## What is correctly covered (passes)

- The split-brain / three-notions-of-leader core (§4) is the highest-risk area and gets the
  highest-priority, partition-driven (not disabled-fault) property: dispatch-implies-lease-holder.
- Concurrency lock-hazards (§4 leadershipChan, §5 updateRunnersStats) each have a dedicated P0
  property well matched to Antithesis interleaving strength.
- The router-mutation-after-Serve startup gap (§3) and dual-token auth boundary (§3) each have
  proportionate properties (no-404, isexternalpath, empty-token).
- Warmup starvation and dangling redispatch liveness (§5) are covered with correct Sometimes
  assertions.

## Uncertainties

- Orchestrator (G8): absent from the SUT model — cannot confirm it is in scope for this DCA
  harness. May be a deliberate scoping decision.
- G7 (failurePolicy) severity is partly apiserver-side; the in-scope DCA slice (webhook
  availability under fault) is narrower than the cluster-wide framing.
- Whether merging the six dispatch-store properties (O1) is desirable depends on Antithesis
  cost per property vs. the value of independent failure attribution — I lean toward some merge
  but flag it as a trade-off, not a certainty.
