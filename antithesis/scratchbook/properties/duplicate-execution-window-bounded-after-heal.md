# duplicate-execution-window-bounded-after-heal — Duplicate-execution window is bounded after a partition heals

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P1 · **Intent:** should-improve

**Provenance:** merged from 1 discovery agent(s): duplicate-execution-window-bounded-after-heal

## Property

Once a previously-expired/partitioned node N re-establishes contact with the leader (partition heals, N resumes heartbeating), N is told to drop any config D that was reassigned away from it within a bounded number of successful poll cycles — so duplicate execution of D on N and M does not persist indefinitely after connectivity is restored. The constructive form of the guarantee: the DCA lacks fencing, but the pull loop should still converge N to the current assignment promptly after heal.


## Invariant / assertion

assert.AlwaysOrUnreachable: for a healed node N whose config D was reassigned to M, within K successful (heartbeat=IsUpToDate-false)+(config-poll) cycles after N resumes contact, N's returned config set omits D (N drops it), collapsing the executing-set of D back to size 1. AlwaysOrUnreachable fits — this path only runs when a reassignment-then-heal actually occurs (optional), but whenever it does, convergence must be bounded. Hazards that could make it FAIL: warmup returns IsUpToDate=true to ALL nodes (dispatcher_nodes.go:73-79), and a coincidental/stale lastConfigChange equality (dispatcher_nodes.go:69) also returns IsUpToDate=true — either can keep N running its stale cached set (including D) across the whole warmup or indefinitely, extending the duplicate window past heal.


## Antithesis angle

On heal, N POSTs status; processNodeStatus auto-recreates a fresh node store for N (getOrCreateNodeStore) and, outside warmup with a differing lastConfigChange, returns IsUpToDate=false, so N re-pulls and gets its new (D-free) set — window closes. BUT: (1) if the new leader is within warmup_duration (30s) when N reconnects, processNodeStatus returns true to N regardless (dispatcher_nodes.go:73-79) and N keeps running cached D; (2) reset() wipes lastConfigChange, so after a leadership flap a fresh counter can coincide with N's cached value (equality check at :69) and N is told up-to-date while actually stale. Antithesis flaps leadership / times the heal to land inside warmup and holds the duplicate past the expected close.


## Why it matters

The strict no-fencing invariant (property 1) may be an accepted transient IF the window closes fast after heal. This property tests exactly that mitigation. If warmup or the equality-based IsUpToDate keeps a reconnected node running a reassigned check, the duplicate is no longer a brief failover blip but a sustained double-count lasting a full warmup (30s+) or, under the epoch coincidence, until the next real config change — a materially worse and operator-invisible outcome.


## Mechanism refinement (from open-question investigation)

Narrow the assertion: the stale/coincidental lastConfigChange-equality hazard (hazard A) is effectively dead — reset() and node re-creation always start lastConfigChange at 0 while a real node posts a nonzero LastChange, so the equality branch cannot falsely mark a stale node up-to-date. Only the warmup-blanket case (hazard B, dispatcher_nodes.go:73-79) can extend the duplicate window past heal (up to warmup_duration=30s). Drop hazard A from the property; keep the warmup-extended window as the remaining should-improve concern.


## Fault dependencies

- network partition N<->leader > node_expiration_timeout then HEAL (leadership stable) — ENABLED by default; or workload-driven heartbeat stop then resume
- to reach the warmup-extended and stale-equality cases: flap leadership (partition leader<->apiserver) so a fresh leader is in warmup / has a reset lastConfigChange when N reconnects — ENABLED by default
- requires leader_election enabled + >=2 replicas + >=2 simulated node identities


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

Workload-side: track per-identity executing-set and assert D leaves N's set within K cycles after N resumes contact. High-value SUT-side addition: emit the IsUpToDate decision (which branch: equality/warmup/re-pull) from processNodeStatus so the trace attributes a sustained window to warmup vs stale-equality. Same SDK/custom-image prerequisite as the other properties.


## Open questions (post-investigation)

- Node-agent re-pull speed after IsUpToDate=false is out of SUT scope; the workload must model a prompt re-pull for the K-cycle bound to be meaningful. DCA side returns false promptly (verified); node-agent poll cadence is not in this package. `(partial)`


### Investigation Log

#### Q1: Does reset() zero nodeStore.lastConfigChange? If not, the stale-equality sustained-duplicate case is live.

