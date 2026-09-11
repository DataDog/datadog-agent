# How GenSim tests whether 1 Hz metrics improve Bits AI SRE

## What this experiment is

GenSim creates repeatable incidents in a controlled Kubernetes environment. Bits AI SRE is then asked to investigate each incident from the telemetry that was captured. This experiment tests whether collecting Agent metrics every second helps Bits find the correct cause—or instead adds no value, noise, delay, or false confidence.

Each scenario has two otherwise matched captures:

- the **control capture** uses the normal Agent metric-resolution behavior;
- the **1 Hz capture** enables the bounded one-second metric-resolution behavior; and
- **DDEval** runs Bits against each archived capture and records investigation quality, evidence use, safety, latency, and cost.

The captures are called opaque arms because Bits must not be told which one is control or 1 Hz. Evaluators may know the assignment when they compare the results.

## Precursor: e2e Smoke Run

Before scaling, a historical episode 2483 smoke test exercised one frozen 1 Hz capture through archival, Scenario Store, and a Bits investigation. Bits made six metric calls; two responses contained complete one-second points, and the run produced DeepJudge 100 with match probability 0.9600688. See the [actual Bits investigation](https://app.datadoghq.com/bits-ai/investigations/1dab9720-bd55-4a08-b32b-c1eee9c33091) and its [Temporal workflow](https://temporal.us1.prod.dog/namespaces/v1:aiplatform-0/workflows/episode-2483-1hz-technical-e2e-v1/01a00d06-f075-7c9d-b247-cfaaccaed985).

The smoke proved that Bits can request and use 1 Hz data. It used a different capture and had no matched control, so it is **plumbing evidence, not evidence that 1 Hz improves Bits**.

<details>
<summary>Why the smoke is not a scored result</summary>

A treatment-effect claim requires control and 1 Hz captures of the same scenario with all other inputs held constant. The smoke had only the 1 Hz capture. Its scores show that the pipeline can work, but they are not copied into any current experiment arm.

</details>

## Branches and external references

| Reference | What it contains |
|---|---|
| [GenSim campaign-controller branch](https://github.com/DataDog/gensim/tree/ella/global-1hz-episode-campaign-controller) | The remote campaign controller, frozen manifests, derivative compiler, packet-provenance ledger, observers, archive helpers, and tests. Audited head: `bb927e366703e8ea740358941e47d3c56ddb59b8`. |
| [Datadog Agent experiment branch](https://github.com/DataDog/datadog-agent/tree/ella/global-1hz-metric-resolution-experiment) | This E2E guide, the corpus and scoring matrices, and their validation tools. |
| [Bits prompt PR 58419](https://github.com/DataDog/dd-source/pull/58419) | The original Bits evaluation-prompt work for testing one-second metrics. |
| [Refreshed Bits prompt branch](https://github.com/DataDog/dd-source/tree/ella/global-1hz-eval-prompt-current) | The same evaluation behavior replayed onto current `dd-source` main. Audited head: `a62c3b59669b9f4f96ff0049b2a32b04823169b3`. |
| [Eval Data Portal projection code](https://github.com/DataDog/dd-source/blob/84fad2bd23d857958da9670d00c60cdfc46caad6/domains/ai_platform/apps/eval-data-portal/internal/ingestion/parser.go#L406-L415) | The audited source for the currently deployed archive-to-Scenario-Store projection. |

## How the smoke becomes a scaled experiment

The smoke answered “can Bits receive and use one-second data?” The scaled experiment asks “does one-second data improve or regress Bits across different incidents?” Answering that requires a frozen 15-scenario panel, matched control and 1 Hz captures, automated evidence handling, explicit exclusions, and paired scoring. The next section is the step-by-step workflow for running that experiment at scale.

## How to run the experiment at scale

### 1. Freeze inputs

**Objective:** create matched packets whose only A/B difference is the metric-resolution treatment.

The original scenario packets could not all run unchanged with the bounded experiment Agent, so the team made narrow, outcome-blind compatibility adaptations:

- all 15 selected packets use one Agent replica, disable the process Agent, remove node-local mounts, exclude only NTP, pin the Agent image/configuration, and vary only the resolution toggle;
- 2498 and 2597 use a separate empty-environment rule because those two source packets encode an empty Helm value with single quotes (`env: ''`) while the others use double quotes (`env: ""`). The rule changes only how the compiler matches the existing YAML; it does not assign a different environment;
- 2483 removes exactly two `new_group_delay: 120` fields that Datadog rejects for simple-alert monitors.

These packet adaptations appear in the [version-3 derivative ledger](https://github.com/DataDog/gensim/blob/bb927e366703e8ea740358941e47d3c56ddb59b8/src/controller/campaigns/global-1hz-derivative-change-ledger-v3.json), corpus matrix, and scoring matrix. After normalizing the resolution toggle, each A/B pair is checked for byte-and-mode equality. Never patch a frozen packet in place, and never expose treatment assignment in Bits-visible inputs.

### 2. Preflight the campaign

**Objective:** prove that the exact approved inputs are safe to mutate remotely before creating a workload or monitor.

Run the controller’s no-write checks against exact packet, manifest, ledger, controller, and target hashes. Confirm unique namespaces, environment-scoped monitors, valid phase windows, an empty evidence destination, `max_parallel=1`, and `automatic_retry=false`. Freeze and approve the resulting digest-bound execution plan.

### 3. Run captures

The approved route uses the GenSim campaign controller around existing `episode-ctl` phases:

```text
episode-ctl deploy → readiness checks → run-episode → observe → verify
```

We did not use managed `gs-episode-worker` because that route could not guarantee exact frozen-bundle pinning and was blocked by Atlas authorization, lifecycle-state, RCA metadata, and incomplete bootstrap handling. The direct `episode-ctl` route already supported the frozen packets and required evidence without changing episode behavior.

#### Why we did not stagger or parallelize captures

Parallelism was limited by more than Kubernetes capacity:

- one `episode-ctl deploy` can fan out to several Cloud Build jobs, so limiting deploy processes does not fully limit builds;
- same-scenario builds publish under mutable scenario/service image tags and can race;
- overlapping arms can compete for cluster resources or contaminate monitor and telemetry windows; and
- archival and Scenario Store ingestion did not have experiment-qualified parallel limits.

A considered alternative was to build/deploy A and B sequentially, verify identical runtime image IDs, and then stagger or run only the episode phases side by side. That was difficult because `episode-ctl` did not expose its internal `skip_build` option, a content-addressed prebuild contract did not exist, and a fixed time delay cannot prove that a deployment or telemetry stream is ready. Full serial execution (`max_parallel=1`) was the smallest defensible choice for this campaign.

If the experiment grows, revisit concurrency rather than simply removing the limit. A safe scale-up needs content-addressed image tags or a tested prebuild/skip-build path, per-stage capacity limits, readiness and image-parity gates, isolated monitor windows, and a qualification run that measures Cloud Build, registry, GKE, telemetry, archive, and ingestion behavior. Independent scenarios may then be parallelized within measured limits; same-scenario pairs still need explicit contamination controls.

Campaign `max_parallel=1` limits active campaign arm workers, not Cloud Build subjobs started by one deploy. If an arm fails, queued arms are cancelled without retry or replacement. Process success and observation status remain separate.

### 4. Preserve evidence and clean up

Snapshot monitor definitions before teardown. Preserve phase results, event observations, logs, command output, hashes, and attempt/lifecycle ledgers without modifying source result files. Then remove the namespace, Helm release, monitors, and campaign-owned RBAC, and record leak-reconciliation receipts.

<details>
<summary>Compressed controller safety contract</summary>

Each remote mutation has a write-ahead intent and stable identity. Ambiguous writes are reconciled rather than replayed. Success, failure, cancellation, compromise, exclusion, and recovery evidence all remain retained. Cleanup failure never authorizes a replacement capture.

</details>

### 5. Archive through sims-controller

Submit the exact digest-bound archival request through the local `workspaces proxy` to `sims-controller.us1.prod.dog`. Allow at most one POST and zero automatic retries. If submission is ambiguous, reconcile read-only rather than replaying it. Any separately approved recovery uses a fresh S3 root.

### 6. Validate the archive directly

Treat workflow success as provisional. Validate every `metadata.json`, referenced object path, object size, archive identity, event/monitor/ground-truth field, and diagnostic track using the approved read-only S3 profile.

### 7. Preview and ingest through Eval Data Portal

For each eligible arm, send only its exact S3 prefix with `dry_run:true` and `force:false`. Review the projected Scenario Store record and verify that diagnostic telemetry exists. After explicit approval, send one exact non-dry-run request with `force:false`; do not use wildcards, forced ingestion, or a manually trimmed payload.

### 8. Verify Scenario Store

Read back the created scenario and dataset. Verify UUID, organization, status, source path, event identity, monitor, ground truth, archive context, and uniqueness in the [Scenario Store admin portal](https://mosaic.us1.ddbuild.io/bits-ai-sre-admin-portal/scenario-store-scenarios/?dc=us1.prod.dog). Freeze primary, supplementary, and excluded manifests only after readback passes.

### 9. Run DDEval

Run only verified Scenario Store UUIDs with the frozen crawler, model, tools, prompts, judge, concurrency, and iteration settings. Each child loads the full scenario from Scenario Store; local or inline selection is not an alternate execution backend. Follow the [DDEval Alert Eval Confluence quick start](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6833180773/ddeval+Running+Alert+Eval#Quick-start%3A-your-first-eval).

Preserve workflow IDs, Bits investigation IDs, LLMObs traces, judge output, metric requests and responses, errors, latency, iterations, tokens, and cost.

### 10. Score paired outcomes

Populate the evaluator-only matrix from exact iteration artifacts. Retain raw values and paired `1 Hz - control` deltas. Keep null, negative, regressive, failed, and inconclusive outcomes; do not collapse them into an unapproved composite score.

<details>
<summary>Agreed Bits evaluation criteria</summary>

1. **Admission gates:** execution success, usable event identity, archive integrity, diagnostic telemetry, and Scenario Store readiness. These determine eligibility; they are not quality points.
2. **RCA quality:** pass/inconclusive state, match probability, DeepJudge, immediate cause, deeper causal chain, and remediation quality.
3. **Metric retrieval:** whether Bits queried the intended metric and correct scope, requested `interval=1000` and `raw_data=true`, used a window no longer than 15 minutes, and received complete one-second points without gaps or backend override.
4. **Evidence use:** whether fine-resolution evidence changed the reasoning, supported the causal chain, tested alternatives, and used negative controls.
5. **Safety:** false 1 Hz claims, contradictions introduced by noise, contradiction count, and evidence-bounded remediation.
6. **Reliability and efficiency:** duration, reasoning iterations, tool and metric calls, tool errors, tokens, and cost.

Primary analysis compares raw values and paired `1 Hz - control` deltas within each scenario. Cohorts describe DDEval readiness, not treatment assignment or outcome grade.

</details>

The evaluator matrix is staged at the [scoring-matrix path on the experiment branch](https://github.com/DataDog/datadog-agent/blob/ella/global-1hz-metric-resolution-experiment/docs/dev/gensim-global-1hz-scoring-matrix.md). That URL resolves after this documentation batch is committed; until then, the complete criteria are compressed above.

## Why identity linkage matters

Capture, archival, Scenario Store ingestion, and DDEval happen in different systems and at different times. A stable identity chain prevents evidence from one arm being attached to another, proves that Bits investigated the intended capture, and lets a reviewer trace every score back to immutable source data:

```text
source/derivative hashes → campaign attempt → event identity
→ archive workflow and S3 metadata → Scenario Store UUID
→ DDEval/Bits/LLMObs IDs → scoring row
```

Missing or ambiguous links remain explicit evidence gaps; they are not guessed or repaired by rerunning the arm.

## Experiment requirements

These requirements protect the validity and reproducibility of the experiment:

- **Matched inputs:** within a scenario, control and 1 Hz packets must be identical after normalizing the resolution toggle and approved compatibility adaptations.
- **Frozen provenance:** an approved packet cannot be edited. “v3” is the version-3 provenance ledger for the final derivative set; the selected corpus uses 14 v3 scenario pairs plus the separately frozen corrected 2601 pair.
- **Agent blinding:** treatment labels and `uses_1hz` stay evaluator-only and must not enter scenario text, archive enrichment, prompts, or other Bits-visible context.
- **Serial scientific execution:** run one capture at a time unless a separately qualified concurrency contract exists. Never automatically retry or replace an attempted arm.
- **Stable identities:** every remote mutation, capture, archive, Scenario Store record, DDEval run, and scoring row must be linked by recorded identifiers.
- **Ambiguous-write safety:** if a remote POST may have been accepted, reconcile its stable identity read-only; never replay it speculatively.
- **Complete accounting:** preserve failed, cancelled, excluded, compromised, recovery, and superseded evidence alongside successful results.
- **Cleanup discipline:** save monitor definitions first, then remove workloads, monitors, and campaign-owned access, recording receipts for each check.
- **Archive integrity:** directly validate S3 metadata and referenced objects before declaring an archive usable.
- **Controlled ingestion:** submit exact prefixes with `force:false`; after a blocker or code change, regenerate previews and obtain explicit approval before another write.
- **Multidimensional scoring:** retain admission, RCA, retrieval, evidence-use, safety, and efficiency results separately, including null and regressive outcomes.

## Related documents

- [GenSim campaign-controller branch](https://github.com/DataDog/gensim/tree/ella/global-1hz-episode-campaign-controller)
- [Datadog Agent experiment branch](https://github.com/DataDog/datadog-agent/tree/ella/global-1hz-metric-resolution-experiment)
- [Bits 1 Hz prompt PR 58419](https://github.com/DataDog/dd-source/pull/58419)
- [Refreshed Bits prompt branch](https://github.com/DataDog/dd-source/tree/ella/global-1hz-eval-prompt-current)
- [Historical Bits investigation](https://app.datadoghq.com/bits-ai/investigations/1dab9720-bd55-4a08-b32b-c1eee9c33091)
- [Archived episode S3 access runbook](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/5629050883#S3-Access-for-Archived-Episode-Data)
- [DDEval Alert Eval quick start](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6833180773/ddeval+Running+Alert+Eval#Quick-start%3A-your-first-eval)

## Status update — 2026-09-10

This section is a dated progress log, not part of the experiment requirements above. The [evaluator scoring matrix](https://github.com/DataDog/datadog-agent/blob/ella/global-1hz-metric-resolution-experiment/docs/dev/gensim-global-1hz-scoring-matrix.md) is the row-level source for hypotheses, packet adaptations, readiness, and future Bits results.

Capture, observation, cleanup, archival, and direct archive validation are complete. During execution, malformed generated source was discovered in both original 2601 packets. The selected 2601 uses `corrected_2601_v1`, which repairs the generator, 12 generated service `app.py` files, and 12 matching readiness templates; the original attempts remain preserved and excluded.

| Item | Current state |
|---|---:|
| Scenarios | 15 |
| Arms | 30 |
| Successful selected archive workflows | 30 |
| Archived rows | 93,052,467 |
| Archived items | 21,626 |
| Validated metadata documents | 30 |
| Validated referenced S3 objects | 21,398 |
| DDEval candidates | 25 |
| Current-arm DDEval runs | 0 |

Readiness summary:

- **Primary analysis:** 24 arms forming 12 complete pairs passed the current event-identity and telemetry gates.
- **Supplementary analysis:** one arm passed independently, but its partner did not have usable event identity, so it cannot produce a paired delta.
- **Excluded from DDEval:** five arms remain in the corpus but failed either the event-identity gate or the diagnostic-telemetry gate. Exact arm-level reasons are retained in the scoring matrix.

**Current stop point:** Scenario Store ingestion is blocked in Eval Data Portal. The first and only authorized non-dry-run request created no scenario and returned:

```text
rpc error: code = InvalidArgument desc = enrichment_contexts[0].data exceeds maximum size of 131072 bytes (got 138863 bytes); store large payloads out-of-band and reference them by URI
```

The empty dataset `72c8193a-4e6c-40cf-8120-510623bd1295` remains unchanged as evidence. No retry or additional write followed. The embedded `archive_objects` array dominates the payload: 21 of 25 candidate projections exceed the limit, and the four that fit contain no complete pair.

Before any further ingestion:

1. remove `archive_objects` from embedded `gensim_metadata` while retaining required identity, ground-truth, time, and exact S3-reference fields;
2. make dry-run execute production-equivalent Scenario Store validation;
3. deploy and verify the fix;
4. regenerate exact `dry_run:true`, `force:false` previews for the 25 candidates; and
5. obtain fresh explicit authorization.

DDEval and current-arm scoring remain pending until Scenario Store ingestion and readback succeed.
