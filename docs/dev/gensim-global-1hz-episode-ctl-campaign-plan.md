# Durable remote `episode-ctl` campaign controller for the global 1 Hz experiment

> **Documentation role: controller design and qualification contract.** The controller is now implemented and the selected campaign has executed. Use `gensim-global-1hz-e2e-flow.md` for the authoritative as-operated path and current downstream status. Dated pre-execution qualification statements below are retained as planning provenance; this file is not a runnable campaign instruction by itself.

## Status and scope

This is the approved execution contract for scored global 1 Hz experiment captures. It supersedes the former requirement that scored captures must run through `gs-episode-worker → gs-flow/Terrapin`. The experiment uses an approved durable remote runner that invokes the existing `episode-ctl` phase commands against the approved GKE target:

```text
frozen source bundle
→ immutable per-arm working directories
→ episode-ctl deploy (first opaque arm)
→ deployment identity gate
→ episode-ctl deploy (second opaque arm)
→ deployment identity gate
→ concurrent episode-ctl run-episode phases
→ verify
→ immutable archive
→ teardown and leak reconciliation
```

This is not a laptop workflow. The remote runner, durable state store, campaign lease, artifact destination, identity, retention period, and launch owner must be approved and recorded before the first remote write.

The controller is deliberately a small pair controller, not a generic fleet system. `GenerateCausalgenEpisodes` is not a dependency: current `dd-source` `origin/main` implements that workflow as an empty stub.

## Authentication and onboarding contract

Authentication/onboarding is a release gate for the controller, not an implementation detail. Complete the following before any controller remote write, including a staging qualification deployment.

### 1. Approve the remote runner and non-human identity

Select the remote execution platform and name its operational owner. Provision a dedicated non-human workload identity for the controller; do not use a developer laptop token, personal account, or interactive `gcloud auth login` as the campaign credential source. Record the runner deployment identity, controller artifact digest, workload identity principal, owning team, and break-glass approver in the approved launch record.

### 2. Establish least-privilege GKE access

The workload identity must have only the approved GKE/Kubernetes permissions needed to create, inspect, and remove the controller-owned namespaces, Helm releases, workloads, pods, events, logs, monitors, and explicitly approved cluster-scoped RBAC objects. The GKE owner must approve the target cluster/context, namespace prefix, environment naming policy, and any ClusterRole/ClusterRoleBinding use. The qualification evidence must include a read-only access preflight and a post-teardown proof that no controller-owned namespaced or cluster-scoped object remains.

### 3. Establish build and image access

Provision the permissions needed to submit deployments, inspect Cloud Build status/logs, and push/pull workload images through the approved registry. Confirm that the runner can observe actual internal build fan-out and resolved runtime image IDs. A top-level deploy submission alone is not sufficient build evidence. Record the approved Cloud Build project, registry/repository scope, and build/log retention owner; do not add unrestricted project-wide build or registry access solely for this experiment.

### 4. Provision durable state and immutable evidence storage

Create separate experiment-owned remote prefixes for mutable-but-append-only campaign state and immutable retained evidence/archive objects. Grant the controller write access only to its campaign prefix and readback access needed to verify archived objects; grant blinded analysts only the redacted evidence path. Record storage identity, retention/deletion policy, encryption/access controls, recovery owner, and the private arm-map location/access owner. Credentials, arm mapping, and secret values must never be written to controller logs, manifests, ledgers, rendered bundles, or evidence artifacts.

### 5. Provision Datadog and evaluation-service access

Obtain secret-backed non-human authorization for the approved Datadog/GKE simulation operations, Scenario Store, DDEval, LLMObs export, crawler routing, and archive readback required by this experiment. Record service account/principal references and owners, not secret values. Confirm that the principal is limited to the approved experiment routing/configuration and cannot modify production defaults. Paid investigation authorization remains a separate recorded launch gate.

### 6. Define secret delivery, rotation, and incident handling

The runner must receive secrets through the approved secret-management mechanism at runtime. Name the secret owner, rotation/revocation process, audit location, and the operator authorized to pause or recover a campaign. Credential exposure, expired credentials, unexpected identity, or an authorization failure after a remote mutation is a stop-and-reconcile event; do not substitute a personal credential to continue.

### 7. Capture onboarding evidence and pass a no-write preflight

Before the first deploy intent, retain read-only evidence of the runner principal, effective access to the approved GKE target, build/log visibility, registry access, state-store read/write, archive readback, and evaluation-service authentication. Validate that a controller restart uses the same workload identity and durable state without a surviving local shell. Any missing authorization, unapproved permission escalation, interactive-only authentication dependency, or inability to redact arm/secret data blocks qualification.

