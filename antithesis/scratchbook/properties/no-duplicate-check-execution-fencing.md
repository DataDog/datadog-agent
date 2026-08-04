# no-duplicate-check-execution-fencing — No cluster check runs on two nodes without the first being told to drop it

**Type:** Safety · **Assertion:** `Always` · **Priority:** P0 · **Intent:** known-defect-reproducer

**Provenance:** merged from 2 discovery agent(s): no-duplicate-execution-without-drop-notice, duplicate-execution-window-reached

## Property

The dispatcher never causes a single cluster-check config digest to be executed simultaneously by two distinct live node identities. Concretely: before a config that was handed to node N is (re)dispatched to a different live node M, node N must first have been told to drop it (via a config-poll response for N that omits the digest). Node expiration by heartbeat timeout does NOT satisfy 'told to drop': a partitioned-but-alive N keeps running its cached checks per the cached-check contract, so reassigning to M opens a window where the same check runs on both.


## Invariant / assertion

assert.Always, workload-witnessed: for every cluster-check digest D, the set of live simulated node identities currently *executing* D (i.e. D is in the config set the node last successfully pulled and the node has not since pulled a set omitting D) has size <= 1. Equivalently, whenever the leader moves D from N to M (expireNodes -> danglingConfigs -> reschedule), N has already received a poll response omitting D. Always is the correct shape: the 'exactly one node' guarantee (README, sut-analysis §10) must hold at every instant. This is expected to go RED under a default partition, which is the deliverable.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: There exists a run state in which a single cluster-check config digest D is simultaneously in the executing-set of two distinct live simulated node identities: the original holder N (partitioned/heartbeat-stopped but still running its cached configs) and the reassignment target M (to which the leader moved D after expiring N). This is the paired witness that the hazardous precondition behind no-duplicate-execution-without-drop-notice was genuinely opened, not merely never exercised.


## Antithesis angle

expireNodes() (dispatcher_nodes.go:142-186) removes a node purely on `node.heartbeat < timestampNow() - nodeExpirationSeconds` — a timeout, never a confirmation of death or a revocation to the node. Its configs are moved to danglingConfigs and re-dispatched to a live node by the 15s cleanup ticker (dispatcher_main.go:400-411 -> reschedule -> addConfig, dispatcher_configs.go:146-165). The pull model has NO push/revocation channel to N and NO fencing token: nothing tells N it was de-assigned, and nothing tells M that N may still be running the check. The workload stops heartbeating for identity N (or a partition drops N<->leader) for > node_expiration_timeout (30s) while N keeps 'running' its last-pulled configs; the leader expires N and hands D to M; both now run D. Antithesis explores the interleaving of the expiry tick, reschedule, and N's (absent) poll to open and hold the window.


## Why it matters

This is the reality-side shadow of the load-bearing 'each cluster check dispatched to exactly one node' guarantee. dispatch-store-bijection is CORRECT by design during this window (after reassignment digestToNode[D]=M only, so the store is a perfect bijection) and therefore structurally cannot catch it — the duplicate is invisible to every store-internal assertion. Only a workload that models node agents keeping cached checks alive while partitioned can witness the double execution, which produces duplicate/double-counted metrics for D cluster-wide with no error and no alert.


## Mechanism refinement (from open-question investigation)

No invalidation. Scope tightened by resolutions: (a) restrict the assertion to load-balanced cluster checks (endpoints are carved out — separate store, not rescheduled on expiry); (b) the reachability precondition requires stable leadership > ~30s warmup + one 15s cleanup tick before reassignment can occur; (c) any single surviving live node suffices as target M (no dedicated spare identity needed).


## Fault dependencies

- network partition workload-node-identity(N) <-> dca-leader for > node_expiration_timeout (default 30s) — ENABLED by default
- WORKLOAD SUBSTITUTE (no fault needed): the workload simply stops POSTing heartbeat status for identity N while continuing to 'run' N's last-pulled config set, and keeps another identity M heartbeating — this deterministically drives expiry+reassignment with zero fault injection
- NO node termination required; NO clock skew required (backward skew would only amplify by mass-expiring)
- requires leader_election enabled + a leader whose dispatcher.run is active (post-warmup) + >=2 simulated node identities


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

Primary assertion is workload-side over the workload's model of per-node-identity executing-config sets. Optional (high-value for trace correlation): in the instrumented DCA image, emit an Antithesis details event or assert.Reachable at the point in expireNodes()/reschedule() where digest D transitions from an expired node N to a new target M, tagged with D, N, M, and N's last heartbeat age. Requires adding github.com/antithesishq/antithesis-sdk-go to the root module (zero existing instrumentation, per existing-assertions.md) and building a custom DCA image; the //go:build clusterchecks tag must be present.


## Open questions (post-investigation)

- Node-agent side (out of SUT scope): the guarantee that a partitioned-but-alive node keeps running its last-pulled cached checks must be modeled by the workload. DCA-side evidence supports the assumption (see log) but the node-agent cache/pull code is not in this repo's cluster-agent package. `(partial)`

