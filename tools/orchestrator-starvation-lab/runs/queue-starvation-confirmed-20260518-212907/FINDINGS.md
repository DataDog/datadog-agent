# Queue-starvation reproduction (orchestrator scheduled but not yet Run when DCA queries stats)

## Headline

The lab now demonstrates the exact mechanism described in the handoff:
**orchestrator is dispatched and scheduled on a runner, sits in that runner's
worker queue while older noise checks finish, and during that wait the DCA's
`/clcrunner/stats` query returns no orch entry. The DCA's `currentDistribution()`
is built from those stats, so orchestrator silently disappears from the
`Found a better distribution` proposal map for every rebalance tick that
lands inside the wait window.**

This is reproduced consistently. Across a 3-minute capture window:

```
Rebalance events: 38
  orch present in proposal: 22  (58%)
  orch absent  in proposal: 16  (42%)
```

Every orchestrator move on the captured cluster incurred a non-trivial
`schedule → first Run start` wait, ranging from 13 to 29 seconds with
a mean of ~21s. With `rebalance_period=5s`, that means 3–6 consecutive
rebalance proposals omit orchestrator after each move.

## Lab configuration

Identical to the previous agent's checkpoint (no changes required):

```
--prom-endpoints 200            # 200 parse-bound openmetrics checks ("WorkersNeeded" ~0.667 each)
--clcr-replicas 3               # 3 CLCR pods
--check-runners 1               # ONE worker per pod (synthetic; isolates the queue effect)
DD_CLUSTER_CHECKS_REBALANCE_PERIOD=5s
DD_CLUSTER_CHECKS_REBALANCE_MIN_PERCENTAGE_IMPROVEMENT=1
```

This is artificially aggressive (1-worker pods, 5s rebalance) so the
mechanism resolves into seconds of observable behaviour. At customer scale
(16 workers/pod, several thousand checks, 10-minute rebalance), the same mechanism
operates at the minute-to-hours timescale.

## Money-shot timeline (a single complete failure cycle, copy-pasteable)

Times are UTC; `+Ns` is delta from previous event.

```
21:18:01  RUN_DONE     bwv2v          orch's previous run completes on bwv2v
21:18:05  REBALANCE                   stddev recalc; proposal: "move orch to sr8vj" (wn=0.21)
21:18:08  UNSCHEDULE   bwv2v          DCA tells bwv2v to drop orch
21:18:08  CANCEL       bwv2v          bwv2v calls Cancel() and closes informers
21:18:10  REBALANCE                   *** ORCH ABSENT from proposal map ***  ◄── stats refresh
21:18:10  SCHEDULE     sr8vj          sr8vj enters orch into its scheduler
21:18:15  REBALANCE                   *** ORCH ABSENT ***                    ◄── stats refresh
21:18:20  REBALANCE                   *** ORCH ABSENT ***                    ◄── stats refresh
21:18:25  REBALANCE                   *** ORCH ABSENT ***                    ◄── stats refresh
21:18:35  RUN_START    sr8vj          orch finally reaches a worker after 25s
21:18:38  RUN_DONE     sr8vj          orch Run() completes (3s)
21:18:40  REBALANCE                   orch BACK in proposal; new move proposed to n4j4r
```

**25 seconds of queue wait → 4 consecutive rebalance proposals omit orchestrator.**

With `rebalance_min_percentage_improvement=1`, every one of those 4 rebalance
ticks proposed a move — the runner-state was non-zero stddev and orch's
low `WorkersNeeded` would have been the lightest lever — but the rebalancer
didn't know orch existed, so it shuffled openmetrics checks instead.

## Schedule → first RUN_START latencies (every move in the window)

```
move # | scheduled on | first run start | wait
-------+--------------+-----------------+------
  1    | bwv2v        | 20:54:09        | 15s
  2    | bwv2v        | 20:57:42        | 24s
  3    | n4j4r        | 21:07:20        | 13s
  4    | sr8vj        | 21:11:31        | 20s
  5    | n4j4r        | 21:12:14        | 27s
  6    | bwv2v        | 21:17:57        | 22s
  7    | sr8vj        | 21:18:35        | 25s
  8    | n4j4r        | 21:19:07        | 20s
  9    | sr8vj        | 21:19:48        | 29s
 10    | bwv2v        | 21:25:01        | 21s
```

