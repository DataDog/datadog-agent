# GenSim global 1 Hz corpus matrix v2 — local draft

> **Draft only.** This document does not replace the active matrix, change Scenario Store membership, or modify the DDEval run occurring in another session. It is intended for manual review before any future Confluence update.

Status date: **2026-09-21**

## Matrix summary

| Category | Scenarios or sources | Arms | Complete pairs | Current meaning |
|---|---:|---:|---:|---|
| Verified primary cohort | 12 | 24 | 12 | Immutable archives ingested and independently verified in Scenario Store |
| Supplementary historical evidence | 1 | 1 | 0 | Episode 2483 B is independently usable but intentionally absent from the primary dataset |
| Historical exclusions | 3 source episodes | 5 | 0 | Preserved archives cannot be admitted without fresh upstream rehabilitation |
| Prospective incident-grounded sources — advance | 3 | 0 | 0 | Worth designing as new Agent-bearing applications; existing DES archives are not admissible |
| Prospective incident-grounded sources — hold | 3 | 0 | 0 | Source quality is useful, but a cadence, lifecycle, or materialization blocker must be resolved first |

Verified primary dataset:

- Name: `global-1hz-alert-eval-primary-v2-20260917`
- UUID: `30853552-3a2a-49bf-acf9-db94c74e40c4`
- Membership: 24 scenarios, all mappings `user_added`, zero excluded mappings
- Independent verification: `verified`

Evidence:

- `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence/eval-data-portal-primary-continuation-20260918T154956Z/summary.json`
- `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence/eval-data-portal-primary-continuation-20260918T154956Z/final-independent-readback-v2.json`

## Interpretation rules

1. **Verified**, **rehabilitation**, and **prospective** are evidence-lifecycle states, not treatment outcomes.
2. No candidate is selected or rejected using DDEval, Bits, judge, or treatment-arm results.
3. A Synthtel DES bundle with `collection_interval_s=1.0` demonstrates simulator cadence only. It does not show that a concrete workload emits through the Datadog Agent or that the Agent resolution toggle changes the evidence.
4. A prospective source becomes a corpus addition only after fresh Agent-bearing materialization, matched derivative freezing, execution, event-identity validation, diagnostic-telemetry validation, immutable archival, and Scenario Store readback.
5. Rehabilitation never mutates or relabels the historical excluded archives.

## Verified primary cohort

The source-based evidence class and 1 Hz hypothesis remain outcome-blind predictions.

| Episode | Scenario | Evidence class | 1 Hz hypothesis | Verified status |
|---:|---|---|---|---|
| 2481 | stack-parser-backtracking | `resolution_exposed` | `improve` | Primary pair verified |
| 2497 | pool-routing-exhaustion | `resolution_irrelevant` | `neutral` | Primary pair verified |
| 2498 | nla-drain-rebalance-storm | `resolution_exposed` | `improve` | Primary pair verified |
| 2500 | flask-migration-console-errors | `resolution_irrelevant` | `neutral` | Primary pair verified |
| 2502 | workflow-history-stall | `metric_neutral` | `neutral` | Primary pair verified |
| 2563 | broker-replication-overload | `metric_neutral` | `neutral` | Primary pair verified |
| 2574 | fts-segment-corruption | `mixed_secondary` | `weak_improve` | Primary pair verified |
| 2590 | empty-metric-zero-fill | `resolution_exposed` | `improve` | Primary pair verified |
| 2592 | audit-logger-config-gap | `resolution_exposed` | `improve` | Primary pair verified |
| 2597 | distributed-login-failures | `mixed_secondary` | `weak_improve` | Primary pair verified |
| 2643 | scanner-native-memory-oom | `metric_neutral` | `neutral` | Primary pair verified |
| 2666 | compliance-query-outage | `metric_neutral` | `neutral` | Primary pair verified |

This table records readiness only. It does not report the DDEval results being generated separately.

## Historical rehabilitation queue

Detailed history and requirements are in
[`gensim-global-1hz-rehabilitation-update-2026-09-21.md`](gensim-global-1hz-rehabilitation-update-2026-09-21.md).

| Episode | Historical evidence class | Missing arms | Admission failure | New evidence required |
|---:|---|---|---|---|
| 2467 | `mixed_secondary` | A and B | Neither archive has a uniquely identifiable alert transition | Fresh matched pair with exact, unique trigger and recovery identities in both arms |
| 2483 | `resolution_exposed` | A; historical B remains supplementary | A's trigger or recovery event was not unique. Operator-provided current status says a later paired replication also failed this condition, but its durable sealing remains outstanding. | New matched pair with independently unique trigger and recovery identities; do not pair a new arm with historical B |
| 2601 | `resolution_irrelevant` | A and B | Both archives contain zero usable logs, traces, metrics, deployments, or profiles | Fresh matched pair with usable diagnostic telemetry and complete archive validation |

Evidence:

- Primary arm admission:
  `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence/ddeval-corpus-prep-continuation-20260910T183856Z/manifests/cohort-plan.md`
