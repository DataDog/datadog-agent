---
sut_path: /Users/jon.rosario/go/src/github.com/DataDog/datadog-agent
commit: f2da1471bb748fb5108f89f36f7b83cab305ca79
updated: 2026-08-04
external_references: []
---

# Deployment Topology — Datadog Cluster Agent

## Instrumentation decision: no coverage instrumentation / static cataloging (2026-08-04)

**Tried and abandoned:** `antithesis-go-instrumentor`'s default mode requires
`go_project_dir` to load and type-check cleanly as one unit — for this repo that
means the **module root** (~11,400 Go files across every binary: agent,
system-probe, security-agent, trace-agent, cluster-agent, …), not just
cluster-agent's dependency subgraph. Coverage instrumentation alone succeeded
(11,369 files, ~2:47 wall time run locally, natively). Assertion cataloging did
not: a handful of unrelated packages elsewhere in the module fail to load (a
missing generated rtloader CGO header, one stale benchmark, several
build-tag-excluded network test-utility dirs) and the instrumentor's
`go/packages.Load` treats any error anywhere in the whole-module load as fatal to
the *entire* catalog — "Assertion catalogs will not be generated," 0 catalogs
written. Deleting the broken leaf packages from a scratch copy just moved the
failure up a level (other unrelated packages import them, and `go mod tidy` then
tries to resolve the now-missing import paths as external modules and 404s
against Datadog's internal proxy). This is not a fixable-in-one-pass problem
without a much larger audit of the whole monorepo's buildability outside CI.

**Resolution:** `blt/antithesis-harness` (PR #51515), an existing Antithesis
effort on the logs-agent in this same repo, hit the identical problem and settled
on the same answer: skip the instrumentor. Link the Antithesis SDK directly and
call `assert.*`/`lifecycle.*` by hand at the sites that matter; build with a
plain `CGO_ENABLED=1 go build -tags ...` (no `dda inv`, no Bazel — `go_build()` in
`tasks/libs/common/go.py` is itself a plain `go build`; Bazel is only used by
`dda inv tidy` for dependency/BUILD-file bookkeeping). `antithesis/dca.Dockerfile`
follows this pattern. **Accepted cost:** no coverage-guided fuzzing feedback, no
pre-run "assertion never reached" catalog. **What's kept:** every `assert.*` call
(the bootstrap property, and every property `antithesis-workload` adds) still
fires and reports correctly at runtime — cataloging is a separate, additive
static-analysis layer, not a prerequisite for assertions to work.

Verified end-to-end: the built image runs unmodified DCA leader-election code
against the harness's bare kube-apiserver — real Lease acquisition, real
single-leader election across 2 replicas (see the setup doc for the full log).

## Deferred decisions (for antithesis-setup)

The following harness/deployment-config questions surfaced during property open-question investigation are
explicitly **deferred to `antithesis-setup`** (user decision, 2026-07-21) rather than pinned now — they are
naturally settled when writing the actual compose/Helm config, not from a markdown research pass:

- Admission webhook Service selector (all DCA pods vs leader-only) and `admission_controller.failure_policy`
  (Ignore vs Fail) — see `admission-webhook-available-under-churn`, `admission-webhook-no-silent-nil-cert`.
- Deployment kind (StatefulSet vs Deployment) — affects severity of the leader-forwarder stale-IP/pod-reuse
  hazards (`forwarder-target-is-live-endpoint`, `forwarder-ip-proxy-consistency`).
- Whether to request clock-skew and/or node-termination faults be enabled for this tenant (both commonly
  disabled by default) — `node-expiry-monotonic-clock` is fully inert without clock skew; several
  crash-replay variants (`kubeactions-at-most-once`, `new-leader-elected-after-loss`) are strengthened by
  node termination but have a partition-only primary path.
- `cluster_checks.rebalance_period` tuning (default 10m) to make `store-lock-bounded-under-slow-clc` and the
  rebalance-convergence properties reachable without an artificially long run.
- Whether `warmup_duration` should be set below `RenewDeadline` to widen the `leader-eventually-dispatches-
  after-warmup` flap-interruption window, or left at the equal-by-default value.
- `datadog-cluster-id` ConfigMap pre-creation (Operator/Helm charts sometimes pre-create it, which would make
  `configmap-concurrent-create-converges`'s create-race branch unreachable) — confirm the harness does NOT
  pre-create it, so the race is actually exercised.

## Committed decisions (from user review)

- **Custom DCA image, built up front — DONE.** The `dca-*` containers run an image built from a tree
  with the Antithesis Go SDK added to the root `github.com/DataDog/datadog-agent` module (see
  `cmd/cluster-agent/subcommands/start/command.go`'s bootstrap `assert.Reachable`), so `assert.*` calls
  are usable from the first run. Built via plain `go build` (see "Instrumentation decision" above) inside
  a Linux container — `antithesis/dca.Dockerfile`. Verified end-to-end: 2-replica Lease-based leader
  election runs correctly against the harness's bare kube-apiserver (see `antithesis/README.md`).
- **SUT-side export of per-replica "am-dispatching" state**, aggregated by the workload for the
  cross-replica split-brain assertion (rather than minting a shared IPC token).
- **Short lease as a fixed harness setting.** Pin a short `leader_lease_duration` (and matching
  `cluster_checks.warmup_duration`) so flap-dependent preconditions are deterministic *without*
  clock-skew or node-termination faults. Derived timings follow: `RenewDeadline = lease/2`,
  `RetryPeriod = lease/4`.
- **Orchestrator is out of scope** — no container for it.

## Design driver

The DCA is a leader-elected singleton whose entire behavior is mediated by the
Kubernetes API server (Leases, ConfigMaps, Endpoints/EndpointSlices, informers,
CRDs). Two consequences dictate the topology:

1. **The kube-apiserver must be its own container**, because the catalog's
   highest-value fault is *partitioning a DCA replica from the apiserver* (drives
   split-brain, leadership loss, stale caches). Antithesis faults are
   container-level, so anything we want to fault independently must be its own
   container.
2. **At least 2 DCA replicas in separate containers**, because leader election,
   split-brain, and follower-forwarding properties are inert with a single
   replica. `leader_election` must be enabled (global default is `false`).

Everything else is added only if a property needs it. `sut-analysis.md` and
`property-catalog.md` are the inputs; the fault→container mapping at the end ties
each property cluster to the containers it exercises.

## Container inventory

### Required core

| Container | Role | Image | Runs | Connects to | Replicas |
|---|---|---|---|---|---|
| `kube-init` | one-shot setup | `alpine:3.20` (`antithesis/config/docker-compose.yaml` inline command) | generates a self-signed CA, apiserver serving cert, service-account signing keypair, and a static `--token-auth-file` bearer token; writes both into shared volumes before `kube-apiserver` starts | writes to `k8s-certs`/`sa-token` volumes | 1 (runs once, exits) |
| `etcd` | dependency | `registry.k8s.io/etcd:3.5.16-0` | etcd, backing store for the apiserver | apiserver | 1 |
| `kube-apiserver` | dependency | **built**: `antithesis/kube-apiserver.Dockerfile` (repackages `registry.k8s.io/kube-apiserver:v1.31.1`'s binary onto Alpine — the official image is distroless and has no shell, so Compose healthchecks can't exec inside it) | a bare kube-apiserver: `--authorization-mode=AlwaysAllow` + `--token-auth-file` (AlwaysAllow force-disables `--anonymous-auth`, so real bearer-token auth is required, not optional) | etcd; serves DCAs + workload | 1 |
| `dca-1`, `dca-2` | service (SUT) | **built**: `antithesis/dca.Dockerfile` (plain `go build`, no `dda inv`/Bazel/instrumentor — see "Instrumentation decision" above) | the cluster-agent `start` command, `leader_election: true` + `leader_election_default_resource: lease` (the code defaults to `configmap` — verified this must be set explicitly on **every** replica identically, or replicas elect via different lock objects and both become leader) | kube-apiserver; each other (follower→leader forwarder on cmd_port 5005) | 2 (min); 3 for richer elections |
| `workload` | client (test driver) | placeholder (`debian:stable-slim`, `tail -f`) — real workload is `antithesis-workload`'s job | impersonates N node agents over the DCA HTTP API, seeds/mutates cluster-check AD configs, manages the DCA Service + EndpointSlice objects, drives leadership events, and emits assertions | kube-apiserver (to create objects), both DCAs (HTTP API) | 1 (may split, below) |

Node agents are **not** run as real `datadog-agent` processes — the node-agent
side is out of SUT scope. The `workload` driver speaks the DCA's HTTP contract
directly (`POST /api/v1/clusterchecks/status/{id}`, `GET
/api/v1/clusterchecks/configs/{id}`, endpoints-checks), impersonating multiple
node identities. This is lighter and gives the workload full control over
heartbeat timing (the input that drives node expiry, duplicate dispatch, etc.).

### Conditional (add per property group)

| Container | Needed by | Image | Notes |
|---|---|---|---|
| `clc-runner` | `store-lock-bounded-under-slow-clc`, `advanced-dispatching-node-set-integrity`, rebalance/utilization properties | **stub** Go image answering the CLC-runner stats API (`GetRunnerStats`/`GetRunnerWorkers`) and registering as a CLC-runner-typed node | A real CLC runner is heavy; a stub that serves the stats HTTP endpoints and heartbeats is sufficient and lets the workload inject slow/partitioned-runner faults. 1–2 replicas. |
| `dd-metrics-backend` | `extmetrics-configmap-no-lost-update`, `extmetrics-crd-*`, `extmetrics-backoff-cap-stays-serving` | **stub** HTTP server answering `/api/v1/query` with canned/controllable series | Only when external metrics are enabled (`external_metrics_provider.enabled: true`). Lets the workload inject 429/500/latency. **Provider decision (user, 2026-07-21): pin `external_metrics_provider.use_datadogmetric_crd: true`** — the harness targets the DatadogMetric CRD store as primary; `extmetrics-crd-store-converges-after-flip` and `extmetrics-crd-status-no-regression-across-flip` are the primary properties for this slice. `extmetrics-configmap-no-lost-update` (legacy ConfigMap path) is deprioritized to a secondary/optional run, not the default. |
| `rc-server` | `kubeactions-at-most-once` | **stub** remote-config backend with controllable (re)delivery of `K8S_ACTIONS` configs | Needed to drive the KubeAction redelivery precondition (duplicate-execution across restart). The split-brain variant needs only a partition, not this stub. |
| `webhook-client` | `admission-webhook-*` | a client that performs TLS handshakes / AdmissionReview POSTs against the DCA webhook (or the workload acting as the apiserver) | No such client exists by default; required to exercise the admission webhook path and observe fail-open/closed behavior. |
| `fakeintake` | *(none in current catalog)* | `test/fakeintake` image | Not required: the catalog asserts DCA-internal state and API responses, not emitted intake payloads. Add only if a future property observes DCA→Datadog payloads. |

## Topology diagram (required core + CLC stub)

```text
                         +---------------------+
                         |   workload / driver |  (Antithesis SDK, test template)
                         |  - impersonates node|
                         |    agents (HTTP)    |
                         |  - seeds AD configs |
                         |  - manages Svc/EPS  |
                         |  - asserts          |
                         +----+-----------+----+
                              |           |
             creates objects  |           | node-agent HTTP API (cmd_port 5005)
                              v           v
                    +------------------+  |
                    |  kube-apiserver  |<-+-------------------+
                    +--------+---------+                      |
                             |  (partition target)           |
                    +--------v---------+          +-----------v--------+
                    |      etcd        |          |  dca-1   <----->  dca-2   (follower→leader
                    +------------------+          |  (SUT, leader-elected)     forwarder)
                                                  +-----------+--------+
                                                              | CLC stats HTTP
                                                              v
                                                    +--------------------+
                                                    |    clc-runner      | (stub, optional)
                                                    +--------------------+
```

## Replica decisions

- **DCA: 2 minimum, 3 recommended.** 2 is enough to exercise every leadership /
  split-brain / forwarding property (one leader, one follower). 3 gives a richer
  election (two followers competing on takeover) and a longer split-brain window
  to observe; cost is a larger state space. Start at 2, escalate to 3 if the
  leadership-core properties need more interleaving pressure.
- **etcd / apiserver: 1 each.** We are not testing Kubernetes' own HA; a single
  apiserver that we can partition/slow is the right, minimal control plane.
- **CLC runner: 1–2** only when lock/rebalance properties are in the run.

## Why not a full Kubernetes distribution

A single-container `k3s`/`kind` would bundle apiserver + etcd + controller-manager
+ scheduler + kubelet in one fate-shared container — we could no longer partition
the DCA from *just* the apiserver, which is the whole point. Running a bare
`etcd` + `kube-apiserver` pair keeps the faultable surface exactly where the
properties need it. The endpoints controller (normally in kube-controller-manager)
is intentionally absent: the `workload` driver owns the DCA Service's
Endpoints/EndpointSlice objects, which turns "endpoint propagation lag" into a
controllable fault (needed by `forwarder-target-is-live-endpoint`,
`forwarder-ip-proxy-consistency`).

## SDK selection

- **Workload:** Antithesis **Go SDK** (assertions + structured randomness). The
  test template lives in the `workload` image at
  `/opt/antithesis/test/v1/clusteragent/`; helper files use the `helper_` prefix.
- **SUT-side instrumentation (optional but high-value):** many catalog properties
  (`dispatch-implies-lease-holder`, `dispatch-store-bijection`,
  `store-lock-bounded-under-slow-clc`, `leadershipchan-no-wedge-under-lock`,
  `reset-restores-store-and-gauges`, `ksm-shard-tracking-consistency`) assert on
  DCA-internal state not observable from the workload. Implementing those needs
  the Antithesis Go SDK added to the root `github.com/DataDog/datadog-agent`
  module and a **custom DCA image** built from the instrumented tree. There is
  **zero existing SDK instrumentation** (`existing-assertions.md`), so this is
  net-new and is the main build-side prerequisite for deep coverage. Workload-only
  properties (forwarding behavior, HTTP contract, external-metrics serving) can
  run against a stock DCA image first.

## Fault → container mapping (how the catalog is exercised)

| Fault | Containers | Property clusters exercised |
|---|---|---|
| Partition `dca-leader` ↔ `kube-apiserver` (asymmetric) ≥ 60s | dca-N, kube-apiserver | leadership-divergence core, warmup/progress, graceful-shutdown |
| **Node termination** of `dca-leader` (DISABLED by default) | dca-N | new-leader-elected, kubeactions crash-replay, forwarder-target stale IP |
| Partition `workload`(as node agent) ↔ `dca-leader` > 30s | workload, dca-N | dispatch-store-bijection (duplicate/orphan), dangling redispatch |
| **Clock skew** backward (DISABLED by default) | global | node-expiry-monotonic-clock, lastconfigchange epoch |
| Latency/partition `dca-leader` → `clc-runner` | dca-N, clc-runner | store-lock-bounded-under-slow-clc, liveness-probe restart loop |
| Partition `dca` ↔ `dd-metrics-backend`, sustained | dca-N, dd-metrics-backend | extmetrics-backoff-cap, (with split brain) extmetrics-configmap lost-update |
| Workload mutates DCA Service EndpointSlice (endpoint lag) | workload, kube-apiserver | forwarder-target-is-live-endpoint, forwarder-ip-proxy-consistency |
| Concurrency/thread interleaving (always on) | dca-N | all lock-hazard and store-integrity properties |

## Assumptions & open questions

- **Fault availability:** node termination and clock skew are commonly disabled
  by default. The mapping above marks which clusters go inert without them —
  confirm tenant config (see catalog-wide Open Questions).
- **AutoDiscovery source for cluster checks:** the workload must feed cluster-check
  configs to the DCA. The simplest deterministic path is Kubernetes
  Services/Endpoints carrying Datadog AD annotations (the `kube_endpoints` /
  `kube_services` providers) or a static/file config provider — to be finalized in
  `antithesis-workload`. Topology only requires that the workload has apiserver
  access to create these objects.
- **kube-apiserver auth/config:** exact flags (static token file vs anonymous,
  RBAC on/off) are a setup detail for `antithesis-setup`; the DCA needs
  permissions for coordination.k8s.io Leases, ConfigMaps, Endpoints/EndpointSlices,
  and (for autoscaling/kubeactions) the relevant CRDs.
- **DCA in-cluster config:** the DCA normally uses `rest.InClusterConfig()` from
  `KUBERNETES_SERVICE_HOST/PORT` + a mounted token. Outside a real cluster the
  setup must supply an equivalent kubeconfig/env so `WaitForAPIClient` succeeds.
