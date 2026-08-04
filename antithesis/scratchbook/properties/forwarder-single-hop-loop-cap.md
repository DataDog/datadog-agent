# forwarder-single-hop-loop-cap — Follower forwarding is capped at a single hop

**Type:** Safety · **Assertion:** `Always` · **Priority:** P1 · **Intent:** invariant

**Provenance:** merged from 1 discovery agent(s): forwarder-single-hop-loop-cap

## Property

A request already carrying the X-DCA-Follower-Forwarded header is never forwarded again; it is answered with 508 Loop Detected, bounding any follower→leader proxy chain to one hop.


## Invariant / assertion

`assert.Always`: in LeaderForwarder.Forward, if the incoming request has X-DCA-Follower-Forwarded set, the outcome is a 508 and no outbound proxy call is made. Always fits — the anti-loop guard must hold on every forwarded request.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: a request bearing X-DCA-Follower-Forwarded actually arrived (the 508 guard was exercised, not skipped).


## Antithesis angle

During a leadership flip two replicas can each believe the other is leader (transient), so A forwards to B and B could forward back to A. The single header is the only loop bound (leader_forwarder.go:90-95). Partition/churn leadership so multiple replicas are simultaneously in follower state and hammer a forwarded endpoint.


## Why it matters

An unbounded forward loop under a leadership flip would amplify into a request storm across replicas, saturating the connection pool (MaxConnsPerHost) and taking down the DCA API for node agents. The guard caps blast radius to one hop — but if it regresses, the failure is cluster-wide.


## Fault dependencies

- network partition (asymmetric) to create mutual-follower state; enabled by default
- node termination/rescheduling to churn leadership (DISABLED by default)
- requires leader_election enabled + >=2 replicas


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.Always(is508, ...)` on the already-forwarded branch in Forward; a `Reachable` marker confirms the loop-detection branch is actually exercised under fault.


## Open questions (post-investigation)

- Magnitude of the mutual-follower leaderless window depends on client-go lease timing and is measured under fault, not derivable statically. `(partial)`


### Investigation Log

#### Q1: Can the ReverseProxy ever drop/rewrite X-DCA-Follower-Forwarded on retry?

Examined leader_forwarder.go:86-144. Found: the 508 guard (line 90-95) runs before any proxy dispatch, so a request already carrying the header never reaches ServeHTTP. On the outbound leg the Director uses req.Header.Add(forwardHeader,"true") (line 126), and httputil.ReverseProxy has NO built-in retry and strips only hop-by-hop headers (this custom header is not hop-by-hop). Concluded: the header cannot be dropped/rewritten by a retry and reliably reaches the next hop; the single-hop cap holds by construction. RESOLVED.


---

## Source discovery evidence (raw, per contributing agent)


### from `forwarder-single-hop-loop-cap`

## Property: forwarding is capped at one hop

### Code path
- `pkg/clusteragent/api/leader_forwarder.go:27` — `forwardHeader = "X-DCA-Follower-Forwarded"`.
- `leader_forwarder.go:86-95` — `Forward`: sets `X-DCA-Forwarded: true` reply header, then `if req.Header.Get(forwardHeader) != "" { SetSpanError; http.Error(rw, ..., http.StatusLoopDetected /*508*/); return }`.
- `leader_forwarder.go:97-109` — only past the guard does it read `currentProxy` and call `currentProxy.ServeHTTP`.
- `leader_forwarder.go:122-135` — the ReverseProxy Director *adds* `forwardHeader:true` on the outbound leg (`req.Header.Add(forwardHeader, "true")`), so the next hop sees it.

### Two forwarding entry points share the guard
- Generic: `pkg/clusteragent/api/leader_handler.go:92-135` `rejectOrForwardLeaderQuery` → `lph.leaderForwarder.Forward`.
- Clusterchecks: `pkg/clusteragent/clusterchecks/handler_api.go:22-43` `RejectOrForwardLeaderQuery` (state==follower) → `h.leaderForwarder.Forward`.
Both funnel into the same `LeaderForwarder.Forward`, so the single-hop guard covers both.

### Failure scenario (must NOT loop)
1. Partition/kill leader → leaderless gap. Replica A (follower) resolves leader IP → replica B (via 5-min-stale EndpointSlice / cache).
2. Node agent polls A. A: not leader → GetLeaderIP→B → SetLeaderIP(B) → Forward: header empty → proxies to B, Director adds `X-DCA-Follower-Forwarded:true`.
3. B is also now a follower (OnStoppedLeading set state). B.Forward: header present → **must** return 508, must NOT proxy to A/B again.
4. Violation would be: B proxies onward → cycle A→B→A→... amplification.

### Assertion
- `assert.Always(req.Header.Get(forwardHeader) == "" , "proxying a request that was already forwarded", ...)` placed immediately before `currentProxy.ServeHTTP` (leader_forwarder.go:109).
- `assert.Reachable("forwarder returned 508 loop-detected", ...)` at leader_forwarder.go:93 to confirm the split-brain path is actually exercised.

### Notes
- Loop protection is a single boolean header, so it only survives one hop by construction — correct as long as the header is never stripped and there is only one forwarder instance per process. The assertion detects any regression (e.g., header renamed, Director overwriting instead of Add).
