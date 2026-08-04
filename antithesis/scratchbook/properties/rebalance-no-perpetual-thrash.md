# rebalance-no-perpetual-thrash — Rebalance does not move a check between runners every cycle

**Type:** Safety · **Assertion:** `Always` · **Priority:** P1 · **Intent:** should-improve

**Provenance:** merged from 1 discovery agent(s): rebalance-no-perpetual-thrash

## Property

Across successive rebalance cycles, a single cluster-check config digest is not repeatedly relocated between the same two runners (A->B->A->B ...) when the busyness/utilization inputs driving the decision are stale or zero because a CLC runner is unreachable. Rebalance must converge to a stable assignment rather than thrash indefinitely.


## Invariant / assertion

assert.Always(no digest is relocated on more than K consecutive rebalance cycles): maintain, per digest, a bounded history of (cycle_index, assignedNode) at the tail of each rebalance; assert that no digest changes node on K consecutive cycles (K a small constant, e.g. 3) while the set of node busyness values it depends on is unchanged from the prior cycle (stale). Paired WITNESS (Reachable/Sometimes): a rebalance actually moved a config to or from a runner whose stats were stale due to an unreachable CLC runner — otherwise the Always is vacuous.


## Antithesis angle

When a CLC runner is unreachable, updateRunnersStats (dispatcher_nodes.go:219-224) does `continue` on GetRunnerStats failure, so the node keeps STALE clcRunnerStats (never zeroed); for the utilization path GetRunnerWorkers failure substitutes DefaultNumWorkers (dispatcher_nodes.go:212-213). A never-reached runner (registered by heartbeat, stats never fetched) shows busyness 0 / utilization ~0, so it looks perpetually 'least busy'. The two anti-thrash guards are the ONLY defense: the busyness path's tolerationMargin=0.9 hysteresis (dispatcher_rebalance.go:300) and the utilization path's rebalanceIsWorthIt(minPercImprovement) + stickiness bias (checks_distribution.go:93-94). moveConfig moves the stats SNAPSHOT to the destination store, so the picture on the next cycle depends on whether the (possibly unreachable/lagging) runner overwrites it in updateRunnersStats — a fault-timing-dependent feedback loop. Antithesis can hold one runner partitioned across several rebalance ticks and observe whether a config oscillates. A unit test calls rebalance once with stubbed stats and cannot reproduce the cross-cycle stale-state loop.


## Why it matters

Each relocation is a real schedule/unschedule on node agents: the check stops on the old runner and restarts on the new one, dropping in-flight data and re-paying warmup. A config that ping-pongs every 10-min cycle produces perpetual gaps and churn for that check cluster-wide, plus inflated rebalancing_decisions/successful_rebalancing telemetry that masks the pathology. Because the guards are heuristic (a tentative 0.9 margin, a percentage threshold), there is no proof they prevent oscillation when the inputs feeding them are stale/wrong — this is a should-improve hypothesis Antithesis is uniquely able to falsify.


## Mechanism refinement (from open-question investigation)

No invalidation. Confirmed both anti-thrash guards are active by default (stickiness_enabled=true common_settings.go:589; busyness tolerationMargin=0.9). Refinement: the utilization path is largely self-protecting against move-back because a just-moved-but-not-yet-reporting config is excluded from currentDistribution (dispatcher_rebalance.go:459-470), so the residual oscillation risk is concentrated in the unreachable-runner-stale-snapshot branch (dispatcher_nodes.go:219-224).


## Fault dependencies

- network latency/partition dca-leader -> one clc-runner sustained across >=K rebalance cycles, so its stats stay stale/zero (ENABLED by default)
- requires leader_election enabled + advanced_dispatching + >=2 CLC runners so there is a source and a stale target
- lower rebalance_period in the harness so multiple cycles fit a timeline; node termination and clock skew NOT required
- the safety assertion is INERT unless the paired witness confirms a move touched a stale/unreachable-runner distribution — instrument both


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

Instrument at the tail of dispatcher.rebalance(). (1) Maintain per-digest a small ring buffer of (cycleIndex, node) plus a snapshot hash of the busyness/utilization inputs used that cycle. assert.Always(!(digest changed node on the last K cycles AND the input-hash was unchanged across those cycles), 'rebalance did not thrash a config under stale inputs', details{digest, recentNodes, inputHash}). (2) PAIRED WITNESS so the Always is not vacuous: when a rebalance move's source or destination node had statsCollectionFails incremented this cycle (unreachable -> stale), call assert.Reachable(true, 'rebalance moved a config involving a stale/unreachable-runner distribution', details{digest, staleNode}) — and an assert.Sometimes(rebalance_returned_zero_moves_with_a_stale_runner_present) as a convergence witness (the healthy outcome: after a few cycles the distribution stabilizes and a cycle produces no moves despite the persistent fault). Workload: register >=2 CLC-runner stubs with controllable stats, seed configs, partition one runner for several rebalance ticks, and count per-digest schedule/unschedule churn observed on the node-agent HTTP side as an external cross-check of the internal move counter.


