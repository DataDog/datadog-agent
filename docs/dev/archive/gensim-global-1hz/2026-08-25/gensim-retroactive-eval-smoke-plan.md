# GenSim Retroactive Bits AI SRE Smoke Test

> **Documentation role: historical qualification evidence.** This preserves prior archive/Scenario Store/DDEval smoke design and failure analysis. It is not the current campaign runbook and does not authorize a scored capture.

## Objective

Complete one end-to-end GenSim smoke that proves Bits AI SRE can investigate a historical incident after the live workload has stopped. The smoke is successful only when the captured monitor alert has a real Datadog feed-event ID, that exact event is retrievable from the immutable `evp/feed` archive, Scenario Store preserves the same ID, Bits produces an investigation in archive mode, and service-side judges evaluate the investigation.

This plan is a qualification runbook, not a dataset-scale experiment. It prevents a successful workload run or a positive archive row count from being mistaken for a replayable eval scenario.

## Provenance and failure being prevented

Episode 2666 exposed a trigger-identity gap:

- `/Users/ella.taira/episode-bundles/episode-2666-v1-run/run-result.json` recorded monitor `313352514` and alert timestamp `2026-08-13T13:02:57Z`, but no `event_id` or `alert_url`.
- `/Users/ella.taira/episode-bundles/evidence/episode-2666-treatment/ep2666-1hz-20260813-083315/archive-production-bucket.log` shows that Sims Archiver successfully wrote 605 `evp/feed` rows. The event feed existed, but the metadata did not identify the trigger row.
- `domains/ai_platform/apps/eval-data-portal/internal/ingestion/parser.go::buildAgentContext` generates a UUID when `metadata.execution.alert.event_id` and the `event_id` query parameter in `alert_url` are both absent.
- `domains/chatbot/libs/ddapi/eventquery/feed_event.go::GetFeedEventByID` queries `@evt.id:<event_id>`.
- `domains/chatbot/libs/ai/copilot/monitor.go` returns `ErrFeedEventNotFound` after three archive-mode attempts because an immutable archive cannot acquire a missing event later.
- The episode 2666 projection consequently used placeholder UUID `cea9b1f2-caf8-42b3-940d-f1fb85f25748`; all three shadow eval iterations failed before an investigation began.

The smoke must fail closed before archival, ingestion, or paid eval whenever this linkage is absent.

## Scope

### In scope

- one maintained GenSim episode with usable ground truth;
- one successful live execution and monitor transition;
- canonical managed execution when available;
- immutable Sims Archiver output;
- production Eval Data Portal dry-run and `force:false` ingestion;
- Scenario Store load-only qualification;
- one explicitly model-pinned paid investigation;
- service-side investigation and judge evidence;
- cleanup after evidence is retained.

### Out of scope

- comparing control and treatment arms;
- proving that 1 Hz metrics improve an investigation;
- dataset-scale scenario selection;
- modifying an existing immutable archive in place;
- fabricating an event ID or using a generated UUID as a substitute;
- using a live monitor as a permanent dependency for retroactive replay;
- rerunning paid investigations to work around local result-blob authorization.

## Selected smoke candidates

Episode **2589** v1, `trial-signup-csrf`, failed fresh local qualification. In environment `episode-2589-v1-run-20260814-145144`, baseline monitor `313622253` reached `OK` in 30 seconds and all disruption calls succeeded, but the monitor remained `OK` for the full 600-second alert gate. The interrupted run was torn down; its namespace, cluster RBAC, and monitors `313622253` and `313622254` are absent. Do not verify it as qualifying evidence, archive it, ingest it, or run DDEval. Preserve the immutable v1 archive and historical result only as provenance.

The next bounded local qualification fan-out uses two different archived bundles:

```text
episode 2576 v1: webhook-routing-outage
episode 2643 v1: scanner-native-memory-oom
```

This fan-out follows Confluence page `7070975041`, version 12, "Running Multiple Episodes at Once": use a unique namespace per run, set `--env` to that same unique value, run only different bundles concurrently, and write each terminal's output to its own namespace-qualified log. The rendered service-image sets are disjoint, so their fixed `:latest` tags do not collide. Do not launch a second concurrent copy of either bundle.

