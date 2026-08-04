# configmap-concurrent-create-converges — Replicas converge on a single token and cluster ID at first boot

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P1 · **Intent:** known-defect-reproducer

**Provenance:** merged from 3 discovery agent(s): cluster-id-single-value-across-replicas, getorcreateclusterid-alreadyexists-not-swallowed-to-empty-id, cluster-id-concurrent-create-window-scheduled

## Property

Whenever two DCA replicas each successfully obtain a cluster ID via GetOrCreateClusterID (from the datadog-cluster-id ConfigMap), the two values are byte-identical. A concurrent first-boot create race may produce a spurious error on the losing replica, but it can never produce two different cluster-ID values.


## Invariant / assertion

for all pairs of successful GetOrCreateClusterID results r_a, r_b across replicas: r_a == r_b (and each == UID(namespace kube-system)) `AlwaysOrUnreachable` fits — the create branch runs only on first boot / when the ConfigMap is absent (optional path), but whenever two replicas create concurrently they must converge to a single value.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Sometimes`: At least once, both DCA replicas observed the datadog-cluster-id ConfigMap as NotFound and both attempted to Create it, i.e. one replica's Create returned AlreadyExists. This proves the concurrent-create hazard window was actually scheduled, so the convergence invariant above was tested non-vacuously.


## Antithesis angle

Schedule dca-1 and dca-2 to both execute GetOrCreateClusterID (command.go:433) before the datadog-cluster-id ConfigMap exists, against a SHARED real kube-apiserver. Both Get -> NotFound, both compute GetKubeSystemUID (common.go:59), both Create. Antithesis controls the interleave so the second Create lands after the first has committed. Assert the two persisted/returned IDs match. The value is deterministic (kube-system namespace UID, common.go:32-38), so convergence should hold structurally; this pins that no code path (e.g. a future switch to a random UUID) reintroduces divergence.


## Why it matters

The cluster ID is the cluster's stable identity used to correlate all telemetry (orchestrator, metadata, KSM). Two replicas disagreeing on it would split a single cluster into two identities in the backend. The current implementation is safe by determinism, but that safety is an emergent property of 'ID = kube-system UID', not an enforced invariant; this assertion locks it in.


## Mechanism refinement (from open-question investigation)

Convergence invariant confirmed, NOT invalidated: cluster ID = kube-system namespace UID (common.go:37), a deterministic cluster-wide constant, so two successful results are byte-identical by construction (AlwaysOrUnreachable holds structurally). Known-defect-reproducer severity raised: the create-race loser returns ("",err) — common.go:73-77 has no errors.IsAlreadyExists branch (only IsNotFound is handled on the Get) — and command.go:433-436 proceeds non-fatally with clusterID="", propagating the empty ID into the RC client, workload/cluster autoscaling, spot scheduling, and kubeactions (command.go:502-656) for the process lifetime. The loser does not cache on the error path (common.go caches only on success), so a later independent GetOrCreateClusterID would self-heal, but the in-frame consumers do not re-fetch.


## Fault dependencies

- None beyond concurrent startup of >=2 DCA replicas against a SHARED real kube-apiserver (no fake clientset). Node-termination and clock-skew NOT required.


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

Add antithesis-sdk-go to the root module and instrument common.go:GetOrCreateClusterID to emit the resolved cluster ID with a stable key per replica (e.g. assert.Always with details {replica, clusterID}). Simpler workload-only alternative: after both replicas are up, the workload reads datadog-cluster-id ConfigMap and each replica's /metadata or status-exposed cluster ID via the DCA API and asserts equality. Pair with the witness property below so a green result is not vacuous.


## Open questions (post-investigation)

- Harness verification only: does the harness co-locate both DCA replicas in the same namespace so they contend on the same datadog-cluster-id ConfigMap? Code confirms GetOrCreateClusterID uses namespace.GetMyNamespace() (own pod namespace, common.go:50), so same-Deployment replicas contend; the placement itself is a harness-config check. `(partial)`
- Does WaitForAPIClient (command.go:376) unblocking near-simultaneously for both replicas measurably widen the collision window? Code confirms both replicas gate on it and proceed to GetOrCreateClusterID at command.go:433, so it synchronizes them; the magnitude of widening is a harness measurement. `(partial)`
- In Operator/Helm deployments is datadog-cluster-id ever pre-created, making the NotFound/Create branch unreachable in production? External chart config, not in this repo. `(needs human input)`


### Investigation Log

#### Confirm the harness gives both DCAs the same namespace (namespace.GetMyNamespace()).

Examined common.go:50 `myNS := namespace.GetMyNamespace()` — the ConfigMap Get/Create target the pod's own namespace. Two replicas of one DCA Deployment share a namespace and thus the same datadog-cluster-id CM. Found: code guarantees same-namespace replicas contend; harness placement not verifiable from source. Kept partial.

#### Does any consumer persist the cluster ID elsewhere that could diverge independently?

Audited consumers: orchestrator/status.go, clustername.go:191-223 GetClusterID (node-agent reads env var from the CM or calls the DCA API), api/v1/kubernetes_metadata.go:46 getClusterID (serves it), comp/metadata/clusteragent. ALL resolve to the same datadog-cluster-id CM / kube-system UID. GetKubeSystemUID (common.go:32-38) is a fixed cluster-wide constant — no uuid.New() path. Conclusion: RESOLVED — no independent/divergent persistence; convergence holds by determinism.

#### How load-bearing is the transient empty clusterID between command.go:433 and the next cache-populating call?

Enumerated consumers of the clusterID captured by value at command.go:433: tracer global tag :466 (guarded by !=""), initializeRemoteConfigClient :502 (:851 only warns, sets empty cluster on RC client), StartWorkloadAutoscaling :572, StartClusterAutoscaling :584, StartSpotScheduling :591, kubeactions.Setup :606, and a struct field ClusterID :656. Found: start() captures clusterID once and passes it by value; on the create-race loser / error path ("") these components are initialized with an empty cluster ID for the process lifetime (no re-fetch in start()). Conclusion: RESOLVED — NOT merely a cosmetic log; RC/autoscaling/spot/kubeactions get an empty identity until restart.

#### In Operator/Helm is datadog-cluster-id ever pre-created, making the create branch unreachable?

Not determinable from agent source (external Helm/Operator chart concern). Kept needs-human.

#### Does WaitForAPIClient returning near-simultaneously for both replicas widen the collision window?

Examined command.go:376 (both replicas block on WaitForAPIClient) then command.go:433 (GetOrCreateClusterID). Found: both unblock when the shared apiserver becomes reachable, then race to Get→NotFound→Create — the shared gate synchronizes them, structurally increasing collision odds. Magnitude is a harness measurement. Kept partial (mechanism code-confirmed).


---

## Source discovery evidence (raw, per contributing agent)


### from `cluster-id-single-value-across-replicas`

## Mechanism (primary source)

`pkg/util/kubernetes/apiserver/common/common.go:43-103` `GetOrCreateClusterID`:
1. Cache hit -> return.
2. `Get(datadog-cluster-id)` in own namespace.
3. On `IsNotFound`: `clusterID := GetKubeSystemUID(coreClient)` then `Create` the ConfigMap with `Data["id"]=clusterID`.
4. On found + 36-byte value: return it.

`GetKubeSystemUID` (common.go:32-38) returns `string(kube-system namespace .UID)` — a **fixed, cluster-wide constant**. Therefore every replica that generates the ID generates the *same* value. There is no randomness (contrast: this is NOT a `uuid.New()`), so the gap's premise of "divergent cluster IDs" does not hold against this code.

## Why AlwaysOrUnreachable (not Always)

The compare only makes sense once *two* replicas have each obtained an ID; on a single-replica timeline or before both have run it, the antecedent is unreached. `AlwaysOrUnreachable` expresses "never divergent, may never be co-observed."

## Fault model

No injected faults required — only concurrent startup scheduling of the two DCA containers against the shared apiserver (topology already provides one real `kube-apiserver` + `etcd`, NOT a fake clientset). A real apiserver is essential: `fake.NewSimpleClientset` serializes calls and would not reproduce the two-process interleave (sut-analysis §9).

## Relationship to catalog

Distinct from `extmetrics-configmap-no-lost-update` (different ConfigMap, split-brain lost-update of HPA values) and from `empty-token-never-authenticates` (auth token emptiness). No existing property covers cluster-ID convergence.


### from `getorcreateclusterid-alreadyexists-not-swallowed-to-empty-id`

## Confirmed defect (code is already read to violate the property)

`common.go:73-77`:
```go
_, err = coreClient.ConfigMaps(myNS).Create(ctx, cm, CreateOptions{})
if err != nil {
    log.Errorf("Cannot create ConfigMap %s/%s: %s", myNS, defaultClusterIDMap, err)
    return "", err   // AlreadyExists is NOT distinguished, NOT retried
}
```
There is no `errors.IsAlreadyExists(err)` branch (only `IsNotFound` is handled, on the earlier Get). So the create-race loser returns `("", err)`.

`cmd/cluster-agent/subcommands/start/command.go:433-446`:
```go
clusterID, err := apicommon.GetOrCreateClusterID(apiCl.Cl.CoreV1())
if err != nil {
    pkglog.Errorf("Failed to generate or retrieve the cluster ID, err: %v", err)
}
// ... continues; clusterID is ""
pkglog.Infof("Cluster ID: %s, ...", clusterID)
```
The error is non-fatal; startup proceeds with an empty cluster ID for the remainder of the frame.

## Self-healing bound

The loser does NOT cache on the error path (common.go:78/85/101 only cache on success), so its *next* GetOrCreateClusterID re-Gets, finds the now-present ConfigMap, returns the correct 36-byte ID, and caches. Convergence (property 1) therefore still holds; the defect is scoped to the transient boot window and the spurious ERROR log.

## Precedent

The same 'transient error treated as fresh start' anti-pattern in the sibling event-checkpoint path was just fixed in commit 9c3331f2fe7 (#53752, 'Retry event checkpoint read') by adding retry-with-backoff around GetTokenFromConfigmap. The cluster-ID create race is the create-side analogue and is still unhandled.

## Intent

known-defect-reproducer: the deliverable is a trace where a replica hits AlreadyExists and command.go logs the failure + proceeds with clusterID=="". Whether to fix (add IsAlreadyExists->re-Get) is a product call; the reproducer documents it.


### from `cluster-id-concurrent-create-window-scheduled`

## What the window is

`common.go:52-77`: replica A and replica B both `Get` -> `IsNotFound` -> both build the ConfigMap and `Create`. The first `Create` to reach the apiserver wins; the second receives a `k8s.io/apimachinery/pkg/api/errors` `IsAlreadyExists` error (returned generically as `err` at common.go:74-77). The witness fires when a replica reaches that error branch.

## Reachability

Narrow window: both replicas must call GetOrCreateClusterID before either's Create commits. On simultaneous cold boot this is plausible because cluster-ID generation happens early and unconditionally in `start()` (command.go:433, step 6 of bring-up per sut-analysis §2). Antithesis's scheduler is well-suited to opening it; a plain integration test rarely would.

## Confidence

Medium: the window exists and is code-verified, but whether the two processes reach command.go:433 close enough in wall-clock depends on relative bring-up (WaitForAPIClient at command.go:376 gates both, which actually helps synchronize them — both unblock when the apiserver becomes reachable, increasing collision odds).
