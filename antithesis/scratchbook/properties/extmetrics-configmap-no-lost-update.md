# extmetrics-configmap-no-lost-update — HPA external-metrics ConfigMap writes are not silently lost

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P1 · **Intent:** known-defect-reproducer

**Provenance:** merged from 2 discovery agent(s): externalmetrics-configmap-lost-update, extmetrics-configmap-rmw-lost-write-and-cache-wedge

## Property

A leader's write of external-metric values to the shared HPA ConfigMap either persists the intended keys or is retried on conflict; a rejected optimistic-concurrency update must not silently drop values or leave the local cache permanently stale.


## Invariant / assertion

`assert.AlwaysOrUnreachable`: after SetExternalMetricValues, either the ConfigMap reflects the values or a retry was attempted; on IsConflict the local cache is refreshed, not left stale. AlwaysOrUnreachable fits — this path runs only when external metrics are enabled. The code does plain Update() with no resourceVersion retry (store_configmap.go:190-200), so the property is expected to expose lost updates under split brain.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: two replicas issued overlapping ConfigMap Updates (a split-brain write actually occurred).


## Antithesis angle

updateConfigMap Updates a locally-cached c.cm with no conflict handling; on IsConflict the error is logged and the write dropped, leaving c.cm stale while GetMetrics reads the stale copy. Two replicas both believing they are leader (partition during lease renewal) both write, overwriting Data wholesale from stale local copies → values lost/flip-flop. Inject apiserver latency/partition around RenewDeadline and assert HPA metric values don't regress. Merged from 2 focus agents.


## Why it matters

Lost or flip-flopping external-metric values make HPAs scale on wrong data or stop scaling — a direct, customer-visible autoscaling failure. Depends on the one-leader guarantee holding.


## Mechanism refinement (from open-question investigation)

Scope/severity weakened (not invalidated). Evidence: (1) the write path is strictly leader-gated (controller_util.go:243) so the RMW conflict requires actual split-brain; (2) ListAllExternalMetricValues re-Gets the ConfigMap at the start of every refresh cycle (controller_util.go:185 -> store_configmap.go:139) immediately before SetExternalMetricValues (:224), so the stale-ResourceVersion window is per-cycle, not persistent; (3) the c.cm==nil 'permanent wedge' claim is false — the same per-cycle ListAll (and the GC loop at :140) repopulates c.cm, self-healing within ~1 refresh_period (~30s). The property should assert 'a leader write is either persisted or retried, and any cache wedge clears within one refresh cycle', not a permanent lost-update/wedge.


## Fault dependencies