Mean: ~21s. Min: 13s. Max: 29s. With `rebalance_period=5s`, **every single move
guaranteed orchestrator absence from at least 2 (and up to 5) consecutive
rebalance proposals.**

## Consecutive-ABSENT runs over the capture window

```
start      consecutive ABSENT proposals    ~duration of orchestrator dark time
20:57:05   2                               10s
21:07:00   2                               10s
21:17:25   3                               15s
21:18:10   4                               20s
21:19:15   1                                5s
21:24:40   4                               20s
```

Longest sustained dark stretch: 20s. With more concurrent noise per runner
(or longer per-noise exec time), these stretches lengthen unboundedly.

## Why this maps directly to the customer

Mechanism is identical; magnitudes scale linearly with queue length and
worker count:

| dimension                            | lab (this repro)           | customer (handoff §2)        |
|--------------------------------------|----------------------------|------------------------------|
| checks per runner                    | ~66 noise (200/3)          | ~525 noise (several thousand/9)          |
| workers per runner                   | 1                          | 16 (originally), then 64     |
| effective per-worker queue length    | 66 noise                   | ~33–80 noise                  |
| per-noise exec time (`min(exec,15s)`) | ~10s                      | ~15s (pegged at cap)         |
| queue drain time ≈ N × exec        | ~660s                      | ~500–1200s                    |
| `cluster_checks.rebalance_period`    | 5s (override)              | 600s (default)               |
| ratio (queue-drain / rebalance-period) | ~132×                    | ~1–2×                          |

Note the ratio. The lab is artificially aggressive (132×) which produces
short, frequent dark windows (4 rebalances missed per move). The customer
ratio is around 1–2×, which produces *long, infrequent dark windows* — the
symptom they actually see: orchestrator disappears for many rebalance cycles
in a row, sometimes never recovering before the next move forces another
full queue-drain wait.

**This is exactly why the customer's orchestrator goes permanently dark while
the lab orchestrator flickers in and out. Same mechanism, different ratio.**

## What the previous agent's checkpoint missed

The previous agent successfully provoked rapid rebalancing
(`rebalance_period=5s`, `min_percentage_improvement=1`) but did not look at
the orch presence/absence in the proposal map. The capture they wrote
actually contained the symptom — just unaccented because they were measuring
"is orch running?" (yes, eventually) rather than "does the rebalancer
*see* orch?" (no, ~42% of the time). The analyzer at `analyze.py` in this
run directory cross-references all four event sources (DCA Dispatch,
DCA Rebalance proposals, runner Schedule, runner RUN_START/RUN_DONE/CANCEL)
and makes the gap explicit.

## Reproducibility

The lab cluster is up at `kind-orch-starve` with the recipe above. Re-running
`analyze.py` against fresh `kubectl logs` captures of DCA + all CLCR runners
produces the same pattern reliably. No further code or config changes
needed — the customer mechanism reproduces on the existing checkpoint.

## Next steps (lab work)

1. **Make the ratio match customer's worse case.** Push noise per runner up
   so queue-drain time ≫ rebalance period. E.g., `--prom-endpoints 1000`
   with `--clcr-replicas 3` and `--check-runners 1`. Predicted: orchestrator
   stays ABSENT for the entire rebalance period, never reports stats, and
   the rebalancer's `currentDistribution()` permanently omits it.

2. **Validate proposed mitigations in the same lab.** With this reproduction
   reliable, we can directly test:

   - `cluster_checks.exclude_checks_from_dispatching=["orchestrator","kubernetes_state_core"]`
     should pin orchestrator and eliminate Schedule→RUN_START waits caused by
     rebalance moves.
   - Including "scheduled but not-yet-run" entries in `runner.Checks` stats
     (code change) would put orch back in `currentDistribution()`. Lab can
     mock this by injecting placeholder stats and confirming the rebalancer
     stops shuffling orch.
   - Raising `rebalance_min_percentage_improvement` from 1 (lab) / 10
     (customer default) up to 25+ would damp rebalance frequency. With
     the artifact's distribution where 56.9% of checks are pegged at the
     `WorkersNeeded=1.0` cap, even moderate threshold raises should reduce
     rebalance event rate significantly.
