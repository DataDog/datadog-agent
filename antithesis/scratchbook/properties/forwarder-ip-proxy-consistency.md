# forwarder-ip-proxy-consistency — Leader forwarder's reported IP is consistent with proxy availability

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P1 · **Intent:** known-defect-reproducer

**Provenance:** merged from 2 discovery agent(s): leader-forwarder-ip-matches-proxy-availability, leader-forwarder-ip-proxy-consistency

## Property

The global leader forwarder never reports a non-empty leader IP while its proxy is nil (forwarding disabled), and never holds a live proxy while reporting an empty IP.


## Invariant / assertion

`assert.AlwaysOrUnreachable`: on every SetLeaderIP/Forward, (proxy==nil) iff (reported leaderIP==""). AlwaysOrUnreachable fits because the follower-forwarding path is optional (a single-replica or always-leader run never exercises it), but whenever it runs the consistency must hold.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: SetLeaderIP("") ran while a concurrent Forward/SetLeaderIP raced on the global forwarder.


## Antithesis angle

`SetLeaderIP("")` sets proxy=nil but RETURNS before clearing lf.leaderIP (leader_forwarder.go:117-121), so GetLeaderIP() misreports a stale IP while forwarding is off. Two writers race to SetLeaderIP: the clusterchecks 1s poll and the generic per-request check-then-act (leader_handler.go:128-131). Partition follower<->apiserver or churn leadership to drive GetLeaderIP()=="" and interleave the two writers.


## Why it matters

A follower that believes it can forward (non-empty IP) but has a nil proxy returns 503/mis-routes node-agent traffic; misreported control-plane state also corrupts status/telemetry an operator trusts during an incident.


## Mechanism refinement (from open-question investigation)

No invariant change. Confirms the bug is exploitable and unmasked: SetLeaderIP("") (leader_forwarder.go:117-119) returns before clearing lf.leaderIP, and the single stale-value consumer (leader_handler.go:128) uses it to skip re-arming the proxy, so proxy stays nil while GetLeaderIP() advertises a live IP. The simple fix (clear lf.leaderIP="" on the empty branch) makes the assertion hold.


## Fault dependencies