Selection evidence:

- episode 2576's immutable v1 archive SHA-256 is `acd36d7bee41e47c76448ac5b850330c7b6880e62f9b022bbb651c8242b0fbfd`; two historical verifier results passed with 634 active metrics, approximately 18,900 logs, traces, five deployment events, and a verified `payload_transform_throughput_low` alert;
- episode 2576 uses direct reversible config/admin actions, but its symptom is a log alert, so backend receipt of the exact trigger logs before disruption remains a hard gate;
- episode 2643's immutable v1 archive SHA-256 is `1c73e2cc7048f5d98c087d8bc1ef7ec6ae2b623e60cd67223c71aaa933d39acc`; its historical `stream_shard_consumer_lag` metric alert occurred 192 seconds after disruption and verification passed with 822 active metrics;
- episode 2643 historically produced 3,103,839 logs and deliberately drives native-memory/OOM pressure, so run only this two-scenario fan-out, watch for broad cluster failure, and fail closed if unrelated services degrade; and
- `/Users/ella.taira/episode-bundles/validate_parallel_treatments.py` requires both working copies to use the approved immutable Agent digest, enable the 1 Hz toggle, exclude NTP, preserve log/containerd access and safe networking, keep scenario/episode/player bytes unchanged, and differ from their bases only in the two Agent chart files.

Historical archived state is discovery evidence only. Neither local run is canonical, and neither can be archived or evaluated unless `episode-ctl verify` passes and a real trigger feed-event ID is retained. Never substitute a generated UUID.

Rejected or deferred candidates:

- **2498 `nla-drain-rebalance-storm`:** the local run printed `EPISODE PASSED`, but `episode-ctl verify` found no logs; execution timing was corrupted, metadata was incomplete, and no supported feed-event identity was retained.
- **2528 `cassandra-connectivity-regression`:** remains unsuitable for the full retroactive-eval identity contract.
- **2589 `trial-signup-csrf`:** fresh treatment monitor `313622253` remained `OK` for the full disruption gate; the interrupted run was torn down and is nonqualifying.
- **2666 `compliance-query-outage`:** useful 1 Hz plumbing evidence, but its projected placeholder UUID failed every crawler attempt because the real event ID was absent.
- **2674 `iceberg-runtime-regression`:** the local rollout timed out, the symptom monitor remained `OK` for the full disruption gate, step pods failed, and cooldown coincided with a broad multi-service outage; the interrupted capture was torn down and must not be archived.
- **2600 `scope-reconciliation-outage` and 2563 `broker-replication-overload`:** both are abandoned as primary candidates. Their log alerts require node-local container-log coverage, and static custom-Agent replica counts proved scheduling-dependent. Building adaptive log collection would test unrelated infrastructure rather than the 1 Hz metric hypothesis.

The current prepared local metric-alert candidate is **2483 v3, `serialization-regression`**. Its symptom and root-cause signals are ordinary non-timestamped DogStatsD event counts, its historical v3 run completed `OK → Alert → OK` and passed verification, and its runtime services, monitor query, thresholds, phase durations, disruption, and recovery remain byte-identical in the derivative. The derivative uses one service-routed custom Agent Deployment without container-log collection. It still requires a fresh strict qualification and numeric trigger/recovery feed-event capture before archival. Source archive SHA-256: `43f6bd6df078626704d6e8922dbd11eba5c77163121f058d90bea999ae26481d`; validator: `/Users/ella.taira/episode-bundles/global-1hz-selected-sources/validate_prepared_2483_1hz.py`.

For future screening, require `causalgen.state=archived`, historical successful alert and recovery transitions, a clean verifier, sustained selective symptoms, bounded reversible recovery, monitor/source-telemetry alignment, and pre-disruption backend telemetry. Reject ignored rollout/step errors, transient failures that age out of the monitor window, broad service refusal, a monitor that reports `OK` while services remain unhealthy, or any interrupted run. Keep these operational gates separate from later feed-event identity, judge-quality, and canonical managed-lineage gates.

