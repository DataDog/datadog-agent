# extmetrics-crd-store-converges-after-flip — DatadogMetric CRD store converges after a leadership flip

**Type:** Liveness · **Assertion:** `Sometimes` · **Priority:** P1 · **Intent:** invariant

**Provenance:** merged from 1 discovery agent(s): extmetrics-crd-store-converges-after-flip

## Property

After a leadership flip, the newly-elected leader's in-memory DatadogMetricsInternalStore eventually reflects reality for every DatadogMetric that has a valid value in the CRD and is referenced by an active HPA: a GetExternalMetric query for that metric returns a present, non-stale value (not 'DatadogMetric not found' and not 'DatadogMetric is stale') once the shared informer has resynced and at least one MetricsRetriever/syncDatadogMetric cycle has run on the new leader.


## Invariant / assertion

flip_observed && new_leader_settled(elapsed > kubernetes_informers_resync_period + external_metrics_provider.refresh_period) ⇒ store.Get(id) != nil && Valid && !IsStale for every active, CRD-valid DatadogMetric served by the new leader `Sometimes` fits — a liveness/progress condition (the store converges) verified under a recovery window, not on every evaluation.


## Antithesis angle

Antithesis controls the exact interleaving of (a) the Lease renew/expiry that drives the flip, (b) the shared DatadogMetric informer's resync/list-watch, and (c) the leader-gated MetricsRetriever and AutoscalerWatcher tickers. The convergence window after a flip is opened only by specific orderings — e.g. flip happens right after a resync so the next resync is ~300s away, and the metric is briefly Inactive so the retriever skips it. A workload-only unit test cannot schedule a real Lease flip against a live informer; this is squarely Antithesis's timing/partial-failure domain.


## Why it matters

GetExternalMetric (provider.go:139) has NO leadership gate — every replica, including a freshly-promoted leader, answers HPA queries straight from its own in-memory store. If the new leader's store is missing an entry (Get returns nil ⇒ 'DatadogMetric not found', provider.go:172-174) or holds a value older than max_age (ToExternalMetricFormat returns 'stale', datadogmetricinternal.go:274-276), the HPA gets an error and stops scaling on that metric. Unlike the clusterchecks store, this store is never reset on a transition, so the failure mode is a silently-stale value rather than an empty store — but the convergence back to reality depends entirely on informer resync + a leader-only refresh cycle firing, which is exactly what a badly-timed flip can delay.


## Mechanism refinement (from open-question investigation)

No invalidation; convergence bound confirmed. Resync IS enabled at 300s (apiserver.go:452 + kubernetes_informers_resync_period default 300s) with UpdateFunc->enqueue (datadogmetric_controller.go:90), so the worst-case post-flip re-reconcile is <= max(resync_period 300s, refresh_period 30s). Refinement: GetExternalMetric has no cache-sync gate (provider.go:153), so an additional startup-transient 'not found' window exists before the store first populates — the AlwaysOrUnreachable staleness guard must be evaluated only after the settle window (as already specified) so this transient degrades to Unreachable rather than false-failing.


## Fault dependencies

- leader_election enabled + >=2 DCA replicas + external_metrics_provider.enabled with the DatadogMetric CRD provider (not legacy ConfigMap)
- a leadership flip: preferred workload-driven substitutes that need NO node-termination — (a) partition current leader <-> kube-apiserver for >= leader_lease_duration (default 60s) to force lease loss, or (b) restart the leader DCA container (container restart is enabled by default), or (c) workload directly mutates/deletes the coordination.k8s.io Lease to trigger re-election
- a stub dd-metrics-backend so the new leader's MetricsRetriever can return values (otherwise convergence to 'fresh' cannot be observed)
- NOT required: node-termination or clock-skew faults (both commonly disabled by default)


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

Add antithesis-sdk-go to the root module and instrument the externalmetrics package. (1) Witness: in processDatadogMetric, on the isLeader()==true branch, assert.Reachable("dca became leader with populated ddm store") guarded by store.Count()>0, and assert.Reachable("dca leader ran syncDatadogMetric after a flip") tagged with a monotonically-observed leadership epoch. (2) Convergence details for the workload to consume: export via the /metrics or a debug endpoint the per-metric UpdateTime/DataTime/Valid/Active and the current isLeader() + leadership epoch. Workload-side: after forcing a flip, poll for a new leader, then wait > (kubernetes_informers_resync_period + refresh_period), issue the HPA external-metric query against the new leader (or the Service), and assert.Sometimes(present && !stale, "new leader converged store served fresh HPA value post-flip"). Also assert.AlwaysOrUnreachable(present && !stale, "post-settle-window, active CRD-valid DatadogMetric is servable on the new leader") evaluated ONLY for queries taken after the settle window, so it degrades to Unreachable rather than false-failing during the legitimate convergence window.


## Open questions (post-investigation)

- Resolved (user decision, 2026-07-21): yes — the harness pins `use_datadogmetric_crd: true`. This is now the **primary** external-metrics property in the catalog.


### Investigation Log

#### Q1: does the shared dynamic informer deliver a resync UpdateFunc at 300s, or is resync disabled?

Examined apiserver.go:184 (defaultInformerResyncPeriod = kubernetes_informers_resync_period) and :452 (DynamicSharedInformerFactory built with it), common_settings.go:565 (default 300s), datadogmetric_controller.go:87-92 (AddFunc/UpdateFunc/DeleteFunc all -> enqueue). Found: resync is enabled at 300s and UpdateFunc re-enqueues every DatadogMetric on each resync (client-go shared/dynamic informers deliver periodic resync as synthetic update events when resyncPeriod>0). Conclusion: RESOLVED -> worst-case re-reconcile of an inactive metric on a new leader <= ~300s.

