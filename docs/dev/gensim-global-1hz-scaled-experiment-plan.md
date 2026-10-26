# Scaled GenSim Global 1 Hz × Bits Experiment Plan

> **Documentation role: authoritative scientific design.** This remains the governing arms, estimand, gate, and analysis contract. Capture and archival have since executed; use `gensim-global-1hz-e2e-flow.md` for the authoritative as-operated path and current downstream status. Dated readiness and proposed-execution passages below remain as design provenance rather than current-state assertions.

Status: capture and archival complete; Scenario Store ingestion and DDEval blocked at the Eval Data Portal projection-size gate

Plan role: this is the controlling scientific design and analysis plan. Supporting documents may add implementation detail but may not silently change its arms, estimand, gates, or stopping rules.

**Execution decision:** scored captures use the approved durable remote `episode-ctl` campaign-controller route. The controller requirements in `docs/dev/gensim-global-1hz-episode-ctl-campaign-plan.md` supersede earlier gs-episode-worker/gs-flow-only execution requirements in this document. This change is accepted only because the controller makes every run reproducible through immutable source/rendered-workspace hashes, write-ahead state, runtime deployment identity, immutable archives, and append-only attempt evidence; it is not a laptop-owned workflow.

Primary execution references:

- [Running an Episode on GKE with episode-ctl](https://datadoghq.atlassian.net/wiki/x/QYR2pQE), page `7070975041`, version 12 — required same-scenario A/B build/deploy sequencing and GKE isolation rules;
- [GenSim — GS Episode Worker](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/6986335236/GenSim+-+GS+Episode+Worker), page `6986335236`, version 11 — managed-path background only; current `origin/main` fleet implementation is not a dependency; and
- [GenSim — How to Run an Episode](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/6489702531/GenSim+-+How+to+Run+an+Episode), page `6489702531`, version 8 — gs-flow background only.

Related plans:

- `docs/dev/gensim-global-1hz-episode-ctl-campaign-plan.md`
- `docs/dev/gensim-global-1hz-custom-agent-plan.md`
- `docs/dev/metric-resolution-bits-eval-plan.md`
- `docs/dev/archive/gensim-global-1hz/2026-08-25/gensim-retroactive-eval-smoke-plan.md`

## Goal

Measure whether a deployable global-1-Hz enablement bundle improves Bits AI SRE root-cause investigations. The bundle consists of globally increasing eligible Agent-owned metric resolution **and** adding concise 1-Hz retrieval guidance to the normal Bits prompt. The experiment also distinguishes telemetry-availability failures from metric-retrieval failures and detects regressions on incidents where high-frequency metrics should not be decisive.

Because telemetry cadence and investigator guidance change together, A versus B estimates the effect of the **bundle**, not the isolated causal effect of Agent cadence or prompt text. A cadence-only or prompt-only causal claim would require additional factorial arms and a new experiment version.

The experiment must preserve reproducible durable-run lineage, immutable source and evidence artifacts, exact numeric alert identity, normal production metrics-backend retrieval, and pinned investigator and judge configurations. The durable remote controller's manifest, write-ahead ledger, runtime identity gate, archive readback, and teardown reconciliation are the authoritative execution provenance.

## Experimental groups

Use two matched telemetry captures and two preregistered Bits prompt variants built from one exact main baseline:

| Group | Telemetry capture | Bits prompt and tools | Identified comparison |
| --- | --- | --- | --- |
| **A — normal cadence** | Normal production/default Agent cadence | Exact default prompt from the frozen main baseline; no 1-Hz guidance | Baseline production-like investigation |
| **B — global 1 Hz enablement** | Global 1 Hz for eligible Agent-owned paths | The same baseline prompt **plus** the exact advisory `global_1hz_eval_v3` instruction; otherwise the same model, tools, and limits | A vs B estimates the end-to-end value of the bundled telemetry-and-guidance intervention |

Call A **normal cadence**, not “15 seconds.” Agent metric paths do not share one production interval: recurring checks, ordinary DogStatsD aggregation, and serializer flushing have independently owned clocks. An exact global 15-second arm would be a separate intervention.

Freeze the exact main source commit and rendered default prompt used by A. Build B from that same commit plus only the reviewed profile change, then freeze the appended instruction, rendered treatment prompt, and both crawler image digests. Later dataset additions must reuse these exact A and B variants. Any additional prompt, tool, model, or investigation-policy difference requires a new experiment version.

## Bits intervention contracts

### Treatment-only guidance additive to main

Group A receives the normal default prompt from the frozen main baseline. Group B receives that complete prompt plus the following advisory behavior; it must not replace or shorten the standard root-cause, tool, conclusion, or citation contract:

1. inspect the alerting symptom metric and plausible upstream and downstream causal metrics;
2. consider whether short-lived ordering or precursor signals could matter;
3. request fine resolution and raw timestamps when causally useful;
4. verify the returned cadence rather than assuming that the requested cadence was honored;
5. avoid attributing the root cause to the alerting service solely because external alternatives were not found; and
6. state when metric resolution was insufficient to distinguish mechanisms.

The treatment guidance must not name the experimental group, expected root cause, or expected winning outcome. It necessarily exposes the availability of fine-resolution retrieval to B, so investigator blinding to the intervention is not claimed; scenario identifiers and judges remain blinded.

### Retrieval observability, not a forced arm

Do not add forced one-second retrieval to the scored experiment. Supplying `interval=1000` can affect automatic count/rate modifiers, while `raw_data:true` can materially increase context usage or truncation. Per-run enforcement would also require an isolated tool-policy boundary that has not been established. Those changes would create a second intervention and complicate the primary telemetry question.

Instead, retain every metric request and response and classify each B iteration as:

```text
metric not queried
metric queried at coarse resolution
one-second data requested but backend overrode it
one-second raw points reached the model but were ignored
one-second evidence contributed to the conclusion
```

Before scored execution, use a non-judged disposable tool-contract smoke to prove that the normal metric tool can retrieve one-second raw points for bounded eligible queries. This is validation evidence, not a third arm or a quality result. If B repeatedly has valid one-second backend data but fails only because retrieval remains coarse, report that as the experiment result and consider a separate retrieval-policy experiment later.

## Scenario population and selection

Do not use only scenarios expected to benefit, and do not dilute the primary estimate with an unstructured sample of log-only incidents. Freeze a stratified candidate manifest before observing A/B outcomes.

The frozen score panel deliberately includes four source-defined strata:

| Stratum | Frozen count | Purpose |
| --- | ---: | --- |
| **Resolution-exposed** | 7 | Primary efficacy cohort: Agent cadence can affect at least one causal metric path, but a positive outcome is not assumed. |
| **Metric-neutral** | 3 | Negative-control cohort: producer cadence or durable signals should limit incremental benefit. |
| **Mixed-secondary** | 2 | Ambiguous cohort: eligible metrics exist, but logs or persistent state may remain more decisive. |
| **Resolution-irrelevant** | 3 | Generalization and safety cohort: independently aggregated metrics, logs, or traces should dominate; measure distraction, contradictions, and cost. |

The complete-panel operational-utility score retains every valid null, negative, and regressive result. The resolution-exposed effect, complete-panel utility, and neutral/mixed/irrelevant safety results are also reported separately so one aggregate cannot hide where 1 Hz helps or hurts. Source strata are assigned before Bits execution and are not post-outcome eligibility decisions.

The 2026-08-25 live GS Admin refresh screened all 96 latest bundles with `causalgen.gates=passed`. Eight are blocked for genuine source defects; 88 are admitted: seven resolution-exposed, three metric-neutral, two mixed-secondary, and 76 resolution-irrelevant. The score panel includes every exposed, neutral, and mixed source plus three resolution-irrelevant sources selected by snapshot-seeded SHA-256 ranking. Its canonical manifest is `gensim-global-1hz-corpus-freeze.json`, snapshot SHA-256 is `97177cddd5d3c66721eec5ae14d0648207f661287f326492226cb48f9b354539`, and corpus SHA-256 is `fbfdb027aecf3ba98b05eb5df0830a2b383b8c70d9045ab5c5be6bdee0477dd5`. Admission does not require a pre-existing 1 Hz advantage, counterfactual, short incident, RCA improvement, or compatibility with the separate campaign re-archiver schema. Episode 2483 is the nonconfirmatory capability reference; the remaining capability smokes and owner approval are pending.

Use opaque identifiers in every investigator- or judge-visible field. Environment tags, scenario/capture aliases, monitor names and messages, alert titles, service context, archive-derived context, and dataset metadata must not contain `normal`, `control`, `treatment`, `1hz`, the toggle value, or another decodable arm marker. Store the private arm mapping separately from the blinded analysis export. Before DDEval, run a leakage audit over the fully rendered alert context, Scenario Store projection, crawler request, and judge input; detected leakage fails pre-investigation qualification and is not repaired after an outcome is observed.

For each candidate record:

```text
scenario_source_revision
app, episode, blueprint, and app-bundle identity
incident family
source_telemetry_class: resolution_exposed | metric_neutral | mixed_secondary | resolution_irrelevant
primary evidence modality
metric source class and cadence owner
alerting symptom metric
upstream and downstream causal metrics
expected temporal relationship
root cause, affected component, failure type, and impact
activation, recovery, and negative-control evidence
judge tier and calibration hashes
```

The exposed stratum should cover ordinary non-timestamped DogStatsD counts, gauges or timings, and recurring Agent checks. Distributions/sketches, timestamped DogStatsD, trace-derived statistics, producer-scheduled OTLP metrics, and independently scheduled processes are not evidence of the global Agent-owned 1 Hz treatment. The Metrics team confirmed an additional hard distribution limit: distribution metrics have a minimum 10-second aggregation upstream of HM-IO; it is enforced at write time when tags are configured and otherwise during query. A distribution may remain as unchanged incident context or appear in a neutral/safety scenario, but it can never be the qualifying causal series for the resolution-exposed cohort ([Slack thread](https://dd.slack.com/archives/C04HBT86H9Q/p1786460862240459?thread_ts=1786038676.108649&cid=C04HBT86H9Q)).

Episode 2483 is a useful scenario pattern because the notification symptom and transformer exception metrics form a causal path. Its existing capture remains excluded from experiment-quality evidence because it is noncanonical and the Agent was materially CPU-throttled.

The bounded nonconfirmatory pilot preregistered fresh matched pairs for episodes 2483 and 2563, not reuse of their historical captures. Its exact serial order was `2483/arm-a`, `2483/arm-b`, `2563/arm-a`, `2563/arm-b`; no arm was retried or replaced, and any failure cancelled all remaining queued arms. Historical runs 2600 and 2563 remain retained separately because they predate the immutable-v2 paired observer and all-attempt accounting contract. This paragraph records the original pilot design and preflight; the later 30-arm selected campaign and current status are summarized in `gensim-global-1hz-e2e-flow.md`.

### Frozen Agent image

Use the already qualified immutable Agent artifact in both arms:

```text
datadog/agent-dev@sha256:bcab32ebb2e768d89a0584b5a35c1043d8d8cb660df8861cd7dd3fccea392414
```

Its source is experiment branch commit `fbe7ac17d919fd501c4c3395e8965333adefdd21`. A 2026-08-17 fetch found that commit four commits ahead and 218 commits behind `origin/main` `c8497747f8f462d9296edd81286534f96511d365`; this is recorded provenance, not a reason to rebuild. The experiment estimates the effect of toggling one-second resolution within this frozen Agent artifact, not the effect on mutable latest main. Rebuilding on current main would introduce unrelated changes and invalidate the completed qualification unless a concrete compatibility, security, provenance, or treatment-integrity problem requires it.

Before the first managed capture, read back the registry digest, runtime image ID, source commit, effective configuration, and matched resources. Run the managed-topology compatibility smoke with this digest. Rebuild only if that smoke or a targeted review identifies a disqualifying problem; any replacement digest requires the complete focused test/build/runtime/soak qualification again and a new experiment version. Continue to re-fetch mutable dd-source and gensim-apps before their formal freeze, then pin their exact reviewed revisions for the experiment lifetime.

## Phase 1 — Scenario selection and capture qualification

Ground-truth quality, operational execution, telemetry fidelity, identity, archival projection, and Bits behavior are separate contracts. A pass in one does not imply a pass in another.

### Selection workspace and durable outputs

Run broad scenario discovery and source inspection in a dedicated Datadog Workspace because it requires multiple large repositories, internal access, and repeatable long-running scans. Follow the official [Getting Started with Workspaces](https://datadoghq.atlassian.net/wiki/x/gYFYuQ) guide (Confluence page `3109585281`, version 101), pin every inspected repository revision, and treat the workspace as disposable compute rather than the source of truth.

The durable outputs are the reviewed repository manifest, per-scenario rationale, rejection reason, source hashes, ground-truth fixture, stratum, and qualification state. Commit or preserve those artifacts outside the workspace before it is retired. Discovery may run in a separate session or fork, but it may not change the frozen arm or scoring contracts.

### Staged scenario funnel

Do not require a scenario to pass the full expensive E2E before it can enter the candidate pool. Advance it through explicit stages:

1. **Source-screened candidate:** maintained source at a recorded revision; explicit immediate cause, affected component, mechanism, impact, activation, recovery, and negative controls; bounded inverse; plausible target telemetry; no dependency on ignored rollout failures or broad cluster destabilization.
2. **Judge-qualified candidate:** the exact proposed Scenario Store description is judge-ready without experiment-specific repair; repeated calibration accepts the canonical conclusion and rejects symptom-only, wrong-mechanism, and explicit-distractor conclusions.
3. **Execution-qualified candidate:** one fresh canonical qualification proves baseline, alert, and recovery behavior; numeric event identity; required telemetry; acceptable resources; archive projection; and cleanup.
4. **Frozen member or reserve:** source, ground truth, stratum, hashes, qualification evidence, and replacement status are reviewed and frozen before randomization.

A source-screened candidate may be rejected cheaply. Only execution-qualified candidates may enter the pilot or confirmatory manifest. Assign each accepted scenario to `resolution_exposed`, `metric_neutral`, `mixed_secondary`, or `resolution_irrelevant`; an unknown metric source cannot be counted as resolution-exposed. Mixed and irrelevant results remain in the complete-panel utility and safety analyses rather than being treated as failed admissions.

### E2E evidence required by observed failure modes

Execution qualification must explicitly prove the boundaries that failed or remained ambiguous in prior attempts:

- canonical managed lineage rather than a direct GKE or local controller smoke;
- immutable workload, Agent, and sim-generator identities with matched, non-throttled resources;
- baseline telemetry before disruption and intended alert/recovery behavior with timing margin;
- a real numeric trigger feed-event ID that is present in the archive and exact Scenario Store readback;
- causal metrics plus required logs/traces under the exact environment and tags, without timestamp rewriting, interpolation, unexplained loss, or wrong scope;
- successful archive, force-false Scenario Store ingestion, scenario-bound DDEval routing, complete LLMObs export, and normal judge execution; and
- terminal cleanup and reconciliation of namespaces, vclusters, monitors, partial archives, and workflow state.

### Fresh execution qualification and stop handling

Before disruption, require direct normal-backend receipt for the exact environment and tags: trigger-monitor telemetry, at least one target causal metric, and required logs/traces.

If this pre-disruption gate fails, stop **only the current execution before injecting the fault**. Mark it `pre_disruption_qualification_failed`, append the reason and identities to the attempted-capture ledger, clean up resources, and do not archive, ingest, or evaluate it. It may use the single predefined infrastructure replacement only after the cause is fixed and before any semantic investigation outcome exists.

After disruption, require:

- baseline, alert, and recovery gates all pass;
- workload and Agent pods are Ready with no unexplained restart or OOM;
- no material Agent CPU throttling or serializer/drop errors;
- no ignored step-pod, rollout, or monitor timeout;
- direct backend cadence statistics for each claimed target metric;
- expected values and tags without timestamp rewriting, interpolation, unexplained loss, or wrong scope; and
- a real numeric trigger feed-event ID and exact recovery identity.

A post-disruption failure during the **pre-randomization qualification stage** is retained as `capture_qualification_failed` and blocked from archive, ingestion, and DDEval. Replacement is allowed only for a preregistered infrastructure failure, never to improve a semantic result. Repeated failures escalate to the fleet stopping rules; otherwise qualification may continue after cleanup and reconciliation.

After opaque arm assignment, monitor behavior becomes an experimental outcome rather than an admission gate. A successfully observed `no_transition`, a transition without a visible feed event, and an event/API evidence failure must be recorded as distinct states in the all-attempt ledger. None may alter fixed phase timing or silently delete or replace the assigned attempt. Archive, Scenario Store, and Bits availability are measured downstream statuses; complete-case Bits analysis is secondary to the full attempted-panel operational analysis.

The bounded controller observer implements this contract by combining complete fixed-phase monitor-state samples with a post-run exact Datadog trigger/recovery event query. Known samples must cover the disruption at no more than 60-second gaps to support `no_transition`; unknown, sparse, malformed, non-unique, or unavailable evidence is retained as `evidence_unavailable`. The observer runs only after the episode subprocess ends and cannot affect timing, retry, cleanup, attempt status, or corpus inclusion. Its event lookup always executes the same fixed twelve-request and eleven-wait schedule, without early exit when an event appears, so observed arm outcomes cannot change the next arm's scheduling. Operator interruption records queued and started attempts as cancelled in the all-attempt ledger before cleanup. The first executable scope is only one explicit `logged_nonconfirmatory_pair`: exactly two opaque arms, `max_parallel=1`, no automatic retry, and a generated `plan.json` whose full digest is the single freeze-and-start confirmation. A first-arm failure records and cancels the second arm; scheduler failure remains an operational outcome, not a reason to replace the pair.

### Archive, Scenario Store, and DDEval qualification

Require the approved durable remote-controller lineage:

```text
frozen source archive and rendered per-arm workspace hashes
  → remote controller lease and write-ahead action ledger
  → phase-separated episode-ctl deploy / run-episode / verify / archive / teardown
  → approved GKE workload and normal Datadog telemetry backend
  → immutable per-arm archive with checksum/readback
  → Scenario Store dry-run and force:false ingestion
  → DDEval / Bits crawler archive mode
```

The controller records namespace, Helm release, Cloud Build, runtime workload-image, Agent-image, monitor/event, result, archive, and cleanup identities for every arm. Every telemetry arm receives an immutable archive and Scenario Store UUID. Require exact identity continuity from live feed event through run result, archive feed row, Scenario Store, DDEval input, and crawler lookup. Archive destination collisions, identity mismatches, missing archival queries, incomplete hydration, or `force:true` are stop conditions.

## Scaled GenSim execution model

### Durable remote `episode-ctl` execution

Confluence page `7070975041`, version 12 documents the required GKE `episode-ctl` sequencing. Scored captures are not laptop-owned: an approved durable remote runner executes the purpose-built controller specified in `docs/dev/gensim-global-1hz-episode-ctl-campaign-plan.md`.

The generated local-GKE prototype is recorded in `docs/dev/archive/gensim-global-1hz/2026-08-25/gensim-global-1hz-pilot-manifest.json` and `/Users/ella.taira/episode-bundles/global-1hz-pilot-bundles-v1`. It is source-derivation evidence only. The controller must create separate immutable per-arm workspaces from the frozen source archive and never run A and B from one mutable checkout.

The controller's required isolation rules are:

- every run must have a unique `--namespace` and matching unique `--env`;
- the active `kubectl` context determines the target cluster;
- different bundles may build concurrently;
- the same bundle must not build concurrently because service image tags are fixed and can race in GAR;
- for same-scenario A/B qualification, stagger build/deploy or pre-build service images once, then run phases side by side;
- verify running Agent image, imageID, effective environment, and configuration in each namespace;
- bound local fan-out because Cloud Build can return `RESOURCE_EXHAUSTED`; and
- always verify namespace, monitors, Helm release, and cluster-scoped RBAC cleanup after interruption.

Do not implement the scaled experiment as a large collection of laptop-owned `episode-ctl` processes or fire-and-forget shell children. The durable controller uses a remote campaign lease, write-ahead state, per-source deploy lock, reconciliation on resume, and bounded active deploy/run phases. It starts at one active pair and may increase only through the qualification gates in the companion controller plan.

### Generate matched remote-GKE bundle variants; do not hand-edit runs

Do not manually edit every downloaded bundle or replace each scenario's Agent manifest with one universal template. Scenario charts differ, and wholesale replacement caused earlier local log-collection failures.

For each immutable source bundle, the controller creates two separate rendered arm workspaces from the same source archive:

```text
source bundle/archive content hash
renderer name and version
common patch: custom Agent digest, matched resources, NTP exclusion, evidence hooks, and supported GKE Agent topology
first opaque arm patch: experiment toggle false
second opaque arm patch: experiment toggle true
rendered per-arm bundle/workspace hashes
allowlisted normalized A/B diff
```

The only A/B difference is the treatment toggle. Common changes needed to run the custom image safely are identical in both variants. The controller serializes the arms' deploy/build phases, then uses the runtime workload-image-ID parity gate before either episode starts. Fail rendering when the renderer cannot prove the expected Agent topology, rendered image digest, log mounts/runtime sockets, environment tags, or exact normalized diff.

The controller retains original source episode/blueprint/app-bundle identities as source provenance when available, but it does not require gs-admin publication to execute. Its approved manifest records source/version/hash identity, derived arm-workspace hashes, destination namespaces/environments, and all required remote-runner/archive readback fields. The owner executes approved external writes; the assistant never publishes. Stop on source, hash, rendering, namespace, environment, or provenance drift.
### Preserve logs and traces in isolated managed environments

The local episode 2600/2563 failures were topology failures, not evidence that log scenarios are intrinsically unsuitable: a bounded single Agent Deployment could not collect node-local container logs across workload nodes, and an unconstrained DaemonSet or host networking was unsafe on the shared GKE cluster.

For canonical gs-flow/Terrapin jobs, preserve each scenario's supported app-local Agent log/APM topology rather than the bounded local workaround. An isolated per-job virtual cluster should make a normal Agent DaemonSet safe, subject to owner confirmation: it must run on every workload node, retain the required container-runtime socket and host log mounts, avoid port conflicts, and use the same topology in A and B. Do not assume isolation fixes collection; prove it per capture before disruption.

A log/trace-led capture qualifies only when:

1. every workload node has a Ready Agent with the intended imageID and effective config;
2. the Agent reports no kubelet/container-runtime initialization error;
3. deterministic preflight logs from at least two relevant workload services are retrievable from the normal logs backend under the exact environment/service tags;
4. traces for the intended services and environment are retrievable;
5. trigger-monitor queries use those exact tags; and
6. log/trace volume and service coverage are materially matched across A and B.

If either arm fails this pre-disruption gate, record the attempted pair as an infrastructure failure and do not start, archive, ingest, or evaluate the incident. Apply the single predefined infrastructure replacement policy; do not adapt topology after seeing investigation outcomes.

### Durable remote campaign fan-out

The scaled path uses the owner-approved remote `episode-ctl` campaign controller. Its append-only launch manifest contains, for every capture pair:

```text
capture_set_id and opaque pair/arm IDs
scenario identity, frozen source archive hash, and rendered per-arm workspace hashes
private arm-mapping reference and blocked launch order
same Agent digest and expected false/true toggle assignment
unique namespace and matching DD_ENV per execution
approved GKE target, runner, state-store, campaign-lease, and artifact/archive identities
requested concurrency class, expected duration/resource class, and retention deadline
Cloud Build, runtime image, result, archive, ingestion, Scenario Store, and DDEval state
```

Controller rules:

1. never allow two mutable builds of the same bundle/tag set to race;
2. serialize same-scenario A/B deployment, then permit their episode phases to overlap only after both deployment identity gates pass;
3. create immutable per-arm workspaces and never concurrently run one mutable bundle checkout;
4. write a deterministic intent record before every remote mutation and reconcile namespace, Helm, pod/image, build, monitor, result, archive, and RBAC state before retrying an ambiguous action;
5. use a durable remote campaign lease and explicit state machine rather than fire-and-forget subprocesses;
6. start with one active deploy, one active pair, and two active episode phases; an active deploy may internally fan out to multiple Cloud Build jobs, so do not claim a numeric Cloud Build-job cap the controller cannot enforce;
7. require equal runtime workload-image maps, equal Agent imageIDs, expected per-arm toggle values, unique namespace/env, and Ready workloads before either `run-episode` action;
8. apply backpressure when Cloud Build/GAR/GKE, Datadog monitor/telemetry, archive, ingestion, or DDEval capacity degrades;
9. prevent failed or ambiguous attempts from automatically entering archive/ingestion/evaluation; and
10. permit at most one replacement for an enumerated, reconciled pre-episode infrastructure failure while preserving every attempted capture. Never semantically rerun a completed investigation.

Use a staged ramp:

```text
controller dry run and durable-state test
1 scenario × 1 matched capture pair, including crash/resume qualification
capacity review
4-scenario pilot at the separately approved bounded concurrency
scaled manifest at the empirically safe concurrency
```

A ramp stage advances only when every execution is reconciled against its remote controller state, namespaces and Helm releases are gone, archives are complete/read back, and no monitor or cluster-scoped RBAC leak remains.
## Pilot and scale

### Four-scenario pilot

Use four complementary scenarios:

1. resolution-exposed ordinary DogStatsD count;
2. resolution-exposed gauge/timing or recurring check;
3. metric-neutral incident; and
4. log/trace-led safety incident.

For each scenario, produce one fresh matched normal/1 Hz capture pair. Evaluate A against the normal capture using the frozen main default prompt, and evaluate B against the 1 Hz capture using the frozen additive v3 treatment prompt. Run 10 pinned-model DDEval iterations per group with one investigation worker initially. Randomize capture order and DDEval run order from one preregistered seed, block so each scenario family alternates which arm runs first, and use opaque neutral names. Retain the seed, generated schedule, and schedule hash before the first launch.

The four pilot capture sets are design data only. Use them to estimate variance, choose the final sample size, and freeze thresholds, but exclude them from the confirmatory efficacy estimate. Report pilot outcomes separately and never relabel a pilot capture as confirmatory after viewing its result.

Iterations are allowed not to query metrics; that is an investigator outcome, not a post-hoc qualification failure. Preserve the exact retrieval behavior rather than forcing it.

### Scale-up

Use the pilot to estimate between-capture and within-capture variance and set the final sample size. Before scale, preregister the minimum detectable efficacy effect plus safety, latency, and cost regression thresholds derived from the pilot; do not choose success criteria after viewing scaled outcomes. Never use fewer than five independent matched capture sets. Prefer multiple incident families and independent reruns. DDEval iterations measure stochasticity conditional on one capture; the capture set is the independent experimental unit.

Do not increase from 10 to 20 iterations per group unless the pilot shows that additional within-capture sampling materially improves precision. Complete all DDEval work before the normal metrics-retention deadline for each capture.

### Campaign stopping rules

Freeze numeric limits after the pilot. Before those limits are frozen, any occurrence of an identity mismatch, unplanned arm leakage beyond the registered additive B guidance, wrong Agent or crawler image/config, archive overwrite attempt, `force:true` ingestion, completed-investigation rerun, or exposed credential stops the fleet immediately. After preregistration, pause new launches and reconcile in-flight work when any of these conditions is met:

- the controller's preregistered external orchestration failure budget is reached, including unreconciled Cloud Build/GAR/GKE failures;
- Agent throttling, restart, OOM, serializer/drop, collision, duplicate, missing-second, gap, or point-loss evidence exceeds its frozen limit in either arm;
- treatment-integrity failures or telemetry-qualification failures exceed the frozen attempted-capture safety margin or differ materially by arm;
- log/trace coverage, workload image identity, phase timing, or non-treatment configuration diverges within a pair;
- archive, Scenario Store, LLMObs, or arm-mapping evidence cannot be reconciled before retention expiry; or
- the approved investigation or infrastructure budget reaches its stop amount.

A stop does not erase completed attempts. Resume only after the named operational owner records the cause, reconciliation evidence, affected attempt IDs, and explicit approval. Changing a scientific threshold, replacement rule, prompt, model, source revision, or inclusion rule after confirmatory outcomes exist requires a new experiment version rather than resuming the existing one.

## Evidence retained per unit

For every execution and DDEval iteration retain:

```text
capture_set_id, scenario_id, stratum, opaque group, iteration
remote runner, controller artifact/config, campaign lease, state-store, and immutable per-arm workspace identities
GKE context/cluster, namespace, Helm release, Cloud Build, workload-image, and Agent identities
Agent imageID, expected toggle, effective config, health, cadence, and resource evidence
archive root, object IDs/checksums/readback, controller archive state, and failure counts
trigger/recovery observation state; numeric event IDs and exact archived feed matches when visible
Scenario Store UUID, dataset UUID, ingestion and hydration status
investigation job, Temporal run, LLMObs trace, models and config hashes
metric queries, tags, windows, requested interval and raw_data
actual response format, interval, timestamps, truncation and overrides
conclusion, complete judge outputs, pass/inconclusive status
latency, tool count, tokens/cost, errors, contradictions
```

The normal metrics backend is the source of Bits metric evidence. Sims metric Parquet is optional archival evidence and is not substituted into the Bits query path.

### Attempted-capture ledger

Keep an append-only JSONL event ledger plus a deterministically generated materialized snapshot. Each event contains:

```text
schema_version, experiment_version, ledger_event_id, recorded_at
attempt_id, capture_set_id, scenario_id, stratum, opaque_arm_id
private_arm_mapping_ref, replacement_of_attempt_id
source_revision, blueprint_version, app_bundle_version, adapter_version
Agent, workload, sim-generator, crawler, investigator, and judge digests/config hashes
runner_job_id, controller_action_id, campaign_lease_id, GKE context/cluster, namespace, Helm release, Cloud Build, archive, ingestion, Scenario Store, DDEval, Temporal, and LLMObs IDs
stage, prior_state, new_state, intent_recorded_at, stage_started_at, stage_finished_at
qualification_result, failure_class, failure_code, failure_detail_ref
replacement_eligible, replacement_authorization_ref
trigger_event_id, recovery_event_id, identity_chain_result
artifact_manifest_ref, artifact_sha256, actor_or_service
```

Allowed lifecycle states are:

```text
planned → deploy_intent_recorded → deploy_submitted → deployment_identity_verified
        → episode_intent_recorded → episode_running → episode_finished → verify_finished
        → archive_finished → teardown_finished → ingested → investigation_started
        → investigation_completed → judged → analysis_frozen
```

Any state may transition to a terminal `failed_external_infrastructure`, `failed_treatment_integrity`, `failed_capture_qualification`, `failed_identity`, `failed_archive`, `failed_ingestion`, `failed_before_investigation`, or `failed_evidence` state. `investigation_completed` is semantically terminal for model execution: it may continue to judging and analysis but may never return to `investigation_started`.

Classify replacement eligibility before outcome inspection. Only enumerated arm-independent failures where reconciliation proves that no episode action began—such as a rejected Cloud Build request, provider/API 5xx, or failed deployment with no live namespace/workload—may use the single predefined replacement. Agent/treatment health failures, telemetry differences, identity failures, any accepted or ambiguous `run-episode` action, and any completed model investigation are non-replaceable attempted outcomes. A retry after an ambiguous write requires reconciliation against the deterministic controller action ID; it is not recorded as a fresh attempt unless the external operation is proven absent.

The blinded analysis export omits `private_arm_mapping_ref`, raw toggle values, and decodable deployment names. Validate event uniqueness, legal state transitions, referential integrity, replacement count, immutable hashes, and agreement between the JSONL ledger and materialized snapshot before outcome freeze.

## Analysis

Primary comparisons:

- **A vs B:** end-to-end effect of the bundled global-1-Hz telemetry plus additive advisory-guidance intervention; and
- **exposed vs neutral/log-led interaction:** whether benefit is concentrated where model-visible one-second evidence provides a plausible mechanism.

Report per capture set and stratum:

- immediate-cause and full-chain correctness;
- DeepJudge and match-probability distributions;
- metric queried/not queried;
- requested versus returned interval and raw point visibility;
- one-second evidence used, ignored, or contradicted;
- symptom-only attribution;
- pass, inconclusive, contradiction, and remediation rates;
- latency, tool calls, tokens/cost, and tool failures; and
- Agent CPU, memory, throttling, point loss, and collision rates.

Use capture sets as independent units; repeated DDEval iterations are conditional measurements within a capture, not additional independent samples. For each confirmatory resolution-exposed capture set `c`, compute the arm-level full-chain correctness rate across its frozen iterations and the paired difference `d_c = correctness_B,c - correctness_A,c`. The primary estimand is the equal-capture-weighted mean `mean(d_c)`; do not weight captures by iteration count, traffic volume, scenario frequency, or judge confidence.

Report a two-sided 95% paired t confidence interval for `mean(d_c)` and an exact paired sign-flip randomization p-value as the primary uncertainty summaries. Report a capture-set bootstrap interval and the continuous DeepJudge/match-probability paired effects as sensitivity analyses, not substitutes for the primary estimand. If scenario families contribute multiple capture sets, include a family-blocked sensitivity analysis. Missing or failed captures are not imputed into per-protocol correctness; their frequency and arm imbalance remain explicit attempted-capture safety outcomes.

Before confirmatory execution, freeze a practical efficacy threshold and one-sided non-inferiority margins for contradiction rate, inconclusive rate, remediation safety, latency, token cost, and Agent/capture failure. A positive **bundle** efficacy claim requires the primary interval to exclude zero in B's favor, the point estimate to meet the practical threshold, and all frozen safety margins to pass. A recommendation specifically justified by high-resolution telemetry additionally requires mechanism traces showing model-visible one-second evidence in the correct investigations; otherwise report the bundle outcome without attributing it to cadence. The headline result is the resolution-exposed paired effect. Report metric-neutral and the 20% log/trace-led panel separately as safety/generalization outcomes; do not publish an equally weighted or production-weighted omnibus effect for this experiment version. Report both the preregistered per-protocol analysis and attempted-capture outcomes: treatment-induced Agent or infrastructure failures must not disappear through post-treatment qualification.

## Interpretation matrix

| Result | Interpretation |
| --- | --- |
| B > A and traces use one-second evidence | The bundled intervention improves investigation quality with a mechanism consistent with high-resolution telemetry; the design still cannot separate cadence from guidance |
| B > A without model-visible one-second evidence | The bundle may help through guidance or trajectory changes, but this is not evidence that global 1 Hz telemetry caused the gain |
| B ≈ A and B receives only coarse metric responses | The bundle does not reliably expose the added telemetry; do not interpret this as a clean test of one-second evidence |
| B ≈ A despite model-visible one-second evidence | The bundle did not improve investigation quality for this corpus/configuration despite successful high-resolution retrieval |
| B regresses on neutral or log-led strata | Narrow or reject the telemetry-and-guidance bundle; do not recommend broad product use |

A positive high-resolution recommendation additionally requires trace evidence that correct investigations consumed one-second evidence, plus no material regression in contradiction rate, inconclusive outcomes, latency, token cost, remediation safety, or Agent health.

## Verified scale architecture findings

### 1. Historical maintained-app adapter study

An earlier nine-app maintained-source study reduced to three chart classes. It is retained as adapter provenance but does not describe or select the current GS Admin score panel. Current-panel topology and Agent injection must be read from each pinned bundle before capability smoke:

| Chart class | Apps | Adapter decision |
| --- | --- | --- |
| Datadog Helm dependency `3.82.0` | `adspend-checkpoint`, `ambulance-dispatch-zone-fairness`, `dogschedule`, `inventory-aging-query-parity`, `pharmacy-replicaepoch-store`, `qdrant-shard-recovery`, `supplierhub-mysql` | One shared values adapter with per-app render/telemetry assertions |
| Datadog Helm dependency `3.129.0` | `delivery-policydrift-controlplane` | Same logical values contract, separate render compatibility test |
| Datadog values but no chart dependency | `care-authcache-session` | Ineligible until the maintained source adds/validates the dependency, or exclude it; do not silently inject an Agent only in experiment packaging |

All nine historical maintained-app cases declared logs enabled and `containerCollectAll`; `care-authcache-session` lacked a chart dependency. Do not project those findings onto the refreshed GS Admin panel. Generate matched values from the pinned bundle's actual topology and fail closed on rendered topology, imageID inputs, logs/APM settings, resources, and normalized diff.

### 2. Managed log topology

The managed platforms support the intended topology, with an explicit k3s value required:

- the GKE-vcluster backend pins each vcluster to one host node, keeps Agent DaemonSet desired count at one, uses privileged Pod Security, and explicitly supports `/proc`, `/sys/fs/cgroup`, and `/var/log/pods` host paths (`gs-flow/internal/cluster/vcluster_templates.go`);
- production is Terrapin-only and the `simulation` tier runs privileged nested k3s with 8 CPU, 16 GiB memory, six-hour TTL, and `max_concurrent: 2` (`general2.us1.prod.dog/gs-flow.yaml`); and
- Datadog chart versions 3.82.0 and 3.129.0 both render a DaemonSet, `/var/log/pods` mounts, and `DD_CRI_SOCKET_PATH=/host/run/k3s/containerd/containerd.sock` when values set `datadog.criSocketPath: /run/k3s/containerd/containerd.sock` with file-based container log collection.

The shared adapter must set the nested-k3s CRI socket and file-log mode identically in A and B. A managed one-scenario smoke must still prove the socket exists, Agent log status is healthy, and logs from two services reach the backend before this support is considered qualified.

### 3. Durable runner and immutable source pinning

The supported execution entry point is the approved durable remote campaign controller invoking `episode-ctl`. The controller must receive a frozen local/remote source archive or checked-out source revision; it must record the source archive hash, rendered per-arm workspace hash, and controller artifact/config hash before the first deploy intent. A controller never resolves "latest" source content after launch approval.

The runner identity, GKE context/cluster identity, credential authority, durable state-store prefix, artifact/archive destination, retention period, and campaign lease must be named and approved before the staging qualification. A local branch or ephemeral CI workspace never becomes the source of truth: immutable campaign input and state objects are authoritative.

For the first owner-executed qualification pair, use only the approved frozen source bundle and record all Cloud Build IDs, resolved runtime workload image IDs, Agent image IDs, namespace/`DD_ENV`, result artifacts, archive object IDs/checksums, and teardown evidence. A later controller invocation must reconcile those identities rather than re-launching an ambiguous action.

### 4. Workload image identity

`EpisodeController.deploy(..., skip_build=True)` exists internally, but `episode-ctl` does not expose `--skip-build`. The normal path rebuilds and pushes service images under scenario/service tags, so concurrent A/B runs of one scenario can race even when their service bytes are intended to be identical.

Safe interim rule: run the same scenario's A and B build/deploy phases sequentially and never let those mutable tags overlap. After both deployments are Ready, capture each arm's resolved workload pod imageID map. Do not begin either `run-episode` phase unless the maps match exactly, the Agent image IDs match, and only the expected false/true Agent toggle differs. An image mismatch is an integrity failure, not a reason to rerun one arm.

Content-addressed prebuild/skip-build reuse remains an optional future optimization. Do not claim build-once reuse until a tested end-to-end contract exists; the controller's runtime image-ID equality gate is mandatory in the interim.

### 5. Initial controller concurrency limits

The controller starts with limits it can actually enforce:

| Stage | Controller bound | Experiment setting |
| --- | ---: | ---: |
| Active `episode-ctl deploy` processes | 1 | One active deploy process; this serializes same-scenario build/deploy but does not imply one Cloud Build subjob. |
| Active matched pairs | 1 | One pair through deploy/run/verify/archive/teardown at a time. |
| Active `run-episode` processes | 2 | Both arms only after deployment identity parity. |
| Same-source build/deploy | 1 | Per-source mutable-tag lock. |
| Verify/archive/teardown | 1 | Sequential during qualification. |
| DDEval | default maximum 3 agent jobs | `-j 1` for pilot and scored runs. |
| Scenario Store ingestion | no experiment-specific safe parallel limit established | Sequential, `force:false`. |

Do not use a fixed elapsed delay as a substitute for deployment readiness. The first qualification pair records Cloud Build fan-out, GAR pull behavior, GKE capacity, Datadog monitor/telemetry behavior, archive latency, and cleanup. Any concurrency increase requires an owner-approved capacity review before results are inspected.

### 6. Lifecycle ownership and reconciliation

Ownership is split deliberately:

- the approved remote runner owns campaign process availability, credential injection, controller artifact execution, and durable state/artifact access;
- the campaign controller owns the campaign lease, append-only manifest/ledger, immutable per-arm workspace creation, deploy/run admission, readiness and deployment identity gates, retry classification, reconciliation, and pause/resume decisions;
- `episode-ctl` owns build/push, Helm deploy, episode execution, verification, archive invocation, and teardown commands;
- the experiment manifest owns pairing, blinding, selection, DDEval scheduling, evidence reconciliation, and arm-aware operational access; and
- named GKE/Datadog/archive owners approve target, budget, retention, and recovery decisions.

`GenerateCausalgenEpisodes` is not a dependency: current `dd-source` `origin/main` exposes a proto but its Go method returns an empty response. The controller must not emulate a broad fleet; it remains a bounded, one-pair-at-a-time remote campaign until an explicit capacity gate authorizes an increase.
## Verified Bits prompt and retrieval findings

### Prompt boundary and freeze contract

Freshness was rechecked against fetched dd-source `origin/main` at `06aff7b04921c633d2f1e9214fdb637abc950d2a` on 2026-08-17. The current candidate source pair is A at that unmodified baseline and B at advisory-v3 commit `0545086d867fadba86053056012d3afeb27613fb`, the third commit on PR 58419 after v1 `5a5a994ada7b898a2a4edc9eed7e495a4d464d67` and mandatory v2 `9519d6962fddb3cbb025ddc16b59389dc27cbace`. V1 remains terminal non-confirmatory capability evidence; v2 is a distinct mandatory-compliance proposal. **Do not execute a future matched capture until the owner explicitly freezes either advisory v3 or a separately versioned replacement as the sole prospective treatment profile.** Phase 0 remains open until the selected rendered prompts and crawler digests are built and read back. The prior local checkout was 18,665 commits behind and is not a valid image-build base. Current `agentsconfig.Config` has two distinct selectors: `react_prompt_profile` for compiled prompt behavior and `system_prompt` for explicitly versioned **complete** embedded templates. `strategy_react.go` gives a named `system_prompt` precedence over deeper/default profile selection. Because the evaluation instruction must augment rather than replace the normal React investigation contract, a short standalone named template would be unsafe: it would discard the standard root-cause, tool, conclusion, and citation instructions.

Implement the treatment guidance as the opt-in compiled profile `global_1hz_eval_v3` in the third commit on PR 58419 branch `ella/global-1hz-eval-prompt-current`. The profile selects the same standard non-deeper React prompt and appends only the frozen advisory instruction on both the legacy and AIAgentSDK loops; it does not change defaults, GPT-5.5, deeper, named-system-prompt, remediation, or ordinary production behavior. V1 and mandatory v2 remain byte-identical, separately selectable historical profiles in the same branch history; the confirmatory B config explicitly selects v3 and never selects v2. This profile is part of treatment B and may remain on a pinned dd-source branch rather than merge to main. Preserve a temporary draft PR if required by Conductor/DDCI source resolution, mark it not intended to merge, and deploy the exact A baseline and B treatment commits to separately identified evaluation crawlers. DDEval's `--test-drive` selects a Rapid **eval-runner** task queue; it does not deploy crawler source. DDEval must separately route each arm's `crawler_app` to its frozen deployment. Current shadow reservations are single-tenant leases in US1 staging/prod and bind deployment intent to branch/commit; preserve both lease identities and read back each running image digest and available source provenance. **The experiment owner must personally approve and execute every deployment; preparation of v3 does not authorize deployment.** The only instruction added to B is:

> Metric resolution varies by source. When short-lived behavior or fine-grained timing could materially affect the diagnosis, query `get_datadog_metric` with `interval: 1000` and `raw_data: true`. Start with the narrowest window that covers the suspected transition; keep each query window at 15 minutes or less and split longer periods into separate requests. Before using fine-grained evidence, inspect returned timestamps, effective interval, gaps, truncation, and interval overrides. The requested interval does not guarantee complete one-second coverage; do not claim one-second resolution when the response is coarser or incomplete.

The 1,000 ms value requests one-second query buckets; it does not require the producer to have emitted at perfectly spaced wall-clock instants. Agent aggregation normally timestamps eligible one-second buckets on integer-second boundaries, while scheduling or collection lag can appear as delayed availability, merged samples, or missing buckets. This instruction is concise and actionable without mentioning the experiment, presupposing that one-second data exists, naming an expected root cause, or requiring a metric call in every investigation. Group A uses the exact default prompt from the frozen main baseline; Group B uses that baseline plus this instruction. Before the first scored run, freeze and hash:

- the fetched upstream main commit used by both prompt builds, the treatment-profile commit, profile source, and exact appended instruction;
- separate fully rendered A and B prompt fixtures proving that B is additive to A rather than a replacement;
- the separate canonical A and B `agent_config` JSON, with investigation strategy, enabled tools, maximum iterations, and model identifier otherwise equal;
- both crawler image digests and their source lineage; and
- each evaluation node/flavor and deployment identity binding DDEval routing to the correct digest.

A mutable branch name, node name, crawler app, or test-drive name is not sufficient provenance by itself. Hold both deployed digests for the full experiment and later additions, record each running image ID and available source provenance, and reject a scored launch if either differs from the manifest. At runtime, preserve the rendered-system-message evidence available for each provider. Validate A against its frozen default fixture and B against its frozen additive fixture; do not require A/B prompt equality.

### Disposable one-second retrieval smoke

Reuse the already retained production metrics from `episode-2483-1hz-technical-e2e-v1`. Do not rerun the disruption, archive, Scenario Store ingestion, or original judges. Run one separate read-only tool call against the exact known query and window:

```text
sum:notification_router.job.enqueue.hits{
  service:notification-router,
  channel:email,
  env:episode-2483-1hz-20260816-164841
}.as_count()
```

Use `2026-08-16T17:02:47Z` through `2026-08-16T17:09:47Z`, `interval: 1000`, `raw_data: true`, and sufficient `max_tokens`. Supplying the already formatted raw query preserves its `.as_count()` semantics and avoids an automatically selected count/rate modifier.

In the current tool contract, `interval` is milliseconds and may be overridden when a request would produce too many points; explicit `raw_data: true` requests CSV points. When `raw_data` is omitted, current `get_metrics.go` auto-selects CSV below 150 returned points and binned output at or above 150; episode 2483's 210 underlying observations therefore remain a valid omitted-field/binned fixture. A generic MCP session cannot represent the Scenario Store organization and is invalid qualification evidence. DDEval is the supported scenario-bound path: it resolves the Scenario Store UUID and forwards its `org_id`, `archive_context`, and event timestamp to the crawler. Because there is no standalone DDEval endpoint for an exact `get_datadog_metric` preflight, the single natural-behavior capability E2E below is also the authoritative retrieval check. It passes retrieval only if it exposes at least 20 consecutive points with exactly 1,000 ms timestamp spacing, matches the independently preserved backend query evidence, and reports no truncation, interval override, or additional automatic semantic modifier. Do not create a second forced investigation to preflight the same route.

### Versioned episode 2483 Bits capability E2E

After crawler provenance and the persisted Scenario Store identity checks pass, run exactly one new DDEval investigation against the existing episode 2483 Scenario Store record `f0db7c0e-df73-4a95-9973-9a2b1778a8ee` using the deployed `global_1hz_eval_v1` profile. Reuse the retained telemetry and original alert context; do not rerun workload, disruption, archive, ingestion, or the original investigation configuration. This is a new prompt-capability experiment version, not a retry or replacement of `episode-2483-1hz-technical-e2e-v1`. Give it new DDEval, investigation, Temporal, LLMObs, and judge identities; preserve the original result unchanged.

Use one iteration, `-j 1`, the frozen model/tools/crawler configuration, and the unchanged normal judges. Do not force a tool call through the user prompt or preseeded result. Pass only if the rendered LLMObs system message contains the exact profile instruction and the model naturally:

1. calls `get_datadog_metric` with explicit `interval: 1000` and `raw_data: true`;
2. receives model-visible raw timestamps with at least 20 consecutive 1,000 ms spacings for an eligible Agent-owned series, with no interval override or truncation;
3. inspects or accurately characterizes effective resolution and gaps; and
4. uses the fine-grained evidence materially in its diagnosis rather than merely issuing the request.

If the model omits the call, requests coarse/binned data, receives an override, or fails to use the evidence, record the capability E2E as failed and do not rerun it. Judges remain observational and the result is non-confirmatory: exclude it from pilot variance, sample-size selection, efficacy, and safety estimates. Store its complete trace, retrieval record, rendered prompt, tool request/response, judge outputs, and identity chain as a labeled derivative beside the immutable original evidence.

The completed v1 capability run requested `interval: 60000` with `raw_data: true` and therefore failed terminally. A later one-iteration default-model probe resolved to `anthropic/claude-opus-4-6`, found the correct root cause, but omitted both `interval` and `raw_data`; Anthropic LLMObs spans expose only the latest message rather than the system blocks, so prompt delivery is supported by the pinned source path and run metadata but cannot be read back verbatim from that trace.

### Mandatory one-second capability v2

`global_1hz_eval_v2_required` is a separate, evaluation-only capability profile. It leaves v1 and all normal defaults unchanged. For every `get_datadog_metric` request it requires `interval: 1000` and `raw_data: true`, bounds each request window to 15 minutes, requires longer windows to be split, and requires inspection of returned timestamps, effective interval, gaps, truncation, and overrides before claiming one-second coverage. This changes the question from advisory selection to mechanical compliance and retrieval capability; it is not a retry, scored arm, prompt-efficacy result, or confirmatory observation. The completed v2 run is a one-off confirmation that the investigator can request and interpret eligible one-second metrics when required. The confirmatory experiment uses the frozen main default prompt for A and additive advisory profile `global_1hz_eval_v3` for B; mandatory v2 is used in neither arm.

The v2 source is commit `9519d6962fddb3cbb025ddc16b59389dc27cbace` on `ella/global-1hz-eval-prompt-current`. It is deployed only to `chatbot-crawler-shadow-8` as crawler digest `sha256:267d12d93778b9e2d61cdb370f8bd0f1ab0616b099cac570984ff550dbb0bd2a`, with MCP digest `sha256:43667c871ed384f6ba45bb0b8e49c10c2df52ad581edb95e3af7d4a9651aa4a5`, one crawler worker, and zero observed container restarts. The guarded one-iteration run `episode-2483-1hz-required-capability-v2` uses the production eval default model, `-j 1`, the same immutable Scenario Store record and telemetry, and no user-prompt forcing or seeded results. It was launched only by the experiment owner and completed successfully: all six metric requests used `interval: 1000`, `raw_data: true`, and windows no longer than 15 minutes; both eligible Agent-owned enqueue series returned 900 complete one-second points with no nulls or structural gaps. Three trace-stat queries returned their effective 10-second resolution and one returned no data, and Bits explicitly reasoned about the 10-second coverage rather than claiming universal one-second resolution. The run scored match probability `0.9600688` and DeepJudge `100`, but remains nonconfirmatory mechanical-capability evidence and does not replace the terminal v1 natural-behavior failure. No retry is allowed.

Durable v2 evidence identities are Temporal workflow run `01a01585-0d03-772b-a70f-bcb0cdcbd774`, job `2da52f5f-f1a3-4302-a7b2-b4be347923ee`, LLMObs experiment `2da00ac4-b93a-42f7-b378-c89724e067a8`, and 95-span agent trace `6a847c410000000089bdcfe9c4cd5618`. The append-only evidence ledger, including per-call retrieval records and SHA-256 hashes of the preserved raw artifacts, is `/Users/ella.taira/episode-bundles/1hz-runs/episode-2483-1hz-20260816-164841-derivatives/bits-1hz-evidence-ledger.md`.

### Scored-trace retrieval evidence

Do not force `interval` or `raw_data` in scored investigations. Natural omission, coarse requests, and binned output remain valid end-to-end results. Production crawler logs redact tool arguments and results, so export and hash the complete LLMObs trace for every run and derive a machine-readable retrieval record for every metric-tool call containing:

- query/formula, filters, requested time range, `interval`, `raw_data`, and `max_tokens`, preserving omitted fields as null;
- returned format (`CSV raw` or `20-bin statistics`), timestamps or bin boundaries, point/bin count, and actual adjacent spacing;
- emitted/total series, unique groups, truncation warnings, automatic modifiers, and no-data warnings;
- requested-versus-returned spacing and an explicit override classification; and
- whether the model-visible representation contained genuine one-second points, repeated/coarser points, or only summaries.

The analyzer must derive spacing from the model-visible payload rather than infer it from Agent/backend qualification. It is deterministic instrumentation, not an LLM judge: the existing new-score judge and DeepJudge prompts, models, and scoring remain unchanged. A missing or inaccessible trace is preserved in the attempted-capture ledger and does not authorize a semantic rerun: retain its judge outcome, mark retrieval-mechanism evidence missing, and report the imbalance by arm.

The dependency-free implementation is `tools/gensim_global_1hz/analyze_llmobs_metric_retrieval.py`, with focused tests, usage documentation, and an episode 2483 source-classification policy in the same directory. Eligibility is policy-driven and defaults to unknown; cadence alone never qualifies a source. Validation against the original technical E2E, advisory v1, default-model probe, and mandatory v2 produced four derivative retrieval records with no parse errors. The records reproduce the frozen behavior: original and default-model calls omitted interval/raw selection and returned binned statistics; advisory v1 requested and received 60-second raw points; mandatory v2 made six compliant requests, exposed two complete eligible one-second series, observed three coarser trace-stat series, and received one no-data response.

## Resolved design decisions

1. The scaled experiment has only A and B; it estimates the end-to-end effect of a bundled global-1-Hz telemetry and advisory-guidance intervention.
2. The experiment does not identify the isolated effect of Agent cadence or prompt text. Any component-level causal claim requires additional factorial arms and a new version.
3. A uses the exact default prompt from the frozen main baseline. B uses that complete prompt plus advisory profile `global_1hz_eval_v3`, third commit `0545086d867fadba86053056012d3afeb27613fb` on the preserved v1 → v2 → v3 PR history. These exact variants are frozen for later dataset additions after Phase 0 deployment readback. Mandatory profile `global_1hz_eval_v2_required` is not part of the confirmatory configuration.
4. Forced one-second retrieval is not a scored arm. The completed v1 run remains the terminal advisory-selection capability result. The separately versioned, one-off v2 run confirms mandatory mechanical compliance only; it does not replace v1 and is never included in efficacy, safety, pilot variance, or sample-size estimates.
5. Pilot variance determines the minimum detectable effect and efficacy/safety/cost thresholds, which are preregistered before scale.
6. Both source-eligible `mixed_secondary` scenarios remain frozen panel members and are reported separately; they are not promoted into the resolution-exposed effect after outcomes are seen.
7. Three `resolution_irrelevant` incidents are selected by snapshot-seeded SHA-256 ranking as the 20% safety/generalization panel.
8. Report the resolution-exposed paired effect, complete-panel operational utility, and neutral/mixed/irrelevant safety results separately; null and negative outcomes remain valid evidence.
9. Allow one predefined replacement only for enumerated pre-investigation infrastructure failures; retain every attempt and never rerun a completed semantic result.
10. Report an attempted-capture ledger and failure imbalance alongside a per-protocol quality estimate on fully qualified matched pairs.
11. Keep the authoritative schemas, selection manifest, and analysis code in the repository; store large immutable evidence externally and record exact IDs and hashes in the repo manifest.
12. Keep arm mapping private from judges and the blinded analyst until inclusion, exclusions, judge outputs, and analysis code are frozen; arm-aware operational validators may access it but must not perform the blinded outcome analysis.
13. Use opaque arm-neutral scenario and capture identifiers and require a pre-DDEval leakage audit. The additive B guidance is a registered treatment exposure, not accidental leakage; it may mention fine-resolution retrieval but must not name the arm, expected root cause, or expected outcome.
14. Treat the four pilot capture sets as design data and exclude them from the confirmatory efficacy estimate.
15. Use the equal-capture-weighted paired full-chain correctness difference as the primary estimand with frozen uncertainty and safety rules.
16. Use the append-only attempted-capture ledger schema and lifecycle defined above; replacement eligibility is determined before outcome inspection.
17. Freeze and hash one blocked randomization schedule before launch; completed schedules are not adapted to observed outcomes.

## Phase exit requirements, owners, and due points

An unresolved item blocks exit from its named phase; it is not deferred generically to “before scale.” Every phase must name accountable people, link the decision or evidence artifact, and record approval in the manifest or append-only decision log.

| Phase | Current status | Required owner roles | Completion requirements before exit |
| --- | --- | --- | --- |
| **0 — Protocol and intervention freeze** | In progress | Experiment owner; Bits/crawler owner; evaluation owner | Approve the bundled estimand; freeze the exact main baseline for A and additive v3 commit for B; freeze both rendered prompt fixtures, crawler digests, agent configs, model, tools, strategy, timeout, maximum iterations, and judges; name ingestion and paid-launch approvers; choose evidence bucket/path, writers, retention policy, and metric-retention scheduling margin; set pilot and scale budget ceilings; name blinded-analysis and arm-aware operational owners. |
| **1 — Scenario selection** | Outcome-blind source panel selected; capability smokes and owner approval pending | Scenario curator; GenSim app owners; judge owner | Preserve `gensim-global-1hz-corpus-freeze.json`, its 96-entry decision ledger, pinned source hashes, ground truths, 7/3/2/3 composition, and deterministic resolution-irrelevant reserves. Run the remaining nonconfirmatory capability smokes and approve the frozen membership before matched execution. No valid result may be replaced because its observed effect is null or negative. |
| **2 — Durable controller readiness** | In progress; remote runner, state store, onboarding, and one-pair qualification pending | Experiment operations owner; GKE/build owner; archive owner; secret-management owner | Complete and retain the authentication/onboarding contract in `gensim-global-1hz-episode-ctl-campaign-plan.md`: dedicated non-human runner identity, least-privilege GKE/Cloud Build/GAR/state/archive/evaluation access, secret delivery/rotation, private arm-map access, and no-write authentication preflight. Then approve the remote runner, campaign lease/state store, artifact/archive destination, and retention policy; build/test the per-arm immutable workspace renderer and allowlisted A/B diff; prove Agent image/config, log/APM topology, matched resources, unique namespace/`DD_ENV`, write-ahead/reconciliation, and runtime workload-image parity. Content-addressed build reuse remains an optional scale improvement. |
| **3 — Agent and telemetry qualification** | Partially complete; managed-topology proof pending | Agent experiment owner; metric-source owners; GenSim runtime owner | Freeze matched non-throttled resources; record exact normal cadence by eligible source; approve exclusions beyond NTP for rate-limited or unsafe checks; prove treatment activation and overlap safety; define preliminary pilot guardrails for collision, missing seconds, duplicates, gaps, point loss, restart, OOM, throttling, serializer/drop, latency, and cost. |
| **4 — Durable remote controller qualification** | Not started | Experiment operations owner; GKE/build owner; archive owner; evaluation operations owner | The experiment owner executes one staging A/B pair only after Phase 2 approval. Prove serialized build/deploy, per-arm immutable workspaces, runtime workload/Agent image parity, expected false/true toggle assignment, concurrent episode phases, logs/traces/metrics, numeric event identity, archive and Scenario Store readback, scenario-bound DDEval, LLMObs export, cleanup, crash/resume reconciliation, and all returned identities. |
| **5 — Four-scenario design pilot** | Not started; blocked by phases 0–4 | Experiment owner; statistician/analysis owner; operational validators | Run the preregistered four-scenario pilot only; estimate between-capture and within-capture variance; choose the minimum detectable effect and independent confirmatory capture-set count; freeze efficacy, safety, latency, cost, cadence-integrity, failure-imbalance, stopping, replacement, concurrency, iteration-count, and budget thresholds; exclude pilot captures from confirmatory efficacy. |
| **6 — Confirmatory launch and scale** | Not authorized | Paid-launch approver; blinded-analysis owner; arm-aware operations owner | Verify all prior exit artifacts and approvals; freeze and hash manifest membership, reserves, randomization schedule, A/B routing, analysis code, evidence destinations, retention deadlines, and launch budget; run the leakage audit; confirm no unresolved identity, capacity, cleanup, qualification, or governance blocker remains. Only then authorize staged confirmatory launches. |

A phase may reopen before confirmatory outcomes exist, but every affected downstream artifact must be regenerated and reapproved. After confirmatory outcomes exist, changing the estimand, prompt, model, thresholds, membership, replacement rules, or analysis contract requires a new experiment version.
## Immediate next actions

1. Complete Phase 1 by first authoring source-grounded judge cases for current panel episodes 2574, 2497, 2500, and 2601, removing nonmember cases from the execution set without deleting their historical library entries, then running repeated calibration for exactly the 15 frozen members. Record judge versions and per-case decisions, obtain judge/experiment-owner approval, and update the outcome-blind manifest without changing membership or ground truths.
2. Complete Phase 0 prompt freeze from one freshly fetched dd-source main baseline: build/read back A's unmodified default prompt and crawler digest, then have the experiment owner deploy/read back B's additive `global_1hz_eval_v3` commit `0545086d867fadba86053056012d3afeb27613fb` and crawler digest, obtain Bits owner review, and freeze separate routing identities. Do not use mandatory v2 in either arm, and do not deploy v3 without the experiment owner.
3. Approve the remote runner, durable campaign lease/state store, archive/artifact destination, retention policy, and credential authority; implement and test the immutable per-arm workspace renderer, write-ahead state machine, reconciliation, and deployment identity gate described in `gensim-global-1hz-episode-ctl-campaign-plan.md`.
4. Verify the frozen Agent digest in each selected bundle's approved GKE topology with matched non-throttled resources, record normal cadence by source, and approve exclusions and preliminary telemetry guardrails. Do not infer current-panel topology from the retired maintained-app study.
5. Have the experiment owner run one staging A/B qualification pair, proving serialized build/deploy, workload-image parity, expected Agent toggle assignment, concurrent episode phases, logs, traces, metrics, numeric event identity, archive and Scenario Store readback, scenario-bound DDEval, LLMObs export, cleanup, and crash/resume reconciliation.
6. Before the four-scenario pilot, review observed Cloud Build fan-out, GAR/GKE/Datadog/archive headroom, cleanup, and controller resume evidence. Treat content-addressed build reuse as optional scale work unless runtime workload-image parity fails or the owner makes it a pilot prerequisite.
7. Keep the implemented LLMObs retrieval-record analyzer (`tools/gensim_global_1hz/`) and scenario classification policies in lockstep with the trace schema and frozen dataset manifest; preserve each generated record beside its immutable source trace.
8. After phases 0–4 pass, freeze an outcome-blind four-scenario design-pilot subset with one source-defined member from each current stratum, then run only that pilot and freeze sample size, efficacy/safety thresholds, concurrency, iteration count, and budget before any confirmatory launch.