The 2026-08-14 local-GKE episode 2600 and 2563 attempts add a mandatory producer-side log gate. Their custom Agents reported `LogsProcessed: 0` and repeated kubelet connectivity failures. Episode 2600 emitted the expected error records in Kubernetes, but the normal Datadog logs backend returned zero records and its log monitor remained `OK`; episode 2563 had the same Agent failure and was stopped. These runs are failed infrastructure qualifications and must not be archived, ingested, or evaluated. Before any future log-alert candidate starts baseline, require one ready Agent per workload node, healthy kubelet discovery, active container log sources, an increasing processed-log count, and a deterministic application log retrievable from the normal backend under the exact monitor tags. Metric-alert success is not evidence that this log gate passes because DogStatsD does not depend on container-log discovery.

## Identities to retain

Record these identifiers in one evidence manifest as they become available:

1. episode, blueprint, app-bundle, and source revisions;
2. gs-admin execution ID and gs-flow/Terrapin identities for a canonical run;
3. environment ID and start/end timestamps;
4. monitor ID, real feed-event ID, and alert timestamp;
5. Sims Archiver workflow/run IDs and immutable archive root;
6. Scenario Store external ID and UUID;
7. DDEval run name and Temporal workflow/run IDs;
8. per-iteration investigation job IDs;
9. LLMObs experiment ID;
10. crawler application and deployed version;
11. investigator and judge model identities.

Never record credentials, bearer tokens, presigned URLs, kubeconfigs, or secret values.

## Non-negotiable acceptance contract

The following identity chain must be exact:

```text
live Datadog monitor transition
  -> real numeric feed-event ID
  -> run-result alert.event_id
  -> metadata.json execution.alert.event_id
  -> matching @evt.id row in archived evp/feed
  -> Scenario Store attributes.event_id
  -> Scenario Store agent_context.metadata.investigation_event_id
  -> crawler archive-mode request event_id
```

Every occurrence must contain the same real event ID. Reject empty values, environment names, generated UUIDs, or mismatches.

A successful smoke additionally requires:

- the workload and disruption complete as intended;
- the archive has no unexplained required-track failures;
- archive-backed tools can retrieve the incident telemetry;
- Bits produces a non-empty investigation rather than failing during trigger resolution;
- service-side reporting shows the investigation was judged;
- the result is interpreted from authoritative service-side evidence if local blob hydration is unavailable.

The judge score does not need to be high for a plumbing smoke. A low score is valid only when an actual investigation was produced and judged.

## Step-by-step procedure

### 1. Select and freeze the smoke candidate

Choose one maintained episode whose monitor, fault, and ground truth are deterministic. Prefer a canonical gs-admin-managed episode over a local `episode-ctl` adaptation.

Before execution, record immutable source identities and verify:

- for a log monitor, Agent placement covers every workload node, kubelet discovery is healthy, container log sources are active, and a deterministic preflight log is retrievable from the normal logs backend under the exact monitor tags;
- the expected monitor is declared;
- the expected alert state is reachable;
- the ground truth names a concrete causal chain;
- required logs, traces, metrics, and feed events are expected during the bounded window;
- the execution path can retain the Datadog feed-event ID, not only the observed monitor state.

**Stop gate 1:** do not run if the execution controller has no supported way to capture the feed-event ID or alert URL. `MonitorEvent` in `gensim/src/controller/controller/phase_executor.py` currently records only monitor name, target status, reached state, and timestamp; that structure alone is insufficient for retroactive eval.

### 2. Preflight access and configuration

Use the current GenSim, Sims Archiver, Eval Data Portal, Scenario Store, and DDEval runbooks. Authenticate through approved internal identity paths. Use `op://` references or the approved secret provider; do not place secrets in command lines or artifacts.

Verify before creating resources:

- the target org and datacenter;
- the canonical execution command or owner-operated workflow;
- production archive destination;
- Eval Data Portal readiness;
- DDEval crawler selection;
- the explicit investigator model override;
- judge model configuration;
- local output and evidence directories.

