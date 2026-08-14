---
commit: f2da1471bb748fb5108f89f36f7b83cab305ca79
updated: 2026-07-21
external_references: []
---


# Datadog Cluster Agent — Antithesis Property Catalog


**37 testable properties across 11 categories.** Synthesized from an 11-agent discovery
ensemble (44 raw candidates), stress-tested by a 4-lens evaluation ensemble (refinements + gap-fills folded
in), then every property's open questions were investigated against the code. The codebase has **zero**
existing Antithesis SDK instrumentation (`existing-assertions.md`); every SUT-side assertion is net-new.
SUT-side instrumentation and investigation logs are inlined per property below.

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
| **SUT-side instrumentation** | MISSING (zero SDK usage). Add `assert.Always` at the point where dispatcher.run transitions store.active=true, checking `IsLeader()`. Also a workload-side cross-replica check polling each replica's clusterchecks status. SUT-side instrumentation on `updateLeaderIP` state transitions gives Antithesis a replay anchor for the self-promotion branch. |
| **Fault deps** | network partition leader<->apiserver >= leader_lease_duration (asymmetric; enabled by default); clock skew past renew deadline (DISABLED by default — amplifies the window); requires leader_election enabled + >=2 replicas |

**Open Questions:**

- The 60s-partition window magnitude is measured under fault; note warmup_duration does NOT protect the primary hazard (the ex-leader keeps dispatching, already past warmup), so warmup masks only newly-promoted replicas, not this path. `(partial)`



#### Investigation Log

#### Q1/Q3: Can client-go surface an empty holderIdentity to a FOLLOWER during the lease gap (self-promotion), or does the follower retain the outgoing leader's name?

Examined leaderelection_engine.go:150-170 (callbacks) and leaderelection.go:250-266 (GetLeader/GetLeaderIP). Found: a follower's leaderIdentity is set ONLY via OnNewLeader(identity), which client-go invokes with the Lease record's spec.holderIdentity. client-go does not clear holderIdentity on lease expiry — it is overwritten only when a new candidate CASes the lease. OnStoppedLeading (which sets leaderIdentity="") fires ONLY on the process that WAS leading. Concluded: a follower retains the outgoing leader's non-empty name during the gap; GetLeaderIP() returns that leader's (possibly stale) IP, NOT "". A follower does NOT self-promote on "" (that empty-observation only happens at startup zero-value or on the ex-leader). RESOLVED.

#### Q2: Does warmup_duration (30s) mask short flaps enough that dispatching windows rarely overlap?

Examined handler.go:95-176 (Run) and 118-141 (warmup gate). Found: warmup applies only to a replica transitioning INTO state==leader before it starts runDispatch. The dominant split-brain hazard is the EX-leader that is already dispatching (past warmup) and never re-enters warmup. Concluded: warmup provides no protection for the ex-leader over-dispatch path; the 60s-partition overlap magnitude is a runtime timing question left to fault measurement. PARTIAL.

#### Q4: Does the old leader's dispatcher reliably observe loss and reset() before a healed partition lets both dispatch?

Examined handler.go:236-280 (updateLeaderIP state machine), 155-195 (Run loss handling + runDispatch reset), leaderelection.go:262-266. Found: the state machine transitions leader→follower ONLY when newIP != "" (handler.go:258-260). After OnStoppedLeading clears the ex-leader's identity to "", GetLeaderIP() returns ("",nil) locally (no network), so newIP=="" and NO transition occurs — the ex-leader stays state==leader and keeps dispatching. reset() (handler.go:194) runs only after dispatchCancel(), which fires only on receiving newState=follower, i.e. only once the ex-leader observes a DIFFERENT non-empty leader IP (post-heal). Concluded: the ex-leader does NOT reset on loss-of-lease; it dispatches lease-less until it observes the new leader's IP. RESOLVED.


---
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
| **SUT-side instrumentation** | MISSING. `assert.Sometimes` in a workload liveness command run under a quiet period. SUT-side `Reachable` markers on OnStartedLeading would confirm the acquisition path is hit. |
| **Fault deps** | network partition leader<->apiserver >= LeaseDuration (works with defaults); node termination (DISABLED by default — needed for the crash-failover variant); requires leader_election enabled + >=2 replicas |

**Open Questions:**

- Code gives the acquire cadence (LeaseDuration 60s, RenewDeadline 30s, RetryPeriod 15s, leaderelection_engine.go:200-202); a hard recovery SLA still needs measurement under injected latency, so keep Sometimes rather than a deadline assertion. `(partial)`
- ReleaseOnCancel performs a blocking Lease Update network call on shutdown (leaderelection_engine.go:196-198); under partition it blocks until the k8s client/dial timeout (not unbounded), delaying handoff — exact bound is measured under fault. `(partial)`



#### Investigation Log

#### Q1: Can recovery time be bounded as a hard SLA?

Examined leaderelection_engine.go:195-204. Found: LeaseDuration=leader_lease_duration (default 60s), RenewDeadline=LeaseDuration/2, RetryPeriod=LeaseDuration/4. Concluded: static cadence is derivable (~LeaseDuration + RetryPeriod after loss under no added latency), but converting to a hard deadline assertion requires confirming client-go acquire behavior under injected apiserver latency — measurement-under-fault. PARTIAL (stays Sometimes).

#### Q2: Can ReleaseOnCancel's shutdown network call hang under partition and delay handoff?

Examined leaderelection_engine.go:196-205 (ReleaseOnCancel gated by leader_election_release_on_shutdown, sets lease to 1s via a network call on ctx cancel). Found: client-go's release is a synchronous Lock.Update; under partition it blocks until the k8s client's dial/TLS timeout rather than hanging forever. Concluded: it CAN delay graceful handoff by the client timeout window; precise bound needs fault measurement. PARTIAL.


---
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
| **SUT-side instrumentation** | MISSING. `assert.AlwaysOrUnreachable` inside SetLeaderIP after the mutation, and in Forward. Fixing the early-return bug is a prerequisite for the invariant to ever hold. |
| **Fault deps** | network partition (follower<->apiserver, or leader churn producing GetLeaderIP()==''); concurrency (two SetLeaderIP writers); requires leader_election enabled + >=2 replicas |

**Open Questions:**

- Reachability of the same-name/new-IP reuse step depends on the harness deployment kind (needs-human): standard helm deploys the DCA as a Deployment (random pod names) so a rescheduled leader gets a NEW HolderIdentity and GetLeaderIP re-resolves under a fresh cache key; a StatefulSet (stable names) is required to hit the stale-same-name path. `(partial)`



#### Investigation Log

#### Q1: Is the forwarder's GetLeaderIP() read anywhere that would mask the bug by falling back to the engine's GetLeaderIP()?

Grepped all GetLeaderIP() call sites. Found the forwarder's getter (leader_forwarder.go:147) has exactly one consumer: leader_handler.go:128, which compares forwarder.GetLeaderIP() against the engine's freshly-fetched ip to decide whether to call SetLeaderIP. Concluded: no fallback to the engine value; the stale forwarder value is load-bearing for the routing decision, not masked. RESOLVED.

#### Q3: Enumerate all consumers of GetLeaderIP() to quantify blast radius (status vs routing).

Grep confirms the only reader of the forwarder's GetLeaderIP() is leader_handler.go:128 (routing check-then-act). No status/telemetry consumer reads it. Concluded: blast radius is confined to routing logic. RESOLVED.

#### Q4: Does the request-path writer ever call SetLeaderIP("") given the early return at leader_handler.go:108?

Examined leader_handler.go:108-131 and leaderelection.go:262-266. Found: SetLeaderIP is reached only when IsLeader()==false (past line 108). In a leaderless gap the follower's GetLeader() can be "" (or the ex-leader's identity), and engine GetLeaderIP() returns ("",nil) when leaderName=="", giving ip=="". Line 128 (forwarder.GetLeaderIP() != ip) is then true whenever the forwarder still holds a stale non-empty IP, firing SetLeaderIP(""). Concluded: yes, the per-request writer can call SetLeaderIP(""), driving proxy=nil while leaderIP stays stale (leader_forwarder.go:117-121). RESOLVED.


---
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
| **SUT-side instrumentation** | MISSING. `assert.Always(is508, ...)` on the already-forwarded branch in Forward; a `Reachable` marker confirms the loop-detection branch is actually exercised under fault. |
| **Fault deps** | network partition (asymmetric) to create mutual-follower state; enabled by default; node termination/rescheduling to churn leadership (DISABLED by default); requires leader_election enabled + >=2 replicas |

**Open Questions:**

- Magnitude of the mutual-follower leaderless window depends on client-go lease timing and is measured under fault, not derivable statically. `(partial)`



#### Investigation Log

#### Q1: Can the ReverseProxy ever drop/rewrite X-DCA-Follower-Forwarded on retry?

Examined leader_forwarder.go:86-144. Found: the 508 guard (line 90-95) runs before any proxy dispatch, so a request already carrying the header never reaches ServeHTTP. On the outbound leg the Director uses req.Header.Add(forwardHeader,"true") (line 126), and httputil.ReverseProxy has NO built-in retry and strips only hop-by-hop headers (this custom header is not hop-by-hop). Concluded: the header cannot be dropped/rewritten by a retry and reliably reaches the next hop; the single-hop cap holds by construction. RESOLVED.


---
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
| **SUT-side instrumentation** | MISSING. `assert.AlwaysOrUnreachable` in Forward's Director comparing target IP against the current endpoint set. A cache-hit-vs-endpoint-mismatch `Reachable` marker anchors the stale-cache branch for replay. |
| **Fault deps** | node termination / pod restart with IP change (DISABLED by default — must be enabled); network partition / EndpointSlice propagation lag (enabled by default); requires leader_election enabled + >=2 replicas |

**Open Questions:**

- Exploitability depends on harness deployment kind (needs-human): a StatefulSet (stable pod name, new IP) makes the stale-same-name/new-IP path reachable; the standard helm Deployment (random names) forces a fresh GetLeaderIP cache key on reschedule, largely closing it. `(partial)`
- Real-world likelihood that a reused pod IP belongs to a pod that actually reads/logs the bearer token (vs. simply RSTs) is a probabilistic operational judgment not answerable from code. `(needs human input)`



#### Investigation Log

#### Q1: Does the ReverseProxy strip Authorization?

Examined the Director (leader_forwarder.go:123-135) and validateToken middleware (server.go:174-196). Found: the Director sets only URL.Scheme/Host, adds forwardHeader, and restores the path — it never touches Authorization; httputil.ReverseProxy removes only hop-by-hop headers; validateToken reads but never Del's Authorization and passes the request through unchanged. Concluded: Authorization is preserved verbatim to the target. RESOLVED (confirms the token-leak hazard).

#### Q4: Is SetLeaderIP called with a freshly-resolved IP quickly enough to shrink the window below the 5-min cache?

Examined leaderelection.go:262-293 and leader_handler.go:112-131. Found: the IP fed to SetLeaderIP comes from engine GetLeaderIP(), which caches per leader NAME (cacheKey "ip://"+leaderName) for 5 minutes (line 292). While the leader name is unchanged, every SetLeaderIP call re-reads the same cached IP regardless of cadence; when the name changes a fresh key resolves immediately. Concluded: SetLeaderIP frequency cannot shrink the stale window below the cache TTL — the window is bounded by leader-NAME stability, not writer cadence. RESOLVED.


---
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
| **SUT-side instrumentation** | In the Director closure (leader_forwarder.go:123-135), after the restoration block, add: `parsed, err := url.ParseRequestURI(req.RequestURI); assert.AlwaysOrUnreachable(err == nil && strings.HasPrefix(req.URL.EscapedPath(), "/api/v") && req.URL.EscapedPath() == parsed.EscapedPath(), "leader-forwarded request retains full un-stripped API path", map[string]any{"requestURI": req.RequestURI, "outPath": req.URL.EscapedPath(), "parseErr": fmt.Sprint(err)})`. Requires adding github.com/antithesishq/antithesis-sdk-go to the root module go.mod (no SDK currently present). |
| **Fault deps** | leader_election enabled + >=2 replicas (required; forwarding path is inert otherwise); A workload that drives node-agent requests at a FOLLOWER replica so the follower->leader forward executes (required); NO node-termination and NO clock-skew needed; a workload that sends edge-case request targets (percent-encoded segments, trailing/double slash, dot segments) maximizes coverage of the parse/escape branch |

**Open Questions:**

- Whether the leader's net/http ServeMux treats a Path/RawPath mismatch as a different route: the assertion keys on EscapedPath equality, which is what Go 1.22 ServeMux matches on, so equality is the right check; residual is confirming the leader mux uses default 1.22 matching with no custom normalization. `(partial)`



#### Investigation Log

#### Q1: Can a legitimate node-agent request make url.ParseRequestURI return err?

Reasoned about net/http server RequestURI semantics and the routes in isExternalPath (server.go:200-213). Found: node-agent calls use origin-form request targets (/api/v1/clusterchecks/..., /api/v2/series), which ParseRequestURI parses with err==nil. err is produced only for authority-form (CONNECT) or asterisk-form (OPTIONS *) or malformed targets. Concluded: the silent-fallthrough (err!=nil) branch is reachable only via a hostile/fuzzed client the workload must synthesize, not by legitimate traffic. RESOLVED.

#### Q3: Are there leader-proxied handlers on the ROOT router (not under /api/vN StripPrefix)?

Examined server.go:64-80,154-162 and grepped ModifyAPIRouter/ModifyRootRouter callers. Found: apiRouter is mounted under StripPrefix("/api/v1") and v2ApiRouter under StripPrefix("/api/v2"); all leader-proxied handlers (clusterchecks/endpointschecks via v1, languagedetection via apiRouter, series via v2ApiRouter) register there. ModifyRootRouter has ZERO callers; ModifyAPIRouter callers (command.go:534,771) register on apiRouter. Concluded: no leader-proxied handler is on the root router, so path restoration never over-prepends a prefix. RESOLVED.

#### Q4: What exact header carries the DCA token?

Examined validateToken (server.go:174-196 → util.TokenValidator/GetDCAAuthToken) and server_test.go:69. Found: token carried as `Authorization: Bearer <token>`. Concluded: the assertion must key on the Authorization header. RESOLVED.

#### Q5: Is there middleware that could strip Authorization?

Examined the only middleware wrapping the router: validateToken (server.go:83, 174-196) and RecoveryHandler (line 143). Found: validateToken reads Authorization for token validation and calls next.ServeHTTP(w, r) with the request unchanged (no Header.Del anywhere in the chain). Concluded: no middleware strips Authorization; the auth-preservation invariant is non-vacuous and holds. RESOLVED.

