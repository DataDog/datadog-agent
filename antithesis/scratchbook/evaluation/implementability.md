---
sut_path: /Users/jon.rosario/go/src/github.com/DataDog/datadog-agent
commit: f2da1471bb748fb5108f89f36f7b83cab305ca79
updated: 2026-07-21
external_references: []
---

# Implementability Evaluation — DCA Antithesis Property Catalog

Lens: for each property, can the invariant be **observed** (from the workload or from
feasible SUT-side instrumentation), and can the **precondition be constructed**
reliably in the topology of `deployment-topology.md`? Biased toward finding what
the catalog has *not* fully accounted for.

Ground truth spot-checked against source at the pinned commit:
`cmd/cluster-agent/api/server.go`, `cmd/cluster-agent/api/v1/clusterchecks.go`,
`pkg/clusteragent/clusterchecks/{stores.go,stats.go,handler.go}`,
`pkg/clusteragent/api/leader_forwarder.go`,
`pkg/clusteragent/autoscaling/externalmetrics/metrics_retriever.go`,
`pkg/util/kubernetes/apiserver/leaderelection/leaderelection_engine.go`.

---

## Catalog-wide implementability concerns

### C1. Most P0/P1 invariants are in-process, unexported, and Linux/clusterchecks-gated

The five P0 properties (`dispatch-implies-lease-holder`, `dispatch-store-bijection`,
`leader-eventually-dispatches-after-warmup`, `store-lock-bounded-under-slow-clc`,
`leadershipchan-no-wedge-under-lock`) plus most of the store-integrity P1s assert on
state that is **not reachable from the workload**:

- `clusterStore` fields (`digestToConfig`, `digestToNode`, `nodes`, `danglingConfigs`,
  `active`) are unexported and live behind an anonymous `sync.RWMutex`
  (`stores.go:23-31`).
- `Handler.state` / `Handler.m`, `leadershipChan`, `ksmShardedConfigs`,
  `dispatcher.advancedDispatching` are all unexported.
- Every one of these files carries `//go:build clusterchecks` (and the package is
  `//go:build !windows && kubeapiserver`).

Consequences the catalog under-states:
- All of this is **net-new instrumentation** in a custom DCA image (confirmed: zero
  existing SDK usage). `deployment-topology.md` §SDK selection says this once; the
  per-property "Invariant" rows repeatedly describe assertions as if drop-in, without
  flagging that each requires editing unexported internals inside a build-tag-gated,
  Linux-only tree.
- **Cannot be compile-checked on the macOS dev host** (clusterchecks/kubeapiserver are
  Linux-only build tags; matches the known "macOS skips linux&&nvml compile" caveat).
  Instrumentation correctness can only be validated in a Linux build / the image build.
- The SDK must be added to the **root module** `github.com/DataDog/datadog-agent`
  go.mod. That module is enormous; adding a dependency there is a heavier lift than
  "add to whichever module hosts the code" implies.

Feasibility verdict: doable, but the catalog should label these properties
"requires instrumented DCA image" as a first-class prerequisite, not a footnote.

### C2. Cross-replica / cross-process invariants have no single-process expression, and the natural external observation point is auth-locked

Several headline properties are inherently **multi-process**:
- `dispatch-implies-lease-holder` corollary: "at most one replica reports
  state==leader at a time."
- `extmetrics-configmap-no-lost-update`: "two replicas both believing they are leader
  both write."
- split-brain framing generally.

Antithesis SDK assertions fire **per process** and are aggregated globally by
assertion id — there is no built-in "conjunction across replicas" primitive. So these
corollaries need either (a) the **workload** to observe both replicas simultaneously,
or (b) a shared external signal.

The natural external observation point defeats (a): the dispatch-state route
`GET /api/v1/clusterchecks` (registered `clusterchecks.go:28`, returns
`StateResponse`/`Stats` with `Leader`/`Active`) is classified **non-external** by
`isExternalPath` (`server.go:199-219`): the path `/api/v1/clusterchecks` splits into 4
segments, and the classifier only treats `/api/v1/clusterchecks/` with **6** segments
as external. Non-external ⇒ it requires the **local IPC token**
(`ipc.GetAuthToken`), which is **per-pod**, not the cluster-shared DCA auth token the
workload can hold. So a workload impersonating node agents **cannot scrape per-replica
dispatch/leader state** to build the "at most one dispatcher" corollary.