Use the production crawler for an unchanged-code primary smoke. Use a shadow only when testing an intentionally deployed crawler change, and record the shadow source revision and dirty-state status.

**Stop gate 2:** do not proceed with an unpinned investigator model, an unidentified crawler revision, or a dirty shadow deployment that would confound the result.

### 3. Execute the live episode

Start the bounded episode through the approved execution path. Observe baseline, disruption, and cooldown without running concurrent diagnostics that can interfere with the foreground workflow.

Require:

- baseline reaches the expected healthy state;
- disruption reaches the selected alert state;
- cooldown reaches the expected recovery state, or a documented scenario contract permits otherwise;
- execution reports success;
- the alert timestamp falls within the execution window;
- the selected monitor and environment tags match the episode.

Retain the live monitor until Steps 4 through 10 complete. This is an operational safety measure while qualifying the capture, not a replay dependency.

### 4. Capture and validate the real alert feed-event ID

Immediately after the alert, use the supported monitor/event API or execution controller to capture the alert transition's real Datadog feed-event ID and alert URL. Do not derive an ID from a UUID generator, monitor ID, timestamp, or environment name.

Before archival, inspect `run-result.json`. Adapt the path only if the canonical controller uses a documented equivalent:

```bash
jq -e '
  (.execution.alert.event_id //
   .result_data.alert.event_id //
   "")
  | test("^[0-9]+$")
' run-result.json
```

Validate monitor identity and timestamp:

```bash
jq -e '
  (.execution.alert.monitor_id //
   .result_data.alert.monitor_id // 0) > 0
  and
  ((.execution.alert.timestamp //
     .result_data.alert.timestamp //
     "") | length > 0)
' run-result.json
```

Record the event ID once and compare all later stages against it.

**Stop gate 3:** do not archive if the real numeric event ID is absent. Episode 2666 would have stopped here.

### 5. Verify live telemetry before archival

Run the episode's strict verifier and inspect the evidence rather than relying on a single success flag. Confirm:

- the selected monitor generated the identified alert event;
- required causal metrics exist in the bounded window;
- logs and traces needed by the ground truth exist;
- expected tags scope the data to the correct environment;
- no unexplained timestamp rewriting or scope loss occurred;
- the event timestamp used in metadata matches the identified feed event.

Warnings affecting replayability are blocking even when the verifier labels them nonfatal.

**Stop gate 4:** do not archive if the event cannot be retrieved live by its exact ID, the timestamp or monitor does not match, or required telemetry is absent.

### 6. Run a dry-run archival qualification

Run the supported Sims Archiver dry-run before creating the production archive. Review requested time bounds, environment, tracks, destination, execution metadata, and monitor definitions.

Require the dry-run input to contain the alert object with:

```json
{
  "monitor_id": 123,
  "event_id": "REAL_NUMERIC_FEED_EVENT_ID",
  "timestamp": "RFC3339_TIMESTAMP"
}
```

If the dry-run output does not expose enough information to verify this contract, inspect the exact execution JSON submitted to Sims Archiver without printing secrets.

**Stop gate 5:** do not start persistent archival when its input omits or changes the event ID.

### 7. Create and verify the immutable production archive

Trigger archival once through the approved production path. Record the workflow and run IDs and wait for completion before performing other diagnostics.

Inspect the completed workflow result and `metadata.json`. Require:

```bash
jq -e '
  (.execution.alert.monitor_id // 0) > 0
  and
  ((.execution.alert.event_id // "") | test("^[0-9]+$"))
  and
  ((.execution.alert.timestamp // "") | length > 0)
' metadata.json
```

Compare `metadata.execution.alert.event_id` byte-for-byte with the ID retained in Step 4.

Verify archive completeness from metadata and objects:

- no required entry appears in `failed_archives`;
- `evp/feed/` exists and has a positive row count;
- an archived feed row matches `@evt.id:<event_id>`;
- the matching row has the expected monitor ID, org, and bounded timestamp;
- required logs, traces, metrics, monitor definitions, and markers exist;
- row counts and ranges are plausible and nonzero where required;
- archive root, manifest, and digests are retained as immutable evidence.

