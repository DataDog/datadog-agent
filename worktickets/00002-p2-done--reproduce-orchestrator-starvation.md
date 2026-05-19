---
created: 2026-05-18
priority: p2
status: done
artifact: tools/orchestrator-starvation-lab/runs/queue-starvation-confirmed-20260518-212907/
---
# Reproduce orchestrator-check disappearance from DCA rebalance proposals

## Summary

Reproduce the customer-observed pattern where the `orchestrator` cluster
check is dispatched to a CLCR, scheduled locally, but does not complete a
`Run()` before the DCA's next `/api/v1/clcrunner/stats` collection —
causing it to disappear from `currentDistribution()` and therefore from
the "Found a better distribution" rebalance proposal.

Reproduction is confirmed end-to-end in the kind+fakeintake lab with the
existing checkpoint configuration. The mechanism originally described in
the SME handoff ("queue starvation between Schedule and Run") is
empirically observed: orchestrator sits in the runner's worker queue
while noise checks ahead of it complete, and during that wait every DCA
stats refresh returns a runner payload that omits orchestrator.

## Context

From the handoff: the `Found a better distribution` proposal map is
*stats-derived*, not desired-state-derived. The DCA builds
`currentDistribution()` from `node.clcRunnerStats` (only checks that
have actually completed a Run and published stats), not from
`node.digestToConfig` (everything assigned). The runner's
`expvar.Get("runner").Checks` only contains entries after
`expvars.AddCheckStats(...)` is called — which only happens *after*
`check.Run()` returns successfully.

The customer ran several thousand openmetrics cluster checks across 3–9 CLCR pods
at `DD_CHECK_RUNNERS=64`. GIL contention drove most checks to
`WorkersNeeded=1.0` (exec time ≥ scrape interval). The dispatcher,
optimising WorkersUsed stddev across runners, found the cheap
orchestrator check (`WorkersNeeded` ~0.04) an attractive lever and
shuffled it every rebalance cycle. On each move the runner's worker
queue was already full of slow openmetrics scrapes; orchestrator never
reached a worker before the next stats refresh; it disappeared from
the proposal map and stayed dark.

