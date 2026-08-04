---
sut_path: /Users/jon.rosario/go/src/github.com/DataDog/datadog-agent
commit: f2da1471bb748fb5108f89f36f7b83cab305ca79
updated: 2026-07-21
external_references: []
---

# Datadog Cluster Agent — SUT Analysis

**SUT scope:** the Datadog Cluster Agent (DCA) only. Code: `cmd/cluster-agent/`,
`pkg/clusteragent/`, `pkg/util/kubernetes/apiserver/leaderelection/`,
`comp/metadata/clusteragent/`. Linux-only (`//go:build !windows && kubeapiserver`);
cluster-check code additionally gated on `//go:build clusterchecks`.

This analysis was produced by a 12-agent discovery ensemble (one per attention
focus) whose structural claims were then verified against primary source. Every
guarantee stated by a doc/comment is recorded as a **claim to test**, not a
verified fact.

## 1. What the DCA is

A **leader-elected singleton** (1 active leader, N warm followers) deployed once
per Kubernetes cluster. It is simultaneously:

- a **control plane** — wins a leader lease, dispatches cluster checks to node
  agents, drives admission/autoscaling/metadata controllers; and
- a **data plane** — node agents poll it over HTTPS for check configs, kube
  tags, and metadata; the HPA controller queries it for external metrics.

Its product purpose (`pkg/clusteragent/README.md`) is to be the *single* point of
contact with the kube-apiserver so node agents don't each hammer it. If the DCA
misbehaves, the blast radius is cluster-wide.

**Scope exclusion (user decision):** the **orchestrator** subsystem (Kubernetes
resource collection → Datadog, `pkg/clusteragent/orchestrator`) is **out of scope**
for this Antithesis harness. It is a largely one-directional collection pipeline
with less split-brain/timing surface than dispatch, leadership, external metrics,
admission, and the gRPC data plane. It is intentionally not modeled below or in the
property catalog. Revisit if collection correctness under leadership flips becomes a
priority.

## 2. Process bring-up (fault-injection ordering)

Entry: `cmd/cluster-agent/main.go` → cobra tree → `start.Commands()` runs
`fxutil.OneShot(start, …)`. The **entire lifecycle lives in one `start()`
function** (`cmd/cluster-agent/subcommands/start/command.go:274-761`), which
blocks on `<-signalCh` and tears down inline. Ordering is manual and
comment-driven ("Initialization order is important", `command.go:316`).

Order (each an injection point):
1. Metrics/expvar server on `metrics_port` (plaintext HTTP), bare goroutine,
   errors only logged (`command.go:330-342`).
2. Leader election engine **created** but not started (`command.go:345`).
3. Leader forwarder created **conditionally** (cluster_checks OR language_detection+reporting OR autoscaling.failover) — other leader-proxied handlers assume it exists (`command.go:352-359`).
4. Main API server started **early** "to ease investigations", before the API-server connection is confirmed (`command.go:368`).
5. **Blocks on `apiserver.WaitForAPIClient`** — hard startup dependency, fatal if unreachable (`command.go:376`).
6. Hostname, cluster name, cluster ID via ConfigMap (`GetOrCreateClusterID`, `command.go:433`).
7. Controllers + optional subsystems, each gated on a config flag and handed `le.IsLeader`/`le.Subscribe`.

**Failure-mode asymmetry:** most subsystems fail *soft* (log + continue:
admission, language detection, appsec, kubeactions, compliance, PAR), but
**autoscaling failures are fatal** and **missing cluster name is fatal when
autoscaling is on** (`command.go:438-440` and the `return errors.New(...)` sites
around `:565-607`). The main API server has a `StopServer()` but it is **not
called** in the shutdown path — no graceful API drain (open question, §11).

## 3. Network surfaces

| Listener | Protocol | Purpose | Auth |
|---|---|---|---|
| Main API (`cluster_agent.cmd_port`, dflt 5005) | HTTPS + gRPC muxed on one TCP port | node-agent REST, IPC, gRPC tagger/kube-metadata streams | dual-token |
| Metrics (`metrics_port`) | HTTP plaintext | Prometheus `/metrics` | none |
| External metrics (HPA) | HTTPS APIService | `custommetrics.RunServer` | k8s APIService |
| Admission webhook (`admission_controller.port`, dflt 8000) | HTTPS | mutating/validating webhook | k8s webhook TLS |