A positive `evp/feed` count is not enough; the exact trigger row must be proven.

**Stop gate 6:** do not ingest when the metadata omits the real event ID, the event ID differs from Step 4, or the exact event is absent from `evp/feed`.

Do not patch `metadata.json` or Parquet objects in place. A failed immutable archive remains evidence of a failed capture. Correct the producer and create a new execution/archive identity.

### 8. Qualify the Eval Data Portal projection with a dry-run

Wait for production Eval Data Portal readiness, then submit the supported single-scenario ingest request with:

```text
dry_run: true
force: false
```

Inspect the projected Scenario Store payload. Require the Step 4 event ID in both:

```text
attributes.event_id
agent_context.metadata.investigation_event_id
```

Also require:

- the expected monitor ID;
- the alert timestamp, not merely the episode start time;
- correct org and archive context;
- correct source and agent labels;
- enrichment protobuf size below 131,072 bytes;
- no placeholder UUID or environment-name fallback.

`eval-data-portal/internal/ingestion/parser.go::agentInvestigationEventID` prefers `metadata.execution.alert.event_id` and then the `event_id` query parameter from `alert_url`. A UUID-shaped `investigation_event_id` indicates the placeholder path and is a hard failure.

**Stop gate 7:** do not persist ingestion when the projected IDs are absent, nonnumeric, or unequal.

### 9. Persist the Scenario Store record once

After the dry-run passes, repeat the supported ingest with:

```text
dry_run: false
force: false
```

Record the Scenario Store UUID and external ID. Repeat only the idempotency check supported by the portal; it should identify or skip the same record rather than create an unexplained duplicate.

Do not use `force:true` to repair trigger metadata without first establishing update and duplicate semantics. Never force-ingest an immutable source archive whose metadata is known to be wrong.

### 10. Run a no-investigation load-only qualification

Use DDEval's load-only mode for the exact Scenario Store UUID. Verify that the loaded scenario reports:

- correct org;
- exact real event ID;
- correct monitor ID;
- correct event timestamp;
- expected archive context;
- expected ground truth.

Treat zero scores from load-only as reporting artifacts because no investigation or judge ran.

**Stop gate 8:** do not launch a paid investigation if load-only displays a UUID placeholder, wrong event, wrong monitor, wrong org, or wrong archive context.

### 11. Run one explicitly pinned primary investigation

Run one iteration through the production crawler unless the smoke intentionally tests a deployed shadow. Explicitly pin the investigator model, for example the experiment-approved override:

```json
{"react_investigation":"anthropic/claude-opus-4-6"}
```

Pin the approved new-score and DeepJudge models as well. Give the run a unique descriptive name and retain Temporal, investigation-job, and LLMObs identities.

Do not start multiple iterations until the single iteration proves the plumbing contract.

### 12. Verify service-side execution before interpreting scores

Use Datadog logs and supported service-side reporting to distinguish trigger failure from investigation quality. Require evidence that:

1. the crawler received the expected real event ID;
2. archive mode and the expected archive context were enabled;
3. the event lookup succeeded;
4. Bits entered the investigation strategy;
5. a non-empty investigation/report was produced;
6. new-score and DeepJudge evaluated that report;
7. per-iteration completion was recorded.

Fail the smoke if logs show `Unable to find event_id`, no investigation result, crawler errors, archive-scope errors, or zero evaluated judge count.

A score of zero may be a valid quality result only when Steps 1 through 7 above are proven. It is not valid evidence when the investigation never started.

### 13. Recover and retain authoritative results safely

Prefer the supported DDEval report path, Slack report, service-side logs, LLMObs, or approved result API. If local CLI output contains an empty `judgeResult` and warns that it cannot read an offloaded blob, do not infer investigation quality from locally displayed zeroes.

Do not:

