# forwarder-target-is-live-endpoint — Follower forwards only to a live DCA Service endpoint IP

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P1 · **Intent:** invariant

**Provenance:** merged from 1 discovery agent(s): forwarder-leaks-dca-token-to-unverified-ip

## Property

When a follower reverse-proxies a node-agent request to the leader, the destination IP is a current member of the Datadog Cluster Agent Service's endpoints — never a stale/reused IP from the 5-minute cache pointing at a dead or unrelated pod.


## Invariant / assertion

`assert.AlwaysOrUnreachable`: whenever Forward dials a target, that target IP is present in the current EndpointSlice/Endpoints set for the DCA service. AlwaysOrUnreachable fits — forwarding is optional, but any forward must target a live endpoint. Because the forwarder uses InsecureSkipVerify:true, TLS will not catch a wrong target, so the invariant must be checked explicitly.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: GetLeaderIP served a cached IP that differed from the current EndpointSlice set (stale-cache branch hit).


## Antithesis angle

GetLeaderIP caches leader pod-name→IP for 5 minutes (leaderelection.go:292) and 'will not return an error if the leader does not exist anymore'. Kill+reschedule the leader (same pod name, new IP — StatefulSet-style) or lag EndpointSlices; the follower forwards auth-bearing requests (carrying the DCA token) to a stale IP for up to 5 minutes. Requires node termination to reschedule the leader.


## Why it matters

Auth-bearing node-agent/HPA traffic sent to a wrong or dead IP → silent black-hole (502) or, if the IP is reused, delivery to an unrelated pod. Cluster checks and external metrics stall for up to the cache TTL with no error surfaced.


## Mechanism refinement (from open-question investigation)

Scope refinement (invariant unchanged): the stale-target hazard is reachable only when the leader's pod NAME persists while its IP changes (StatefulSet-style reuse), because GetLeaderIP caches by name for 5 min (leaderelection.go:268-292). Under the standard Deployment topology (random pod names) a reschedule yields a new HolderIdentity and a fresh cache key, so the invariant is largely vacuous there; the R1 witness/fault should assume StatefulSet-style naming to be non-vacuous.


## Fault dependencies

- node termination / pod restart with IP change (DISABLED by default — must be enabled)
- network partition / EndpointSlice propagation lag (enabled by default)
- requires leader_election enabled + >=2 replicas


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.AlwaysOrUnreachable` in Forward's Director comparing target IP against the current endpoint set. A cache-hit-vs-endpoint-mismatch `Reachable` marker anchors the stale-cache branch for replay.


## Open questions (post-investigation)

- Exploitability depends on harness deployment kind (needs-human): a StatefulSet (stable pod name, new IP) makes the stale-same-name/new-IP path reachable; the standard helm Deployment (random names) forces a fresh GetLeaderIP cache key on reschedule, largely closing it. `(partial)`
- Real-world likelihood that a reused pod IP belongs to a pod that actually reads/logs the bearer token (vs. simply RSTs) is a probabilistic operational judgment not answerable from code. `(needs human input)`


### Investigation Log

#### Q1: Does the ReverseProxy strip Authorization?

Examined the Director (leader_forwarder.go:123-135) and validateToken middleware (server.go:174-196). Found: the Director sets only URL.Scheme/Host, adds forwardHeader, and restores the path — it never touches Authorization; httputil.ReverseProxy removes only hop-by-hop headers; validateToken reads but never Del's Authorization and passes the request through unchanged. Concluded: Authorization is preserved verbatim to the target. RESOLVED (confirms the token-leak hazard).

#### Q4: Is SetLeaderIP called with a freshly-resolved IP quickly enough to shrink the window below the 5-min cache?

Examined leaderelection.go:262-293 and leader_handler.go:112-131. Found: the IP fed to SetLeaderIP comes from engine GetLeaderIP(), which caches per leader NAME (cacheKey "ip://"+leaderName) for 5 minutes (line 292). While the leader name is unchanged, every SetLeaderIP call re-reads the same cached IP regardless of cadence; when the name changes a fresh key resolves immediately. Concluded: SetLeaderIP frequency cannot shrink the stale window below the cache TTL — the window is bounded by leader-NAME stability, not writer cadence. RESOLVED.


---

## Source discovery evidence (raw, per contributing agent)


### from `forwarder-leaks-dca-token-to-unverified-ip`

## Claim
A follower's leader-forwarder can send node-agent requests (bearing the shared DCA auth token) to an IP that is no longer a Cluster Agent, with TLS verification disabled, leaking the credential.

## Mechanism (verified in source)

**TLS verification disabled on the forward transport:**
```go
// pkg/clusteragent/api/leader_forwarder.go:58
TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
```

**Authorization header is forwarded verbatim.** The Director (leader_forwarder.go:122-135) rewrites scheme/host/path and adds `X-DCA-Follower-Forwarded`, but does not remove the inbound `Authorization` header, so the node-agent's DCA bearer token is proxied to the target.

**Target IP comes from a divergent, cached source.** `SetLeaderIP` (leader_forwarder.go:113-144) builds the proxy pointed at `net.JoinHostPort(leaderIP, apiPort)`. `leaderIP` is fed from `GetLeaderIP` which resolves the leader pod name → IP via Service Endpoints/EndpointSlices and **caches for 5 minutes** (leaderelection.go:262-325, per SUT §4). This is a different notion of leadership than the Lease and can lag reality.

**Stale-IP-on-clear bug compounds it:** `SetLeaderIP("")` nils the proxy but returns before clearing `lf.leaderIP` (leader_forwarder.go:117-121), so `GetLeaderIP()` keeps reporting the stale IP. (In this specific case `proxy==nil` so Forward returns 503 rather than forwarding, but the more dangerous case is when `proxy` is set to a now-stale IP.)

## Failure scenario
1. Follower resolves leader IP 10.0.0.7 and caches it (5-min TTL); SetLeaderIP("10.0.0.7") builds the proxy.
2. Antithesis terminates the leader pod (node termination). Kubernetes reschedules it; the EndpointSlice lags or the follower's endpoint informer is frozen by an asymmetric partition (informer_client_timeout=0 → no client-side watch timeout, SUT §7).
3. Kubernetes assigns 10.0.0.7 to an unrelated workload's pod (IP reuse).
4. A node agent POSTs a heartbeat / GETs configs to the follower, which forwards it to https://10.0.0.7:5005 with InsecureSkipVerify:true.
5. The unrelated pod terminates TLS (self-signed, accepted) and receives the request including `Authorization: Bearer <DCA token>` → cluster-wide credential leaked.

## Key observations
- Forwarding the token to a *legitimate* other DCA replica is benign (replicas share the token); the hazard is specifically forwarding to a NON-DCA IP, which is why the checkable invariant is "target ∈ current DCA EndpointSlice".
- Loop protection (single `X-DCA-Follower-Forwarded` header → 508) does not help here; it only prevents multi-hop, not wrong-target.
- Two writers race SetLeaderIP (the clusterchecks 1s poll and the per-request check-then-act in leader_handler.go), widening the window where leaderIP is stale/inconsistent (SUT §4).

## Timing window
Up to the 5-minute GetLeaderIP cache TTL after a leader reschedule, and potentially unbounded while the endpoint informer is frozen by a partition (no client-side watch timeout).
