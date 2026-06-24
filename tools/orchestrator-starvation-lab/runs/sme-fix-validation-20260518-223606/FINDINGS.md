# SME fix validation: `cluster_check: false` + `skip_leader_election: false`

## Headline

**The SME's proposed configuration eliminates the queue-starvation symptom.**

Applied via:

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

This takes the orchestrator check out of the cluster-checks dispatch
pipeline entirely. The leader DCA pod runs it locally on its own check
scheduler, with leader election ensuring exactly one DCA executes it at a
time in a multi-DCA setup.

## A/B comparison (lab; same noise + worker config; 3-minute capture windows)

Both captures use the existing flicker-reproducible setup (it was the
baseline established in 00002-p2-done):

```
--prom-endpoints 200            # 200 parse-bound openmetrics checks
--clcr-replicas 3               # 3 CLCR pods
--check-runners 1               # 1 worker per pod (synthetic; isolates effect)
DD_CLUSTER_CHECKS_REBALANCE_PERIOD=5s            # aggressive
DD_CLUSTER_CHECKS_REBALANCE_MIN_PERCENTAGE_IMPROVEMENT=1   # aggressive
```

### BASELINE — default `cluster_check: true` orchestrator

```
Rebalance events:        10
  orch PRESENT in map:   3   (30%)
  orch ABSENT  in map:   7   (70%)
Longest consecutive ABSENT streak: 6 rebalances (≈30 seconds dark)

Runner-side orchestrator activity:
  4mv7f: 14 runs done, 3 cancels, 3 schedules
  fs8kx: 0  runs done, 0 cancels, 0 schedules
  q4k44: 10 runs done, 1 cancel,  2 schedules
```

70% of rebalance ticks show the dispatcher silently ignoring orchestrator
because it's queue-starved on its assigned runner and hasn't reported
stats in time. Orch is being moved 5 times in 3 minutes.

### FIX — `cluster_check: false` + DCA-local with leader election

```
Rebalance events:                  1   (and orch wasn't in it — correctly)
`Dispatching configuration orchestrator` events from DCA: 0
Cancel events for orchestrator on CLCRs: 0  (post-rollover steady-state)

Orchestrator Total Runs on the DCA over 162 seconds:
  t=0s:     runs=20
  t=21s:    runs=22  (+2)
  t=41s:    runs=24  (+2)
  t=61s:    runs=26  (+2)
  t=81s:    runs=28  (+2)
  t=102s:   runs=30  (+2)
  t=122s:   runs=32  (+2)
  t=142s:   runs=34  (+2)
  t=162s:   runs=36  (+2)

Observed cadence: ~1 run / 10 seconds, exactly matching the
  configured min_collection_interval. Zero misses.
```

Orch on DCA: status `[OK]` / `[WARNING]` (transient due to OOTB-CRD
collector limits, which is a separate noise), execution time ~3.7s,
Last Successful Execution Date always within ~10s of sample time.

## What changed structurally

1. **No more DCA → CLCR dispatch for orchestrator.**
   `agent clusterchecks` on the DCA no longer assigns orchestrator to
   any runner. The orchestrator config has been removed from
   `digestToConfig` because it's no longer a cluster check.
2. **No more rebalance moves.**
   The rebalancer's proposal map cannot contain orchestrator. Even at
   `rebalance_period=5s` it has nothing to shuffle.
3. **No more CLCR-side queue starvation.**
   The DCA pod's own check scheduler runs orchestrator. The DCA isn't
   contending with thousands of openmetrics scrapes for worker slots.
4. **Leader election applies, so HA still works.**
   In a multi-DCA setup the standby DCAs see the check config but only
   the leader actually runs it (`skip_leader_election: false`).

## Caveats

- **DCA pod becomes the single point of execution for orchestrator data.**
  If the leader DCA is busy (heavy KSM, large cluster-checks dispatcher
  load, etc.) or restarting, orch goes dark for that duration. Customer
  should ensure the DCA pod has adequate resources.
- **DCA failover incurs an informer-resync cost.** Same 60–70s
  `SyncInformersReturnErrors` blocking call we saw on CLCR moves.
  Failover should be infrequent (DCA pod restarts only on rollouts /
  evictions), but every restart costs ~1–2 minutes of orchestrator
  data while informers re-warm on the new leader.
- **The orchestrator informer caches live in DCA memory.** Sizing the
  DCA pod's memory limit needs to account for the cluster's resource
  counts. The customer’s 35 GiB / 50 GiB DCA memory observation from
  the slack thread was about *KSM*, not orchestrator. Orchestrator's
  informer footprint is meaningfully smaller per resource type but
  still scales with cluster size.
- **The orchestrator check still has a `min_collection_interval=10s`
  default and Run() exec time still scales with cluster size.** What
  the fix solves is *dispatch starvation*, not exec-time cost.

## Reproducibility

Reproducing the baseline:

```bash
./orchestrator-starvation.py up \
  --prom-endpoints 200 \
  --workload-deployments 0 \
  --workload-crs 0 \
  --clcr-replicas 3 \
  --check-runners 1

# After up complete, add rebalance aggression on the DCA:
kubectl -n datadog patch datadogagent lab --type=json -p='[
  {"op":"add","path":"/spec/override/clusterAgent/env/-",
   "value":{"name":"DD_CLUSTER_CHECKS_REBALANCE_MIN_PERCENTAGE_IMPROVEMENT","value":"1"}},
  {"op":"add","path":"/spec/override/clusterAgent/env/-",
   "value":{"name":"DD_CLUSTER_CHECKS_REBALANCE_PERIOD","value":"5s"}}
]'

# Capture for 3+ minutes:
for pod in <DCA> <CLCR1> <CLCR2> <CLCR3>; do
  kubectl -n datadog logs -f $pod > /tmp/logs/$pod.log &
done
sleep 180
```

Applying the fix on top of the running baseline:

```bash
kubectl -n datadog patch datadogagent lab --type=merge -p='
{
  "spec": {
    "features": {
      "orchestratorExplorer": {
        "enabled": true,
        "conf": {
          "configData": "cluster_check: false\nad_identifiers:\n  - _kube_orchestrator\ninit_config:\ninstances:\n  - skip_leader_election: false\n"
        }
      }
    }
  }
}'
# Wait ~90s for DCA rollout and orch warmup, then re-capture.
```

## Files in this directory

```
baseline/                    # 3-minute capture with default cluster_check=true
  dca.log
  runner-*.log
  start.txt

fix/                         # 3-minute capture with SME's cluster_check=false config
  dca.log
  runner-*.log
  orch-tracker.csv          # 20s-interval samples of orch Total Runs on the DCA
  start.txt
```
