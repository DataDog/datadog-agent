# Metric Resolution Evaluation with GenSim and Bits AI SRE

## Objective

Measure whether Bits AI SRE produces better root-cause investigations when a controlled GenSim incident includes 1 Hz metrics instead of normal-cadence metrics.

This experiment excludes Observer, anomaly-detection components, and the legacy datadog-agent GenSim EKS runner. Its data path uses the current GenSim control plane:

```text
gs-episode-worker
  -> gs-flow selects a Terrapin tier and runs sim-generator
  -> simulated environment runs the pinned Datadog Agent and incident
  -> Agent sends metrics through the normal metrics backend
  -> Sims Archiver publishes logs, traces, events, metadata, and execution lineage
  -> Eval Data Portal registers that historical context in Scenario Store
  -> Bits AI SRE runs now against the historical scenario
       -> archive tools read the captured non-metric context
       -> the metric tool queries the normal backend at the capture's timestamps
  -> DeepJudge and remediation judges score each investigation
```

The incident runs live only during GenSim capture. DDEval does not rerun the workload or fault: it starts a new Bits investigation against historical evidence. Per the [Bits Eval team clarification](https://dd.slack.com/archives/C07TUJB9FQA/p1785957892930239?thread_ts=1785944181.807409&cid=C07TUJB9FQA), the metric tool does not read Sims Archiver metric Parquet; it queries the normal metrics backend and is limited by its approximately 15-month retention. The [Metrics Platform clarification](https://dd.slack.com/archives/C04HBT86H9Q/p1786041218146099?thread_ts=1786038676.108649&cid=C04HBT86H9Q) confirms that the backend maintains the source data and returns the high-granularity points when the query interval is set low enough. Therefore historical backend compaction is not the expected loss mechanism; the remaining risk is an automatically or explicitly coarse query interval. Sims metric Parquet is optional preservation evidence, not an input or gate for this evaluation.

`gs-admin execution_id` is the provenance spine for each capture. Keep gs-flow job IDs, Sims Archiver workflow IDs, archive roots, Scenario Store UUIDs, DDEval workflow/run IDs, and LLMObs trace IDs as narrower identities linked to that execution.

## References

- [GenSim — Use Cases & Applications](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/5629083664): controlled incidents should provide a bounded window, known cause, monitor behavior, negative controls, and enough telemetry for competing hypotheses.
- [GenSim — Sims Archiver](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/6237454343): publishes immutable execution archives with checksums and metadata. Its metric Parquet is not consumed by the current Bits metric tool.
- [GenSim — Execution, Scheduling, Eval, and Index](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/7021135064): defines current gs-admin execution identity, producer ownership, gs-flow execution, and downstream evaluation lineage.
- [GenSim — Scheduled Sets](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/7020545321): defines immutable evaluation membership and distinguishes source executions from physical evaluation attempts.
- [Eval Data Portal](https://datadoghq.atlassian.net/wiki/spaces/AIP/pages/6649577703): hydrates archives for query access and provides ingestion endpoints that register GenSim archives in Bits Scenario Store.
- [What is an Eval?](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6571982883): describes the ReAct investigation strategy, archived scenario model, and DeepJudge scoring.
- [DDEval — Running Alert Eval](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6833180773): current Bits AI SRE evaluation procedure using DDEval and Temporal.
- [GenSim — Getting Started](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/5629050883): GenSim setup, credentials, repositories, and service entry points.
- [GenSim — How to Run an Episode](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/6489702531): episode lifecycle, telemetry verification, and the supported archival handoff.
- [Selecting GenSim Scenarios](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/6253085259): scenario-selection criteria.
- [Eval Scoring](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/6286902480): baseline, disruption, cooldown, and GenSim metadata contract.
- [1 Hz Metric Path Requirements](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/6770557146): fine-resolution metric requirements and constraints.
- [MSFT AI Workgroup — 1 Hz Metrics](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/6667175594): product context and check-overrun findings.
- [Bits AI SRE Evaluation FAQs](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6443074123): Scenario Store, datasets, run operations, and support channels.
- [DeepJudge](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6082167763): root-cause scoring model and penalties.
- [Interpreting Alert Eval Results](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/5006917773): per-iteration and per-scenario result fields and comparison workflow.

Do not use `q_branch/gensim-eval-scenarios.json` for scenario selection. That manifest predates the current Bits Alert Eval workflow. Start with the current DDEval quick-start scenario, then choose a maintained GenSim app or episode from the current catalog.

## Authentication and access preflight

Complete these checks before starting an experiment. Never paste API keys, application keys, bearer tokens, kubeconfigs, or temporary AWS credentials into this document, shell history, archive metadata, or test output.

### Local tools

**Read first:** [GenSim — Getting Started](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/5629050883).

Confirm that the required clients are installed:

```bash
command -v git atlas temporal ddtool dd-auth bzl kubectl jq curl
```

`dda`, `aws-vault`, and the Agent E2E setup are not prerequisites for the canonical service-managed GenSim path. Use them only for separate datadog-agent repository work.

### Current GenSim execution-plane access

**Read first:** [GenSim — Execution, Scheduling, Eval, and Index](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/7021135064), [GenSim — Sims Archiver](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/6237454343), and the gs-flow and gs-episode-worker runbooks in `dd-source`.

Do not provision or submit through `tasks/e2e_framework/aws/gensim_eks.py`; that is the legacy Agent EKS/Observer harness and is outside this experiment. Do not use `episode-ctl` to create the canonical captures described here.

The current production execution path is gs-episode-worker -> gs-flow -> Terrapin -> sim-generator. gs-flow's `generator_image` is not a Datadog Agent image override: production Terrapin resolves simulator bytes and snapshots from its named tier. The current `gs-flow` request and `gs-episode-worker` workflow do not expose a documented per-execution Agent image or integration configuration field.

GenSim owner feedback on 2026-08-05 narrowed the ownership boundary: a custom simulator image requires a Terrapin tier entry, while the Datadog Agent image and configuration are part of the scenario's own deployment and therefore require a blueprint change. The immediate investigation must locate the maintained blueprint source, identify how it renders the Agent DaemonSet/Deployment, and prove whether a digest-pinned image plus bounded config can be changed without mutating shared Terrapin state. See the [GenSim thread](https://dd.slack.com/archives/C09FH4THJTH/p1785949454279909?thread_ts=1785878213.523029&cid=C09FH4THJTH).

Before creating either arm, obtain an owner-approved execution command or API workflow that:

- creates a canonical gs-admin execution UUID;
- reports the gs-flow job ID and resolved Terrapin tier/snapshot;
- pins the episode, blueprint/app-bundle, sim-generator, and Agent image identities;
- supports one reviewed custom Agent image in both arms plus a bounded per-execution global metric-resolution setting without changing application bytes; and
- finishes through gs-episode-worker's managed Sims Archiver handoff.

The Agent treatment contract is defined in `docs/dev/gensim-global-1hz-custom-agent-plan.md`. Both arms use one immutable custom Agent digest. Control disables the experimental setting; treatment enables one-second scheduling for every normal recurring check, one-second ordinary DogStatsD aggregation, and one-second serializer flushing while preserving timestamped DogStatsD's no-aggregation semantics. Prefer typed bounded fields forwarded through gs-episode-worker to the Agent deployment. An immutable deployment definition is acceptable only if GenSim owners confirm it as the supported boundary and retain the complete normalized diff.

This missing custom-image and per-execution configuration surface is a hard blocker, not permission to fall back to the legacy EKS runner or to repoint the shared Terrapin tier.

Human S3 write access is not required for the canonical production route: gs-episode-worker and Sims Archiver write through service Emissary IAM roles. Do not manually write or modify a canonical archive. Request an AWS permission set only for a documented direct-inspection or local-staging workflow:

- `gensim-s3-operator` covers only `dd-gensim`, `dd-gensim-staging`, and `dd-gensim-temp`;
- `datascience-operator` is required for `dd-applied-ai-research-us1-prod`, where production GenSim archives are stored; and
- the local AtlasLite gs-episode-worker helper requires an AWS profile with `dd-gensim-staging` access because its embedded archiver writes real staging objects.

Read-only archive validation is optional for this experiment because service-returned execution/archive identities and Eval Data Portal hydration are the primary evidence.

#### Request direct S3 validation access

**Read first:** [GenSim — Datasets](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/5644911007), [GenSim — Getting Started](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/5629050883#S3-Access-for-Archived-Episode-Data), and [Human Access FAQ](https://datadoghq.atlassian.net/wiki/spaces/CLOUDA/pages/4197320586/Human+Access+FAQ).

For this experiment, request these team-level grants through a `DataDog/cloud-inventory` PR:

| Environment | AWS account | Permission set | Intended use |
| --- | ---: | --- | --- |
| Production | `464622532012` | `datascience-operator` | List/read canonical archives under `s3://dd-applied-ai-research-us1-prod/gensim/` |
| Staging | `727006795293` | `gensim-s3-operator` | Inspect `dd-gensim-staging`; support the local AtlasLite helper when owner-approved |

Do not request production `gensim-s3-operator` for the canonical archive: it covers `dd-gensim`, `dd-gensim-temp`, and the legacy GenSim buckets, not `dd-applied-ai-research-us1-prod`.

Request workflow:

1. Clone `DataDog/cloud-inventory` and start Claude from its repository root.
2. Run `/managing-human-access`.
3. Request the grants above for team `q-branch`, with the business reason: paired GenSim/Bits DDEval validation needs direct list/read checks of production archive metadata and staging access for controlled local-helper validation; no direct writes to canonical production archive prefixes.
4. Require passing CI, address Codex review, and obtain teammate approval.
5. Share the PR in `#cloud-accounts-support` for Cloud Accounts review.
6. Wait for the merged change to apply, then refresh the AWS SSO session.

After approval, add profiles with the documented account and role names; local profile names are arbitrary:

```ini
[profile sso-prod-datascience-operator]
sso_start_url = https://d-906757b57c.awsapps.com/start
sso_account_id = 464622532012
sso_role_name = datascience-operator
sso_region = us-east-1
region = us-east-1

[profile sso-staging-gensim-s3-operator]
sso_start_url = https://d-906757b57c.awsapps.com/start
sso_account_id = 727006795293
sso_role_name = gensim-s3-operator
sso_region = us-east-1
region = us-east-1
```

Validate without mutating canonical objects:

```bash
aws-vault login sso-prod-datascience-operator
aws-vault exec sso-prod-datascience-operator -- aws sts get-caller-identity
aws-vault exec sso-prod-datascience-operator -- \
  aws s3 ls s3://dd-applied-ai-research-us1-prod/gensim/ --page-size 10

aws-vault login sso-staging-gensim-s3-operator
aws-vault exec sso-staging-gensim-s3-operator -- aws sts get-caller-identity
aws-vault exec sso-staging-gensim-s3-operator -- \
  aws s3 ls s3://dd-gensim-staging/gensim/ --page-size 10
```

Use `aws s3api head-object`, bounded `aws s3 ls`, and a local copy of `metadata.json` for intermediate validation. Never test write access by modifying an existing archive; let gs-episode-worker/Sims Archiver create the objects. If a manual write test is explicitly required, coordinate a unique disposable prefix with the GenSim owners first.

### Datadog internal identity

**Read first:** [DDEval — Running Alert Eval](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6833180773).

Authenticate to both the production service plane and the build plane used by `bzl`:

```bash
ddtool auth login --datacenter us1.prod.dog
ddtool auth whoami --datacenter us1.prod.dog

ddtool auth login --datacenter us1.ddbuild.io
ddtool auth whoami --datacenter us1.ddbuild.io
```

Use `--force` with `ddtool auth login` only when cached credentials cannot be renewed normally. Do not print tokens; use `ddtool auth token` only when a documented client explicitly requires one.

### Sims Archiver and Temporal

**Read first:** [GenSim — Sims Archiver](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/6237454343) and the archival section of [GenSim — How to Run an Episode](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/6489702531).

Use Atlas with appended identity headers rather than constructing Temporal credentials manually. This read-only preflight confirms that the production worker is polling:

```bash
atlas context exec --context prod --append-auth-headers -- \
  temporal task-queue describe --tls --task-queue sims-archiver
```

For this experiment, do not start archival directly through Temporal, `episode-ctl`, or sims-controller. The canonical gs-episode-worker execution must own the Sims Archiver handoff so the archive remains linked to the gs-admin execution and its resolved execution context.

Sims Archiver writes S3 through its Emissary IAM role. Local AWS credentials are not needed for the worker's S3 writes. Direct S3 inspection is optional and requires a separately authorized data-science or GenSim operator role.

### Eval Data Portal

**Read first:** [Eval Data Portal](https://datadoghq.atlassian.net/wiki/spaces/AIP/pages/6649577703), especially its hydration, query, and scenario-ingestion sections.

Use `dd-auth` to inject short-lived Datadog credentials into `curl`. Verify service access without mutating state:

```bash
dd-auth --domain dd.datad0g.com -- \
  curl -fsS https://eval-data-portal.us1.prod.dog/health
```

Use the staging hostname for staging archives. Keep `dd-auth` outside `curl` exactly as shown so credentials are injected into the child process instead of appearing in command arguments or files.

### Scenario Store API, optional Mosaic, and Bits DDEval

**Read first:** [What is an Eval?](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6571982883), [Bits AI SRE Evaluation FAQs](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6443074123), and [DDEval — Running Alert Eval](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6833180773).

The [Mosaic Scenario Store](https://mosaic.us1.ddbuild.io/bits-ai-sre-admin-portal/scenario-store-scenarios/?dc=us1.prod.dog) is an optional browsing and dataset-management surface. Portal authorization is not documented as a requirement for API ingestion or DDEval. If Mosaic is unavailable, retain the scenario and dataset UUIDs returned by Eval Data Portal and use those identifiers directly. Request Mosaic access only if visual inspection or UI-based dataset management is needed.

Run the Bits DDEval command from an up-to-date `dd-source` checkout after the `ddtool` production and build-plane checks above. A shadow crawler is needed only when evaluating modified Bits agent or tool code. If a shadow is required, use `shadow-eval apply`, accept its recommended available shadow, and announce the reservation and worker count in `#bits-alert-eval-headsup` before launching the run.

#### Workspace connectivity and shadow deployment

As of 2026-07-31, a standard Datadog Workspace cannot independently perform this production shadow deployment. `shadow-eval apply` needs both the production Vault endpoint for its reservation and the Centurion production Kubernetes API, while direct `*.prod.dog` access is blocked from Workspaces. `workspaces proxy` can forward those endpoints from a managed laptop, but the forwarding process and Appgate connection must remain active; `tmux` preserves the Workspace process but not that network path. Do not answer yes to the CLI's fallback prompt when the reservation cannot be acquired.

As of 2026-08-03, shadow 4 with one worker has been announced and deployed successfully through the approved release path. The release Temporal execution (`531c5947-29c7-4af4-a949-c4147a12aa41`, run `019fc8c4-dc77-777e-b62f-d1494a3753a0`) returned `SUCCESSFUL`; Kubernetes then reported one updated, ready, and available `chatbot-crawler-shadow-4` replica with zero unavailable replicas. This establishes the Step 0 deployment prerequisite only; it does not create a DDEval run, GenSim episode, archive, Scenario Store scenario, or dataset.

### Preflight acceptance

**Read first:** the operational checks in [GenSim — Sims Archiver](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/6237454343), [Eval Data Portal](https://datadoghq.atlassian.net/wiki/spaces/AIP/pages/6649577703), and [DDEval — Running Alert Eval](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6833180773).

Proceed only when:

- `ddtool auth whoami` succeeds for production and the build plane;
- an owner-approved gs-episode-worker launch path and Agent configuration boundary are documented;
- the selected episode/app bundle, sim-generator, Terrapin runtime, and Agent image can be pinned;
- the Sims Archiver task queue has at least one workflow poller and one activity poller;
- Eval Data Portal health returns success;
- the current Bits DDEval runner is accessible; and
- the scenario can supply a valid trigger, ground truth, and Scenario Store projection.

Scenario Store identifiers are outputs of the capture/ingestion steps, not preflight prerequisites.

### Immediate GenSim investigation phase

Complete two GenSim gates before beginning the canonical matched matrix. Gate A proves the custom Agent deployment; Gate B proves the custom Agent, normal metrics backend, and Bits live historical-query path. The Bits metric route is no longer blocked on archive routing because the metric tool does not consume a metrics archive.

#### Gate A: custom Agent blueprint contract

Work with GenSim owners to identify the maintained scenario blueprint or app-bundle source that installs the Datadog Agent. Record:

```text
blueprint repository, path, revision, and owner
rendering component and deployment resource name
current Agent image source and immutable digest support
app-local Helm environment injection surface and immutable arm-bundle provenance
registry, platform, signing, scanning, and pull requirements
whether the Agent is baked into a snapshot or installed at execution time
managed launch command/API and gs-admin execution linkage
running workload image-ID read-back command
effective Agent config read-back command
archive/execution fields that retain image and config provenance
```

Prepare one immutable throwaway blueprint revision using the custom Agent digest and bounded treatment configuration. Do not edit a shared Terrapin tier, use `generator_image` as an Agent selector, or bypass gs-episode-worker. Gate A passes only when the running workload proves the expected image digest and effective one-second check, ordinary DogStatsD bucket, and serializer-flush settings.

#### Gate B: throwaway 1 Hz live-query fidelity capture

Use the selected smoke contract in `docs/dev/gensim-global-1hz-smoke-manifest.json`: the maintained `pharmacy-replicaepoch-store` app with `episodes/prescription-read-serves-prior-epoch.episode.yml` at `gensim-apps` revision `01cf1863321359982ddeaa38e341024440526f1f`. The episode lasts 915 seconds and uses one API, PostgreSQL, and a continuous 12-user Locust workload. Inject a smoke-only Agent custom check through app-local Helm values that emits the uniquely tagged monotonic `metric_resolution.smoke.sequence` gauge once per second for 60–120 seconds. Also validate the app's existing `pharmacy.replica_epoch.apply_ms` DogStatsD timing stream: dispenses are task weight four of seven with a constant 0.5-second wait, and the app's datadog-go v5.6.0 client does not enable extended client aggregation, so timing samples reach the Agent without timing aggregation. The throwaway run validates scheduled-check transport, ordinary DogStatsD aggregation, and query routing only; it is not a control/treatment pair and must not contribute to the quality result.

Trace the same series through:

```text
source/check fixture or DogStatsD packets
Agent serializer payload or intake-visible series
normal Datadog metrics backend query
Bits metric-tool request and response in the DDEval trace
```

Retain the telemetry org/datacenter, environment and exact tags, capture bounds, expected values, backend query, and raw returned rows. Calculate point count, timestamp bounds, median/p95 interval, maximum gap, missing seconds, duplicates, out-of-order points, and value changes. Delivery may be batched only when original timestamps remain distinct.

Register the execution as a disposable Scenario Store smoke scenario through the normal managed archive path so Bits receives the same execution's historical logs, traces, events, alert, and metadata. In the DDEval trace, require the metric tool to query the same telemetry scope and historical window and return useful fine-resolution points without an unexpected coarse rollup.

Any timestamp collapse, wrong org/window/tags, missing metric, or unexplained point loss is a stop condition. Sims Archiver metric Parquet and metric hydration may be inspected, but they are optional and do not gate Gate B. Gate B ends after custom-image read-back, live-backend fidelity, and one Bits historical metric query are proven.

## Experiment plan

### 1. Define the matched capture contract

**Read first:** [GenSim — Use Cases & Applications](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/5629083664), [GenSim — Execution, Scheduling, Eval, and Index](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/7021135064), [Selecting GenSim Scenarios](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/6253085259), and [What is an Eval?](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6571982883).

Choose maintained incidents with a known root cause, a short-lived metric change that normal cadence can obscure, competing evidence, stable execution behavior, and metadata that projects cleanly into Scenario Store. Do not use `q_branch/gensim-eval-scenarios.json`.

#### Ground-truth and judge-quality preflight

Historical GenSim labels are discovery evidence, not automatically judge-ready ground truth. Before selecting an incident, audit the exact short Scenario Store `description` consumed by DDEval against the richer trajectory metadata. Require a clean source revision; explicit root cause, affected component, failure type, impact or invariant, activation proof, recovery proof, and negative controls; and no contradictions among the short description, long description, native taxonomy, and structured ground truth. Use only records that satisfy this contract as written; do not repair or mutate historical GenSim scenarios for this experiment.

Calibrate the maintained DeepJudge with four evidence-bounded conclusions per scenario: canonical correct, symptom-only, wrong mechanism, and an explicit distractor. Extract the ground-truth chain once per scenario and repetition, reuse that extraction for all four variants, and run three independent repetitions. A scenario passes only when the canonical conclusion finds the immediate cause and scores at least the maintained 50-point threshold in every repetition while every negative variant remains below 50 in every repetition. Retain every extraction, score, contradiction count, rationale, and content hash.

```bash
cd "$DD_SOURCE"
bzl run \
  //domains/ai_platform/apps/apis/bits_ai_eval_runner/tools:deepjudge_calibration -- \
  calibration-input.json \
  --output calibration-results.json \
  --cache calibration-results.jsonl \
  --repeats 3 \
  --concurrency 6
```

The initial six-scenario calibration passed only two labels. Two short labels allowed explicit distractors to score 100, one causal label allowed a reversed-causality distractor to cross the threshold in one repetition, and one label let a symptom-only answer score 50–67. This confirms the Slack warning that historical labels can make judge scores misleading. Exclude failed labels rather than modifying them. If the existing high-quality Scenario Store pool is insufficient, inspect additional maintained GenSim source scenarios directly and admit only scenarios whose existing definitions and projected labels pass the same quality gate unchanged. DeepJudge passing establishes label/judge discriminability only and does not establish scenario eligibility or 1 Hz relevance.

The subsequent full read-only export found 1,163 GenSim rows, 1,057 active rows, 971 distinct active app/description candidates, and 695 unchanged candidates with an explicitly clean source revision, complete structured ground truth, negative controls, native taxonomy, alert, monitor evidence, and a maintained app. A first diversity screen produced 14 retained candidates across ten native incident families and nine apps after three-repeat DeepJudge calibration, known-adversarial-failure exclusion, and new-score characterization. Keep two reported tiers rather than overfitting selection to one judge: a core tier where both maintained judges reject all tested negatives, and an extended diversity tier where DeepJudge remains causally discriminative but new-score is permissive. Expected 1 Hz exposure is an analysis annotation (`exposed`, `neutral`, or `mixed_or_unknown`), never an eligibility gate.

The frozen discovery manifest is `docs/dev/gensim-global-1hz-dataset-manifest.json`. Its historical Scenario Store UUIDs are provenance seeds, not final dataset members. The dedicated `gensim-global-1hz-bits-paired-v1` dataset remains uncreated until fresh managed control/treatment captures exist. It will contain only those fresh captures; the plumbing-only smoke is excluded. The selected apps are `adspend-checkpoint`, `ambulance-dispatch-zone-fairness`, `care-authcache-session`, `delivery-policydrift-controlplane`, `dogschedule`, `inventory-aging-query-parity`, `pharmacy-replicaepoch-store`, `qdrant-shard-recovery`, and `supplierhub-mysql`.

For each matched pair, pin and retain:

```text
pair_id
canonical episode and blueprint/app-bundle versions
sim-generator image digest
Terrapin tier and resolved snapshot ID
Agent image digest
complete `metric_resolution_experiment` configuration
metric-source coverage inventory and target causal series
expected root cause and trigger
control and treatment time bounds
gs-admin execution UUID for each arm
DDEval runner revision, crawler, model, tools, strategy, judge config
```

The control and treatment must use the same workload, source artifacts, sim-generator bytes, Terrapin runtime, Agent image, trigger, and ground truth. The intended behavioral difference is the single global experiment-mode setting implemented by the same custom image:

| Arm | Agent configuration |
| --- | --- |
| Control | `metric_resolution_experiment.enabled: false` |
| Treatment | Global one-second recurring-check scheduling, ordinary DogStatsD aggregation, and serializer flushing as specified in `gensim-global-1hz-custom-agent-plan.md` |

This is a global multi-source intervention, not a selected-check experiment. Timestamped DogStatsD remains producer-timestamped and bypasses ordinary aggregation; producer-controlled OTLP/runtime cadence and dedicated non-metric payloads must be classified separately. The treatment claim is limited to Agent-owned metric resolution that survives the normal backend and the Bits metric query. Managed archival is evaluated separately for non-metric scenario context.

### 2. Produce canonical control and treatment executions

Use the owner-approved gs-episode-worker/gs-flow/Terrapin path identified in preflight. Do not run the legacy Agent EKS task, use Observer, or directly submit a gs-flow job if that bypasses canonical gs-admin execution creation and managed archival.

For each arm, retain the complete non-secret request preview and service-returned identities:

```text
gs_admin_execution_id
gs_flow_job_id
episode_id
blueprint_version
app_bundle_version
execution type and tags
resolved Terrapin tier and snapshot ID
sim-generator digest
Agent image digest
Agent config digest
environment/telemetry filter
start and end timestamps
```

Require the effective Agent configuration from inside the simulated environment before the incident starts. It must prove that all non-cadence fields match and that only the treatment instance resolves to one second.

### 3. Validate live-backend and Bits query fidelity

Metric Lookback is prior evidence that the normal backend supports distinct one-second points. For this experiment, verify that the custom Agent's new global path emits the intended series through that backend. At fixture/check output and the direct backend query, retain timestamps, values, complete tags, telemetry org/datacenter, and capture bounds. Compute:

```text
point_count
first and last timestamps
median and p95 inter-point interval
maximum gap
missing-second count
duplicate timestamp count
out-of-order count
```

Then inspect a DDEval trace and require the Bits metric tool to use the same org, environment/tags, and historical time window. The treatment is invalid if Bits queries the wrong scope/window or returns only a coarse rollup despite fine-resolution backend points.

Separately require active Sims Archiver workflow/activity pollers and a successful managed archive for the execution's logs, traces, events, alert metadata, and Scenario Store lineage. Retain archive identities and failure counts, but do not gate metric validity on Sims metric Parquet or metric hydration.

### 4. Register blinded paired scenarios

The standard Bits Alert Eval corpus described in “What is an Eval?” is primarily shared archival of real past incidents from Org 2 and customers. GenSim is not the assumed source of the default weekly powerpacks. This experiment uses a supported custom path: Eval Data Portal discovers GenSim archives, hydrates them, and its single/batch ingest APIs project GenSim metadata into `bits-ai-eval-rpc` Scenario Store explicitly for downstream DDEval evaluation (AIPAE-811).

Here, **projector** means the translation implemented by `domains/ai_platform/apps/eval-data-portal/internal/ingestion/parser.go`: it reads a GenSim archive's `metadata.json` and constructs a `bitsaievalpb.Scenario`. It does not copy the entire Sims Archiver archive into Scenario Store and it does not establish that every archived telemetry product is queryable by Bits.

The current projector intentionally separates the two data paths:

- it creates archival-manager queries for the EVP tracks `trace`, `logs`, `errors`, `netflow`, `netpath`, and `feed` (`gensimEVPTracks`);
- it does not create a metrics archival query because Bits queries metrics from the normal backend;
- it strips `metrics_catalog`, `results`, and `archiver_config` from the Scenario Store enrichment payload;
- it may create a Scenario Store row with zero telemetry queries when the whole capture window is outside archival-manager's retention window;
- malformed or absent start/end timestamps, monitor definitions, alert identity, or trajectory narrative can yield a failed create or a structurally valid but investigation-useless scenario; and
- `force: true` bypasses source-path idempotency and can create a duplicate rather than update an existing scenario.

Scenario Store creation is therefore the historical non-metric context and control-plane gate. Before publishing either arm, require all of the following:

1. Sims Archiver `metadata.json` is present and its requested non-metric outputs, object hashes, monitor sidecars, and execution linkage are complete.
2. Direct `POST /api/v1/scenarios/ingest` with `dry_run: true` returns parseable start/end timestamps, a real alert event ID and timestamp, a useful root-cause narrative, monitor definition, `archive_context`, and `husky_snapshot_namespace: gensim`.
3. The projected scenario has non-empty archival queries, and every requested query window is inside supported retention.
4. The selected metric and exact tags are queryable from the normal metrics backend at the capture's timestamps.
5. The resulting Scenario Store UUID reaches `ARCHIVAL_TELEMETRY_SCOPE_STATUS_COMPLETE`; `NOT_STARTED`, pending, failed, or legacy-fallback-only replay is not acceptable.
6. A real Bits investigation against that UUID uses archived non-metric context while its metric tool queries the normal backend with the correct org, tags, historical window, and useful resolution.

If the Bits metric-tool check fails while the direct backend query passes, stop and correct eval-time routing or query rollup. Do not add a metrics archival query as a workaround: the current Bits metric tool does not consume it.

Preview each archive through `POST /api/v1/scenarios/ingest` with `dry_run: true` before publishing it to Scenario Store. Use `force: false`; if a capture or projection is wrong, create a new immutable archive root and scenario rather than force-duplicating or mutating the published one. Treat HTTP 200 as insufficient: require `failed == 0`, expected `created`/`skipped` status, non-empty scenario and dataset UUIDs, and exact dataset membership after linking. Prefer the direct single-scenario endpoint for new captures because batch ingestion depends on the asynchronously refreshed `scenarios_discovery` catalog.

Preserve the root cause, trigger, time bounds, and expected evidence. Give control and treatment distinct immutable archive, scenario, and dataset identifiers, but use neutral arm labels in agent-visible content so the investigator is not told which cadence it received. Use a dedicated paired dataset; do not add experimental captures to the default weekly powerpacks.

Retain the archive root and version, hydration ID and status, Scenario Store scenario UUID, dataset UUID, gs-admin execution UUID, exact dataset-membership manifest, ingestion response, and the private `pair_id`/arm mapping.

### 5. Prove Bits can consume the treatment resolution

Before quality scoring, inspect one treatment investigation trace and require that Bits:

- calls the metric tool;
- requests the selected metric over the intended window;
- receives one-second timestamps without a coarse rollup; and
- can reference the transient ordering in its reasoning.

Classify every later iteration as: metric not queried; queried but rolled up; one-second points returned but ignored; immediate cause improved; deeper causal chain improved; or high-frequency noise introduced a contradiction.

### 6. Run a variance-estimation pilot

The native quick-start smoke run completed three iterations with a scenario-level pass but a highly variable DeepJudge distribution. Its report showed iteration scores of 20, 40, and 100, mean 53.3, one of three above 50, immediate-cause success in all three, and no remediation match. Therefore three iterations are adequate for harness validation but not for a product conclusion.

After the full control and treatment route passes, run one matched capture pair with:

```text
10 DDEval iterations per arm
-j 1
same crawler deployment, model, tools, strategy, judge config, and timeout
```

Run the arms close together and alternate arm order in later pairs. This pilot estimates agent-level variability and validates retention. It does not establish that 1 Hz is generally beneficial.

### 7. Retain every iteration and all provenance

For every DDEval iteration, retain:

```text
pair_id, capture_id, arm, scenario_id, iteration
passed, is_inconclusive, match_probability, deepjudge_score
complete new_score_json, deepjudge_json, remediation_json
agent conclusion and ground truth
job/investigation ID and LLMObs trace URL
metric queries, requested windows, raw tool responses, and tool errors
duration, agent iterations, tokens/model usage when available
```

Also retain the DDEval workflow/run IDs, Temporal and LLMObs URLs, S3 `results.json`, local `--save-results` output, report permalink or DM, and exported scenario- and iteration-level Datadog logs. The executor round-trips complete judge JSON by `job_id` (`ddeval/bits_sre_agent/executor.py`), but the service artifacts remain the source of record for iteration arrays and workflow provenance; do not preserve only the Slack aggregate.

For each arm report the raw DeepJudge distribution, mean, median, standard deviation, interquartile range, minimum/maximum, proportion above 50, immediate-cause rate, pass/inconclusive rates, deeper-cause findings, contradictions, remediation recall/F1, and investigation cost.

### 8. Scale across independent matched captures

The independent experiment unit is the matched GenSim capture pair, not each LLM iteration. DDEval iterations measure stochasticity conditional on one capture and must not be presented as independent telemetry experiments.

After the pilot passes, use at least five independent matched capture pairs. Prefer several relevant incident types; if only one maintained incident is valid, rerun it independently. Start with 10 iterations per arm per capture and increase to 20 only if the pilot shows that additional within-capture sampling is needed.

Analyze treatment-minus-control differences for mean/median DeepJudge, score-above-50 rate, immediate-cause rate, pass/inconclusive rate, contradictions, remediation, latency, and tool use. Aggregate across capture pairs with paired intervals and show the raw distributions. A positive claim additionally requires traces showing that Bits consumed the one-second evidence and no material regression in contradictions, inconclusive results, latency, or remediation safety.

## Smoke test

The smoke test builds outward from the maintained Bits Alert Eval workflow:

1. prove native DDEval with the current runbook's known scenario;
2. run one maintained GenSim incident at normal cadence through archival, ingestion, and DDEval; and
3. only then repeat that exact incident with the same custom Agent image in global 1 Hz treatment mode.

This sequence separates DDEval access, GenSim projection, and high-resolution telemetry failures. It is not intended to prove product value.

### Workspace topology and command conventions

**Purpose:** keep long-running work inside the Workspace while making the laptop-only production route, repository context, and evidence locations explicit. Every later command names its required checkout so that it can be resumed safely after reconnecting.

Use two terminals:

1. **Managed laptop terminal:** owns Appgate and `workspaces proxy`. It must remain awake, online, and connected for every command that reaches the production Vault or Centurion Kubernetes API.
2. **Workspace terminal:** owns the `tmux` session, source checkouts, GenSim run, archival, ingestion, and DDEval commands. `tmux` preserves processes across SSH disconnects, but it does not preserve the laptop's proxy or Appgate path.

On the managed laptop, discover the Workspace, install its SSH alias, and start the required production tunnels. Replace only `WORKSPACE_NAME`; keep this foreground process running:

```bash
export WORKSPACE_NAME="REPLACE_ME"
workspaces list

# Run this only when the named Workspace does not already exist.
workspaces create "$WORKSPACE_NAME" \
  --repo dd-source \
  --branch main \
  --shell zsh

workspaces ssh-config "$WORKSPACE_NAME"

workspaces proxy "$WORKSPACE_NAME" \
  --tunnel vault.us1.release.mgmt.dog:443 \
  --tunnel vault.us1.prod.dog:443 \
  --tunnel k8s-centurion.us1.prod.dog:443 \
  --tunnel v1-aiplatform-0.us1.prod.dog:443 \
  --tunnel bits-ai-eval-admin-api.us1.prod.dog:443 \
  --tunnel eval-data-portal.us1.prod.dog:443
```

In a second laptop terminal, connect to the Workspace:

```bash
export WORKSPACE_NAME="REPLACE_ME"
ssh "workspace-$WORKSPACE_NAME"
```

Inside the Workspace, create or reattach the durable shell:

```bash
tmux new-session -A -s metric-resolution-smoke
```

After the shell appears inside `tmux`, define checkout locations. Shell-local values do not survive a new SSH login, so rerun this block after reconnecting. The preferred defaults use `$HOME/dd`; the fallback preserves older Workspaces whose `dd-source` checkout is directly under `$HOME`. Override them before running the block if the repositories live elsewhere:

```bash
if test -z "${DD_SOURCE:-}"; then
  if test -f "$HOME/dd/dd-source/MODULE.bazel"; then
    export DD_SOURCE="$HOME/dd/dd-source"
  elif test -f "$HOME/dd-source/MODULE.bazel"; then
    export DD_SOURCE="$HOME/dd-source"
  else
    echo 'dd-source checkout not found; set DD_SOURCE explicitly' >&2
    return 1 2>/dev/null || exit 1
  fi
fi

export EVIDENCE_DIR="${EVIDENCE_DIR:-$HOME/metric-resolution-smoke/$(date -u +%Y%m%dT%H%M%SZ)}"

mkdir -p -m 700 "$EVIDENCE_DIR"

# Record only source revisions that actually participate in this service-managed
# experiment. GenSim execution artifacts and image digests are recorded later
# from gs-admin/gs-flow responses rather than inferred from a local clone.
if revisions="$(
  set -euo pipefail
  test -d "$DD_SOURCE/.git"
  printf 'dd-source=%s\n' "$(git -C "$DD_SOURCE" rev-parse HEAD)"
)"; then
  printf '%s\n' "$revisions" | tee "$EVIDENCE_DIR/revisions.txt"
else
  echo 'dd-source checkout validation failed; revisions.txt was not updated' >&2
fi
```

Do not write environment dumps, credentials, kubeconfigs, or authentication output to `EVIDENCE_DIR`. Record only stable IDs, revisions, timestamps, non-secret request previews, and service responses that contain no credentials.

### Workspace one-time setup and preflight

**Purpose:** fail before reserving a shadow or creating infrastructure if a required binary, identity, route, or worker is unavailable.

Run inside the Workspace `tmux` session. A `dd-source` Workspace does not necessarily include Temporal CLI, so install the official Homebrew package when needed:

```bash
if ! command -v temporal >/dev/null; then
  if command -v brew >/dev/null; then
    brew install temporal
  else
    echo 'Homebrew is required to install Temporal CLI in this Workspace' >&2
  fi
fi

command -v temporal >/dev/null && temporal --version
```

Check the complete tool list without using `exit`, which would terminate the Workspace login shell:

```bash
missing_tools=""
for tool in git ddtool dd-auth bzl atlas temporal kubectl jq curl; do
  command -v "$tool" >/dev/null || missing_tools="$missing_tools $tool"
done

if test -n "$missing_tools"; then
  echo "missing tools:$missing_tools" >&2
  false
else
  echo 'required tools are available'
fi
```

Do not continue until the final command prints `required tools are available`. Then run:

```bash
ddtool auth whoami --datacenter us1.prod.dog
ddtool auth whoami --datacenter us1.ddbuild.io

dd-auth --domain dd.datad0g.com -- \
  curl -fsS https://eval-data-portal.us1.prod.dog/health

atlas context exec --context prod --append-auth-headers -- \
  temporal task-queue describe --tls --task-queue sims-archiver
```

The Temporal output must show at least one active poller and no growing workflow backlog. If it does not, stop before running GenSim; a successful episode with no archiver poller cannot complete the smoke route.

For the laptop-proxied production route, validate from the Workspace before invoking `shadow-eval`:

```bash
cd "$DD_SOURCE"
ddtool clusters context centurion.us1.prod.dog
kubectl --context centurion.us1.prod.dog get namespace >/dev/null
atlas task-queue describe \
  --domain aiplatform \
  --datacenter us1.prod.dog \
  --task-queue bits-ai-eval-runner
```

The Atlas check must reach `v1-aiplatform-0.us1.prod.dog:443` and report a poller before DDEval starts. If any command times out, first confirm that the laptop's `workspaces proxy` and Appgate connection are still active. Do not accept `shadow-eval`'s unreserved fallback or retry DDEval until the task-queue check succeeds.

### Preconditions

**Read first:** [GenSim — Getting Started](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/5629050883), [GenSim — How to Run an Episode](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/6489702531), and [DDEval — Running Alert Eval](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6833180773).

- An owner-approved canonical gs-episode-worker execution path is available.
- The selected episode/app bundle and Agent check configuration boundary are known.
- The same Agent image digest can be pinned for both arms.
- Metrics are sent to the GenSim production organization, currently documented as org `1573830`.
- Every check in [Authentication and access preflight](#authentication-and-access-preflight) succeeds.
- The scenario supplies a trigger, ground truth, and replayable execution lineage.

### Step 0: Follow DDEval's first-eval quick start

**Completed (2026-08-04).** The reserved `chatbot-crawler-shadow-4` run completed all three investigations, judging, reporting, S3 publication, Datadog logs/metrics, and LLMObs publication. This proves the native DDEval harness independently of GenSim and metric resolution. Do not rerun Step 0 unless the crawler, DDEval runner, or evaluation configuration changes materially.

**Read first:** [DDEval — Running Alert Eval: Quick start — your first eval](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6833180773/ddeval+Running+Alert+Eval#Quick-start%3A-your-first-eval).

**Purpose:** prove production DDEval, crawler, Temporal, judging, and reporting independently of GenSim and metric resolution. A failure here is a harness failure, not evidence about the experiment.

Follow the maintained quick start without substituting a custom runner path. Inside the Workspace `tmux` session, restore and validate shell-local paths before defining a wrapper; these guards prevent an empty checkout path from leaving the shell in `$HOME` and an empty evidence path from resolving under `/`:

```bash
: "${DD_SOURCE:?rerun the Workspace path bootstrap}"
: "${EVIDENCE_DIR:?rerun the Workspace path bootstrap}"
test -f "$DD_SOURCE/MODULE.bazel"
mkdir -p -m 700 "$EVIDENCE_DIR"
cd "$DD_SOURCE"

shadow-eval() { "$DD_SOURCE/domains/chatbot/scripts/shadow-eval" "$@"; }
shadow-eval --help
```

1. From the current `dd-source` branch, inspect active workers and get the CLI's shadow recommendation:

   ```bash
   cd "$DD_SOURCE"
   ddtool clusters context centurion.us1.prod.dog
   shadow-eval status | tee "$EVIDENCE_DIR/shadow-status.txt"
   ```

   Every shadow row must contain a real deployment age, and worker-usage lookup must succeed. Do not trust the recommendation if rows contain `error` or `kubectl failed`, or if the Mortar request returns `401`; the current CLI can misclassify lookup failures as available shadows. If either authorization is unavailable, ask `#bits-alert-eval-headsup` for a valid shadow instead of selecting one from the failed output.

   Do not run this production preflight from a standard Workspace unless an approved production route is active and will remain available through deployment. A missing `centurion.us1.prod.dog` context or a timeout reaching `vault.us1.prod.dog` is a connectivity blocker, not permission to skip the reservation.

2. Deploy to the recommended shadow explicitly. The current `dd-source` CLI requires `-n`; the Confluence quick start's no-argument example is ahead of or inconsistent with the checked-in interface:

   ```bash
   export SHADOW_NUMBER="REPLACE_ME"
   case "$SHADOW_NUMBER" in REPLACE_ME) echo 'set SHADOW_NUMBER' >&2; exit 1 ;; esac

   shadow-eval apply -n "$SHADOW_NUMBER" \
     | tee "$EVIDENCE_DIR/shadow-deploy.log"
   ```

3. Post `Taking shadow $SHADOW_NUMBER, 1 worker` in `#bits-alert-eval-headsup`.
4. Launch the documented scenario with three iterations and one concurrent investigation:

   ```bash
   bzl run //domains/ai_platform/apps/apis/bits_ai_eval_runner/ddeval:run_eval -- \
     --scenario-ids "3cde4a12-f53f-47e1-bd09-4cd2bfd1d40e" \
     --iterations 3 \
     -j 1 \
     --prod \
     --agent-version "chatbot-crawler-shadow-$SHADOW_NUMBER" \
     | tee "$EVIDENCE_DIR/native-ddeval.log"
   ```

Do not pass `--golden-run`, `--scheduled-run`, or `--test-run`; none is part of the current first-eval quick start. Follow the Temporal and LLMObs URLs printed by the CLI. The observed run's report was delivered by Data Science Evaluation DM because no Slack channel was configured; a specific report channel is not a pass criterion.

Observed run:

```text
scenario UUID: 3cde4a12-f53f-47e1-bd09-4cd2bfd1d40e
crawler: chatbot-crawler-shadow-4
DDEval workflow ID: us1.prod.dog-ella-taira-20260804-e0839c
Temporal run ID: 019fccfc-ff4d-76aa-ba0a-ab1a61503212
DDEval source revision: a4285ceeaf83e317b507d5f1abcf95dcb8768107
iterations / concurrency: 3 / 1
new-score judge: gpt-4o-mini
DeepJudge: gpt-4.1, temperature 0.1
investigation type: alert
worker environment: prod
```

Artifacts:

- [Temporal workflow](https://temporal.us1.prod.dog/namespaces/v1:aiplatform-0/workflows/us1.prod.dog-ella-taira-20260804-e0839c/019fccfc-ff4d-76aa-ba0a-ab1a61503212)
- [LLMObs experiment](https://app.datadoghq.com/llm/experiments/82028aa7-8100-4dbf-853f-30bdd4fd541a)
- `s3://dd-llm-data-us1-prod/ddeval/run_id=us1.prod.dog-ella-taira-20260804-e0839c/results.json`

Observed result:

```text
scenario pass: true
DeepJudge per iteration: 20, 40, 100
DeepJudge mean: 53.3
95% CI: [-50.1, 156.8]
score > 50: 1/3
new-score precision: 100%
immediate cause: 3/3
remediation match: 0/1
remediation recall: 0%
remediation F1 safety: 0%
```

The score spread is evidence that three iterations validate plumbing but do not estimate treatment effect reliably. Retain this run as the harness baseline; the canonical GenSim pair receives its own independent control and treatment Scenario Store UUIDs.

**Pass: complete.** The maintained quick-start scenario completed three investigations, judging, and reporting through the reserved shadow.

### Step 1: Select a maintained GenSim replacement for the live-incident baseline

**Read first:** [What is an Eval?](https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6571982883), [GenSim — Execution, Scheduling, Eval, and Index](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/7021135064), [GenSim — How to Run an Episode](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/6489702531), and [Selecting GenSim Scenarios](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/6253085259).

**Purpose:** keep the validated live-incident DDEval baseline separate from the new reproducible GenSim experiment.

The Scenario Store API establishes that `3cde4a12-f53f-47e1-bd09-4cd2bfd1d40e` is not a GenSim execution:

```text
creation_source: shared_archival
label and agent-context source: sre-feedback
scenario_origin: web_ui
investigation_source_type: monitor-alert
org_id: 2
monitor: [incidents-ai-grpc] Pods not ready in prtest07.prod.dog
archive_context: monitor-trigger-8610148421526988207
archived services: incidents-ai-grpc, ddvector
archival status: complete
```

The incident was a real `prtest07.prod.dog` failure: a missing Vault secret prevented `ddvector` from initializing, which cascaded into `incidents-ai-grpc` readiness failure. Shared archival captured the historical telemetry after feedback validation. The scenario has no gs-admin execution, episode, blueprint/app-bundle, Terrapin, sim-generator, or Agent image lineage.

Searches of the maintained local GenSim repository, `dd-source/domains/sims`, and Confluence found no scenario linked to this UUID or incident description. A separately created derivative could exist in remote gs-admin without being linked, but there is no evidence that this incident was subsequently converted into GenSim.

Therefore:

- retain this UUID and its 20/40/100 results only as the native DDEval harness baseline;
- do not replay its archive as the control arm;
- do not claim that its existing telemetry can be changed to 1 Hz retroactively; and
- create new control and treatment Scenario Store UUIDs from one maintained GenSim episode.

Prefer an existing maintained episode with the same useful causal shape: a short-lived upstream failure followed by a dependent-service readiness or health failure, with metric evidence whose temporal ordering may be obscured at normal cadence and whose source classes can exercise the global treatment. Converting the live incident description through ScenarioGen is a separate episode-development project and adds generation/validation risk; do not make that the first E2E smoke unless no suitable maintained episode exists.

Record the selected replacement:

```text
native baseline UUID: 3cde4a12-f53f-47e1-bd09-4cd2bfd1d40e
selection decision: maintained GenSim replacement
replacement episode ID/version
blueprint and app-bundle version
expected root cause and causal chain
target causal metric series and cadence owner
metric-source coverage represented by the episode
reason the incident tests metric-resolution sensitivity
```

**Pass:** a maintained, canonically runnable GenSim episode is selected; the live archived scenario remains only the harness baseline.

### Step 2: Obtain the supported control/treatment configuration boundary

**Read first:** the gs-episode-worker README and job submission code, the gs-flow API, and the current GenSim execution documentation.

**Purpose:** prevent an unsupported local harness or shared Terrapin mutation from becoming the experiment.

Confirm with the GenSim owners the supported managed boundary for:

1. pinning one immutable custom Datadog Agent image digest in both arms; and
2. selecting normalized immutable app-bundle variants that inject the bounded internal `DD_METRIC_RESOLUTION_EXPERIMENT_*` environment variables through the app-local Datadog Helm dependency.

`RunEpisodeJobRequest` does not carry an Agent image or arbitrary Agent configuration; its `image` field selects sim-generator. Do not describe the worker request as forwarding Agent settings. Unless owners add an approved per-execution override, the arm boundary is the app bundle: both variants pin the same Agent repository/digest and interval values, and only the enabled environment value differs.

Record the approved launch interface and immutable artifact IDs in `gensim-launch-contract.md`. The contract must prove that both arms create gs-admin executions and pass through managed Sims Archiver. Stop if the only available option is the legacy Agent EKS runner, direct `episode-ctl`, a shared Terrapin-tier edit, different Agent images between arms, or an unrecorded configuration mutation.

Before execution, complete the metric-source inventory required by `docs/dev/gensim-global-1hz-custom-agent-plan.md` and record:

```text
target causal metric series and cadence owner
all metric provenance classes expected from the Agent and live backend
control experiment configuration
treatment check, ordinary DogStatsD, and serializer intervals
timestamped DogStatsD and producer-controlled source expectations
expected point tags
expected incident window
```

Retain the identical Agent image digest, both immutable app-bundle identities, and an exact normalized bundle/configuration diff showing that only the global experiment-mode setting differs.

**Pass:** an owner-approved launch/configuration contract exists and both immutable arm definitions can be named before execution.

### Step 2A: Run the throwaway 1 Hz live-query fidelity capture

Before the normal-cadence control, run Gate B using the approved blueprint boundary from Step 2. This run must use a unique environment and metric name and remain excluded from the canonical paired dataset and statistical analysis.

Retain:

```text
THROWAWAY_GS_ADMIN_EXECUTION_ID
THROWAWAY_BLUEPRINT_REVISION
THROWAWAY_AGENT_IMAGE_DIGEST
THROWAWAY_AGENT_CONFIG_DIGEST
THROWAWAY_ENVIRONMENT_ID
THROWAWAY_TELEMETRY_ORG_AND_DATACENTER
THROWAWAY_CAPTURE_START_AND_END
THROWAWAY_ARCHIVE_ROOT
THROWAWAY_SCENARIO_UUID
THROWAWAY_DDEVAL_RUN_ID
THROWAWAY_BITS_TRACE_ID
```

Require running-workload image/config read-back, save the direct normal-backend metric rows and cadence statistics, then register a disposable smoke scenario through the normal managed archive path. Run one DDEval investigation and inspect the metric-tool request and response. Sims metric Parquet and hydration are optional evidence.

**Pass:** the custom image and effective 1 Hz configuration are proven, the normal backend retains distinct one-second timestamps/values/tags, and Bits queries the same historical metric scope without an unexpected coarse rollup.

### Step 3: Verify readiness and execute the normal-cadence control

**Read first:** [GenSim — Sims Archiver](https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/6237454343) and the owner-approved launch runbook from Step 2.

Recheck Sims Archiver immediately before mutation:

```bash
atlas context exec --context prod --append-auth-headers -- \
  temporal task-queue describe --tls --task-queue sims-archiver \
  | tee "$EVIDENCE_DIR/control-sims-archiver-queue.txt"
```

Require at least one workflow poller and one activity poller. Zero backlog without pollers is a stop condition.

Use only the approved gs-episode-worker entry point. Save the exact non-secret request and response, then extract:

```text
CONTROL_GS_ADMIN_EXECUTION_ID
CONTROL_GS_FLOW_JOB_ID
CONTROL_EPISODE_VERSION
CONTROL_APP_BUNDLE_VERSION
CONTROL_TERRAPIN_TIER
CONTROL_TERRAPIN_SNAPSHOT
CONTROL_SIM_GENERATOR_DIGEST
CONTROL_AGENT_IMAGE_DIGEST
CONTROL_AGENT_CONFIG_DIGEST
CONTROL_ENVIRONMENT_ID
CONTROL_RUN_START
CONTROL_RUN_END
```

Before the incident starts, retain `agent configcheck` output from the simulated environment and confirm that the target instance has its maintained normal cadence. Do not manually start Sims Archiver; gs-episode-worker must own the archival transition.

**Pass:** the control gs-admin execution reaches its expected completed/archived state with effective normal-cadence configuration and stable service identities.

### Step 4: Validate, hydrate, and publish the control

Retain normal-backend points for the selected metric and calculate point count, timestamp bounds, inter-point deltas, missing intervals, duplicates, and out-of-order points. Record the telemetry org/datacenter, exact tags, and historical query window. Then inspect the managed archive metadata and require:

- the archive is linked to `CONTROL_GS_ADMIN_EXECUTION_ID`;
- expected monitors, ground truth, incident bounds, logs, traces, and events are represented;
- no required archive activity failed; and
- `metadata.json`, object inventory, and hashes are complete.

Preview Scenario Store ingestion and reject the projection if it has no archival queries, lacks a real alert event ID/timestamp, monitor definition, useful trajectory-derived ground truth, or valid time bounds. Publish with `force: false`, require `failed == 0`, verify exact dataset membership, and wait for archival-manager status `COMPLETE`. The subsequent control investigation—not archive hydration—proves the Bits metric route. Record:

```text
CONTROL_ARCHIVE_ROOT
CONTROL_ARCHIVER_WORKFLOW_ID
CONTROL_ARCHIVER_RUN_ID
CONTROL_HYDRATION_ID
CONTROL_SCENARIO_UUID
CONTROL_DATASET_UUID
```

**Pass:** the normal-cadence execution has verified normal-backend metrics and a valid Scenario Store entry whose archived non-metric context links back to its gs-admin execution.

### Step 5: Complete the GenSim control investigation

Use the same crawler and judge configuration as the native harness validation. One iteration is sufficient for this path smoke; score interpretation waits for the paired pilot.

```bash
: "${CONTROL_SCENARIO_UUID:?complete control ingestion first}"
: "${EVIDENCE_DIR:?restore the Workspace path bootstrap}"
: "${DD_SOURCE:?restore the Workspace path bootstrap}"

cd "$DD_SOURCE"
bzl run //domains/ai_platform/apps/apis/bits_ai_eval_runner/ddeval:run_eval -- \
  --scenario-ids "$CONTROL_SCENARIO_UUID" \
  --iterations 1 \
  -j 1 \
  --prod \
  --agent-version chatbot-crawler-shadow-4 \
  --name "metric-resolution-arm-a-smoke" \
  --save-results "$EVIDENCE_DIR/control-ddeval-results.json" \
  | tee "$EVIDENCE_DIR/control-ddeval.log"
```

Inspect the LLMObs trace and require that the investigation queries the intended metric and receives the expected normal-cadence points. Retain the DDEval workflow/run IDs, S3 result, complete judge JSON, conclusion, trace IDs, and tool responses.

**Pass:** one canonical GenSim control completes live-backend capture, managed non-metric archival, Scenario Store projection, investigation, judging, and trace review.

### Step 6: Execute and validate the matched 1 Hz treatment

Create the treatment from the normalized immutable app-bundle variant at the same episode, sim-generator digest, Terrapin tier/snapshot, and Agent image digest. Its only approved bundle/configuration difference from control is `DD_METRIC_RESOLUTION_EXPERIMENT_ENABLED=true` instead of `false`; both variants explicitly carry the same one-second dormant interval values. The image's bounded treatment settings then apply the global one-second behavior specified in `docs/dev/gensim-global-1hz-custom-agent-plan.md`.

Recheck Sims Archiver pollers, submit through the same approved gs-episode-worker interface, and retain the treatment equivalents of every control identity. Before the incident, retain `agent configcheck` and compare it with the control output.

Reject the pair if any of these differ without prior approval:

```text
episode or workload
blueprint/app-bundle content or Agent configuration outside the bounded experiment-mode switch
sim-generator digest
Terrapin tier or resolved snapshot
Agent image digest
fault sequence, trigger, ground truth, or incident duration
```

Validate the selected metric directly in the normal backend, then require the Bits metric-tool trace to query the same org, tags, and historical window and return useful one-second evidence without an unexpected coarse rollup. Validate the managed archive separately for the treatment execution's non-metric context. Publish a separate Scenario Store scenario and dataset entry, linked privately to the same `pair_id`.

Run one treatment path-smoke investigation:

```bash
: "${TREATMENT_SCENARIO_UUID:?complete treatment ingestion first}"
cd "$DD_SOURCE"
bzl run //domains/ai_platform/apps/apis/bits_ai_eval_runner/ddeval:run_eval -- \
  --scenario-ids "$TREATMENT_SCENARIO_UUID" \
  --iterations 1 \
  -j 1 \
  --prod \
  --agent-version chatbot-crawler-shadow-4 \
  --name "metric-resolution-arm-b-smoke" \
  --save-results "$EVIDENCE_DIR/treatment-ddeval-results.json" \
  | tee "$EVIDENCE_DIR/treatment-ddeval.log"
```

**Pass:** one-second points survive every boundary and the treatment investigation can access them.

### Step 7: Run the paired variance pilot

After both path-smoke investigations pass, run 10 iterations per arm with one worker. Keep crawler, source revision, model, tools, strategy, judge configuration, timeout, and maximum agent iterations identical. Use neutral arm names and run the arms close together.

```bash
cd "$DD_SOURCE"

bzl run //domains/ai_platform/apps/apis/bits_ai_eval_runner/ddeval:run_eval -- \
  --scenario-ids "$CONTROL_SCENARIO_UUID" \
  --iterations 10 \
  -j 1 \
  --prod \
  --agent-version chatbot-crawler-shadow-4 \
  --name "metric-resolution-arm-a-pilot" \
  --save-results "$EVIDENCE_DIR/control-pilot-results.json" \
  | tee "$EVIDENCE_DIR/control-pilot.log"

bzl run //domains/ai_platform/apps/apis/bits_ai_eval_runner/ddeval:run_eval -- \
  --scenario-ids "$TREATMENT_SCENARIO_UUID" \
  --iterations 10 \
  -j 1 \
  --prod \
  --agent-version chatbot-crawler-shadow-4 \
  --name "metric-resolution-arm-b-pilot" \
  --save-results "$EVIDENCE_DIR/treatment-pilot-results.json" \
  | tee "$EVIDENCE_DIR/treatment-pilot.log"
```

Retain the local result files plus service S3 results, iteration/scenario log exports, Temporal IDs, LLMObs traces, and reports. Compare raw score distributions and metric-use classifications; do not make a product claim from this single capture pair.

### Scale-up gate

Scale beyond the first pair only when:

- native DDEval harness validation is recorded as passed;
- the scenario has canonical replay lineage, or a maintained replacement episode is documented;
- the owner-approved launch path creates gs-admin executions through gs-episode-worker;
- control and treatment use the same pinned sim-generator, Terrapin runtime, and Agent image;
- the normalized Agent config diff contains only the selected cadence change;
- Sims Archiver had workflow and activity pollers for both arms;
- backend, archive, hydration, and Bits queries preserve their intended timestamps;
- control and treatment have separate immutable archive, scenario, and dataset identifiers;
- one smoke investigation and judge completes per arm;
- the 10-iteration pilot retains every iteration and complete judge/tool evidence; and
- every remaining operational blocker has an owner and explicit stop condition.

After the pilot, use at least five independent matched capture pairs. Alternate arm order and start with 10 DDEval iterations per arm per capture. Treat capture pairs—not individual LLM iterations—as the independent experimental units.

## Residual uncertainties and blockers

1. A maintained GenSim replacement episode with an appropriate short-lived causal signal has not yet been selected; the native DDEval scenario is confirmed to be a shared archive of a real incident, not replayable GenSim lineage.
2. GenSim owners have not yet confirmed a supported per-execution Agent check-configuration mechanism.
3. The exact owner-approved production gs-episode-worker launch command/API and required service access have not been recorded.
4. Sims Archiver readiness remains unconfirmed until both workflow and activity poller identities are visible.
5. Eval Data Portal's GenSim projector creates archival-manager queries only for six EVP tracks; this is expected because Bits metrics use the normal backend. The remaining gate is proving the correct live historical org, tags, and window in a Bits trace.
6. GenSim metadata projection must preserve the expected RCA, real alert identity and timestamp, monitor definition, trigger, valid retention-bounded time range, and archive context through Scenario Store ingestion. A created row with zero archival queries or fallback-only replay is invalid.
7. The metrics backend retains source-resolution data, but Bits or the underlying query API may request an interval that is too coarse and return rolled-up points. Proving the requested interval and returned timestamp spacing in the Bits trace remains the most important measurement risk.
8. Every DDEval run must complete before the capture leaves the approximately 15-month metrics retention window; record the deadline per execution.
9. Revocation of the previously exposed Anthropic credential has not been confirmed; that credential is not required by this workflow.
