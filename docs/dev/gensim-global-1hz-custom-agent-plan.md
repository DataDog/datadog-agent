# Throwaway global 1 Hz Agent for GenSim

> **Documentation role: historical treatment provenance.** This records the treatment design that produced the frozen experimental Agent artifact. It is retained for review and reproducibility, but the current execution path is governed by the scaled design and durable remote controller plans.

## Purpose

Build one experimental Datadog Agent image for a bounded GenSim/Bits evaluation:

- **control:** normal Agent behavior;
- **treatment:** Agent-owned recurring checks, ordinary DogStatsD aggregation, and metric flushing use a hardcoded one-second cadence.

This code is test scaffolding and is not intended to merge. The same immutable image digest must run in both arms.

## Runtime switch

The image has one private environment toggle:

```text
DD_METRIC_RESOLUTION_EXPERIMENT_ENABLED=true
```

Unset, invalid, or false values retain normal behavior. There are no configurable treatment intervals, YAML settings, generated schema, or release notes. When enabled, every treatment interval is hardcoded to one second.

## Behavior

### Recurring checks

Treatment forces every positive normal check interval to one second at the scheduler boundary. It preserves:

- interval zero as one-shot;
- negative-interval rejection;
- shadow-check intervals;
- cancellation through the effective queue; and
- existing same-check overlap prevention.

A one-second schedule does not create source information that an integration, API, cache, or producer does not expose.

### Ordinary DogStatsD

Treatment constructs ordinary TimeSamplers with a one-second bucket and flushes the metric aggregator every second. Gauge, count/rate, histogram, distribution, and sketch behavior continues through the existing TimeSampler path.

Timestamped DogStatsD remains unchanged: producer timestamps survive and count/rate normalization plus `Serie.Interval` retain the existing fixed ten-second semantics.

An explicit zero flush interval used by one-shot commands remains zero even in treatment mode.

## GenSim deployment

`RunEpisodeJobRequest.image` selects sim-generator, not the Datadog Agent. The Agent is owned by the maintained app bundle.

For each selected app:

1. set `SHARED_DATADOG_MODE=false`;
2. deploy the app-local Datadog Helm dependency;
3. pin `datadog.agents.image.repository` and `datadog.agents.image.digest`; and
4. inject the toggle through `datadog.agents.containers.agent.envDict`.

Example app-local hook:

```bash
SHARED_DATADOG_MODE=false

RUNTIME_AGENT_IMAGE_REPOSITORY="REGISTRY/PROJECT/datadog-agent"
RUNTIME_AGENT_IMAGE_DIGEST="sha256:FULL_RESOLVED_DIGEST"
RUNTIME_METRIC_RESOLUTION_ENABLED="CONTROL_FALSE_OR_TREATMENT_TRUE"

runtime_append_helm_set_args() {
    HELM_SET_ARGS+=(
        --set-string "datadog.agents.image.repository=${RUNTIME_AGENT_IMAGE_REPOSITORY}"
        --set-string "datadog.agents.image.digest=${RUNTIME_AGENT_IMAGE_DIGEST}"
        --set-string "datadog.agents.containers.agent.envDict.DD_METRIC_RESOLUTION_EXPERIMENT_ENABLED=${RUNTIME_METRIC_RESOLUTION_ENABLED}"
    )
}
```

The current worker request has no per-execution Agent override. Unless GenSim owners add one, use normalized immutable control/treatment app-bundle variants. Both variants must pin the same Agent digest and differ only in the toggle value.

## Required runtime proof

Before using a capture, retain:

```text
DaemonSet PodSpec image == expected repository@digest
Pod imageID == expected resolved digest
filtered Agent environment/config == expected toggle
Agent status is healthy
```

Stop if shared-Agent mode remains enabled, the image cannot be pulled, the digest differs, or the effective toggle is wrong.

## Validation status

Completed locally:

