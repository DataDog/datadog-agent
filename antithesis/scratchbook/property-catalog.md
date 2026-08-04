---
sut_path: /Users/jon.rosario/go/src/github.com/DataDog/datadog-agent
commit: f2da1471bb748fb5108f89f36f7b83cab305ca79
updated: 2026-07-21
external_references: []
---


# Datadog Cluster Agent — Antithesis Property Catalog


**37 testable properties across 11 categories.** Synthesized from an 11-agent discovery
ensemble (44 raw candidates), stress-tested by a 4-lens evaluation ensemble (refinements + gap-fills folded
in), then every property's open questions were investigated against the code (resolutions + Investigation
Logs live in the evidence files). The codebase has **zero** existing Antithesis SDK instrumentation
(`existing-assertions.md`); every SUT-side assertion is net-new. Per-property evidence: `properties/{slug}.md`.

**Committed decisions (user review):** build a **custom instrumented DCA image up front** (SDK in the root
module) so in-process P0 invariants are testable in the first run; cross-replica leader state is exported via
**SUT-side instrumentation**; **orchestrator is out of scope**. See `deployment-topology.md` and
`evaluation/synthesis.md`.

**Field legend.** *Priority* P0 crown-jewel / P1 high / P2 medium. *Intent* — `invariant` (should hold today) /
`should-improve` (aspirational) / `known-defect-reproducer` (code is already read to violate it; the deliverable
is a minimized reproducing trace that flips to a live invariant once fixed — NOT expected green on day one).
*Witness* — a paired `Reachable`/`Sometimes` assertion proving the hazardous window was actually scheduled.
*⚠ Requires fault* — depends on a fault commonly **disabled by default**; inert unless the tenant enables it.

## Priority summary

| Priority | Count | Slugs |
|---|---|---|
| P0 | 6 | dispatch-implies-lease-holder, dispatch-store-bijection, leader-eventually-dispatches-after-warmup, leadershipchan-no-wedge-under-lock, no-duplicate-check-execution-fencing, store-lock-bounded-under-slow-clc |
| P1 | 25 | admission-webhook-available-under-churn, advanced-dispatching-node-set-integrity, configmap-concurrent-create-converges, dangling-eventually-redispatched, dangling-redispatch-no-resurrect, duplicate-execution-window-bounded-after-heal, empty-token-never-authenticates, extmetrics-configmap-no-lost-update, extmetrics-crd-store-converges-after-flip, forwarder-ip-proxy-consistency, forwarder-request-fidelity, forwarder-single-hop-loop-cap, forwarder-target-is-live-endpoint, graceful-shutdown-releases-lease-bounded, grpc-stream-subscription-accounting, informer-fresh-or-staleness-surfaced, ksm-shard-tracking-consistency, kubeactions-at-most-once, liveness-probe-no-restart-loop, new-leader-elected-after-loss, no-404-on-registered-cluster-check-routes, node-expiry-monotonic-clock, rebalance-cycle-terminates, rebalance-no-perpetual-thrash, reset-restores-store-and-gauges |
| P2 | 6 | admission-webhook-no-silent-nil-cert, autoscaling-fatal-startup-crashloop, extmetrics-backoff-cap-stays-serving, extmetrics-crd-status-no-regression-across-flip, getconfigs-distinguishes-unknown-node, isexternalpath-classifier-consistency |

**Intent mix:** invariant=23, should-improve=2, known-defect-reproducer=12. The 12 known-defect reproducers assert behavior the code is currently read to violate — they ship as minimized traces and are not expected green until fixed.

**Require a disabled-by-default fault (1):** node-expiry-monotonic-clock. Other properties list optional *enhancement* faults (e.g. node termination for crash-replay variants) in their Fault deps but reach their primary hazard via default-enabled partition/latency.


## Leadership & Forwarding (Control Plane)

Leadership is three loosely-coupled facts (lease `IsLeader()`, Service-endpoint IP resolution, the `GetLeaderIP()==""` heuristic) that diverge under fault. These assert they agree and that follower-forwarding stays correct in target, loop-bound, and path/header fidelity.


### dispatch-implies-lease-holder — Active cluster-check dispatch implies this replica holds the lease

| | |
|---|---|
| **Priority** | P0 |
| **Type** | Safety |
| **Assertion** | `Always` |
| **Intent** | invariant |
| **Property** | A replica whose clusterchecks Handler is actively dispatching (state==leader, store active) must simultaneously be the Kubernetes lease holder, so at most one replica dispatches cluster checks at any instant. |
| **Invariant** | `assert.Always`: whenever the clusterchecks Handler state==leader and dispatcher.run is live (store.active true after warmup), `LeaderEngine.IsLeader()` (lease-derived) must be true for the same process. Cross-replica corollary asserted from the workload: at most one replica reports clusterchecks state==leader at a time. Always fits — the guarantee must hold on every evaluation; any divergence is the split-brain bug. |
| **Witness** | `Reachable`: the **ex-leader** observed GetLeaderIP()=="" while its clusterchecks state was still `leader` (lease-less dispatch after OnStoppedLeading) — NOT a follower self-promoting. Investigation confirmed client-go keeps the Lease holderIdentity until reacquired, so a follower retains the old leader's name; the reachable split-brain is the ex-leader continuing to dispatch until it observes a different non-empty leader IP post-heal. |
| **Antithesis Angle** | The clusterchecks Handler derives its role ONLY from whether `GetLeaderIP()` returns "" (handler.go:257-272, verified). `GetLeaderIP()` returns ("",nil) for TWO opposite conditions (leaderelection.go:262-266): "I am the leader" AND "no leader observed / leaderIdentity is empty." `OnStoppedLeading` clears leaderIdentity to "". So a follower that observes "" during a leaderless gap self-promotes and dispatches while the lease is held elsewhere. Inject an asymmetric partition leader<->apiserver for >= LeaseDuration (60s) so the old leader loses the lease while a follower acquires it; assert no two replicas dispatch, and in-process state agrees with the lease. Investigation refinement: the violating replica is the EX-LEADER (state==leader, store active, GetLeaderIP()=="", IsLeader()==false after OnStoppedLeading), which never re-enters warmup and keeps dispatching until it observes the new leader's IP (handler.go:258-260 never leaves state==leader on newIP==""). Evaluate the assertion on every replica; expect the failure on the ex-leader. |
| **Why It Matters** | This is the load-bearing 'exactly one leader' guarantee behind every leader-gated behavior. Two dispatchers → the same check scheduled from two control planes → duplicate metrics, conflicting node assignments cluster-wide. The single most important property in the catalog; surfaced independently by 4 focus agents. |
| **Investigation refinement** | Mechanism refinement (invariant still valid, does not invalidate): the reachable split-brain is the EX-LEADER continuing to dispatch (state==leader, store active, newIP=="", IsLeader()==false after OnStoppedLeading), NOT a follower self-promoting on "". Evidence: handler.go:258-260 never leaves state==leader on newIP==""; leaderelection_engine.go:164-169 clears identity only on the ex-leader; client-go keeps the Lease holderIdentity until reacquired so followers retain the old leader's name. The assertion (!dispatching || IsLeader()) should be evaluated on every replica, but the violation fires on the ex-leader; the paired R1 witness 'follower observed GetLeaderIP()==""' should be reframed to 'ex-leader observed GetLeaderIP()=="" while state==leader'. |
| **Fault deps** | network partition leader<->apiserver >= leader_lease_duration (asymmetric; enabled by default); clock skew past renew deadline (DISABLED by default — amplifies the window); requires leader_election enabled + >=2 replicas |
| **Evidence** | `properties/dispatch-implies-lease-holder.md` |

**Open Questions:**

- The 60s-partition window magnitude is measured under fault; note warmup_duration does NOT protect the primary hazard (the ex-leader keeps dispatching, already past warmup), so warmup masks only newly-promoted replicas, not this path. `(partial)`


### new-leader-elected-after-loss — A new leader is eventually elected after the current one is lost

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Liveness |
| **Assertion** | `Sometimes` |
| **Intent** | invariant |
| **Property** | After the replica holding the lease loses it (partition, termination, or graceful step-down), some surviving replica acquires the lease and resumes leader-only work within a bounded time. |
| **Invariant** | `assert.Sometimes(a_new_distinct_leader_acquired_after_loss)`: during a quiet period (ANTITHESIS_STOP_FAULTS) following a forced leadership loss, a replica different from the previous holder becomes `IsLeader()==true` and its dispatcher becomes active. Sometimes is correct — it is a progress/liveness condition that must become true at least once per run, verified under a recovery window, not on every evaluation. |
| **Antithesis Angle** | client-go renews at RenewDeadline=LeaseDuration/2 (30s) and re-acquires; runLeaderElection loops re-running Run after loss (leaderelection.go:236-248). Kill or partition the current leader, pause faults, and assert a new leader appears within ~LeaseDuration. With graceful shutdown, ReleaseOnCancel shortens the lease to 1s for fast failover. |
| **Why It Matters** | If no replica takes over, all leader-only work halts cluster-wide: dispatch stops, HPA metrics stop refreshing, controllers stall. The core availability guarantee of a leader-elected singleton. |
| **Fault deps** | network partition leader<->apiserver >= LeaseDuration (works with defaults); node termination (DISABLED by default — needed for the crash-failover variant); requires leader_election enabled + >=2 replicas |
| **Evidence** | `properties/new-leader-elected-after-loss.md` |

**Open Questions:**

- Code gives the acquire cadence (LeaseDuration 60s, RenewDeadline 30s, RetryPeriod 15s, leaderelection_engine.go:200-202); a hard recovery SLA still needs measurement under injected latency, so keep Sometimes rather than a deadline assertion. `(partial)`
- ReleaseOnCancel performs a blocking Lease Update network call on shutdown (leaderelection_engine.go:196-198); under partition it blocks until the k8s client/dial timeout (not unbounded), delaying handoff — exact bound is measured under fault. `(partial)`


### forwarder-ip-proxy-consistency — Leader forwarder's reported IP is consistent with proxy availability

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | known-defect-reproducer |
| **Property** | The global leader forwarder never reports a non-empty leader IP while its proxy is nil (forwarding disabled), and never holds a live proxy while reporting an empty IP. |
| **Invariant** | `assert.AlwaysOrUnreachable`: on every SetLeaderIP/Forward, (proxy==nil) iff (reported leaderIP==""). AlwaysOrUnreachable fits because the follower-forwarding path is optional (a single-replica or always-leader run never exercises it), but whenever it runs the consistency must hold. |
| **Witness** | `Reachable`: SetLeaderIP("") ran while a concurrent Forward/SetLeaderIP raced on the global forwarder. |
| **Antithesis Angle** | `SetLeaderIP("")` sets proxy=nil but RETURNS before clearing lf.leaderIP (leader_forwarder.go:117-121), so GetLeaderIP() misreports a stale IP while forwarding is off. Two writers race to SetLeaderIP: the clusterchecks 1s poll and the generic per-request check-then-act (leader_handler.go:128-131). Partition follower<->apiserver or churn leadership to drive GetLeaderIP()=="" and interleave the two writers. |
| **Why It Matters** | A follower that believes it can forward (non-empty IP) but has a nil proxy returns 503/mis-routes node-agent traffic; misreported control-plane state also corrupts status/telemetry an operator trusts during an incident. |
| **Investigation refinement** | No invariant change. Confirms the bug is exploitable and unmasked: SetLeaderIP("") (leader_forwarder.go:117-119) returns before clearing lf.leaderIP, and the single stale-value consumer (leader_handler.go:128) uses it to skip re-arming the proxy, so proxy stays nil while GetLeaderIP() advertises a live IP. The simple fix (clear lf.leaderIP="" on the empty branch) makes the assertion hold. |
| **Fault deps** | network partition (follower<->apiserver, or leader churn producing GetLeaderIP()==''); concurrency (two SetLeaderIP writers); requires leader_election enabled + >=2 replicas |
| **Evidence** | `properties/forwarder-ip-proxy-consistency.md` |

**Open Questions:**

- Reachability of the same-name/new-IP reuse step depends on the harness deployment kind (needs-human): standard helm deploys the DCA as a Deployment (random pod names) so a rescheduled leader gets a NEW HolderIdentity and GetLeaderIP re-resolves under a fresh cache key; a StatefulSet (stable names) is required to hit the stale-same-name path. `(partial)`


### forwarder-single-hop-loop-cap — Follower forwarding is capped at a single hop

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `Always` |
| **Intent** | invariant |
| **Property** | A request already carrying the X-DCA-Follower-Forwarded header is never forwarded again; it is answered with 508 Loop Detected, bounding any follower→leader proxy chain to one hop. |
| **Invariant** | `assert.Always`: in LeaderForwarder.Forward, if the incoming request has X-DCA-Follower-Forwarded set, the outcome is a 508 and no outbound proxy call is made. Always fits — the anti-loop guard must hold on every forwarded request. |
| **Witness** | `Reachable`: a request bearing X-DCA-Follower-Forwarded actually arrived (the 508 guard was exercised, not skipped). |
| **Antithesis Angle** | During a leadership flip two replicas can each believe the other is leader (transient), so A forwards to B and B could forward back to A. The single header is the only loop bound (leader_forwarder.go:90-95). Partition/churn leadership so multiple replicas are simultaneously in follower state and hammer a forwarded endpoint. |
| **Why It Matters** | An unbounded forward loop under a leadership flip would amplify into a request storm across replicas, saturating the connection pool (MaxConnsPerHost) and taking down the DCA API for node agents. The guard caps blast radius to one hop — but if it regresses, the failure is cluster-wide. |
| **Fault deps** | network partition (asymmetric) to create mutual-follower state; enabled by default; node termination/rescheduling to churn leadership (DISABLED by default); requires leader_election enabled + >=2 replicas |
| **Evidence** | `properties/forwarder-single-hop-loop-cap.md` |

**Open Questions:**

- Magnitude of the mutual-follower leaderless window depends on client-go lease timing and is measured under fault, not derivable statically. `(partial)`