- network partition (follower<->apiserver, or leader churn producing GetLeaderIP()=='')
- concurrency (two SetLeaderIP writers)
- requires leader_election enabled + >=2 replicas


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.AlwaysOrUnreachable` inside SetLeaderIP after the mutation, and in Forward. Fixing the early-return bug is a prerequisite for the invariant to ever hold.


## Open questions (post-investigation)

- Reachability of the same-name/new-IP reuse step depends on the harness deployment kind (needs-human): standard helm deploys the DCA as a Deployment (random pod names) so a rescheduled leader gets a NEW HolderIdentity and GetLeaderIP re-resolves under a fresh cache key; a StatefulSet (stable names) is required to hit the stale-same-name path. `(partial)`


### Investigation Log

#### Q1: Is the forwarder's GetLeaderIP() read anywhere that would mask the bug by falling back to the engine's GetLeaderIP()?

Grepped all GetLeaderIP() call sites. Found the forwarder's getter (leader_forwarder.go:147) has exactly one consumer: leader_handler.go:128, which compares forwarder.GetLeaderIP() against the engine's freshly-fetched ip to decide whether to call SetLeaderIP. Concluded: no fallback to the engine value; the stale forwarder value is load-bearing for the routing decision, not masked. RESOLVED.

#### Q3: Enumerate all consumers of GetLeaderIP() to quantify blast radius (status vs routing).

Grep confirms the only reader of the forwarder's GetLeaderIP() is leader_handler.go:128 (routing check-then-act). No status/telemetry consumer reads it. Concluded: blast radius is confined to routing logic. RESOLVED.

#### Q4: Does the request-path writer ever call SetLeaderIP("") given the early return at leader_handler.go:108?

Examined leader_handler.go:108-131 and leaderelection.go:262-266. Found: SetLeaderIP is reached only when IsLeader()==false (past line 108). In a leaderless gap the follower's GetLeader() can be "" (or the ex-leader's identity), and engine GetLeaderIP() returns ("",nil) when leaderName=="", giving ip=="". Line 128 (forwarder.GetLeaderIP() != ip) is then true whenever the forwarder still holds a stale non-empty IP, firing SetLeaderIP(""). Concluded: yes, the per-request writer can call SetLeaderIP(""), driving proxy=nil while leaderIP stays stale (leader_forwarder.go:117-121). RESOLVED.


---

## Source discovery evidence (raw, per contributing agent)


### from `leader-forwarder-ip-matches-proxy-availability`

## Mechanism (verified, `leader_forwarder.go:112-135`)

```go
func (lf *LeaderForwarder) SetLeaderIP(leaderIP string) {
    lf.proxyLock.Lock()
    defer lf.proxyLock.Unlock()

    if leaderIP == "" {
        lf.proxy = nil
        return              // <-- returns BEFORE clearing lf.leaderIP
    }
    lf.leaderIP = leaderIP
    lf.proxy = &httputil.ReverseProxy{ ... }
}
```

`GetLeaderIP()` (a getter on the forwarder, returning `lf.leaderIP`) therefore returns the last non-empty IP after a `SetLeaderIP("")`, even though `Forward` will now 503 because `currentProxy == nil` (`leader_forwarder.go:102-107`).

## Two racing writers (verified)

- clusterchecks poll: `handler.go:250-252` — `if h.leaderForwarder != nil && newIP != h.leaderIP { h.leaderForwarder.SetLeaderIP(newIP) }`, every `leaderStatusFreq` (1s).
- generic handler: `leader_handler.go:128-131` — `if lph.leaderForwarder.GetLeaderIP() != ip { lph.leaderForwarder.SetLeaderIP(ip) }`, per request.

Both can call SetLeaderIP("") when GetLeaderIP() (engine) returns "" during a leaderless gap.

## Failure scenario

1. Follower is forwarding to leader IP 10.0.0.5 (proxy set, leaderIP=10.0.0.5).
2. Partition causes engine GetLeaderIP()==""; clusterchecks poll calls forwarder.SetLeaderIP("") → proxy=nil, leaderIP STILL 10.0.0.5.
3. New leader elected at 10.0.0.5 again (same pod name reused — StatefulSet, or same IP). Engine GetLeaderIP() returns 10.0.0.5.
4. Generic handler: `GetLeaderIP()==10.0.0.5 == ip` → condition false → SetLeaderIP NOT called → proxy stays nil → all forwarded node-agent requests 503 indefinitely. VIOLATION observed at step 2 (proxy nil, leaderIP!="").

## Where to assert (SUT instrumentation — MISSING)

- Add `assert.AlwaysOrUnreachable(!(lf.proxy==nil && lf.leaderIP!=""), "forwarder ip/proxy consistent", details)` at the end of `SetLeaderIP` (still holding proxyLock).
- Simplest real fix (out of scope for property, but informs the assertion): clear `lf.leaderIP = ""` in the empty branch before returning.


### from `leader-forwarder-ip-proxy-consistency`

## Claim
`SetLeaderIP("")` disables forwarding (`proxy = nil`) but does **not** clear the recorded `leaderIP`, so `GetLeaderIP()` returns a stale non-empty IP while forwarding is off - a representation-invariant violation on the global singleton, reachable via two racing writers.

## Code path (verified)
`pkg/clusteragent/api/leader_forwarder.go`
```go
func (lf *LeaderForwarder) SetLeaderIP(leaderIP string) {
    lf.proxyLock.Lock()
    defer lf.proxyLock.Unlock()
    if leaderIP == "" {
        lf.proxy = nil
        return            // :119  returns BEFORE clearing lf.leaderIP
    }
    lf.leaderIP = leaderIP // :121  only set on the non-empty path
    lf.proxy = &httputil.ReverseProxy{ ... }
}
func (lf *LeaderForwarder) GetLeaderIP() string {
    lf.proxyLock.RLock(); defer lf.proxyLock.RUnlock()
    return lf.leaderIP     // :150  can be stale-non-empty while proxy==nil
}
func (lf *LeaderForwarder) Forward(...) {
    ... if currentProxy == nil { http.Error(rw, "leader proxy is not available", 503); return } // :102-107
}
```
After any `SetLeaderIP("")`, the object is in state `proxy==nil && leaderIP!=""`: `Forward` 503s but `GetLeaderIP()` advertises a live IP.

## Two racing writers on the same global instance (verified)
- clusterchecks `updateLeaderIP` -> `h.leaderForwarder.SetLeaderIP(newIP)` (`handler.go:250-252`), driven by the 1s `leaderWatch` poll; `newIP` can be `""` during a leaderless gap.
- generic `LeaderProxyHandler.Forward` check-then-act: `if lph.leaderForwarder.GetLeaderIP() != ip { lph.leaderForwarder.SetLeaderIP(ip) }` (`leader_handler.go:128-130`), per request, on every follower.
Both mutate `globalLeaderForwarder` (single instance from `GetGlobalLeaderForwarder`, `leader_forwarder.go:81`). Internally serialized by `proxyLock` (no data race), but they are two independent notions of the target that interleave and can drive the object through the inconsistent state repeatedly.

## Suggested SUT instrumentation (MISSING - net new)
At the end of `SetLeaderIP` and inside `GetLeaderIP` (still under `proxyLock`), add `assert.Always((lf.proxy != nil) || (lf.leaderIP == ""), "forwarder proxy/leaderIP consistent", ...)`. The simple fix (clear `lf.leaderIP` on the empty path) would make the assertion hold; the assertion documents the contract.