- focused configuration-toggle, scheduler, collector, demultiplexer, aggregator, and ring-buffer tests;
- Go lint, Agent build, schema generation, and static diff checks;
- treatment runtime against a local decoding HTTP sink;
- consecutive one-second core-check points;
- one-second ordinary DogStatsD gauge, count, and timing buckets;
- exact timestamped DogStatsD timestamp/value preservation with interval 10;
- approximately one-second serializer request cadence;
- clean Agent shutdown;
- pre-merge immutable `datadog/agent-dev` image publication and local pull through Colima;
- pre-merge packaged image revision `48f294bd3a3` and installed `datadog_checks` verification;
- pre-merge native arm64 packaged smoke-only Python check loading and one-second treatment cadence;
- pre-merge matched native arm64 packaged-image control/treatment proof for ordinary and timestamped DogStatsD behavior;
- pre-merge `injector-dev` Kind-on-Colima deployment with the same immutable digest and verified PodSpec image, imageID, effective toggle, readiness, and zero restarts in both arms;
- pre-merge Kubernetes control proof retaining 15-second check cadence and ten-second ordinary DogStatsD buckets;
- pre-merge Kubernetes treatment proof producing 20 distinct one-second timestamps for custom, cpu, uptime, gauge, count, and timing-derived series while preserving timestamped DogStatsD interval 10;
- pre-merge amd64 image filesystem, embedded Python, and `datadog_checks` availability under emulation;
- merge-head multi-platform publication at `sha256:bcab32ebb2e768d89a0584b5a35c1043d8d8cb660df8861cd7dd3fccea392414`;
- merge-head native arm64 pull and identity verification proving revision `fbe7ac17d919fd501c4c3395e8965333adefdd21` and Agent version `7.84.0-devel+git.29.fbe7ac1.pipeline.129732203`;
- merge-head matched native arm64 packaged-container control/treatment requalification proving ten-second control buckets, one-second treatment check/DogStatsD/serializer cadence, exact timestamped DogStatsD preservation, and clean exits;
- matched 15-minute `injector-dev` Kind-on-Colima control/treatment soak with no selected-series gaps, restart, OOM, CPU throttling, forwarder drop, or serializer error;
- treatment resource measurement of 0.349 average CPU cores versus 0.039 control (8.86x), and 153 MiB p95 RSS versus 148 MiB control (1.04x); and
- treatment slow-check evidence: `kubernetes_apiserver` completed 328 of approximately 900 effective one-second intervals and `ntp` completed 764, with a separate 30-second DEBUG probe confirming 18 and 6 same-check-overlap skips respectively; and
- a deployment safety blocker: the NTP check's default 900-second interval explicitly follows pool.ntp.org query-rate guidance, so forcing it toward one second caused hundreds of runs during the soak and must not reach a managed environment unchanged; and
- maintained Pharmacy revision `01cf1863321359982ddeaa38e341024440526f1f` does not disable or redirect NTP, and Helm `ignoreAutoConfig` is insufficient because it only skips `auto_conf.yaml`, while NTP is loaded from `conf.yaml.default`;
- rejected empty regular `ntp.yaml` suppression because the Agent reported no valid instances and continued scheduling the packaged default;
- passed matched local NTP suppression with the supported `integration.excluded: [ntp]` setting: NTP was absent from control and treatment status, effective configuration was mounted, and no NTP skip or execution path remained;
- passed the focused control cadence run with no internal point gaps, restart, drop, or serializer error; and
- characterized best-effort treatment timestamp survival after successful NTP suppression: a clean 90-second capture emitted 96 points for each selected series, while integer-second serialization left 67 distinct timestamps for `metric_resolution.smoke.sequence` and 83 for `system.cpu.user`; ordinary DogStatsD and `system.uptime` retained 96 distinct one-second timestamps; and
- wired the maintained Pharmacy working copy from base revision `01cf1863321359982ddeaa38e341024440526f1f` and committed it locally as `fcee69958c43c0df90150b86de23a0354f92f28f` with the immutable treatment digest, private toggle, `integration.excluded: [ntp]`, and smoke-only custom check whose run ID derives from managed `DD_ENV`. Helm 3.82.0 rendering produced the intended app-local DaemonSet, image, environment, and ConfigMaps; Helm lint passed. The repository syntax gate passed every available check but could not execute ShellCheck because that executable is unavailable; and
- completed the canonical read-only gs-admin sync plan against `https://gs-admin.us1.staging.dog`, group `gensim`, for only `pharmacy-replicaepoch-store/prescription-read-serves-prior-epoch`. No existing Pharmacy identity was found; the plan has zero conflicts/failures and proposes creating one episode, one 30,720-byte blueprint (`a569b391...`), and one 168-member app bundle with 23,550,850 content bytes (`3082eb5a...`). No remote action was attempted.