## Open questions (post-investigation)

- Right K for the consecutive-move bound (assertion gated on 'busyness inputs unchanged from prior cycle'): too small flags legitimate rebalancing under genuinely changing load; a tuning/intended-behavior judgment not decidable from code. `(needs human input)`


### Investigation Log

#### Does moveConfig writing the stats snapshot make an unreachable target look busy, or does the utilization path re-empty it?

Examined moveConfig:199-217 (copies moved instances' stats into destNode via AddRunnerStats :210) and updateRunnersStats dispatcher_nodes.go:219-224 (GetRunnerStats error -> statsCollectionFails.Inc + continue, snapshot NOT overwritten). Conclusion: if dest is UNREACHABLE the moved snapshot persists -> dest 'looks busy' (self-limiting); if dest is REACHABLE its clcRunnerStats is overwritten by the fresh report and a not-yet-started check disappears (re-emptied). Both branches exist; which fires is reachability/timing-dependent. RESOLVED.

#### When a config moves to a runner that has not started it, stats are absent next cycle — does digestToNode staying put prevent move-back?

Examined currentDistribution:445-500. Found: it keys off clcRunnerStats + idToDigest (:459-470), NOT digestToNode. A just-moved config whose stats are absent is excluded from the distribution entirely -> not proposed for any move that cycle -> cannot be moved back while absent. digestToNode (set moveConfig:220) is not consulted by the utilization algorithm; the absence itself prevents move-back. Once stats reappear on the new runner, addToLeastBusy places it with preferredRunner=new node plus stickiness bias. RESOLVED.

#### Right K for the consecutive-move bound.

Conclusion: choice of K (and the stale-input gate) is a false-positive/tuning tradeoff not derivable from code. NEEDS-HUMAN.

#### Is stickiness_enabled on by default?

Examined common_settings.go:589-592 and checks_distribution.go:92-96. Found: stickiness_enabled default true; stickiness_factor 4, upper_limit 1, lower_limit 0.05. leastBusyRunner subtracts a bias from the preferredRunner's utilization when stickiness enabled. Conclusion: the utilization path's anti-thrash stickiness bias is ON by default. RESOLVED.


---

## Source discovery evidence (raw, per contributing agent)


### from `rebalance-no-perpetual-thrash`

## Mechanism (verified, commit f2da1471bb7)

### Stale/zero busyness from an unreachable runner
`updateRunnersStats` (`dispatcher_nodes.go:190-246`) is called at the top of BOTH rebalance algorithms (`:263`, `:380`). On an unreachable runner:
- `GetRunnerStats(ip)` errors -> `statsCollectionFails.Inc` + **`continue`** (`:219-224`): the node's `clcRunnerStats` is **not overwritten** — the previous snapshot persists (stale). It is never zeroed.
- utilization path: `GetRunnerWorkers(ip)` errors -> `node.workers = DefaultNumWorkers` (`:212-213`).

A runner registered via heartbeat but never successfully polled has empty `clcRunnerStats` (busyness 0) and, for utilization, DefaultNumWorkers -> utilization ~0 -> it is the perpetual 'least busy' target of `pickNode` (`:150-165`) / `addToLeastBusy` (`checks_distribution.go:113`).

### The only anti-thrash guards
- **Busyness:** move only if `destDiff+configWeight < int(float64(sourceDiff)*tolerationMargin)` with `tolerationMargin=0.9` (`dispatcher_rebalance.go:29-32, 300`). Comment: "the 0.9 value is tentative and could be changed" and "lean towards stability over perfectly optimal balance."
- **Utilization:** `rebalanceIsWorthIt` requires the proposed stddev to beat current by `rebalance_min_percentage_improvement` (`:543-554`), plus optional stickiness bias keeping a check on its `preferredRunner` (`checks_distribution.go:85-95`).

Neither guard has a proof against cross-cycle oscillation when the busyness/utilization inputs are themselves wrong (stale snapshot on an unreachable node, or a snapshot that disappears for one cycle because a reachable runner has not yet started a just-moved check and reports it absent). `moveConfig` writes the moved check's stats into the destination nodeStore (`:210, :226-228`), so the very next `updateRunnersStats` either overwrites them (reachable runner -> may drop them if the check has not started reporting) or keeps them (unreachable runner -> stale). Which branch happens is fault/timing-dependent — exactly Antithesis's reachable state space.

### Why not a unit test
`TestRebalanceUsingUtilization*` and `TestRebalance` call the algorithm once with fixed stubbed stats. The oscillation is a property of the *sequence* of cycles feeding each other through the shared store while a fault persists across ticks — not observable in a single synchronous call. No existing test runs multiple rebalance cycles against a persistent store with an injected unreachable runner.

## Intent: should-improve
No code comment or PR claims 'a config is moved at most K times' or 'rebalance converges'. The guards reduce but do not provably eliminate thrash under stale inputs. The deliverable is either a green Always (guards hold under the explored interleavings, raising confidence) or a reproducing oscillation trace motivating a hard convergence bound / stale-input guard.