- HTTP+gRPC share one listener via `helpers.NewMuxedGRPCServer` (`server.go:137`).
- **The router is mutated after `Serve()` starts.** Cluster-check endpoints are
  installed via `ModifyAPIRouter` at `command.go:534`, *after* `StartServer`
  returned and `go srv.Serve` is running (`server.go:150-162`) — a live
  `http.ServeMux` mutation; node requests in the registration gap get 404.
- **Dual-token auth** (`server.go:174-219`): "external" (node-agent) paths need
  the DCA auth token; others need the local IPC token. The `isExternalPath`
  classifier is a hand-maintained set of prefix + exact-segment-count checks
  (`== 6`, `== 7`); a trailing slash or a mis-classified new endpoint silently
  rejects node agents.

## 4. Leadership — the master control knob (and the core hazard)

Leader election uses client-go `leaderelection.LeaderElector` on a
`coordination.k8s.io` **Lease** (or a ConfigMap for k8s ≤1.13, chosen by
`CanUseLeases` / `leader_election_default_resource`). Lock name =
`leader_lease_name`; HolderIdentity = pod name.

**Timings** (`leaderelection_engine.go:195-204`): `LeaseDuration =
leader_lease_duration` (default **60s**); `RenewDeadline = LeaseDuration/2`
(30s); `RetryPeriod = LeaseDuration/4` (15s). `ReleaseOnCancel =
leader_election_release_on_shutdown` makes a network call to shorten the lease to
1s on graceful shutdown. Timings are **derived with no floor** beyond
`LeaseDuration > 0`; a legal value of 1 yields sub-second renew/retry that flap
under any latency.

### The headline finding: THREE independent notions of "who is leader"

The DCA has **no single source of truth for leadership.** Three loosely-coupled
facts are assumed to agree but are sourced independently and diverge under fault:

1. **The Lease** — `LeaderEngine.IsLeader()` = `GetLeader() == HolderIdentity`,
   driven by client-go callbacks (`leaderelection.go:328`). Used by the generic
   `LeaderProxyHandler` (`leader_handler.go:108`) and every leader-gated
   controller (admission, autoscaling, languagedetection, kubeactions,
   compliance).

2. **Service-endpoint IP resolution** — followers route to the leader by
   resolving the leader pod name → IP via the DCA Service's Endpoints /
   EndpointSlices, **cached 5 minutes** (`GetLeaderIP`, `leaderelection.go:262-325`).
   A different controller populates this with different readiness semantics than
   the Lease.

3. **The `""`-means-leader heuristic** — the clusterchecks `Handler` decides its
   own leader/follower state purely from whether `GetLeaderIP()` returns the
   empty string (`handler.go:239-280`, **verified**):
   - `case follower: if newIP == "" { newState = leader }`
   - `case unknown:  if newIP == "" { newState = leader }`
   - `case leader:   if newIP != "" { newState = follower }`

**The trap (verified, `leaderelection.go:262-266`):** `GetLeaderIP()` returns
`("", nil)` in two *opposite* cases — "I am the leader" **and** "no leader has
been observed / leaderIdentity is empty." `OnStoppedLeading` sets
`leaderIdentity=""` (`leaderelection_engine.go:164-169`), and it starts empty.

**Consequence:** any condition that makes a *follower* observe `GetLeaderIP()==""`
— a leaderless gap after `OnStoppedLeading`, a not-yet-observed leader at
startup, or inability to resolve the real leader's IP from lagging EndpointSlices
— causes that follower's clusterchecks Handler to **promote itself to leader and
begin dispatching**, while the lease is held elsewhere. Multiple dispatchers →
**duplicate cluster-check execution**. Within one process you can simultaneously
have `IsLeader()==false` (API layer stands down / 503s) and clusterchecks
`state==leader` (dispatches).