- Replication sealing status:
  `/Users/ella.taira/Documents/Obsidian Vault/ella.taira/agent-journal/sessions/2026/08/2026-08-21-pi-90f057e2-0dcc-4b17-ba4c-68e077bbb1e8.md`
- Corrected 2601 archive:
  `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence/archive-2601-corrected-v1-20260910T133205Z/archive-receipts/archive-summary.json`

## Prospective incident-grounded sources

### Read-only audit method

On 2026-09-21, production GS Admin was queried read-only for each episode, blueprint, app-bundle metadata document, and archive. Archives were independently hashed and inspected for unsafe paths, links, runtime entrypoints, Agent configuration, concrete telemetry definitions, and source cadence. Read-only archive copies for 4635/v4 and 4281/v1 were preserved outside Git as primary audit evidence; other temporary downloads were removed. No episode, execution, archive, dataset, repository, or Confluence mutation occurred.

Audit evidence:

- Primary 4635/v4 and 4281/v1 metadata/archive readbacks, with evidence-manifest SHA-256 `9c12527af7f50181a446b2b54c9bbd8aff0d3044fb6ea923eaf5c04fee402e0c`:
  `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence/corpus-expansion-readonly-20260921T192815Z`
- 4474/v1 and 2510/v20 structured read-only audit:
  `/Users/ella.taira/.pi/agent/sessions/--Users-ella.taira-.pi-agent-worktrees-gensim-campaign-core-90f057e2--/subagent-artifacts/b6fcd8de-775e-4c31-b984-65bf90529c30_delegate_0_output.md`
- 3803/v1 and 3704/v1 structured read-only audit:
  `/Users/ella.taira/.pi/agent/sessions/--Users-ella.taira-.pi-agent-worktrees-gensim-campaign-core-90f057e2--/subagent-artifacts/1fe14ff7-a6f5-4830-8298-abab3aab341a_delegate_0_output.md`

All six candidates are first-party `DD_INCIDENT` sources. None embeds a public postmortem URL, so this draft does not invent one. Their current app bundles are Synthtel DES generators that write synthetic telemetry directly to `simulation.db`; none contains an Agent, Helm chart, workload containers, `start.sh`, or `play-episode.sh`.

### Prioritization

| Priority | Episode/version | Incident-grounded scenario | Current evidence | Event feasibility | Main gap | Recommendation |
|---:|---|---|---|---|---|---|
| 1 | 4635/v4 | GCP load-balancer connection drops starve Cutty dialtone submissions and propagate through Koutris and Alerting | Six-service DES with metrics, logs, spans, six monitors, and deterministic verifier pass; Cutty probes modeled at 500 ms | Strong baseline, causal alert ordering, and recovery design | No real workload or Agent emission; sub-second probe behavior exists only in DES | **Advance** to fresh Agent-bearing design |
| 2 | 4474/v1 | Mongo query/cache failure amplifies reducer failures and Kafka consumer lag | Passing DES with 1,111,275 metric rows, 3,129 logs, 15,282 spans, 54 events, and five monitors | Same primary Kafka-lag monitor is gated `ok → alert → ok` | DES-only; incident-era historical metrics were unavailable | **Advance** to fresh Agent-bearing design |
| 3 | 3704/v1 | OrgStore client connection-pool leak causes pty-streaming gRPC deadlines while the server remains healthy | Small DES topology with client pool, wait, gRPC, logs, spans, and healthy-server controls | Four monitors recovered in DES; bounded leak is feasible | Existing pool/tracer signals are synthetic; Agent-controlled high-cadence emission must be designed | **Advance** to fresh Agent-bearing design |
| 4 | 4281/v1 | appDB replica lock contention exhausts Metabase HikariCP pool | Strong PostgreSQL/JMX/APM causal model with logs, spans, six monitors, and deterministic recovery | DES transitions are coherent | Likely producer/check-cadence limited; no event-driven Agent-controlled metric has been identified | **Hold** for cadence proof |
| 5 | 2510/v20 | Consul configuration deadlock stalls trace-indexer Kafka consumption | Rich passing DES with Consul, Kafka, intake, and Elasticsearch telemetry | Primary Kafka-lag monitor intentionally remains alert after cooldown; recovery gates a different monitor | Redesign lifecycle to recover the same preregistered primary monitor; episode also remains `needs-fixup` | **Hold** for lifecycle redesign |
| 6 | 3803/v1 | timeseries-query memory saturation and OOMs under per-org load | Rich DES with metrics, logs, spans, five recovered monitors | Alert/recovery design is plausible | Decisive trace/Kubernetes metrics lack Agent-cadence proof; realistic OOM materialization is expensive and nondeterministic | **Hold** for cadence proof and bounded design |

The revised order reflects deeper materialization and cadence analysis. It supersedes the earlier source-quality-only ranking, but it still does not use treatment outcomes.

### Immutable source identities

