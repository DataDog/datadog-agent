---
created: 2026-05-18
priority: p2
status: done
artifact: tools/orchestrator-starvation-lab/runs/sme-fix-validation-20260518-223606/
---
# Validate SME's `cluster_check: false` config eliminates orchestrator starvation

## Summary

With the queue-starvation mechanism reproduced reliably (see
`00002-p2-done--reproduce-orchestrator-starvation.md`), validate the
SME's proposed mitigation by applying it to the live lab and confirming
orchestrator no longer disappears from the dispatcher's view.

The SME's proposal moves the orchestrator check off the cluster-checks
dispatch pipeline entirely:

```yaml
spec:
  features:
    orchestratorExplorer:
      enabled: true
      conf:
        configData: |-
          cluster_check: false
          ad_identifiers:
            - _kube_orchestrator
          init_config:
          instances:
            - skip_leader_election: false
```

With `cluster_check: false`, the orchestrator check runs on the leader
DCA pod's own check scheduler, never being assigned to a CLCR.
`skip_leader_election: false` ensures exactly one DCA executes it at a
time in a multi-DCA setup.

## Done When

- [x] Re-establish the reproducible baseline (orch flickering in/out of
      DCA rebalance proposals at the established `noise=200 / clcr=3 /
      workers=1 / rebalance_period=5s / min_pct=1` configuration).
- [x] Apply the SME's `orchestratorExplorer.conf.configData` override
      via the DDA CR.
- [x] Verify orchestrator is no longer dispatched as a cluster check
      (`agent clusterchecks --check orchestrator` on the DCA returns
      nothing assigned to runners; runners' `agent status` does not show
      the orchestrator check; no `Dispatching configuration orchestrator`
      log lines emitted).
- [x] Verify orchestrator runs on the DCA pod itself at its configured
      `min_collection_interval` (~10s) with no missed runs.
- [x] Capture an A/B comparison over equal-length windows so the
      before/after numbers are directly comparable.

## Result summary

**Baseline** (`cluster_check: true`, current default):
  - 10 rebalance events in 3 minutes
  - 70% of those had orchestrator ABSENT from the proposal map
  - Longest consecutive ABSENT streak: 6 proposals (≈30s of darkness)
  - 5 schedule events, 4 cancel events on CLCRs (orch moved every cycle)

**Fix** (`cluster_check: false`, SME's recipe):
  - 0 orchestrator dispatch events from the DCA
  - 0 orchestrator cancel events on CLCRs (post-rollover steady state)
  - DCA-local orchestrator: ~1 run / 10 seconds = full intended cadence
  - 16 successful Run()s observed in a 162-second sample window, 0 misses

## Caveats (documented in the run's FINDINGS.md)

1. **DCA becomes the single point of orchestrator execution.** Customer's
   DCA pod must have enough resources to host orchestrator's informer
   caches *in addition to* whatever it already runs (KSM, autoscaling
   etc.). The 35–50 GiB DCA memory observation from the slack thread is
   about KSM specifically, not orchestrator — but orchestrator does add
   to that footprint and scales with cluster object count.
2. **DCA failover incurs a fresh informer resync** (~60–70 seconds of
   blocking `SyncInformersReturnErrors`). Same code path as the CLCR
   resync we've measured; it just happens far less often (DCA pod
   restarts vs every-rebalance moves).
3. **The fix solves dispatch starvation, not exec-time cost.** Orch's
   `Run()` still walks its informer caches and scales with cluster size.
   At very large cluster sizes the DCA-local Run() can still exceed the
   `min_collection_interval` for non-dispatch reasons (CRD count,
   pod count, etc.).

## Artefacts

`tools/orchestrator-starvation-lab/runs/sme-fix-validation-20260518-223606/`:
  - `FINDINGS.md`               write-up + numbers + caveats
  - `baseline/dca.log` + runner-*.log     3-min capture with default config
  - `fix/dca.log` + runner-*.log + orch-tracker.csv    3-min capture with SME config

## Notes

- The `[WARNING]` status that appears on orchestrator under both
  baseline and fix is unrelated to the dispatch issue — it's the
  persistent OOTB-CRD collector limit warning ("reached to the limit
  5000, skipping") from the workload-crs from prior runs. It does not
  represent execution failure.
- We attempted to push the lab into a true perma-dark variant by scaling
  noise to 3000 (the customer's actual scale) but the orchestrator check
  still managed to complete ≈1 run every 32 seconds in that
  configuration. The customer's perma-dark is reached because their
  effective `(queue-drain / rebalance-period)` ratio is closer to unity
  due to GIL contention pegging WorkersNeeded at 1.0 for ~57% of checks
  (see the 2026-05-15 rebalance.txt analysis). The lab's 1-worker
  artificial constraint produces enough flicker (70% ABSENT) to be a
  rigorous before/after testbed without needing to recreate the customer's
  full GIL-saturation conditions.
- Reproducing perma-dark in the lab would require either (a) running
  against the customer's actual agent image at customer scale with
  realistic Prometheus payloads, or (b) injecting an artificial
  per-Run\(\) sleep into orchestrator to simulate a slow informer
  resync. Both are doable as follow-up work but the A/B demonstrated
  here is sufficient to validate the SME's proposal.
