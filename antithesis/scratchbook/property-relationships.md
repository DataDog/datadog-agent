---
sut_path: /Users/jon.rosario/go/src/github.com/DataDog/datadog-agent
commit: f2da1471bb748fb5108f89f36f7b83cab305ca79
updated: 2026-07-21
external_references: []
---


# Property Relationships

Clusters sharing code paths, failure mechanisms, or dominance. Lightweight — connections noticed during synthesis and evaluation.


## The leadership-divergence core (root cause)

Properties: `dispatch-implies-lease-holder`, `dispatch-store-bijection`, `no-duplicate-check-execution-fencing`, `leadershipchan-no-wedge-under-lock`, `kubeactions-at-most-once`, `extmetrics-configmap-no-lost-update`, `extmetrics-crd-status-no-regression-across-flip`, `forwarder-ip-proxy-consistency`, `forwarder-target-is-live-endpoint`

All trace to the `GetLeaderIP()==""` overload / three-notions-of-leadership hazard. **Dominance:** `dispatch-implies-lease-holder` is the root — if two replicas lead, then `no-duplicate-check-execution-fencing`, `dispatch-store-bijection`, `kubeactions-at-most-once`, and both external-metrics-write properties fail as downstream consequences. Establish at-most-one-leader first.


## Duplicate execution: store-consistent vs reality-consistent

Properties: `dispatch-store-bijection`, `no-duplicate-check-execution-fencing`, `duplicate-execution-window-bounded-after-heal`, `dangling-eventually-redispatched`

`dispatch-store-bijection` asserts the store is internally consistent; it is *correct by design* while two node agents run the same check (expired-but-alive node). `no-duplicate-check-execution-fencing` (workload-witnessed) closes exactly that gap — the guarantee lives between store-consistency and reality-consistency. `duplicate-execution-window-bounded-after-heal` is the liveness bound on how long that window persists.


## Dispatch store lifecycle & regression cluster

Properties: `dispatch-store-bijection`, `reset-restores-store-and-gauges`, `dangling-redispatch-no-resurrect`, `ksm-shard-tracking-consistency`

Share the `clusterStore` reset()/dangling machinery and a common fix history (#52876/#52078/#50715). `dispatch-store-bijection` subsumes much of dangling-no-resurrect; kept separate for independent failure attribution.


## Warmup & dispatch progress

Properties: `leader-eventually-dispatches-after-warmup`, `dangling-eventually-redispatched`, `new-leader-elected-after-loss`, `rebalance-cycle-terminates`

Progress properties gated on a stable leader. Flapping (partition at ~warmup/lease boundary) is the shared adversary; `new-leader-elected-after-loss` is the precondition for all dispatch progress.


## Lock-hold / liveness-probe feedback loop

Properties: `store-lock-bounded-under-slow-clc`, `leadershipchan-no-wedge-under-lock`, `liveness-probe-no-restart-loop`, `graceful-shutdown-releases-lease-bounded`

A blocked lock or channel send stops the health-probe drain. `liveness-probe-no-restart-loop` is the shared observable *consequence* of the two lock hazards; `graceful-shutdown` is the shutdown-path analogue (a hung network call under partition).


## Rebalance

Properties: `rebalance-cycle-terminates`, `rebalance-no-perpetual-thrash`, `store-lock-bounded-under-slow-clc`, `advanced-dispatching-node-set-integrity`

The freshly-landed utilization rebalancer (#52884). Termination (liveness) + no-thrash (safety) under stale/unreachable-runner busyness; shares the CLC-runner-stats path (and its store-lock hazard) and the advanced-dispatching node set.


## Node-set / clock integrity

Properties: `node-expiry-monotonic-clock`, `advanced-dispatching-node-set-integrity`, `dispatch-store-bijection`

Wall-clock node expiry (mass expiry under clock skew) and a poisoned node set (advanced-dispatching latch / empty IP) both cascade into bijection violations and dangling churn.


## Follower forwarding & the forwarded trust boundary

Properties: `forwarder-ip-proxy-consistency`, `forwarder-single-hop-loop-cap`, `forwarder-target-is-live-endpoint`, `forwarder-request-fidelity`, `empty-token-never-authenticates`, `isexternalpath-classifier-consistency`

The follower→leader proxy and the auth of forwarded/served requests. Right destination (`target-is-live`) + right path/creds (`request-fidelity`) + right authorization (`empty-token`, `isexternalpath`) together bound the forwarded trust boundary.


## Premature-Serve() / first-boot startup window

Properties: `no-404-on-registered-cluster-check-routes`, `empty-token-never-authenticates`, `configmap-concurrent-create-converges`

The API server accepts connections at command.go:368 before routes/token/apiserver are ready, and dca-1/dca-2 boot simultaneously before any leader exists. Once the listener accepts, every gating prerequisite must be ready or the response is an honest retryable 503 — never a 404, never auth-against-empty-token, never a divergent token/cluster-ID. Needs only concurrent startup — Antithesis's core competency.


## gRPC streaming data plane

Properties: `grpc-stream-subscription-accounting`

The tagger + kube-metadata streams share one subscription-registry accounting pattern (the #48026/#50670 reconnect race); folded into one property covering both streams with a paired overlap witness.


## Informer freshness (root cause + symptoms)

Properties: `informer-fresh-or-staleness-surfaced`, `forwarder-target-is-live-endpoint`, `admission-webhook-no-silent-nil-cert`, `extmetrics-crd-store-converges-after-flip`

`informer_client_timeout=0` silently freezes caches under a watch-drop partition. `informer-fresh-or-staleness-surfaced` is the cheap root-cause assertion; stale endpoints (forwarder), stale cert (admission), and stale CRD store are its downstream symptoms.


## External metrics / HPA

Properties: `extmetrics-configmap-no-lost-update`, `extmetrics-crd-store-converges-after-flip`, `extmetrics-crd-status-no-regression-across-flip`, `extmetrics-backoff-cap-stays-serving`

Two provider paths (legacy ConfigMap lost-update vs DatadogMetric CRD divergence) plus degraded-mode serving under backend outage. Provider-mutually-exclusive per run.


## Admission availability

Properties: `admission-webhook-available-under-churn`, `admission-webhook-no-silent-nil-cert`

Webhook availability under fault (fail-closed blast radius) and the silent-nil-cert path that can trip it. Both DCA-side; the apiserver-enforced fail-closed portion is out of scope.


## Coverage check

- Every referenced slug exists in the catalog: YES
- Standalone (in no cluster): autoscaling-fatal-startup-crashloop, getconfigs-distinguishes-unknown-node