#### Q2: does the harness enable use_datadogmetric_crd?

Examined common_settings.go:561 (default false) and antithesis/ (no harness config yet). Found: undecided. Conclusion: needs-human, kept.

#### Q3: is the metric kept Active across the flip or does AutoscalerWatcher briefly flip it Inactive?

Examined autoscaler_watcher.go:159 (leader-only), :150-151 (WaitForCacheSync on HPA/WPA listers), processAutoscalers -> updateDatadogMetricStatus:221-235 (only changes Active when computed `active` differs; `active` derived from live HPA references; on Active->Inactive it discards Valid). Found: a continuously-referenced metric stays Active (Valid preserved); a transient Inactive only if getAutoscalerReferences returns empty (e.g., HPA lister not yet synced on the new leader before first processAutoscalers). Conclusion: RESOLVED -> unservable-widening window exists only during the brief pre-sync interval, not for a stably-referenced metric.

#### Q4: can GetExternalMetric hit a new-leader before its informer's initial WaitForCacheSync completes?

Examined provider.go getExternalMetric:153-184 (store.Get with no isLeader and no cache-sync gate) vs datadogmetric_controller.go:115 (WaitForCacheSync gates only the controller worker loop, not APIService serving). Found: YES — a just-started/just-promoted replica can answer HPA queries before its store is populated, returning nil -> 'DatadogMetric not found' (provider.go:172-174). Conclusion: RESOLVED -> confirmed startup transient, distinct from the flip; worth a separate witness but does not change the flip-convergence invariant.


---

## Source discovery evidence (raw, per contributing agent)


### from `extmetrics-crd-store-converges-after-flip`

## Mechanism (verified against source at commit f2da147)

One shared `DatadogMetricsInternalStore` (`store.go:33`, in-memory `map[string]DatadogMetricInternal` under an RWMutex) is the single source for HPA answers on **every** replica.

### Leader vs follower write paths diverge (`datadogmetric_controller.go:189-280`)
- **Follower** (`isLeader()==false`, lines 217-224): on every informer event it *blindly mirrors the CRD into the store* — `store.Set(key, NewDatadogMetricInternal(key, *cached), ddmControllerStoreID)` or `store.Delete`. `NewDatadogMetricInternal` (`datadogmetricinternal.go:57-110`) derives `Valid/Active/Value/UpdateTime/DataTime` purely from the CRD's `Status.Conditions` + `Status.Value`, and resets `Retries=0`.
- **Leader** (`isLeader()==true`, line 213-214 → `syncDatadogMetric`): the in-memory store is the **status source of truth**; it writes status back to the CRD via `UpdateStatus` only when `datadogMetricInternal.IsNewerThan(datadogMetric.Status)` (lines 274-277). Values come from the leader-only `MetricsRetriever` querying Datadog.

### HPA read path has no leader gate
`provider.getExternalMetric` (`provider.go:153-184`) does `p.store.Get(id)` with no `isLeader()` check. `nil` ⇒ `Warnf("DatadogMetric not found ...")` (lines 172-174). A found-but-old entry ⇒ `ToExternalMetricFormat` returns `"DatadogMetric is stale"` (`datadogmetricinternal.go:274-276`). The k8s external-metrics APIService routes to the DCA Service (all Ready pods), so a follower or a just-promoted leader can be the one answering.

### What drives convergence after a flip (and why it can lag)
There is **no explicit re-reconcile-all on becoming leader**. A newly-promoted leader re-syncs the store→CRD only when keys get re-enqueued:
1. Shared dynamic informer periodic resync fires `UpdateFunc`→`enqueue` for every object. Period = `kubernetes_informers_resync_period`, **default 300s** (`common_settings.go:565`; factory built with this period at `apiserver.go:452`).
2. `MetricsRetriever.Run` refreshes **only `Active`** metrics every `external_metrics_provider.refresh_period`, **default 30s** (`common_settings.go:552`; `metrics_retriever.go:60-65`, `retrieveMetricsValues` filters `datadogMetric.Active`), and its `UnlockSet` fires the store observer `enqueueID` (`datadogmetric_controller.go:98-100,150-155`) → reconcile.
3. `AutoscalerWatcher` (leader-only, `autoscaler_watcher.go:157-168`) recomputes `Active` from live HPAs; on Active→Inactive it **discards Valid** (`updateDatadogMetricStatus`, lines 227-230), which can transiently make a metric unservable right after a flip.

Worst-case convergence bound ≈ `max(resync_period, refresh_period)` = ~300s for an inactive/unreferenced-then-reactivated metric; ~30s for a continuously-active one.

## Existing coverage gap
`existing-assertions.md`: zero Antithesis SDK instrumentation. `sut-analysis.md` §9: leader-election transitions are deferred to E2E and the only failover E2E passes `restartLeader=false` (dead code). The existing catalog `extmetrics-configmap-no-lost-update` covers the **legacy ConfigMap** store RMW, not this CRD in-memory store. This property is distinct.

## Witness (must be scheduled, not just green)
Emit `assert.Reachable("dca: became leader with a non-empty DatadogMetric store")` at the isLeader-true branch of `processDatadogMetric` when `store.Count() > 0`, and `assert.Sometimes(served_fresh_after_flip, "new leader served a present non-stale HPA value within convergence bound of a flip")` from the workload after it forces a flip and queries. Without the Reachable witness firing, an `Always` staleness guard is worthless (the flip window was never opened).