### Notification hazards

- `notify()` uses **buffered(1)** channels and **skips** a subscriber whose
  buffer is non-empty (`leaderelection_engine.go:227-239`). A rapid
  leading→not→leading flap collapses to one edge; a subscriber that flushes/
  relinquishes on loss can **miss the loss entirely**. `OnNewLeader` does **not**
  notify — subscribers learn only about *self* transitions, never who the leader
  is.
- The clusterchecks Handler avoids this by *polling* (`leaderStatusFreq = 1s`),
  but sends the new state on a **buffered(1) `leadershipChan` while holding
  `h.m.Lock()`** (`handler.go:246-277`). If the `Run` consumer is mid-warmup
  (up to 30s) during back-to-back transitions, the second send blocks under the
  lock → every `RejectOrForwardLeaderQuery`/`GetState`/`GetConfigs` reader stalls
  (data plane wedged by control plane), and `leaderWatch` stops draining its
  liveness probe → pod restart ~30s later. The code comments at
  `handler.go:211-212` and `dispatcher_main.go:398-399` self-acknowledge the
  hang risk.

### Leader forwarder

Followers reverse-proxy to the leader (`leader_forwarder.go`): a
`httputil.ReverseProxy` behind an RWMutex, `TLSClientConfig{InsecureSkipVerify:
true}`, dial 1s / TLS handshake 5s / response-header 5s. Loop protection is a
single header `X-DCA-Follower-Forwarded` → **508** (single-hop only). Two writers
race to `SetLeaderIP`: the clusterchecks 1s poll and the generic per-request
check-then-act (`leader_handler.go:128-131`). `SetLeaderIP("")` nils the proxy
but **returns before clearing `leaderIP`** (`leader_forwarder.go:117-121`), so
`GetLeaderIP()` misreports a stale IP while forwarding is disabled.

## 5. Cluster-check dispatch (data model + control loop)

Authoritative state is the in-memory `clusterStore` (`stores.go:24-33`):
`digestToConfig`, `digestToNode` (each digest → exactly one node), `nodes`
(each `nodeStore` has its **own** RWMutex — two-level lock, order store→node),
`danglingConfigs`, `endpointsConfigs`, `active` (false during warmup). **No
persistence** — the store is wiped by `reset()` on leadership loss and rebuilt
from AutoDiscovery config replay + node heartbeats on acquisition.

**Pull model** (node-agent-initiated, no push):
- `POST /api/v1/clusterchecks/status/{id}` — heartbeat; `processNodeStatus`
  auto-registers unknown nodes, updates `heartbeat`, and returns `IsUpToDate` by
  comparing `node.lastConfigChange == status.LastChange`. **Side effect:** a
  NodeAgent-typed (non-CLC-runner) heartbeat disables advanced dispatching
  cluster-wide via a **one-way CAS** that never re-enables for the dispatcher's
  lifetime (`dispatcher_nodes.go:60-62`, `dispatcher_main.go:369-374`).
- `GET /api/v1/clusterchecks/configs/{id}` — node pulls its configs; **unknown
  node → HTTP 500**, so a node must POST-then-GET; config-propagation latency =
  poll interval, not push.

**Background loop `dispatcher.run`** (leader-only, `dispatcher_main.go:377-421`),
three tickers: `cleanupTicker` (`node_expiration_timeout/2` = 15s) expires nodes
whose heartbeat is older than `node_expiration_timeout` (30s) and re-dispatches
their configs from `danglingConfigs`; `unscheduledCheckTicker` (60s) only *flags*
stuck configs (detection, not remediation); `rebalanceTicker`
(`rebalance_period`, 10m) rebalances by busyness/utilization when advanced
dispatching is on. **`updateRunnersStats` holds the full store write lock across
synchronous HTTP calls to every CLC runner** (`dispatcher_nodes.go:201-245`) —
N slow runners serialize and can stall all dispatch for N×timeout and trip the
liveness probe.

**Warmup:** on becoming leader the store is `active=false` for
`warmup_duration` (30s); `processNodeStatus` returns `IsUpToDate=true` to *all*
nodes so they keep running cached checks while the new leader rebuilds. A leader
flap shorter than warmup means checks may never dispatch.

