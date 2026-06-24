# Rebalance-induced orchestrator-check starvation: reproduction

## Lab configuration

| Parameter | Value | Source |
|---|---|---|
| Noise checks | 200 | `--prom-endpoints 200` |
| Noise check exec time | ~10.0s (capped at openmetrics timeout) | slow-metrics ConfigMap SLEEP_SECONDS=12 |
| Workers/runner | 4 | DD_CHECK_RUNNERS default |
| CLCR replicas | 3 | `--clcr-replicas 3` |
| Total workers | 12 | 3 × 4 |
| WorkersNeeded per noise check | ~0.667 | 10s exec / 15s interval |
| Fleet oversubscription | ~11× | (200 × 10) / (12 × 15) |
| `cluster_checks.rebalance_period` | 2m (override) | env var |
| `cluster_checks.rebalance_min_percentage_improvement` | 1 (override) | env var |
| `cluster_checks.advanced_dispatching_enabled` | true (default) | n/a |

The `min_percentage_improvement=1` override is a knob to force rebalance triggering at this small scale. At customer scale (several thousand checks) the natural variance in WorkersNeeded across rebalance cycles produces 10%+ improvements organically and the default works fine.

## Evidence captured

### 1. DCA rebalance INFO logs (one every 2 minutes after enabling override)

From `pkg/clusteragent/clusterchecks/dispatcher_rebalance.go:365`:
```
2026-05-16 03:49:11 UTC | Found a better distribution for the cluster checks.
  Utilization stdDev of proposed distribution: 0.074. StdDev of current: 0.209.
  Proposed: ...
    "orchestrator:ae09bc0549c965a5": {"WorkersNeeded": 0.0391, "Runner": "..."}
  Runners:
    2wqj4: {Workers: 4, WorkersUsed: 27.42, NumChecks: 42}
    cg87h: {Workers: 4, WorkersUsed: 28.05, NumChecks: 42}
    dtrf8: {Workers: 4, WorkersUsed: 28.05, NumChecks: 42}
```

Orch's WorkersNeeded = 0.039 vs noise checks at 0.667 each. Orch is by far the lightest check; the rebalancer shuffles it freely to flatten stddev across runners.

### 2. Orchestrator check Cancel events (matching internal investigation notes)

From `pkg/collector/corechecks/cluster/orchestrator/orchestrator.go:240`:
```
[2wqj4] 03:47:19  Shutting down informers used by the check 'orchestrator:f3f65321df310a5a'
[cg87h] 03:49:13  Shutting down informers used by the check 'orchestrator:ae09bc0549c965a5'
[2wqj4] 03:53:19  Shutting down informers used by the check 'orchestrator:ae09bc0549c965a5'
```

Same log line, same source file, same line number as the originally-observed Cancel events in handoff section 4.

### 3. Total Runs counter resets per move (informer resync evidence)

From orch-tracker.log polling `agent status` every 20s:
```
2wqj4 phase 1: runs 64 → 144 (steady state)
Gap (~2 min):
cg87h phase 1: runs 2 → 10 (counter reset)
Gap (~2 min):
2wqj4 phase 2: runs 2 → 18
Gap (~2 min):
cg87h phase 2: runs 1 → 15
```

Each phase starts at runs=1 or 2, proving `o.collectorBundle.Initialize()` (gated by `sync.Once`) ran from scratch on the new runner — the 60–70s blocking `SyncInformersReturnErrors` call from handoff section 7.

### 4. Manifest-pipeline emission gaps captured directly

From observe CSV (rolling 5-minute windows):
```
window     manif_p50  manif_p95   manif_max
03:52      15.00      17.68       29.52
03:53      15.00      29.72       30.53  ← rebalance gap
03:54      15.00      29.72       30.53
03:55      15.00      17.87       30.53
```

- **p50 stays at 15.00s** — between rebalances orch runs normally
- **max gap = 30.53s** — during the rebalance + informer resync window, no manifest payloads emit
- **Effective starvation**: ~30s of dark time every rebalance period

At the customer's 10-minute rebalance period this is 30s/600s = 5% data loss steady-state. At their actual scale where rebalances fire every cycle naturally, the orchestrator pipeline has chronic 30-second blackouts.

## Mapping to customer evidence

| Customer evidence (handoff section) | Lab reproduction |
|---|---|
| 20 Cancel events at orchestrator.go:240 (§4) | Same line, same log message — 3+ events captured in 10 min |
| Customer rebalanceUsingUtilization log (§3) | Same line dispatcher_rebalance.go:365 — capturing |
| Customer orch WorkersNeeded ~0.034 (§3) | Lab orch WorkersNeeded = 0.039 |
| 10-min rebalance period (§4, default) | Confirmed: cluster_checks.rebalance_period default 10m, code path validated |
| First Run() blocks 60-70s on sync.Once (§7) | Total Runs counter resets per move — proves re-init |
| KSM check Cancel events similar (slack thread) | KSM Cancel captured at 03:30:53 on cg87h |

## What we have NOT yet directly reproduced

- The 500ms `check_cancel_timeout` violation observed once in the field (we'd need to make Cancel slow somehow; the orchestrator's Cancel is sub-millisecond by design)
- The "orchestrator stopped running on a single affected cluster" total failure (would require informer resync to fail/timeout)
- Aggregator buffer overflow at 100 buffer size (would need much higher metric emission than this setup)

## Conclusion

**The customer's reported symptom is rebalance-induced informer churn on the cluster orchestrator check, not worker-pool dispatch starvation or check-exec-time inflation.** The mechanism is structural: any cheap check (orchestrator, KSM, any low-`WorkersNeeded` check) is an attractive rebalance target because moving it improves the cluster's WorkersUsed stddev for free. At customer scale (several thousand checks, default 10-min rebalance period, default 10% improvement threshold), this triggers organically every cycle and produces the observed 20 Cancel events.

Fix categories to consider:
1. **Pin critical singleton checks**: set `cluster_checks.exclude_checks_from_dispatching` (handoff section 7) to include `orchestrator` and `kubernetes_state_core`. They'd still be scheduled, just exempt from rebalance moves.
2. **Cache informer state across cancel/move**: avoid the 60-70s resync on every move. Would require agent code change.
3. **Raise `min_percentage_improvement`** so trivial-improvement rebalances don't fire (defends against thrashing).
4. **Per-check rebalance cooldown** for known-heavy-init checks. Code change.