#### Q6: Which leader-proxied endpoints most reliably fire the witness early?

Examined clusterchecks.go:25-29. Found the highest-traffic follower-forwarded routes: GET /clusterchecks/configs/{identifier} and POST /clusterchecks/status/{identifier} (node-agent config pulls and heartbeats, both under /api/v1 StripPrefix). Concluded: drive these two endpoints at a follower to fire the stripped-path witness earliest. RESOLVED.


---
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
| **SUT-side instrumentation** | MISSING. Add checkStoreConsistency() under d.store.Lock at the tail of every mutating op, wrapped in assert.Always. Endpoints configs (node-pinned 1:1) need a carve-out from the load-balanced shape. |
| **Fault deps** | network partition (node-agent<->leader >30s to trigger expiry; enabled by default); node hang/throttle on a node agent; clock skew backward (DISABLED by default — amplifies mass expiry); requires leader_election enabled + >=2 replicas to exercise reset/re-acquire |

**Open Questions:**

- None — all resolved during investigation (see evidence file Investigation Log)



#### Investigation Log

#### Does reset() zero nodeStore.lastConfigChange, or can a stale epoch survive a leadership cycle?

Examined stores.go:42-55 (reset) and :129-137 (newNodeStore). Found: reset() does `s.nodes = make(...)`, discarding every *nodeStore object entirely; lastConfigChange is a field OF nodeStore (stores.go:120), so it cannot survive a reset. A re-registered node gets a fresh newNodeStore with lastConfigChange=0. Conclusion: RESOLVED - no per-node epoch survives; there is no stale lastConfigChange in the rebuilt store.

#### Are endpoints configs (node-pinned 1:1) expected to violate the load-balanced shape and need a carve-out?

Examined stores.go:31 (endpointsConfigs map[nodeName][digest]), dispatcher_main.go:188-198 (Schedule routes NodeName!="" to addEndpointConfig), dispatcher_endpoints_configs.go:42-64. Found: endpoints live in a disjoint endpointsConfigs map that is NEVER touched by digestToNode/digestToConfig/danglingConfigs. Conclusion: RESOLVED - the bijection invariant scopes only to the cluster-check maps and naturally excludes endpoints; a carve-out is correct/required.

#### Exact set of store mutators that must carry the invariant check.

Examined dispatcher_configs.go, dispatcher_nodes.go, dispatcher_rebalance.go, stores.go. Found digest/node-touching mutators, all under d.store.Lock: addConfig (dispatcher_configs.go:120), removeConfig (:168), deleteDangling (:225, caller holds lock), expireNodes (dispatcher_nodes.go:142), moveConfig/rebalance (dispatcher_rebalance.go:168, lock at :175), store.reset (stores.go:42 via d.reset dispatcher_main.go:301), getOrCreateNodeStore (stores.go:86, inserts nodes; callers hold lock). Conclusion: RESOLVED - that 7-site set is the enumerated coverage.

#### Exact set of gauges still reset-asymmetric after #52876.

