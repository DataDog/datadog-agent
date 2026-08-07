# Global 1 Hz metrics in GenSim with one custom Agent image

## Purpose

Build one experimental Datadog Agent image that can run both arms of a GenSim/Bits SRE evaluation:

- **control:** normal Agent metric cadences;
- **treatment:** every Agent-owned metric path that can meaningfully produce one-second points uses a one-second cadence.

The same immutable image digest must run in both arms. A bounded runtime configuration switch, not a different binary, selects the treatment. The managed episode-worker request has no Agent image or Agent-config override: its `image` field selects sim-generator. Therefore, unless GenSim owners add an approved per-execution override, control and treatment use normalized immutable app-bundle variants whose only content difference is the recorded experiment-enabled value. Both arms otherwise reuse the same episode, workload, fault sequence, simulator, backend, and evaluation settings.

This plan complements `docs/dev/metric-resolution-bits-eval-plan.md`. That document owns the experiment, archival, Scenario Store, and DDEval workflow. This document owns the custom Agent behavior and its GenSim deployment contract. Keep both files standalone; do not add them to `docs/dev/README.md`.

## Decision summary

1. Do not use metric-lookback retention, trigger monitoring, or lookback egress for this experiment.
2. Do not use a metric allowlist. Treatment mode changes every normal recurring check, including scheduled checks that primarily emit non-metric payloads, plus the global ordinary DogStatsD aggregation width and Agent serializer flush cadence. Those side effects are measured but are not relabeled as metric evidence.
3. Preserve timestamped DogStatsD's no-aggregation path, producer timestamps, and existing ten-second count/rate normalization semantics.
4. Use the ordinary Agent serializers and metrics intake so Bits queries the same live backend path used by normal deployments.
5. Do not claim that Agent configuration can manufacture one-second information from a slower producer, cache, SDK, cloud API, or dedicated product payload.
6. Do not call process, trace, orchestrator, network, security, or inventory payloads “metrics” unless Bits can query their native schema explicitly.
7. Build once and reuse the same image digest. Control and treatment differ only by a recorded runtime setting carried by normalized app-bundle variants; `RunEpisodeJobRequest.image` is not an Agent selector.
8. Do not require Sims Archiver metric Parquet for this evaluation. Bits queries metrics from the normal historical metrics backend within its retention window; managed archival remains required for each execution's logs, traces, events, metadata, and Scenario Store lineage.

## Definitions and claim boundary

### “All metrics at 1 Hz”

For this experiment, the phrase means:

> Every metric-series path owned by the core Agent is configured to observe or aggregate at one-second cadence when its upstream source can provide new information that often.

It does **not** mean interpolation, repetition presented as new information, or converting every Datadog product payload into a metric series.

### Three cadence owners

A point's achievable resolution is bounded by three independent clocks:

1. **Producer cadence:** how often the application, SDK, kernel source, runtime API, or upstream service creates new information.
2. **Agent observation/aggregation cadence:** how often a check reads the source or DogStatsD seals a bucket.
3. **Delivery/query cadence:** how often the Agent serializes payloads and whether the live metrics backend and Bits metric query retain original timestamps.

Treatment mode changes the second clock and the Agent-controlled serializer-flush portion of the third clock. Delivery may be batched as long as the live backend retains distinct one-second point timestamps. The remaining evaluation risk is query-time rollup in the Bits metric tool, not Sims Archiver metric fidelity.

## Evidence from the current codebase

