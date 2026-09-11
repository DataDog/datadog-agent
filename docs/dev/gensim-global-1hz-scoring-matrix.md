# GenSim global 1 Hz scoring matrix

> **Evaluator only.** Never expose `uses_1hz` or treatment labels to Bits.

Status: **30 arms recorded; 0 current-arm DDEval runs.** Twenty-five are blocked on the Eval Data Portal projection fix; five are excluded.

## Scoring key

Report raw values and paired `1 Hz - control` deltas. Admission fields are gates, not points; missing evidence stays `null`.

| Group | Measures |
|---|---|
| Admission | Execution, event identity, archive integrity, diagnostic telemetry, Scenario Store readiness |
| RCA quality | Pass/inconclusive, judge scores, immediate/deeper cause, remediation |
| Metric retrieval | Intended query/scope, requested and effective cadence, completeness, gaps, overrides |
| Evidence use | Whether 1 Hz evidence changed reasoning, supported the chain, and tested alternatives |
| Safety | False-resolution claims, contradictions, remediation safety |
| Reliability / efficiency | Duration, iterations, calls, errors, tokens, cost |

## Scenario expectations and packet adaptations

`weak_improve` counts as an expected improvement. `—` means no scenario-specific packet exception beyond the shared bounded Agent adaptation.

| Episode | Scenario | Class | Expected | Why | Packet exception |
|---:|---|---|---|---|---|
| 2467 | cold-cache-stampede | `resolution_exposed` | `improve` | One-second cache telemetry should expose the short miss/load burst that starts the stampede. | — |
| 2481 | stack-parser-backtracking | `resolution_exposed` | `improve` | One-second container CPU should align transient regex backtracking with processing lag. | — |
| 2483 | serialization-regression | `resolution_exposed` | `improve` | One-second enqueue/error counts should separate serializer failure onset from the downstream drop. | remove_two_invalid_simple_alert_new_group_delay_fields |
| 2497 | pool-routing-exhaustion | `resolution_irrelevant` | `neutral` | The decisive pool-routing evidence is in logs and traces, not metric cadence. | — |
| 2498 | nla-drain-rebalance-storm | `resolution_exposed` | `improve` | One-second service gauges should resolve the simultaneous drain and rebalance sequence. | empty-env adapter |
| 2500 | flask-migration-console-errors | `resolution_irrelevant` | `neutral` | Deterministic Flask dispatch errors are already explicit in logs and traces. | — |
| 2502 | workflow-history-stall | `metric_neutral` | `neutral` | The workflow stall is durable, so finer sampling should add little causal information. | — |
| 2563 | broker-replication-overload | `resolution_exposed` | `improve` | One-second broker gauges should reveal overload onset and replication-pressure ordering. | — |
| 2574 | fts-segment-corruption | `mixed_secondary` | `weak_improve` | A finer accuracy gauge may help timing, but logs and persistent corruption remain primary. | — |
| 2590 | empty-metric-zero-fill | `resolution_exposed` | `improve` | One-second empty-series counts should distinguish the zero-fill burst from apparent success. | — |
| 2592 | audit-logger-config-gap | `resolution_exposed` | `improve` | One-second audit counts should expose the enqueue gap and its downstream alert impact. | — |
| 2597 | distributed-login-failures | `mixed_secondary` | `weak_improve` | Finer failure counts may improve timing, but authentication logs should dominate RCA. | empty-env adapter |
| 2601 | cloud-control-cascade | `resolution_irrelevant` | `neutral` | The cascade is log-led, and the corrected captures contain no diagnostic telemetry tracks. | `corrected_2601_v1`: Correct the generator, 12 generated service app.py files, and 12 matching readiness templates; exclude the malformed original v3 attempts. |
| 2643 | scanner-native-memory-oom | `metric_neutral` | `neutral` | The producer emits the relevant gauge every 10 seconds, so Agent 1 Hz cannot add samples. | — |
| 2666 | compliance-query-outage | `metric_neutral` | `neutral` | The compactor gauge is emitted every 30 seconds, so Agent 1 Hz cannot improve cadence. | — |

## Cohorts

Cohorts describe DDEval readiness, not treatment or outcome quality.

| Cohort | Size | Definition |
|---|---:|---|
| `primary` | 24 arms / 12 complete pairs | Twelve complete A/B pairs whose arms have usable event identity and diagnostic telemetry. |
| `supplementary` | 1 arm / 0 complete pairs | 2483 arm B is individually eligible, but its arm-A counterpart lacks a unique event identity. |
| `excluded` | 5 arms / 0 complete pairs | 2467 A/B and 2483 A lack usable event identity; corrected 2601 A/B are diagnostic-telemetry empty. |

