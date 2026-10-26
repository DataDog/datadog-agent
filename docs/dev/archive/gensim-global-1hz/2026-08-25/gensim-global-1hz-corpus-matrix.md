# GenSim Global 1 Hz Experiment — Source-Frozen 12-Scenario Corpus and Run Matrix

> **Documentation role: active readiness input.** This preserves scenario-selection and source-provenance evidence. It is not a launch manifest and no listed scenario is eligible for scored execution until the governing design and controller gates are closed.

Status: the 12 core scenarios and three preregistered reserves are source-pinned from live gs-admin with immutable bundle and canonical episode-YAML hashes. Membership hash: `811e898de91ac61a6bb8fdea96aff008e304ca42a9b69c70eb89e0fc6b7499ab`. The four-scenario historical local-GKE pilot selection is recorded in `gensim-global-1hz-pilot-manifest.json`: 2483, 2481, 2643, and 2674. Its bundle output is source-pin evidence only and is not an input to the durable remote-controller campaign. Phase 1 remains blocked on repeated G1b judge calibration and owner approvals; no scenario has a qualifying new matched capture.

## Provenance and selection method

This corpus was selected from the **live gs-admin** catalog, not from locally downloaded bundles:

- Catalog query: `causalgen.state=archived`
- Query result: 241 catalog records; 235 current episode-detail records resolved (six archived IDs were stale at the detail endpoint).
- Latest-bundle screen: 234 usable latest bundles after excluding one `causalgen.do-not-use=true` bundle.
- Quality screen: 96 latest bundles tagged `causalgen.gates=passed`.
- Source review: retrieved the canonical episode YAML and monitor definitions for all 96 passed bundles.