The no-write preflight must also parse each exact execution derivative before extraction or launch. Its source-quality profile must require source-grounded root cause, symptom, causal path, and detailed RCA text. Its episode-execution profile must require positive baseline/disruption/cooldown durations, disruption and cooldown actions, and at least one complete symptom monitor scoped by the run environment placeholder. Derivatives must bind the source archive and canonical episode hashes, preserve `services/**`, `episodes/**`, and `scenario.yaml`, and attest that normalized opaque-arm trees differ only at the resolution toggle. Pre-rendered derivative mode must verify the exact public-ledger SHA-256, require both opaque arms, validate both boolean toggle values and the pinned Agent/NTP contract, and never reapply or reveal the arm transformation. The permanent preflight manifest remains no-write. Executable manifests are limited to either the exact-one-pair `logged_nonconfirmatory_pair` contract or the exact-two-pair `logged_nonconfirmatory_pilot` contract; both require serial execution, no automatic retry or replacement, fixed opaque order, and full plan-digest confirmation. Monitor and event observation must run after fixed phase windows or otherwise be proven timing-neutral: an observed `no_transition` is a valid semantic result, while API or malformed-evidence failures are recorded separately and may not silently censor the attempt. Numeric event identity is required only when a matching transition event is visible; missing event identity is not itself grounds to delete or replace an attempt. The direct `sims-controller /api/v1/archival` route preserves and validates `rca_causal_chain` when present but does not require it; only the managed GS Admin `gs-episode-worker` compatibility profile requires the canonical count invariant. Static compilation does not claim deployment, cleanup, archival, Scenario Store publication, or Bits success; those remain recorded runtime gates.

On 2026-08-26 the immutable episode-2483 opaque pair passed the real Workspace no-write preflight with 41 checks, zero attempts, zero results, and an empty preparation directory. The retained evidence is `/home/bits/episode-campaign-evidence/preflight-2483-20260826T181650Z` in the Workspace and `/Users/ella.taira/episode-campaign-evidence/preflight-2483-20260826T181650Z` locally; `preflight.json` has SHA-256 `60bf6bdfec0312a9695b112efa080d7bc388adcef35a8df7175bbea273d540bc`. The bounded observer implementation combines complete fixed-phase state polling with exact post-run trigger/recovery event lookup, records the four preregistered observation states, and writes a hash-bound per-attempt artifact plus an all-attempt ledger. Event lookup uses the same fixed twelve-request and eleven-wait schedule for every outcome, with no event-dependent early exit. The freeze point is the generated `plan.json`: it binds the runtime-manifest and fresh-preflight hashes, code and packet identities, serial/no-retry contract, opaque order, target, and evidence destination. Typing its full digest starts the logged run; a failed first arm cancels the queued second arm without retry, and Kubernetes scheduling failure is retained as an operational outcome rather than hidden behind a generalized capacity policy.

The executable runtime manifest then passed its own real Workspace no-write preflight at controller commit `ef6049d6cbe7ac96fa85eff7fd22cfec924037d9`: 41 checks, zero attempts, and an empty runtime bundle directory. Its retained Workspace/local evidence paths are `/home/bits/episode-campaign-evidence/runtime-preflight-2483-20260826T192942Z` and `/Users/ella.taira/episode-campaign-evidence/runtime-preflight-2483-20260826T192942Z`; the runtime manifest SHA-256 is `105edf12648d59bf6a2e8cf5553b341ea50b6a3484cdc123d77065ec69aed96e` and `preflight.json` SHA-256 is `01ee69a9a8c62e6750c7b537558baae9ce9293c3de742e617d4670310693ebd4`.

The selected pilot now preregisters fresh matched runs for episodes 2483 and 2563 in exact order `2483/arm-a`, `2483/arm-b`, `2563/arm-a`, `2563/arm-b`. Its bounded manifest is `src/controller/campaigns/global-1hz-two-scenario-runtime.yaml`, SHA-256 `d3e01e6658d260775cf35d9512315d9923335a208ce9a2f0263bea2cbc6fa2d2`. Any failed arm cancels every later arm; nothing is retried or replaced. The historical 2600 and 2563 runs remain retained but unarchived because they predate this paired-v2 observer/accounting contract.

At controller commit `909322e1168462f7d0293031d7bcd5df2d5240bb`, the selected four-arm manifest passed the real Workspace no-write preflight with 41 checks, zero attempts, and an empty runtime bundle directory. The retained Workspace/local evidence paths are `/home/bits/episode-campaign-evidence/runtime-preflight-2483-2563-20260826T194402Z` and `/Users/ella.taira/episode-campaign-evidence/runtime-preflight-2483-2563-20260826T194402Z`; `preflight.json` SHA-256 is `0d6d14d35c1f72b861f3fcf0b1ca852512043c50ce6b0b61b7965471432416a4`. This paragraph records the pre-execution snapshot; subsequent campaign execution and current status are summarized in `gensim-global-1hz-e2e-flow.md`.

