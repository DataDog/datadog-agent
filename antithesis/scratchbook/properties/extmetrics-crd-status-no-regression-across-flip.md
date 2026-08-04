# extmetrics-crd-status-no-regression-across-flip — DatadogMetric status does not regress across a leadership flip

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P2 · **Intent:** invariant

**Provenance:** merged from 1 discovery agent(s): extmetrics-crd-status-no-regression-across-flip

## Property

When a newly-promoted leader writes a DatadogMetric CRD status (UpdateStatus), the persisted status Value/DataTime is never older than the status that was already present in the CRD: the IsNewerThan guard prevents a stale in-memory store (inherited from the follower's blind CRD-mirroring) from overwriting a fresher status that the previous leader had committed.


## Invariant / assertion

for every UpdateStatus call on the new leader: datadogMetricInternal.IsNewerThan(existing CRD status) == true; equivalently the committed status's Active-condition LastUpdateTime is monotonically non-decreasing across leadership flips `AlwaysOrUnreachable` fits — the CRD-status write path runs only when the DatadogMetric provider is active (optional), but whenever a status write occurs it must not regress.


## Antithesis angle

The regression window opens only if a flip is scheduled such that the new leader reconciles-and-writes before its retriever refreshes the value, using a store entry whose UpdateTime was reconstructed from the CRD by the follower path (NewDatadogMetricInternal). Antithesis can interleave the flip, the informer delivery of the last leader's UpdateStatus, and the new leader's first reconcile to probe whether IsNewerThan's 1-second (.Unix()) granularity or a reconstructed UpdateTime lets an equal/older status slip through as an update.


## Why it matters

A regressed CRD status re-published to all followers (who mirror the CRD into their stores) would propagate a stale Value cluster-wide, and HPAs reading any replica would scale on old data. IsNewerThan (datadogmetricinternal.go:192-204) gated at datadogmetric_controller.go:274 is the only thing preventing this; confirming it holds under real flip timing — and witnessing that the update path was actually exercised post-flip — is the point.


## Mechanism refinement (from open-question investigation)

Assertion refined. The IsNewerThan guard (datadogmetricinternal.go:192-204) enforces monotonicity of the Active-condition LastUpdateTime/internal UpdateTime only, at whole-second (.Unix()) granularity with a conservative `>=` tie-reject. So: (a) the safe/green assertion should target Active-condition/UpdateTime monotonicity (which the guard enforces) — NOT DataTime; (b) DataTime is stamped independently from BuildStatus (:249) and is NOT covered by the guard, so a DataTime regression is reachable when a follower mirrored a staler CRD and AutoscalerWatcher's Active-only UpdateTime bump then lets the guard pass — this is the genuine bug-probe. The '.Unix() lets an equal-second update overwrite with an older Value' hypothesis is refuted (equal-second is rejected).


## Fault dependencies

- same as the convergence property: leader_election + >=2 replicas + DatadogMetric CRD provider + stub dd-metrics-backend
- a leadership flip via apiserver partition (>=60s), leader container restart, or Lease mutation — no node-termination/clock-skew needed
- to stress the .Unix() granularity, the ability to drive two flips within a short window (workload-controlled Lease churn)


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

In updateDatadogMetric (datadogmetric_controller.go:318), before UpdateStatus, compute the existing CRD status's Active/Updated timestamps and Value and assert.AlwaysOrUnreachable(newStatus not older than existing, "ddm status write does not regress DataTime/Value"). Also assert.Reachable("ddm updateDatadogMetric executed on a post-flip new leader") tagged with the leadership epoch, so the safety assertion is only meaningful once the witness confirms the write path ran after a flip. The workload should record each committed CRD status (Value + condition timestamps) and independently assert non-decreasing DataTime across observed leadership epochs.


## Open questions (post-investigation)

- Can an informer-lagged store entry (follower mirrored a STALER CRD than the last committed status) combined with AutoscalerWatcher's Active-only UpdateTime bump (autoscaler_watcher.go:~224) make IsNewerThan pass and republish an OLDER DataTime? The guard inspects only the Active-condition timestamp, never DataTime, so it does NOT prevent this; needs the harness to schedule the lag+bump interleaving to demonstrate whether it is actually reachable. `(partial)`


### Investigation Log

#### Q1: can Active LastUpdateTime advancing while Value/DataTime stay old publish an older DataTime?

Examined autoscaler_watcher.go updateDatadogMetricStatus (bumps in-memory UpdateTime=time.Now() on Active/reference change, leaves DataTime/Value), datadogmetric_controller.go:274 IsNewerThan gate -> updateDatadogMetric, and datadogmetricinternal.go:192-204 (compares CRD Active LastUpdateTime vs in-memory UpdateTime only). Found: the Active-only bump can make IsNewerThan return true, triggering a status write carrying the OLD DataTime/Value. In the common case the store was reconstructed from the SAME CRD (follower path NewDatadogMetricInternal) so the written DataTime EQUALS the CRD's (no regression). A true OLDER-DataTime regression requires the store to have mirrored a STALER CRD version than the last committed status (informer lag) — which the guard does not protect against. Conclusion: PARTIAL -> mechanism is real and unguarded for DataTime; reachability of the lag window is a harness-scheduling question, kept partial.

#### Q2: does the 1-second .Unix() truncation permit an equal-second update to overwrite an older sub-second Value?

Examined datadogmetricinternal.go:196 — `if condition.LastUpdateTime.Unix() >= d.UpdateTime.Unix() { return false }`. Found: equal-second is REJECTED (>=), so no write occurs within the same second; the truncation is conservative on ties. Conclusion: RESOLVED -> it cannot overwrite with an older sub-second value; the only failure mode from the truncation is a SKIPPED legitimately-newer sub-second update (a missed update, not a regression).

#### Q3: is monotonicity the right notion given DataTime and UpdateTime are distinct?

Examined IsNewerThan (uses internal UpdateTime = status-write time, via the Active condition) vs BuildStatus:249 (Updated condition stamped from DataTime). Found: the guard enforces monotonicity of the Active-condition LastUpdateTime / internal UpdateTime, NOT of DataTime. Conclusion: RESOLVED -> the invariant as stated ('Active-condition LastUpdateTime non-decreasing') is exactly what the guard guarantees and is the correct thing to assert; DataTime-non-regression is a SEPARATE, unguarded property that is the real bug-probe (ties to Q1).


---

## Source discovery evidence (raw, per contributing agent)


### from `extmetrics-crd-status-no-regression-across-flip`

## Guard under test
`syncDatadogMetric` writes status only when `datadogMetricInternal.IsNewerThan(datadogMetric.Status)` (`datadogmetric_controller.go:274-277`). `IsNewerThan` (`datadogmetricinternal.go:192-204`) compares the CRD's **Active** condition `LastUpdateTime.Unix()` against the in-memory `UpdateTime.Unix()`; `>=` ⇒ returns false ⇒ no write. Granularity is **whole seconds** (`.Unix()`), and only the Active condition is consulted.

## Why a flip stresses it
After a flip the new leader's store entry for a metric was last written by the **follower path** (`datadogmetric_controller.go:220`) via `NewDatadogMetricInternal`, which sets `UpdateTime` from the CRD's Active-condition `LastUpdateTime` (`datadogmetricinternal.go:81-83`) or, if unset, to `time.Now()` (lines 95-97). So immediately post-flip `internal.UpdateTime == CRD Active LastUpdateTime` ⇒ `IsNewerThan` returns false ⇒ no regression write. The invariant is expected to HOLD; the value is a green AlwaysOrUnreachable **paired with a Reachable witness** proving the new leader actually reached the `updateDatadogMetric` call after a flip (otherwise the guard was never exercised).

## Antithesis-reachable stressors
- Sub-second flips (two flips within one wall-clock second) so `.Unix()` collapses distinct updates and could permit an equal-timestamp overwrite.
- A metric whose in-memory UpdateTime was bumped to `time.Now()` by AutoscalerWatcher's Active-toggle (`autoscaler_watcher.go:224`) right after a flip, making the new leader *think* it is newer than the CRD and issue a write with a possibly-older Value/DataTime.

## Distinctness
Complements the convergence/liveness property above and is unrelated to the legacy-ConfigMap `extmetrics-configmap-no-lost-update` entry.