The runbook specifies the `causalgen.state=archived` filter as the starting point for downloadable, known-good bundles: [Running an Episode on GKE with episode-ctl — Step 1](https://datadoghq.atlassian.net/wiki/spaces/~71202001c02fd36efc4735998a938dc4d3e52f/pages/7070975041/Running+an+Episode+on+GKE+with+episode-ctl#Step-1-%E2%80%94-Get-an-episode-bundle).

## Corpus shape

The provisional core shortlist is **12 scenarios**: 7 resolution-exposed candidates, 2 metric-neutral controls, and 3 log-led controls (approximately 58% / 17% / 25%). This intentionally prioritizes scenario quality and credible resolution sensitivity over forcing a nominal 60/20/20 split from a small eligible direct-metric pool.

“Expected 1 Hz helps” is a hypothesis, not a completed check. The G1 preflight must still establish that the cited direct metric is an Agent-owned adjustable signal, rather than a trace-derived, sketch/distribution, producer-scheduled, or independently cadenced metric.

| Cohort | gs-admin episode ID | Scenario | App / service category | Scenario classification | Expected 1 Hz helps | Direct signal or decisive evidence | Latest bundle ID | Source qualification |
|---|---:|---|---|---|---|---|---:|---|
| Resolution-exposed | [2483](https://gs-admin.static-app.us1.staging.dog/episodes/2483) | serialization-regression | events-notification-pipeline; event transformer + notification router | application serialization failure | High — statically eligible | Non-timestamped DogStatsD counts: `event_transformer.payload.encode.exceptions` and `notification_router.job.enqueue.hits` | v3 | `causalgen.gates=passed` |
| Resolution-exposed | [2481](https://gs-admin.static-app.us1.staging.dog/episodes/2481) | stack-parser-backtracking | mobile-analytics-pipeline; JVM/Kafka stream processor | CPU saturation → consumer stall | High — statically eligible | Agent-owned recurring `container.cpu.user` metric rises before the consumer-lag log symptom | v3 | `causalgen.gates=passed` |
| Resolution-exposed | [2498](https://gs-admin.static-app.us1.staging.dog/episodes/2498) | nla-drain-rebalance-storm | metric-context-pipeline; Kafka context rewriter | node drain → consumer rebalance → monitor suppression | Medium — statically eligible | Non-timestamped DogStatsD gauges: `kafka.consumer.lag_seconds` and `mork.intake.unresolved_contexts` | v1 | `causalgen.gates=passed`; prior laptop result is not qualifying evidence |
| Resolution-exposed | [2467](https://gs-admin.static-app.us1.staging.dog/episodes/2467) | cold-cache-stampede | regional-metrics-pipeline; stream processor + entity resolver | cache stampede → stream lag | Medium — statically eligible | Non-timestamped DogStatsD gauge `kafka.estimated_consumer_lag`; trace latency is supplementary and excluded | v3 | `causalgen.gates=passed` |
| Resolution-exposed | [2590](https://gs-admin.static-app.us1.staging.dog/episodes/2590) | empty-metric-zero-fill | cloud-metric-pipeline; metric collector | off-grid zero-fill timing error | High — statically eligible | Non-timestamped DogStatsD count `dd.crawler.vendor_insights.batch_api.metrics.empty` plus zero-fill log | v5 | `causalgen.gates=passed` |
| Resolution-exposed | [2592](https://gs-admin.static-app.us1.staging.dog/episodes/2592) | audit-logger-config-gap | portal-logistics-platform; API + audit/security pipeline | partial-rollout config failure | High — statically eligible | Non-timestamped DogStatsD audit queue-failure and generated-notification counters | v5 | `causalgen.gates=passed` |
| Resolution-exposed | [2563](https://gs-admin.static-app.us1.staging.dog/episodes/2563) | broker-replication-overload | span-ingest-pipeline; Kafka broker + ingest worker | broker rotation → handler exhaustion → consumer lag | Medium — statically eligible | Non-timestamped DogStatsD gauge `kafka.request.handler.avg.idle.pct.rate`; consumer-lag log corroborates | v1 | `causalgen.gates=passed` |
| Metric-neutral | [2502](https://gs-admin.static-app.us1.staging.dog/episodes/2502) | workflow-history-stall | reposync-platform; workflow/replay pipeline | durable workflow-history stall | No expected material benefit | persistent absence of `mirror_destination.pipeline.trigger.count` | v1 | `causalgen.gates=passed` |
| Metric-neutral | [2643](https://gs-admin.static-app.us1.staging.dog/episodes/2643) | scanner-native-memory-oom | stream-processing-platform; scanner + stream shard | gradual native-memory growth and sustained lag | No expected material benefit | durable `kafka.consumer_lag` symptom | v2 | `causalgen.gates=passed` |
| Log-led | [2528](https://gs-admin.static-app.us1.staging.dog/episodes/2528) | cassandra-connectivity-regression | uptime-probe-platform; Cassandra clients + persistence workers | ring maintenance / stale host-pool failure | No expected material benefit | evaluator, persistence-worker, and consumer-lag logs | v1 | `causalgen.gates=passed` |
| Log-led | [2589](https://gs-admin.static-app.us1.staging.dog/episodes/2589) | trial-signup-csrf | trial-signup-platform; edge auth + registration API | CSRF migration/session-token regression | No expected material benefit | `csrf_not_found` and `csrf_mismatch` logs | v5 | `causalgen.gates=passed` |
| Log-led | [2674](https://gs-admin.static-app.us1.staging.dog/episodes/2674) | iceberg-runtime-regression | batch-analytics-platform; JVM batch worker | runtime upgrade → table-format JVM abort | No expected material benefit | batch failure, executor-lost, and exit-code logs | v2 | `causalgen.gates=passed` |

## G1a static source-eligibility audit

G1a is complete for the metric scenarios. The frozen treatment is documented to apply one-second ordinary DogStatsD aggregation/flush and Agent-owned recurring checks; timestamped DogStatsD and trace-derived series are excluded. The source evidence below establishes whether the target series enters one of those treatment paths. G7 remains the separate post-deployment proof that the intended image/config actually produced the expected series spacing.

| ID | Static source classification | Code / image-path evidence | G1a result |
|---:|---|---|---|
| 2483 | ordinary non-timestamped DogStatsD counts | `services/event-transformer/app.py` increments `event_transformer.payload.encode.exceptions`; `services/notification-router/app.py` increments `notification_router.job.enqueue.hits` through `datadog.statsd` | passed |
| 2481 | Agent-owned recurring container metric | The bundle deploys the Agent container and monitors `container.cpu.user`; the frozen treatment changes positive recurring Agent check intervals to one second | passed |
| 2498 | ordinary non-timestamped DogStatsD gauges | `services/context-rewriter-cache/app.py` emits `kafka.consumer.lag_seconds` through `statsd_gauge` during the rebalance path | passed |
| 2467 | ordinary non-timestamped DogStatsD gauge | `services/stream-processor/main.go` emits `kafka.estimated_consumer_lag` through `statsd.Gauge` | passed |
| 2590 | ordinary non-timestamped DogStatsD count | `services/cloud-metrics-collector/app.py` increments `dd.crawler.vendor_insights.batch_api.metrics.empty` through `statsd.increment` | passed |
| 2592 | ordinary non-timestamped DogStatsD counts | `services/portal-gateway/api/metrics.py` and `services/security-alert-engine/app.py` emit the target counters through `statsd.increment` | passed |
| 2563 | ordinary non-timestamped DogStatsD gauge | `services/message-broker-core/main.go` emits `kafka.request.handler.avg.idle.pct.rate` through `statsd.Gauge` | passed |
| 2502 | ordinary non-timestamped DogStatsD count | `services/mirror-destination/main.go` emits `mirror_destination.pipeline.trigger.count` through `dogstats.Incr` | passed; retained as metric-neutral |
| 2643 | producer-scheduled ordinary DogStatsD gauge | `services/event-bus/main.go` emits `kafka.consumer_lag` through `Gauge` on a fixed 10-second ticker | passed as a metric-neutral control; ineligible for the exposed cohort because producer cadence bounds resolution |
| 2528, 2589, 2674 | log-led | The decisive monitor evidence is logs, not a treatment-adjustable metric | not applicable |

## Historical gs-admin evidence — reference only

These are pointers to the most recent archived gs-admin execution record for each source. Their latest statuses are `FAILED` or `MD_ONLY`; they **do not** satisfy any new control/treatment run, archive, Scenario Store, cadence, or DDEval gate below. They are retained only to make source provenance and prior artifacts traceable.

| Episode ID | Latest gs-admin execution ID | Latest status | Historical artifact prefix |
|---:|---|---|---|
| 2483 | `0e50860a-2fa5-4db4-8529-f7c838b343c7` | `FAILED` | `schema-v1/gensim/episodes/events-pipeline-serialization-fault-ir14582--main/executions/0e50860a-2fa5-4db4-8529-f7c838b343c7/` |
| 2481 | `56aa2b2b-b984-407b-8fb1-c2fd12ed7b27` | `FAILED` | `schema-v1/gensim/episodes/rum-processing-lag-regex-backtracking--regex-backtracking/executions/56aa2b2b-b984-407b-8fb1-c2fd12ed7b27/` |
| 2498 | `3d149acf-3059-416f-9b9d-fdb37f633099` | `FAILED` | `schema-v1/gensim/episodes/ctx-rewriter-nla-drain-sticky-assignor--nla-drain-sticky-assignor/executions/3d149acf-3059-416f-9b9d-fdb37f633099/` |
| 2467 | `cfd26c54-b3b6-4e2f-8c8d-5f418b3ce49d` | `FAILED` | `schema-v1/gensim/episodes/pylae-cold-cache-stampede-eu--cold-cache-stampede/executions/cfd26c54-b3b6-4e2f-8c8d-5f418b3ce49d/` |
| 2590 | `45bad1b5-46ae-4145-bf5e-ca4ec31aebc6` | `FAILED` | `schema-v1/gensim/episodes/azure-metric-discrepancy-ir22837--empty-feed-zero-fill/executions/45bad1b5-46ae-4145-bf5e-ca4ec31aebc6/` |
| 2592 | `5bc9391e-e3bf-4120-80e2-9dfd2fd5913f` | `FAILED` | `schema-v1/gensim/episodes/91c154e8-74f3-5994-9d04-0810c521e5be--activity-tracker-singleton-bootstrap-failure/executions/5bc9391e-e3bf-4120-80e2-9dfd2fd5913f/` |
| 2563 | `cf69d25e-8f18-4b3f-8f85-9ab21fc032be` | `FAILED` | `schema-v1/gensim/episodes/kafka-apm-spans-3949-overload-ap1--main/executions/cf69d25e-8f18-4b3f-8f85-9ab21fc032be/` |
| 2502 | `823ddb7a-1908-4d9b-b90c-ec457596d446` | `FAILED` | `schema-v1/gensim/episodes/codesync-temporal-history-overflow--temporal-history-overflow/executions/823ddb7a-1908-4d9b-b90c-ec457596d446/` |
| 2643 | `c0171d3b-0884-415f-9c12-9598de9f6d10` | `FAILED` | `schema-v1/gensim/episodes/native-memory-oom-sds-scanner-proliferation--tenant-rollout-saturation/executions/c0171d3b-0884-415f-9c12-9598de9f6d10/` |
| 2528 | `4b5f2036-25ba-4759-8382-b75d19170dc5` | `FAILED` | `schema-v1/gensim/episodes/synthetics-cassandra-gocql-connectivity-failure--cassandra-gocql-connectivity-failure/executions/4b5f2036-25ba-4759-8382-b75d19170dc5/` |
| 2589 | `760c8c7a-8a5b-44ee-8d9e-22c5fc448dec` | `FAILED` | `schema-v1/gensim/episodes/signup-csrf-token-mismatch-ir22748--csrf-query-promotion-regression/executions/760c8c7a-8a5b-44ee-8d9e-22c5fc448dec/` |
| 2674 | `416b125a-02be-45f0-891e-70415b4b660f` | `FAILED` | `schema-v1/gensim/episodes/spark-jvm-crash-iceberg-incompatibility--worker-version-rollout/executions/416b125a-02be-45f0-891e-70415b4b660f/` |

## New 1 Hz evaluation ledger

All entries below apply to new, matched control/treatment work only. `pending` means no qualifying result exists; it is deliberately different from a historical gs-admin execution.

| ID | G0 live source | G1a static source eligibility | G1b repeated judge calibration | G2 matched run success | G3 verify success | G4 numeric trigger feed-event ID | G5 archive success | G6 Scenario Store ingest success | G7 metric cadence/retrieval proof | G8 pinned-model DDEval success |
|---:|---|---|---|---|---|---|---|---|---|---|
| 2483 | pinned: bundle v3, tar `43f6bd6df078626704d6e8922dbd11eba5c77163121f058d90bea999ae26481d` | passed: ordinary DogStatsD count | pending calibration | pending | pending | pending | pending | pending | pending | pending |
| 2481 | pinned: bundle v3, tar `24537e723a24271437f03317032433d6429d4b3ac94182204eaa24e18131880c` | passed: Agent recurring check | pending calibration | pending | pending | pending | pending | pending | pending | pending |
| 2498 | pinned: bundle v1, tar `f9bfbb47511302ae2a27480e29ed82d354e82f9caf30284d2069a2b16abddc09` | passed: ordinary DogStatsD gauge | pending calibration | pending | pending | pending | pending | pending | pending | pending |
| 2467 | pinned: bundle v3, tar `fb92013fae5f88708cea0885a92177c8fbf8b4d0eceb727e204fa05ade74b5e1` | passed: ordinary DogStatsD gauge | pending calibration | pending | pending | pending | pending | pending | pending | pending |
| 2590 | pinned: bundle v5, tar `b1615aa3fdf01fd8e257c476a8dd8450e98db2fe3118c50fe961fb662c2e589f` | passed: ordinary DogStatsD count | pending calibration | pending | pending | pending | pending | pending | pending | pending |
| 2592 | pinned: bundle v5, tar `9336b9a12e30b97b7da434db97447ee046fbb1d609e2af95c0b72969dd7b31c6` | passed: ordinary DogStatsD count | pending calibration | pending | pending | pending | pending | pending | pending | pending |
| 2563 | pinned: bundle v1, tar `575042fe624e8efcf4b03900f78233d124d864f41faabdcd69d27b5ada9d4453` | passed: ordinary DogStatsD gauge; logs corroborate | pending calibration | pending | pending | pending | pending | pending | pending | pending |
| 2502 | pinned: bundle v1, tar `de471e1e17f97bada54e82461a3d39d1d5ade43136106ad582155591f9ebd497` | passed: durable absence signal; neutral | pending calibration | pending | pending | pending | pending | pending | pending | pending |
| 2643 | pinned: bundle v2, tar `1c73e2cc7048f5d98c087d8bc1ef7ec6ae2b623e60cd67223c71aaa933d39acc` | passed: fixed 10-second producer; neutral | pending calibration | pending | pending | pending | pending | pending | pending | pending |
| 2528 | pinned: bundle v1, tar `693787b3d50e8b2f40b993f31a23dbcfb11f8451b66d324f8d616a246162d7da` | not applicable: log-led | pending calibration | pending | pending | pending | pending | pending | pending | pending |
| 2589 | pinned: bundle v5, tar `2f6014d0d9755fefdead01d963a6fc90b252e9bb399dc6dcb4f5e3e8307d64c0` | not applicable: log-led | pending calibration | pending | pending | pending | pending | pending | pending | pending |
| 2674 | pinned: bundle v2, tar `be17e96fcb2f5701d9d572132382e89176ecb0f118d78ea1a291a041236e8a27` | not applicable: log-led | pending calibration | pending | pending | pending | pending | pending | pending | pending |

## Gate contract and IDs to retain

| Gate | Pass condition | IDs / artifacts to record in this matrix |
|---|---|---|
| P0 — managed platform/auth preflight | The owner-approved `gs-episode-worker → gs-flow/Terrapin` path is available for the intended staging smoke or production capture; worker/gs-flow authentication and the approved tier succeed. Direct laptop `episode-ctl` runs and the local-GKE bundle compiler are source/adapter qualification only, never matched-evidence input. | auth timestamp, Atlas worker request ID, gs-flow job ID, Terrapin tier, namespace/environment, deploy-time `DD_ENV` |
| G0 — source pin | Downloaded source is the exact gs-admin episode and app-bundle revision selected above. | gs-admin episode ID, app-bundle version, archive key, local immutable bundle digest |
| G1a — static source eligibility | Direct causal metrics are classified from source code and the frozen Agent treatment semantics as eligible, excluded, or producer-cadence-limited. | metric names, source class, source-file evidence, treatment-path rationale |
| G1b — repeated judge calibration | The exact Scenario Store ground truth repeatedly accepts the canonical conclusion and rejects symptom-only, wrong-mechanism, and explicit-distractor conclusions under the frozen judges. | calibration artifact, judge versions, repetition count, per-case decisions, owner approval |
| G2 — matched run | The managed `RunEpisodeJob` workflow completes control and treatment with distinct namespaces/environments, equivalent resources/windows, immutable published app-bundle identities, and the intended Agent image in each namespace. Until deployed exact-version pins exist, record the worker-resolved version and the approved immutable latest-version publication receipt. | worker workflow ID, gs-flow job ID, Terrapin tier, namespace, environment tag, gs-admin execution and app-bundle version, result JSON path, timestamps, resource and Agent-image evidence |
| G3 — episode verify | All expected verification checks pass with the deploy-time `DD_ENV`: clean baseline, monitor data, intended alert, logs/traces where applicable, and recovery. | verify-report path and digest; monitor IDs; deploy-time `DD_ENV`; failed-check names if any |
| G4 — trigger identity | Exactly one numeric feed-event ID is captured for the intended alert before archival is allowed. | monitor ID, feed-event ID, alert timestamp, query/lookup evidence |
| G5 — archive | Archive runs only after G2/G3/G4 pass and `run-result.json` has `status=success`, non-null result data/path, and non-empty execution metadata. Read-back must match the local manifest. | archive prefix, metadata path and SHA-256, object count, archival execution ID, read-back receipt |
| G6 — Scenario Store | Compact projection serializes below the 128 KiB protobuf limit before submission and ingestion succeeds. | Scenario Store UUID, serialized payload bytes, response ID, projection schema/version |
| G7 — deployed cadence and retrieval | The deployed control/treatment Agent images/configurations are read back and their cadence is proven over the required windows; evaluation retrieval uses the intended interval/raw-data behavior. | control/treatment image digests, effective config, metric query IDs, time windows, cadence-spacing artifacts, retrieval request/result IDs |
| G8 — DDEval | The pinned model completes canonical and wrong-RCA calibration plus the matched investigation. | evaluation run ID, model/version, prompt/dataset revision, tool-call evidence, score/result artifact |

## Prior-failure prevention policy

Every condition below is a blocking control, not a best-effort reminder. A blocked scenario remains `pending` or `failed` in the ledger and cannot advance to the next gate.

| Prior failure class | Mandatory preventive control | Blocking condition |
|---|---|---|
| Laptop sleep or local execution drift | Use the managed `gs-episode-worker → gs-flow/Terrapin → GenSim` path for all scored pilot and confirmatory captures. The local compiler may validate source pins and generate a GKE-topology prototype, but it cannot become a managed app bundle or matched-evidence input. | No managed worker/workflow and gs-flow job identity, or a scored result relies on a local laptop process. |
| Wrong cluster, expired access, or mismatched environment | Run P0 before every batch; confirm GKE context and derive the verify environment from the deployed Agent's `DD_ENV`, rather than supplying a guessed `--env`. | Auth/context fails, or the verify environment differs from the deploy-time Agent environment. |
| Non-comparable control/treatment resources or Agent images | Pin one source revision; preserve equivalent workload resources and windows; record and inspect control/treatment Agent-image digests before playing the episode. | Source, resource, window, or Agent image cannot be proven for both sides. |
| Alert event absent, duplicated, or non-numeric | Capture exactly one numeric feed-event ID after the intended monitor fires; perform the lookup before archival. | Zero or multiple candidate events, non-numeric event ID, or monitor mismatch. |
| Archive created from a failed or empty result | Archive wrapper validates successful status, non-null result path/data, and non-empty execution metadata before it can invoke archival. | Any validation field is missing or failed; no archive call is made. |
| Archive is present but cannot support replay/verification | Require manifest completeness, metadata digest, object count, and an exact read-back receipt. | Read-back differs from the manifest or required execution/telemetry fields are absent. |
| Scenario Store protobuf exceeds 128 KiB | Serialize the compact projection first and reject it locally at or above 131,072 bytes; retain large immutable manifests in S3 only. | Serialized projection is `>= 131,072` bytes or response lacks a Scenario Store UUID. |
| Missing GKE log evidence | Verify logs using the deploy-time environment and validate DaemonSet/container-runtime log collection before selecting a log-led control. | `logs_exist` or log-source preflight fails. |
| Claimed 1 Hz benefit without a controllable signal | G1a classifies the source from code and treatment semantics; G7 proves deployed cadence separately for control and treatment. Trace, sketch/distribution, and independently scheduled metrics do not qualify as exposed evidence. | Static eligibility or deployed-cadence proof is absent. |
| DDEval conclusion without a complete dataset | Gate DDEval on successful archive, Scenario Store ingest, metric-retrieval proof, and judge calibration. | Any upstream evidence ID is missing or calibration fails. |

## Future-corpus catalog and source-frozen reserves

The complete 96-scenario passed-gate decision ledger is preserved in [gensim-global-1hz-corpus-rejection-ledger.json](gensim-global-1hz-corpus-rejection-ledger.json), generated from [gensim-global-1hz-future-candidate-catalog.md](gensim-global-1hz-future-candidate-catalog.md). There is no eighth high-confidence resolution-exposed case. Episode 2597 is therefore a preregistered mixed-secondary fallback, not a silently promoted high-confidence case; if substituted, its results remain labeled and separately reported.

| Episode ID | Scenario | App | Candidate role | Pinned source |
|---:|---|---|---|---|
| [2597](https://gs-admin.static-app.us1.staging.dog/episodes/2597) | distributed-login-failures | identity-platform | mixed-secondary resolution-exposed fallback | bundle v5, tar `bb5a7ad9e41e1ae9e0712ba5afa1115ff7b62632253eb92462748cba978a6b56` |
| [2666](https://gs-admin.static-app.us1.staging.dog/episodes/2666) | compliance-query-outage | compliance-platform | high-confidence metric-neutral reserve | bundle v1, tar `333aeacea76c380c71fa912886e56b33dfb6ad8ffd2734605e92cc2d82e14256` |
| [2684](https://gs-admin.static-app.us1.staging.dog/episodes/2684) | archive-credential-expiry | stream-pipeline-platform | high-confidence log-led reserve | bundle v1, tar `fb9f0f9df0513e95ca251613d16d3993808455c7018d18f3137f6827d9ddaaef` |

A reserve may replace a core scenario only for a preregistered pre-investigation infrastructure or qualification failure. Completed semantic investigations are never replaceable.