## 30-arm results

`blocked` means no DDEval launched. Excluded rows remain visible.

| Episode | Scenario | Arm / run | 1 Hz? | Expected improve? | Execution / event | Archive | Cohort | EDP / DDEval | Bits |
|---:|---|---|:---:|:---:|---|---|---|---|---|
| 2467 | cold-cache-stampede | arm-a<br>`001-pre-rendered-2467-611aea` | no | yes | succeeded; `no_transition` | 5,561,455 rows / 844 objects | `excluded` | excluded; `not_planned_event_identity_ineligible` | not run |
| 2467 | cold-cache-stampede | arm-b<br>`002-pre-rendered-2467-ac5659` | yes | yes | succeeded; `no_transition` | 4,593,849 rows / 839 objects | `excluded` | excluded; `not_planned_event_identity_ineligible` | not run |
| 2481 | stack-parser-backtracking | arm-a<br>`003-pre-rendered-2481-3a6f06` | no | yes | succeeded; `transition_observed` | 6,110,473 rows / 874 objects | `primary` | ingest failed; 138,863 B; `pending_blocked_edp_projection_size` | not run |
| 2481 | stack-parser-backtracking | arm-b<br>`004-pre-rendered-2481-318b31` | yes | yes | succeeded; `transition_observed` | 3,813,702 rows / 754 objects | `primary` | previewed; 142,882 B; `pending_blocked_edp_projection_size` | not run |
| 2483 | serialization-regression | arm-a<br>`005-pre-rendered-2483-cc42a6` | no | yes | succeeded; `evidence_unavailable` | 3,441,742 rows / 651 objects | `excluded` | excluded; `not_planned_event_identity_ineligible` | not run |
| 2483 | serialization-regression | arm-b<br>`006-pre-rendered-2483-8b20b0` | yes | yes | succeeded; `transition_observed` | 2,663,183 rows / 790 objects | `supplementary` | previewed; 142,784 B; `pending_blocked_edp_projection_size` | not run |
| 2497 | pool-routing-exhaustion | arm-a<br>`007-pre-rendered-2497-5a336b` | no | no | succeeded; `transition_observed` | 5,401,467 rows / 681 objects | `primary` | previewed; 132,416 B; `pending_blocked_edp_projection_size` | not run |
| 2497 | pool-routing-exhaustion | arm-b<br>`008-pre-rendered-2497-8ad701` | yes | no | succeeded; `transition_observed` | 416,248 rows / 681 objects | `primary` | previewed; 123,066 B; `pending_blocked_edp_projection_size` | not run |
| 2498 | nla-drain-rebalance-storm | arm-a<br>`007-pre-rendered-2498-864d71` | no | yes | succeeded; `transition_observed` | 5,046,642 rows / 726 objects | `primary` | previewed; 139,016 B; `pending_blocked_edp_projection_size` | not run |
| 2498 | nla-drain-rebalance-storm | arm-b<br>`008-pre-rendered-2498-07bac5` | yes | yes | succeeded; `transition_observed` | 6,172,473 rows / 722 objects | `primary` | previewed; 139,943 B; `pending_blocked_edp_projection_size` | not run |
| 2500 | flask-migration-console-errors | arm-a<br>`009-pre-rendered-2500-a751af` | no | no | succeeded; `transition_observed` | 395,411 rows / 803 objects | `primary` | previewed; 146,309 B; `pending_blocked_edp_projection_size` | not run |
| 2500 | flask-migration-console-errors | arm-b<br>`010-pre-rendered-2500-5578c6` | yes | no | succeeded; `transition_observed` | 481,551 rows / 779 objects | `primary` | previewed; 141,309 B; `pending_blocked_edp_projection_size` | not run |
| 2502 | workflow-history-stall | arm-a<br>`015-pre-rendered-2502-a36a08` | no | no | succeeded; `transition_observed` | 2,137,497 rows / 835 objects | `primary` | previewed; 152,046 B; `pending_blocked_edp_projection_size` | not run |
| 2502 | workflow-history-stall | arm-b<br>`016-pre-rendered-2502-911a4b` | yes | no | succeeded; `transition_observed` | 1,971,702 rows / 700 objects | `primary` | previewed; 126,290 B; `pending_blocked_edp_projection_size` | not run |
| 2563 | broker-replication-overload | arm-a<br>`009-pre-rendered-2563-ba5d84` | no | yes | succeeded; `transition_observed` | 3,733,825 rows / 708 objects | `primary` | previewed; 133,937 B; `pending_blocked_edp_projection_size` | not run |
| 2563 | broker-replication-overload | arm-b<br>`010-pre-rendered-2563-526567` | yes | yes | succeeded; `transition_observed` | 1,220,347 rows / 695 objects | `primary` | previewed; 127,912 B; `pending_blocked_edp_projection_size` | not run |
| 2574 | fts-segment-corruption | arm-a<br>`003-pre-rendered-2574-f160dc` | no | yes | succeeded; `transition_observed` | 1,275,828 rows / 824 objects | `primary` | previewed; 151,899 B; `pending_blocked_edp_projection_size` | not run |
| 2574 | fts-segment-corruption | arm-b<br>`004-pre-rendered-2574-1dbd88` | yes | yes | succeeded; `transition_observed` | 526,094 rows / 648 objects | `primary` | previewed; 118,758 B; `pending_blocked_edp_projection_size` | not run |
| 2590 | empty-metric-zero-fill | arm-a<br>`011-pre-rendered-2590-d586d8` | no | yes | succeeded; `transition_observed` | 1,419,956 rows / 850 objects | `primary` | previewed; 156,751 B; `pending_blocked_edp_projection_size` | not run |
| 2590 | empty-metric-zero-fill | arm-b<br>`012-pre-rendered-2590-1ae252` | yes | yes | succeeded; `transition_observed` | 3,373,265 rows / 728 objects | `primary` | previewed; 134,987 B; `pending_blocked_edp_projection_size` | not run |
| 2592 | audit-logger-config-gap | arm-a<br>`013-pre-rendered-2592-a96008` | no | yes | succeeded; `transition_observed` | 5,047,940 rows / 737 objects | `primary` | previewed; 137,302 B; `pending_blocked_edp_projection_size` | not run |
| 2592 | audit-logger-config-gap | arm-b<br>`014-pre-rendered-2592-b1b6dd` | yes | yes | succeeded; `transition_observed` | 2,922,732 rows / 827 objects | `primary` | previewed; 150,624 B; `pending_blocked_edp_projection_size` | not run |
| 2597 | distributed-login-failures | arm-a<br>`005-pre-rendered-2597-6a00e2` | no | yes | succeeded; `transition_observed` | 742,529 rows / 806 objects | `primary` | previewed; 145,994 B; `pending_blocked_edp_projection_size` | not run |
| 2597 | distributed-login-failures | arm-b<br>`006-pre-rendered-2597-f6353f` | yes | yes | succeeded; `transition_observed` | 811,936 rows / 847 objects | `primary` | previewed; 151,874 B; `pending_blocked_edp_projection_size` | not run |
| 2601 | cloud-control-cascade | arm-a<br>`001-pre-rendered-2601-0e9cde` | no | no | succeeded; `transition_observed` | 1,216 rows / 13 objects | `excluded` | excluded; `not_planned_telemetry_empty` | not run |
| 2601 | cloud-control-cascade | arm-b<br>`002-pre-rendered-2601-ac1de0` | yes | no | succeeded; `transition_observed` | 1,232 rows / 13 objects | `excluded` | excluded; `not_planned_telemetry_empty` | not run |
| 2643 | scanner-native-memory-oom | arm-a<br>`017-pre-rendered-2643-dc8eb6` | no | no | succeeded; `transition_observed` | 8,723,359 rows / 778 objects | `primary` | previewed; 150,579 B; `pending_blocked_edp_projection_size` | not run |
| 2643 | scanner-native-memory-oom | arm-b<br>`018-pre-rendered-2643-881e78` | yes | no | succeeded; `transition_observed` | 10,054,758 rows / 790 objects | `primary` | previewed; 153,307 B; `pending_blocked_edp_projection_size` | not run |
| 2666 | compliance-query-outage | arm-a<br>`001-pre-rendered-2666-bbdc19` | no | no | succeeded; `transition_observed` | 3,201,349 rows / 948 objects | `primary` | previewed; 178,037 B; `pending_blocked_edp_projection_size` | not run |
| 2666 | compliance-query-outage | arm-b<br>`002-pre-rendered-2666-6e9bc5` | yes | no | succeeded; `transition_observed` | 1,788,706 rows / 735 objects | `primary` | previewed; 137,964 B; `pending_blocked_edp_projection_size` | not run |

## Update rule and current blocker

Populate Bits/judge fields only from exact iteration artifacts. The historical 2483 smoke proves plumbing only and is not a current-arm result.

**Blocker:** `rpc error: code = InvalidArgument desc = enrichment_contexts[0].data exceeds maximum size of 131072 bytes (got 138863 bytes); store large payloads out-of-band and reference them by URI`

No Scenario Store write or DDEval launch is authorized until the projection fix is deployed, exact previews pass production-equivalent validation, and a fresh write is approved.

Machine-readable fields, provenance, and evidence hashes: `gensim-global-1hz-scoring-matrix.json`.