## Existing contracts used

- Confluence page `7070975041`, **Running an Episode on GKE with episode-ctl**, requires same-scenario A/B runs to serialize build/deploy because service tags are shared and mutable, then permits their baseline/disruption/cooldown phases to overlap.
- `episode-ctl` exposes `deploy`, `run-episode`, `verify`, `archive`, and `teardown` in `gensim/src/controller/controller/cli.py`.
- `EpisodeController.deploy()` builds and pushes service images during deployment. One deploy can submit multiple Cloud Builds concurrently; the campaign can cap active deploy processes, but it must not claim to cap individual Cloud Build jobs unless `episode-ctl` gains that control.
- The scaled experiment's source freeze, blocked randomization, opaque identifiers, private arm mapping, frozen Bits configuration, archive/Scenario Store/DDEval contracts, and append-only attempted-capture rules remain binding.

## Preconditions

Before the controller may perform a remote write, the launch manifest must freeze and hash:

```text
campaign version and controller artifact digest
source bundle/archive hash and source revision per scenario
rendered control and treatment bundle hashes
normalized allowlisted A/B diff
Agent digest and expected toggle per arm
frozen blocked randomization schedule and its hash
opaque capture/pair/arm identifiers
private arm map location and access owner
namespace and matching DD_ENV per arm
approved GKE context/cluster identity
remote runner identity, state store prefix, artifact/archive prefix, and retention policy
DDEval/crawler/judge/model configuration and paid-run approval
retry, replacement, pause, budget, and cleanup policies
```

The launch manifest is append-only after approval. It must not contain arm-revealing terms such as `control`, `treatment`, `normal`, `1hz`, or toggle values in investigator- or judge-visible aliases, namespaces, environments, monitor names, event text, archive-derived context, or Scenario Store/DDEval metadata. The private arm map is separately stored and unavailable to blinded analysis.

## Immutable arm workspaces

The controller must never run two arms from one mutable bundle checkout. For each arm it creates a distinct immutable working directory from the same frozen source archive:

```text
<campaign-state>/arms/<capture-id>/bundle/
<campaign-state>/arms/<capture-id>/logs/
<campaign-state>/arms/<capture-id>/results/
<campaign-state>/arms/<capture-id>/metadata/
```

It records the source archive hash, rendered bundle hash, per-arm patch hash, and normalized A/B diff before deploying. The only permitted rendered configuration difference is the Agent experiment toggle. Any other difference fails the arm before deployment.

This prevents scenario-local result paths such as `run-result.json` and `results/<episode>.json` from being shared by concurrently running arms.

## Durable state machine and campaign lease

The controller uses write-ahead state: it writes intent before every remote mutation, then records observed external identities after the mutation. It uses a durable remote state store rather than an ephemeral CI workspace.

Per-arm lifecycle:

```text
planned
→ deploy_intent_recorded
→ deploy_submitted
→ deployment_identity_verified
→ episode_intent_recorded
→ episode_running
→ episode_finished
→ verify_finished
→ archive_finished
→ teardown_finished
```

Terminal states are explicit:

```text
infra_failed_pre_episode
integrity_failed_pre_episode
semantic_result_recorded
verification_failed
archive_failed
cleanup_failed
cancelled
```

Every intent includes a deterministic `action_id`, capture ID, expected namespace, `DD_ENV`, source/rendered hashes, and controller run ID. Before retrying after a crash or timeout, the controller reconciles the intended external state:

```text
namespace and Helm release
pod readiness, image IDs, and effective Agent environment/configuration
Cloud Build IDs/statuses known from logs or labels
monitor IDs and events
result artifacts
archive objects and checksums
cluster roles and bindings
```

It must not repeat an ambiguous deploy or episode action until reconciliation shows the action did not take effect. A completed semantic result is never rerun or overwritten.

A durable campaign lease prevents two controllers from operating the same manifest concurrently. The lease records owner run ID, heartbeat, and expiry. Lease stealing requires an expired lease plus recorded operator approval after reconciliation.

## Pair sequencing and resource controls

The first qualification configuration is intentionally bounded:

| Control | Initial limit | Meaning |
| --- | ---: | --- |
| Active `episode-ctl deploy` processes | 1 | Serializes same-scenario build/deploy and limits aggregate build pressure; does not promise one Cloud Build subjob. |
| Active matched pairs | 1 | One source scenario is in the campaign lifecycle at a time. |
| Active `run-episode` processes | 2 | The two arms may run concurrently only after both deployments pass identity gates. |
| Same-source build/deploy lock | 1 | Never permit shared scenario/service tags to overlap. |
| Verify/archive/teardown | 1 | Preserve deterministic evidence and simplify recovery during qualification. |