- Recurring checks are scheduled according to `Check.Interval()` and the scheduler already permits one second; interval zero is a distinct one-shot contract: `pkg/collector/scheduler/scheduler.go` (`minAllowedInterval`, `Scheduler.Enter`).
- The ordinary default check interval is 15 seconds: `pkg/collector/check/defaults/defaults.go` (`DefaultCheckInterval`).
- Core `CheckBase`, Python checks, and shared-library checks parse or inherit per-instance cadence, including explicit `min_collection_interval`: `pkg/collector/corechecks/checkbase.go`, `pkg/collector/python/check.go`, and `pkg/collector/sharedlibrary/sharedlibraryimpl/check.go`.
- Several checks override `Interval()` or have special lifecycle behavior, including orchestrator, discovery, embedded APM/process, GPU, network-device, one-shot, and long-running checks. Search `func (...) Interval()` under `pkg/collector/corechecks/` before changing scheduling semantics.
- Workers prevent concurrent runs of the same check by tracking running IDs and skipping an already-running check: `pkg/collector/worker/worker.go` and `pkg/collector/runner/tracker/tracker.go`. At one second, a slow check therefore misses observations rather than becoming truly one-second.
- Normal DogStatsD uses a fixed ten-second bucket passed to `NewTimeSampler`: `pkg/aggregator/aggregator.go` (`bucketSize`) and `pkg/aggregator/demultiplexer_agent.go`.
- DogStatsD bucket width and serializer flush interval are separate. `pkg/aggregator/time_sampler.go` selects buckets by sample timestamp and flushes completed buckets; `pkg/aggregator/demultiplexer_agent.go` separately owns automatic flushing.
- Timestamped DogStatsD gauges and counts are routed through the no-aggregation stream when enabled and retain producer timestamps: `comp/dogstatsd/server/impl/parse.go`, `comp/dogstatsd/server/impl/server.go`, and `pkg/aggregator/no_aggregation_stream_worker.go`. `dogstatsd_no_aggregation_pipeline` defaults to true in `pkg/config/setup/common_settings.go`.
- DogStatsD bucket width participates in rate/counter normalization and series interval metadata: `pkg/aggregator/time_sampler.go`, `pkg/aggregator/no_aggregation_stream_worker.go`, and the rate tests in `pkg/aggregator/time_sampler_test.go`.
- The buffered aggregator also creates recurrent internal series, including `datadog.<agent>.running`, at serializer flush time: `pkg/aggregator/aggregator.go` (`BufferedAggregator.appendDefaultSeries`, `AddRecurrentSeries`). These require a one-second treatment flush if the global claim includes them.
- The local GenSim V2 specification describes real integrations through pod Autodiscovery and controller-managed Agent resources: `gensim/specs/episode-spec-v2.md` and `gensim/specs/episode-v1-to-v2-migration.md`. It is evidence for the local contract only; production gs-episode-worker/Terrapin ownership remains unresolved until GenSim owners confirm it.
- Historical GenSim provenance code distinguishes integration metrics, trace-derived metrics, and custom DogStatsD metrics; use those classes for the source inventory, not as evidence that Bits reads archived metrics: `gensim/src/controller/controller/archiver.py` (`compute_dmetrics` documentation).
- Current gs-episode-worker requests expose an episode and generator image but no Datadog Agent image or arbitrary Agent treatment config: `DataDog/dd-source@a4285ceeaf83e317b507d5f1abcf95dcb8768107:domains/sims/shared/libs/proto/gsepisodeworker/gsepisodeworker.proto` and `DataDog/dd-source@a4285ceeaf83e317b507d5f1abcf95dcb8768107:domains/sims/apps/apis/gs-episode-worker/worker/job_submission.go`.
- `generator_image` is a generator container, not the Datadog Agent image: `DataDog/dd-source@a4285ceeaf83e317b507d5f1abcf95dcb8768107:domains/sims/apps/gs-flow/internal/api/api.go` and `DataDog/dd-source@a4285ceeaf83e317b507d5f1abcf95dcb8768107:domains/sims/apps/gs-flow/docs/API.md`.
- The maintained `ddoghq-sandbox/gensim-apps` runtime sources app-local `runtime-config.sh`, supports `runtime_append_helm_set_args`, and invokes that hook immediately before the app Helm install: `scripts/app-runtime/start.sh` and the existing hook in `account-and-ledger-cdc-boundary-rook-ceph/runtime-config.sh`, inspected at revision `01cf1863321359982ddeaa38e341024440526f1f`.
- The Datadog Helm chart renders `agents.image.repository` plus `agents.image.digest` as an immutable `repository@sha256:...` reference and uses `image.tag` only when no digest is supplied: `insightgrid-analytics/chart/charts/datadog/templates/_helpers.tpl` (`image-path`) at the same `gensim-apps` revision.
- `dda inv agent.image-build` consumes an Agent Debian package from the Omnibus output, builds `Dockerfiles/agent`, accepts a full `--tag`, and pushes that tag when `--push` is set: `tasks/agent.py` (`image_build`).
- Metric Lookback already sends timestamped one-second series through the ordinary serializer/backend path, so backend support for distinct one-second points is established prior art: `pkg/collector/metriclookback/lookbacksender/sender.go` and its tests. This experiment still verifies that the new global Agent path emits the intended points.
- The Bits team confirmed that the metrics tools do not query a metrics archive; they query the normal metrics backend and are limited by its approximately 15-month retention: [Bits Eval Slack clarification](https://dd.slack.com/archives/C07TUJB9FQA/p1785957892930239?thread_ts=1785944181.807409&cid=C07TUJB9FQA).
- Eval Data Portal's GenSim projector creates archival queries only for `trace`, `logs`, `errors`, `netflow`, `netpath`, and `feed`; it creates no metrics query: `DataDog/dd-source@a4285ceeaf83e317b507d5f1abcf95dcb8768107:domains/ai_platform/apps/eval-data-portal/internal/ingestion/parser.go` (`gensimEVPTracks`, `buildTelemetryQueries`).
- Bits archive configuration carries logs/events and trace archive contexts but no metrics archive context: `DataDog/dd-source@a4285ceeaf83e317b507d5f1abcf95dcb8768107:domains/chatbot/proto/chatbotpb/investigation_trigger.proto` (`InvestigationEvalConfig`).
- Bits DDEval routes completed GenSim Scenario Store records to the `gensim` Husky snapshot namespace for archived non-metric context: `DataDog/dd-source@a4285ceeaf83e317b507d5f1abcf95dcb8768107:domains/ai_platform/apps/apis/bits_ai_eval_runner/service/impl/bits_rpc_client.py` and its `service/service_test.py` coverage.

## Metric-source coverage audit

| Source visible around GenSim | Cadence owner | Treatment behavior | Claim status |
| --- | --- | --- | --- |
| Go/Python/shared-library integration checks | Core check scheduler | Force every normal recurring check to one second; classify non-metric scheduled side effects separately | In scope |
| OpenMetrics | OpenMetrics check plus application endpoint | Force check to one second; endpoint must update at least that often | In scope, source-limited |
| Container/system metrics emitted by ordinary checks | Core check scheduler plus runtime/kubelet caches | Force check to one second; report stale/repeated reads | In scope, cache-limited |
| JMX integration metrics | JMX check and jmxfetch/StatsD bridge | Force scheduling to one second; require completion and load validation | In scope when deployed, potentially infeasible |
| eBPF metrics consumed by ordinary Agent checks | Core check plus system-probe producer/cache | Force consuming check to one second; producer may remain slower or event-driven | In scope, producer-limited |
| GPU check metrics | GPU check plus system-probe GPU producer | Global check override supersedes normal cadence; verify probe scan/cache clocks separately | In scope when environment supports GPU |
| Ordinary untimestamped DogStatsD | Producer packet cadence plus Agent `TimeSampler` bucket width | Seal one-second global buckets | In scope, producer-limited |
| Timestamped DogStatsD gauge/count | Producer cadence and timestamps | Preserve existing no-aggregation path; do not rebucket or change its ten-second count/rate normalization contract | In scope only when producer emits one-second timestamps |
| Aggregator recurrent/internal metric series | Agent serializer flush | Flush at one second in treatment so recurrent points are created at one-second cadence | In scope; Agent-overhead signal, not incident evidence |
| Runtime metrics over DogStatsD | Language SDK | Preserve one-second input if the SDK emits it; Agent switch cannot increase SDK cadence | Source-controlled; report separately |
| OTLP metrics | SDK/collector observation and export | Preserve received timestamps through serializer; no universal Agent cadence override | Source-controlled; report separately |
| Process/container/connections payloads from process-agent | Process-agent loops and dedicated intake | Any embedded normal recurring check is scheduled at one second, but independent process-agent loops are unchanged; do not relabel payloads as metric series | Out of metric claim unless Bits native query support is proven |
| APM traces and trace stats | SDK, trace-agent concentrator, stats intake | Any embedded normal recurring check is scheduled at one second, but SDK/concentrator buckets are unchanged; do not relabel as metric series | Out of metric claim unless separately designed |
| Orchestrator resources/manifests | Orchestrator scheduled checks and dedicated `/api/v2/orch` endpoints | Scheduled invocation is accelerated to one second; resulting dedicated payloads are excluded from metric evidence | Out of metric claim |
| Network/USM/security/discovery payloads | normal discovery checks, system-probe loops, and dedicated product intakes | Normal recurring checks are accelerated; independent producer loops are unchanged and dedicated payloads are excluded from metric evidence | Out of metric claim |
| Cloud APIs or integrations with slower upstream refresh | External service | Poll at one second only if safe, but label repeated/stale values | Source-limited; never claim new one-second information |

This table is the initial audit, not permission to force every code path blindly. The implementation inventory in work item 1 must identify the exact checks and inputs present in the selected GenSim app bundle and archive.

## Proposed runtime contract

Use one dedicated internal experimental configuration group with safe normal defaults. Bind it through the core config system so the Agent can consume environment variables, but do not publish it in `config_template.yaml`, the public YAML schema, or release notes. There is intentionally no metric or check allowlist.

Treatment enables the group. Control leaves it disabled. The maintained GenSim worker cannot forward arbitrary Agent settings; the app-local Datadog Helm dependency injects these internal environment variables:

```text
DD_METRIC_RESOLUTION_EXPERIMENT_ENABLED=true
DD_METRIC_RESOLUTION_EXPERIMENT_CHECK_INTERVAL=1s
DD_METRIC_RESOLUTION_EXPERIMENT_DOGSTATSD_AGGREGATION_INTERVAL=1s
DD_METRIC_RESOLUTION_EXPERIMENT_SERIALIZER_FLUSH_INTERVAL=1s
```

Do not encode the arm label in metric tags or agent-visible scenario text. Record the private arm mapping outside the telemetry seen by Bits.

## Required semantics

### Scheduled checks

Treatment mode must:

- preserve interval zero as one-shot;
- force every other normal scheduled check to a one-second scheduler queue, including checks with an explicit positive `min_collection_interval` and scheduled checks whose primary output is a dedicated non-metric payload;
- avoid changing shadow checks because this experiment does not use metric lookback;
- retain the runner's same-check no-overlap protection;
- count and expose skipped one-second executions when a prior run is still active;
- identify non-metric checks in reporting rather than attempting an unavailable scheduler-level metric classifier;
- identify long-running wrappers separately: one-second wrapper commits do not imply that their underlying producer updates every second;
- preserve cancellation, rescheduling, and configuration-reload behavior;
- emit no treatment-identifying metric tag.

Changing only `DefaultCheckInterval` is insufficient because explicit positive intervals and custom `Interval()` implementations remain authoritative. The implementation should introduce an effective scheduling interval at the scheduler/collector boundary rather than rewriting every integration configuration.

### DogStatsD

Treatment mode must:

- replace the ordinary TimeSampler's fixed ten-second aggregation width with a runtime-provided one-second width;
- preserve control behavior at ten seconds;
- keep bucket width distinct from packet-buffer timeout and serializer flush interval;
- propagate the selected width consistently through the ordinary TimeSampler gauge, counter, rate, histogram, distribution/sketch, series interval, and normalization code;
- introduce a separately named fixed ten-second normalization constant for timestamped no-aggregation count/rate samples, and leave their values, interval metadata, and producer timestamps unchanged between arms;
- add explicit tests for timestamped no-aggregation gauge and count values, timestamps, types, and `Serie.Interval` in both arms;
- keep late-sample, context, tag enrichment, origin, and no-index semantics unchanged;
- define shutdown handling for the final incomplete one-second bucket without changing normal control semantics unexpectedly.

Treatment also sets the buffered aggregator/serializer flush interval to one second. This is required for recurrent internal series created at flush time and gives completed one-second DogStatsD buckets prompt egress. Keep packet-buffer timeout independent. Measure the resulting request/serialization amplification explicitly.

### Producer-controlled paths

Treatment mode must not interpolate or fabricate points. For timestamped DogStatsD, retain the producer point timestamp. For scheduled checks, ordinary DogStatsD, runtime metrics, OTLP, trace-derived metrics, system-probe caches, and cloud APIs, retain only timestamps actually carried by that path plus external test-fixture timing. The ordinary metric schema does not generically retain source-observation, Agent-receipt, and serialized clocks simultaneously; do not claim universal source-to-serialized lag without adding a separate diagnostic mechanism.

Classify each series as:

- genuinely one-second source variation;
- one-second Agent observations of a slower-changing source;
- producer-controlled cadence preserved by the Agent;
- unsupported/dedicated payload not represented as a metric series.

## Mandatory execution sequence

Execute these five stages in order. A stage is complete only when its listed evidence is retained; do not skip ahead because a build, Helm render, backend request, archive workflow, Scenario Store ingestion, or DDEval run merely returned success.

### 1. Write the custom Agent behavior

Implement the default-off `metric_resolution_experiment` contract and its tests before producing an image. Complete detailed work items 1–5 below: config binding, the effective one-second recurring-check interval, ordinary DogStatsD's one-second bucket, one-second serializer flushing, disabled-mode parity, timestamped DogStatsD parity, and fakeintake coverage.

Use `dda inv`, never raw Go commands. At minimum, run the focused tests listed in work item 6. Record the Agent source commit and a patch digest; fail this stage if disabled mode changes existing scheduling, ordinary ten-second DogStatsD behavior, or timestamped DogStatsD semantics.

Implementation status on 2026-08-06:

- Implemented the default-off config group in `pkg/config/metricresolution` and core config setup. The experiment is intentionally internal and environment-injected, so it is absent from `config_template.yaml` and has no public schema documentation or release note.
- Injected the validated normal-check override at `Scheduler.Enter`; interval zero and shadow intervals remain unchanged, and cancellation uses the effective queue.
- Added an independent `DogStatsDAggregationInterval` demultiplexer option and kept serializer `FlushInterval` separate.
- Split timestamped no-aggregation normalization into a fixed ten-second constant and retained timestamp, value, type, and interval behavior.
- Added config default/environment/YAML/validation tests, scheduler override tests, one-second ordinary DogStatsD gauge/count/distribution tests, demultiplexer option tests, and timestamped no-aggregation parity assertions.
- Passed focused `dda inv test` suites for config setup/schema/metricresolution, scheduler, collector, worker, aggregator, demultiplexer, and metric-lookback ringbuffer; passed `dda inv linter.go`, `dda inv agent.build --build-exclude=systemd`, schema regeneration, and `git diff --check`.
- Ran the built Agent locally with treatment enabled against an author-local HTTP intake sink. Core `cpu` and `uptime` checks were scheduled at one second and produced 11–12 consecutive one-second points. Ordinary DogStatsD gauge, count, and timing series produced eight or more distinct one-second buckets with `interval: 1`; timestamped DogStatsD preserved the exact three producer timestamps and values with `interval: 10`; series POST cadence had a one-second median; and the Agent exited cleanly. The local files are not committed and are not reviewable repository attestation; the smoke manifest retains only the non-secret summary.
- Source commit: `f2da1471bb748fb5108f89f36f7b83cab305ca79`. The author-local build patch is not committed or reviewable repository attestation; its SHA-256 is `c5d25981ea72ac1340bf2d0bf241d10935da44ea7b6821ba56a07a73d4e29e09`.
- Remaining qualification blockers: no dedicated skipped-because-running or scheduler queue-lag metric was added; those signals must be supplied before the soak gate or treated as explicit qualification blockers. Fakeintake, app-bundle smoke wiring, runtime soak, and backend/Bits proof remain unexecuted.

### 2. Build and publish the custom image

Build the Linux architecture required by the selected Terrapin tier in an approved Agent package-build environment. The repository's supported prior art is `tasks/omnibus.py` (`omnibus.build`) followed by `tasks/agent.py` (`agent.image-build`); the latter requires the Debian package produced under `<base-dir>/pkg`.

For an `amd64` tier, use a commit-derived, non-reused publication tag:

```bash
set -euo pipefail

SOURCE_SHA="$(git rev-parse HEAD)"
IMAGE_REPOSITORY="REGISTRY/PROJECT/datadog-agent"
IMAGE_TAG="${IMAGE_REPOSITORY}:metric-resolution-${SOURCE_SHA}"

# Produces omnibus/pkg/datadog-agent*_<arch>.deb.
dda inv omnibus.build --flavor=base --base-dir=omnibus

# Builds Dockerfiles/agent, runs its testing target, and pushes IMAGE_TAG.
dda inv agent.image-build \
  --arch=amd64 \
  --base-dir=omnibus \
  --tag="${IMAGE_TAG}" \
  --push
```

Do not use `--skip-tests`. Record the complete build log, source SHA, target architecture, flavor, base image, SBOM/signature result, registry, and publication tag. A successful local `agent.build` is not an OCI image and does not complete this stage.

### 3. Set and prove the custom image selection

The publication tag is a discovery handle, not the deployment identity. Resolve it once after push and deploy only the immutable digest:

```bash
set -euo pipefail

IMAGE_DIGEST="$(
  docker buildx imagetools inspect "${IMAGE_TAG}" |
    awk '$1 == "Digest:" { print $2; exit }'
)"
[[ "${IMAGE_DIGEST}" =~ ^sha256:[0-9a-f]{64}$ ]]
printf 'Agent image: %s@%s\n' "${IMAGE_REPOSITORY}" "${IMAGE_DIGEST}"
```

`RunEpisodeJobRequest` has no Agent image or arbitrary Agent-config fields; its `image` field becomes the gs-flow sim-generator image. In the selected app's `runtime-config.sh`, disable shared-Agent reuse and append the chart-local Agent image and environment overrides. If the app already defines `runtime_append_helm_set_args`, append to that function rather than replacing its existing arguments. Freeze one control and one treatment app-bundle variant with the same repository/digest and interval values; only `RUNTIME_METRIC_RESOLUTION_EXPERIMENT_ENABLED` differs:

```bash
SHARED_DATADOG_MODE=false

RUNTIME_AGENT_IMAGE_REPOSITORY="REGISTRY/PROJECT/datadog-agent"
RUNTIME_AGENT_IMAGE_DIGEST="sha256:FULL_RESOLVED_DIGEST"
RUNTIME_METRIC_RESOLUTION_EXPERIMENT_ENABLED="CONTROL_FALSE_OR_TREATMENT_TRUE"

runtime_append_helm_set_args() {
    HELM_SET_ARGS+=(
        --set-string "datadog.agents.image.repository=${RUNTIME_AGENT_IMAGE_REPOSITORY}"
        --set-string "datadog.agents.image.digest=${RUNTIME_AGENT_IMAGE_DIGEST}"
        --set-string "datadog.agents.containers.agent.envDict.DD_METRIC_RESOLUTION_EXPERIMENT_ENABLED=${RUNTIME_METRIC_RESOLUTION_EXPERIMENT_ENABLED}"
        --set-string "datadog.agents.containers.agent.envDict.DD_METRIC_RESOLUTION_EXPERIMENT_CHECK_INTERVAL=1s"
        --set-string "datadog.agents.containers.agent.envDict.DD_METRIC_RESOLUTION_EXPERIMENT_DOGSTATSD_AGGREGATION_INTERVAL=1s"
        --set-string "datadog.agents.containers.agent.envDict.DD_METRIC_RESOLUTION_EXPERIMENT_SERIALIZER_FLUSH_INTERVAL=1s"
    )
}
```

This is supported by two maintained precedents: app-local Helm arguments are already added through `runtime_append_helm_set_args`, and the Datadog chart's `image-path` helper renders `repository@digest` whenever a digest is set. Do not configure only `datadog.agents.image.tag`; a mutable tag does not satisfy the experiment's same-image invariant.

Before publication, render the selected chart and require the Agent container image to equal the expected `repository@digest`. During the managed run, retain all three runtime proofs:

```text
DaemonSet PodSpec container image == expected repository@digest
Pod status imageID == expected resolved digest
filtered `agent config --all` == expected experiment settings
```

A Helm value or rendered manifest alone is not sufficient. Stop if shared mode remains enabled, the registry cannot be pulled, the image ID differs, or either arm uses a different digest. Control and treatment must use this same digest; only the recorded `enabled` value changes.

### 4. Set and prove the live historical metric query path

Do not modify Sims Archiver to set a metric interval for this experiment. Bits does not read Sims metric Parquet; it queries the normal metrics backend. Metric Lookback is prior evidence that the backend supports one-second points, so this stage verifies only the custom Agent output and the exact query coordinates the evaluation will use.

For every throwaway, control, and treatment capture, retain:

```text
telemetry org and datacenter
environment ID and exact metric tags
metric name and type
capture start and end timestamps
expected source cadence and values
Agent image and config digests
backend query and returned timestamp/value/tag rows
last permissible evaluation date under the approximately 15-month retention window
```

Query the normal metrics backend immediately after capture and require distinct one-second timestamps for the treatment fixture. Delivery batching is acceptable; event-time collapse, interpolation, tag loss, or unexplained point loss is not. Preserve the full query and response as evidence.

This direct check does not prove what Bits receives. During stage 5, inspect the DDEval trace and require the Bits metric tool to use the same telemetry org, environment tags, and historical time window and to return the expected point spacing without a coarser query-time rollup.

Sims Archiver may continue to write metric Parquet as part of its normal workflow, but its metric interval, Parquet rows, and metric hydration are optional preservation evidence. They do not gate image qualification, matched captures, Scenario Store publication, or DDEval.

### 5. Run the managed smoke and live-metric fidelity gate

Publish the exact app bundle containing the digest-pinned Agent and launch it through gs-episode-worker and gs-flow/Terrapin. Let gs-episode-worker complete its normal managed archive handoff for logs, traces, events, metadata, and Scenario Store lineage; do not use direct gs-flow or manual archive writes.

Use the frozen smoke selection in `docs/dev/gensim-global-1hz-smoke-manifest.json`: `pharmacy-replicaepoch-store/episodes/prescription-read-serves-prior-epoch.episode.yml` from `gensim-apps` revision `01cf1863321359982ddeaa38e341024440526f1f`. Inject a smoke-only custom check through app-local Helm values and emit the uniquely tagged monotonic `metric_resolution.smoke.sequence` gauge once per second for 60–120 seconds. In the same execution, use the existing high-rate `pharmacy.replica_epoch.apply_ms` DogStatsD timing stream to prove ordinary DogStatsD aggregation. The app calls datadog-go v5.6.0 without extended client aggregation, so timing samples are not aggregated by the client before reaching the Agent. Retain the expected sequence epoch/value/tag table and compare, in order:

```text
fixture output
running Agent image/config proof
normal metrics backend rows
Bits metric-tool request and response from the DDEval trace
```

Register the execution through the normal archive and Scenario Store path so Bits receives the same capture's historical non-metric context. The smoke scenario may use a disposable dataset, but it must remain outside the canonical paired dataset and final statistics.

Require exact tags and values plus distinct approximately one-second epochs from the backend. In the Bits trace, require the intended historical window, telemetry scope, selected metric, and no unexpected coarse rollup. Stop on timestamp rewriting, interpolation, tag loss, wrong-org or wrong-window routing, or unexplained point loss. Detailed work item 8 below remains the authoritative fidelity acceptance contract.

## Detailed implementation plan

### 1. Freeze the coverage inventory and behavioral contract

Enumerate every metric-producing component enabled in the selected GenSim Agent flavor and app bundle. Map each to scheduled check, ordinary DogStatsD, timestamped DogStatsD, producer-controlled serializer input, or dedicated non-metric intake. Resolve custom `Interval()` implementations, long-running wrappers, and cache/producer clocks before changing code.

- Areas: `pkg/collector/corechecks/`, `pkg/collector/python/`, `pkg/collector/sharedlibrary/`, `pkg/aggregator/`, `comp/dogstatsd/`, `comp/otelcol/`, `pkg/process/`, `pkg/trace/`, `pkg/config/setup/`, and the selected GenSim app bundle.
- Documentation: update this matrix and the affected Allium/spec files before implementation is reported complete.
- Done when: every metric name family expected in the baseline live-backend capture has a named cadence owner and one of the four classifications above; no “unknown” source remains.
- Stop condition: do not claim universal coverage if the backend capture contains a metric provenance class whose producer or intake cannot be identified.

### 2. Add the default-off experiment configuration with tests first

Introduce the smallest runtime configuration surface that lets one image run normal and treatment modes. Bind defaults and environment variables through the established core-Agent config setup pattern. Read the group through one shared validated config helper so the collector and demultiplexer cannot interpret it differently. When enabled, require every interval to be a whole-second duration of at least one second; reject sub-second, fractional-second, zero, or negative treatment intervals. Ensure disabled mode is byte-for-byte behaviorally equivalent at scheduler and DogStatsD construction boundaries even if dormant interval values are present.

- Areas: a dedicated setup file under `pkg/config/setup/`, config model/schema tests, `pkg/config/schema/yaml/`, and any affected enriched schema or example template.
- Tests first: defaults disabled; environment/YAML parsing; one-second acceptance; invalid-value rejection; disabled-mode construction parity.
- Docs/specs: update config documentation and any nearby Allium contract. Keep the setting clearly experimental and default off.
- Done when: one built image accepts control and treatment settings, schema generation is clean, and disabled mode selects the existing 15-second/default-check, ten-second DogStatsD, and normal serializer-flush behavior.

### 3. Apply a global effective one-second interval to every normal recurring check

Add a treatment-aware effective interval at `pkg/collector/scheduler/scheduler.go::Enter`, supplied through an explicit scheduler option from `comp/collector/collector/impl`. Do not change `DefaultCheckInterval`, mutate source Autodiscovery bytes, or require every app annotation to change: those approaches would miss explicit positive integration intervals. Compute the effective interval once per `Enter` call, preserve zero as one-shot, and leave every shadow check on its original interval. Preserve normal cancellation, reload, queue lookup, and same-ID overlap prevention. There is no existing scheduler-level metric-output classifier, so treatment intentionally accelerates every normal recurring check, including discovery/orchestrator/product checks; reporting separates their dedicated payloads from metric evidence. Add or reuse non-arm-tagged observability for requested runs, completed runs, skipped-because-running runs, duration above one second, and queue lag; if any signal is unavailable in the minimal code change, record it as a smoke/soak qualification blocker rather than claiming it exists.

- Areas: `pkg/collector/scheduler/`, `comp/collector/collector/impl/`, `pkg/collector/worker/`, and focused check lifecycle tests.
- Tests first: disabled parity; positive configured intervals forced to one second; interval zero remains one-shot; shadow interval unaffected; slow check is skipped rather than overlapped; long-running wrapper behavior characterized; representative discovery/orchestrator checks are shown to be intentionally accelerated; cancellation and reload clean up the effective queue.
- Performance gate: measure 1 Hz thundering-herd behavior because a one-second queue has one dispatch bucket. Do not add unbounded catch-up or duplicate queues. Abort if scheduler queue-lag p95 exceeds 500 ms for three consecutive minutes, any check overlaps itself, or the Agent skips more than 5% of expected runs for a metric source used in the experiment.
- Done when: every normal recurring check observed in the GenSim inventory attempts one-second execution, no check ID overlaps itself, missed executions are measurable, and non-metric scheduled work is classified rather than silently omitted.

### 4. Make ordinary DogStatsD aggregation width treatment-configurable

Replace the fixed ordinary `bucketSize` assumption with a separately named DogStatsD aggregation interval in `AgentDemultiplexerOptions`, passed explicitly to each `TimeSampler`. Keep normal control at ten seconds and keep this interval independent from `FlushInterval`, which controls buffered aggregation and serializer wakeups. Split the timestamped no-aggregation count/rate normalization and `Serie.Interval` value into its own fixed ten-second constant before making ordinary width configurable. Do not route timestamped samples into ordinary buckets or derive their fixed normalization metadata from the treatment bucket width.

- Areas: `pkg/aggregator/aggregator.go`, `pkg/aggregator/demultiplexer_agent.go`, `pkg/aggregator/time_sampler.go`, `pkg/aggregator/no_aggregation_stream_worker.go`, tests, and any metric-lookback code that still assumes a ten-second normal DogStatsD interval even though metric lookback is out of experiment scope.
- Tests first: adjacent one-second gauge buckets; ordinary counter and rate normalization; histogram and distribution/sketch semantics; late points; timestamped no-aggregation gauge/count value, timestamp, type, and ten-second `Serie.Interval` parity; recurrent internal series at one-second treatment flush; one-second bucket plus one-second flush; shutdown/incomplete bucket behavior; disabled ten-second parity.
- Performance gate: quantify contexts, buckets, sketches, points, payload bytes, serialization time, request count, and packet drops. Expect up to ten times more DogStatsD points, up to fifteen times more serializer wakeups/requests, and materially fewer samples per sketch.
- Done when: ordinary DogStatsD produces correctly timestamped and normalized one-second buckets in treatment, timestamped DogStatsD remains unchanged, recurrent internal series appear at one second, and all existing control behavior remains intact.

### 5. Validate source-limited and non-metric paths without broadening the claim

Exercise OpenMetrics, a real integration, container/system metrics, ordinary DogStatsD, timestamped DogStatsD, runtime/OTLP input when present, and at least one slow/cache-backed source. Confirm that process, APM stats, orchestrator, network, security, and inventory payloads are either natively queryable by Bits or explicitly outside the metric claim.

- Tests/fixtures: deterministic producer fixtures with known one-second and slower cadences; retain available source timestamps and expected aggregation.
- Intake-level regression: add a fakeintake E2E assertion for control and treatment showing one-second scheduled-check points, ordinary DogStatsD buckets, unchanged timestamped DogStatsD count/gauge semantics, recurrent internal series, and multiple timestamps surviving serialization. Follow `test/fakeintake/AGENTS.md` and the `/write-e2e` workflow when implementing it.
- Done when: each coverage-matrix row has observed evidence from the custom image or an explicit owner-approved exclusion, fakeintake proves payload fidelity, and no dedicated product payload is counted as a metric-series success.

### 6. Build, qualify, and publish one immutable custom Agent image

Build once from a pinned Agent commit and record the full repository SHA, uncommitted patch digest if applicable, toolchain, build tags/flavor, base image, architecture, registry URI, SBOM/signature requirements, and final manifest digest. Use the same digest for control and every treatment execution.

- Required command policy: use `dda inv`; never invoke raw `go build`, `go test`, `go vet`, or `golangci-lint`.
- Focused verification:

```bash
dda inv test --targets=./pkg/collector/scheduler
dda inv test --targets=./pkg/collector/worker
dda inv test --targets=./pkg/collector/sharedlibrary/sharedlibraryimpl
dda inv test --targets=./pkg/collector/python
dda inv test --targets=./pkg/aggregator
dda inv agent.build --build-exclude=systemd
dda inv linter.go
```

- Soak gate: compare control and treatment CPU, RSS, check duration, skipped checks, runner utilization, DogStatsD packets/drops, context count, point/sketch volume, payload bytes, request count, and serializer latency under representative GenSim load.
- Initial isolated-run limits: no restart/OOM; Agent CPU and RSS each remain below 80% of their container limit; zero sustained DogStatsD packet drops or serializer errors; scheduler queue-lag p95 below 500 ms; at least 95% of expected one-second points for every source used in the quality claim; no upstream API throttling. Tighten these limits after the control baseline rather than relaxing them after a treatment failure.
- Done when: the immutable digest passes tests/build/lint and fakeintake, control behavior matches baseline, treatment remains inside every limit above, and the image provenance record is complete.

### 7. Establish the managed GenSim blueprint contract

Do not submit through legacy Agent EKS, local `episode-ctl`, or direct raw gs-flow merely because those paths permit mutation. Obtain an owner-approved blueprint change that still runs through gs-episode-worker lifecycle and managed Sims Archiver provenance.

GenSim owner feedback on 2026-08-05 establishes that the simulator image is selected by the Terrapin tier, while the Datadog Agent image and configuration belong to the scenario's own deployment. A custom Agent is therefore a blueprint/app-bundle change, not a `generator_image` override. The investigation must now identify the maintained blueprint source and its Agent rendering path rather than assume a new gs-flow parameter. See the [GenSim thread](https://dd.slack.com/archives/C09FH4THJTH/p1785949454279909?thread_ts=1785878213.523029&cid=C09FH4THJTH).

The supported blueprint and execution contract must carry and retain:

```text
blueprint repository, path, revision, and owner
Agent deployment resource and rendering component
agent_image_digest
metric_resolution_experiment.enabled
metric_resolution_experiment.check_interval
metric_resolution_experiment.dogstatsd_aggregation_interval
metric_resolution_experiment.serializer_flush_interval
episode_id and app_bundle_digest
generator/simulator digest
Terrapin tier and snapshot
workload/fault/seed identity
```

- Preferred mechanism: one immutable blueprint/app-bundle revision pins the digest-qualified custom Agent image; a bounded execution-time configuration toggle selects disabled or enabled mode if the supported renderer permits it.
- Acceptable fallback: two immutable blueprint revisions generated from the same source whose normalized deployment diff is limited to the experiment setting. This is weaker than the same rendered blueprint and must not change application, simulator, fault, or Agent image bytes.
- Required proof: inspect the rendered Agent resource, running workload image ID, and effective Agent configuration. The observed image must match the expected digest, and the relevant check, ordinary DogStatsD bucket, and serializer-flush settings must resolve to one second in the throwaway treatment.
- Provenance requirement: link blueprint revision, Agent digest, normalized config digest, gs-admin execution UUID, archive root, Terrapin tier/snapshot, and generator digest in the evidence record even if gs-admin does not yet expose first-class Agent fields.
- Done when: GenSim owners provide the maintained blueprint path, production submission command/API, registry/platform constraints, running-workload read-back procedure, rendered-manifest evidence, and archive/execution metadata locations.
- Stop condition: do not use `generator_image` as the Agent image, create or mutate a shared Terrapin tier for the Agent, bypass the managed archival lifecycle, or proceed when the executed Agent digest/config cannot be read back.

### 8. Prove live-backend and Bits query fidelity with one throwaway run

Before any canonical control/treatment pair, run one short treatment-only plumbing capture with a uniquely named deterministic metric. The fixture must emit changing values with exact tags and one-second source timestamps so aggregation cannot be mistaken for legitimate repeated values. It may be registered as a disposable Scenario Store smoke scenario, but it is excluded from the canonical paired dataset and final treatment estimate.

Compare the same selected series at:

```text
source/check fixture or DogStatsD packets
Agent serializer payload or intake-visible series
normal Datadog metrics backend query
Bits metric-tool request and response in the DDEval trace
```

Retain raw timestamp/value/tag rows where available and calculate:

```text
point_count
first and last timestamps
median and p95 inter-point interval
maximum gap
missing-second count
duplicate timestamp count
out-of-order count
value-change count
repeated-value fraction
```

Delivery batching is acceptable; event-time collapse is not. Require the live backend to retain distinct approximately one-second timestamps, expected changing values, and the complete tag set. Require the Bits trace to prove the same telemetry org, tags, historical window, and useful fine-resolution response. A coarse query-time rollup, timestamp rewriting, wrong scope/window, tag loss, failed metric request, or unexplained point-count reduction is a stop condition.

The managed archive remains required for this execution's logs, traces, events, metadata, and Scenario Store lineage. Sims Archiver metric Parquet and Eval Data Portal metric hydration are optional and do not gate the experiment.

- Done when: one managed gs-admin execution proves the expected custom Agent digest/config, the normal backend contains the expected one-second series, and a disposable DDEval investigation can query the intended historical metric evidence.
- Stop condition: do not build the paired matrix while custom-image ownership is ambiguous or the custom Agent, live backend, or Bits metric query removes or misroutes one-second evidence.

## Stop and rollback conditions

Stop treatment execution if any of the following occurs:

- the Agent image digest differs between arms;
- app bundle, workload, fault sequence, simulator, Terrapin snapshot, or evaluation configuration drifts;
- a custom image cannot be read back from the running pod;
- the treatment causes sustained runner starvation, unbounded queueing, packet drops, OOM, or episode instability;
- check duration prevents meaningful one-second observation for a material source class;
- DogStatsD rate/counter/sketch semantics differ for reasons beyond bucket width;
- normal control metrics disappear or treatment tags reveal the arm to Bits;
- the managed non-metric archive or Scenario Store record is incomplete;
- the live backend or Bits query-time rollup removes one-second timestamps;
- the evaluation queries the wrong telemetry org, environment, tags, or historical window.

Rollback is configuration-only: disable `metric_resolution_experiment` while retaining the same image. If disabled mode fails parity, discard the image rather than using a separate control binary.

## Questions and details for GenSim owners

Send the GenSim team the following concise request:

> We are preparing paired Bits SRE DDEval captures using one custom Datadog Agent image. Both arms must use the identical immutable Agent digest and app/workload; control leaves an experimental global-1-Hz setting disabled and treatment enables one-second scheduling for every normal recurring check, one-second ordinary DogStatsD aggregation, and one-second serializer flushing. Timestamped DogStatsD remains on its existing no-aggregation path. We understand that the Agent image/config is part of the scenario blueprint rather than `generator_image`. Which maintained blueprint/app-bundle source installs the Agent, how should we pin its digest and bounded config, how do we submit that revision through gs-episode-worker, and where can we read the effective values back from the running workload and resulting execution/archive metadata?

Include these details in the request:

1. **Image identity:** proposed registry URI plus immutable `@sha256:` digest, Agent repository commit, image flavor, OS/architecture, and whether the image is signed/scanned. If the digest does not exist yet, provide the intended registry and build workflow rather than a mutable tag.
2. **Same-image invariant:** explicitly state that both arms use the same image; only `metric_resolution_experiment.enabled` changes.
3. **Treatment keys:** provide the exact bounded YAML/env keys and defaults, including the check, ordinary DogStatsD bucket, and serializer-flush intervals; no arbitrary secret map and no metric allowlist.
4. **Blueprint ownership:** ask for the maintained repository/path and owner of the blueprint or app bundle that renders the Agent resource, including which source supersedes older generated Agent templates.
5. **Installation semantics:** ask whether that blueprint installs the Agent at execution time or references bytes baked into a snapshot, while keeping the simulator's Terrapin tier separate from Agent selection. State that `generator_image` is known not to be the Agent selector.
6. **Registry requirements:** ask for registry allowlists, pull credentials, mirroring, architecture, signing, vulnerability policy, and digest-only requirements.
7. **Config injection:** record that the current worker request has no bounded Agent toggle. Use owner-approved immutable app-bundle variants with a normalized diff limited to the enabled environment value unless owners provide a new supported per-execution boundary before capture.
8. **Pair determinism:** require pinning episode ID, app-bundle digest, generator/simulator digest, Terrapin tier/snapshot, traffic/fault parameters, seed, backend, and telemetry org.
9. **Read-back proof:** require the running pod image ID, effective Agent configuration (`agent configcheck` plus relevant config dump), and rendered control/treatment manifest diff.
10. **Archive provenance:** require image digest and normalized treatment config in gs-admin/execution/archive metadata, not only local notes.
11. **Lifecycle:** confirm the run remains under gs-episode-worker and managed Sims Archiver; direct gs-flow is acceptable only if owners document equivalent lifecycle/provenance.
12. **Operational envelope:** provide expected CPU/RSS/payload amplification, one worker/concurrent episode requirement, warm-up duration, and stop thresholds.

Do not include API keys, application keys, bearer tokens, kubeconfigs, AWS credentials, registry credentials, or auth-bearing URLs in the request or plan artifacts.

## Assumptions requiring verification

- The selected episode's useful metric evidence belongs primarily to scheduled checks and DogStatsD rather than unsupported dedicated payloads.
- GenSim can run a digest-pinned custom Agent without changing application bytes or shared Terrapin state.
- The custom Agent's normal serializer path retains distinct one-second points already supported by the backend and demonstrated by Metric Lookback prior art.
- Bits' metric tool can query the capture's normal-backend metrics within approximately 15 months using the correct telemetry org, tags, and historical window.
- Bits' metric query does not apply a coarse rollup that erases the treatment resolution.
- The managed archive and Scenario Store preserve the same execution's non-metric incident context.
- Resource amplification is acceptable only in isolated, reserved GenSim executions; this is not a proposed production default.

If any assumption fails, update this plan and the main evaluation runbook before continuing.