- network partition producing split brain (two replicas writing; enabled by default)
- apiserver latency around lease renewal
- requires leader_election enabled + >=2 replicas + external_metrics_provider enabled (ConfigMap store, not CRD)


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.AlwaysOrUnreachable` after Update checking success-or-retry and cache freshness. Confirm which provider (ConfigMap vs DatadogMetric CRD) is active via external_metrics_provider.use_datadogmetric_crd.


**Resolved (user decision, 2026-07-21):** the harness pins the DatadogMetric CRD provider (`use_datadogmetric_crd: true`) as primary. This ConfigMap-path property is **deprioritized to secondary/optional** — run only if the legacy provider is separately exercised in a dedicated pass.

## Open questions (post-investigation)

- Do real-world DCA deployments predominantly enable use_datadogmetric_crd (CRD path) vs the default legacy ConfigMap store? Code confirms the default is ConfigMap (use_datadogmetric_crd=false), so the RMW path is default-reachable; the adoption skew itself is a product fact not answerable from code. `(needs human input)`


### Investigation Log

#### Q1: real deployments predominantly CRD vs legacy ConfigMap?

Examined common_settings.go:561 — external_metrics_provider.use_datadogmetric_crd defaults to false; controllers.go:53 gates the legacy ConfigMap autoscalersController on `enabled && !use_datadogmetric_crd`. Found: legacy ConfigMap is the DEFAULT provider, so the RMW hazard path is reachable out-of-the-box. Conclusion: code answers 'default=ConfigMap'; real-world adoption skew is a product fact -> needs-human, kept.

#### Q2: does refresh call ListAllExternalMetricValues before SetExternalMetricValues in same cycle?

Examined controller_util.go updateExternalMetrics(): ListAllExternalMetricValues at :185 (which calls getConfigMap at store_configmap.go:139 -> re-Gets CM, refreshing c.cm and its ResourceVersion), then SetExternalMetricValues at :224. Found: YES, same-cycle re-Get immediately precedes the write. Conclusion: the stale-ResourceVersion window is bounded to one cycle's compute time, not persistent -> RESOLVED (omitted).

#### Q3: is SetExternalMetricValues strictly leader-gated (legacy vs modern)?

Examined controller_util.go processingLoop:243 `if !h.isLeaderFunc() { continue }` before updateExternalMetrics -> legacy CM write path is strictly leader-gated. Modern CRD writers (metrics_retriever.go:61 isLeader; syncDatadogMetric isLeader branch, datadogmetric_controller.go:213) are also leader-gated. Found/Conclusion: both gated; the RMW hazard only manifests under split-brain (two believing-leaders) -> RESOLVED (omitted).

#### Q4: does a caller invoke ListAllExternalMetricValues often enough to self-heal the wedge within a refresh cycle?

Examined controller_util.go: both updateExternalMetrics (:185) and the GC loop (:140) call ListAllExternalMetricValues every refresh/gc period (external_metrics_provider.refresh_period default 30s). ListAll -> getConfigMap repopulates c.cm even after a conflict set it nil. Found: the errNotInitialized wedge self-heals at the next cycle. Conclusion: wedge bounded to <= ~1 refresh cycle (~30s) -> RESOLVED (omitted).

#### Q5: which provider is active in the harness?

Examined antithesis/ tree — only scratchbook/, no compose/manifests/config yet; deployment-topology.md:66 says the stub backend lets the workload pin ConfigMap-vs-CRD per run. Found: undecided harness-design choice; config default is ConfigMap. Conclusion: needs-human, kept.


---

## Source discovery evidence (raw, per contributing agent)


### from `externalmetrics-configmap-lost-update`

## Property
The HPA external-metrics ConfigMap (legacy custommetrics path) must not silently lose writes under concurrent writers.

## Where the state lives
`pkg/clusteragent/autoscaling/custommetrics/store_configmap.go`:
- `configMapStore.cm *v1.ConfigMap` — locally cached copy, guarded by `mu`.
- `updateConfigMap()` (178-189): `c.cm, err = client.ConfigMaps(ns).Update(ctx, c.cm, UpdateOptions{})` — sends the cached object (with its cached ResourceVersion). On error: `log.Infof(...); return err`. No retry, no re-Get.
- `SetExternalMetricValues` (81-104): locks `mu`, mutates `c.cm.Data[key]=...`, calls `updateConfigMap()`. Does NOT re-Get first.
- `getConfigMap()` (170-177) refreshes `c.cm` but is only called from the constructor and `ListAllExternalMetricValues`.

## Why this loses updates
Kubernetes Update uses optimistic concurrency ONLY via ResourceVersion. If replica Y updated the CM after replica X's last Get/Update, X's cached ResourceVersion is stale -> X's Update returns 409 Conflict. The code drops it (log + return). X's whole `added` batch for that cycle never persists. The 30s leader refresh (analysis §7) means the values are stale until the next cycle, which will conflict again if the split brain persists.

## Create race variant
`NewConfigMapStore` (46-77): Get; if NotFound, Create. Two replicas booting concurrently both see NotFound and both Create -> one gets AlreadyExists (returned as error, store init fails for that replica) — a concurrent-create hazard on the same shared CM (also applies to cluster ID / DCA token per analysis §6).

## Failure scenario
1. HPA external metrics enabled on the legacy ConfigMap path; >=2 DCA replicas.
2. Network partition causes split brain: both replicas run the leader-only metrics_retriever refresh loop.
3. Replica X caches CM at ResourceVersion R. Replica Y writes -> CM now R+1.
4. X's metrics_retriever computes fresh values, calls SetExternalMetricValues -> Update(c.cm@R) -> 409 Conflict -> logged, returned, dropped.
5. HPAs backed by X's autoscalers read stale/missing values -> scaling frozen. No error surfaced above info log.

## Assertion points (MISSING — net-new)
- `assert.Sometimes(updateErr is Conflict, "external-metrics CM update hit optimistic-concurrency conflict", ...)` in updateConfigMap — proves the hazard is reached under fault.
- Stronger Always goal: after a write intended by the leader, ListAllExternalMetricValues eventually reflects it; assert the intended keys are present after a bounded retry, else the write was lost.

## Existing coverage gap
`store_configmap_test.go` uses fake clientset with no ResourceVersion conflict simulation (analysis §9); the lost-update path is untested.


### from `extmetrics-configmap-rmw-lost-write-and-cache-wedge`

## Mechanism (verified)

**RMW against a cached object** (store_configmap.go):
- SetExternalMetricValues mutates `c.cm.Data[key]` in place (:98-105) then calls updateConfigMap; DeleteExternalMetricValues likewise (:121-131). Neither re-Gets the ConfigMap first — the write base is whatever c.cm was last set to (init Get at :54, or a prior ListAll refresh at :139).
- updateConfigMap: `c.cm, err = c.client.ConfigMaps(c.namespace).Update(context.TODO(), c.cm, metav1.UpdateOptions{})` (:193). The client-go typed Update returns a nil object on error, so **on conflict c.cm becomes nil**. The function logs and returns err (:194-196); there is no conflict-retry, no re-Get-and-reapply.

**Wedge** (store_configmap.go:91-93, :118-120): once c.cm==nil, all writes return errNotInitialized until ListAllExternalMetricValues (:136-143) calls getConfigMap and repopulates c.cm. Nothing on the write path itself recovers.

**No optimistic-concurrency retry loop** anywhere in the store (contrast with client-go's RetryOnConflict, which is absent). SUT analysis §6 flags this store as 'read-modify-write with no resourceVersion/optimistic-concurrency guard' — verified that the guard is only implicit (the cached rv) and its rejection is mishandled.

## Failure scenario
1. Replicas P1 and P2 each hold a configMapStore with c.cm at rv=10 (both read it).
2. P1 SetExternalMetricValues -> Update succeeds, server rv=11, P1.c.cm rv=11.
3. P2 SetExternalMetricValues (base rv=10) -> Update -> 409 Conflict -> P2.c.cm=nil, P2's new metric values dropped, not retried.
4. P2 next SetExternalMetricValues -> errNotInitialized (wedged) until some caller invokes ListAllExternalMetricValues.

Assertion 'c.cm != nil after updateConfigMap returns' FAILS at step 3.

## Note on cluster-ID create-race (bounded, not a bug)
GetOrCreateClusterID (common.go:43-103) has a create race between replicas, but the value is deterministic (kube-system namespace UID, :32-38), so a lost create does not corrupt the ID — deliberately excluded as a property.
