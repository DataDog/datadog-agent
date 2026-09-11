# GenSim global 1 Hz corpus matrix

> Outcome-blind source panel and current DDEval-readiness summary. Cohorts are readiness groups, not treatment assignments or outcome grades.

Status: **15 scenarios captured and archived; 25 arms are candidates for DDEval; Scenario Store ingestion is blocked.**

## 15-scenario corpus matrix

| Episode | Scenario | Evidence class | Expected 1 Hz effect | Packet exception | Current readiness |
|---:|---|---|---|---|---|
| 2467 | cold-cache-stampede | `resolution_exposed` | `improve` | — | Excluded: both arms lack usable event transitions |
| 2481 | stack-parser-backtracking | `resolution_exposed` | `improve` | — | Primary pair; ingestion blocked |
| 2483 | serialization-regression | `resolution_exposed` | `improve` | Remove two invalid simple-alert `new_group_delay` fields | One supplementary arm; partner lacks unique event identity |
| 2497 | pool-routing-exhaustion | `resolution_irrelevant` | `neutral` | — | Primary pair; ingestion blocked |
| 2498 | nla-drain-rebalance-storm | `resolution_exposed` | `improve` | Empty-environment YAML adapter | Primary pair; ingestion blocked |
| 2500 | flask-migration-console-errors | `resolution_irrelevant` | `neutral` | — | Primary pair; ingestion blocked |
| 2502 | workflow-history-stall | `metric_neutral` | `neutral` | — | Primary pair; ingestion blocked |
| 2563 | broker-replication-overload | `resolution_exposed` | `improve` | — | Primary pair; ingestion blocked |
| 2574 | fts-segment-corruption | `mixed_secondary` | `weak_improve` | — | Primary pair; ingestion blocked |
| 2590 | empty-metric-zero-fill | `resolution_exposed` | `improve` | — | Primary pair; ingestion blocked |
| 2592 | audit-logger-config-gap | `resolution_exposed` | `improve` | — | Primary pair; ingestion blocked |
| 2597 | distributed-login-failures | `mixed_secondary` | `weak_improve` | Empty-environment YAML adapter | Primary pair; ingestion blocked |
| 2601 | cloud-control-cascade | `resolution_irrelevant` | `neutral` | `corrected_2601_v1` source/readiness repair | Excluded: both arms lack diagnostic telemetry |
| 2643 | scanner-native-memory-oom | `metric_neutral` | `neutral` | — | Primary pair; ingestion blocked |
| 2666 | compliance-query-outage | `metric_neutral` | `neutral` | — | Primary pair; ingestion blocked |

## Gate key

| Gate | Pass condition |
|---|---|
| Source | Immutable source identity, runnable episode, objective ground truth, and outcome-blind evidence classification |
| Matched capture | Equivalent source, workload, resources, timing, monitors, and only the approved Agent-resolution difference |
| Event identity | Exact trigger/recovery identity is usable; ambiguity remains explicit |
| Archive | Workflow completed and every metadata/object reference passed direct readback |
| Diagnostic telemetry | Logs, traces, metrics, deployments, or profiles can support RCA |
| Scenario Store | Exact projection validates, ingests with `force:false`, and passes UUID/org/source readback |

## Packet adaptation key

All selected packets use the same bounded one-replica Agent configuration in `chart/templates/datadog-agent.yaml` and `chart/values.yaml`; only the resolution toggle differs between arms.

| Packet group | Episodes | Additional change |
|---|---|---|
| Standard `v3` | 2467, 2481, 2497, 2500, 2502, 2563, 2574, 2590, 2592, 2643, 2666 | None |
| Empty-environment `v3` | 2498, 2597 | Match the source packet's `env: ''` YAML representation |
| 2483 `v3` | 2483 | Remove exactly two invalid simple-alert `new_group_delay: 120` fields |
| `corrected_2601_v1` | 2601 | Repair the generator, 12 generated apps, and 12 readiness templates; retain malformed attempts as excluded evidence |

`v3` means the final frozen, outcome-blind derivative set. The selected corpus uses 14 v3 scenario pairs plus the separately frozen corrected 2601 pair. Exact hashes and pair-normalization proofs are in the [GenSim derivative ledger](https://github.com/DataDog/gensim/blob/bb927e366703e8ea740358941e47d3c56ddb59b8/src/controller/campaigns/global-1hz-derivative-change-ledger-v3.json).

## Cohort totals

| Cohort | Arms | Complete pairs | Meaning |
|---|---:|---:|---|
| Primary | 24 | 12 | Both arms pass event-identity and diagnostic-telemetry gates |
| Supplementary | 1 | 0 | Arm is independently usable but its partner is not |
| Excluded | 5 | 0 | Event identity is unusable or diagnostic telemetry is absent |

Detailed arm identities, archive counts, projection sizes, and future Bits results are in the [scoring matrix](https://github.com/DataDog/datadog-agent/blob/ella/global-1hz-metric-resolution-experiment/docs/dev/gensim-global-1hz-scoring-matrix.md).