**Resolved (user decision, 2026-07-21):** transient failover duplication is a **real defect**, not an accepted tradeoff — the team should add real fencing (e.g. a generation/epoch token per dispatch) rather than treat the window as tolerable. This confirms `known-defect-reproducer` as the correct intent (not a downgrade to `should-improve`). The deliverable stays a minimized reproducing trace of a single digest executed by two live node identities simultaneously, escalated as a finding that argues for adding a fencing mechanism to the dispatch protocol (e.g. an ownership token N must present, or a generation counter bumped on reassignment that N's next poll response must reflect).


### Investigation Log

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

## Source discovery evidence (raw, per contributing agent)


### from `no-duplicate-execution-without-drop-notice`

## Mechanism (primary source, commit f2da1471bb)

**Expiration is a timeout, not death, and carries no revocation.** `dispatcher_nodes.go:142-186` `expireNodes()`:
```go
cutoffTimestamp := timestampNow() - d.nodeExpirationSeconds
...
if node.heartbeat < cutoffTimestamp {
    for digest, config := range node.digestToConfig {
        delete(d.store.digestToNode, digest)
        d.store.danglingConfigs[digest] = createDanglingConfig(config)  // -> re-dispatched
    }
    delete(d.store.nodes, name)
}
```
The decision is time-based only. There is **no message sent to the expired node**; the node-agent side is pull-only (`GET /api/v1/clusterchecks/configs/{id}`), so a partitioned N cannot be reached to drop the config.

**Re-dispatch to another live node** — `dispatcher_main.go:400-411`:
```go
case <-cleanupTicker.C:          // every node_expiration_timeout/2 = 15s
    d.expireNodes()
    if d.shouldDispatchDangling() { // requires >=1 live node
        danglingConfigs := d.retrieveDangling()
        scheduledConfigIDs := d.reschedule(danglingConfigs) // -> addConfig -> targetNode M
        ...
    }
```
`addConfig` (`dispatcher_configs.go:146-165`) sets `digestToNode[digest]=M` and adds it to M's store. The store is now a clean bijection D->M.

**Cached-check contract keeps N running.** `dispatcher_nodes.go:73-79` (warmup) and the pull model mean a node runs whatever it last pulled until it successfully pulls a set that omits D. The README/sut-analysis §5,§10 state nodes keep running cached checks while the leader is unavailable. So between expiry of N and N's next successful omitting-poll, **N and M both run D**.

**No fencing token.** There is no generation/epoch on a dispatch that N could present and have rejected, and no lease/ownership token per config. `lastConfigChange` (dispatcher_nodes.go:69) is a per-node counter used only for an equality IsUpToDate check, wiped by reset() — it is not an ownership fence.

## Why this is a genuine gap in the catalog

`dispatch-store-bijection` (P0) asserts store-internal structure: each digest maps to exactly one existing node, never two. During the duplicate window that invariant **holds** (D->M only; N was deleted from the store). The hazard lives *between* 'store consistent' and 'reality consistent' and is only observable by a workload that tracks what each simulated node is actually executing. No existing catalog property covers it.

## Intent

`known-defect-reproducer`: the code is already read to permit a reassignment while the prior holder is alive and un-notified (expiration != death, no fencing). The deliverable is a reproducing trace of a single digest executed by two live node identities simultaneously, plus a measurement of the window duration. Whether Datadog treats transient failover duplication as an accepted tradeoff vs. a bug is a product decision; the reproducer makes the window concrete either way.

## Assertion placement

Workload-side (no SUT instrumentation strictly required): the workload owns every simulated node identity's 'currently executing' set (it pulls configs on their behalf and decides when a partitioned identity stops pulling but keeps its cached set). The workload evaluates the Always over that model. An optional SUT-side `assert` / details emission at the tail of `expireNodes`/`reschedule` (digest D moved from expired N to M) lets the trace correlate the reassignment instant with the workload's double-run detection.


### from `duplicate-execution-window-reached`

## What must be scheduled

From `expireNodes()` (dispatcher_nodes.go:142-186) + the cleanup loop (dispatcher_main.go:400-411), the window opens iff all of the following co-occur:

1. N previously pulled config D (`getClusterCheckConfigs`, dispatcher_nodes.go:28-40) and is still running it.
2. N's heartbeat ages past `node_expiration_timeout` (30s) — workload stops POSTing `/status/{id}` for N, or a partition drops it.
3. `shouldDispatchDangling()` is satisfied — **at least one other live node M exists** to receive D (dispatcher_main.go:405; with zero nodes, dangling is NOT drained, so M must be present).
4. M pulls and begins running D before N ever pulls an omitting set.

## Reachability argument

Each condition is workload-controllable: the workload owns heartbeat cadence for every identity and the AD config that seeds D. No fault injection is strictly needed — silencing N's heartbeat is sufficient. The only timing Antithesis must find is ordering the expiry+reschedule between N's last pull and N's next (absent) pull, which is the default concurrency exploration.

## Pairing

This is the mandatory witness for `no-duplicate-execution-without-drop-notice`. Report both: the Reachable proves the window opened; the Always/AlwaysOrUnreachable measures whether it was ever illegally held. A run where this Reachable stays un-fired means the safety result is untrustworthy (window never scheduled).