The local decoding-sink and Kubernetes runs are author-local preflight evidence, not repository attestations or substitutes for native amd64, fakeintake, the normal metrics backend, Bits, or managed GenSim proof. CPU and RSS are retained as informational observations rather than pass gates. Immediate discarded ticks for a check that is already running are accepted because this is pre-existing overlap protection in `pkg/collector/worker/worker.go`; no change to that guard is required. NTP query-rate safety is locally resolved through `integration.excluded`. The experiment now treats one-second normal-check collection as best effort: known same-context integer-second collisions are measured per metric rather than treated as a deployment blocker. The Agent must not rewrite timestamps to manufacture uniqueness.

## Local GKE log-monitor blocker

The 2026-08-14 episode 2600 and 2563 qualification attempts exposed a common infrastructure failure. Both custom Agents reported `LogsProcessed: 0` and continuously failed kubelet initialization because they could not reach the kubelet with the deployed TLS configuration. Episode 2600's application emitted sustained `OverviewPageLoadError` records locally, while the normal Datadog logs backend returned zero matching records and its correctly scoped monitor remained `OK`. Episode 2563 had the same Agent failure and was stopped. This explains repeated failures of log-based candidate monitors; it does not invalidate metric-alert successes such as episode 2643 because DogStatsD submission does not require container-log discovery.

The app-local Agent templates also used a single Deployment replica while workload pods were scheduled across multiple nodes. Namespace isolation does not make host log mounts cross-node. An unconstrained DaemonSet is unsafe because it would schedule the custom experimental Agent on every node in the shared cluster. Host networking is also unsafe here: the shared cluster Agent already occupies ports 8125 and 8126 on every node. Static one- and two-replica remediations then proved scheduling-dependent: one replica missed a workload node, while two hard-anti-affined replicas could become unschedulable when all workloads landed on one node. Do not continue engineering adaptive node-local log collection for this metric-resolution experiment. Episodes 2600 and 2563 are abandoned as primary candidates because their log-alert dependency is unrelated to the 1 Hz metric hypothesis.

Before any future log-monitor episode enters its baseline phase, fail closed unless all of the following are proven:

1. every node hosting candidate workload pods has a ready Agent pod;
2. no Agent reports a kubelet connectivity failure during startup;
3. a deterministic preflight application log is retrievable from the normal Datadog logs backend with the exact environment and service tags; and
4. the eventual monitor query uses those same environment and service tags.

Runs `episode-2600-1hz-20260814-193056` and `episode-2563-1hz-20260814-194046` are failed infrastructure qualifications. Preserve them as negative evidence; do not archive, ingest, or evaluate them.

## Current local metric-alert candidate

Episode 2483 v3, `serialization-regression`, is the prepared replacement. Its unchanged runtime emits ordinary non-timestamped DogStatsD counts per event: `notification_router.job.enqueue.hits` falls during disruption while `event_transformer.payload.encode.exceptions` rises. The historical v3 run reached baseline `OK`, alerted after 363 seconds, recovered after 182 seconds, and passed verification. The pinned source archive SHA-256 is `43f6bd6df078626704d6e8922dbd11eba5c77163121f058d90bea999ae26481d`.