## 6. State inventory (persistence boundaries)

| State | Where | Persisted? |
|---|---|---|
| Cluster-check dispatch (`clusterStore`, `nodeStore`, KSM shard map) | in-memory | **No** — rebuilt each leadership cycle |
| Leader identity | k8s Lease (or ConfigMap) | Yes (k8s) |
| External-metric values (legacy HPA) | k8s ConfigMap, read-modify-write **with no resourceVersion/optimistic-concurrency guard** | Yes (k8s) |
| `DatadogMetric` spec/status | CRD; status source-of-truth = local in-memory store only for the leader | mixed |
| Workload/external-metrics stores | in-memory | No |
| Cluster ID, DCA token | k8s ConfigMap, read-then-create/update **with no conflict guard** | Yes (k8s) |
| kubeactions dedup ("processed actions") | in-memory map, wiped on restart | **No** |

The unguarded read-modify-write on shared ConfigMaps (external metrics, token,
cluster ID) is a lost-update / concurrent-create hazard whenever two replicas
both believe they should write (split brain, or first-boot create race).

## 7. External dependencies (by blast radius)

1. **Kubernetes API server** — dominant; everything funnels through it. Global
   singleton client, all-or-nothing `connect()` builds 12 clients; `connect()`
   confirms connectivity only via a *cached local* discovery call, so it can
   "succeed" against a half-broken apiserver. **`informer_client_timeout = 0`**
   → watches have no client-side timeout; a partition that drops the watch
   without RST freezes the informer cache with no error surfaced. Leader-election
   client has isolated QPS (claim: "cannot be starved" — server-side/ network
   faults defeat it). `IsAPIServerReady` string-matches `/readyz` body.
2. **Datadog metrics backend** (external metrics/HPA only) — leader-only refresh
   every 30s; error typing is **fragile string-parsing** of an untyped upstream
   error (format change → all errors collapse to "unknown"); exponential backoff
   up to 1800s; stale metrics flip HPAs to non-scaling.
3. **Node agents connecting in** — heartbeat/poll data plane; liveness is
   poll-based, so expiration ≠ death (see §8).
4. **DNS / pod-IP resolution** for the forwarder (5-min IP cache).
5. **apiserver → DCA admission webhook** — `failure_policy` defaults `Ignore`
   (unknown value also → Ignore): DCA down → pods admitted un-mutated (silent);
   with `Fail`, DCA down → **all pod creation blocked cluster-wide**. Cert
   fetched per-handshake from a possibly-stale informer lister; on error returns
   `(nil,nil)`. Cert rotation is leader-gated → can be missed during churn.
6. **Secret backend** — indirect; `GetKubeSecret` makes a fresh 10s-timeout,
   no-retry client per call.

## 8. Attack surfaces where Antithesis is strongest (timing / partial failure)

- **Split-brain leadership** (§4): partition the leader↔apiserver for ≥ lease
  duration, and/or inject clock skew past the renew deadline; assert at most one
  replica dispatches cluster checks, and clusterchecks-leader agrees with
  lease-leader.
- **Follower self-promotion** on `GetLeaderIP()==""` (§4) — the `""` overload.
- **Expiration ≠ death → duplicate check execution** (§5): partition one node
  agent (alive, still reaching Datadog) from the leader for >30s; its checks are
  re-dispatched elsewhere while it keeps running them → duplicate metrics. No
  fencing token.
- **Stale leader-IP cache** (5 min): kill+reschedule the leader (same pod name,
  new IP, or lagging EndpointSlice); followers forward to a dead/wrong IP.
- **KSM sharding silent drop** — self-documented race (`handler.go:187-191`):
  AD `Schedule` between `reset()` and `RemoveScheduler` repopulates
  `ksmShardedConfigs` → check silently dropped next cycle. Ordering is the fix;
  exercise leadership churn concurrent with `Schedule`.