Net: the cross-replica half of the single most important property (and the split-brain
observation for lost-update) is **not workload-observable as described**. It needs
SUT-side instrumentation that exports each replica's "I am actively dispatching" fact
to a globally-aggregated assertion (e.g. an `assert.Always(!(dispatching && !IsLeader))`
in-process, plus a workload-visible counter), or the harness must mint a shared IPC
token for the workload — neither is in the catalog.

### C3. Harness can *tune* the SUT to make flap preconditions reachable — catalog leaves this as an open question rather than a decision

`LeaseDuration = leader_lease_duration` (config-driven, no floor beyond `>0`), and
`RenewDeadline`/`RetryPeriod` are derived from it (`leaderelection_engine.go:200-202`);
`warmup_duration` is config-driven (`handler.go:72`). Only `leaderStatusFreq` (1s) is
hardcoded (`handler.go:71`). This means the many flap-dependent properties
(`leader-eventually-dispatches-after-warmup`, `leadershipchan-no-wedge-under-lock`,
`dispatch-implies-lease-holder`, `store-lock`, `reset-restores`) can be made to flap
**deterministically by lowering `leader_lease_duration` in the DCA config**, without
needing clock-skew or node-termination faults. The catalog repeatedly lists this as an
open question ("whether client-go can realistically deliver transitions faster than
30s") instead of committing the harness to a short lease. This is an
implementability *enabler* the catalog should adopt, and it materially reduces the
number of properties that go vacuous under default faults.

### C4. Default-disabled faults leave a real fraction of the catalog with unconstructible preconditions

The catalog flags this, but from the implementability lens the impact is larger than
"marked inert": the workload **cannot construct the precondition at all** for these
without a tenant fault-config change:
- `node-expiry-monotonic-clock` — fully inert; needs backward clock skew. No workload
  substitute exists (leader stamps heartbeat on receipt using its own wall clock).
- `kubeactions-at-most-once` crash-replay variant, `new-leader-elected-after-loss`
  crash variant, `autoscaling-fatal-startup-crashloop` restart — need node
  termination.
- `lastconfigchange-monotonic-epoch` — primary trigger is backward clock skew.

Partial mitigations the catalog misses: split-brain (partition, enabled by default)
gives the *two-leaders* path for `kubeactions-at-most-once` without node termination;
and `new-leader-elected-after-loss` works via partition alone. So only clock-skew
properties are truly unconstructible. Worth separating "needs node termination (has a
partition substitute)" from "needs clock skew (no substitute)."

---

## Property-specific implementability findings

### dispatch-implies-lease-holder (P0)
- In-process half `assert.Always(!(state==leader && !IsLeader()))` is cleanly
  implementable inside the clusterchecks handler (both `h.state` and the global
  `LeaderEngine` are reachable in-process). Good.
- Cross-replica "at most one dispatches" — see **C2**; not workload-observable via
  `GET /clusterchecks` (auth-locked to the per-pod local token). Needs a dedicated
  exported signal. Catalog says "asserted from the workload" without accounting for
  the auth classification.
- Precondition (force follower `GetLeaderIP()==""` during a lease gap) hinges on the
  unresolved client-go question (§11) of whether a follower ever sees empty
  holderIdentity; the workload owns EndpointSlices, so it *can* force
  `GetLeaderIP()==""` via the Service-endpoint resolution path (lagging/empty
  EndpointSlice) independent of client-go — this is a stronger, controllable lever the
  catalog under-uses.

### store-lock-bounded-under-slow-clc (P0)
- `clusterStore` embeds an **anonymous** `sync.RWMutex` (`stores.go:24`). "Timing
  instrumentation around Lock/Unlock" is not a small edit: you must replace the
  embedded field with a custom timed-mutex type (transparent because `Lock/Unlock`
  stay methods) or wrap every call site. Feasible, but more invasive than stated.
- Precondition is fully workload-constructible: the `clc-runner` stub can answer
  `GetRunnerStats`/`GetRunnerWorkers` slowly / be partitioned. Good. Requires the CLC
  stub container + `advanced_dispatching` enabled.

### leadershipchan-no-wedge-under-lock (P0)
- "Send never blocks under `h.m`" is not directly observable. Implementable as an
  in-process `assert.Always(len(leadershipChan) < cap)` immediately before the send in
  `updateLeaderIP` (both are unexported, reachable in-package). Catalog acknowledges
  the observation constraint. OK, but note the assertion is a *proxy* for blocking, not
  the blocking itself.
- Precondition (two self-transitions inside one warmup window) is questionable at the
  default 60s lease; **C3** short-lease config is the realistic enabler. Without it the
  property likely passes vacuously.