- rerun a paid investigation merely to repair local hydration;
- use an AWS account explicitly denied by `dd-llm-data-us1-prod`;
- copy credentials or presigned result URLs into evidence;
- assume a crawler shadow changes local S3 authorization.

Retain authoritative counts, scores, model identities, run identities, and a credential-free reference to the result location.

**Stop gate 9:** do not call the smoke successful until service-side evidence proves an investigation was produced and judged.

### 14. Clean up and close the smoke

Only after Steps 1 through 13 pass:

- delete temporary monitors through the approved path;
- remove the temporary namespace/workload;
- release any shadow reservation;
- confirm resource absence;
- preserve immutable archive and Scenario Store identities;
- update the evidence manifest with cleanup status.

The completed replay must continue to point at the immutable archive. A new monitor with a different ID or event cannot repair the existing scenario.

## Final success checklist

The smoke is complete only when every item is true:

- [ ] Maintained episode and immutable source revisions recorded.
- [ ] Canonical execution identity recorded, or noncanonical status explicitly documented.
- [ ] Live episode completed with usable ground truth.
- [ ] Real numeric feed-event ID captured before archival.
- [ ] `run-result.json` contains the exact event ID, monitor ID, and alert timestamp.
- [ ] Sims Archiver input preserves those values.
- [ ] Production archive completed with no unexplained required-track failure.
- [ ] `metadata.json.execution.alert.event_id` matches the live event.
- [ ] Exact event exists in archived `evp/feed` with matching monitor and timestamp.
- [ ] Eval Data Portal dry-run preserves the exact ID in both projected fields.
- [ ] Projected enrichment is below the protobuf size limit.
- [ ] Persistent ingestion used `force:false` and returned a stable Scenario Store UUID.
- [ ] Load-only shows the correct org, event, monitor, timestamp, and archive context.
- [ ] One explicitly pinned primary investigation ran.
- [ ] Crawler logs prove archive-mode event lookup succeeded.
- [ ] Bits produced a non-empty investigation.
- [ ] Service-side judges evaluated the investigation.
- [ ] Authoritative results and model identities were retained.
- [ ] Temporary resources were cleaned only after qualification.

## Episode 2483 disposition

Scenario Store UUID `f0db7c0e-df73-4a95-9973-9a2b1778a8ee` satisfies the technical replay contract. DDEval run `episode-2483-1hz-technical-e2e-v1` preserved event `8768398996205424422`, monitor `313936953`, and archive context `episode-2483-1hz-20260816-164841`; crawler logs prove archive mode and exact `@evt.id:8768398996205424422` lookup. The investigation completed, used the metric tool successfully, produced a hypothesis, and reached both judges. It scored `0/1` because it attributed the metric drop to generic `notification-router` logic rather than the datetime serialization regression in `event-transformer`.

Classify this as `technical_bits_alert_e2e_completed_model_rca_failed`, not `failed_before_investigation`. Do not infer treatment effect: this is one CPU-throttled, noncanonical treatment-only run. LLMObs export proves the metric request omitted `interval` and `raw_data`; the model received 20 binned summaries rather than raw one-second points and queried only the alerting notification-enqueue metric, not the causal transformer-exception metric.

## Episode 2666 disposition

Do not use Scenario Store UUID `3c774cd2-ca21-446d-8f33-e6d152196e4b` as a successful retroactive-eval smoke. It remains useful evidence that:

- the treatment Agent and ordinary telemetry plumbing worked;
- Sims Archiver captured broad telemetry, including `evp/feed`;
- Scenario Store ingestion and load-only worked structurally;
- placeholder trigger IDs can pass ingestion but fail crawler replay.

Do not mutate its production archive in place. The safe paths are:

1. fix the execution metadata producer so future captures retain the real feed-event ID, then create a new execution and immutable archive; or
2. implement and review a deterministic Eval Data Portal lookup that selects the unique matching archived feed event by monitor, org, and bounded timestamp, with ambiguity and no-match failures, then re-project under understood Scenario Store update semantics.

Path 1 is the preferred smoke-test path because it preserves explicit provenance and avoids heuristic reconstruction.