- **`reset()` asymmetry** — a *series* of past fixes (#52876, #52078, #50715)
  addressed gauges/maps not reset symmetrically on leadership loss; strong signal
  more remain. Flap leadership and assert `nodes_reporting`/`dangling`/
  `unscheduled` gauges return to ground truth and every configured check
  dispatches exactly once.
- **Clock skew on wall-clock node expiry** (`helpers.go:52-53`,
  `time.Now().Unix()`): a backward jump mass-expires all nodes ("No nodes
  reporting"); no monotonic protection.
- **liveness-probe hang → needless restart → leadership churn** feedback loop
  under apiserver latency (§4, §5).
- **Rebalance non-convergence / infinite loop** — freshly-landed complex
  algorithm (merged→reverted→reapplied; `continue`→`break` fix #52884) acting on
  stale busyness when a runner is unreachable.

## 9. Existing test coverage (where Antithesis adds value)

Broad but almost entirely **single-process, single-goroutine, fake-clientset**
unit tests. The `fake.NewSimpleClientset` has **no lease-expiry, no
resourceVersion conflicts, no renew clock** — so client-go's renew/acquire timing
is never exercised. Leader-election unit tests explicitly defer transition
testing to E2E ("tested as part of an end to end test",
`leaderelection_test.go:90-92`); simulate loss via `ctx.cancel()`, not lease
expiry. Node expiry is tested by hand-overwriting `heartbeat` fields, not real
time. Rebalance is called synchronously with stubbed failures. **The E2E
`testDCALeaderElection(restartLeader)` supports killing the leader, but the only
caller passes `false`** — the actual failover path is dead code in the suite.
The forwarder test even *locks in* the stale-`leaderIP` behavior rather than
testing recovery. Net: real multi-replica leader election under fault, stale-data
reads, concurrent dispatch transitions, and API-server partial failure are all
unexercised — exactly Antithesis's domain.

## 10. Claimed guarantees (to test, NOT verified)

- "At most one leader" (implicit, load-bearing everywhere).
- "Each cluster check is dispatched to exactly one node" (no duplicate, no silent
  drop) — README + `digestToNode`.
- "A check on a dead node is eventually reassigned" (dangling redispatch, ~45s
  worst case) — *only if ≥1 live node exists*; zero nodes → no redispatch.
- "A new leader eventually takes over" within ~LeaseDuration.
- "Warmup keeps node caches running safely" (`dispatcher_nodes.go:73-79`).
- "Serving stale data is better than serving no data" — DCA stays Ready and marks
  external metrics stale rather than dropping out of the Service (`command.go:389-392`).
- "Only the leader mutates external state" (appsec `doc.go`, instrumentation
  controller — with a self-documented gap for follower handler errors).
- kubeactions "exactly one action per config" (in-memory dedup, wiped on restart;
  ties to the one-leader guarantee).
- Leader-election client QPS isolation "cannot be starved."
- Forwarder path restoration from `RequestURI` after `StripPrefix` is correct.
- Store "external locking held by the dispatcher" makes compound ops atomic.

## 11. Open questions / assumptions

- **Node-agent side is out of SUT scope** but is the client half of the pull
  loop. The duplicate-execution property (§8) hinges on node agents continuing to
  run cached checks while partitioned from the leader — asserted by the README
  but not verified in node-agent code.
- Whether client-go can surface an empty `holderIdentity` to followers during a
  lease gap governs the *magnitude* of the self-promotion window; the code-level
  `""`-overload is verified, the window size must be measured under fault.
- Whether the main API server is drained on shutdown (`StopServer` exists,
  appears uncalled in `start()`).
- Whether `store.reset()` zeroes `lastConfigChange` (relevant to the config-
  version epoch problem — no leader-generation is attached to the counter).
- `leader_election` global default is `false`; Helm/Operator deployments enable
  it. Much of §4 is inert without it — the harness must enable leader election
  and run ≥2 replicas.
- Endpoints checks are intentionally node-pinned (1:1 with a pod's node), so the
  "exactly one node" invariant differs from load-balanced cluster checks.
- Whether deployments use StatefulSet (stable names/IPs) vs Deployment (random)
  changes the severity of the 5-min IP cache under name reuse.