Examined metrics.go and all Inc/Dec/Set/Delete sites. reset() (stores.go:42-55) clears only dispatchedConfigs (per-node Delete loop), nodeAgents (Dec loop), danglingConfigs+unscheduledCheck (via clearDangling). NOT cleared by reset(): dispatchedEndpoints (Inc addEndpointConfig:49 / Dec removeEndpointConfig:63, NO Delete anywhere -> ratchets up every leadership cycle on AD replay; the strongest genuine surviving leak, same shape as the #52876 nodeAgents bug), configsInfo (Set/Delete only in addConfig/removeConfig/rebalance/expireNodes - self-heals only by overwrite if same check lands on same node), busyness (Set updateRunnersStats / Delete expireNodes only), predictedUtilization (Set-only dispatcher_rebalance.go:539, never Deleted). statsCollectionFails is a counter. Conclusion: RESOLVED - {dispatchedEndpoints, configsInfo, busyness, predictedUtilization} survive reset asymmetrically; dispatchedEndpoints is the clean Inc-without-clear leak.

#### No leader-generation epoch on lastConfigChange; after reset() the counter restarts (folded from R6) - can a node be told IsUpToDate when it is not?

Examined helpers.go:47 (lastConfigChange = time.Now().UnixNano()) and dispatcher_nodes.go:69 (comparison is exact equality node.lastConfigChange == status.LastChange). Found: comparison is equality, not ordering, on a nanosecond wall-clock value. After reset a reused node gets a fresh nodeStore (0, then a fresh nano on next addConfig). A false IsUpToDate requires node.lastConfigChange == status.LastChange EXACTLY; a nanosecond timestamp never coincidentally equals a node's stale cached value (edge case both==0 correctly means no configs on either side). Even under backward clock skew the values still differ -> node re-pulls (returns false). Conclusion: RESOLVED - missing epoch is not exploitable for false IsUpToDate given exact-equality nanosecond comparison; not a bijection defect.


---
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
| **SUT-side instrumentation** | MISSING. `assert.AlwaysOrUnreachable` at the end of reset() checking every map empty and each gauge==ground-truth. Confirm each historical fix (#52876/#52078/#50715) mechanism from the diff before treating it as a regression anchor. R11: the GAUGE half (nodes_reporting/dangling/unscheduled) is Prometheus telemetry on the unauth'd metrics_port — the workload can scrape /metrics and assert ground-truth across a flap with NO instrumentation; reserve SUT-side asserts for the in-memory-map-empty half. |
| **Fault deps** | network partition to force lease loss then heal (leadership flap; enabled by default); requires leader_election enabled + >=2 replicas |

**Open Questions:**

- Should the narrow in-flight heartbeat race be explicitly closed? A PostStatus that passed RejectOrForwardLeaderQuery reading state==leader just before h.state flips to follower can have its getOrCreateNodeStore land after reset() (both serialize on d.store.Lock), registering a phantom node + nodeAgents.Inc post-reset. It self-heals via expireNodes within node_expiration_timeout next term, so bounded/transient - but whether processNodeStatus should re-check active/leadership is a design call. `(needs human input)`



#### Investigation Log

#### Should a heartbeat arriving during teardown be rejected once dispatchCancel fires, or is auto-registration after reset a genuine leak?

Examined cmd/cluster-agent/api/v1/clusterchecks.go:44 (PostStatus gated by RejectOrForwardLeaderQuery), handler_api.go:22-42 (gate reads h.state), handler.go:257-277 (updateLeaderIP sets h.state=follower at :275 BEFORE the leadershipChan send at :276 that triggers dispatchCancel at :172 and then reset() at :194). Found: the gate is h.state, which flips to follower strictly before reset() begins, so new heartbeats are forwarded not registered. processNodeStatus (dispatcher_nodes.go:44-52) has NO active/leadership re-check; it unconditionally getOrCreateNodeStore + nodeAgents.Inc. A request that passed the gate before the flip can register a phantom post-reset (narrow race, serialized on d.store.Lock). Conclusion: PARTIAL - gate exists and closes the common case; residual narrow race is self-healing (expireNodes next term). Whether to harden is intended-behavior -> needs-human.

#### Does reset() zero nodeStore.lastConfigChange so a reused node isn't falsely IsUpToDate on the next term?

Examined stores.go:42-55, :120, :129-137, dispatcher_nodes.go:69. Found: reset() remakes s.nodes, discarding all nodeStore objects (lastConfigChange is a nodeStore field); re-registered node starts at 0. Comparison is exact-equality on nanosecond timestamp. Conclusion: RESOLVED - lastConfigChange is effectively zeroed (nodeStore recreated); no false IsUpToDate (see dispatch-store-bijection R6 analysis).

#### Does reset() zero lastConfigChange (affects recovered nodes told IsUpToDate / dangling never re-pulled)?

Same evidence as above (stores.go reset remakes nodes map; equality comparison dispatcher_nodes.go:69). Conclusion: RESOLVED - duplicate of prior question; zeroed, not exploitable.

#### Is there a concrete Inc-without-Dec path today, or is the current code balanced?

Examined all gauge sites. Found: dispatchedEndpoints (dispatcher_endpoints_configs.go:49 Inc / :63 Dec) has NO Delete and is NOT cleared by reset() (stores.go:42-55 remakes endpointsConfigs map without decrementing) -> on AD replay after each leadership loss addEndpointConfig Inc's again, ratcheting the endpoint_checks.configs_dispatched gauge up every cycle. Also the narrow nodeAgents.Inc post-reset race (above). Balanced gauges: dispatchedConfigs, nodeAgents (both cleared in reset loop), danglingConfigs/unscheduledCheck (clearDangling). Conclusion: RESOLVED - yes, dispatchedEndpoints is a concrete Inc-without-clear path surviving resets.


---
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
| **SUT-side instrumentation** | MISSING. `assert.Sometimes` in a workload liveness command after restoring a node under a quiet period. |
| **Fault deps** | network partition of node agents > node_expiration_timeout then heal (enabled by default); requires leader_election enabled + >=2 replicas (dispatch is leader-only) |

**Open Questions:**

- None — all resolved during investigation (see evidence file Investigation Log)



#### Investigation Log

#### Does reset() zero lastConfigChange?

Examined stores.go:42-55/:120/:129-137, dispatcher_nodes.go:69. Found: reset() remakes s.nodes, nodeStore (and its lastConfigChange) discarded; re-registered node starts at 0; comparison is exact-equality. Conclusion: RESOLVED - zeroed; does not cause recovered nodes to be falsely IsUpToDate, so does not block dangling re-pull.

#### Is there a concrete Inc-without-Dec path today (affecting dangling drain)?

Examined danglingConfigs gauge sites: Inc addConfig(dispatcher_configs.go:140)/expireNodes(dispatcher_nodes.go:161), Dec removeConfig(:191)/deleteDangling(:229), Delete clearDangling(stores.go:110). Found: danglingConfigs is balanced. The only Inc-without-clear leak in the package is dispatchedEndpoints, which is unrelated to dangling drain liveness. shouldDispatchDangling (dispatcher_configs.go:208-213) requires len(nodes)>0; drain loop dispatcher_main.go:405-411 runs each cleanup tick (nodeExpirationSeconds/2). Conclusion: RESOLVED - no Inc-without-Dec affecting dangling; drain liveness path is intact once >=1 node exists.


---
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
| **SUT-side instrumentation** | MISSING. `assert.AlwaysOrUnreachable` in reschedule checking the digest is still present in digestToConfig (not tombstoned) before re-adding; or a tombstone set asserted against. |
| **Fault deps** | node expiry to populate danglingConfigs (network partition; enabled by default); concurrent AutoDiscovery Unschedule; requires leader_election enabled + >=2 replicas |

**Open Questions:**

- Frequency/latency of AD Unschedule relative to the cleanup cadence is runtime/environment-dependent and not derivable from code. Mechanism confirmed: the resurrect window is the gap between retrieveDangling (RUnlock, dispatcher_configs.go:217-222) and the per-config addConfig inside reschedule->add (each takes d.store.Lock separately); AD Unschedule runs on the AutoConfig single worker goroutine (controller.go:151-215) contending only on d.store.Lock. Reaching it requires Antithesis interleaving control, not wall-clock luck. `(partial)`



#### Investigation Log

#### Frequency/latency of AD Unschedule relative to the 15s cleanup cadence.

Examined dispatcher_main.go:385 (cleanupTicker=nodeExpirationSeconds/2), :400-411 (compound op drops store lock between retrieveDangling and reschedule->addConfig), dispatcher_configs.go:120-166 (addConfig never checks digest still in danglingConfigs/known), controller.go:151-215 (AD single-worker Schedule/Unschedule under ms.m). Found: mechanism/window fully confirmed; the actual Unschedule frequency is deployment-dependent and unknowable from code. Conclusion: PARTIAL - window mechanism resolved, timing needs runtime data / Antithesis interleaving.

#### Whether endpoints configs have an analogous resurrection path.

Examined dispatcher_endpoints_configs.go (addEndpointConfig/removeEndpointConfig only), dispatcher_main.go:196/:248 (called ONLY directly from Schedule/Unschedule), dispatcher_nodes.go:142-186 (expireNodes iterates node.digestToConfig, NOT endpointsConfigs), dispatcher_configs.go:216-235 (retrieveDangling/deleteDangling touch only danglingConfigs). Found: endpoints are never placed in danglingConfigs and have no reschedule path. Conclusion: RESOLVED - no analogous resurrection path exists for endpoints configs.


---
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
| **SUT-side instrumentation** | MISSING. `assert.AlwaysOrUnreachable` reconciling ksmShardedConfigs against store shards after Schedule/reset. Confirm the documented fix ordering from the diff. |
| **Fault deps** | network partition to flap leadership concurrent with AD config replay (enabled by default); concurrency (ksmShardingMutex vs store lock split); requires leader_election enabled + >=2 replicas + ksm_sharding_enabled + advanced_dispatching |

**Open Questions:**

- None — all resolved during investigation (see evidence file Investigation Log)



#### Investigation Log

#### Whether AutoConfig RemoveScheduler synchronously drains in-flight Schedule calls.

Examined autoconfig.go:665-668 (RemoveScheduler->Controller.Deregister), controller.go:131-139 (Deregister takes ms.m), :163-215 (processNextWorkItem holds ms.m across the ENTIRE scheduler.Schedule() call section :191-208), :87 (single worker goroutine). Found: an in-flight Schedule holds ms.m, so Deregister BLOCKS until it completes; after Deregister returns the dispatcher is removed from activeSchedulers so no subsequent Schedule reaches it. runDispatch (handler.go:191) calls RemoveScheduler BEFORE reset() (:194). Conclusion: RESOLVED - yes, it synchronously drains; the self-documented KSM race window (handler.go:187-191) is effectively closed at the AutoConfig layer by the shared ms.m plus the single-worker model.

#### Whether reset() zeroing under two locks can interleave with a concurrent Schedule seeing half-cleared state.

Examined dispatcher_main.go:294-304 (reset takes ksmShardingMutex then store.Lock sequentially), dispatcher_ksm.go:38-40/:96-110 (isAlreadySharded/markAsSharded read/write ksmShardedConfigs only under ksmShardingMutex, called only from Schedule). Found: because RemoveScheduler (serialized with Schedule via ms.m) precedes reset(), no AD-driven Schedule can be concurrent with reset(); the only reader of ksmShardedConfigs is scheduleKSMCheck via Schedule. Conclusion: RESOLVED - no concurrent observer of the half-cleared state exists once the scheduler is deregistered.

#### Requires a shardable KSM check config present for the branch to be reachable.

Examined dispatcher_main.go:111-141 (KSM sharding enabled only when advanced_dispatching_enabled AND ksm_sharding_enabled) and Schedule->scheduleKSMCheck (:201). Found: the isAlreadySharded/markAsSharded branch is reachable only with both config flags set plus an actual KSM check config. Conclusion: RESOLVED - harness precondition confirmed: leader_election + >=2 replicas + advanced_dispatching_enabled + ksm_sharding_enabled + a KSM check config.


---
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
| **SUT-side instrumentation** | MISSING. `assert.Sometimes` on the store.active=true transition plus a first-dispatch marker; a `Reachable` on runDispatch entry helps Antithesis steer toward the stable-leadership branch. |
| **Fault deps** | network partition leader<->apiserver to induce flapping (enabled by default — sufficient); clock jitter (DISABLED by default — sharper trigger); requires leader_election enabled + >=2 replicas |

**Open Questions:**

- Is warmup_duration ever set below RenewDeadline (leader_lease_duration/2) in real deployments? Code shows both are independently tunable and equal by default (30s == 30s); a field survey is needed to know actual deployment configs. `(needs human input)`



#### Investigation Log

#### Q1: Can client-go deliver leader->non-leader transitions faster than 30s repeatedly under the default 60s lease, or does it require lowering lease_duration?

Examined handler.go:239-277 (updateLeaderIP), leaderelection_engine.go:150-205 (callbacks + RenewDeadline/LeaseDuration), common_settings.go:278 (leader_lease_duration default 60). Found: the handler observes leadership only via GetLeaderIP polling at 1s; a demotion (`follower` send) requires newIP!='' i.e. a *different* replica observed as leader (handler.go:257-261). Another replica cannot acquire the lease until it expires (~LeaseDuration=60s after last renew); our own OnStoppedLeading fires only after RenewDeadline=30s. Concluded: RESOLVED — sub-30s repeated leader->non-leader transitions are NOT achievable under the default 60s lease; the harness must lower leader_lease_duration to make lease churn sub-warmup. Confirms the question's hypothesis.

#### Q2: Does a leadership-lost during warmup leak any goroutine/scheduler across cycles?

Examined handler.go:129-176. Found: runDispatch (which calls AddScheduler and dispatcher.run) and dispatchCtx/dispatchCancel are created only AFTER warmup completes (:151-152). On warmup abort the code hits `continue` at :136 — no dispatch goroutine started, no scheduler registered, nothing to cancel; the warmup span is closed via finishWarmupSpan('leadership_lost') (:134). Only residue: the abandoned time.After(warmupDuration) timer from :138 lingers until it fires (<=30s) then is GC'd — bounded, not a cross-cycle goroutine/scheduler leak. Concluded: RESOLVED — no leak.

#### Q3: Does flap-window magnitude depend on whether a follower sees a non-empty leader IP during the gap (empty holderIdentity -> self-promote)?

Examined handler.go:257-277 and leaderelection.go:250-294 + engine.go:151-169. Found: warmup abort (the `follower` continue) fires ONLY when GetLeaderIP returns a non-empty IP, which happens only when GetLeader() returns a DIFFERENT identity (via OnNewLeader). OnStoppedLeading sets leaderIdentity='' -> GetLeaderIP='' -> updateLeaderIP sends NO transition (newState stays unknown). Concluded: RESOLVED and property-narrowing — a pure gap where our pod loses the lease with no successor keeps GetLeaderIP='' and does NOT abort warmup; the warmup timer keeps running. Starvation requires a DIFFERENT replica to be repeatedly observed as leader within sub-warmup windows. (Self-promotion is moot here: the pod is already the aspiring leader and simply stays leader.)

#### Q4: Does a duplicate `leader` send restart warmup?

Examined handler.go:129-141 and 257-277. Found: updateLeaderIP sends only on genuine state transitions (leader<->follower); while state==leader it never re-sends `leader` (newIP=='' -> unknown -> no send; newIP!='' -> follower). Even structurally, the :135 guard `if newState != leader { continue }` means a `leader` value would NOT continue — it falls through and cuts warmup SHORT to dispatch, never restarting it. Concluded: RESOLVED — only follower edges abort warmup; a duplicate leader neither restarts warmup nor can it be emitted by the normal path. Confirms scratchbook.

#### Q5: Does leadershipChan coalescing (buffered(1)) deliver a spurious non-leader during a fast gain->lose->gain, interrupting warmup even when the lease was effectively held?

Examined handler.go:73 (make(chan state,1)), :274-277 (send under h.m.Lock()), :197-234 (leaderWatch, sole sender at 1s). Found: sends are serialized under the lock by a single sender; buffered(1) provides FIFO with backpressure (a full buffer blocks the next send under lock) — no coalescing/dropping of states. Combined with Q3: a self gain->lose->gain yields GetLeaderIP='' throughout, so NO sends occur; a `follower` is emitted only upon observing a distinct non-empty leader IP. Concluded: RESOLVED — coalescing cannot manufacture a spurious non-leader; every delivered follower reflects a real observation of a different leader.

#### Q6: Is warmup_duration ever set below RenewDeadline in real deployments?

Examined common_settings.go:278 (leader_lease_duration=60) & :576 (warmup=30); engine.go:201 (RenewDeadline=LeaseDuration/2=30). Found: defaults make warmup==RenewDeadline (30s==30s); the two are independent knobs. Would fall below RenewDeadline only if leader_lease_duration is raised (>60 -> RenewDeadline>30) or warmup lowered (<30). Concluded: PARTIAL/needs-human — code gives defaults and the mechanism, but actual field-deployment values require a human survey.


---
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
| **SUT-side instrumentation** | MISSING. SUT-side timing: record lock-acquire/release timestamps; `assert.Always(held < bound)`. The real fix is to snapshot node IPs, drop the lock, then do HTTP — the assertion documents the requirement. |
| **Fault deps** | network latency/congestion on leader->CLC-runner HTTP (enabled by default); asymmetric partition of a subset of CLC runners; requires leader_election + advanced_dispatching + CLC runners in the topology |

**Open Questions:**

- Whether any production Helm/Operator deployment runs rebalance_period low enough (default 10m) to hit this outside a tuned harness; only rebalance() (rebalanceTicker) reaches updateRunnersStats, so triggering fast needs a lowered rebalance_period or forced rebalance. `(needs human input)`



#### Investigation Log

#### Exact aggregate CLC runner client per-call timeout (dial+TLS+response-header).

Examined pkg/util/clusteragent/clcrunner.go:60-90. Found: init() builds http.Client with Transport{TLSClientConfig:...} only (no per-phase Dial/ResponseHeader timeouts) and sets c.clcRunnerAPIClient.Timeout = 2*time.Second (:86). Conclusion: the single 2s http.Client.Timeout is the total per-call deadline covering dial+TLS+response header+body; worst case per runner call = 2s. RESOLVED.

#### Whether rebalance_with_utilization is enabled by default.

Examined pkg/config/setup/common_settings.go:581 and config_test.go:1394. Found: BindEnvAndSetDefault(cluster_checks.rebalance_with_utilization, true); config_test asserts it true. Conclusion: default ON, so the GetRunnerWorkers branch (dispatcher_nodes.go:209) is also taken -> 2 HTTP calls/node under the write lock. RESOLVED.

#### Whether any production deployment runs rebalance_period low enough to hit this.

Examined common_settings.go:586 (rebalance_period default 10m) and dispatcher_main.go:388,415-417 (only rebalanceTicker fires rebalance -> updateRunnersStats). Conclusion: code path clear but whether any real deployment lowers rebalance_period is a deployment/intended-behavior call. NEEDS-HUMAN.

#### What is the CLCRunnerClient HTTP timeout (clcrunner.go)?

Same as Q1: clcrunner.go:86 http.Client.Timeout = 2*time.Second. Conclusion: 2s. Worst-case lock hold ~ N nodes x 2 calls x 2s. RESOLVED.


---
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
| **SUT-side instrumentation** | MISSING. SUT-side: assert non-blocking send (select with default, or measure send latency) under h.m; `assert.Always`. Also a `Reachable` marker on the second-transition-during-warmup branch. |
| **Fault deps** | network partition leader<->apiserver near lease boundary to flap leadership (enabled by default); clock jitter past renew (DISABLED by default); requires leader_election enabled + >=2 replicas |

**Open Questions:**

- Refined: Run DOES read leadershipChan during warmup (handler.go:133), and buffer is drained on entry at :111, so the only windows where Run is not selecting on the channel are the brief non-select code stretches (:116-128 and :143-152, sub-second) — not the full 30s warmup. Open: measure whether two sends can land in that narrow window. `(partial)`
- updateLeaderIP sends only on a real leader<->follower transition (handler.go:257-277); self-transitions produce NO send. So a wedge needs TWO real flips (lose then regain) inside a sub-second no-read window at 60s lease / 30s RenewDeadline / 15s RetryPeriod — realistic only under clock-skew/partition fault; needs flap frequency measured under fault. `(needs human input)`



#### Investigation Log

#### Whether leadershipChan's single buffer is always drained by Run before the next tick (1s poll vs 30s warmup).

Examined handler.go:106-176. Found: entry select :108-116 consumes the leader state (buffer=0 entering warmup); warmup select :129-141 ALSO reads leadershipChan at :133; steady select :158-164 reads it. Conclusion: the discovery premise 'consumer parked in time.After not draining for 30s' is INACCURATE — Run drains during warmup. True no-read window = brief code between selects (:116-128, :143-152). PARTIAL (refined).

#### Whether the observation assertion can be added without the select refactor that removes the hang.

Examined handler.go:246-277. Found: bare send at :276 under h.m held :246. Conclusion: YES — capture lockAcquired at :246, measure time.Since after :276 and assert.Always(elapsed<T); this is pure observation and leaves the blocking send intact, unlike a select-with-default which would itself remove the hang. RESOLVED.

#### Interaction with notify() coalescing dropping the intermediate loss edge.

Examined handler.go:240 (leaderStatusCallback), handler_test.go:59 (le.get), leaderelection_engine.go:227-239 (notify) and leaderelection.go:339 (Subscribe). Found: the clusterchecks Handler polls GetLeaderIP via leaderStatusCallback every 1s; it does NOT use Subscribe()/notify(). Conclusion: notify() coalescing only affects Subscribe consumers, not this handler's leadershipChan; irrelevant. The relevant coalescing is 1s poll sampling (two flips within one poll are invisible to the handler). RESOLVED.

#### Whether client-go can emit two self-transitions within one warmup window at 60s lease.

Examined handler.go:257-277 and leaderelection_engine.go:200-202 (LeaseDuration 60s default, RenewDeadline=LD/2=30s, RetryPeriod=LD/4=15s; leaderelection.go:46,90). Found: the state switch only sets newState (and sends) on an actual leader<->follower change; staying-leader/staying-follower yields newState=unknown -> no send. Conclusion: self-transitions are a red herring — the wedge requires two REAL transitions; feasibility at 60s lease is fault-dependent. NEEDS-HUMAN (refined).

#### Whether any h.m reader is on the node-agent hot path so the stall is externally observable.

Examined handler_api.go:22-24 (RejectOrForwardLeaderQuery takes h.m.RLock) and cmd/cluster-agent/api/v1/clusterchecks.go:44,76,98,127 + endpointschecks.go:30. Found: RejectOrForwardLeaderQuery runs on EVERY node-agent cluster-check request (GET /configs, POST /status, endpoints). Conclusion: a writer stalled on h.m blocks all these RLocks -> externally observable as node-agent request timeouts. RESOLVED.


---
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
| **SUT-side instrumentation** | MISSING. Distinguish 'transient slowness' from 'genuine deadlock' — the assertion should fire only when a recoverable delay caused a restart. SUT-side probe-drain timing instrumentation. |
| **Fault deps** | network latency/congestion on DCA->apiserver and DCA->CLC-runner (enabled by default); node hang/throttle; requires leader_election enabled |

**Open Questions:**

- Where is the boundary between a legitimate liveness failure (real deadlock) and a false positive (recoverable transient latency)? The assertion must fire only when a delay the system recovers from on its own caused a restart, without penalizing correct hang-detection — an intended-behavior judgment. `(needs human input)`



#### Investigation Log

#### Exact k8s liveness probe timeout/failureThreshold in shipped manifests.

Examined Dockerfiles/manifests/cluster-checks-runners/cluster-agent-deployment.yaml:199-208 and pkg/status/health/{global.go:22,health.go:15,76}. Found: canonical repo DCA livenessProbe = failureThreshold 6, periodSeconds 15, timeoutSeconds 5, initialDelay 15; health component internal timeout 30s, ping every 15s. Conclusion: /live goes unhealthy after ~30s of no channel drain; pod restart requires ~6 failed probes ~= 90s of sustained failure. Caveat: shipped Helm/Operator chart (separate repo) may override these. RESOLVED (with caveat).

#### Whether updateRunnersStats (rebalance 10m) vs cleanup (15s) is the overlapping lock user.

Examined dispatcher_main.go:385-419. Found: cleanupTicker = nodeExpiration/2 = 15s -> expireNodes (store lock, NO network); rebalanceTicker = 10m -> rebalance -> updateRunnersStats (store lock + HTTP). Both run in the SAME select goroutine that drains healthProbe.C (:398), so any long rebalance() starves the probe regardless of lock. Conclusion: the >30s-hold risk is updateRunnersStats (network under lock) via rebalance, not the frequent-but-I/O-free cleanup. RESOLVED.

#### Boundary between a legitimate liveness failure and a false positive.

Examined dispatcher_main.go:398-399 and handler.go:211-212 self-acknowledged 'might hang' comments. Conclusion: code cannot define the recoverable-vs-genuine-deadlock threshold; it is a design/intent decision the property must encode. NEEDS-HUMAN.


---
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
| **SUT-side instrumentation** | Add the Antithesis Go SDK to the root module and instrument dispatcher_rebalance.go. (1) LIVENESS WITNESS: in rebalanceUsingBusyness, track a per-cycle bool `sawMoveConfigError`; on the moveConfig error break set it; after the outer loop returns call assert.Sometimes(true, 'rebalance cycle returned after a moveConfig failure', details{cycle,moves,errors}). This fires only if the cycle both saw a failure and reached the return — a hang makes it never fire. (2) HARD SAFETY GUARD: maintain a per-cycle counter of moveConfig attempts and assert.Always(attempts <= numNodesAtStart*numConfigsAtStart, 'rebalance inner-loop bounded', ...) at cycle end. (3) REACHABILITY of the hazardous precondition: assert.Reachable at the moveConfig-error break site with details recording the error string, so triage can confirm the real (not synthetic) failure path was scheduled. Workload side: run >=2 CLC-runner stubs, seed >1 cluster-check config, drive AD Unschedule churn while a partition to one runner is active, and lower rebalance_period so cycles fire within a timeline. |
| **Fault deps** | network latency/partition dca-leader -> clc-runner to make GetRunnerStats fail/stale (ENABLED by default); concurrency: AD Unschedule/removeConfig or expireNodes interleaved between pickConfigToMove and moveConfig (always-on thread interleaving); requires leader_election enabled + advanced_dispatching + >=1 CLC runner in topology; node termination and clock skew NOT required |

**Open Questions:**

- None — all resolved during investigation (see evidence file Investigation Log)



#### Investigation Log

#### Can two source nodes each hit a moveConfig failure with the outer loop still completing, or a pathological blow-up?

Examined dispatcher_rebalance.go:281-325 and moveConfig:168-238. Found: outer loop is fixed range over weights (len=#nodes); inner loop exits on first failure via break (:288 pick fail, :305 move fail, :322 toleration fail); only a successful moveConfig iterates, and it shrinks the source's stats (removeConfig/RemoveRunnerStats :211,:223). Conclusion: each node contributes <= (initial stat count)+1 attempts; two failing nodes just break independently and the outer loop completes. The static bound numNodes x numConfigsAtStart is safe and loose (true bound ~ total stats + #nodes). No blow-up. RESOLVED.

#### Does applyDistribution continue-on-failure re-propose the same failing move next cycle?

Examined rebalanceUsingUtilization:378-443, applyDistribution:502-535, currentDistribution:445-500. Found: proposedDistribution is rebuilt each cycle from currentDistribution(), which enumerates only checks present in clcRunnerStats with a registered digest (:459-470). A config that failed to move (Unscheduled/removed, or stats absent) is excluded from currentDistribution next cycle. Conclusion: applyDistribution's continue-on-failure does NOT perpetually re-propose the same failing move — the failing config drops out of the input. RESOLVED.

#### CLC-runner client timeout: a slow GetRunnerStats prolongs updateRunnersStats under the store write lock.

Examined clcrunner.go:86 (2s per-call timeout). Conclusion: a slow runner adds up to 2s/call to updateRunnersStats (bounded, returns after all nodes time out) and prolongs the store-write-lock hold (that is the store-lock-bounded-under-slow-clc property's concern) but does NOT break termination of the rebalance cycle. RESOLVED (cross-referenced).


---
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
| **SUT-side instrumentation** | Instrument at the tail of dispatcher.rebalance(). (1) Maintain per-digest a small ring buffer of (cycleIndex, node) plus a snapshot hash of the busyness/utilization inputs used that cycle. assert.Always(!(digest changed node on the last K cycles AND the input-hash was unchanged across those cycles), 'rebalance did not thrash a config under stale inputs', details{digest, recentNodes, inputHash}). (2) PAIRED WITNESS so the Always is not vacuous: when a rebalance move's source or destination node had statsCollectionFails incremented this cycle (unreachable -> stale), call assert.Reachable(true, 'rebalance moved a config involving a stale/unreachable-runner distribution', details{digest, staleNode}) — and an assert.Sometimes(rebalance_returned_zero_moves_with_a_stale_runner_present) as a convergence witness (the healthy outcome: after a few cycles the distribution stabilizes and a cycle produces no moves despite the persistent fault). Workload: register >=2 CLC-runner stubs with controllable stats, seed configs, partition one runner for several rebalance ticks, and count per-digest schedule/unschedule churn observed on the node-agent HTTP side as an external cross-check of the internal move counter. |
| **Fault deps** | network latency/partition dca-leader -> one clc-runner sustained across >=K rebalance cycles, so its stats stay stale/zero (ENABLED by default); requires leader_election enabled + advanced_dispatching + >=2 CLC runners so there is a source and a stale target; lower rebalance_period in the harness so multiple cycles fit a timeline; node termination and clock skew NOT required; the safety assertion is INERT unless the paired witness confirms a move touched a stale/unreachable-runner distribution — instrument both |

**Open Questions:**

- Right K for the consecutive-move bound (assertion gated on 'busyness inputs unchanged from prior cycle'): too small flags legitimate rebalancing under genuinely changing load; a tuning/intended-behavior judgment not decidable from code. `(needs human input)`



#### Investigation Log

#### Does moveConfig writing the stats snapshot make an unreachable target look busy, or does the utilization path re-empty it?

Examined moveConfig:199-217 (copies moved instances' stats into destNode via AddRunnerStats :210) and updateRunnersStats dispatcher_nodes.go:219-224 (GetRunnerStats error -> statsCollectionFails.Inc + continue, snapshot NOT overwritten). Conclusion: if dest is UNREACHABLE the moved snapshot persists -> dest 'looks busy' (self-limiting); if dest is REACHABLE its clcRunnerStats is overwritten by the fresh report and a not-yet-started check disappears (re-emptied). Both branches exist; which fires is reachability/timing-dependent. RESOLVED.

#### When a config moves to a runner that has not started it, stats are absent next cycle — does digestToNode staying put prevent move-back?

Examined currentDistribution:445-500. Found: it keys off clcRunnerStats + idToDigest (:459-470), NOT digestToNode. A just-moved config whose stats are absent is excluded from the distribution entirely -> not proposed for any move that cycle -> cannot be moved back while absent. digestToNode (set moveConfig:220) is not consulted by the utilization algorithm; the absence itself prevents move-back. Once stats reappear on the new runner, addToLeastBusy places it with preferredRunner=new node plus stickiness bias. RESOLVED.

#### Right K for the consecutive-move bound.

Conclusion: choice of K (and the stale-input gate) is a false-positive/tuning tradeoff not derivable from code. NEEDS-HUMAN.

#### Is stickiness_enabled on by default?

Examined common_settings.go:589-592 and checks_distribution.go:92-96. Found: stickiness_enabled default true; stickiness_factor 4, upper_limit 1, lower_limit 0.05. leastBusyRunner subtracts a bias from the preferredRunner's utilization when stickiness enabled. Conclusion: the utilization path's anti-thrash stickiness bias is ON by default. RESOLVED.


---
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
| **SUT-side instrumentation** | MISSING. The fix is to use a monotonic clock for expiry; the assertion (`AlwaysOrUnreachable`) checks expiry decisions against elapsed monotonic time. Flag to user: needs clock faults enabled. |
| **Fault deps** | clock jitter forward/backward (DISABLED BY DEFAULT) — the property is INERT unless the tenant enables clock faults; requires leader_election enabled + >=2 replicas |
| **⚠ Requires fault** | clock skew (disabled by default) |

**Open Questions:**

- Does the Antithesis harness/tenant actually enable clock-skew faults? Without them the property is inert. No harness config exists in the repo (only antithesis/scratchbook), so this is a tenant fault-config decision that cannot be settled from code. `(needs human input)`
- Do the KSM shard map and per-node gauges recover cleanly after a mass-expiry+redispatch at runtime? Code shows the ksmShardedConfigs map is untouched by the expiry/redispatch path, but full gauge recovery overlaps clusterchecks-dispatch-consistency-after-leadership-recovery and warrants a runtime check. `(partial)`



#### Investigation Log

#### Heartbeat stamped on receipt (leader clock only) vs node-supplied timestamp?

Examined dispatcher_nodes.go:44-57 and types/types.go:33-37. Found: node.heartbeat = timestampNow() is set by the LEADER on receipt (dispatcher_nodes.go:56); the wire type NodeStatus carries only LastChange (a config-version counter, not wall-clock time) and NodeType. No node-supplied timestamp is read or trusted. Conclusion: only the leader's clock governs expiry; question resolved.

#### Does Go's time.Now() monotonic component shield the subtraction? (.Unix() strips it)

Examined helpers.go:47-54. Found: timestampNow() returns time.Now().Unix(); .Unix() returns wall-clock seconds and discards the monotonic reading. expireNodes (dispatcher_nodes.go:143) subtracts two such wall-clock values. Conclusion: monotonic clock does NOT protect this path; code is genuinely exposed to backward/forward wall-clock jumps. Resolved.

#### Does the harness enable clock skew (disabled by default)?

Examined antithesis/ directory (only scratchbook present) and grepped for clock/skew/jitter fault config. Not found: no harness/compose/fault config checked in. Conclusion: cannot resolve from repo; depends on tenant fault configuration. Kept as needs-human.

#### Do KSM shard map and gauges recover cleanly after mass-expiry+redispatch?

Examined dispatcher_nodes.go:142-186 (expireNodes), dispatcher_main.go:404-411 (redispatch), dispatcher_ksm.go:93-139, stores.go:42-55. Found: expireNodes moves per-node (already-sharded child) configs to danglingConfigs and deletes per-node gauges; reschedule() re-adds them to surviving nodes. Neither expireNodes nor reschedule touches ksmShardedConfigs (parent->shard map) — it is only cleared on leadership loss via reset() (dispatcher_main.go:297-299). Conclusion: shard map persists intact through expiry/redispatch; gauge/full-recovery behavior needs runtime confirmation and overlaps another property. Kept partial.


---
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
| **SUT-side instrumentation** | MISSING. Two distinct assertions per the sub-invariants. A `Reachable` on the disable-latch CAS confirms the downgrade path is exercised. |
| **Fault deps** | none beyond default config (advanced_dispatching_enabled=true); needs a crafted NodeAgent-typed and an empty-X-Real-Ip heartbeat in the workload; requires leader_election enabled |

**Open Questions:**

- None — all resolved during investigation (see evidence file Investigation Log)



#### Investigation Log

#### Can a CLC runner transiently report NodeType==NodeTypeNodeAgent during startup?

Examined comp/core/autodiscovery/providers/clusterchecks.go:42,57,73-78,130,229. Found: the client's nodeType is set once at provider construction — default NodeTypeCLCRunner (line 57), then set from the static IsCLCRunner config (lines 73-78) before the first heartbeat is sent, and emitted identically on every heartbeat (:130,:229). Conclusion: a correctly-configured CLC runner never transiently sends NodeAgent; the value is config-derived and fixed at startup. Resolved.

#### Is advancedDispatching re-initialized to true on every dispatcher construction, or persisted?

Examined dispatcher_main.go:111-122 and handler.go:74. Found: advancedDispatching.Store(true) runs only at construction, and only if GetCLCRunnerClient() succeeds. newDispatcher is called exactly once (NewHandler, handler.go:74). Conclusion: the true default is a per-process construction default, set once; not reloaded per leadership term. Resolved.

#### Does reset() create a fresh dispatcher (re-Store(true)) or reuse the atomic?

Examined dispatcher_main.go:294-304 (reset), stores.go:42-55 (store.reset), handler.go:180-195. Found: on leadership loss runDispatch calls d.dispatcher.reset(), which only clears ksmShardedConfigs and calls store.reset(); store.reset() rebuilds store maps but NEVER touches d.advancedDispatching. The same dispatcher struct (constructed once) is reused across leadership cycles. Conclusion: the disable latch is PER-PROCESS, not per-leadership-cycle. This DIRECTLY CONTRADICTS the scratchbook discovery claim that 'a fresh dispatcher term restores advancedDispatching=true.' Resolved.

#### Do any real legacy node-agent versions still omit node_type?

Examined git log for the node_type field: added by commit 3cee1cf97a3 on 2025-07-18 (~7.68). Found: any agent built before mid-2025 sends no node_type, which decodes to 0; 0 != NodeTypeNodeAgent so the latch never trips for such node agents. Conclusion: the omit-node_type gap is broadly reachable across a large fraction of deployed agents, not only via a hand-crafted client. Resolved.

#### Does a legacy node agent that omits X-Real-Ip also omit node_type (co-occur), or independent?

Examined pkg/util/clusteragent/clusteragent.go:44,150 (X-Real-Ip set to pod IP; per validateClientIP comment requires agent >=6.17, ~2020) vs node_type added 2025-07-18. Found: agents in the 6.17..~7.67 range send X-Real-Ip but omit node_type. Conclusion: the two gaps occur INDEPENDENTLY; empty-X-Real-Ip (pre-6.17) is far older/rarer than omit-node_type. They do not necessarily co-occur. Resolved.

#### What does GetRunnerWorkers('')/GetRunnerStats('') do — immediate error vs blocking dial?

Examined pkg/util/clusteragent/clcrunner.go:81-97,133-198. Found: with IP=='', runnerURL uses net.JoinHostPort('',port)=':port', http.NewRequest succeeds, and clcRunnerAPIClient.Do(req) attempts a real dial; the client has Timeout=2s (line 86). Conclusion: neither an immediate error nor an unbounded block — each call blocks up to ~2s while the store write lock is held (dispatcher_nodes.go:201-224), so N empty-IP nodes cost up to N*2s of lock-stall. Resolved.


---
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
| **SUT-side instrumentation** | Primary assertion is workload-side over the workload's model of per-node-identity executing-config sets. Optional (high-value for trace correlation): in the instrumented DCA image, emit an Antithesis details event or assert.Reachable at the point in expireNodes()/reschedule() where digest D transitions from an expired node N to a new target M, tagged with D, N, M, and N's last heartbeat age. Requires adding github.com/antithesishq/antithesis-sdk-go to the root module (zero existing instrumentation, per existing-assertions.md) and building a custom DCA image; the //go:build clusterchecks tag must be present. |
| **Fault deps** | network partition workload-node-identity(N) <-> dca-leader for > node_expiration_timeout (default 30s) — ENABLED by default; WORKLOAD SUBSTITUTE (no fault needed): the workload simply stops POSTing heartbeat status for identity N while continuing to 'run' N's last-pulled config set, and keeps another identity M heartbeating — this deterministically drives expiry+reassignment with zero fault injection; NO node termination required; NO clock skew required (backward skew would only amplify by mass-expiring); requires leader_election enabled + a leader whose dispatcher.run is active (post-warmup) + >=2 simulated node identities |

**Open Questions:**

- Node-agent side (out of SUT scope): the guarantee that a partitioned-but-alive node keeps running its last-pulled cached checks must be modeled by the workload. DCA-side evidence supports the assumption (see log) but the node-agent cache/pull code is not in this repo's cluster-agent package. `(partial)`



#### Investigation Log

#### Q4: Endpoints checks node-pinned — scope to load-balanced cluster checks or carve out.

Examined dispatcher_main.go:188-198 (Schedule routes c.NodeName!="" to addEndpointConfig), dispatcher_endpoints_configs.go, stores.go:31,53, and expireNodes (dispatcher_nodes.go:142-186). Found: endpoints configs live in a separate store.endpointsConfigs map keyed by node name; expireNodes iterates ONLY node.digestToConfig and moves those to danglingConfigs — it never touches endpointsConfigs, so endpoints are not rescheduled via the dangling path. Conclusion: RESOLVED — the property must scope to load-balanced cluster checks; endpoints are carved out (different, node-pinned lifecycle).

#### Q5: Minimum stable-leadership duration to be post-warmup and reschedule (>warmup_duration 30s).

Examined handler.go:118-152 (warmup waits warmupDuration THEN starts runDispatch->d.run), dispatcher_main.go:377-421 (run sets store.active=true at entry; cleanupTicker = node_expiration_timeout/2 = 15s drives expireNodes+reschedule) and config defaults (warmup_duration=30s, node_expiration_timeout=30s). Found: during the 30s warmup store.active is false so processNodeStatus tells all nodes up-to-date (no dispatch); reschedule only fires on the 15s cleanup tick after run starts. Conclusion: RESOLVED — leadership must be held > warmup_duration (30s) and then up to one 15s cleanup tick before a reassignment occurs (~30-45s minimum).

#### Q6: Does shouldDispatchDangling require the target M to be a fresh node?

Examined dispatcher_configs.go:206-213 (shouldDispatchDangling: len(danglingConfigs)>0 && len(nodes)>0) and reschedule->add->getNodeToScheduleCheck (dispatcher_main.go:262-284, dispatcher_nodes.go:98-137: returns a random node or the node with fewest checks). Found: M can be ANY live node, including an already-loaded one; no freshness requirement. Conclusion: RESOLVED — the workload needs only >=1 surviving live identity to receive D; a dedicated spare identity is not required.

#### Q3: How long does the window last after heal — bounded by N's poll interval or extended by warmup?

Examined processNodeStatus (dispatcher_nodes.go:44-84). Found: outside warmup, a reconnecting N with a differing lastConfigChange gets IsUpToDate=false and re-pulls a D-free set within one poll cycle; but a heal landing inside a freshly-elected leader's 30s warmup returns true to all nodes, extending the window up to a full warmup. Conclusion: RESOLVED — bounded by one poll interval normally, extended by up to warmup_duration (30s) if heal coincides with a new leader's warmup (see property duplicate-execution-window-bounded-after-heal).

#### Q1: node-agent keeps running cached checks while partitioned (workload must model).

Examined dispatcher_nodes.go:73-79 warmup comment: 'We tell node-agents they are up to date to keep their cached configurations running.' Found: this is direct DCA-side evidence that the DESIGN INTENT is node-agents keep executing their last-pulled config set until told otherwise, supporting the workload's model. However the actual node-agent pull/cache execution code is not in the cluster-agent package (out of SUT). Conclusion: PARTIAL — assumption is well-supported by DCA source; the node-agent behavior itself must still be modeled by the workload and cannot be verified here.

#### Q2: accepted tradeoff vs defect to fix with fencing?

Examined expireNodes (time-based removal, no message to N), addConfig/removeConfig (no generation/epoch token), lastConfigChange (a bare equality counter, not an ownership fence, dispatcher_nodes.go:69,92). Found: there is provably no fencing/revocation mechanism and warmup intentionally keeps caches running. Conclusion: NEEDS-HUMAN — the code confirms the window exists by design; classifying it as accepted-tradeoff vs bug is a product decision, not a code fact.


---
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
| **SUT-side instrumentation** | MISSING. `assert.Always` around the executor keyed by (ID,Version) with a durable/observed dedup record; the property argues dedup must survive restart. Confirm ActionStore has no persistence before asserting the failure. |
| **Fault deps** | node termination (DISABLED by default — required for the crash-replay variant); leader election enabled + >=2 replicas (kubeactions is leader-gated; inert otherwise); network partition to induce split brain; requires remote config client |

**Open Questions:**

- Does backend ActionTTL/timestamp practice make the 1-min execution window commonly still-open at failover time in real deployments? Code confirms the only time gate is ValidateTimestamp vs action.Timestamp with ActionTTL=1m; whether the action's creation timestamp is still <1m at failover is a backend delivery-timing practice, not determinable from SUT code. `(needs human input)`



#### Investigation Log

#### Q1: Does the RC backend re-push an already-acknowledged K8S_ACTIONS config to a new leader, or only on version change?

Examined client.go update()/applyUpdate (513-547) and SubscribeIgnoreExpiration (393). Found: the RC client is per-process; the listener fires only for changedProducts, and it is handed state.GetConfigs(product) (the FULL active config set), not a delta (client.go:541). A freshly started leader pod = new process = empty client state, so the first successful poll marks every active K8S_ACTIONS config as changed and replays the full set to actionsCallback. Not gated on version change. NOTE: an in-place follower->leader promotion inside an already-running process does NOT re-fire the callback (config already in local state, no changedProducts). Conclusion: RESOLVED — redelivery to a NEW process is a full replay; the duplicate hinges on a new process (restart/new pod), which matches the property's node-termination fault requirement, not a mere leadership handover.

#### Q2: Are any executors truly idempotent (restart_deployment annotation patch may coalesce)?

Examined restart_deployment.go:61 and setup.go:73-77. Found: restart sets annotation kubectl.kubernetes.io/restartedAt = time.Now().Format(RFC3339) on every Execute — a new value each run, so a repeat is a distinct second rollout (NOT idempotent/coalescing). delete_pod/patch_deployment/rollback_deployment are likewise mutating. Only get_resource is read-only (harmless if repeated). Conclusion: RESOLVED — no mutating executor is idempotent; the annotation patch does not coalesce because the timestamp differs.

#### Q3: Is metadata.Version stable across redelivery (if it increments, ActionKey changes and dedup is moot)?

Examined processor.go:90-93 (ActionKey={ID,Version} from rawConfig.Metadata) and state/repository.go:218,447 + repository_test.go (Version stays 1 across redelivery, becomes 2 only on content change). Found: Metadata.Version is the TUF/backend config-file version, incremented only when config content changes, stable across redeliveries of the same config. Conclusion: RESOLVED — ActionKey is stable across redelivery, so dedup is meaningful WITHIN a process; the only failure mode is the empty in-memory map on a fresh process, exactly as the property claims (dedup key is not the problem).

#### Q5: Is there any persistence/leader-scoped fencing for ActionStore beyond the in-memory map?

Examined action_store.go (executed map[string]ActionRecord, sync.RWMutex; NewActionStore makes a fresh map, setup.go:42 calls it once per Setup) and cleanup() (only removes records older than RecordRetentionTTL). Found: no disk/CRD/k8s backing, no cross-replica sharing, no leader-generation/epoch fence. Claim inserts StatusClaimed but is not persisted anywhere durable. Conclusion: RESOLVED — dedup is purely in-memory and non-durable; the property's premise (state wiped on restart, no fencing) is confirmed.

#### Q4: backend ActionTTL window still-open at failover

Examined ValidateTimestamp/isExpired (action_store.go:78-104): the sole time bound is action.Timestamp age vs ActionTTL=1m, plus a 10s future-skew buffer. Whether a redelivered action is still within 1m at failover depends on backend delivery/timestamp practice, which is outside the cluster-agent code. Conclusion: code side confirmed (1-min gate is the only barrier and a prompt failover easily fits inside it), but the real-world commonality is a backend-practice judgment -> kept as needs-human.


---
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
| **SUT-side instrumentation** | Workload-side: track per-identity executing-set and assert D leaves N's set within K cycles after N resumes contact. High-value SUT-side addition: emit the IsUpToDate decision (which branch: equality/warmup/re-pull) from processNodeStatus so the trace attributes a sustained window to warmup vs stale-equality. Same SDK/custom-image prerequisite as the other properties. |
| **Fault deps** | network partition N<->leader > node_expiration_timeout then HEAL (leadership stable) — ENABLED by default; or workload-driven heartbeat stop then resume; to reach the warmup-extended and stale-equality cases: flap leadership (partition leader<->apiserver) so a fresh leader is in warmup / has a reset lastConfigChange when N reconnects — ENABLED by default; requires leader_election enabled + >=2 replicas + >=2 simulated node identities |

**Open Questions:**

- Node-agent re-pull speed after IsUpToDate=false is out of SUT scope; the workload must model a prompt re-pull for the K-cycle bound to be meaningful. DCA side returns false promptly (verified); node-agent poll cadence is not in this package. `(partial)`



#### Investigation Log

#### Q1: Does reset() zero nodeStore.lastConfigChange? If not, the stale-equality sustained-duplicate case is live.

Examined dispatcher.reset() (dispatcher_main.go:293-304) -> store.reset() (stores.go:42-55) which does s.nodes = make(map[string]*nodeStore), dropping every nodeStore; and getOrCreateNodeStore/newNodeStore (stores.go:86-137) which creates a fresh nodeStore whose lastConfigChange is the zero value 0 (never set in newNodeStore). The equality check (dispatcher_nodes.go:69) is node.lastConfigChange==status.LastChange. Found: after reset (or after expireNodes deletes the node), a reconnecting N always meets a fresh store with lastConfigChange=0; a node that has actually pulled configs posts a NONZERO status.LastChange (timestampNowNano set in addConfig/removeConfig, stores.go:140,151), so 0 != nonzero -> returns false -> N re-pulls. Equality can only falsely fire when status.LastChange==0 (a node that never pulled). Conclusion: RESOLVED — reset() effectively zeroes lastConfigChange (drops the nodeStore), so the stale/coincidental-equality sustained-duplicate scenario is NOT live for any node that had a real config; only the warmup case remains.

#### Q2: K (poll cycles to converge) — one interval, or exclude warmup?

Examined processNodeStatus (dispatcher_nodes.go:44-84) and getClusterCheckConfigs (28-40). Found: outside warmup, one status POST returns IsUpToDate=false and the next config poll returns N's current (D-free) dispatched set — convergence in a single poll cycle. Warmup blanket-returns true for up to warmup_duration (30s) and is documented intended-degraded behavior. Conclusion: RESOLVED — K = 1 poll interval outside warmup; warmup (30s) should be excluded from / added to the bound as intended degradation.

#### Q3: Node-agent re-pull speed after IsUpToDate=false.

Examined: DCA returns false promptly and immediately serves the D-free set on the next /configs pull. Found: how fast N issues that pull is governed by node-agent poll cadence, which lives outside the cluster-agent package (out of SUT). Conclusion: PARTIAL — DCA-side convergence is prompt and verified; the workload must model a prompt node re-pull for the K bound to be observable.


---
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
| **SUT-side instrumentation** | MISSING. `assert.AlwaysOrUnreachable` after Update checking success-or-retry and cache freshness. Confirm which provider (ConfigMap vs DatadogMetric CRD) is active via external_metrics_provider.use_datadogmetric_crd. **Resolved (user decision, 2026-07-21):** the harness pins the DatadogMetric CRD provider (`use_datadogmetric_crd: true`) as primary. This ConfigMap-path property is **deprioritized to secondary/optional** — run only if the legacy provider is separately exercised in a dedicated pass. |
| **Fault deps** | network partition producing split brain (two replicas writing; enabled by default); apiserver latency around lease renewal; requires leader_election enabled + >=2 replicas + external_metrics_provider enabled (ConfigMap store, not CRD). **Deprioritized (user decision, 2026-07-21): the harness pins the DatadogMetric CRD provider as primary; this ConfigMap-path property is secondary/optional** (run only if the legacy provider is separately exercised). |

**Open Questions:**

- Do real-world DCA deployments predominantly enable use_datadogmetric_crd (CRD path) vs the default legacy ConfigMap store? Code confirms the default is ConfigMap (use_datadogmetric_crd=false), so the RMW path is default-reachable; the adoption skew itself is a product fact not answerable from code. `(needs human input)`



#### Investigation Log

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
| **SUT-side instrumentation** | Add antithesis-sdk-go to the root module and instrument the externalmetrics package. (1) Witness: in processDatadogMetric, on the isLeader()==true branch, assert.Reachable("dca became leader with populated ddm store") guarded by store.Count()>0, and assert.Reachable("dca leader ran syncDatadogMetric after a flip") tagged with a monotonically-observed leadership epoch. (2) Convergence details for the workload to consume: export via the /metrics or a debug endpoint the per-metric UpdateTime/DataTime/Valid/Active and the current isLeader() + leadership epoch. Workload-side: after forcing a flip, poll for a new leader, then wait > (kubernetes_informers_resync_period + refresh_period), issue the HPA external-metric query against the new leader (or the Service), and assert.Sometimes(present && !stale, "new leader converged store served fresh HPA value post-flip"). Also assert.AlwaysOrUnreachable(present && !stale, "post-settle-window, active CRD-valid DatadogMetric is servable on the new leader") evaluated ONLY for queries taken after the settle window, so it degrades to Unreachable rather than false-failing during the legitimate convergence window. |
| **Fault deps** | leader_election enabled + >=2 DCA replicas + external_metrics_provider.enabled with the DatadogMetric CRD provider (not legacy ConfigMap); a leadership flip: preferred workload-driven substitutes that need NO node-termination — (a) partition current leader <-> kube-apiserver for >= leader_lease_duration (default 60s) to force lease loss, or (b) restart the leader DCA container (container restart is enabled by default), or (c) workload directly mutates/deletes the coordination.k8s.io Lease to trigger re-election; a stub dd-metrics-backend so the new leader's MetricsRetriever can return values (otherwise convergence to 'fresh' cannot be observed); NOT required: node-termination or clock-skew faults (both commonly disabled by default) |

**Open Questions:**

- Resolved (user decision, 2026-07-21): yes — the harness pins `use_datadogmetric_crd: true`; this is the PRIMARY external-metrics property.



#### Investigation Log

#### Q1: does the shared dynamic informer deliver a resync UpdateFunc at 300s, or is resync disabled?

Examined apiserver.go:184 (defaultInformerResyncPeriod = kubernetes_informers_resync_period) and :452 (DynamicSharedInformerFactory built with it), common_settings.go:565 (default 300s), datadogmetric_controller.go:87-92 (AddFunc/UpdateFunc/DeleteFunc all -> enqueue). Found: resync is enabled at 300s and UpdateFunc re-enqueues every DatadogMetric on each resync (client-go shared/dynamic informers deliver periodic resync as synthetic update events when resyncPeriod>0). Conclusion: RESOLVED -> worst-case re-reconcile of an inactive metric on a new leader <= ~300s.

#### Q2: does the harness enable use_datadogmetric_crd?

Examined common_settings.go:561 (default false) and antithesis/ (no harness config yet). Found: undecided. Conclusion: needs-human, kept.

#### Q3: is the metric kept Active across the flip or does AutoscalerWatcher briefly flip it Inactive?

Examined autoscaler_watcher.go:159 (leader-only), :150-151 (WaitForCacheSync on HPA/WPA listers), processAutoscalers -> updateDatadogMetricStatus:221-235 (only changes Active when computed `active` differs; `active` derived from live HPA references; on Active->Inactive it discards Valid). Found: a continuously-referenced metric stays Active (Valid preserved); a transient Inactive only if getAutoscalerReferences returns empty (e.g., HPA lister not yet synced on the new leader before first processAutoscalers). Conclusion: RESOLVED -> unservable-widening window exists only during the brief pre-sync interval, not for a stably-referenced metric.

#### Q4: can GetExternalMetric hit a new-leader before its informer's initial WaitForCacheSync completes?

Examined provider.go getExternalMetric:153-184 (store.Get with no isLeader and no cache-sync gate) vs datadogmetric_controller.go:115 (WaitForCacheSync gates only the controller worker loop, not APIService serving). Found: YES — a just-started/just-promoted replica can answer HPA queries before its store is populated, returning nil -> 'DatadogMetric not found' (provider.go:172-174). Conclusion: RESOLVED -> confirmed startup transient, distinct from the flip; worth a separate witness but does not change the flip-convergence invariant.


---
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
| **SUT-side instrumentation** | MISSING. `assert.Reachable` at the backoff-cap branch; `assert.Sometimes` on stale-but-served. Long timeline needed for 1800s cap — flag Antithesis timeline-length constraint. |
| **Fault deps** | sustained network partition DCA<->Datadog metrics backend, long duration (enabled by default); requires external_metrics_provider enabled + leader |

**Open Questions:**

- Does reaching the 1800s cap (~15-30+ min of sustained outage) fit a single Antithesis timeline? The backoff constants (2,30,1800,2) are a hardcoded package var at metrics_retriever.go:29 and are NOT config-driven — only external_metrics_provider.refresh_period (30s) is configurable, so shortening the cap for the test requires a code change, not config. `(needs human input)`



#### Investigation Log

#### Q1: default refreshPeriod; do rate-limit vs generic errors increment Retries differently?

Examined common_settings.go:552 (refresh_period default 30s), :569 (split_batches_with_backoff default FALSE), and metrics_retriever.go:87-88,147-148,165,183-188. Found: incrementRetries runs ONLY when splitBatchBackoffOnErrors==true AND the error is NOT RateLimitExceededError; rate-limit errors are batched with valid queries and never increment Retries; on a valid result in split mode Retries is reset to 0 (:148). CRITICAL: with the default config (split_batches_with_backoff=false) incrementRetries is never called, so Retries stays 0 and the backoff never grows. Conclusion: RESOLVED -> reaching the cap requires split_batches_with_backoff=true (non-default). See property_change.

#### Q2: is a Valid=false metric still returned by the provider or filtered before the HPA?

Examined datadogmetricinternal.go ToExternalMetricFormat:267-276 (returns d.Error / a 'stale' error when !Valid) and provider.go getExternalMetric:176-179 (propagates that error). Found: on the CRD/HPA provider path a Valid=false (stale/error) metric is NOT served as a value — the HPA receives an error. The 'serve stale data is better than none' comment (command.go:389-392) is about the WPA-controller/custommetrics path serving last-stored values flagged stale, a different path. Conclusion: RESOLVED -> the 'stale-marked but served' sub-assertion does not hold on the CRD path. See property_change.

#### Q3: does the 1800s cap fit a single timeline or must constants be shortened?

Examined backoff schedule + RetryAfter gating (metrics_retriever.go:101,186-188): each errored metric is re-queried only after its backoff elapses, so cumulative wait to Retries>5 ~= 30+60+120+240+480 ~= 15.5 min then 1800s spacing. Found: backoff params are a hardcoded package var (metrics_retriever.go:29), not config-driven. Conclusion: whether ~15-30+ min fits a run is a harness/duration decision -> needs-human; noted that constants cannot be shortened via config.


---
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
| **SUT-side instrumentation** | In updateDatadogMetric (datadogmetric_controller.go:318), before UpdateStatus, compute the existing CRD status's Active/Updated timestamps and Value and assert.AlwaysOrUnreachable(newStatus not older than existing, "ddm status write does not regress DataTime/Value"). Also assert.Reachable("ddm updateDatadogMetric executed on a post-flip new leader") tagged with the leadership epoch, so the safety assertion is only meaningful once the witness confirms the write path ran after a flip. The workload should record each committed CRD status (Value + condition timestamps) and independently assert non-decreasing DataTime across observed leadership epochs. |
| **Fault deps** | same as the convergence property: leader_election + >=2 replicas + DatadogMetric CRD provider + stub dd-metrics-backend; a leadership flip via apiserver partition (>=60s), leader container restart, or Lease mutation — no node-termination/clock-skew needed; to stress the .Unix() granularity, the ability to drive two flips within a short window (workload-controlled Lease churn) |

**Open Questions:**

- Can an informer-lagged store entry (follower mirrored a STALER CRD than the last committed status) combined with AutoscalerWatcher's Active-only UpdateTime bump (autoscaler_watcher.go:~224) make IsNewerThan pass and republish an OLDER DataTime? The guard inspects only the Active-condition timestamp, never DataTime, so it does NOT prevent this; needs the harness to schedule the lag+bump interleaving to demonstrate whether it is actually reachable. `(partial)`



#### Investigation Log

#### Q1: can Active LastUpdateTime advancing while Value/DataTime stay old publish an older DataTime?

Examined autoscaler_watcher.go updateDatadogMetricStatus (bumps in-memory UpdateTime=time.Now() on Active/reference change, leaves DataTime/Value), datadogmetric_controller.go:274 IsNewerThan gate -> updateDatadogMetric, and datadogmetricinternal.go:192-204 (compares CRD Active LastUpdateTime vs in-memory UpdateTime only). Found: the Active-only bump can make IsNewerThan return true, triggering a status write carrying the OLD DataTime/Value. In the common case the store was reconstructed from the SAME CRD (follower path NewDatadogMetricInternal) so the written DataTime EQUALS the CRD's (no regression). A true OLDER-DataTime regression requires the store to have mirrored a STALER CRD version than the last committed status (informer lag) — which the guard does not protect against. Conclusion: PARTIAL -> mechanism is real and unguarded for DataTime; reachability of the lag window is a harness-scheduling question, kept partial.

#### Q2: does the 1-second .Unix() truncation permit an equal-second update to overwrite an older sub-second Value?

Examined datadogmetricinternal.go:196 — `if condition.LastUpdateTime.Unix() >= d.UpdateTime.Unix() { return false }`. Found: equal-second is REJECTED (>=), so no write occurs within the same second; the truncation is conservative on ties. Conclusion: RESOLVED -> it cannot overwrite with an older sub-second value; the only failure mode from the truncation is a SKIPPED legitimately-newer sub-second update (a missed update, not a regression).

#### Q3: is monotonicity the right notion given DataTime and UpdateTime are distinct?

Examined IsNewerThan (uses internal UpdateTime = status-write time, via the Active condition) vs BuildStatus:249 (Updated condition stamped from DataTime). Found: the guard enforces monotonicity of the Active-condition LastUpdateTime / internal UpdateTime, NOT of DataTime. Conclusion: RESOLVED -> the invariant as stated ('Active-condition LastUpdateTime non-decreasing') is exactly what the guard guarantees and is the correct thing to assert; DataTime-non-regression is a SEPARATE, unguarded property that is the real bug-probe (ties to Q1).


---
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
| **SUT-side instrumentation** | MISSING. `assert.Always` at the route handler asserting the path resolved to a real handler; the cleaner fix installs routes before Serve. A `Reachable` on the gap window confirms it is exercised. |
| **Fault deps** | network latency/congestion on DCA<->apiserver to widen the startup window (enabled by default); concurrency (mux mutation vs Serve); requires leader_election enabled |

**Open Questions:**

- Node-agent behavior on an authenticated 404 vs 503 (retry vs disable cluster-check polling) — lives in node-agent code, out of DCA/SUT scope; governs blast radius, not existence of the gap. `(needs human input)`



#### Investigation Log

#### Does client-go's default 404 return before or after token validation?

Examined server.go:83 `httpHandler := validateToken(ipc)(router)` and validateToken (server.go:174-196): the mux default 404 is emitted by router/apiRouter only inside next.ServeHTTP (line 193), which runs AFTER auth succeeds. Found: an unauthenticated probe gets 401/403 from TokenValidator (util_dca.go:109/117/124), never the mux 404. Conclusion: RESOLVED — the 404 window is observable only to an authenticated caller (node agent presenting the DCA token); assert/witness must sit on the auth-passed path.

#### Exact upper bound of WaitForAPIClient under partition (max window size).

Examined apiserver.go:188-194 (retrier Strategy=Backoff, InitialRetryDelay 1s, MaxRetryDelay 5m) and WaitForAPIClient loop (apiserver.go:212-232) plus retrier.go:120-144. Found: the Backoff branch NEVER sets PermaFail (only OneTry/RetryCount do); the loop exits only on retry.OK or ctx.Done(). mainCtx is not deadline-bounded. Conclusion: RESOLVED — upper bound is effectively unbounded (whole partition duration / process lifetime); per-retry sleep capped at 5m. The 404 window width is bounded only by how long the apiserver is unreachable.

#### Node-agent behavior on authenticated 404 vs 503 (out of scope).

Not resolvable from cluster-agent source — requires node-agent config-provider retry logic (separate SUT). Kept as needs-human; it is a blast-radius question.

#### Whether a node-agent request can arrive before WaitForAPIClient returns (listener reachability vs apiserver readiness).

Examined command.go:368-376: StartServer (which opens the listener at server.go:87 and does `go srv.Serve` at server.go:150) is called at command.go:369, BEFORE apiserver.WaitForAPIClient at command.go:376. Found: the TLS listener is accepting connections before WaitForAPIClient (and thus ModifyAPIRouter at ~command.go:534) runs. Conclusion: RESOLVED — yes, a node-agent request can arrive in the gap; listener reachability strictly precedes route registration.


---
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
| **SUT-side instrumentation** | MISSING. `assert.Always(token != "")` guarding the compare in validateToken and the gRPC auth path; plus a `Reachable` on the startup window where the API server serves before the token is loaded. |
| **Fault deps** | filesystem/IO latency or error injection during startup (enabled by default); apiserver failure during token ConfigMap read; clock jitter (not required) |

**Open Questions:**

- Whether any real deployment leaves cluster_agent.auth_token unset AND the token-file directory (ConfFileDirectory, security.go:204) unwritable (read-only rootfs), making the DCA token durably empty rather than transiently — a deployment/ops call. `(needs human input)`



#### Investigation Log

#### Default auth_init_timeout and how easily an IO/latency fault pushes FetchOrCreateArtifact past it.

Examined common_settings.go:1135 `BindEnvAndSetDefault("auth_init_timeout", 30*time.Second)`. FetchOrCreateAuthToken/FetchOrCreateArtifact (security.go:173-207) runs under this 30s ctx; an injected IO/latency fault exceeding 30s makes CreateOrGetClusterAgentAuthToken return ("",err) → InitDCAAuthToken sets dcaToken="" and returns err → discarded at server.go:95. Conclusion: RESOLVED — default 30s.

#### Whether the local IPC token can independently be empty during the same window.

Examined command.go:265 (DCA start uses ipcfx.ModuleReadWrite → ipcimpl.NewComponent). NewComponent (ipc.go:71-93) calls FetchOrCreateAuthToken under auth_init_timeout and RETURNS AN ERROR on failure (ipc.go:79-82) → fx graph fails → the DCA process does not start/serve. Only NewInsecureComponent (ipc.go:108-132, flare/diagnose) sets token="". Conclusion: RESOLVED — the local IPC token CANNOT be transiently empty in a serving DCA (fail-fast); the 'internal endpoints also compromised' sub-scenario is unreachable in the server process. Empty-token risk applies only to the DCA token surface, whose init error is discarded.

#### Whether any deployment leaves auth_token empty AND the token file unwritable.

cluster_agent.auth_token unset is the default (security.go:197-207 falls through to a generated file in ConfFileDirectory). Read-only conf dir → FetchOrCreateArtifact create fails → durably empty. Whether a real chart mounts that dir read-only is a deployment/ops call not encoded in agent source. Kept needs-human.

#### Does the API server accept authenticated requests before the token is populated (command.go:368 vs token init)?

CORRECTION to scratchbook premise: InitDCAAuthToken is called at server.go:95 SYNCHRONOUSLY INSIDE StartServer, BEFORE `go srv.Serve` at server.go:150. Token load is therefore ordered strictly before serving — there is NO transient race window where the server serves before init runs. Conclusion: RESOLVED — the DCA token is empty at serve time ONLY if InitDCAAuthToken FAILED (auth_init_timeout exceeded); it is then DURABLY empty (InitDCAAuthToken early-returns when dcaToken!="" at util_dca.go:38, and nothing re-invokes it), lasting the whole process lifetime.


---
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
| **SUT-side instrumentation** | Add antithesis-sdk-go to the root module and instrument common.go:GetOrCreateClusterID to emit the resolved cluster ID with a stable key per replica (e.g. assert.Always with details {replica, clusterID}). Simpler workload-only alternative: after both replicas are up, the workload reads datadog-cluster-id ConfigMap and each replica's /metadata or status-exposed cluster ID via the DCA API and asserts equality. Pair with the witness property below so a green result is not vacuous. |
| **Fault deps** | None beyond concurrent startup of >=2 DCA replicas against a SHARED real kube-apiserver (no fake clientset). Node-termination and clock-skew NOT required. |

**Open Questions:**

- Harness verification only: does the harness co-locate both DCA replicas in the same namespace so they contend on the same datadog-cluster-id ConfigMap? Code confirms GetOrCreateClusterID uses namespace.GetMyNamespace() (own pod namespace, common.go:50), so same-Deployment replicas contend; the placement itself is a harness-config check. `(partial)`
- Does WaitForAPIClient (command.go:376) unblocking near-simultaneously for both replicas measurably widen the collision window? Code confirms both replicas gate on it and proceed to GetOrCreateClusterID at command.go:433, so it synchronizes them; the magnitude of widening is a harness measurement. `(partial)`
- In Operator/Helm deployments is datadog-cluster-id ever pre-created, making the NotFound/Create branch unreachable in production? External chart config, not in this repo. `(needs human input)`



#### Investigation Log

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
| **SUT-side instrumentation** | MISSING. `assert.AlwaysOrUnreachable` on the unknown-node branch checking the response is the distinct code, not 500. Node-agent-side retry semantics are out of SUT scope — flag the interaction. |
| **Fault deps** | network partition / request reordering (POST vs GET; enabled by default); clock skew on wall-clock expiry (DISABLED by default); requires leader_election enabled + >=2 replicas |

**Open Questions:**

- Node-agent client retry/backoff on 500 vs 4xx for GET configs — determines whether an unknown-node 500 merely delays or fully stalls config propagation. Node-agent code, out of DCA/SUT scope. `(needs human input)`
- Whether any production monitor/SLO pages on DCA 5xx rate (would make the false-500 operationally severe) — observability config, not in repo. `(needs human input)`



#### Investigation Log

#### Node-agent retry/backoff on 500 vs 4xx (out of scope).

Verified the DCA-side assertion is real: dispatcher_nodes.go:32-35 returns `node %s is unknown` for !found; handler_api.go GetConfigs propagates it; clusterchecks.go:81-84 maps any error to http.StatusInternalServerError (500), indistinguishable from a genuine failure or a json.Marshal error. Not-resolvable part: the node-agent retry/backoff behavior is in node-agent code. Conclusion: core defect code-confirmed; retry impact kept needs-human.

#### Whether any monitor/SLO pages on DCA 5xx rate.

Not derivable from cluster-agent source (operational/observability configuration). Kept needs-human.

#### How does the node-agent client treat 500 vs 4xx here?

Duplicate of Q1 — same out-of-scope node-agent concern; consolidated into the single retry/backoff needs-human item.


---
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
| **SUT-side instrumentation** | MISSING. `assert.AlwaysOrUnreachable` comparing isExternalPath output to a declared per-route auth class; ideally the auth class becomes a route attribute rather than a segment-count heuristic. |
| **Fault deps** | none required (input-domain property; not fault-timing dependent); needs a workload that presents both tokens against each endpoint and path variant |

**Open Questions:**

- None — all resolved during investigation (see evidence file Investigation Log)



#### Investigation Log

#### Does any current intra-pod endpoint collide with an external prefix+count?

Audited all apiRouter routes (v1/*.go) and root IPC routes (agent.go) against isExternalPath (server.go:199-219). Found: NO intra-pod-only endpoint is currently misclassified external (the dangerous local-client-lockout direction). CLI-only routes fall through correctly: /clusterchecks (getState, 4 seg), /clusterchecks/rebalance (5 seg), /clusterchecks/isolate/check/{id} (7 seg), /endpointschecks/configs (5 seg), /tags/pod all (5 seg). Instrumentation /configs & /status (==5, external) ARE node-agent-facing (DCAClient, pkg/util/clusteragent/instrumentationchecks.go) so external is correct. One inconsistency the OTHER direction: /api/v1/info/node/{nodeName} is a genuine DCA-token endpoint (DCAClient.GetNodeInfo, clusteragent.go:411-421, called from cloudprovider.go:53) but has NO isExternalPath clause → classified non-external. Conclusion: RESOLVED — no internal→external collision today; /info/node is an external endpoint missing from the classifier (see property_change).

#### Do in-process/CLI clients ever hold the DCA token?

Examined DCA CLI subcommands: status (command.go:54 ipcfx.ModuleReadOnly + ipchttp client) and clusterchecks (pkg/cli/subcommands/clusterchecks/command.go:123,133,179 use ipc.HTTPClient) — all use the LOCAL IPC token. The DCA token (security.GetClusterAgentAuthToken, clusteragent.go:142) is held only by DCAClient (node-agent→DCA and DCA self-calls like GetNodeInfo). Conclusion: RESOLVED — CLI/local clients use the local token, NOT the DCA token, so fragility #1 (internal endpoint misclassified external → local client 403) is NOT masked; it would be a real breakage.

#### Is the query-string inclusion in r.URL.String() intentional vs r.URL.Path?

Examined server.go:180 `path := r.URL.String()` (includes query/fragment); no comment or code supports it as deliberate. buildQueryList (clusteragent.go) appends `?filter=` to real node endpoints (e.g. info/node, cf/apps), so query strings do reach the classifier and a '/' in a query value flips the segment count. ServeMux itself routes on Path. Conclusion: RESOLVED — using r.URL.String() is a latent defect, not intentional; r.URL.Path is the canonical key. The scratchbook assertion isExternalPath(Path)==isExternalPath(String()) is a valid check.

#### Is there a canonical per-route auth-class declaration, or is isExternalPath the only source of truth (tautological)?

Examined route registration (agent.go, v1/*.go): routes are registered via http.ServeMux HandleFunc with NO auth metadata. isExternalPath is the only server-side source of truth → asserting it against itself is tautological. The non-tautological ground truth is client-side: DCAClient methods (pkg/util/clusteragent/*, DCA token) = intended-external set; ipc.HTTPClient CLI callers (pkg/cli/subcommands/*) = intended-internal set. Conclusion: RESOLVED — no server-side declaration exists; the workload must derive intended auth-class from client call-site token choice.


---
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
| **SUT-side instrumentation** | Add the Antithesis Go SDK to the root module and instrument pkg/util/kubernetes/apiserver/controllers/store.go and cmd/cluster-agent/api/v1/kubernetes_metadata_stream.go. In StreamKubeMetadata, tag each handler with a unique per-connection token and record (nodeName, token, ch) into a test-only live-handler set on Subscribe and remove it on handler return. Under m.mu / metadataMutex, add assert.AlwaysOrUnreachable that the multiset of channels in subscribers[nodeName] equals the multiset of channels held by currently-live handlers for that node (details: nodeName, lenRegistry, lenLiveHandlers). Also assert.AlwaysOrUnreachable inside Unsubscribe that the channel being removed is one this node's handler actually created (guards against identity confusion). Keep the added tracking map lock-consistent with the registry it mirrors. |
| **Fault deps** | workload-driven stream drop+reconnect for the same nodeName (client close / context cancel) — NO node-termination or clock-skew fault required; this is the workload substitute; optional: asymmetric network partition workload(as node agent)<->DCA to force the old stream to RST while the new one connects (enabled by default); concurrency / goroutine interleaving (always on) — the core enabler; does NOT require leader_election or >=2 replicas (stream serving is not leader-gated), though a >=2-replica run adds reconnect churn via leadership-driven client failover |

**Open Questions:**

- Is the DCA tagger gRPC stream actually consumed in the target harness topology? Code shows node agents' remote tagger dials the LOCAL core agent (remote.go start()/options.Target), while the DCA tagger server is consumed by in-cluster DCA-targeting clients via cluster_agent.cluster_tagger (e.g. cluster-check runner). Confirm whether the harness runs such a consumer, else the tagger arm of this property is inert. `(partial)`



#### Investigation Log

#### Q1: Does the workload hold nodeName stable across reconnects like a real node agent?

Examined comp/core/workloadmeta/collectors/internal/kubemetadata/stream.go:488,657. Found: nodeName is captured once in newDCAStreamClient(nodeName,cfg) (L82,486-488) and reused as sc.nodeName on every StreamKubeMetadata call (L656-657); reconnects reuse the same value. Server keys by req.GetNodeName() (kubernetes_metadata_stream.go:151). Concluded: SUT reuses a fixed node identity across reconnects, so the nodeName-keyed collision reproduces only if the workload likewise holds nodeName stable — a workload-authoring requirement (do not randomize per connection), now confirmed against the real client. Resolved.

#### Q2: Other consumers of Subscribe/Unsubscribe for the same nodeName?

Examined grep of MetaBundleStore.Subscribe/Unsubscribe and GetGlobalMetaBundleStore across pkg/util/kubernetes/apiserver, cmd/cluster-agent, pkg/clusteragent. Found: the ONLY caller is kubernetes_metadata_stream.go:153-154; GetGlobalMetaBundleStore() has a single caller, grpc_kubemetadata.go:19. Other Subscribe/Unsubscribe hits are unrelated types (workloadmeta wlm, RC client, leaderEngine). Concluded: no other consumer can concurrently register/deregister for the same nodeName; accounting is not confounded. Resolved.

#### Q3: Does slices.Delete's in-place shift alias a channel pointer unsafely under m.mu?

Examined store.go: Subscribe (L119-128, m.mu.Lock append), Unsubscribe (L132-150, m.mu.Lock + slices.Delete), notifyLocked (L154-164, requires lock; called only from set()/delete() which hold m.mu.Lock at L99/L109). Get (L58) takes RLock but never touches subscribers. Concluded: every read and write of the subscribers slice header occurs under the m.mu write lock; notify and Unsubscribe are mutually exclusive, no slice header is read outside the lock, so the in-place shift is safe. Resolved.

#### Q4: Can two paths call unsubscribe for the same id (double-decrement)?

Examined tagger subscription_manager.go:91-119 and kube-metadata store.go:132-150. Found: tagger unsubscribe re-checks `found` (L92-96), then delete()+close(sub.ch)+Subscribers.Dec() (L116-118); Notify may force-unsubscribe a full-channel subscriber (L152) racing the handler's deferred Unsubscribe (server.go:125), but the second call hits the not-found guard -> no-op, no double-close, no double-Dec. Kube-metadata store has NO server-initiated force-unsubscribe (notifyLocked only non-blocking sends), so each handler's channel is removed exactly once by its single deferred Unsubscribe; store keeps no per-node gauge. Concluded: no double-decrement/double-close on either path. Resolved.

#### Q5: Is the tagger gRPC stream consumed by node agents (remote_tagger) or only in-cluster?

Examined comp/core/tagger/impl-remote/remote.go (start()/DialContext to options.Target; maxMsgSize from cluster_agent.cluster_tagger.grpc_max_message_size L228) and startTaggerStream L646-648. Found: node agents' remote tagger dials a configured Target (local core agent in the standard node topology); the DCA tagger server (cmd/cluster-agent/api/server.go:131) is consumed by DCA-targeting clients (cluster-check runner via cluster_agent.cluster_tagger). Critically, StreamingID = fmt.Sprintf("%s:%s", flavor, uuid.New()) FRESH per startTaggerStream, so reconnects never collide on subscriptionID. Concluded (partial): the nodeName-style drop-on-reconnect collision CANNOT occur for the tagger path; whether the harness actually runs a DCA tagger consumer is topology-dependent and unresolved from code.

#### Q6: Does the throttler token release path leak a token without leaking a subscriber?

Examined comp/core/tagger/server/syncthrottler.go:55-62 and server.go:114-166. Found: Release is idempotent (guarded by activeRequests `found` check before draining tokensChan and delete). TaggerStreamEntities acquires one token (L115), releases it once initBurst completes (L164) and again via defer (L116) — the second is a guaranteed no-op. Token lifetime is intentionally decoupled from subscription lifetime: the token gates only the initial sync burst, while the subscription persists for the whole stream. Concluded: no token leak (idempotent double-release) and no coupling that could leak a subscriber vs a token. Resolved.

#### Q7: Is len>=2 reachable with a single workload node identity?

Examined store.go:119-128 (Subscribe appends) and the reconnect-overlap mechanism. Found: when a stream drops and the same-nodeName stream reopens before the old handler's deferred Unsubscribe (kubernetes_metadata_stream.go:154) runs, the new Subscribe appends a second channel -> subscribers[nodeName] has length 2 with a single node identity. The multi-process comment (L96-100) is an additional, not required, route. Concluded: len>=2 is reliably reachable with one node identity via drop+reconnect overlap; modeling agent+diagnose separately is unnecessary. Resolved.

#### Q8: Should the witness also fire on the namespaceSubscribers registry independently?

Examined kubernetes_metadata_stream.go:153-157,407-439 vs store.go:119-150. Found: each handler registers in TWO independent registries under DIFFERENT locks — subscribers (m.mu, store.go) and namespaceSubscribers (metadataMutex, stream.go) — each with its own append/identity-delete + deferred Unsubscribe. Because they are separate lock domains, a given schedule can produce an overlap window in one registry but not the other at the same observation instant. Concluded: yes, the witness/assertion should be instrumented on both registries independently rather than assuming they move in lockstep. Resolved.


---
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
| **SUT-side instrumentation** | Net-new (zero existing SDK usage per existing-assertions.md). Add the Antithesis Go SDK to the root module and instrument at each lister read on a decision path: (1) record the informer's last-observed watch-event/resync timestamp (or LastSyncResourceVersion) alongside the served value; (2) at the read, emit assert.AlwaysOrUnreachable(fresh_or_surfaced) comparing (now - lastSync) < B OR staleness surfaced. Simplest concrete probe: instrument GetCertificateFromLister (admission) and the endpoint/metadata lister read. Pair with the witness property below so a green result is not vacuous. Because the assertion is expected to fail, the deliverable is the reproducing trace: stale value served while the pod is Ready. |
| **Fault deps** | network partition DCA-replica <-> kube-apiserver, silent/blackhole drop (no RST/FIN) so the watch stream hangs — enabled by default on most tenants; workload-driven mutation of the watched object during the partition (rotate admission Secret / change DCA EndpointSlice / update kube-service endpoints) — the topology already gives the workload apiserver ownership of Service/EndpointSlice objects; no node termination or clock skew required; requires whichever informer path is enabled: admission_controller.enabled, or kube-service metadata, or a CRD controller |

**Open Questions:**

- Which staleness budget B is defensible per surface? No B is defined anywhere in code (HasSynced latches, no staleness metric). Two natural bounds exist: (a) ~45s = HTTP/2 dead-connection detection window (relist trigger); (b) the per-surface correctness window (cert rotation vs endpoint churn). Choosing the authoritative B is an intended-behavior call. `(needs human input)`
- Which surface (admission Secret / EndpointSlice / kube-endpoints / CRD) most reliably opens the freeze window under the harness partition primitive? From code the admission cert is the most deterministic decision-path read (read from secretsLister on every TLS handshake, server.go:137-140); metadata is served from a reconciled store fed by informer events. Ranking requires empirical harness measurement. `(partial)`



#### Investigation Log

#### Q1: Does client-go HTTP/2 ReadIdleTimeout ping detect the blackholed connection and relist, or block indefinitely?

Examined k8s.io/client-go@v0.35.5/transport/cache.go:134 (builds transport via utilnet.SetTransportDefaults) and k8s.io/apimachinery@v0.35.5/pkg/util/net/http.go:131-190. Found: SetTransportDefaults calls configureHTTP2Transport, which unconditionally sets t2.ReadIdleTimeout=30s and t2.PingTimeout=15s unless DISABLE_HTTP2 or HTTP2_READ_IDLE_TIMEOUT_SECONDS=0 env vars are set. Grep confirms the agent sets neither (no DISABLE_HTTP2 anywhere in repo). The rest.Config path (no custom Transport; only clientConfig.Wrap of the RoundTripper, apiserver.go:274) uses this default transport. Conclusion: the HTTP/2 health-check ping IS enabled by default. Under a silent blackhole, the ping fails and the dead connection is torn down after ~30-45s, forcing a watch relist. The 0 client-side timeout (kubernetes_apiserver_informer_client_timeout=0) does NOT make the freeze indefinite — transport-layer ping bounds it. RESOLVED.

#### Q3: Does any readiness/health check flip unready under a frozen informer?

Examined cmd/cluster-agent/admission/server.go:91,176-178 and pkg/status/health/global.go. Found: the admission webhook registers health.RegisterReadiness('admission-controller-webhook') but the Run loop only drains s.healthHandle.C ('// Drain the health check channel to stay healthy'). The health package is a pure goroutine-liveness ping (30s timeout); it has zero coupling to informer watch liveness or LastSyncResourceVersion. No readiness gate anywhere ties to informer freshness. Conclusion: readiness is fully decoupled from informer liveness; a frozen informer keeps the pod Ready. RESOLVED (confirms property premise).

#### Q4: Are endpoints/metadata served to node agents read from the informer lister or re-fetched directly?

Examined pkg/util/kubernetes/apiserver/controllers/metadata_controller.go:98-110,296 and cmd/cluster-agent/admission/server.go:137-140. Found: metadata controller reads m.endpointSliceLister / m.endpointsLister.Endpoints(ns).Get(name) (line 296) — informer-backed lister; results reconciled into globalMetaBundleStore and served to node agents. Admission cert read from s.secretsLister on every TLS handshake. Both are informer-lister-backed and WILL freeze. Contrast: GetLeaderIP uses direct Endpoints().Get/EndpointSlices().List (per discovery evidence, leaderelection.go) — not informer, would not freeze. Conclusion: the node-agent metadata/endpoint path and the cert path are lister-backed. RESOLVED.

#### Q5: How long does the divergence persist (partition-bounded vs indefinite)?

Derived from Q1 plus apiserver.go:184 / common_settings.go:565. Found: HTTP/2 ping tears down the connection ~30-45s after silence and triggers a relist; the relist fails/retries during the partition, so the cache stays stale for the partition duration, then converges once the partition heals. kubernetes_informers_resync_period default 300s does NOT help — client-go resync re-delivers cached objects to handlers, it does not re-fetch from the apiserver. Conclusion: divergence is PARTITION-BOUNDED (~45s detection + partition duration + relist latency), NOT indefinite; it self-heals when the partition ends. RESOLVED.


---
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
| **SUT-side instrumentation** | MISSING. `assert.Sometimes` on bounded shutdown completion under partition; the release call should have a timeout — the assertion documents the requirement. |
| **Fault deps** | network partition (leader<->apiserver; enabled by default); SIGTERM delivery (ordinary shutdown; node termination graceful mode); requires leader_election enabled + >=2 replicas |

**Open Questions:**

- Do long-lived gRPC/HTTP streams (tagger/kube-metadata) or an fx OnStop hook prolong process exit? (Largely moot: the release call is bounded and not awaited, so no indefinite hang either way.) Confirmed only that shutdown does NOT wait on the leader-election goroutine (command.go:746 wg covers only extmetrics+admission) and StopServer() is never called; the primary API/gRPC listener drain path on exit was not fully traced. `(partial)`



#### Investigation Log

#### Q1: Default of leader_election_release_on_shutdown

Examined pkg/config/setup/common_settings.go:282 and pkg/util/kubernetes/apiserver/leaderelection/leaderelection_engine.go:198. Found BindEnvAndSetDefault("leader_election_release_on_shutdown", true). Concluded: default TRUE, so the ReleaseOnCancel path is active by default. RESOLVED.

#### Q3: Does client-go ReleaseOnCancel inherit the cancelled ctx or use a bounded context?

Examined client-go v0.35.5 (go.mod) tools/leaderelection/leaderelection.go:304-335. Found release() at :311 creates a FRESH context.Background(), NOT the cancelled ctx; renew() calls release() synchronously after the renew loop exits (:305). Concluded: does not inherit the cancelled ctx. RESOLVED.

#### Q4: Does the ReleaseOnCancel call have a timeout, or block indefinitely under partition?

Examined leaderelection.go:312 context.WithTimeout(ctx, le.config.RenewDeadline); both Lock.Get (:315) and Lock.Update (:335) use that timeoutCtx. RenewDeadline = LeaseDuration/2 (leaderelection_engine.go:201), i.e. 30s at the 60s default. Concluded: the release is BOUNDED (max ~RenewDeadline per call), cannot block indefinitely. RESOLVED.

#### Q2: Do long-lived gRPC streams keep the process alive at exit vs listener closing fast?

Examined command.go:724-760 shutdown path. Found mainCtxCancel() then wg.Wait() (:746) covers only external-metrics + admission webhook servers; the leader-election goroutine is launched bare (leaderelection.go:199) and is NOT in wg, and cmd/cluster-agent/api/server.go StopServer() is never called. Did not fully trace whether an fx OnStop hook drains the primary API/gRPC listener and prolongs exit. PARTIAL (and moot given bounded, un-awaited release).


---
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
| **SUT-side instrumentation** | MISSING (zero existing SDK usage). (1) In GetCertificate (server.go:137-144), after the fetch: assert.AlwaysOrUnreachable(err != nil || cert != nil, "admission webhook returns a non-nil cert whenever the secret lister has it", details{err, isLeader, secretName}) — plus emit details tagging the replica's leader state and whether the informer HasSynced, so a nil-cert event is attributable to unsynced-cache vs partition vs rotation. (2) Workload-side (preferred, runs against a stock image): while the cert Secret exists in the apiserver, POST a probe AdmissionReview to the webhook Service and assert the TLS handshake + AdmissionResponse succeed — this is the end-to-end availability assertion and does not require SDK inside the DCA. Pair with the witness property below so the AlwaysOrUnreachable is not vacuous. |
| **Fault deps** | network partition leader<->apiserver (enabled by default) to freeze a replica's secret informer and to churn leadership >= leader_lease_duration; node termination of the leader (DISABLED by default) — only needed for the crash-based churn variant; partition-based churn is a workload-driven substitute; workload must drive admission traffic: POST AdmissionReview probes to the webhook Service endpoint during/after the fault, and observe the cert Secret existence via the apiserver as ground truth; requires admission_controller.enabled + leader_election enabled + >=2 replicas; failure_policy is deploy-config (does not gate the DCA-side assertion) |

**Open Questions:**

- Does the webhook Service (admission_controller.service_name) select ALL DCA pods or only the leader in the harness? DCA code serves on every replica (command.go:648, no leader gate on server.Run :712), but the harness workload 'manages the DCA Service + EndpointSlice' (deployment-topology.md:52) so the selector is a harness-authoring decision not yet pinned. Confirm the shipped Service selector when the harness is built. `(needs human input)`
- Rotation-during-churn: when the leader regenerates the cert AND its CABundle while a follower is partitioned, does the follower's cached old cert remain trusted or get rejected by the new CABundle? Cert rotation is rare (yearly; 30d-before refresh) and CABundle update is leader-gated (controller_base.go:286,299); the code does not guarantee a partitioned follower's old cert matches a freshly-rotated CABundle. Resolving the actual trust outcome needs the cert-controller regeneration/CABundle semantics plus an intended-behavior call. `(partial)`
- Default admission_controller.failure_policy rendered in the harness (code default 'Ignore', common_settings.go:653; harness/Helm value not pinned in topology docs). Sets severity (silent mutation-drop vs cluster-wide pod-creation block); the DCA-side assertion is policy-agnostic. `(needs human input)`
- Witness design: require the serving replica to be a non-leader (stronger, proves not-leader-gated serving) or accept any replica? Pure test-design choice (scratchbook advises 'start permissive, tighten'); no code answer. `(needs human input)`



#### Investigation Log

#### Q2: Fresh/never-synced replica — is serving a nil cert reachable in harness startup ordering (server starts before/independently of SyncInformers)?

Examined command.go:687-716 and pkg/clusteragent/admission/start.go:118-154. server.Run (command.go:712) starts ONLY inside the else-branch entered when StartControllers succeeds. StartControllers ends with apiserver.SyncInformers(informers,0) (start.go:154) which WaitForCacheSync's the Secrets informer (util.go:37-58) with kube_cache_sync_timeout_seconds (default 10, common_settings.go:284). If the secret informer never syncs, StartControllers returns a SecretsInformer SyncInformersError → command.go:690 condition true → logs 'Could not start admission controller' and the server is NOT started. Concluded: a never-synced replica does NOT serve a nil cert — it fails to bring up the webhook server entirely; the informer must have synced >=1 before serving. NOT reachable. RESOLVED (strengthens the invariant).

#### Q4: Does client-go's lister retain the last-synced Secret across a watch disconnect with informer_client_timeout=0?

client-go informers serve reads from a local thread-safe cache (Indexer); a watch disconnect does not evict entries — the reflector re-lists/re-watches on reconnect and does not clear the store on transient error. Concluded: the last-synced Secret is retained across a disconnect, so a synced replica keeps returning the cached cert through a partition. Underpins the 'synced replica keeps serving' invariant. RESOLVED (design of client-go; discovery evidence concurs).

#### Q7: Can the workload reliably observe 'churn in effect'?

Examined antithesis/scratchbook/deployment-topology.md:52. The workload 'drives leadership events' and manages the DCA Service + EndpointSlice itself. Concluded: the workload owns the fault-active flag by driving leadership transitions/partitions itself rather than inferring churn from the Lease object. RESOLVED (harness design per topology).

#### Q1: Service selects all pods or only leader (deploy-side)

Examined command.go:648-717 — serving is gated only on admission_controller.enabled with no le.IsLeader check on server.Run (:712), so every replica serves. The Service selector itself is harness-managed (topology:52) and not pinned. KEPT needs-human (deploy/harness config).

#### Q3: Rotation-during-churn trust

Examined cert rotation config (common_settings.go:636-637, ~yearly) and leader-gating of cert/CABundle maintenance (controller_base.go:286,299). Rare edge; actual trust outcome not determinable from code alone. KEPT partial.

#### Q5: Default failure_policy rendered in harness

Code default 'Ignore' (common_settings.go:653); harness value not pinned. KEPT needs-human (deploy-side).

#### Q6: Witness require non-leader replica?

Test-design decision, no code answer. KEPT needs-human.


---
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
| **SUT-side instrumentation** | MISSING. `assert.AlwaysOrUnreachable(cert != nil || err != nil)` in GetCertificate; a `Reachable` on the error branch confirms the stale/failed-fetch path is exercised. |
| **Fault deps** | network partition between DCA and apiserver (freezes secret informer; enabled by default); requires admission_controller.enabled + leader |

**Open Questions:**

- Default admission_controller.failure_policy actually rendered by the Helm chart / Operator in the harness (code default is 'Ignore', common_settings.go:653; the shipped/harness-rendered value lives in external repos and is not pinned in antithesis/scratchbook/deployment-topology.md). Governs severity only. `(needs human input)`



#### Investigation Log

#### Q1/Q5: crypto/tls behavior when GetCertificate returns (nil,nil) with no static Certificates; does it fail the handshake or fall back?

Examined Go 1.26 src/crypto/tls/common.go getCertificate (:1317-1328). With GetCertificate returning (nil,nil), the guard `cert != nil || err != nil` (:1321) is false so it falls through; len(Certificates)==0 (admission server sets only GetCertificate, server.go:136-147) → returns errNoCertificates ("tls: no certificates configured", :1327). Concluded: hard handshake failure at the TLS layer, no panic, no empty/wrong cert, no fallback. RESOLVED (both Q1 and Q5).

#### Q3: Does GetCertificateFromLister return stale-but-valid vs strictly error when Secret missing?

Examined pkg/util/kubernetes/certificate/certificate.go:139-151. lister.Get reads the informer CACHE; if the Secret is present (even stale) it parses & returns it (stale-but-valid); it errors ONLY when the Secret is absent from the cache or ParseSecretData fails. Concluded: stale-but-valid masking is possible; strict error only on cache-miss/parse-fail. RESOLVED.

#### Q4: Frequency of cert rotation relative to leadership churn

Examined common_settings.go:636-637: certificate.validity_bound = 365*24h (1yr), expiration_threshold = 30*24h (refresh 1mo before expiry). Concluded: rotation is ~yearly; leadership churn is far more frequent, so the leader-gated resync gap during rotation is rarely hit. RESOLVED.


---
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
| **SUT-side instrumentation** | MISSING. `assert.Reachable` at the fatal return; ensure no path swallows the error into a degraded run. |
| **Fault deps** | network partition / latency (leader<->apiserver) during startup (enabled by default); node termination to exercise restart (DISABLED by default); requires autoscaling enabled |

**Open Questions:**

- None — all resolved during investigation (see evidence file Investigation Log)



#### Investigation Log

#### Q1: Does GetRFC1123CompliantClusterName retry/block, or return '' immediately on transient apiserver failure?

Examined pkg/util/kubernetes/clustername/clustername.go:67-149. getClusterName does NOT retry: each ProviderCatalog func is called once, errors are Debug-logged and skipped (:101-105), node-label lookup errors are logged (:126-128); the result is cached via data.initDone=true (:145) which is set regardless of success. Concluded: returns '' promptly on a transient blip (bounded only by each provider call's own timeout), and once '' is cached it stays '' until ResetClusterName. So command.go:439 fatal is easily reached under a brief startup partition. RESOLVED.

#### Q2: Is rcClient==nil reachable in typical autoscaling deployments?

Examined command.go:477-513 and initializeRemoteConfigClient (:846). rcClient stays nil only when (a) rcEnabled&&isSet is false (RC disabled or rcService fx component not wired, :480) or (b) initializeRemoteConfigClient errors (:503). rcclient.NewClient is a LOCAL construction (no network call), so a transient apiserver partition does NOT null it. Concluded: rcClient==nil is config/fx-gated (RC disabled or RC-service unavailable), not driven by a transient apiserver blip; in a correctly-configured autoscaling deployment RC is required+enabled so nil arises mainly from misconfiguration or RC-service init failure. RESOLVED (narrows the fault-reachability angle: the cluster-name path, not rcClient, is the fault-driven fatal).

#### Q3: Does fxutil.OneShot returning an error always yield a non-zero exit code?

Examined pkg/util/fxutil/oneshot.go:22-60 (returns err from delayedCall.call() at :54-57), command.go:168 (RunE returns fxutil.OneShot(...)), and cmd/cluster-agent/main.go:28-31 (Execute() error → os.Exit(-1)). Concluded: a returned error propagates to os.Exit(-1) (exit 255, non-zero) → CrashLoopBackOff. Not swallowed. RESOLVED.


---
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
