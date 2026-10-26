# GenSim global 1 Hz outcome-blind candidate catalog

> **Active source-screening record.** Admission validates provenance, CausalGen quality, runnable ground truth, telemetry class, and capability-smoke readiness. It does not require or predict a 1 Hz benefit.

Snapshot SHA-256: `97177cddd5d3c66721eec5ae14d0648207f661287f326492226cb48f9b354539`. GS Admin returned 241 archived episodes; 96 latest bundles passed CausalGen gates.

## Admission policy

Admitted scenarios may help, be neutral, or regress under 1 Hz. Existing RCA improvement, a 15-second counterfactual, a short incident, and `scenario.yaml.rca_causal_chain` are not source-admission gates. They are capability, execution, analysis, or separate archive-compatibility concerns.

The score panel includes every admitted resolution-exposed, metric-neutral, and mixed-secondary source, plus a deterministic SHA-256-ranked sample of resolution-irrelevant sources. Selection does not inspect capability outcomes or observed scores.

## Deterministic score panel

| Episode | Scenario | App | Admission class | Capability | Static evidence |
|---|---|---|---|---|---|
| [2467](https://gs-admin.static-app.us1.staging.dog/episodes/2467) | cold-cache-stampede | regional-metrics-pipeline | `mixed_secondary` | `not_run` | Per-request resolver counters can gain finer buckets, but the monitored consumer-lag gauge is emitted every 15 seconds and no cache-miss/load metric is established. |
| [2481](https://gs-admin.static-app.us1.staging.dog/episodes/2481) | stack-parser-backtracking | mobile-analytics-pipeline | `resolution_exposed` | `not_run` | Agent-owned recurring `container.cpu.user` metric. |
| [2483](https://gs-admin.static-app.us1.staging.dog/episodes/2483) | serialization-regression | events-notification-pipeline | `resolution_exposed` | `passed_nonconfirmatory_mandatory_1hz_capability` | Non-timestamped DogStatsD counts in event-transformer and notification-router. |
| [2497](https://gs-admin.static-app.us1.staging.dog/episodes/2497) | pool-routing-exhaustion | analytics-platform-db | `resolution_irrelevant` | `not_run` | logs |
| [2498](https://gs-admin.static-app.us1.staging.dog/episodes/2498) | nla-drain-rebalance-storm | metric-context-pipeline | `resolution_exposed` | `not_run` | Non-timestamped DogStatsD gauges in context-rewriter services. |
| [2500](https://gs-admin.static-app.us1.staging.dog/episodes/2500) | flask-migration-console-errors | alert-console-platform | `resolution_irrelevant` | `not_run` | logs |
| [2502](https://gs-admin.static-app.us1.staging.dog/episodes/2502) | workflow-history-stall | reposync-platform | `metric_neutral` | `not_run` | Non-timestamped DogStatsD count; durable workflow stall makes it neutral. |
| [2563](https://gs-admin.static-app.us1.staging.dog/episodes/2563) | broker-replication-overload | span-ingest-pipeline | `metric_neutral` | `not_run` | Causal broker gauges are emitted together every 10 seconds; 1 Hz improves timestamp precision but adds no source samples or ordering evidence. |
| [2574](https://gs-admin.static-app.us1.staging.dog/episodes/2574) | fts-segment-corruption | log-search-platform | `mixed_secondary` | `not_run` | Non-timestamped DogStatsD accuracy gauge, but logs and persistent corruption dominate. |
| [2590](https://gs-admin.static-app.us1.staging.dog/episodes/2590) | empty-metric-zero-fill | cloud-metric-pipeline | `resolution_exposed` | `not_run` | Non-timestamped DogStatsD count in cloud-metrics-collector. |
| [2592](https://gs-admin.static-app.us1.staging.dog/episodes/2592) | audit-logger-config-gap | portal-logistics-platform | `resolution_exposed` | `not_run` | Non-timestamped DogStatsD counts in portal-gateway and security-alert-engine. |
| [2597](https://gs-admin.static-app.us1.staging.dog/episodes/2597) | distributed-login-failures | identity-platform | `mixed_secondary` | `not_run` | Non-timestamped DogStatsD count, but authentication logs dominate the investigation. |
| [2601](https://gs-admin.static-app.us1.staging.dog/episodes/2601) | cloud-control-cascade | edge-observability-stack | `resolution_irrelevant` | `not_run` | logs |
| [2643](https://gs-admin.static-app.us1.staging.dog/episodes/2643) | scanner-native-memory-oom | stream-processing-platform | `metric_neutral` | `not_run` | Producer emits a DogStatsD gauge on a fixed 10-second ticker; neutral only. |
| [2666](https://gs-admin.static-app.us1.staging.dog/episodes/2666) | compliance-query-outage | compliance-platform | `metric_neutral` | `not_run` | Compactor gauge is emitted on a fixed 30-second ticker; neutral only. |

## Admitted population

| Episode | Scenario | Admission class | Panel |
|---|---|---|---|
| [2100](https://gs-admin.static-app.us1.staging.dog/episodes/2100) | order-policy-flag-deletion | `resolution_irrelevant` | no |
| [2455](https://gs-admin.static-app.us1.staging.dog/episodes/2455) | lookup-miss-spike | `resolution_irrelevant` | no |
| [2465](https://gs-admin.static-app.us1.staging.dog/episodes/2465) | edge-pipeline-outage | `resolution_irrelevant` | no |
| [2467](https://gs-admin.static-app.us1.staging.dog/episodes/2467) | cold-cache-stampede | `mixed_secondary` | yes |
| [2473](https://gs-admin.static-app.us1.staging.dog/episodes/2473) | flag-schema-migration | `resolution_irrelevant` | no |
| [2479](https://gs-admin.static-app.us1.staging.dog/episodes/2479) | workspace-init-stall | `resolution_irrelevant` | no |
| [2481](https://gs-admin.static-app.us1.staging.dog/episodes/2481) | stack-parser-backtracking | `resolution_exposed` | yes |
| [2483](https://gs-admin.static-app.us1.staging.dog/episodes/2483) | serialization-regression | `resolution_exposed` | yes |
| [2487](https://gs-admin.static-app.us1.staging.dog/episodes/2487) | transition-consumer-lag | `resolution_irrelevant` | no |
| [2492](https://gs-admin.static-app.us1.staging.dog/episodes/2492) | feature-flag-rollout-outage | `resolution_irrelevant` | no |
| [2495](https://gs-admin.static-app.us1.staging.dog/episodes/2495) | quota-lock-cascade | `resolution_irrelevant` | no |
| [2497](https://gs-admin.static-app.us1.staging.dog/episodes/2497) | pool-routing-exhaustion | `resolution_irrelevant` | yes |
| [2498](https://gs-admin.static-app.us1.staging.dog/episodes/2498) | nla-drain-rebalance-storm | `resolution_exposed` | yes |
| [2499](https://gs-admin.static-app.us1.staging.dog/episodes/2499) | route-exclusion-mismatch | `resolution_irrelevant` | no |
| [2500](https://gs-admin.static-app.us1.staging.dog/episodes/2500) | flask-migration-console-errors | `resolution_irrelevant` | yes |
| [2501](https://gs-admin.static-app.us1.staging.dog/episodes/2501) | shard-partitioner-oom-cell | `resolution_irrelevant` | no |
| [2502](https://gs-admin.static-app.us1.staging.dog/episodes/2502) | workflow-history-stall | `metric_neutral` | yes |
| [2505](https://gs-admin.static-app.us1.staging.dog/episodes/2505) | replay-clock-drift | `resolution_irrelevant` | no |
| [2506](https://gs-admin.static-app.us1.staging.dog/episodes/2506) | deserialization-regression | `resolution_irrelevant` | no |
| [2507](https://gs-admin.static-app.us1.staging.dog/episodes/2507) | metadata-memory-pressure | `resolution_irrelevant` | no |
| [2512](https://gs-admin.static-app.us1.staging.dog/episodes/2512) | tls-library-fleet-exposure | `resolution_irrelevant` | no |
| [2513](https://gs-admin.static-app.us1.staging.dog/episodes/2513) | silence-pipeline-cascade | `resolution_irrelevant` | no |
| [2519](https://gs-admin.static-app.us1.staging.dog/episodes/2519) | catalog-replica-staleness | `resolution_irrelevant` | no |
| [2522](https://gs-admin.static-app.us1.staging.dog/episodes/2522) | onboarding-access-redirect | `resolution_irrelevant` | no |
| [2523](https://gs-admin.static-app.us1.staging.dog/episodes/2523) | false-credential-warning-regression | `resolution_irrelevant` | no |
| [2525](https://gs-admin.static-app.us1.staging.dog/episodes/2525) | metadata-growth-timeout | `resolution_irrelevant` | no |
| [2527](https://gs-admin.static-app.us1.staging.dog/episodes/2527) | cnab-runtime-incompatibility | `resolution_irrelevant` | no |
| [2528](https://gs-admin.static-app.us1.staging.dog/episodes/2528) | cassandra-connectivity-regression | `resolution_irrelevant` | no |
| [2530](https://gs-admin.static-app.us1.staging.dog/episodes/2530) | handle-normalization-regression | `resolution_irrelevant` | no |
| [2533](https://gs-admin.static-app.us1.staging.dog/episodes/2533) | unicode-metadata-scheduler-outage | `resolution_irrelevant` | no |
| [2536](https://gs-admin.static-app.us1.staging.dog/episodes/2536) | null-offset-corruption | `resolution_irrelevant` | no |
| [2541](https://gs-admin.static-app.us1.staging.dog/episodes/2541) | shard-pool-collapse | `resolution_irrelevant` | no |
| [2542](https://gs-admin.static-app.us1.staging.dog/episodes/2542) | protobuf-field-regression | `resolution_irrelevant` | no |
| [2546](https://gs-admin.static-app.us1.staging.dog/episodes/2546) | client-callback-regression | `resolution_irrelevant` | no |
| [2548](https://gs-admin.static-app.us1.staging.dog/episodes/2548) | credential-repo-sync-flood | `resolution_irrelevant` | no |
| [2549](https://gs-admin.static-app.us1.staging.dog/episodes/2549) | vitals-ingestor-segfault | `resolution_irrelevant` | no |
| [2550](https://gs-admin.static-app.us1.staging.dog/episodes/2550) | cross-region-tag-join | `resolution_irrelevant` | no |
| [2551](https://gs-admin.static-app.us1.staging.dog/episodes/2551) | controller-backup-disconnect | `resolution_irrelevant` | no |
| [2563](https://gs-admin.static-app.us1.staging.dog/episodes/2563) | broker-replication-overload | `metric_neutral` | yes |
| [2564](https://gs-admin.static-app.us1.staging.dog/episodes/2564) | cross-store-segment-merge | `resolution_irrelevant` | no |
| [2565](https://gs-admin.static-app.us1.staging.dog/episodes/2565) | replica-connection-saturation | `resolution_irrelevant` | no |
| [2566](https://gs-admin.static-app.us1.staging.dog/episodes/2566) | entity-pipeline-stall | `resolution_irrelevant` | no |
| [2569](https://gs-admin.static-app.us1.staging.dog/episodes/2569) | policy-rules-update-failure | `resolution_irrelevant` | no |
| [2573](https://gs-admin.static-app.us1.staging.dog/episodes/2573) | privatelink-evaluation-delay | `resolution_irrelevant` | no |
| [2574](https://gs-admin.static-app.us1.staging.dog/episodes/2574) | fts-segment-corruption | `mixed_secondary` | yes |
| [2575](https://gs-admin.static-app.us1.staging.dog/episodes/2575) | catalog-state-lag | `resolution_irrelevant` | no |
| [2576](https://gs-admin.static-app.us1.staging.dog/episodes/2576) | webhook-routing-outage | `resolution_irrelevant` | no |
| [2577](https://gs-admin.static-app.us1.staging.dog/episodes/2577) | interval-alignment-regression | `resolution_irrelevant` | no |
| [2580](https://gs-admin.static-app.us1.staging.dog/episodes/2580) | workflow-cancellation | `resolution_irrelevant` | no |
| [2586](https://gs-admin.static-app.us1.staging.dog/episodes/2586) | trace-sampling-drift | `resolution_irrelevant` | no |
| [2587](https://gs-admin.static-app.us1.staging.dog/episodes/2587) | worker-inline-migration-regression | `resolution_irrelevant` | no |
| [2588](https://gs-admin.static-app.us1.staging.dog/episodes/2588) | legacy-ring-outage | `resolution_irrelevant` | no |
| [2589](https://gs-admin.static-app.us1.staging.dog/episodes/2589) | trial-signup-csrf | `resolution_irrelevant` | no |
| [2590](https://gs-admin.static-app.us1.staging.dog/episodes/2590) | empty-metric-zero-fill | `resolution_exposed` | yes |
| [2591](https://gs-admin.static-app.us1.staging.dog/episodes/2591) | dict-index-corruption | `resolution_irrelevant` | no |
| [2592](https://gs-admin.static-app.us1.staging.dog/episodes/2592) | audit-logger-config-gap | `resolution_exposed` | yes |
| [2594](https://gs-admin.static-app.us1.staging.dog/episodes/2594) | invalid-push-credential | `resolution_irrelevant` | no |
| [2596](https://gs-admin.static-app.us1.staging.dog/episodes/2596) | sds-scrubbing-silent-failure | `resolution_irrelevant` | no |
| [2597](https://gs-admin.static-app.us1.staging.dog/episodes/2597) | distributed-login-failures | `mixed_secondary` | yes |
| [2599](https://gs-admin.static-app.us1.staging.dog/episodes/2599) | legacy-merge-regression | `resolution_irrelevant` | no |
| [2600](https://gs-admin.static-app.us1.staging.dog/episodes/2600) | scope-reconciliation-outage | `resolution_irrelevant` | no |
| [2601](https://gs-admin.static-app.us1.staging.dog/episodes/2601) | cloud-control-cascade | `resolution_irrelevant` | yes |
| [2609](https://gs-admin.static-app.us1.staging.dog/episodes/2609) | retention-shard-gap | `resolution_irrelevant` | no |
| [2610](https://gs-admin.static-app.us1.staging.dog/episodes/2610) | stats-consumer-lag | `resolution_irrelevant` | no |
| [2611](https://gs-admin.static-app.us1.staging.dog/episodes/2611) | calendar-rollup-regression | `resolution_irrelevant` | no |
| [2612](https://gs-admin.static-app.us1.staging.dog/episodes/2612) | auth-tag-regression | `resolution_irrelevant` | no |
| [2618](https://gs-admin.static-app.us1.staging.dog/episodes/2618) | metric-tag-suffix-regression | `resolution_irrelevant` | no |
| [2620](https://gs-admin.static-app.us1.staging.dog/episodes/2620) | usage-intake-gap | `resolution_irrelevant` | no |
| [2621](https://gs-admin.static-app.us1.staging.dog/episodes/2621) | host-handle-leak | `resolution_irrelevant` | no |
| [2625](https://gs-admin.static-app.us1.staging.dog/episodes/2625) | training-artifact-leak | `resolution_irrelevant` | no |
| [2626](https://gs-admin.static-app.us1.staging.dog/episodes/2626) | metadata-cleanup-regression | `resolution_irrelevant` | no |
| [2629](https://gs-admin.static-app.us1.staging.dog/episodes/2629) | redirect-validation-regression | `resolution_irrelevant` | no |
| [2631](https://gs-admin.static-app.us1.staging.dog/episodes/2631) | sdk-session-leak | `resolution_irrelevant` | no |
| [2633](https://gs-admin.static-app.us1.staging.dog/episodes/2633) | source-type-collision | `resolution_irrelevant` | no |
| [2634](https://gs-admin.static-app.us1.staging.dog/episodes/2634) | document-renderer-cert-outage | `resolution_irrelevant` | no |
| [2643](https://gs-admin.static-app.us1.staging.dog/episodes/2643) | scanner-native-memory-oom | `metric_neutral` | yes |
| [2655](https://gs-admin.static-app.us1.staging.dog/episodes/2655) | schema-privilege-failure | `resolution_irrelevant` | no |
| [2657](https://gs-admin.static-app.us1.staging.dog/episodes/2657) | sketch-double-count | `resolution_irrelevant` | no |
| [2659](https://gs-admin.static-app.us1.staging.dog/episodes/2659) | narrative-status-mismatch | `resolution_irrelevant` | no |
| [2662](https://gs-admin.static-app.us1.staging.dog/episodes/2662) | tls-cert-fqdn-mismatch | `resolution_irrelevant` | no |
| [2665](https://gs-admin.static-app.us1.staging.dog/episodes/2665) | producer-config-build-failure | `resolution_irrelevant` | no |
| [2666](https://gs-admin.static-app.us1.staging.dog/episodes/2666) | compliance-query-outage | `metric_neutral` | yes |
| [2672](https://gs-admin.static-app.us1.staging.dog/episodes/2672) | replica-routing-drift | `resolution_irrelevant` | no |
| [2674](https://gs-admin.static-app.us1.staging.dog/episodes/2674) | iceberg-runtime-regression | `resolution_irrelevant` | no |
| [2680](https://gs-admin.static-app.us1.staging.dog/episodes/2680) | route-mismatch-outage | `resolution_irrelevant` | no |
| [2681](https://gs-admin.static-app.us1.staging.dog/episodes/2681) | access-policy-bypass | `resolution_irrelevant` | no |
| [2683](https://gs-admin.static-app.us1.staging.dog/episodes/2683) | activation-store-divergence | `resolution_irrelevant` | no |
| [2684](https://gs-admin.static-app.us1.staging.dog/episodes/2684) | archive-credential-expiry | `resolution_irrelevant` | no |

## Blocked source defects

| Episode | Scenario | Reasons |
|---|---|---|
| [1972](https://gs-admin.static-app.us1.staging.dog/episodes/1972) | merle-koutris-deployment-failure | source_screening_defect |
| [1980](https://gs-admin.static-app.us1.staging.dog/episodes/1980) | premature-cache-invalidation | source_screening_defect |
| [1988](https://gs-admin.static-app.us1.staging.dog/episodes/1988) | stale-grpc-connections | source_screening_defect |
| [2029](https://gs-admin.static-app.us1.staging.dog/episodes/2029) | mortar-lock-race-condition | source_screening_defect |
| [2033](https://gs-admin.static-app.us1.staging.dog/episodes/2033) | catalog-snapshot-overrun | source_screening_defect |
| [2078](https://gs-admin.static-app.us1.staging.dog/episodes/2078) | quantile-merge-cardinality | source_screening_defect |
| [2156](https://gs-admin.static-app.us1.staging.dog/episodes/2156) | delete-order-race | source_screening_defect |
| [2578](https://gs-admin.static-app.us1.staging.dog/episodes/2578) | shard-query-saturation | source_screening_defect |

## Interpretation

Report results by stratum and for the complete frozen panel. Keep every semantically valid completed result, including null and negative effects. Replace only preregistered infrastructure-invalid captures, without observing arm outcomes.