### leader-eventually-dispatches-after-warmup (P0)
- `store.active` flipping true is in-process; can also be inferred by the workload from
  `Stats.Active` **if** it could read `GET /clusterchecks` — but that route is
  auth-locked (C2), so realistically this is an in-process `Sometimes`. Fine.
- Precondition (flap near warmup) needs C3 short lease/warmup tuning.

### dispatch-store-bijection (P0)
- Validator over all store maps is implementable in-package under `d.store` lock at
  mutator tails. Good. Must special-case `endpointsConfigs` (node-pinned 1:1) — the
  catalog's own open question; without the carve-out the invariant will false-positive.
- Precondition (node expiry + reset/re-acquire) constructible via workload heartbeat
  timing + partition. Backward-skew amplifier is default-disabled but not required.

### reset-restores-store-and-gauges (P1)
- **Mixed observability the catalog misses (a strength):** the gauges
  (`nodeAgents`, `dispatchedConfigs`, dangling/unscheduled, KSM shard map) are
  Prometheus metrics on the plaintext `metrics_port` with **no auth** — the workload
  can scrape `/metrics` and assert they return to ground truth across a leadership
  flap, no SUT instrumentation needed for that half. The "store maps empty" half is
  in-process. The catalog treats the whole property as SUT-instrumented; the gauge half
  is workload-only and cheaper.

### advanced-dispatching-node-set-integrity (P1)
- Precondition is fully workload-constructible: a NodeAgent-typed heartbeat (`node_type:2`
  in the POST body) and an empty-`X-Real-Ip` heartbeat are both header/body choices the
  workload controls. Good.
- But the observed state — `dispatcher.advancedDispatching` (atomic) and the node set —
  is in-process only; `Stats` exposes no `advancedDispatching` field (`stats.go`), so
  the invariant needs SUT instrumentation. Catalog implies observability that the stats
  API does not provide.

### forwarder-ip-proxy-consistency (P1)
- Confirmed the bug shape: `SetLeaderIP("")` nils `proxy` and **returns before**
  clearing `lf.leaderIP` (`leader_forwarder.go:112-118`), so `GetLeaderIP()` returns a
  stale IP with `proxy==nil`. `SetLeaderIP`/`GetLeaderIP` are methods on the
  `LeaderForwarder` singleton in `pkg/clusteragent/api` (kubeapiserver-gated, not
  clusterchecks-gated) — instrumentation feasible.
- Invariant `(proxy==nil) iff (leaderIP=="")` is in-process only (both fields
  unexported); not workload-observable. Precondition (drive `GetLeaderIP()==""` +
  interleave two writers) constructible via workload-owned EndpointSlice manipulation.

### forwarder-target-is-live-endpoint (P1)
- Catalog lists **node termination (disabled)** as required to reschedule the leader to
  a new IP. But per `deployment-topology.md` the **workload owns the DCA Service's
  Endpoints/EndpointSlice** — it can point the endpoint at a dead/stale IP directly,
  constructing the stale-cache precondition **without** node termination. The "requires
  node termination" dep is overstated; flag as avoidable.
- Observing "dial target ∈ current endpoint set" needs instrumentation at the
  `ReverseProxy` Director (in-process) or workload observation of where the forwarded
  request lands (workload can run a decoy at the stale IP and detect delivery).
  Workload-observable variant exists and is cheaper than catalog implies.

### extmetrics-backoff-cap-stays-serving (P2)
- **Reachability likely unattainable in a timeline.** `backoffPolicy` is a hardcoded
  package var `NewExpBackoffPolicy(2, 30, 1800, 2, false)`
  (`metrics_retriever.go:29`), not config-driven. Reaching the 1800s cap takes ~7
  increments (30,60,120,…,1800) ⇒ ~60+ minutes of sustained partition. An Antithesis
  timeline is often shorter, so `assert.Reachable(backoff_reached_cap)` may never fire.
  It is a package `var` (not `const`), so an instrumented build can override it — but
  that is net-new and unmentioned. Flag: either shorten the constant in the test build
  or accept the property is inert in short runs.
- "Stays serving / marks stale" half is workload-observable (HPA APIService responds,
  metrics flagged stale). OK.

### extmetrics-configmap-no-lost-update (P1)
- The RMW-without-resourceVersion path is real (`store_configmap.go` per SUT). The
  "two replicas both write" precondition is a split-brain scenario — see C2 for the
  cross-process observation problem. The workload can, however, observe the *effect*
  (HPA metric value regressing/flip-flopping) by reading the external-metrics
  APIService — a workload-observable consequence assertion is available and the catalog
  should lead with it rather than the in-process write-path assertion.