Examined dispatcher.reset() (dispatcher_main.go:293-304) -> store.reset() (stores.go:42-55) which does s.nodes = make(map[string]*nodeStore), dropping every nodeStore; and getOrCreateNodeStore/newNodeStore (stores.go:86-137) which creates a fresh nodeStore whose lastConfigChange is the zero value 0 (never set in newNodeStore). The equality check (dispatcher_nodes.go:69) is node.lastConfigChange==status.LastChange. Found: after reset (or after expireNodes deletes the node), a reconnecting N always meets a fresh store with lastConfigChange=0; a node that has actually pulled configs posts a NONZERO status.LastChange (timestampNowNano set in addConfig/removeConfig, stores.go:140,151), so 0 != nonzero -> returns false -> N re-pulls. Equality can only falsely fire when status.LastChange==0 (a node that never pulled). Conclusion: RESOLVED — reset() effectively zeroes lastConfigChange (drops the nodeStore), so the stale/coincidental-equality sustained-duplicate scenario is NOT live for any node that had a real config; only the warmup case remains.

#### Q2: K (poll cycles to converge) — one interval, or exclude warmup?

Examined processNodeStatus (dispatcher_nodes.go:44-84) and getClusterCheckConfigs (28-40). Found: outside warmup, one status POST returns IsUpToDate=false and the next config poll returns N's current (D-free) dispatched set — convergence in a single poll cycle. Warmup blanket-returns true for up to warmup_duration (30s) and is documented intended-degraded behavior. Conclusion: RESOLVED — K = 1 poll interval outside warmup; warmup (30s) should be excluded from / added to the bound as intended degradation.

#### Q3: Node-agent re-pull speed after IsUpToDate=false.

Examined: DCA returns false promptly and immediately serves the D-free set on the next /configs pull. Found: how fast N issues that pull is governed by node-agent poll cadence, which lives outside the cluster-agent package (out of SUT). Conclusion: PARTIAL — DCA-side convergence is prompt and verified; the workload must model a prompt node re-pull for the K bound to be observable.


---

## Source discovery evidence (raw, per contributing agent)


### from `duplicate-execution-window-bounded-after-heal`

## Convergence path (source)

On heal N POSTs `/status/{id}`; `processNodeStatus` (dispatcher_nodes.go:44-84):
```go
node := d.store.getOrCreateNodeStore(nodeName, clientIP) // N re-registered fresh
...
if node.lastConfigChange == status.LastChange { return true }   // (A) equality -> up-to-date
if warmingUp { return true }                                    // (B) warmup -> up-to-date
... return false                                                // else: N must re-pull
```
When it returns false, N pulls `getClusterCheckConfigs` (dispatcher_nodes.go:28-40) and receives only what is now dispatched to N — D having moved to M, N's set omits D, so N drops it and the window closes. Bounded, good.

## Two ways the window fails to close promptly

**(B) Warmup blanket up-to-date** — dispatcher_nodes.go:73-79 explicitly returns `true` to *all* nodes during warmup so caches keep running. If N reconnects while a freshly-elected leader is in its 30s warmup, N is told up-to-date and keeps running cached D for the remainder of warmup, extending the duplicate. sut-analysis §5 documents warmup=30s.

**(A) Stale/coincidental lastConfigChange equality** — the check at :69 is `==`, and `reset()` wipes lastConfigChange on every leadership loss (sut-analysis §6, §11 open question: does reset() zero it?). After a flap, a rebuilt counter can equal N's cached value by coincidence -> N told up-to-date while genuinely stale -> keeps D. No leader-generation/epoch is attached (see lastconfigchange-monotonic-epoch, P2). This is the sustained-duplicate case.

## Relationship to catalog

Constructive complement to no-duplicate-execution-without-drop-notice: property 1 shows the window opens with no fencing; this shows whether the pull loop closes it promptly after heal. Distinct from lastconfigchange-monotonic-epoch, which asserts counter monotonicity per se; here the observable is the cross-node duplicate persisting past reconnection.

## Intent

`should-improve`: prompt convergence after heal is the desired behavior; warmup-blanket and equality-IsUpToDate are read as able to defeat it, so under the right timing this is expected to expose a sustained window. Framed AlwaysOrUnreachable because the reassignment-then-heal path is optional but must converge whenever taken.