The derivative at `/Users/ella.taira/episode-bundles/episode-2483-v3-1hz` preserves every `services/**` file and `episodes/serialization-regression.yaml` byte-for-byte. Only Agent configuration, execution gates, feed-event identity capture, and scenario metadata aliases change. The custom Agent is one bounded Deployment behind the existing DogStatsD/APM ClusterIP Service; it has no container-log collection, runtime socket, host log mount, or node-placement dependency. `/Users/ella.taira/episode-bundles/global-1hz-selected-sources/validate_prepared_2483_1hz.py` enforces this boundary. Run `episode-2483-1hz-20260816-162640` failed before baseline because the inherited monitor payload set `new_group_delay` on simple-alert queries; Datadog accepts that option only for multi-alert monitors. The immutable failed run is preserved, the invalid option is removed from create and update payloads, and the validator rejects its reintroduction.

Run `episode-2483-1hz-20260816-164841` then passed the episode contract: baseline reached `OK`, disruption reached `Alert` after 181 seconds, cooldown returned to `OK` after 272 seconds, and numeric trigger/recovery events `8768398996205424422` / `8768411077708371021` were retained. Standard verification failed because the intentionally metric-only custom Agent collected no logs. Strict verification additionally exposed stale production aliases in false-positive metadata (`propjoe`, `sobotka`, and `kafka.producer.message_count`) even though all three synthetic true-positive metrics and all three causal-path traces were present. An explicit metric-only qualification accepted the run for a technical Bits path smoke, but material Agent CPU throttling (`93,808 / 98,032` cgroup periods) excludes it from clean experiment-quality evidence.

The immutable archive `s3://dd-applied-ai-research-us1-prod/gensim/episode-2483-1hz-20260816-164841/` preserves exactly one trigger and recovery feed row. Eval Data Portal ingestion with `force:false` created Scenario Store UUID `f0db7c0e-df73-4a95-9973-9a2b1778a8ee`; read-back and DDEval mapping preserve event `8768398996205424422`, monitor `313936953`, and archive context `episode-2483-1hz-20260816-164841`. DDEval run `episode-2483-1hz-technical-e2e-v1` completed one `gpt-4o` investigation and both pinned judges. Crawler logs prove archive mode and exact `@evt.id:8768398996205424422` lookup; the metric tool succeeded and was cited as best evidence. The model nevertheless blamed a generic `notification-router` logic issue rather than the datetime serialization regression in `event-transformer`, yielding match probability `0.401`, DeepJudge `0`, and `0/1` passed. This is a completed technical E2E with an incorrect RCA, not evidence that 1 Hz improves investigation quality. LLMObs export subsequently proved why this run cannot test model-visible 1 Hz: the metric request omitted `interval` and `raw_data`, so automatic downsampling returned 20 binned statistical summaries over 418 seconds rather than raw timestamp/value points. It queried only `notification_router.job.enqueue.hits`; it never queried the causal `event_transformer.payload.encode.exceptions` or downstream delivery metric. Backend one-second spacing remains proven separately, but it was not exposed to the model by this tool call.

## Remaining gates

Before collecting canonical pairs:

1. obtain explicit approval for the exact retained gs-admin plan at `/tmp/pharmacy-global-1hz-gs-admin-sync-plan.json`, then apply it with the same target, group, revision, exact episode selection, and parallelism; reserve `SHARED_DATADOG_MODE=false` for managed launch and do not use the rejected empty `ntp.yaml` workaround;
2. confirm deployment acceptance given successful gateway signature verification from the production signing environment but `Signing: false` for destination publication, then prove native amd64 deployment (the full amd64 Agent cannot be qualified under local QEMU because its Go runtime fails during initialization);
3. run the disposable managed Pharmacy smoke with the immutable treatment image and smoke-only custom check;
4. compare emitted sequence values with distinct normal-backend timestamps and quantify per-metric integer-second deduplication; require treatment resolution to remain materially finer than control and stop on loss not explained by same-context timestamp collisions;
5. prove Bits uses the correct org, tags, historical window, requested interval, returned resolution, and the surviving fine-resolution points; and
6. stop on coarse rollup, wrong scope, interpolation, Agent-side timestamp rewriting, tag loss, missing metrics, or unexplained point loss.

The disposable smoke and historical discovery records are not members of the canonical paired dataset.