### forwarder-target-is-live-endpoint — Follower forwards only to a live DCA Service endpoint IP

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | invariant |
| **Property** | When a follower reverse-proxies a node-agent request to the leader, the destination IP is a current member of the Datadog Cluster Agent Service's endpoints — never a stale/reused IP from the 5-minute cache pointing at a dead or unrelated pod. |
| **Invariant** | `assert.AlwaysOrUnreachable`: whenever Forward dials a target, that target IP is present in the current EndpointSlice/Endpoints set for the DCA service. AlwaysOrUnreachable fits — forwarding is optional, but any forward must target a live endpoint. Because the forwarder uses InsecureSkipVerify:true, TLS will not catch a wrong target, so the invariant must be checked explicitly. |
| **Witness** | `Reachable`: GetLeaderIP served a cached IP that differed from the current EndpointSlice set (stale-cache branch hit). |
| **Antithesis Angle** | GetLeaderIP caches leader pod-name→IP for 5 minutes (leaderelection.go:292) and 'will not return an error if the leader does not exist anymore'. Kill+reschedule the leader (same pod name, new IP — StatefulSet-style) or lag EndpointSlices; the follower forwards auth-bearing requests (carrying the DCA token) to a stale IP for up to 5 minutes. Requires node termination to reschedule the leader. |
| **Why It Matters** | Auth-bearing node-agent/HPA traffic sent to a wrong or dead IP → silent black-hole (502) or, if the IP is reused, delivery to an unrelated pod. Cluster checks and external metrics stall for up to the cache TTL with no error surfaced. |
| **Investigation refinement** | Scope refinement (invariant unchanged): the stale-target hazard is reachable only when the leader's pod NAME persists while its IP changes (StatefulSet-style reuse), because GetLeaderIP caches by name for 5 min (leaderelection.go:268-292). Under the standard Deployment topology (random pod names) a reschedule yields a new HolderIdentity and a fresh cache key, so the invariant is largely vacuous there; the R1 witness/fault should assume StatefulSet-style naming to be non-vacuous. |
| **Fault deps** | node termination / pod restart with IP change (DISABLED by default — must be enabled); network partition / EndpointSlice propagation lag (enabled by default); requires leader_election enabled + >=2 replicas |
| **Evidence** | `properties/forwarder-target-is-live-endpoint.md` |

**Open Questions:**

- Exploitability depends on harness deployment kind (needs-human): a StatefulSet (stable pod name, new IP) makes the stale-same-name/new-IP path reachable; the standard helm Deployment (random names) forces a fresh GetLeaderIP cache key on reschedule, largely closing it. `(partial)`
- Real-world likelihood that a reused pod IP belongs to a pod that actually reads/logs the bearer token (vs. simply RSTs) is a probabilistic operational judgment not answerable from code. `(needs human input)`