- Provider pinning (legacy ConfigMap vs DatadogMetric CRD) is a required config the
  harness must set; if unset the path is unreachable.

### kubeactions-at-most-once (P1)
- Two-leader path is constructible via partition alone (no node termination needed) —
  catalog lists node termination as required for crash-replay but the split-brain
  variant is available by default. Observing "executed at most once" requires
  instrumenting the executor or a workload-visible mutation counter (the mutating
  targets are cluster objects the workload can watch via the apiserver — so
  workload-observable). Requires remote-config client + the RC redelivery contract
  (out of SUT scope) to actually redeliver; if the RC stub never re-pushes, the
  precondition can't be built. Needs an RC-server stub that redelivers — not in the
  topology inventory.

### empty-token-never-authenticates (P1)
- Compare site (`util.TokenValidator`, `server.go`) is instrumentable. But the
  precondition — token momentarily empty while the API server already accepts — is
  timing-fragile and may be unreachable if token load is ordered before `StartServer`.
  Needs FS/IO fault on the token file during startup (an IO-fault the tenant must
  support). Reliability of constructing the window is low; likely a rare/never trigger.

### no-404-on-registered-cluster-check-routes (P1)
- Workload-observable (send `GET /api/v1/clusterchecks/configs/{id}` during startup,
  check for literal `"404 page not found"` vs 503). Note the workload must present the
  **DCA token** to get past `validateToken` before reaching the mux (the route is
  6-segment external — OK). Precondition (widen the WaitForAPIClient gap) needs
  apiserver latency, default-enabled. Solid.

### isexternalpath-classifier-consistency (P2)
- Pure input-domain, workload-constructible (send each route with each token + trailing
  slash / extra segment variants). But the assertion needs a **canonical per-route
  auth-class oracle** to compare against; `isExternalPath` is the only source of truth,
  so asserting "classifier matches intended class" risks being tautological unless the
  catalog supplies an independent expected-class table. The catalog's own open question
  flags this — it is a genuine implementability blocker, not just a caveat. Concrete
  seed: the state route `/api/v1/clusterchecks` (getState) is classified non-external
  (4 segments ≠ 6) and therefore requires the local IPC token — is that intended? This
  is a ready-made test vector.

### getconfigs-distinguishes-unknown-node / node-expiry-monotonic-clock / graceful-shutdown / admission-webhook-no-silent-nil-cert / lastconfigchange-monotonic-epoch
- `getconfigs-distinguishes-unknown-node`: workload-observable (poll GET for an unknown
  node, inspect status code) — easy; but it is a "claim-to-improve" (today returns 500),
  so the assertion is really asserting a *desired* future behavior, which will fail on
  current code by design. Fine if intended as a finding, but it is not an invariant of
  the current SUT.
- `node-expiry-monotonic-clock`, `lastconfigchange-monotonic-epoch`: inert without
  clock skew (C4); no workload substitute.
- `graceful-shutdown-releases-lease-bounded`: observable by the workload watching the
  Lease object (apiserver) for release timing after SIGTERM; needs SIGTERM delivery to a
  specific container (Antithesis can stop containers) + partition. Constructible.
- `admission-webhook-no-silent-nil-cert`: `GetCertificate` returning `(nil,nil)` is
  in-process; a workload could observe the TLS handshake failure by acting as the
  apiserver hitting the webhook, but distinguishing "nil cert swallowed" from other
  handshake failures needs SUT instrumentation at the callback. Requires
  `admission_controller.enabled` + a webhook client in the topology (not in the
  inventory) — added container needed.

---

## Summary of instrumentation/topology gaps not in the catalog

1. A globally-aggregated **per-replica "am dispatching" signal** (C2) — required for the
   #1 property's cross-replica corollary; `GET /clusterchecks` is auth-locked.
2. **Custom timed-mutex** on `clusterStore` for the lock-hold property (not a trivial
   wrap; anonymous embedded mutex).
3. **Override of the hardcoded `backoffPolicy` var** to make the 1800s-cap property
   reachable within a timeline.
4. **RC-server stub that redelivers** K8S_ACTIONS for `kubeactions-at-most-once`
   (absent from the container inventory).
5. **Admission-webhook client container** for `admission-webhook-no-silent-nil-cert`
   (absent from the inventory).
6. Commit the harness to a **short `leader_lease_duration`** (C3) rather than leaving
   flap-reachability as an open question.
7. An **independent expected-auth-class table** for
   `isexternalpath-classifier-consistency` (else tautological).