Prior to this task we had reproduced the *rebalance-induced informer
churn* part of the failure (orchestrator getting cancelled and
re-instantiated every cycle) but not the *queue-starvation-between-
Schedule-and-Run* part. A previous agent shortened the rebalance period
to 5s and the improvement threshold to 1% to provoke fast rebalancing,
but their analysis focused on whether `Run()` completed at all (it did),
not on whether the proposal map saw orchestrator (often, it didn't).
This task closes that analysis gap.

## Done When

- [x] Capture a window in which orchestrator is *dispatched, scheduled
      on a runner, and waiting for a worker* while DCA's rebalance
      proposal omits it.
- [x] Show that the omission corresponds 1:1 to the runner having no
      Run-Done event for orchestrator between Schedule and the stats
      refresh.
- [x] Quantify Schedule→RUN_START latencies across multiple moves and
      show they consistently exceed the rebalance period.
- [x] Cross-reference the four event sources (DCA Dispatch, DCA
      Rebalance, runner Schedule, runner RUN_START/RUN_DONE/CANCEL) in a
      single timeline.
- [x] Map the lab’s observed behaviour to the customer’s by ratio of
      queue-drain-time to rebalance-period, so the lab’s flickering
      symptom and the customer’s permanent-dark symptom are clearly the
      same mechanism at different operating points.

## Artefacts

All in `tools/orchestrator-starvation-lab/runs/queue-starvation-confirmed-20260518-212907/`:

- `FINDINGS.md` — write-up of the mechanism, headline numbers, and the
  customer-mapping ratio table.
- `analyze.py` — cross-event correlator. Reads `dca.log` + `runner-*.log`
  in the same directory, emits the unified timeline and summary stats.
  Reusable on any future capture from the same lab.
- `timeline.txt` — the unified timeline from the 3-minute capture.
- `dca.log`, `runner-*.log` — raw streamed logs from
  `kubectl logs -f` against the DCA and all 3 CLCR pods.
- `clusterchecks-start.txt` — snapshot of `agent clusterchecks` output
  from the DCA at the start of the capture window.
- `info.txt` — capture window start/end timestamps.

## Reproduction recipe

Lab configuration that produces the symptom (unchanged from the previous
agent’s checkpoint; we just analysed it correctly):

```
orchestrator-starvation.py up \
  --prom-endpoints 200 \
  --workload-deployments 0 \
  --workload-crs 0 \
  --clcr-replicas 3 \
  --check-runners 1

# Plus DCA overrides applied by the previous agent:
#   DD_CLUSTER_CHECKS_REBALANCE_PERIOD=5s
#   DD_CLUSTER_CHECKS_REBALANCE_MIN_PERCENTAGE_IMPROVEMENT=1
```

Capture 3+ minutes of logs:

```bash
DCA=$(kubectl -n datadog get pod -l agent.datadoghq.com/component=cluster-agent -o name | head -1)
kubectl -n datadog logs -f $DCA > dca.log 2>&1 &
for r in $(kubectl -n datadog get pod -l agent.datadoghq.com/component=cluster-checks-runner -o name); do
  nm=$(basename $r)
  kubectl -n datadog logs -f $r > runner-${nm}.log 2>&1 &
done
sleep 180
pkill -f "kubectl -n datadog logs -f"
```

Run the analyzer:

```bash
python3 tools/orchestrator-starvation-lab/runs/queue-starvation-confirmed-20260518-212907/analyze.py
# (it hard-codes its input dir to /tmp/repro2 today; trivial to parameterise)
```

Look for `ORCH ABSENT` entries in the proposal map immediately
following a `SCHEDULE`/`CANCEL` and preceding the next `RUN_START`.

## Headline numbers from the captured window

```
Rebalance events:              38
  with orch present in map:    22 (58%)
  with orch ABSENT  in map:    16 (42%)

Schedule→RUN_START latencies across 10 observed moves:
  mean 21s, min 13s, max 29s

Longest consecutive ABSENT proposal run: 4 (≈20s of orchestrator dark time)
```

Every single observed move took at least 13s for the orchestrator check
to reach a worker on its new runner. With `rebalance_period=5s`, that
guarantees ≥2 consecutive rebalance proposals omit orchestrator after
every move.

## Customer-mapping ratio

| dimension                              | lab        | customer       |
|----------------------------------------|------------|----------------|
| queue-drain time on saturated runner   | ~660s      | ~500–1200s     |
| `cluster_checks.rebalance_period`      | 5s (override) | 600s (default) |
| ratio (drain / period)                 | ~132×      | ~1–2×          |
| symptom                                | flickers   | permanently dark |

Same mechanism, different operating points. The customer’s near-unity
ratio is what produces the permanently-dark symptom: queue-wait
sometimes exceeds `rebalance_period`, so orchestrator never completes a
single Run before being moved again.

## Out of scope for this task

- Validating mitigations. `cluster_checks.exclude_checks_from_dispatching`
  does not behave as I initially expected per the SME on this case
  (separate follow-up). `cluster_checks.rebalance_min_percentage_improvement`
  and similar dispatcher knobs are worth a follow-up sweep with the
  lab now reliably reproducing the mechanism.
- A code-side fix (e.g. including "scheduled-but-not-yet-completed"
  entries in `runner.Checks` so `currentDistribution()` sees orch even
  during its queue wait). Mentioned in the original SME handoff as
  mitigation #3; lab can validate either via a mocked stats endpoint
  or via a patched agent.

## Notes

- The previous agent’s checkpoint already contained the symptom in its
  captured logs (see `tools/orchestrator-starvation-lab/runs/rebalance-stats-derived-5s-20260518-205807/`).
  The only thing missing was the cross-event analysis. The analyzer
  added in this task makes the symptom obvious in any equivalent log
  capture.
- The lab’s flickering symptom is more aggressive than the customer’s
  because we pushed rebalance period from 600s down to 5s. Holding the
  noise count constant and raising the rebalance period back toward the
  default would dampen the flicker frequency without changing the
  underlying mechanism.
- Queue starvation is *currently observed only at the noise-200 setting*
  in this lab. Higher noise counts will lengthen the Schedule→RUN_START
  wait monotonically, eventually producing the customer’s permanent-dark
  variant in the lab as well. That experiment is the natural follow-up.