The controller must not use a fixed 15- or 30-minute delay as its build barrier. It launches the second deployment only after the first deployment reaches the required readiness and evidence state. It launches either episode phase only after **both** arms pass the deployment identity gate.

One active deploy can internally fan out to multiple Cloud Build jobs. `RESOURCE_EXHAUSTED`, temporary Cloud Build/GAR/GKE API failures, and documented transient Datadog API failures are retryable only before episode execution, with bounded exponential backoff and recorded attempts. They trigger a campaign pause when the preregistered failure budget is reached.

Do not increase limits after results are visible. A subsequent limit requires an explicit qualification decision based on captured Cloud Build, GAR, GKE, Datadog, archive, and cleanup headroom.

## Deployment identity gate

Before either `run-episode` command, write `deployment-identity.json` for each arm and require:

```text
expected unique namespace exists
expected matching DD_ENV is configured in Agent and workload telemetry configuration
all expected workload and Agent pods are Ready
Agent runtime imageID equals the frozen Agent digest in both arms
control toggle is false; treatment toggle is true
application workload imageID maps match exactly across arms
no namespace/env/monitor identity overlaps another active capture
```

The application image map is based on resolved runtime image IDs, not only source hashes or mutable image tags. A mismatch is `integrity_failed_pre_episode`; neither arm starts its episode phase. This qualification also establishes whether fresh sequential builds are stable enough for the selected scenarios. If they are not, the experiment pauses rather than silently accepting different application bytes.

## Retry, replacement, and stopping rules

The controller retries only pre-episode failures where reconciliation proves no episode action began. Examples include a rejected Cloud Build submission, failed deployment with no live namespace/workload, or a transient preflight API failure.

It must not automatically retry when any of the following exists:

```text
run-episode command was accepted or may have begun
baseline/disruption/cooldown may have occurred
a result artifact, monitor event, or scored investigation exists
an archive object exists
an identity, leakage, source, image, or toggle mismatch occurred
```

Every attempt, including aborted and failed attempts, remains in the append-only ledger. Replacement eligibility uses the frozen arm-independent classification above; CLI wording such as "infrastructure failure" is not sufficient.

Pause new admissions immediately on an identity mismatch, image mismatch, arm leakage, archive overwrite attempt, unresolved archive/result identity, duplicate active lease, cleanup leak, credential exposure, or the preregistered infrastructure/budget threshold. Reconcile active work and require named operational-owner approval before resuming.

## Per-arm evidence and terminal handling

Each arm retains machine-readable metadata and raw logs for:

```text
manifest, controller, and workspace hashes
intent/event ledger entries and campaign lease history
runner identity, GKE context/cluster identity, namespace, Helm release, DD_ENV
Cloud Build IDs/statuses and relevant logs
pod readiness snapshots, Kubernetes events, resource requests/limits, restart/throttle evidence
Agent imageID, effective toggle/configuration, and application workload imageID map
monitor/event IDs and timing
episode result JSON and command output
verification output
archive destination, object IDs, checksums, and readback evidence
teardown output and namespace/monitor/Helm/cluster-scoped-RBAC reconciliation
Scenario Store, DDEval, LLMObs, crawler, and judge identities after ingestion/evaluation
```

Archive uses an approved immutable per-arm destination and must complete before teardown. The controller verifies the archive object/readback before recording archive success. It then tears down the arm namespace and verifies no namespace, Helm release, monitor, ClusterRole, or ClusterRoleBinding leak remains.

## Qualification and scale gates

The controller must pass these gates in order:

1. **Dry run:** schema, hashes, opaque identities, runner access, state-store write/read, and no-write command rendering.
2. **Crash/resume test:** interrupt after each lifecycle boundary and prove reconciliation prevents a duplicate deploy or episode run.
3. **One-pair qualification:** prove serialized build/deploy, two-arm deployment identity parity, concurrent episode phases, verification, immutable archive, and cleanup.
4. **Capacity review:** record Cloud Build fan-out, GAR pulls, GKE pod/node resources, Datadog monitor/telemetry behavior, archive latency, and cleanup results; approve or reject any increase.
5. **Pilot only after all experiment protocol gates pass:** preserve the four-scenario pilot as design data and freeze final numeric thresholds before confirmatory scale.

No result from this controller is eligible for scoring until its metadata, archive, and cleanup evidence reconcile. The execution route, runner, state location, and evidence schema are part of the experiment version; changing them after confirmatory outcomes requires a new version.