| Episode/app-bundle version | Blueprint version | Blueprint archive SHA-256 | App-bundle archive SHA-256 | App-bundle archive key |
|---|---:|---|---|---|
| 4635/v4 | 1 | `731863ec414ec37d5f0b18aa790b4c2a6fbb0cdce55883860f0e2b2b9aa0ac2f` | `237fb7cef83b22b8c2376b1051bb91216789d462a01d1ddb0bb0d62df0a52ef4` | `schema-v1/gensim/episodes/ddincident-2-68299056-9838-5d73-82a5-2d33c6bbc4ab/app-bundles/4/archive.tar` |
| 4474/v1 | 1 | `65d4d241ceabfcc13cf4e62395107056bdf1c3097f265caa1388003a5404a753` | `941f299430331caaf3d8597980ed5f8c1c64534639fb79380a0406e10b079fe0` | `schema-v1/gensim/episodes/ddincident-2-6ce2a9ae-2821-5e53-a9fd-c564c486cf3a/app-bundles/1/archive.tar` |
| 3704/v1 | 1 | `27a934fd2ba1029d573e8e8c5325ae413cece7f0aa36a42e79f105521d6fe9ab` | `8277639807189723565d40d15df60b5dcf2fd445cd682ac8e204557f11c04982` | `schema-v1/gensim/episodes/ddincident-2-86b71942-3ba0-5ab3-b0c4-0d8863d97575/app-bundles/1/archive.tar` |
| 4281/v1 | 1 | `116fba858183f459d70d757d851417a1babf47b42be288ec963455a03d17d993` | `1a40b8fa2b5813ed451d780dfe7fa833b3246168d9f408f5d5cee9cc798038ae` | `schema-v1/gensim/episodes/ddincident-2-f12c6003-e72a-50b4-beb2-2560d061193f/app-bundles/1/archive.tar` |
| 2510/v20 | 2 | `8b4ecf459adc3bd4a6a9bb311d90b2b6cfa220fb75d23dda12fa4dd6f03b8c7f` | `5f5d72f08b6135cb11ac16d6b58b67af0347ee7220b2f59219d72ed145c4d8bf` | `schema-v1/gensim/episodes/trace-indexer-consul-deadlock-us3--consul-config-deadlock/app-bundles/20/archive.tar` |
| 3803/v1 | 1 | `60c974e5b7bbeefbc1de39b8811c74e4c4a276370625c5a2506057d32ace1674` | `6129f78629bd5971d4fd52191c9138a752bd5fcf79b5f637bb864cae38c72cf5` | `schema-v1/gensim/episodes/ddincident-2-73c17502-217b-52e8-915f-ca2c3f0528a3/app-bundles/1/archive.tar` |

### Required materialization contracts

#### 4635/v4 — proposed first design

- Implement bounded GCP load-balancer/Cutty/Propjoe/Koutris/Alerting substitutes.
- Emit concrete Cutty probe success/timeout metrics through an Agent-visible, treatment-sensitive path.
- Preserve the sub-second source behavior without forcing one-second output timestamps in application code.
- Reproduce the causal alert order and recover the same preregistered primary monitor.

#### 4474/v1 — proposed second design

- Implement bounded Mongo, reducer, Kafka, and bundler behavior.
- Reproduce cache-read timeout and upsert-amplification behavior deterministically.
- Emit validated reducer and lag telemetry through the Agent with documented source cadence.
- Preserve the `ok → alert → ok` primary-monitor lifecycle with unique execution identity.

#### 3704/v1 — proposed third design

- Implement a small Go gRPC service with a bounded client connection pool and reversible leak mode.
- Keep the Postgres/server side healthy to preserve the RCA discriminator.
- Add high-cadence non-timestamped client-pool occupancy/wait metrics through DogStatsD/Agent.
- Preserve logs and traces for queueing and `DEADLINE_EXCEEDED`.

#### Held candidates

- **4281/v1:** identify a real Agent-controlled sub-10-second signal before application work. If only PostgreSQL, JMX, Kubernetes, and tracer check intervals remain, classify the new design as metric-neutral or do not materialize it for this experiment.
- **2510/v20:** select one primary monitor and design both exact trigger and recovery for that monitor before materialization. Resolve metric-name drift and the source episode's `needs-fixup` state without rewriting historical versions.
- **3803/v1:** define a deterministic resource-capped memory-pressure mechanism and prove a treatment-sensitive Agent metric. Do not rely on uncontrolled kernel OOM behavior.

## Admission sequence for any future addition

1. Approve one materialization design without looking at DDEval outcomes.
2. Create a new immutable Agent-bearing source version; do not mutate the DES archive.
3. Freeze matched derivatives with private assignment and exact hashes.
4. Run serially with automatic retry disabled.
5. Require unique trigger and recovery event identities in both arms.
6. Require usable diagnostic telemetry.
7. Archive and verify exact immutable S3 objects.
8. Ingest with `force=false`, link explicitly, and independently read back Scenario Store membership.
9. Add the pair to a future dataset version rather than mutating the dataset currently under DDEval.

## Explicit non-actions

- The active `gensim-global-1hz-corpus-matrix.md` was not changed.
- Confluence was not changed.
- The current primary dataset was not changed.
- No DDEval, Bits, judging, or scoring operation was invoked.
- No candidate was published, dispatched, executed, archived, or ingested.
- No DES archive was classified as Agent-bearing evidence.