### forwarder-request-fidelity — Forwarded request preserves path and Authorization

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | invariant |
| **Property** | When a follower forwards a node-agent request to the leader, the outbound request's URL path equals the original request target (the full /api/v1/... or /api/v2/... path), not the StripPrefix-stripped path. |
| **Invariant** | In the ReverseProxy Director (leader_forwarder.go:123-135), after path restoration the outbound req.URL.EscapedPath() equals url.ParseRequestURI(req.RequestURI).EscapedPath(), and it begins with a registered API prefix (/api/v1 or /api/v2). Equivalently: ParseRequestURI(req.RequestURI) succeeded (err==nil) AND req.URL.Path/RawPath were set from it. `AlwaysOrUnreachable` fits — the forward path is optional (only followers forward), but any forwarded request must preserve its path and Authorization. |
| **Witness** | `Sometimes`: At least once, a follower forwards to the leader a request whose incoming URL.Path had been stripped (differs from the RequestURI path), proving the StripPrefix->restore interaction was genuinely scheduled and the fidelity invariant did not pass vacuously. |
| **Antithesis Angle** | The follower's leader-proxy handlers are registered UNDER http.StripPrefix (server.go:66,79), so by the time Forward runs, req.URL.Path is already stripped to e.g. /clusterchecks/status/{id} while req.RequestURI still holds /api/v1/clusterchecks/status/{id}. The Director is the ONLY code that restores the prefix, and it does so behind `if err == nil` (line 130): any RequestURI that fails url.ParseRequestURI silently falls through, forwarding the STRIPPED path — the leader then 404s or routes to the wrong handler. A percent-encoded segment (check digest, node name, tag with %2F or reserved chars) can also mismatch if RawPath is not carried faithfully. Antithesis reaches this only by running >=2 replicas with real leader election so the follower->leader forwarding path actually executes through the real StripPrefix router; existing unit tests stub the target and set req to /foo, never exercising restoration. A fuzzing workload that sends edge-case request targets (percent-encodings, trailing slash, double slash, dot segments, semicolon params) through a follower drives the parse/escape edges. |
| **Why It Matters** | A silent mis-route sends an authenticated node-agent request to the wrong leader handler or yields a 404/500 while the operator sees the DCA as healthy. Cluster-check config pulls (GET /api/v1/clusterchecks/configs/{id}) or heartbeats (POST .../status/{id}) that mis-route stop dispatching checks for that node — a cluster-wide data-plane outage with no error surfaced on the leader. This restoration logic is new (net/http refactor #50380) and untested for its actual purpose. |
| **Fault deps** | leader_election enabled + >=2 replicas (required; forwarding path is inert otherwise); A workload that drives node-agent requests at a FOLLOWER replica so the follower->leader forward executes (required); NO node-termination and NO clock-skew needed; a workload that sends edge-case request targets (percent-encoded segments, trailing/double slash, dot segments) maximizes coverage of the parse/escape branch |
| **Evidence** | `properties/forwarder-request-fidelity.md` |

**Open Questions:**

- Whether the leader's net/http ServeMux treats a Path/RawPath mismatch as a different route: the assertion keys on EscapedPath equality, which is what Go 1.22 ServeMux matches on, so equality is the right check; residual is confirming the leader mux uses default 1.22 matching with no custom normalization. `(partial)`


## Cluster-Check Dispatch Store Integrity

The in-memory `clusterStore` maps each digest to exactly one node and is rebuilt every leadership cycle. Structural invariants and symmetric reset — the densest bug-fix area.


### dispatch-store-bijection — Config-digest ↔ node assignment is an exact bijection

| | |
|---|---|
| **Priority** | P0 |
| **Type** | Safety |
| **Assertion** | `Always` |
| **Intent** | invariant |
| **Property** | At every quiescent point, each known cluster-check config digest is in exactly one of {assigned to exactly one existing node} XOR {held in danglingConfigs} — never both, never neither, never on two nodes, never mapped to a non-existent node. |
| **Invariant** | `assert.Always` via a store validator run under d.store lock at the tail of addConfig/removeConfig/expireNodes/rebalance/deleteDangling: (1) every digest in digestToNode maps to a node present in nodes, and that node's digestToConfig holds it; (2) no digest appears in two nodes' maps; (3) every node-held digest maps back via digestToNode; (4) digestToConfig has an entry for every referenced digest; (5) a digest is not simultaneously dangling and assigned. Always fits — a global structural invariant that must hold on every store mutation. |
| **Witness** | `Reachable`: a node was expired while still holding configs, concurrent with a Schedule/reset (the fracture interleaving actually occurred). |
| **Antithesis Angle** | Two-level lock (store then node) released between operations, plus reset() wiping the store mid-flight, is the fracture point. addConfig (dispatcher_configs.go:154-163) does check-then-act with a `foundCurrent && currentNode != targetNode` guard (PR #3023). Interleave AD Schedule/Unschedule with expireNodes (30s heartbeat timeout) and a leadership loss→reset→re-acquire cycle; assert the bijection after each. Node-agent partition drives expiry; backward clock skew amplifies mass expiry. |
| **Why It Matters** | This is the store-level shadow of the split-brain hazard and the concrete mechanism behind 'each check dispatched to exactly one node.' Orphaned digest → silent check drop (monitoring gap, no alert). Digest on two nodes → duplicate check execution → double-counted metrics. Surfaced by 4 focus agents. |
| **Investigation refinement** | Scope clarifications (no invalidation): (1) invariant must be scoped to non-endpoint maps (digestToConfig/digestToNode/nodes/danglingConfigs) - endpointsConfigs (stores.go:31) is a disjoint node-pinned structure, confirming the carve-out. (2) The reset-asymmetry angle should name endpoint_checks.configs_dispatched (dispatchedEndpoints) as the concrete surviving Inc-without-clear gauge across resets - no Delete site exists anywhere, so it ratchets each leadership cycle on AD replay. |
| **Fault deps** | network partition (node-agent<->leader >30s to trigger expiry; enabled by default); node hang/throttle on a node agent; clock skew backward (DISABLED by default — amplifies mass expiry); requires leader_election enabled + >=2 replicas to exercise reset/re-acquire |
| **Evidence** | `properties/dispatch-store-bijection.md` |

**Open Questions:**

- None — all resolved during investigation (see evidence file Investigation Log)


### reset-restores-store-and-gauges — Leadership loss resets store and telemetry gauges to ground truth

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | invariant |
| **Property** | On losing leadership, dispatcher.reset() returns the in-memory store AND its exported gauges (nodes_reporting, dangling, unscheduled, KSM shard map) to empty/ground truth, so a later re-acquisition starts clean with no leaked counts. |
| **Invariant** | `assert.AlwaysOrUnreachable`: after reset() completes, all store maps are empty AND each gauge equals its ground-truth value (0 / len(map)). AlwaysOrUnreachable fits — reset only runs on a leadership loss (optional path), but whenever it runs the post-condition must hold. |
| **Witness** | `Reachable`: reset() ran with a non-empty store and non-zero gauges (a real leadership loss with live state). |
| **Antithesis Angle** | A series of past fixes addressed reset asymmetry: #52876 (nodeAgents.Dec missing → nodes_reporting drifted up every cycle), #52078 (dangling/unscheduled gauges), #50715 (KSM shard map). Strong signal more remain. Flap leadership repeatedly (partition→heal) and assert gauges return to ground truth each cycle. This is a regression cluster — confirm each fixed mechanism still holds and probe adjacent gauges/maps. |
| **Why It Matters** | Gauge drift misleads operators (false 'N nodes reporting') and, for the KSM shard map, causes a check to be silently dropped next cycle. The repeated fix history makes this a high-value regression target. |
| **Investigation refinement** | Strengthen the assertion set: the reset() post-condition check must cover endpoint_checks.configs_dispatched (dispatchedEndpoints) plus configsInfo/busyness/predictedUtilization, not just nodes_reporting/configs_dangling/unscheduled - dispatchedEndpoints is a confirmed Inc-without-clear that ratchets across leadership cycles (primary evidence: no Delete site in code; reset() at stores.go:42-55 never touches it). |
| **Fault deps** | network partition to force lease loss then heal (leadership flap; enabled by default); requires leader_election enabled + >=2 replicas |
| **Evidence** | `properties/reset-restores-store-and-gauges.md` |

**Open Questions:**

- Should the narrow in-flight heartbeat race be explicitly closed? A PostStatus that passed RejectOrForwardLeaderQuery reading state==leader just before h.state flips to follower can have its getOrCreateNodeStore land after reset() (both serialize on d.store.Lock), registering a phantom node + nodeAgents.Inc post-reset. It self-heals via expireNodes within node_expiration_timeout next term, so bounded/transient - but whether processNodeStatus should re-check active/leadership is a design call. `(needs human input)`


### dangling-eventually-redispatched — Dangling configs are eventually re-dispatched when a node is available

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Liveness |
| **Assertion** | `Sometimes` |
| **Intent** | invariant |
| **Property** | When at least one live node exists, configs in danglingConfigs are eventually re-dispatched (the dangling map drains toward zero), so a check orphaned by node loss resumes running. |
| **Invariant** | `assert.Sometimes(dangling_map_drained_to_zero_with_live_nodes)`: during a quiet period after a node-loss event, with >=1 node registered, danglingConfigs reaches empty. Sometimes fits — a liveness/progress condition verified under a recovery window. |
| **Antithesis Angle** | The cleanup ticker (node_expiration_timeout/2 = 15s) calls shouldDispatchDangling (requires >=1 node) then reschedule→deleteDangling (dispatcher_main.go:400-411). With ZERO nodes, dangling is NOT drained — only a warning logs. Kill all node agents, then bring one back, and assert dangling flushes. Worst-case recovery ~node_expiration_timeout + cleanup period (~45s). |
| **Why It Matters** | If re-dispatch stalls, checks silently stop running (monitoring gap). The known corner case (zero nodes → no drain) means a full node-agent outage leaves every config stuck until a node returns. |
| **Fault deps** | network partition of node agents > node_expiration_timeout then heal (enabled by default); requires leader_election enabled + >=2 replicas (dispatch is leader-only) |
| **Evidence** | `properties/dangling-eventually-redispatched.md` |

**Open Questions:**

- None — all resolved during investigation (see evidence file Investigation Log)


### dangling-redispatch-no-resurrect — Dangling re-dispatch never resurrects an unscheduled config

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | invariant |
| **Property** | A cluster-check config that AutoDiscovery removed via Unschedule is never re-added to the dispatch store by the periodic dangling re-dispatch loop. |
| **Invariant** | `assert.AlwaysOrUnreachable`: a digest removed by Unschedule/removeConfig never reappears in digestToConfig/digestToNode via reschedule. AlwaysOrUnreachable fits — the re-dispatch path is periodic/optional, but whenever it runs it must not resurrect a removed config. |
| **Witness** | `Reachable`: an Unschedule interleaved the retrieveDangling→reschedule→deleteDangling span. |
| **Antithesis Angle** | The dangling re-dispatch sequence retrieveDangling()→reschedule→deleteDangling is NOT under a single store lock across the whole span (dispatcher_main.go:405-411); a concurrent Unschedule can interleave. A config Unscheduled mid-re-dispatch could be re-added (resurrected) or a live config dropped. Interleave AD Unschedule with the 15s cleanup tick during node churn. |
| **Why It Matters** | A resurrected check keeps collecting metrics for a config the user deleted — a correctness and cost problem that is invisible until someone notices duplicate/zombie data. |
| **Fault deps** | node expiry to populate danglingConfigs (network partition; enabled by default); concurrent AutoDiscovery Unschedule; requires leader_election enabled + >=2 replicas |
| **Evidence** | `properties/dangling-redispatch-no-resurrect.md` |

**Open Questions:**

- Frequency/latency of AD Unschedule relative to the cleanup cadence is runtime/environment-dependent and not derivable from code. Mechanism confirmed: the resurrect window is the gap between retrieveDangling (RUnlock, dispatcher_configs.go:217-222) and the per-config addConfig inside reschedule->add (each takes d.store.Lock separately); AD Unschedule runs on the AutoConfig single worker goroutine (controller.go:151-215) contending only on d.store.Lock. Reaching it requires Antithesis interleaving control, not wall-clock luck. `(partial)`


### ksm-shard-tracking-consistency — KSM shard tracking never diverges from the dispatch store

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | invariant |
| **Property** | The ksmShardedConfigs tracking map and the dispatch store never diverge such that a KSM check is marked 'already sharded' while it has no shards in the store — which would silently drop the check on the next leadership cycle. |
| **Invariant** | `assert.AlwaysOrUnreachable`: for every digest in ksmShardedConfigs, its shard digests exist in the store; and no KSM source config is marked sharded without live shards. AlwaysOrUnreachable fits — KSM sharding is an optional feature, but whenever active the tracking must match the store. |
| **Witness** | `Reachable`: an AD Schedule of a KSM check landed between reset() and RemoveScheduler. |
| **Antithesis Angle** | Self-documented race (handler.go:187-191): RemoveScheduler must precede reset() or an AD Schedule between reset() clearing ksmShardedConfigs and RemoveScheduler repopulates it → isAlreadySharded returns true → KSM check silently dropped next cycle. ksmShardingMutex and the store lock are taken separately (never together), so the 'is sharded' bit and store shards can disagree under interleaving. Flap leadership concurrent with AD config replay of a KSM check. |
| **Why It Matters** | A silently dropped KSM check means Kubernetes State Metrics stop flowing for that resource — a large, invisible monitoring gap. This is a claimed fix (ordering-dependent); Antithesis is the right tool to confirm it holds under interleaving. |
| **Investigation refinement** | Scope note (no invalidation): the self-documented KSM Schedule-during-reset race appears defended in depth - RemoveScheduler/Deregister serializes with Schedule via the controller's shared ms.m (controller.go:131 vs :191-208) under a single worker goroutine, and runs before reset() (handler.go:191-194), so the specific interleaving may be UNREACHABLE through the AutoConfig path. The invariant must still hold, so the property remains valid as a regression guard, but the paired Reachable witness (a KSM Schedule landing between reset() and RemoveScheduler) may be unschedulable given current controller locking. |
| **Fault deps** | network partition to flap leadership concurrent with AD config replay (enabled by default); concurrency (ksmShardingMutex vs store lock split); requires leader_election enabled + >=2 replicas + ksm_sharding_enabled + advanced_dispatching |
| **Evidence** | `properties/ksm-shard-tracking-consistency.md` |

**Open Questions:**

- None — all resolved during investigation (see evidence file Investigation Log)


## Cluster-Check Dispatch Liveness

A stable leader must eventually dispatch; churn must not starve it forever.


### leader-eventually-dispatches-after-warmup — A stable leader eventually dispatches after warmup

| | |
|---|---|
| **Priority** | P0 |
| **Type** | Liveness |
| **Assertion** | `Sometimes` |
| **Intent** | invariant |
| **Property** | Whenever a replica holds cluster-check leadership continuously for at least warmup_duration, the dispatcher becomes active (store.active=true) and begins dispatching; leadership churn shorter than warmup must not starve dispatch forever. |
| **Invariant** | `assert.Sometimes(dispatcher_became_active_and_dispatched)`: across the run, store.active flips true and at least one config is dispatched after a stable-leadership interval >= warmup_duration. Sometimes fits — a progress condition that must be reached at least once; the failure mode is that under flapping it is NEVER reached. |
| **Antithesis Angle** | On every leadership acquisition the store is reset (active=false) and a warmup timer runs BEFORE dispatch (handler.go:118-141). During warmup, processNodeStatus tells all nodes 'up to date' without dispatching. A partition-induced flap at ~warmup period re-enters warmup each cycle → dispatch never starts (livelock/starvation). Flap leadership at ~warmup_duration and assert dispatch eventually occurs during a stable window. |
| **Why It Matters** | If dispatch never starts, cluster checks silently stop running cluster-wide while nodes believe they are current — the worst kind of outage (no error, no alert). Surfaced by 3 focus agents. |
| **Investigation refinement** | Scope refinement (property remains VALID, not invalidated). Discovery agents assumed a partition blip makes GetLeaderIP resolve non-empty -> follower -> warmup abort. Primary evidence (handler.go:257-261 sends `follower` only when newIP!=''; leaderelection.go:262-266 + engine.go:164-165 return '' on OnStoppedLeading/self-leader) shows warmup is aborted ONLY when a DIFFERENT replica is observed as leader (non-empty IP). A single-pod lease flap (lose/regain with no successor) keeps GetLeaderIP='' and does NOT abort warmup, so dispatch is not starved by self-flap. The Sometimes(dispatcher_became_active) assertion stands, but the antagonist workload must (a) run >=2 replicas that actually alternate lease ownership, and (b) lower leader_lease_duration (default 60s bounds successor acquisition to ~lease-expiry >> the 30s warmup), otherwise sub-warmup starvation is unreachable. |
| **Fault deps** | network partition leader<->apiserver to induce flapping (enabled by default — sufficient); clock jitter (DISABLED by default — sharper trigger); requires leader_election enabled + >=2 replicas |
| **Evidence** | `properties/leader-eventually-dispatches-after-warmup.md` |

**Open Questions:**

- Is warmup_duration ever set below RenewDeadline (leader_lease_duration/2) in real deployments? Code shows both are independently tunable and equal by default (30s == 30s); a field survey is needed to know actual deployment configs. `(needs human input)`


## Concurrency & Lock Hazards

Lock-hold, channel-send, and rebalance-convergence hazards that can wedge the dispatcher, trip liveness probes, or thrash.


### store-lock-bounded-under-slow-clc — Dispatch store write lock is never held across a CLC-runner HTTP call

| | |
|---|---|
| **Priority** | P0 |
| **Type** | Safety |
| **Assertion** | `Always` |
| **Intent** | invariant |
| **Property** | The dispatcher never holds the global clusterStore write lock across a blocking network call, so a slow/partitioned CLC runner cannot stall node heartbeats, config polls, and dispatch. |
| **Invariant** | `assert.Always` (structural, no wall-clock bound): no outbound CLC-runner HTTP call (GetRunnerStats/GetRunnerWorkers) is ever in progress while d.store's write lock is held. Implement a `storeLockHeld` boolean set/cleared around Lock/Unlock and assert it is false at the HTTP call site in updateRunnersStats. A duration bound was rejected (3 evaluation lenses): Antithesis controls the scheduler, so wall-clock 'time under lock' is not a faithful production proxy and the threshold is arbitrary. The real invariant is structural — no blocking I/O under the lock. |
| **Witness** | `Reachable`: an HTTP call to a CLC runner was initiated from the stats path under contention. |
| **Antithesis Angle** | updateRunnersStats takes d.store.Lock() then makes synchronous HTTP calls (GetRunnerWorkers/GetRunnerStats) to every CLC runner while holding it (dispatcher_nodes.go:201-245). N slow runners serialize; every processNodeStatus (heartbeat), getClusterCheckConfigs (poll), and Schedule blocks for N×timeout. Inject latency/partition to CLC-runner IPs during a rebalance and assert node-agent poll latency and lock-hold time stay bounded. |
| **Why It Matters** | A single slow CLC runner stalls the entire dispatch control plane and trips the clusterchecks-dispatch liveness probe (dispatcher_main.go:398-399 self-acknowledges 'might hang'), causing a pod restart and needless leadership churn. Surfaced by 2 focus agents. |
| **Investigation refinement** | Threshold calibration (not an invalidation): worst-case store-write-lock hold in updateRunnersStats ~= numNodes x 2 (GetRunnerWorkers+GetRunnerStats, since rebalance_with_utilization defaults true) x 2s per-call timeout. e.g. 8 nodes -> ~32s > 30s health timeout. Evidence: clcrunner.go:86, common_settings.go:581, dispatcher_nodes.go:209/219. |
| **Fault deps** | network latency/congestion on leader->CLC-runner HTTP (enabled by default); asymmetric partition of a subset of CLC runners; requires leader_election + advanced_dispatching + CLC runners in the topology |
| **Evidence** | `properties/store-lock-bounded-under-slow-clc.md` |

**Open Questions:**

- Whether any production Helm/Operator deployment runs rebalance_period low enough (default 10m) to hit this outside a tuned harness; only rebalance() (rebalanceTicker) reaches updateRunnersStats, so triggering fast needs a lowered rebalance_period or forced rebalance. `(needs human input)`


### leadershipchan-no-wedge-under-lock — Leadership-state send never blocks while holding the handler mutex

| | |
|---|---|
| **Priority** | P0 |
| **Type** | Safety |
| **Assertion** | `Always` |
| **Intent** | invariant |
| **Property** | A leadership-state transition send on the buffered(1) leadershipChan never blocks while h.m is held, because that mutex guards every node-agent-facing clusterchecks request handler. |
| **Invariant** | `assert.Always`: the `h.leadershipChan <- newState` in updateLeaderIP (executed under h.m.Lock, handler.go:246-277) never blocks — i.e. the channel is never full at send time, or the send is moved out from under the lock. Always fits — a no-blocking-under-lock invariant on every transition. |
| **Witness** | `Reachable`: a second leadership transition arrived while Run was mid-warmup (leadershipChan full at send). |
| **Antithesis Angle** | leadershipChan is buffered size 1; updateLeaderIP holds h.m and sends. If the Run consumer is mid-warmup (up to 30s) during a back-to-back transition, the second send blocks under h.m → RejectOrForwardLeaderQuery/GetState/GetConfigs (all RLock h.m) stall → data plane wedged by control plane; leaderWatch stops draining its liveness probe → restart. Flap leadership near the lease boundary while Run is busy in warmup. Surfaced by 3 focus agents. |
| **Why It Matters** | A wedged handler stalls all node-agent cluster-check traffic and self-restarts the pod, cascading into more leadership churn — a self-reinforcing outage under exactly the flapping Antithesis induces. |
| **Investigation refinement** | Scope narrowing (not invalidation): the property's central trigger premise is weaker than stated. (1) Run drains leadershipChan during warmup (handler.go:133), so the vulnerable no-read window is sub-second code stretches, not 30s. (2) Only real leader<->follower flips send (handler.go:257-277); self-transitions never send. (3) Handler polls GetLeaderIP, so notify() coalescing is irrelevant. Net: trigger probability much lower and requires two real flips landing in a narrow window (fault-dependent). A residual hazard window still exists (:143-152), so the invariant stands but is harder to violate. |
| **Fault deps** | network partition leader<->apiserver near lease boundary to flap leadership (enabled by default); clock jitter past renew (DISABLED by default); requires leader_election enabled + >=2 replicas |
| **Evidence** | `properties/leadershipchan-no-wedge-under-lock.md` |

**Open Questions:**

- Refined: Run DOES read leadershipChan during warmup (handler.go:133), and buffer is drained on entry at :111, so the only windows where Run is not selecting on the channel are the brief non-select code stretches (:116-128 and :143-152, sub-second) — not the full 30s warmup. Open: measure whether two sends can land in that narrow window. `(partial)`
- updateLeaderIP sends only on a real leader<->follower transition (handler.go:257-277); self-transitions produce NO send. So a wedge needs TWO real flips (lose then regain) inside a sub-second no-read window at 60s lease / 30s RenewDeadline / 15s RetryPeriod — realistic only under clock-skew/partition fault; needs flap frequency measured under fault. `(needs human input)`


### liveness-probe-no-restart-loop — Health probe stays drained under transient, recoverable delay

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Liveness |
| **Assertion** | `Always` |
| **Intent** | invariant |
| **Property** | The clusterchecks liveness health-probe goroutines (clusterchecks-dispatch, clusterchecks-leadership) always drain their health channel within the probe period under transient apiserver/CLC-runner slowness, so the DCA is not needlessly killed and thrown into leadership churn. |
| **Invariant** | Reframed to the directly-observable symptom: a transient (bounded, recoverable) apiserver/CLC-runner delay does not drive the clusterchecks-dispatch / clusterchecks-leadership health probe to unhealthy. Instrument the probe-drain and assert it stays drained for delays below a recovery bound. The bare-container topology has no kubelet, so the restart→election→churn cascade is NOT asserted here (add a probe/restart shim to the topology if that cascade itself is to be tested); the assertable core is 'probe accuracy under recoverable latency'. |
| **Antithesis Angle** | Both probes only drain healthProbe.C in a no-op select case in the same loop that does blocking work (dispatcher_main.go:398, handler.go:211) — comments self-acknowledge hang risk. A blocking GetLeaderIP (apiserver Get, transport-default timeout) or a store deadlock stops the drain → unhealthy after ~2 missed 15s pings → restart → new election → churn. Inject bounded apiserver latency and assert liveness does NOT flap for transient (recoverable) slowness. |
| **Why It Matters** | Needless restarts convert a transient dependency blip into leadership churn, which (via other properties) risks dispatch starvation and split-brain windows — the assertable core is that the probe does not report unhealthy for a delay the system recovers from on its own. |
| **Investigation refinement** | Assertion calibration: restart cascade requires the probe to stay undrained for ~90s (failureThreshold 6 x periodSeconds 15), not merely one 30s health-timeout window; the internal health component reports unhealthy at 30s (health.go). Instrument the recovery bound against ~90s sustained-failure, per Dockerfiles/manifests/.../cluster-agent-deployment.yaml:200-206. |
| **Fault deps** | network latency/congestion on DCA->apiserver and DCA->CLC-runner (enabled by default); node hang/throttle; requires leader_election enabled |
| **Evidence** | `properties/liveness-probe-no-restart-loop.md` |

**Open Questions:**

- Where is the boundary between a legitimate liveness failure (real deadlock) and a false positive (recoverable transient latency)? The assertion must fire only when a delay the system recovers from on its own caused a restart, without penalizing correct hang-detection — an intended-behavior judgment. `(needs human input)`


### rebalance-cycle-terminates — Each rebalance cycle terminates

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Liveness |
| **Assertion** | `Sometimes` |
| **Intent** | invariant |
| **Property** | Every invocation of the leader's rebalance cycle (rebalanceUsingBusyness / rebalanceUsingUtilization, fired by the 10-min rebalanceTicker) runs a bounded number of moveConfig operations and returns, even when a CLC runner is unreachable, stats are missing, or an AutoDiscovery Unschedule concurrently removes a config mid-cycle. It never spins retrying the same failing move. |
| **Invariant** | assert.Sometimes(rebalance_cycle_completed_after_a_moveconfig_failure): across the run there is at least one rebalanceUsingBusyness invocation that (a) had >=1 moveConfig return an error during the cycle AND (b) still returned to its caller. The hang regression (pre-#52884 `continue`) would loop forever on the failing move, so the Sometimes never fires. Complementary SUT-side hard guard: assert.Always(inner_loop_moveConfig_attempts_per_cycle <= numNodes*numConfigsAtCycleStart) — a static upper bound; exceeding it is the infinite-loop regression. |
| **Antithesis Angle** | rebalanceUsingBusyness (dispatcher_rebalance.go:281-325) has an inner loop `for diffMap[source] > 0` whose only progress is a successful moveConfig (which shrinks the source node's cluster-check stat set and recomputes diffMap via updateDiff). #52884 (84f11df1f18) fixed a `continue`->`break` bug: on moveConfig failure the loop used to retry the SAME digest forever because the store was unchanged, so pickConfigToMove returned it again — an infinite loop that also spiked rebalancing_decisions. The store is NOT held across the whole cycle (calculateAvg/getDiffAndWeights/pickConfigToMove take RLock, moveConfig takes Lock, each released between calls), so Antithesis can interleave a concurrent AD Unschedule/removeConfig or an expireNodes between pickConfigToMove and moveConfig to make moveConfig fail with 'no config registered for digest' / 'node not found' — the REAL failure modes the synthetic unit test (TestRebalanceUsingBusyness_BreaksOnMoveConfigFailure) only fakes. Additionally an unreachable CLC runner makes GetRunnerStats fail in updateRunnersStats (dispatcher_nodes.go:219-224, `continue` keeps stale stats), and moveConfig's per-instance GetRunnerStats can then return no movable instances (movedAny=false -> error -> break). Inject latency/partition dca-leader->clc-runner concurrent with AD config churn, force a rebalance, and assert the cycle returns. |
| **Why It Matters** | The rebalance loop runs under the dispatcher goroutine that also drains the clusterchecks-dispatch liveness probe (dispatcher_main.go:398-399, self-acknowledged hang risk) and holds/contends the global store lock. A non-terminating cycle wedges dispatch cluster-wide, trips the liveness probe -> pod restart -> leadership churn -> more instability. The class of bug (loop that only makes progress on the success path, retries the failure path) is exactly what #52884 was; Antithesis confirms it stays fixed under real concurrent failure interleavings the unit test cannot produce. |
| **Fault deps** | network latency/partition dca-leader -> clc-runner to make GetRunnerStats fail/stale (ENABLED by default); concurrency: AD Unschedule/removeConfig or expireNodes interleaved between pickConfigToMove and moveConfig (always-on thread interleaving); requires leader_election enabled + advanced_dispatching + >=1 CLC runner in topology; node termination and clock skew NOT required |
| **Evidence** | `properties/rebalance-cycle-terminates.md` |

**Open Questions:**

- None — all resolved during investigation (see evidence file Investigation Log)


### rebalance-no-perpetual-thrash — Rebalance does not move a check between runners every cycle

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `Always` |
| **Intent** | should-improve |
| **Property** | Across successive rebalance cycles, a single cluster-check config digest is not repeatedly relocated between the same two runners (A->B->A->B ...) when the busyness/utilization inputs driving the decision are stale or zero because a CLC runner is unreachable. Rebalance must converge to a stable assignment rather than thrash indefinitely. |
| **Invariant** | assert.Always(no digest is relocated on more than K consecutive rebalance cycles): maintain, per digest, a bounded history of (cycle_index, assignedNode) at the tail of each rebalance; assert that no digest changes node on K consecutive cycles (K a small constant, e.g. 3) while the set of node busyness values it depends on is unchanged from the prior cycle (stale). Paired WITNESS (Reachable/Sometimes): a rebalance actually moved a config to or from a runner whose stats were stale due to an unreachable CLC runner — otherwise the Always is vacuous. |
| **Antithesis Angle** | When a CLC runner is unreachable, updateRunnersStats (dispatcher_nodes.go:219-224) does `continue` on GetRunnerStats failure, so the node keeps STALE clcRunnerStats (never zeroed); for the utilization path GetRunnerWorkers failure substitutes DefaultNumWorkers (dispatcher_nodes.go:212-213). A never-reached runner (registered by heartbeat, stats never fetched) shows busyness 0 / utilization ~0, so it looks perpetually 'least busy'. The two anti-thrash guards are the ONLY defense: the busyness path's tolerationMargin=0.9 hysteresis (dispatcher_rebalance.go:300) and the utilization path's rebalanceIsWorthIt(minPercImprovement) + stickiness bias (checks_distribution.go:93-94). moveConfig moves the stats SNAPSHOT to the destination store, so the picture on the next cycle depends on whether the (possibly unreachable/lagging) runner overwrites it in updateRunnersStats — a fault-timing-dependent feedback loop. Antithesis can hold one runner partitioned across several rebalance ticks and observe whether a config oscillates. A unit test calls rebalance once with stubbed stats and cannot reproduce the cross-cycle stale-state loop. |
| **Why It Matters** | Each relocation is a real schedule/unschedule on node agents: the check stops on the old runner and restarts on the new one, dropping in-flight data and re-paying warmup. A config that ping-pongs every 10-min cycle produces perpetual gaps and churn for that check cluster-wide, plus inflated rebalancing_decisions/successful_rebalancing telemetry that masks the pathology. Because the guards are heuristic (a tentative 0.9 margin, a percentage threshold), there is no proof they prevent oscillation when the inputs feeding them are stale/wrong — this is a should-improve hypothesis Antithesis is uniquely able to falsify. |
| **Investigation refinement** | No invalidation. Confirmed both anti-thrash guards are active by default (stickiness_enabled=true common_settings.go:589; busyness tolerationMargin=0.9). Refinement: the utilization path is largely self-protecting against move-back because a just-moved-but-not-yet-reporting config is excluded from currentDistribution (dispatcher_rebalance.go:459-470), so the residual oscillation risk is concentrated in the unreachable-runner-stale-snapshot branch (dispatcher_nodes.go:219-224). |
| **Fault deps** | network latency/partition dca-leader -> one clc-runner sustained across >=K rebalance cycles, so its stats stay stale/zero (ENABLED by default); requires leader_election enabled + advanced_dispatching + >=2 CLC runners so there is a source and a stale target; lower rebalance_period in the harness so multiple cycles fit a timeline; node termination and clock skew NOT required; the safety assertion is INERT unless the paired witness confirms a move touched a stale/unreachable-runner distribution — instrument both |
| **Evidence** | `properties/rebalance-no-perpetual-thrash.md` |

**Open Questions:**

- Right K for the consecutive-move bound (assertion gated on 'busyness inputs unchanged from prior cycle'): too small flags legitimate rebalancing under genuinely changing load; a tuning/intended-behavior judgment not decidable from code. `(needs human input)`


## Node Liveness & Dispatch Mode

Node expiration is wall-clock-based; advanced-dispatching latches one-way. Time and node-set integrity.


### node-expiry-monotonic-clock — Node expiry uses elapsed (monotonic) time, not wall clock

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | known-defect-reproducer |
| **Property** | A node is expired (checks moved to dangling) only when real elapsed time since its last heartbeat exceeds node_expiration_timeout — a backward wall-clock jump must never mass-expire all nodes. |
| **Invariant** | `assert.AlwaysOrUnreachable`: expireNodes removes a node only if monotonic elapsed since heartbeat > timeout. AlwaysOrUnreachable fits — expiry is periodic/optional, but any expiry decision must be based on real elapsed time. Today expiry uses time.Now().Unix() (helpers.go:52-53), so the property is currently expected to FAIL under backward skew — that is the point. |
| **Witness** | `Reachable`: a backward clock jump was applied while nodes had recent heartbeats. |
| **Antithesis Angle** | Heartbeat and cutoff use wall-clock Unix seconds (dispatcher_nodes.go:143-152). A backward NTP/clock jump makes heartbeat < cutoff fire for all nodes at once → 'No nodes reporting, cluster checks will not run' → every config dumped to dangling. Inject backward clock jitter and assert no spurious mass expiry. |
| **Why It Matters** | A single backward clock jump silently halts ALL cluster checks cluster-wide until nodes re-register — a severe, hard-to-diagnose outage triggered by an ordinary NTP correction. Merged from 2 focus agents. |
| **Investigation refinement** | No change. Code confirms the property premise verbatim: expiry uses wall-clock seconds with no monotonic guard (helpers.go:52-53, dispatcher_nodes.go:143,152) and the leader stamps heartbeats on receipt, so the AlwaysOrUnreachable invariant and its expected-FAIL-under-skew framing stand as written. |
| **Fault deps** | clock jitter forward/backward (DISABLED BY DEFAULT) — the property is INERT unless the tenant enables clock faults; requires leader_election enabled + >=2 replicas |
| **⚠ Requires fault** | clock skew (disabled by default) |
| **Evidence** | `properties/node-expiry-monotonic-clock.md` |

**Open Questions:**

- Does the Antithesis harness/tenant actually enable clock-skew faults? Without them the property is inert. No harness config exists in the repo (only antithesis/scratchbook), so this is a tenant fault-config decision that cannot be settled from code. `(needs human input)`
- Do the KSM shard map and per-node gauges recover cleanly after a mass-expiry+redispatch at runtime? Code shows the ksmShardedConfigs map is untouched by the expiry/redispatch path, but full gauge recovery overlaps clusterchecks-dispatch-consistency-after-leadership-recovery and warrants a runtime check. `(partial)`


### advanced-dispatching-node-set-integrity — Advanced dispatching operates only on a valid CLC-runner node set

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | known-defect-reproducer |
| **Property** | While utilization-based (advanced) dispatching is enabled, every node it rebalances over is a CLC runner with a resolvable IP; a plain node-agent heartbeat or an empty-clientIP node must not silently corrupt the utilization view — and the one-way disable latch must reflect current composition. |
| **Invariant** | Two `assert.AlwaysOrUnreachable` sub-invariants (distinct messages): (a) while advancedDispatching==true, no node in the store has nodetype==NodeAgent; (b) no node with empty clientIP is treated as a reachable CLC runner for utilization stats. AlwaysOrUnreachable fits — advanced dispatching is optional, but whenever on, the node set must be valid. |
| **Witness** | `Reachable`: a NodeAgent-typed heartbeat and/or an empty-X-Real-Ip heartbeat was processed while advanced dispatching was enabled. |
| **Antithesis Angle** | disableAdvancedDispatching is a one-way CAS true→false (dispatcher_main.go:369-374) triggered by ANY NodeAgent-typed heartbeat (dispatcher_nodes.go:60-62); it never re-enables for the dispatcher's lifetime. Separately, a node with empty X-Real-Ip (legacy agent) gets DefaultNumWorkers substituted / stale busyness (dispatcher_nodes.go:209-223), poisoning rebalance weights. Inject one spurious NodeAgent heartbeat and one empty-clientIP heartbeat during warmup; assert the mode/latch and rebalance node-set stay valid. |
| **Why It Matters** | A single stray/transient heartbeat permanently degrades load distribution (and disables KSM sharding) cluster-wide until process restart — a durable downgrade from a momentary blip. Merged from 3 focus agents. |
| **Investigation refinement** | Scope strengthened. Discovery assumed the disable latch resets to true on each leadership term ('fresh dispatcher term restores advancedDispatching=true'); code disproves this: reset()/store.reset() never touch d.advancedDispatching (dispatcher_main.go:294-304, stores.go:42-55) and the dispatcher is constructed once (handler.go:74). The latch is therefore PER-PROCESS — once disabled by any NodeAgent heartbeat it stays disabled across all subsequent leadership cycles until process restart. The 'once observed false, never true again' monotonicity assertion should be scoped to the whole process lifetime, and the stale-mode/degraded-load-distribution window persists across leadership recoveries, not just within one term. |
| **Fault deps** | none beyond default config (advanced_dispatching_enabled=true); needs a crafted NodeAgent-typed and an empty-X-Real-Ip heartbeat in the workload; requires leader_election enabled |
| **Evidence** | `properties/advanced-dispatching-node-set-integrity.md` |

**Open Questions:**

- None — all resolved during investigation (see evidence file Investigation Log)


## Idempotency, Replay & Duplicate Execution

At-most-once destructive actions, and the workload-witnessed fencing guarantee that no check runs on two nodes at once (the SUT's #1 timing hazard).


### no-duplicate-check-execution-fencing — No cluster check runs on two nodes without the first being told to drop it

| | |
|---|---|
| **Priority** | P0 |
| **Type** | Safety |
| **Assertion** | `Always` |
| **Intent** | known-defect-reproducer |
| **Property** | The dispatcher never causes a single cluster-check config digest to be executed simultaneously by two distinct live node identities. Concretely: before a config that was handed to node N is (re)dispatched to a different live node M, node N must first have been told to drop it (via a config-poll response for N that omits the digest). Node expiration by heartbeat timeout does NOT satisfy 'told to drop': a partitioned-but-alive N keeps running its cached checks per the cached-check contract, so reassigning to M opens a window where the same check runs on both. |
| **Invariant** | assert.Always, workload-witnessed: for every cluster-check digest D, the set of live simulated node identities currently *executing* D (i.e. D is in the config set the node last successfully pulled and the node has not since pulled a set omitting D) has size <= 1. Equivalently, whenever the leader moves D from N to M (expireNodes -> danglingConfigs -> reschedule), N has already received a poll response omitting D. Always is the correct shape: the 'exactly one node' guarantee (README, sut-analysis §10) must hold at every instant. This is expected to go RED under a default partition, which is the deliverable. |
| **Witness** | `Reachable`: There exists a run state in which a single cluster-check config digest D is simultaneously in the executing-set of two distinct live simulated node identities: the original holder N (partitioned/heartbeat-stopped but still running its cached configs) and the reassignment target M (to which the leader moved D after expiring N). This is the paired witness that the hazardous precondition behind no-duplicate-execution-without-drop-notice was genuinely opened, not merely never exercised. |
| **Antithesis Angle** | expireNodes() (dispatcher_nodes.go:142-186) removes a node purely on `node.heartbeat < timestampNow() - nodeExpirationSeconds` — a timeout, never a confirmation of death or a revocation to the node. Its configs are moved to danglingConfigs and re-dispatched to a live node by the 15s cleanup ticker (dispatcher_main.go:400-411 -> reschedule -> addConfig, dispatcher_configs.go:146-165). The pull model has NO push/revocation channel to N and NO fencing token: nothing tells N it was de-assigned, and nothing tells M that N may still be running the check. The workload stops heartbeating for identity N (or a partition drops N<->leader) for > node_expiration_timeout (30s) while N keeps 'running' its last-pulled configs; the leader expires N and hands D to M; both now run D. Antithesis explores the interleaving of the expiry tick, reschedule, and N's (absent) poll to open and hold the window. |
| **Why It Matters** | This is the reality-side shadow of the load-bearing 'each cluster check dispatched to exactly one node' guarantee. dispatch-store-bijection is CORRECT by design during this window (after reassignment digestToNode[D]=M only, so the store is a perfect bijection) and therefore structurally cannot catch it — the duplicate is invisible to every store-internal assertion. Only a workload that models node agents keeping cached checks alive while partitioned can witness the double execution, which produces duplicate/double-counted metrics for D cluster-wide with no error and no alert. **User decision (2026-07-21): this is a real defect, not an accepted tradeoff — the finding should be escalated toward adding real fencing** (e.g. a generation/epoch token per dispatch, or an ownership token N must present) to the dispatch protocol, not filed away as tolerable failover noise. |
| **Investigation refinement** | No invalidation. Scope tightened by resolutions: (a) restrict the assertion to load-balanced cluster checks (endpoints are carved out — separate store, not rescheduled on expiry); (b) the reachability precondition requires stable leadership > ~30s warmup + one 15s cleanup tick before reassignment can occur; (c) any single surviving live node suffices as target M (no dedicated spare identity needed). |
| **Fault deps** | network partition workload-node-identity(N) <-> dca-leader for > node_expiration_timeout (default 30s) — ENABLED by default; WORKLOAD SUBSTITUTE (no fault needed): the workload simply stops POSTing heartbeat status for identity N while continuing to 'run' N's last-pulled config set, and keeps another identity M heartbeating — this deterministically drives expiry+reassignment with zero fault injection; NO node termination required; NO clock skew required (backward skew would only amplify by mass-expiring); requires leader_election enabled + a leader whose dispatcher.run is active (post-warmup) + >=2 simulated node identities |
| **Evidence** | `properties/no-duplicate-check-execution-fencing.md` |

**Open Questions:**

- Node-agent side (out of SUT scope): the guarantee that a partitioned-but-alive node keeps running its last-pulled cached checks must be modeled by the workload. DCA-side evidence supports the assumption (see log) but the node-agent cache/pull code is not in this repo's cluster-agent package. `(partial)`


### kubeactions-at-most-once — A KubeAction executes at most once across restarts and failover

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `Always` |
| **Intent** | known-defect-reproducer |
| **Property** | A KubeAction identified by (metadata.ID, metadata.Version) has its mutating executor (delete_pod, restart_deployment, patch_deployment, ...) run at most once against the cluster, even across DCA restarts and leadership handovers. |
| **Invariant** | `assert.Always(action_executed_at_most_once)`: for each (ID,Version), the executor side effect fires no more than once across the run. Always fits — an at-most-once safety guarantee. NOTE: dedup state (ActionStore) is an in-memory map wiped on restart, so this property is expected to FAIL under a crash between Claim and MarkExecuted, or under two leaders — that is the finding. |
| **Witness** | `Reachable`: two replicas were simultaneously in leader/dispatch state, or a crash landed between Claim and MarkExecuted. |
| **Antithesis Angle** | ActionStore tracks processed actions in-memory only (action_store.go). Claim marks StatusClaimed, then the side effect runs, then MarkExecuted. A crash after the mutation but before MarkExecuted, or a restart (empty map) with the action re-delivered within its 1-min TTL, re-executes it. If dispatch-implies-lease-holder is violated (two leaders), each has its own map → guaranteed double execution. Requires node termination to exercise the crash-replay path. |
| **Why It Matters** | Double execution of delete_pod / restart_deployment is a destructive, externally-visible action taken twice — a real operational hazard (e.g. two rollout restarts). Ties directly to the one-leader guarantee. Merged from 2 focus agents. |
| **Investigation refinement** | Refine (no invalidation): the duplicate-execution hazard requires a NEW process (DCA restart / new leader pod), not a mere in-place leadership handover — a follower that already holds the config in its RC client state is NOT re-delivered on promotion (client.go fires listeners only on changedProducts). The assertion/witness should tie the double-execution to a process restart between Claim and the empty-map replay, consistent with the node-termination fault dependency already listed. All other premises (in-memory-only dedup, stable ActionKey, non-idempotent executors) are confirmed by primary source. |
| **Fault deps** | node termination (DISABLED by default — required for the crash-replay variant); leader election enabled + >=2 replicas (kubeactions is leader-gated; inert otherwise); network partition to induce split brain; requires remote config client |
| **Evidence** | `properties/kubeactions-at-most-once.md` |

**Open Questions:**

- Does backend ActionTTL/timestamp practice make the 1-min execution window commonly still-open at failover time in real deployments? Code confirms the only time gate is ValidateTimestamp vs action.Timestamp with ActionTTL=1m; whether the action's creation timestamp is still <1m at failover is a backend delivery-timing practice, not determinable from SUT code. `(needs human input)`


### duplicate-execution-window-bounded-after-heal — Duplicate-execution window is bounded after a partition heals

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | should-improve |
| **Property** | Once a previously-expired/partitioned node N re-establishes contact with the leader (partition heals, N resumes heartbeating), N is told to drop any config D that was reassigned away from it within a bounded number of successful poll cycles — so duplicate execution of D on N and M does not persist indefinitely after connectivity is restored. The constructive form of the guarantee: the DCA lacks fencing, but the pull loop should still converge N to the current assignment promptly after heal. |
| **Invariant** | assert.AlwaysOrUnreachable: for a healed node N whose config D was reassigned to M, within K successful (heartbeat=IsUpToDate-false)+(config-poll) cycles after N resumes contact, N's returned config set omits D (N drops it), collapsing the executing-set of D back to size 1. AlwaysOrUnreachable fits — this path only runs when a reassignment-then-heal actually occurs (optional), but whenever it does, convergence must be bounded. Hazards that could make it FAIL: warmup returns IsUpToDate=true to ALL nodes (dispatcher_nodes.go:73-79), and a coincidental/stale lastConfigChange equality (dispatcher_nodes.go:69) also returns IsUpToDate=true — either can keep N running its stale cached set (including D) across the whole warmup or indefinitely, extending the duplicate window past heal. |
| **Antithesis Angle** | On heal, N POSTs status; processNodeStatus auto-recreates a fresh node store for N (getOrCreateNodeStore) and, outside warmup with a differing lastConfigChange, returns IsUpToDate=false, so N re-pulls and gets its new (D-free) set — window closes. BUT: (1) if the new leader is within warmup_duration (30s) when N reconnects, processNodeStatus returns true to N regardless (dispatcher_nodes.go:73-79) and N keeps running cached D; (2) reset() wipes lastConfigChange, so after a leadership flap a fresh counter can coincide with N's cached value (equality check at :69) and N is told up-to-date while actually stale. Antithesis flaps leadership / times the heal to land inside warmup and holds the duplicate past the expected close. |
| **Why It Matters** | The strict no-fencing invariant (property 1) may be an accepted transient IF the window closes fast after heal. This property tests exactly that mitigation. If warmup or the equality-based IsUpToDate keeps a reconnected node running a reassigned check, the duplicate is no longer a brief failover blip but a sustained double-count lasting a full warmup (30s+) or, under the epoch coincidence, until the next real config change — a materially worse and operator-invisible outcome. |
| **Investigation refinement** | Narrow the assertion: the stale/coincidental lastConfigChange-equality hazard (hazard A) is effectively dead — reset() and node re-creation always start lastConfigChange at 0 while a real node posts a nonzero LastChange, so the equality branch cannot falsely mark a stale node up-to-date. Only the warmup-blanket case (hazard B, dispatcher_nodes.go:73-79) can extend the duplicate window past heal (up to warmup_duration=30s). Drop hazard A from the property; keep the warmup-extended window as the remaining should-improve concern. |
| **Fault deps** | network partition N<->leader > node_expiration_timeout then HEAL (leadership stable) — ENABLED by default; or workload-driven heartbeat stop then resume; to reach the warmup-extended and stale-equality cases: flap leadership (partition leader<->apiserver) so a fresh leader is in warmup / has a reset lastConfigChange when N reconnects — ENABLED by default; requires leader_election enabled + >=2 replicas + >=2 simulated node identities |
| **Evidence** | `properties/duplicate-execution-window-bounded-after-heal.md` |

**Open Questions:**

- Node-agent re-pull speed after IsUpToDate=false is out of SUT scope; the workload must model a prompt re-pull for the K-cycle bound to be meaningful. DCA side returns false promptly (verified); node-agent poll cadence is not in this package. `(partial)`


## External Metrics / HPA

ConfigMap-backed and DatadogMetric-CRD external-metrics stores under split brain, leadership flip, and backend outage.


### extmetrics-configmap-no-lost-update — HPA external-metrics ConfigMap writes are not silently lost

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | known-defect-reproducer |
| **Property** | A leader's write of external-metric values to the shared HPA ConfigMap either persists the intended keys or is retried on conflict; a rejected optimistic-concurrency update must not silently drop values or leave the local cache permanently stale. |
| **Invariant** | `assert.AlwaysOrUnreachable`: after SetExternalMetricValues, either the ConfigMap reflects the values or a retry was attempted; on IsConflict the local cache is refreshed, not left stale. AlwaysOrUnreachable fits — this path runs only when external metrics are enabled. The code does plain Update() with no resourceVersion retry (store_configmap.go:190-200), so the property is expected to expose lost updates under split brain. |
| **Witness** | `Reachable`: two replicas issued overlapping ConfigMap Updates (a split-brain write actually occurred). |
| **Antithesis Angle** | updateConfigMap Updates a locally-cached c.cm with no conflict handling; on IsConflict the error is logged and the write dropped, leaving c.cm stale while GetMetrics reads the stale copy. Two replicas both believing they are leader (partition during lease renewal) both write, overwriting Data wholesale from stale local copies → values lost/flip-flop. Inject apiserver latency/partition around RenewDeadline and assert HPA metric values don't regress. Merged from 2 focus agents. |
| **Why It Matters** | Lost or flip-flopping external-metric values make HPAs scale on wrong data or stop scaling — a direct, customer-visible autoscaling failure. Depends on the one-leader guarantee holding. |
| **Investigation refinement** | Scope/severity weakened (not invalidated). Evidence: (1) the write path is strictly leader-gated (controller_util.go:243) so the RMW conflict requires actual split-brain; (2) ListAllExternalMetricValues re-Gets the ConfigMap at the start of every refresh cycle (controller_util.go:185 -> store_configmap.go:139) immediately before SetExternalMetricValues (:224), so the stale-ResourceVersion window is per-cycle, not persistent; (3) the c.cm==nil 'permanent wedge' claim is false — the same per-cycle ListAll (and the GC loop at :140) repopulates c.cm, self-healing within ~1 refresh_period (~30s). The property should assert 'a leader write is either persisted or retried, and any cache wedge clears within one refresh cycle', not a permanent lost-update/wedge. |
| **Fault deps** | network partition producing split brain (two replicas writing; enabled by default); apiserver latency around lease renewal; requires leader_election enabled + >=2 replicas + external_metrics_provider enabled (ConfigMap store, not CRD). **Deprioritized (user decision, 2026-07-21): the harness pins the DatadogMetric CRD provider as primary; this ConfigMap-path property is secondary/optional** (run only if the legacy provider is separately exercised). |
| **Evidence** | `properties/extmetrics-configmap-no-lost-update.md` |

**Open Questions:**

- Do real-world DCA deployments predominantly enable use_datadogmetric_crd (CRD path) vs the default legacy ConfigMap store? Code confirms the default is ConfigMap (use_datadogmetric_crd=false), so the RMW path is default-reachable; the adoption skew itself is a product fact not answerable from code. `(needs human input)`


### extmetrics-crd-store-converges-after-flip — DatadogMetric CRD store converges after a leadership flip

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Liveness |
| **Assertion** | `Sometimes` |
| **Intent** | invariant |
| **Property** | After a leadership flip, the newly-elected leader's in-memory DatadogMetricsInternalStore eventually reflects reality for every DatadogMetric that has a valid value in the CRD and is referenced by an active HPA: a GetExternalMetric query for that metric returns a present, non-stale value (not 'DatadogMetric not found' and not 'DatadogMetric is stale') once the shared informer has resynced and at least one MetricsRetriever/syncDatadogMetric cycle has run on the new leader. |
| **Invariant** | flip_observed && new_leader_settled(elapsed > kubernetes_informers_resync_period + external_metrics_provider.refresh_period) ⇒ store.Get(id) != nil && Valid && !IsStale for every active, CRD-valid DatadogMetric served by the new leader `Sometimes` fits — a liveness/progress condition (the store converges) verified under a recovery window, not on every evaluation. |
| **Antithesis Angle** | Antithesis controls the exact interleaving of (a) the Lease renew/expiry that drives the flip, (b) the shared DatadogMetric informer's resync/list-watch, and (c) the leader-gated MetricsRetriever and AutoscalerWatcher tickers. The convergence window after a flip is opened only by specific orderings — e.g. flip happens right after a resync so the next resync is ~300s away, and the metric is briefly Inactive so the retriever skips it. A workload-only unit test cannot schedule a real Lease flip against a live informer; this is squarely Antithesis's timing/partial-failure domain. |
| **Why It Matters** | GetExternalMetric (provider.go:139) has NO leadership gate — every replica, including a freshly-promoted leader, answers HPA queries straight from its own in-memory store. If the new leader's store is missing an entry (Get returns nil ⇒ 'DatadogMetric not found', provider.go:172-174) or holds a value older than max_age (ToExternalMetricFormat returns 'stale', datadogmetricinternal.go:274-276), the HPA gets an error and stops scaling on that metric. Unlike the clusterchecks store, this store is never reset on a transition, so the failure mode is a silently-stale value rather than an empty store — but the convergence back to reality depends entirely on informer resync + a leader-only refresh cycle firing, which is exactly what a badly-timed flip can delay. |
| **Investigation refinement** | No invalidation; convergence bound confirmed. Resync IS enabled at 300s (apiserver.go:452 + kubernetes_informers_resync_period default 300s) with UpdateFunc->enqueue (datadogmetric_controller.go:90), so the worst-case post-flip re-reconcile is <= max(resync_period 300s, refresh_period 30s). Refinement: GetExternalMetric has no cache-sync gate (provider.go:153), so an additional startup-transient 'not found' window exists before the store first populates — the AlwaysOrUnreachable staleness guard must be evaluated only after the settle window (as already specified) so this transient degrades to Unreachable rather than false-failing. |
| **Fault deps** | leader_election enabled + >=2 DCA replicas + external_metrics_provider.enabled with the DatadogMetric CRD provider (not legacy ConfigMap); a leadership flip: preferred workload-driven substitutes that need NO node-termination — (a) partition current leader <-> kube-apiserver for >= leader_lease_duration (default 60s) to force lease loss, or (b) restart the leader DCA container (container restart is enabled by default), or (c) workload directly mutates/deletes the coordination.k8s.io Lease to trigger re-election; a stub dd-metrics-backend so the new leader's MetricsRetriever can return values (otherwise convergence to 'fresh' cannot be observed); NOT required: node-termination or clock-skew faults (both commonly disabled by default) |
| **Evidence** | `properties/extmetrics-crd-store-converges-after-flip.md` |

**Open Questions:**

- Resolved (user decision, 2026-07-21): yes — the harness pins `use_datadogmetric_crd: true`; this is the PRIMARY external-metrics property.


### extmetrics-backoff-cap-stays-serving — External-metrics backoff reaches its cap and DCA keeps serving

| | |
|---|---|
| **Priority** | P2 |
| **Type** | Reachability |
| **Assertion** | `Reachable` |
| **Intent** | invariant |
| **Property** | Under a prolonged Datadog-backend outage, the external-metrics retriever's per-query exponential backoff reaches its 1800s cap and the DCA keeps serving (stale-marked) metric values rather than crashing or dropping out of the Service. |
| **Invariant** | `assert.Reachable(backoff_reached_cap)` plus a `Sometimes(metric_marked_stale_but_served)`. Reachable fits the primary goal — confirming the max-backoff branch is actually hit under a long outage (a state deterministic tests rarely reach). The 'stays serving' aspect verifies the claimed 'serving stale data is better than no data' guarantee. |
| **Antithesis Angle** | metrics_retriever uses NewExpBackoffPolicy(2,30,1800,...) (metrics_retriever.go:29); a metric in error backoff is skipped until RetryAfter, up to 1800s. Sustained partition DCA<->Datadog backend, long duration; assert the cap is reached and a recovered metric is eventually re-queried, and that the DCA stays Ready and marks metrics stale (command.go:389-392). |
| **Why It Matters** | Confirms the documented degraded-mode guarantee (stay Ready, mark stale) actually holds, and that backoff neither wedges permanently nor drops the DCA from HPA service. Otherwise HPAs silently stop receiving metrics. |
| **Investigation refinement** | Two corrections. (1) SCOPE: the 1800s cap is UNREACHABLE under default config — external_metrics_provider.split_batches_with_backoff defaults to false (common_settings.go:569) and Retries is incremented only in that mode (metrics_retriever.go:165). The Reachable(backoff==1800s) goal requires the harness to set split_batches_with_backoff=true; also rate-limit (429) errors never increment Retries, so a pure-429 outage never advances the backoff. (2) ASSERTION: the 'Sometimes(metric_marked_stale_but_served)' sub-assertion is contradicted on the CRD/HPA path this retriever feeds — ToExternalMetricFormat returns an error for Valid=false (datadogmetricinternal.go:267-276; provider.go:176), so the HPA gets an error, not a stale value. On this path only 'DCA process stays alive/Ready and backoff stays bounded' holds; 'serving stale values' applies to the WPA/ConfigMap bundle path instead. |
| **Fault deps** | sustained network partition DCA<->Datadog metrics backend, long duration (enabled by default); requires external_metrics_provider enabled + leader |
| **Evidence** | `properties/extmetrics-backoff-cap-stays-serving.md` |

**Open Questions:**

- Does reaching the 1800s cap (~15-30+ min of sustained outage) fit a single Antithesis timeline? The backoff constants (2,30,1800,2) are a hardcoded package var at metrics_retriever.go:29 and are NOT config-driven — only external_metrics_provider.refresh_period (30s) is configurable, so shortening the cap for the test requires a code change, not config. `(needs human input)`


### extmetrics-crd-status-no-regression-across-flip — DatadogMetric status does not regress across a leadership flip

| | |
|---|---|
| **Priority** | P2 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | invariant |
| **Property** | When a newly-promoted leader writes a DatadogMetric CRD status (UpdateStatus), the persisted status Value/DataTime is never older than the status that was already present in the CRD: the IsNewerThan guard prevents a stale in-memory store (inherited from the follower's blind CRD-mirroring) from overwriting a fresher status that the previous leader had committed. |
| **Invariant** | for every UpdateStatus call on the new leader: datadogMetricInternal.IsNewerThan(existing CRD status) == true; equivalently the committed status's Active-condition LastUpdateTime is monotonically non-decreasing across leadership flips `AlwaysOrUnreachable` fits — the CRD-status write path runs only when the DatadogMetric provider is active (optional), but whenever a status write occurs it must not regress. |
| **Antithesis Angle** | The regression window opens only if a flip is scheduled such that the new leader reconciles-and-writes before its retriever refreshes the value, using a store entry whose UpdateTime was reconstructed from the CRD by the follower path (NewDatadogMetricInternal). Antithesis can interleave the flip, the informer delivery of the last leader's UpdateStatus, and the new leader's first reconcile to probe whether IsNewerThan's 1-second (.Unix()) granularity or a reconstructed UpdateTime lets an equal/older status slip through as an update. |
| **Why It Matters** | A regressed CRD status re-published to all followers (who mirror the CRD into their stores) would propagate a stale Value cluster-wide, and HPAs reading any replica would scale on old data. IsNewerThan (datadogmetricinternal.go:192-204) gated at datadogmetric_controller.go:274 is the only thing preventing this; confirming it holds under real flip timing — and witnessing that the update path was actually exercised post-flip — is the point. |
| **Investigation refinement** | Assertion refined. The IsNewerThan guard (datadogmetricinternal.go:192-204) enforces monotonicity of the Active-condition LastUpdateTime/internal UpdateTime only, at whole-second (.Unix()) granularity with a conservative `>=` tie-reject. So: (a) the safe/green assertion should target Active-condition/UpdateTime monotonicity (which the guard enforces) — NOT DataTime; (b) DataTime is stamped independently from BuildStatus (:249) and is NOT covered by the guard, so a DataTime regression is reachable when a follower mirrored a staler CRD and AutoscalerWatcher's Active-only UpdateTime bump then lets the guard pass — this is the genuine bug-probe. The '.Unix() lets an equal-second update overwrite with an older Value' hypothesis is refuted (equal-second is rejected). |
| **Fault deps** | same as the convergence property: leader_election + >=2 replicas + DatadogMetric CRD provider + stub dd-metrics-backend; a leadership flip via apiserver partition (>=60s), leader container restart, or Lease mutation — no node-termination/clock-skew needed; to stress the .Unix() granularity, the ability to drive two flips within a short window (workload-controlled Lease churn) |
| **Evidence** | `properties/extmetrics-crd-status-no-regression-across-flip.md` |

**Open Questions:**

- Can an informer-lagged store entry (follower mirrored a STALER CRD than the last committed status) combined with AutoscalerWatcher's Active-only UpdateTime bump (autoscaler_watcher.go:~224) make IsNewerThan pass and republish an OLDER DataTime? The guard inspects only the Active-condition timestamp, never DataTime, so it does NOT prevent this; needs the harness to schedule the lag+bump interleaving to demonstrate whether it is actually reachable. `(partial)`


## Protocol, API & Auth Contracts

Node-agent HTTP contract, the dual-token boundary, and first-boot ConfigMap convergence.


### no-404-on-registered-cluster-check-routes — Enabled cluster-check routes never return the ServeMux default 404

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `Always` |
| **Intent** | known-defect-reproducer |
| **Property** | Once the DCA API listener accepts connections, every node-agent-facing cluster-check route resolves to an installed handler — never the ServeMux default 404 — even in the window between Serve() starting and the routes being installed. |
| **Invariant** | `assert.Always(no_default_404_on_clusterchecks_path)`: a request to an enabled /api/v1/clusterchecks/* path never receives the literal ServeMux '404 page not found'. Always fits — the routing guarantee must hold on every request once the listener is up. (Distinct from a handler-level 503 'startup in progress', which is a valid, intended response.) |
| **Witness** | `Reachable`: a clusterchecks request arrived in the Serve()-before-route-registration window. |
| **Antithesis Angle** | The API server starts (go srv.Serve) at command.go:368, but cluster-check routes are installed later via ModifyAPIRouter at command.go:534 — after WaitForAPIClient blocks on the apiserver. A node agent hitting the endpoint in that gap gets a bare 404 (indistinguishable from a real misconfiguration) and it is a live http.ServeMux mutation (data-race concern). Inject apiserver latency to widen the WaitForAPIClient block and race node-agent polls against route registration. |
| **Why It Matters** | A 404 during the registration window makes node agents treat the endpoint as absent (vs retrying a 503), potentially disabling cluster-check polling; the concurrent mux mutation is also a latent data race. Merged from 2 focus agents. |
| **Investigation refinement** | Refinement (not invalidating): the 404 gap is reachable only by an authenticated (DCA-token) node-agent request, because validateToken wraps the router and returns 401/403 before routing — assertion should fire in the auth-passed handler/wrapper and the R1 `Reachable` witness must require an authenticated /api/v1/clusterchecks/* request during the Serve()-to-ModifyAPIRouter window. Window size confirmed effectively unbounded (apiserver Backoff retrier never PermaFails). |
| **Fault deps** | network latency/congestion on DCA<->apiserver to widen the startup window (enabled by default); concurrency (mux mutation vs Serve); requires leader_election enabled |
| **Evidence** | `properties/no-404-on-registered-cluster-check-routes.md` |

**Open Questions:**

- Node-agent behavior on an authenticated 404 vs 503 (retry vs disable cluster-check polling) — lives in node-agent code, out of DCA/SUT scope; governs blast radius, not existence of the gap. `(needs human input)`


### empty-token-never-authenticates — An empty auth token never authenticates any caller

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `Always` |
| **Intent** | invariant |
| **Property** | The Cluster Agent never authorizes a request while its configured auth token (DCA token or local IPC token) is the empty string. |
| **Invariant** | `assert.Always(configured_token != "" whenever a request is authorized)`: the token-compare path never succeeds against an empty configured token. Always fits — an absolute security invariant on every authenticated request. |
| **Witness** | `Reachable`: an authenticated request arrived while the configured token was still empty (startup window). |
| **Antithesis Angle** | Tokens are read/created at startup from a ConfigMap or file (validateToken, server.go:174-196). Inject filesystem/IO latency or error during startup, or an apiserver failure during token ConfigMap read, so the token is momentarily empty while the API server (started early, command.go:368) is already accepting connections. Assert no request is authorized against an empty token. |
| **Why It Matters** | If an empty token ever compares equal (e.g. a caller also sending an empty token, or a constant-time compare of two empty strings), the DCA's entire node-agent/IPC trust boundary collapses — any caller is authorized. A hard security floor. |
| **Investigation refinement** | Core defect confirmed (property valid): TokenValidator (util_dca.go:122) authorizes `Bearer ` (empty) when the configured token is "" — tok=["Bearer",""], len==2 passes, constantCompareStrings("","")==true; gRPC path server.go:109 ConstantTimeCompare("","")==1 likewise authorizes. Reachability model corrected: the empty-token condition is NOT a startup ordering race (init precedes Serve inside StartServer); it requires InitDCAAuthToken to return an error (fault pushing FetchOrCreateArtifact past auth_init_timeout=30s, error discarded at server.go:95), and it is then DURABLE for the process lifetime, not a transient window. R1 witness should be 'authenticated request served while dcaToken=='' due to init failure'. Scope narrowed to the DCA-token surface only — the local IPC token cannot be empty in the server (NewComponent fails fast). |
| **Fault deps** | filesystem/IO latency or error injection during startup (enabled by default); apiserver failure during token ConfigMap read; clock jitter (not required) |
| **Evidence** | `properties/empty-token-never-authenticates.md` |

**Open Questions:**

- Whether any real deployment leaves cluster_agent.auth_token unset AND the token-file directory (ConfFileDirectory, security.go:204) unwritable (read-only rootfs), making the DCA token durably empty rather than transiently — a deployment/ops call. `(needs human input)`


### configmap-concurrent-create-converges — Replicas converge on a single token and cluster ID at first boot

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | known-defect-reproducer |
| **Property** | Whenever two DCA replicas each successfully obtain a cluster ID via GetOrCreateClusterID (from the datadog-cluster-id ConfigMap), the two values are byte-identical. A concurrent first-boot create race may produce a spurious error on the losing replica, but it can never produce two different cluster-ID values. |
| **Invariant** | for all pairs of successful GetOrCreateClusterID results r_a, r_b across replicas: r_a == r_b (and each == UID(namespace kube-system)) `AlwaysOrUnreachable` fits — the create branch runs only on first boot / when the ConfigMap is absent (optional path), but whenever two replicas create concurrently they must converge to a single value. |
| **Witness** | `Sometimes`: At least once, both DCA replicas observed the datadog-cluster-id ConfigMap as NotFound and both attempted to Create it, i.e. one replica's Create returned AlreadyExists. This proves the concurrent-create hazard window was actually scheduled, so the convergence invariant above was tested non-vacuously. |
| **Antithesis Angle** | Schedule dca-1 and dca-2 to both execute GetOrCreateClusterID (command.go:433) before the datadog-cluster-id ConfigMap exists, against a SHARED real kube-apiserver. Both Get -> NotFound, both compute GetKubeSystemUID (common.go:59), both Create. Antithesis controls the interleave so the second Create lands after the first has committed. Assert the two persisted/returned IDs match. The value is deterministic (kube-system namespace UID, common.go:32-38), so convergence should hold structurally; this pins that no code path (e.g. a future switch to a random UUID) reintroduces divergence. |
| **Why It Matters** | The cluster ID is the cluster's stable identity used to correlate all telemetry (orchestrator, metadata, KSM). Two replicas disagreeing on it would split a single cluster into two identities in the backend. The current implementation is safe by determinism, but that safety is an emergent property of 'ID = kube-system UID', not an enforced invariant; this assertion locks it in. |
| **Investigation refinement** | Convergence invariant confirmed, NOT invalidated: cluster ID = kube-system namespace UID (common.go:37), a deterministic cluster-wide constant, so two successful results are byte-identical by construction (AlwaysOrUnreachable holds structurally). Known-defect-reproducer severity raised: the create-race loser returns ("",err) — common.go:73-77 has no errors.IsAlreadyExists branch (only IsNotFound is handled on the Get) — and command.go:433-436 proceeds non-fatally with clusterID="", propagating the empty ID into the RC client, workload/cluster autoscaling, spot scheduling, and kubeactions (command.go:502-656) for the process lifetime. The loser does not cache on the error path (common.go caches only on success), so a later independent GetOrCreateClusterID would self-heal, but the in-frame consumers do not re-fetch. |
| **Fault deps** | None beyond concurrent startup of >=2 DCA replicas against a SHARED real kube-apiserver (no fake clientset). Node-termination and clock-skew NOT required. |
| **Evidence** | `properties/configmap-concurrent-create-converges.md` |

**Open Questions:**

- Harness verification only: does the harness co-locate both DCA replicas in the same namespace so they contend on the same datadog-cluster-id ConfigMap? Code confirms GetOrCreateClusterID uses namespace.GetMyNamespace() (own pod namespace, common.go:50), so same-Deployment replicas contend; the placement itself is a harness-config check. `(partial)`
- Does WaitForAPIClient (command.go:376) unblocking near-simultaneously for both replicas measurably widen the collision window? Code confirms both replicas gate on it and proceed to GetOrCreateClusterID at command.go:433, so it synchronizes them; the magnitude of widening is a harness measurement. `(partial)`
- In Operator/Helm deployments is datadog-cluster-id ever pre-created, making the NotFound/Create branch unreachable in production? External chart config, not in this repo. `(needs human input)`


### getconfigs-distinguishes-unknown-node — Unknown-node config poll is distinguishable from a server error

| | |
|---|---|
| **Priority** | P2 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | known-defect-reproducer |
| **Property** | A node agent polling GET configs for an unknown/expired node receives a response distinguishable from a genuine internal server error, so it can re-register rather than treating a transient 500 as a hard failure. |
| **Invariant** | Detects the ambiguity: TODAY the unknown-node branch returns HTTP 500 (dispatcher_nodes.go:33-35), identical to genuine server errors, so this assertion FAILS against current code by design — the deliverable is the reproducing trace plus the argument for a distinct code. `AlwaysOrUnreachable` on the unknown-node branch asserting the response is a distinct 4xx-style code, not 500. Its value is gated on the workload modeling node-agent backoff-on-error (a workload requirement, not a scope footnote). |
| **Witness** | `Reachable`: an unknown/expired node polled GET configs. |
| **Antithesis Angle** | An unknown node → error → HTTP 500 (clusterchecks.go / dispatcher_nodes.go:33-35), the same code as genuine failures. A node whose heartbeat expired (partition) then polls GET before re-POSTing status gets a 500 and cannot tell 'you must register' from 'leader is broken.' Reorder POST/GET or expire a node then poll. |
| **Why It Matters** | Ambiguous error codes make node-agent retry/backoff logic misbehave — a node may back off hard on a benign 're-register' signal, extending cluster-check gaps. A protocol-contract clarity property. |
| **Investigation refinement** | None. Core assertion confirmed against current code: unknown/expired node → HTTP 500 (dispatcher_nodes.go:34, clusterchecks.go:83), identical to genuine internal errors, so the property FAILS by design as intended (known-defect-reproducer). No distinct 4xx exists. |
| **Fault deps** | network partition / request reordering (POST vs GET; enabled by default); clock skew on wall-clock expiry (DISABLED by default); requires leader_election enabled + >=2 replicas |
| **Evidence** | `properties/getconfigs-distinguishes-unknown-node.md` |

**Open Questions:**

- Node-agent client retry/backoff on 500 vs 4xx for GET configs — determines whether an unknown-node 500 merely delays or fully stalls config propagation. Node-agent code, out of DCA/SUT scope. `(needs human input)`
- Whether any production monitor/SLO pages on DCA 5xx rate (would make the false-500 operationally severe) — observability config, not in repo. `(needs human input)`


### isexternalpath-classifier-consistency — Auth-path classifier matches each endpoint's intended token

| | |
|---|---|
| **Priority** | P2 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | invariant |
| **Property** | Every node-agent-facing endpoint is classified 'external' (requires DCA token) and every intra-pod IPC endpoint is classified non-external (accepts the local token), so the hand-maintained exact-segment-count classifier never mis-authorizes or silently rejects a registered endpoint. |
| **Invariant** | `assert.AlwaysOrUnreachable`: for each served route, isExternalPath's verdict matches the route's intended auth class. AlwaysOrUnreachable fits — evaluated per registered route (optional set), must hold whenever a route is classified. Best exercised by a workload enumerating endpoints with each token. |
| **Antithesis Angle** | isExternalPath (server.go:199-219) uses prefix + exact segment-count checks (==6, ==7); any path not matching falls back to requiring the local token, so a mis-classified new/edge endpoint silently rejects node agents (or, worse, mis-authorizes). Not fault-timing dependent — a workload sends requests with the wrong/right token to each endpoint (including trailing-slash and extra-segment variants) and asserts the expected accept/reject. |
| **Why It Matters** | A silent auth misclassification either breaks node-agent connectivity (endpoint rejects the DCA token path) or weakens the trust boundary. The brittle segment-count logic is a maintenance hazard as endpoints are added. |
| **Investigation refinement** | Assertion design refined (property stands, non-vacuous): (1) ground truth for 'intended auth class' must come from client call sites (DCAClient methods → external; ipc.HTTPClient CLI → internal), not a server declaration — isExternalPath is the sole server-side source. (2) Concrete discriminating audit finding: /api/v1/info/node/{nodeName} (DCAClient.GetNodeInfo, clusteragent.go:411, used by cloudprovider.go:53) is a DCA-token endpoint absent from isExternalPath, so it is classified non-external. PLAUSIBLE live defect worth a workload probe: for a non-external path hit with only the DCA token, validateToken calls localTokenGetter first, which on failure writes http.Error 403 (util_dca.go:109/124) to the ResponseWriter BEFORE the DCA-token fallback (server.go:188-192) succeeds — so the DCA-token fallback may not cleanly mask the misclassification (response status already committed 403). (3) Classifier keys on r.URL.String() (query-inclusive) rather than r.URL.Path — latent inconsistency. |
| **Fault deps** | none required (input-domain property; not fault-timing dependent); needs a workload that presents both tokens against each endpoint and path variant |
| **Evidence** | `properties/isexternalpath-classifier-consistency.md` |

**Open Questions:**

- None — all resolved during investigation (see evidence file Investigation Log)


## gRPC Streaming Data Plane

The tagger + kube-metadata server-streaming RPCs on the main port — subscription-registry accounting across reconnect and leadership flip (regression #48026/#50670).


### grpc-stream-subscription-accounting — gRPC stream subscriptions are never leaked or dropped on reconnect

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | invariant |
| **Property** | When a node agent's StreamKubeMetadata stream drops and the node reconnects (opening a second, overlapping stream for the same nodeName) before the old stream's deferred Unsubscribe runs, the old handler's cleanup must remove only the channel it created and must never remove the newly-registered channel; every currently-live stream's notify channel remains registered in both the MetaBundleStore.subscribers registry and the namespaceSubscribers registry for as long as that stream runs. |
| **Invariant** | assert.AlwaysOrUnreachable: for every nodeName, at any observation point the set of channels present in subscribers[nodeName] (and namespaceSubscribers[nodeName]) is exactly the set of channels created by currently-live StreamKubeMetadata handlers for that node — no live handler's channel is absent (no permanent drop), and no returned handler's channel is still present (no leak). Equivalently: len(subscribers[nodeName]) == count of live handlers for nodeName. AlwaysOrUnreachable fits because the overlapping-reconnect path is optional (a run with no stream churn never opens it), but whenever a reconnect overlaps the previous stream's teardown the registry accounting must hold. |
| **Witness** | `Sometimes`: During the run, at least once two distinct live StreamKubeMetadata handlers for the same nodeName are simultaneously registered in the MetaBundleStore (subscribers[nodeName] has length >= 2), proving the hazardous reconnect-overlap window that the no-drop safety invariant guards was genuinely exercised — not merely never opened. |
| **Antithesis Angle** | This is the exact bug fixed by #48026 (2b7eb1ece36) and #50670 (8b12b036cea): originally subscribers was map[string]chan struct{}, so a reconnecting node's new Subscribe(nodeName) overwrote the map entry and the OLD handler's `defer Unsubscribe(nodeName)` then deleted the NEW channel — the new stream received the initial full-state snapshot and keepalives but never any diff (permanently dropped subscription, silent). The fix relies on precise interleaving of two goroutines (old handler's deferred cleanup vs new handler's Subscribe) for the same nodeName. Antithesis controls goroutine scheduling and can drive the workload (impersonating one node identity) to drop and immediately reopen the stream repeatedly so the new Subscribe reliably lands before the old defer runs — the window a race-detector unit test can only hit by hand-ordering. No node-termination or clock-skew fault is required: a client-driven stream close (or a workload<->DCA partition that RSTs the old stream) is a sufficient substitute. |
| **Why It Matters** | A silently dropped subscription means the node agent's pod-to-service mappings and namespace/Kueue metadata stop updating (only the stale initial snapshot survives) with no error surfaced — tags and metadata quietly go stale cluster-wide for that node. It was a real reported main-branch regression (reported by @gabedos, #48026). Making it a permanent Antithesis regression guard is high value because the fix is entirely interleaving-dependent and invisible to the existing single-goroutine unit tests. |
| **Investigation refinement** | Scope refinement (not invalidation): the reproducible drop/leak hazard this AlwaysOrUnreachable invariant guards is specific to the nodeName-KEYED kube-metadata registries (subscribers via m.mu and namespaceSubscribers via metadataMutex), which reuse a stable identity across reconnects (client stream.go:488,657). The tagger registry cannot exhibit the same collision because subscriptionID uses a fresh uuid per stream (impl-remote/remote.go:646-648), so the tagger arm reduces to a no-panic / balanced-count guard (regression guard for #40968 map-race), not the drop-on-reconnect bug. Instrument/assert both kube-metadata registries independently (Q8). The tagger arm's activation is additionally gated on the harness running a DCA tagger consumer (Q5, still open). |
| **Fault deps** | workload-driven stream drop+reconnect for the same nodeName (client close / context cancel) — NO node-termination or clock-skew fault required; this is the workload substitute; optional: asymmetric network partition workload(as node agent)<->DCA to force the old stream to RST while the new one connects (enabled by default); concurrency / goroutine interleaving (always on) — the core enabler; does NOT require leader_election or >=2 replicas (stream serving is not leader-gated), though a >=2-replica run adds reconnect churn via leadership-driven client failover |
| **Evidence** | `properties/grpc-stream-subscription-accounting.md` |

**Open Questions:**

- Is the DCA tagger gRPC stream actually consumed in the target harness topology? Code shows node agents' remote tagger dials the LOCAL core agent (remote.go start()/options.Target), while the DCA tagger server is consumed by in-cluster DCA-targeting clients via cluster_agent.cluster_tagger (e.g. cluster-check runner). Confirm whether the harness runs such a consumer, else the tagger arm of this property is inert. `(partial)`


## External Dependency / Informer Freshness

The API-server dependency and the informer caches that back tags, metadata, endpoints, secrets, and CRDs — the root-cause `informer_client_timeout=0` freeze.


### informer-fresh-or-staleness-surfaced — DCA does not silently serve authoritative data from a frozen informer

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | known-defect-reproducer |
| **Property** | When a DCA replica is partitioned from the kube-apiserver such that a watch stream is silently dropped (no RST/FIN), the DCA must not keep serving informer-backed data (admission cert, kube-service/endpoint metadata, CRD state) as authoritative and current while continuing to report Ready/healthy. Either the lister-served value converges to apiserver ground truth within a bounded staleness budget, or the DCA surfaces the staleness (explicit error, staleness metric, or readiness=unready). |
| **Invariant** | assert.AlwaysOrUnreachable(informer_backed_value_is_fresh_or_staleness_is_surfaced): at every point where an informer-backed lister value is used on a decision path AND the DCA reports healthy, the backing informer has observed a successful watch event or resync within a bounded window B (or the value equals current apiserver ground truth). AlwaysOrUnreachable fits because the informer paths are optional (admission/metadata/CRD may be disabled) but whenever exercised the freshness-or-surfaced invariant must hold. EXPECTED TO FAIL today: there is no post-startup freshness check (HasSynced stays true forever after the initial WaitForCacheSync), no staleness metric, and readiness is not tied to informer liveness, so under a watch-drop partition the DCA serves stale data silently — that failing trace is the deliverable. |
| **Witness** | `Reachable`: The hazardous precondition for the root-cause property is actually scheduled: during a silent watch-drop partition, a DCA replica's informer-backed lister returns a value that differs from the current apiserver ground truth for the same object (i.e., the cache genuinely froze and the staleness window opened). |
| **Antithesis Angle** | kubernetes_apiserver_informer_client_timeout defaults to 0 (common_settings.go:2039, core_schema.yaml:9180). That flows to defaultInformerTimeout (apiserver.go:183), is handed to every informer client (apiserver.go:402-432), and becomes rest.Config.Timeout=0 (GetClientConfig, apiserver.go:266); the CustomRoundTripper wrap only logs timeouts, it adds no deadline (roundtrip.go:37-47). So informer watch requests have no client-side timeout. Inject an asymmetric/blackhole partition (drop packets, no RST) between one DCA replica and the apiserver, then MUTATE the watched object on the apiserver from the workload: rotate the admission cert Secret (admission_controller.certificate.secret_name), change the DCA Service EndpointSlice, or update kube-service endpoints. Assert the replica's lister-served value converges to the new ground truth within B, or the replica flips unready / surfaces staleness. This is a state only Antithesis reaches: a silent watch drop concurrent with a real data change — a fake clientset cannot reproduce it. |
| **Why It Matters** | This is the ROOT CAUSE behind several symptom properties (stale admission cert -> opaque TLS failures or wrong cert after rotation; stale endpoint/service metadata; stale CRD-derived state). A frozen informer surfaces no error and keeps the pod Ready, so operators see a healthy DCA silently serving wrong data cluster-wide with no alert. Because HasSynced latches true after the one-time startup sync, there is no built-in freshness signal anywhere. |
| **Investigation refinement** | REFINEMENT (does not invalidate): The Open-Q1/Q5 and Antithesis-angle premise that a 0 client-side informer timeout leaves the watch hanging indefinitely is incomplete. client-go's default transport enables an HTTP/2 connection health-check ping (ReadIdleTimeout=30s, PingTimeout=15s; k8s.io/apimachinery util/net/http.go:187-188, wired via client-go transport/cache.go:134), and the agent does not disable it. So a silent blackhole is detected in ~30-45s and forces a relist; the stale window is bounded by the partition duration (self-heals on partition end), not unbounded. The core defect still holds and the property stands: HasSynced latches true after startup, there is no staleness metric, and readiness only drains a liveness ping (admission server.go:176) with no informer-freshness gate — so during the partition the DCA silently serves stale lister values (cert per TLS handshake, endpoint/metadata to node agents) while reporting Ready. Recommend the assertion's staleness budget B be set to bound the DURING-PARTITION window (e.g. detection ~45s plus partition length), and drop any 'freeze is indefinite' framing. |
| **Fault deps** | network partition DCA-replica <-> kube-apiserver, silent/blackhole drop (no RST/FIN) so the watch stream hangs — enabled by default on most tenants; workload-driven mutation of the watched object during the partition (rotate admission Secret / change DCA EndpointSlice / update kube-service endpoints) — the topology already gives the workload apiserver ownership of Service/EndpointSlice objects; no node termination or clock skew required; requires whichever informer path is enabled: admission_controller.enabled, or kube-service metadata, or a CRD controller |
| **Evidence** | `properties/informer-fresh-or-staleness-surfaced.md` |

**Open Questions:**

- Which staleness budget B is defensible per surface? No B is defined anywhere in code (HasSynced latches, no staleness metric). Two natural bounds exist: (a) ~45s = HTTP/2 dead-connection detection window (relist trigger); (b) the per-surface correctness window (cert rotation vs endpoint churn). Choosing the authoritative B is an intended-behavior call. `(needs human input)`
- Which surface (admission Secret / EndpointSlice / kube-endpoints / CRD) most reliably opens the freeze window under the harness partition primitive? From code the admission cert is the most deterministic decision-path read (read from secretsLister on every TLS handshake, server.go:137-140); metadata is served from a reconciled store fed by informer events. Ranking requires empirical harness measurement. `(partial)`


## Lifecycle Transitions

Startup ordering, graceful shutdown under partition, cert/fatal-path behavior, and webhook availability under churn.


### graceful-shutdown-releases-lease-bounded — Shutdown completes and releases the lease within a bound, even under partition

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Liveness |
| **Assertion** | `Always` |
| **Intent** | known-defect-reproducer |
| **Property** | After SIGTERM the DCA completes shutdown and releases its leader lease within a bounded deadline, even when the apiserver is partitioned, so failover is not delayed the full lease duration. |
| **Invariant** | `assert.Always` (bounded-time liveness): after SIGTERM under an apiserver partition, the process completes shutdown and the lease is released-or-expires within a bounded deadline. Framed as Always-within-deadline so a HANG in the ReleaseOnCancel network call FAILS the assertion. (Evaluation caught the original Sometimes(released) as an assertion-shape inversion — a success-witness can never fire on 'sometimes it hangs', which is the actual bug.) |
| **Witness** | `Reachable`: SIGTERM was delivered while the leader was partitioned from the apiserver (ReleaseOnCancel attempted under partition). |
| **Antithesis Angle** | ReleaseOnCancel (leader_election_release_on_shutdown, default true) makes a network call on ctx-cancel to shorten the lease to 1s (leaderelection_engine.go:196-198). If shutdown coincides with an apiserver partition, that call hangs/fails → the lease is neither released nor renewed → full LeaseDuration (60s) gap with no leader. Partition the leader<->apiserver, then SIGTERM it. |
| **Why It Matters** | A shutdown that hangs on the release call blocks the pod's termination and delays failover by up to a full lease duration — during a rolling upgrade this stacks up as cluster-wide leader-gated downtime. |
| **Investigation refinement** | INVALIDATION of the core mechanism as framed. The premise 'ReleaseOnCancel hangs/blocks shutdown indefinitely under partition' is false for client-go v0.35.5: (a) release() runs on a fresh context.Background() bounded by RenewDeadline (LeaseDuration/2 = 30s default) at leaderelection.go:311-335, so it is time-bounded, not indefinite; (b) the leader-election goroutine is bare (leaderelection.go:199) and NOT awaited by the shutdown wg (command.go:746), so the release cannot block pod termination at all. The residual REAL risk is a different failure: if the process exits before the bounded release completes, the lease is not shortened and the successor waits up to full LeaseDuration (failover gap) — an abandoned-release, not a hang. Recommend re-framing the assertion away from 'shutdown completes within bound' (already guaranteed) toward the failover gap: lease is shortened OR successor still acquires within ~LeaseDuration. |
| **Fault deps** | network partition (leader<->apiserver; enabled by default); SIGTERM delivery (ordinary shutdown; node termination graceful mode); requires leader_election enabled + >=2 replicas |
| **Evidence** | `properties/graceful-shutdown-releases-lease-bounded.md` |

**Open Questions:**

- Do long-lived gRPC/HTTP streams (tagger/kube-metadata) or an fx OnStop hook prolong process exit? (Largely moot: the release call is bounded and not awaited, so no indefinite hang either way.) Confirmed only that shutdown does NOT wait on the leader-election goroutine (command.go:746 wg covers only extmetrics+admission) and StopServer() is never called; the primary API/gRPC listener drain path on exit was not fully traced. `(partial)`


### admission-webhook-available-under-churn — The DCA that should serve the admission webhook stays available under churn

| | |
|---|---|
| **Priority** | P1 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | invariant |
| **Property** | Whenever a DCA replica is running the admission webhook HTTPS server, has synced its secret informer at least once, and the admission cert Secret exists in the apiserver, a TLS handshake against that replica completes with a NON-nil certificate — so the DCA does not needlessly fail its webhook (and, under failurePolicy=Fail, needlessly block cluster-wide pod creation) merely because leadership moved or the apiserver was briefly partitioned. Serving is not leader-gated: this must hold on the leader AND on followers. |
| **Invariant** | assert.AlwaysOrUnreachable: at each GetCertificate invocation (cmd/cluster-agent/admission/server.go:137-144), if the cert Secret is present in the replica's secret-informer cache (GetCertificateFromLister returns err==nil), the returned *tls.Certificate is non-nil. Corollary asserted from the workload: while the cert Secret exists in the apiserver (workload-observable ground truth) and >=1 DCA replica is in the webhook Service's Ready endpoints, an AdmissionReview probe POSTed to the webhook Service completes a TLS handshake and returns a valid AdmissionResponse — regardless of which replica is the lease leader and regardless of an in-flight leader<->apiserver partition. AlwaysOrUnreachable fits: the webhook path is optional (admission_controller.enabled + a request must arrive), but every served handshake, once reached, must not needlessly fail-closed while the cert is available. |
| **Witness** | `Reachable`: At least once per run, a DCA replica completes a webhook TLS handshake (non-nil cert) and produces a valid AdmissionResponse WHILE a leadership-churn or leader<->apiserver partition event is in effect (and ideally while the serving replica is NOT the lease leader). This proves the hazardous availability window from admission-webhook-available-under-churn was actually scheduled, so the paired AlwaysOrUnreachable is not a vacuous green. |
| **Antithesis Angle** | The webhook HTTPS server is started on EVERY replica gated only on admission_controller.enabled (command.go:648), with no leader check on server.Run (command.go:712); the cert is fetched per-handshake from a per-replica informer-backed secretsLister (server.go:140). Cert creation/rotation, however, IS leader-gated (controller_base.go:286,299 early-return when !isLeaderFunc). informer_client_timeout=0 (sut-analysis §7) means a leader<->apiserver partition can silently freeze a replica's secret informer with no error. Antithesis can: (1) partition a replica from the apiserver and keep sending webhook requests — a synced replica's lister must keep returning the cached cert (client-go does not evict on watch disconnect), so the handshake must still succeed; (2) churn leadership (partition >= lease duration, or terminate the leader) while webhook traffic flows and assert followers still serve; (3) drive a leader-gated cert ROTATION while a follower is partitioned to probe whether the follower serves a cert the freshly-rotated CABundle no longer trusts. Readiness ('admission-controller-webhook', server.go:91,176) is drained by the Run loop independent of cert state, so a Ready replica still in the Service endpoints can serve a nil cert — the exact needless fail-closed this asserts against. This is unreachable in the existing single-process, fake-clientset unit tests (no real informer freeze, no multi-replica churn, sut-analysis §9). |
| **Why It Matters** | This is the availability half of the admission story (the existing admission-webhook-no-silent-nil-cert property is the code-contract half: 'fail loudly, never (nil,nil)'). Here the concern is operational: with admission_controller.failure_policy=Fail, a needless handshake failure on a Ready, Service-selected replica blocks ALL matching pod creation cluster-wide with a misleading TLS error; with the default Ignore it silently drops the intended mutation. Because serving is not leader-gated but cert maintenance is, leadership churn is a real trigger for a serving replica whose view of the cert diverges from the leader's. Antithesis is the only way to hold a frozen-informer + churn + concurrent-traffic state. |
| **Investigation refinement** | Scope refinement, no invalidation. Q2 resolves in the invariant's favor: a never-synced replica cannot serve a nil cert because webhook-server startup is gated behind SyncInformers/WaitForCacheSync of the Secrets informer (start.go:154 → command.go:690), removing one hypothesized 'fresh-replica fail-closed' violation and strengthening the 'synced replica keeps serving' invariant (Q4 confirms cache retention across disconnect). The remaining live violation candidate is rotation-during-churn (Q3), not fresh-start. |
| **Fault deps** | network partition leader<->apiserver (enabled by default) to freeze a replica's secret informer and to churn leadership >= leader_lease_duration; node termination of the leader (DISABLED by default) — only needed for the crash-based churn variant; partition-based churn is a workload-driven substitute; workload must drive admission traffic: POST AdmissionReview probes to the webhook Service endpoint during/after the fault, and observe the cert Secret existence via the apiserver as ground truth; requires admission_controller.enabled + leader_election enabled + >=2 replicas; failure_policy is deploy-config (does not gate the DCA-side assertion) |
| **Evidence** | `properties/admission-webhook-available-under-churn.md` |

**Open Questions:**

- Does the webhook Service (admission_controller.service_name) select ALL DCA pods or only the leader in the harness? DCA code serves on every replica (command.go:648, no leader gate on server.Run :712), but the harness workload 'manages the DCA Service + EndpointSlice' (deployment-topology.md:52) so the selector is a harness-authoring decision not yet pinned. Confirm the shipped Service selector when the harness is built. `(needs human input)`
- Rotation-during-churn: when the leader regenerates the cert AND its CABundle while a follower is partitioned, does the follower's cached old cert remain trusted or get rejected by the new CABundle? Cert rotation is rare (yearly; 30d-before refresh) and CABundle update is leader-gated (controller_base.go:286,299); the code does not guarantee a partitioned follower's old cert matches a freshly-rotated CABundle. Resolving the actual trust outcome needs the cert-controller regeneration/CABundle semantics plus an intended-behavior call. `(partial)`
- Default admission_controller.failure_policy rendered in the harness (code default 'Ignore', common_settings.go:653; harness/Helm value not pinned in topology docs). Sets severity (silent mutation-drop vs cluster-wide pod-creation block); the DCA-side assertion is policy-agnostic. `(needs human input)`
- Witness design: require the serving replica to be a non-leader (stronger, proves not-leader-gated serving) or accept any replica? Pure test-design choice (scratchbook advises 'start permissive, tighten'); no code answer. `(needs human input)`


### admission-webhook-no-silent-nil-cert — Admission webhook never serves a nil cert while swallowing the error

| | |
|---|---|
| **Priority** | P2 |
| **Type** | Safety |
| **Assertion** | `AlwaysOrUnreachable` |
| **Intent** | known-defect-reproducer |
| **Property** | The admission-controller HTTPS server never presents a nil certificate while swallowing the fetch error; every TLS handshake either uses a valid cert or fails loudly with a surfaced error. |
| **Invariant** | `assert.AlwaysOrUnreachable`: the GetCertificate callback never returns (nil,nil). AlwaysOrUnreachable fits — the callback runs per handshake (optional, admission must be enabled), but any invocation must not silently return a nil cert. Today it logs and returns (nil,nil) on error (server.go:141-144), so this is expected to expose the silent-nil path. |
| **Witness** | `Reachable`: the GetCertificate error branch executed (lister miss / frozen informer). |
| **Antithesis Angle** | Cert is fetched per-handshake from a secrets Lister; on error it logs and returns (nil,nil) → handshake proceeds with nil cert, failing opaquely. If the informer cache is stale/unsynced (partition freezes it, informer_client_timeout=0), the served cert can be the old one after rotation, or nil. Partition DCA<->apiserver to freeze the secret informer during a cert fetch. |
| **Why It Matters** | A silent nil cert yields opaque TLS handshake failures that are hard to diagnose; combined with failurePolicy=Fail it can block pod creation cluster-wide with a misleading error. Health/observability accuracy property. |
| **Investigation refinement** | Scope clarification, no invalidation. The (nil,nil) path DOES fail the TLS handshake loudly (errNoCertificates) rather than serving a nil/empty cert — so 'serves a nil certificate' is inaccurate at the transport level; the accurate defect is that the ORIGINAL fetch error is swallowed (logged only, server.go:142-144) making the resulting handshake failure opaque/unattributable to the apiserver. The assertion 'GetCertificate never returns (nil,nil)' remains valid and worth testing. |
| **Fault deps** | network partition between DCA and apiserver (freezes secret informer; enabled by default); requires admission_controller.enabled + leader |
| **Evidence** | `properties/admission-webhook-no-silent-nil-cert.md` |

**Open Questions:**

- Default admission_controller.failure_policy actually rendered by the Helm chart / Operator in the harness (code default is 'Ignore', common_settings.go:653; the shipped/harness-rendered value lives in external repos and is not pinned in antithesis/scratchbook/deployment-topology.md). Governs severity only. `(needs human input)`


### autoscaling-fatal-startup-crashloop — Autoscaling startup failure is a clean fatal exit, not a partial run

| | |
|---|---|
| **Priority** | P2 |
| **Type** | Reachability |
| **Assertion** | `Reachable` |
| **Intent** | invariant |
| **Property** | When autoscaling is enabled but the cluster name cannot be resolved or the remote-config client is unavailable, startup returns a fatal error and the process exits cleanly (pod crash-loops) rather than running in a half-initialized state. |
| **Invariant** | `assert.Reachable(autoscaling_fatal_startup_path)`: under a startup fault that denies cluster name / remote config, the fatal return path (command.go:438-440, ~:565-607) is reached and the process exits. Reachable fits — the goal is to confirm the fatal path is actually taken (not silently swallowed) under fault, steering Antithesis to the startup-failure region. |
| **Antithesis Angle** | Most subsystems fail soft (log+continue), but autoscaling paths call return errors.New(...) → fatal. Inject apiserver latency/partition during startup so cluster name / remote config init fails; assert the process exits rather than serving a partially-initialized autoscaling controller. |
| **Why It Matters** | A half-initialized autoscaling controller could serve wrong/empty HPA data silently; a clean crash-loop is the intended, observable failure. Confirms fail-fast is honored under startup faults. (Lower Antithesis value — near-deterministic — but cheap and confirms the fail-fast contract.) |
| **Investigation refinement** | No invalidation; confirms the Reachable invariant. Minor scope note: the fault-driven fatal path is the cluster-name one (command.go:437-440), since GetRFC1123CompliantClusterName returns '' promptly under a startup partition and caches it. The rcClient==nil fatals (:565/:579/:600) are config/fx-gated rather than transient-partition-driven (NewClient is local), so they are less relevant to the 'transient fault escalates to crash-loop' framing. |
| **Fault deps** | network partition / latency (leader<->apiserver) during startup (enabled by default); node termination to exercise restart (DISABLED by default); requires autoscaling enabled |
| **Evidence** | `properties/autoscaling-fatal-startup-crashloop.md` |

**Open Questions:**

- None — all resolved during investigation (see evidence file Investigation Log)


## Catalog-wide Open Questions

- **Fault availability (confirm with tenant):** only `node-expiry-monotonic-clock` is fully inert without a
  disabled-by-default fault (clock skew). Other properties reach their primary hazard via default-enabled
  partition/latency and list node-termination/clock-skew only as optional *enhancements*. Open: whether a
  plain container stop/restart in the compose harness substitutes for Antithesis node-termination (would
  strengthen the crash-replay variants of `kubeactions-at-most-once` and `new-leader-elected-after-loss`).
- **Instrumented image is a committed prerequisite:** most P0/P1 invariants assert on unexported,
  build-tag-gated in-process state and require the custom Linux-only instrumented DCA image (SDK in the root
  go.mod). Not compile-checkable on the macOS dev host; watch for Bazel/Gazelle flavor complications.
- **Short lease is a committed harness setting:** pin a short `leader_lease_duration` + matching
  `warmup_duration` so flap-dependent preconditions are deterministic without clock/node faults.
- **Workload must faithfully model node agents** (keep cached checks running while partitioned; back off on
  error codes) — a requirement for `antithesis-workload`, gating the fencing and getconfigs properties.
- **Provider selection (RESOLVED, user decision 2026-07-21):** the harness pins
  `external_metrics_provider.use_datadogmetric_crd: true`. `extmetrics-crd-store-converges-after-flip` and
  `extmetrics-crd-status-no-regression-across-flip` are the **primary** external-metrics properties;
  `extmetrics-configmap-no-lost-update` (legacy ConfigMap path) is **secondary/optional** — run only in a
  dedicated pass if the legacy provider is also exercised.
- **Deployment kind:** several stale-cache/reuse hazards (`forwarder-*`) are sharper under StatefulSet
  (stable pod names, new IPs) than Deployment (random names). Confirm the harness deployment kind.
- **Remaining per-property open questions** carry `(partial)` / `(needs human input)` tags; the
  `(needs human input)` ones (intended-behavior calls, deploy-side manifest values, tenant fault config)
  are the human-decision backlog and are listed under each property with an Investigation Log behind them.
- **Dropped candidate:** `isapiserverready-readyz-string-mismatch` (dormant, gated behind kube-apiserver
  >= 1.35, low value). **Out of scope:** orchestrator resource collection (user decision).
