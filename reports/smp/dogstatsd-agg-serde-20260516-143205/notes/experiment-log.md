# Execution log

## 2026-05-16 14:32 local

Started local SMP execution for DogStatsD aggregator + serializer experiment.

Initial repository state:

- Current branch: `dsd-milestone-0-guardrails`
- Current HEAD: `53f7e8fdc3d`
- Untracked local files observed before execution:
  - `.pi/`
  - `PLAN.md`
  - `SMP_LOCAL_BENCHMARK_SKILL.md`
  - `comp/dogstatsd/SMP_EXPERIMENT_PLAN.md`

Initial tooling checks:

- `smp` found at `/private/tmp/nono-session-4875/shims/smp`; version output says `smp 0.0.0` with startup log `v0.27.0-alpha.0`.
- `dda` found at `/opt/homebrew/bin/dda`; version `0.33.1`.
- `docker` found at `/opt/homebrew/bin/docker`, but Docker daemon was not initially reachable at `/var/run/docker.sock`.
- `colima` found at `/opt/homebrew/bin/colima`, but was not running.

Next action: start a local Docker daemon via Colima for SMP local runs.

## 2026-05-16 14:33 local

Primary sandbox could not start Colima because `ssh` is mediated by nono:

```text
nono-mediation: 'ssh' cannot be invoked from the primary sandbox
```

Started Colima successfully from a cmux terminal pane instead:

```bash
colima start --cpu 6 --memory 8 --disk 80
```

For subsequent tool calls, use:

```bash
export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"
export TMPDIR="$HOME/.tmp-smp"
```

Docker server is reachable through that socket:

```text
29.2.1 aarch64 Ubuntu 24.04.4 LTS
```

## 2026-05-16 14:35 local

Initial main image build was started with the default `agent.hacky-dev-image-build` options. Process inspection showed it was compiling with debug flags (`-gcflags=all=-N -l`) because the task defaults to `--development`. That would distort SMP performance data. Aborted that attempt and will rebuild all comparison images with `--no-development`.

## 2026-05-16 14:39 local

Mac-host `agent.hacky-dev-image-build` is not suitable for SMP Linux target images: process inspection showed it was building a Darwin/arm64 Agent binary and would later copy that into a Linux image. Aborted the host-side optimized build as well. Next attempt: run `dda inv agent.hacky-dev-image-build --no-development` inside the Linux `datadog/agent-dev-env-linux` container, with the Colima Docker socket mounted, so produced binaries are Linux/arm64.

## 2026-05-16 14:45 local

First Linux-container build attempt pulled `datadog/agent-dev-env-linux:latest` but failed before building because mounting the host-side Colima socket path is invalid from the Colima VM:

```text
docker: Error response from daemon: error while creating mount source path '/Users/luke.steensen/.colima/default/docker.sock': mkdir /Users/luke.steensen/.colima/default/docker.sock: operation not supported
```

Verified the correct pattern is:

```bash
docker run --entrypoint bash \
  -v "$PWD:/work" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -w /work datadog/agent-dev-env-linux:latest -lc '...'
```

Also need `--entrypoint bash` because the dev-env default entrypoint starts `sshd` and ignores the command.

## 2026-05-16 14:47 local

Linux-container main image build failed on stale CMake cache from the earlier Mac-host build attempt:

```text
CMake Error: The current CMakeCache.txt directory /work/rtloader/build/CMakeCache.txt is different than the directory /Users/luke.steensen/code/datadog-agent-smp-main/rtloader/build where CMakeCache.txt was created.
```

Cleaned generated rtloader/dev/bin outputs in the main and foundation worktrees before retrying:

```bash
rm -rf rtloader/build dev/embedded dev/include dev/lib bin/agent
```

## 2026-05-16 14:49 local

Linux-container main image build progressed through rtloader but failed because the worktree was mounted at `/work`; the Git worktree `.git` file points to an absolute host gitdir under `/Users/luke.steensen/code/datadog-agent/.git/worktrees/...`, which did not exist in the container:

```text
fatal: not a git repository: /Users/luke.steensen/code/datadog-agent/.git/worktrees/datadog-agent-smp-main
```

Cleaned generated outputs again. Next retry mounts `/Users/luke.steensen/code` into the same absolute path inside the container and runs from `/Users/luke.steensen/code/datadog-agent-smp-main`.

## 2026-05-16 14:54 local

Built and smoke-checked main baseline image successfully using the Linux dev-env container.

Image:

```text
datadog/agent-dev:smp-dsd-main
commit: 3ec880f14a3
image id: sha256:6f67e85689c833453bb60ba0697d234561a39a9f8df37d36f2a7fd6372316419
platform: linux/arm64
agent version: Agent 7.81.0-devel - Meta: git.21.3ec880f - Commit: 3ec880f14a3 - Serialization version: v5.0.196 - Go version: go1.25.10
```

Proceeding to build foundation image from commit `53f7e8fdc3d` using the same Linux-container method.

## 2026-05-16 14:58 local

Built and smoke-checked foundation image successfully.

Image:

```text
datadog/agent-dev:smp-dsd-foundation
commit: 53f7e8fdc3d
image id: sha256:10c128c6e2f0857ebbdeb9c5a5b2644cb8c112ec4345882537bd24c643966580
platform: linux/arm64
agent version: Agent 7.81.0-devel - Meta: git.32.53f7e8f - Commit: 53f7e8fdc3d - Serialization version: v5.0.196 - Go version: go1.25.10
```

Proceeding to SMP smoke tests and then baseline-vs-foundation comparisons.

## 2026-05-16 local, continuation after pi crash

The prior pi TUI session crashed because too many cmux splits had been created. Continuing with the existing report directory and local images, but changing execution strategy:

- Avoid creating additional cmux panes unless absolutely necessary.
- Run SMP commands sequentially through normal `bash` tool calls with output captured via `tee`.
- Keep raw SMP logs under `cases/` and append execution notes here.

Current usable images confirmed after restart:

```text
datadog/agent-dev:smp-dsd-main       sha256:6f67e85689c8... linux/arm64
datadog/agent-dev:smp-dsd-foundation sha256:10c128c6e2f0... linux/arm64
ghcr.io/datadog/lading:0.31.2        present locally
```

## Smoke test: foundation on `uds_dogstatsd_to_api_v3`

Command:

```bash
smp local smoketest \
  --experiment-dir test/regression \
  --case uds_dogstatsd_to_api_v3 \
  --target-image datadog/agent-dev:smp-dsd-foundation \
  --total-samples 60 \
  --follow all
```

Smoke test result: completed successfully for foundation image on `uds_dogstatsd_to_api_v3` with 60 samples. Raw log: `cases/smoke-foundation-uds_dogstatsd_to_api_v3.log`. Captures copied to `captures/smoke-foundation-uds_dogstatsd_to_api_v3/`.

Notable local-only noise in logs:

- DNS failures for external Datadog intake endpoints due local/network sandboxing.
- Prometheus scrape warnings against `http://127.0.0.1:5000/telemetry` during startup.
- These occurred during smoke; need consider when interpreting local SMP results.

## SMP comparison: baseline main vs foundation, `uds_dogstatsd_to_api_v3`

Experiment represented: Stage A baseline-vs-foundation, primary DogStatsD v3 serializer throughput case.

Command:

```bash
smp local run --experiment-dir test/regression --case uds_dogstatsd_to_api_v3 --baseline-image datadog/agent-dev:smp-dsd-main --comparison-image datadog/agent-dev:smp-dsd-foundation --replicates 3 --total-samples 270
```

Completed `uds_dogstatsd_to_api_v3` with exit status 1. Raw SMP log: `cases/stageA-main-vs-foundation-uds_dogstatsd_to_api_v3.log`. Captures copied to `captures/stageA-main-vs-foundation-uds_dogstatsd_to_api_v3/` when present.

The first full Stage A run for `uds_dogstatsd_to_api_v3` failed before producing a comparative result because SMP attempted to allocate CPU sets outside the Colima VM's available CPUs:

```text
Error: Docker responded with status code 400: Requested CPUs are not available - requested 6-8, available: 0-5
```

This is a local environment capacity issue: the case requests `target.cpu_allotment: 4`, and SMP local tries to isolate CPUs for baseline/comparison. The Colima VM currently has 6 CPUs. Attempts to stop/resize Colima from the primary sandbox failed due local process permissions, and `colima start --cpu 10` was ignored because the VM is already running.

To continue collecting same-machine comparative data without more cmux/VM manipulation, creating a local-only scaled experiment directory with the same cases but `target.cpu_allotment` reduced from 4 to 2. This changes absolute performance, so results must be labeled as `local-2cpu` and used for directional local evidence only, not as official quality-gate-equivalent data.

## SMP comparison: local-2cpu main vs foundation, `uds_dogstatsd_to_api_v3`

Experiment represented: Stage A baseline-vs-foundation, primary DogStatsD v3 serializer throughput case, local-only scaled CPU allotment () because Colima exposes only CPUs 0-5.

Command:

```bash
smp local run --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-2cpu --case uds_dogstatsd_to_api_v3 --baseline-image datadog/agent-dev:smp-dsd-main --comparison-image datadog/agent-dev:smp-dsd-foundation --replicates 3 --total-samples 270
```

Completed local-2cpu `uds_dogstatsd_to_api_v3` with exit status 1. Raw SMP log: `cases/stageA-local2cpu-main-vs-foundation-uds_dogstatsd_to_api_v3.log`. Captures copied to `captures/stageA-local2cpu-main-vs-foundation-uds_dogstatsd_to_api_v3/` when present.

## Colima resized for official local SMP cases

Opened one temporary cmux surface solely to resize Colima, then closed it immediately to avoid the prior TUI crash mode. Resize succeeded:

```text
PROFILE    STATUS     ARCH       CPUS    MEMORY    DISK     RUNTIME
default    Running    aarch64    10      12GiB     80GiB    docker
```

The earlier `local-2cpu` workaround attempts both failed with the same cpuset error and should be ignored except as environment troubleshooting artifacts. Returning to the source experiment definitions under `test/regression` for the real Stage A runs.

## SMP comparison: main vs foundation, uds_dogstatsd_to_api_v3

Experiment represented: Stage A baseline-vs-foundation, primary DogStatsD v3 serializer throughput case, original source case from test/regression after resizing Colima to 10 CPUs.

Command: smp local run --experiment-dir test/regression --case uds_dogstatsd_to_api_v3 --baseline-image datadog/agent-dev:smp-dsd-main --comparison-image datadog/agent-dev:smp-dsd-foundation --replicates 3 --total-samples 270

Completed uds_dogstatsd_to_api_v3 with exit status 1. Raw SMP log: cases/stageA-main-vs-foundation-uds_dogstatsd_to_api_v3.log. Captures copied to captures/stageA-main-vs-foundation-uds_dogstatsd_to_api_v3/ when present.

The official `uds_dogstatsd_to_api_v3` run failed during replicate 2 due stale local containers from earlier failed SMP attempts:

```text
Conflict. The container name "/baseline-1-init" is already in use
```

Removed stale `baseline-*` / `comparison-*` local containers and will rerun the case from scratch. The partial log remains as `cases/stageA-main-vs-foundation-uds_dogstatsd_to_api_v3.log` for troubleshooting; completed result will be written with `-rerun1` suffix.

## SMP comparison rerun1: main vs foundation, uds_dogstatsd_to_api_v3

Experiment represented: Stage A baseline-vs-foundation, primary DogStatsD v3 serializer throughput case, original source case from test/regression. Rerun after cleaning stale Docker containers.

Completed rerun1 uds_dogstatsd_to_api_v3 with exit status 0. Raw SMP log: cases/stageA-main-vs-foundation-uds_dogstatsd_to_api_v3-rerun1.log. Captures copied to captures/stageA-main-vs-foundation-uds_dogstatsd_to_api_v3-rerun1/ when present.

## SMP comparison: main vs foundation, uds_dogstatsd_to_api

Experiment represented: Stage A baseline-vs-foundation, DogStatsD-to-API compatibility/guardrail throughput case, original source case from test/regression.

Command: smp local run --experiment-dir test/regression --case uds_dogstatsd_to_api --baseline-image datadog/agent-dev:smp-dsd-main --comparison-image datadog/agent-dev:smp-dsd-foundation --replicates 3 --total-samples 270

Completed uds_dogstatsd_to_api with exit status 0. Raw SMP log: cases/stageA-main-vs-foundation-uds_dogstatsd_to_api.log. Captures copied to captures/stageA-main-vs-foundation-uds_dogstatsd_to_api/ when present.

## SMP comparison: main vs foundation, uds_dogstatsd_20mb_12k_contexts_20_senders

Experiment represented: Stage A baseline-vs-foundation, DogStatsD memory/cardinality/concurrency case, original source case from test/regression.

Command: smp local run --experiment-dir test/regression --case uds_dogstatsd_20mb_12k_contexts_20_senders --baseline-image datadog/agent-dev:smp-dsd-main --comparison-image datadog/agent-dev:smp-dsd-foundation --replicates 3 --total-samples 270

Completed uds_dogstatsd_20mb_12k_contexts_20_senders with exit status 0. Raw SMP log: cases/stageA-main-vs-foundation-uds_dogstatsd_20mb_12k_contexts_20_senders.log. Captures copied to captures/stageA-main-vs-foundation-uds_dogstatsd_20mb_12k_contexts_20_senders/ when present.

## Stage A summary extraction and report generation

Generated persistent summary artifacts from the completed official Stage A runs:

- `summary.csv`: exact SMP table values from completed logs only.
- `selected_metrics.sql`: DuckDB query used to derive explanatory metrics from copied `captures.parquet` files.
- `selected_metrics.csv`: selected metric aggregates by case and variant.
- `report.html`: single-page HTML report.
- `README.md`: updated with Stage A verdict and artifact index.

Completed Stage A verdict: foundation is SMP-neutral in the three primary DogStatsD cases. No completed SMP run was classified as a regression or improvement. The direct aggregator-row / payload-segment serializer improvement hypothesis remains unproven and needs an invasive experiment branch before it can be claimed.

## Stage B start: instrument-only experiment branch

Created local branch `dogstatsd-agg-serde-experiment` from foundation commit `53f7e8fdc3d`. Stage B goal: add per-flush/per-payload telemetry only, with no DogStatsD dataflow or output behavior changes, then build `datadog/agent-dev:smp-dsd-experiment` and compare foundation vs experiment on the same three SMP cases.

### Stage B implementation checkpoint

Added instrument-only telemetry on `dogstatsd-agg-serde-experiment`:

- `dogstatsd_pipeline.flush_duration_ns{phase}` and `dogstatsd_pipeline.flushes{phase}` for per-flush phase timings:
  - `dogstatsd_samplers`
  - `check_samplers`
  - `producer_total`
  - `serialize_series`
  - `serialize_sketches`
  - `flush_total`
- `dogstatsd_pipeline.items{kind}` for flushed series/sketch counts, point counts, and tag counts.
- `serializer.series_pipeline_duration_ns{phase=marshal_split_compress}` and event count.
- `serializer.v3_payload_stats{stat}` for payloads, points, compressed bytes, and uncompressed metric-data bytes.
- `serializer.v3_dictionary_entries{dictionary}` for metrics-v3 payload-local dictionary entry counts.

The implementation records only per-flush/per-payload/per-series counters. It does not change aggregation semantics, serializer output, or DogStatsD dataflow.

Verification:

```bash
dda inv test --targets=./pkg/aggregator,./pkg/serializer/internal/metrics --timeout=300
```

Result: 244/244 reported tests passed (310 package-level test executions including packages without tests).

### Stage B experiment image build start

Building optimized Linux/arm64 experiment image from commit `538ae360d89`:

```bash
dda inv agent.hacky-dev-image-build --target-image datadog/agent-dev:smp-dsd-experiment --no-development
```

The build runs inside `datadog/agent-dev-env-linux:latest` with `/Users/luke.steensen/code` mounted at the same absolute path and `/var/run/docker.sock` mounted for the Colima daemon.

Stage B experiment image build exit status: 1. Raw log: notes/build-experiment-stageB-linux-container.log.

Stage B first experiment image build failed before Go compilation because Git inside the Linux dev-env container rejected the mounted repo as dubious ownership:

```text
fatal: detected dubious ownership in repository at '/Users/luke.steensen/code/datadog-agent'
```

Retrying with `git config --global --add safe.directory /Users/luke.steensen/code/datadog-agent` inside the build container.

Stage B experiment image build rerun1 exit status: 0. Raw log: notes/build-experiment-stageB-linux-container-rerun1.log.

Stage B experiment image built successfully on rerun1.

```text
image: datadog/agent-dev:smp-dsd-experiment
image id: sha256:38a3a685d98e4b6d801750b084d11a186a3a18cddd65516ab7a28738e0a4828a
agent version: Agent 7.81.0-devel - Meta: git.33.538ae36 - Commit: 538ae360d89 - Serialization version: v5.0.196 - Go version: go1.25.10
```

### Stage B smoke test: experiment image on uds_dogstatsd_to_api_v3

Command: smp local smoketest --experiment-dir test/regression --case uds_dogstatsd_to_api_v3 --target-image datadog/agent-dev:smp-dsd-experiment --total-samples 60 --follow all

Stage B smoke test uds_dogstatsd_to_api_v3 exit status: 0. Raw log: cases/smoke-stageB-experiment-uds_dogstatsd_to_api_v3.log. Captures copied to captures/smoke-stageB-experiment-uds_dogstatsd_to_api_v3/ when present.

### Stage B design adjustment: v3 case activation

The Stage B smoke capture for source case `uds_dogstatsd_to_api_v3` showed the new aggregate serializer timing metrics but no `serializer.v3_payload_stats` / `serializer.v3_dictionary_entries` metrics. Inspecting current serializer config showed that v3 is selected by `serializer_experimental_use_v3_api.series.endpoints`, while the SMP source case still sets the older-looking boolean env var `DD_SERIALIZER_EXPERIMENTAL_USE_V3_API_SERIES=true`.

Design adjustment: preserve the source case results as compatibility throughput data, but add a local corrected case for actual v3 serializer proof:

```text
reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-v3-endpoint-fixed/cases/uds_dogstatsd_to_api_v3_endpoint_fixed
```

The local case is copied from `test/regression/cases/uds_dogstatsd_to_api_v3` and replaces the env var with:

```yaml
DD_SERIALIZER_EXPERIMENTAL_USE_V3_API_SERIES_ENDPOINTS: '["http://127.0.0.1:9091"]'
```

This targets the configured local `dd_url` resolver and should activate the v3 payload builder. Results from this local corrected case must be labeled separately from official source-case SMP results.

### Stage B smoke test: experiment image on corrected v3 local case

Command: smp local smoketest --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-v3-endpoint-fixed --case uds_dogstatsd_to_api_v3_endpoint_fixed --target-image datadog/agent-dev:smp-dsd-experiment --total-samples 60 --follow all

Stage B corrected-v3 smoke test uds_dogstatsd_to_api_v3_endpoint_fixed exit status: 0. Raw log: cases/smoke-stageB-experiment-uds_dogstatsd_to_api_v3_endpoint_fixed.log. Captures copied to captures/smoke-stageB-experiment-uds_dogstatsd_to_api_v3_endpoint_fixed/ when present.

## Stage B SMP comparison: foundation vs experiment, uds_dogstatsd_to_api_v3

Experiment represented: Stage B instrumentation-overhead comparison using the source SMP case. Note: this source case did not activate v3 payload telemetry in smoke and is treated as compatibility throughput unless corrected separately.

Command: smp local run --experiment-dir test/regression --case uds_dogstatsd_to_api_v3 --baseline-image datadog/agent-dev:smp-dsd-foundation --comparison-image datadog/agent-dev:smp-dsd-experiment --replicates 3 --total-samples 270

Completed Stage B uds_dogstatsd_to_api_v3 with exit status 0. Raw SMP log: cases/stageB-foundation-vs-experiment-uds_dogstatsd_to_api_v3.log. Captures copied to captures/stageB-foundation-vs-experiment-uds_dogstatsd_to_api_v3/ when present.

## Stage B SMP comparison: foundation vs experiment, uds_dogstatsd_to_api

Experiment represented: Stage B instrumentation-overhead comparison, DogStatsD-to-API compatibility/guardrail throughput case.

Command: smp local run --experiment-dir test/regression --case uds_dogstatsd_to_api --baseline-image datadog/agent-dev:smp-dsd-foundation --comparison-image datadog/agent-dev:smp-dsd-experiment --replicates 3 --total-samples 270

Completed Stage B uds_dogstatsd_to_api with exit status 0. Raw SMP log: cases/stageB-foundation-vs-experiment-uds_dogstatsd_to_api.log. Captures copied to captures/stageB-foundation-vs-experiment-uds_dogstatsd_to_api/ when present.

## Stage B SMP comparison: foundation vs experiment, uds_dogstatsd_20mb_12k_contexts_20_senders

Experiment represented: Stage B instrumentation-overhead comparison, DogStatsD memory/cardinality/concurrency case.

Command: smp local run --experiment-dir test/regression --case uds_dogstatsd_20mb_12k_contexts_20_senders --baseline-image datadog/agent-dev:smp-dsd-foundation --comparison-image datadog/agent-dev:smp-dsd-experiment --replicates 3 --total-samples 270

Completed Stage B uds_dogstatsd_20mb_12k_contexts_20_senders with exit status 0. Raw SMP log: cases/stageB-foundation-vs-experiment-uds_dogstatsd_20mb_12k_contexts_20_senders.log. Captures copied to captures/stageB-foundation-vs-experiment-uds_dogstatsd_20mb_12k_contexts_20_senders/ when present.

## Stage B SMP comparison: foundation vs experiment, corrected local v3 endpoint case

Experiment represented: Stage B instrumentation-overhead comparison with actual metrics-v3 payload builder enabled via local corrected endpoint configuration.

Command: smp local run --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-v3-endpoint-fixed --case uds_dogstatsd_to_api_v3_endpoint_fixed --baseline-image datadog/agent-dev:smp-dsd-foundation --comparison-image datadog/agent-dev:smp-dsd-experiment --replicates 3 --total-samples 270

Completed Stage B corrected-v3 uds_dogstatsd_to_api_v3_endpoint_fixed with exit status 0. Raw SMP log: cases/stageB-foundation-vs-experiment-uds_dogstatsd_to_api_v3_endpoint_fixed.log. Captures copied to captures/stageB-foundation-vs-experiment-uds_dogstatsd_to_api_v3_endpoint_fixed/ when present.

## Stage C start: shadow payload-aligned segment builder

Implemented an output-neutral shadow segment builder in `pkg/serializer/internal/metrics` that consumes the already-flushed `metrics.Serie` / `metrics.SketchSeries` objects during serializer iteration and builds payload-local dictionary/cardinality estimates. The authoritative serializer output is unchanged.

Telemetry added:

- `serializer.segment_shadow_stats{stat}` for flushes, series rows, sketch rows, points, sketch bins, estimated uncompressed bytes, and fallbacks.
- `serializer.segment_shadow_dictionary_entries{dictionary}` for names, tag strings, tagsets, resource strings, resources, source type names, origins, and units.
- `serializer.segment_shadow_duration_ns{phase}` for series/sketch shadow build time.

Verification:

```bash
dda inv test --targets=./pkg/serializer/internal/metrics --timeout=300
dda inv test --targets=./pkg/aggregator,./pkg/serializer/internal/metrics --timeout=300
```

Result: targeted serializer package passed 33/33 reported tests; broader aggregator+serializer run passed 245/245 reported tests.

### Stage C shadow image build start

Building optimized Linux/arm64 shadow image from commit `911b22716ca`:

```bash
dda inv agent.hacky-dev-image-build --target-image datadog/agent-dev:smp-dsd-shadow --no-development
```

Stage C shadow image build exit status: 0. Raw log: notes/build-shadow-stageC-linux-container.log.

## Stage C smoke: shadow image, corrected v3 endpoint case

Command: smp local smoketest --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-v3-endpoint-fixed --case uds_dogstatsd_to_api_v3_endpoint_fixed --target-image datadog/agent-dev:smp-dsd-shadow --total-samples 60 --follow all

Completed Stage C shadow smoke with exit status 0. Raw SMP log: cases/smoke-stageC-shadow-uds_dogstatsd_to_api_v3_endpoint_fixed.log. Captures copied to captures/smoke-stageC-shadow-uds_dogstatsd_to_api_v3_endpoint_fixed/ when present.

Stage C smoke telemetry check: DuckDB over the copied smoke capture found the expected shadow metrics:

```text
target/serializer.segment_shadow_dictionary_entries
target/serializer.segment_shadow_duration_ns
target/serializer.segment_shadow_stats
```
\n## Stage C SMP comparison: experiment vs shadow, uds_dogstatsd_to_api_v3
Command: smp local run --experiment-dir test/regression --case uds_dogstatsd_to_api_v3 --baseline-image datadog/agent-dev:smp-dsd-experiment --comparison-image datadog/agent-dev:smp-dsd-shadow --replicates 3 --total-samples 270

Completed Stage C uds_dogstatsd_to_api_v3 with exit status 0. Raw SMP log: cases/stageC-experiment-vs-shadow-uds_dogstatsd_to_api_v3.log. Captures copied to captures/stageC-experiment-vs-shadow-uds_dogstatsd_to_api_v3/ when present.
\n## Stage C SMP comparison: experiment vs shadow, uds_dogstatsd_to_api
Command: smp local run --experiment-dir test/regression --case uds_dogstatsd_to_api --baseline-image datadog/agent-dev:smp-dsd-experiment --comparison-image datadog/agent-dev:smp-dsd-shadow --replicates 3 --total-samples 270

Completed Stage C uds_dogstatsd_to_api with exit status 0. Raw SMP log: cases/stageC-experiment-vs-shadow-uds_dogstatsd_to_api.log. Captures copied to captures/stageC-experiment-vs-shadow-uds_dogstatsd_to_api/ when present.
\n## Stage C SMP comparison: experiment vs shadow, uds_dogstatsd_20mb_12k_contexts_20_senders
Command: smp local run --experiment-dir test/regression --case uds_dogstatsd_20mb_12k_contexts_20_senders --baseline-image datadog/agent-dev:smp-dsd-experiment --comparison-image datadog/agent-dev:smp-dsd-shadow --replicates 3 --total-samples 270

Completed Stage C uds_dogstatsd_20mb_12k_contexts_20_senders with exit status 0. Raw SMP log: cases/stageC-experiment-vs-shadow-uds_dogstatsd_20mb_12k_contexts_20_senders.log. Captures copied to captures/stageC-experiment-vs-shadow-uds_dogstatsd_20mb_12k_contexts_20_senders/ when present.
\n## Stage C SMP comparison: experiment vs shadow, uds_dogstatsd_to_api_v3_endpoint_fixed
Command: smp local run --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-v3-endpoint-fixed --case uds_dogstatsd_to_api_v3_endpoint_fixed --baseline-image datadog/agent-dev:smp-dsd-experiment --comparison-image datadog/agent-dev:smp-dsd-shadow --replicates 3 --total-samples 270

Completed Stage C uds_dogstatsd_to_api_v3_endpoint_fixed with exit status 0. Raw SMP log: cases/stageC-experiment-vs-shadow-uds_dogstatsd_to_api_v3_endpoint_fixed.log. Captures copied to captures/stageC-experiment-vs-shadow-uds_dogstatsd_to_api_v3_endpoint_fixed/ when present.

## Stage D start: direct aggregator row shadow sink

Implemented an output-neutral direct row shadow sink in `pkg/aggregator` that observes aggregator-emitted `metrics.Serie` and `metrics.SketchSeries` after context resolution/filtering and before appending to the existing sinks. Authoritative output remains unchanged and Stage C serializer shadowing remains enabled for comparison.

Telemetry added:

- `dogstatsd_direct_row_shadow.stats{stat}` for flushes, series rows, sketch rows, points, sketch bins, tags, estimated uncompressed bytes, and fallbacks.
- `dogstatsd_direct_row_shadow.dictionary_entries{dictionary}` for names, tag strings, tagsets, resource strings, resources, sources, and units.
- `dogstatsd_direct_row_shadow.duration_ns{phase}` for series/sketch direct-row shadow time.

Verification:

```bash
dda inv test --targets=./pkg/aggregator --timeout=300
dda inv test --targets=./pkg/aggregator,./pkg/serializer/internal/metrics --timeout=300
```

Result: aggregator run passed 213/213 reported tests; broader aggregator+serializer run passed 246/246 reported tests.

### Stage D direct-row image build start

Building optimized Linux/arm64 direct-row image from commit `e3f2f987056`:

```bash
dda inv agent.hacky-dev-image-build --target-image datadog/agent-dev:smp-dsd-direct-row --no-development
```

Stage D direct-row image build exit status: 0. Raw log: notes/build-direct-row-stageD-linux-container.log.

## Stage D smoke: direct-row image, corrected v3 endpoint case

Command: smp local smoketest --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-v3-endpoint-fixed --case uds_dogstatsd_to_api_v3_endpoint_fixed --target-image datadog/agent-dev:smp-dsd-direct-row --total-samples 60 --follow all

Completed Stage D direct-row smoke with exit status 0. Raw SMP log: cases/smoke-stageD-direct-row-uds_dogstatsd_to_api_v3_endpoint_fixed.log. Captures copied to captures/smoke-stageD-direct-row-uds_dogstatsd_to_api_v3_endpoint_fixed/ when present.

Stage D smoke telemetry check: DuckDB over the copied smoke capture found the expected direct-row shadow metrics:

```text
target/dogstatsd_direct_row_shadow.dictionary_entries
target/dogstatsd_direct_row_shadow.duration_ns
target/dogstatsd_direct_row_shadow.stats
```
\n## Stage D SMP comparison: shadow vs direct-row, uds_dogstatsd_to_api_v3
Command: smp local run --experiment-dir test/regression --case uds_dogstatsd_to_api_v3 --baseline-image datadog/agent-dev:smp-dsd-shadow --comparison-image datadog/agent-dev:smp-dsd-direct-row --replicates 3 --total-samples 270

Completed Stage D uds_dogstatsd_to_api_v3 with exit status 0. Raw SMP log: cases/stageD-shadow-vs-direct-row-uds_dogstatsd_to_api_v3.log. Captures copied to captures/stageD-shadow-vs-direct-row-uds_dogstatsd_to_api_v3/ when present.
\n## Stage D SMP comparison: shadow vs direct-row, uds_dogstatsd_to_api
Command: smp local run --experiment-dir test/regression --case uds_dogstatsd_to_api --baseline-image datadog/agent-dev:smp-dsd-shadow --comparison-image datadog/agent-dev:smp-dsd-direct-row --replicates 3 --total-samples 270

Completed Stage D uds_dogstatsd_to_api with exit status 0. Raw SMP log: cases/stageD-shadow-vs-direct-row-uds_dogstatsd_to_api.log. Captures copied to captures/stageD-shadow-vs-direct-row-uds_dogstatsd_to_api/ when present.
\n## Stage D SMP comparison: shadow vs direct-row, uds_dogstatsd_20mb_12k_contexts_20_senders
Command: smp local run --experiment-dir test/regression --case uds_dogstatsd_20mb_12k_contexts_20_senders --baseline-image datadog/agent-dev:smp-dsd-shadow --comparison-image datadog/agent-dev:smp-dsd-direct-row --replicates 3 --total-samples 270

Completed Stage D uds_dogstatsd_20mb_12k_contexts_20_senders with exit status 0. Raw SMP log: cases/stageD-shadow-vs-direct-row-uds_dogstatsd_20mb_12k_contexts_20_senders.log. Captures copied to captures/stageD-shadow-vs-direct-row-uds_dogstatsd_20mb_12k_contexts_20_senders/ when present.
\n## Stage D SMP comparison: shadow vs direct-row, uds_dogstatsd_to_api_v3_endpoint_fixed
Command: smp local run --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-v3-endpoint-fixed --case uds_dogstatsd_to_api_v3_endpoint_fixed --baseline-image datadog/agent-dev:smp-dsd-shadow --comparison-image datadog/agent-dev:smp-dsd-direct-row --replicates 3 --total-samples 270

Completed Stage D uds_dogstatsd_to_api_v3_endpoint_fixed with exit status 0. Raw SMP log: cases/stageD-shadow-vs-direct-row-uds_dogstatsd_to_api_v3_endpoint_fixed.log. Captures copied to captures/stageD-shadow-vs-direct-row-uds_dogstatsd_to_api_v3_endpoint_fixed/ when present.

## Stage D summary and Stage E decision

Stage D SMP completed for the three source cases plus the corrected local v3 endpoint case. Exact results are in `summary.csv` and raw logs under `cases/stageD-*`.

Stage D results:

| Case | Goal | Δ mean | Δ mean CI | Regression | Improvement |
|---|---|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3` | ingress throughput | +0.46% | [+0.18%, +0.74%] | false | true |
| `uds_dogstatsd_to_api` | ingress throughput | -0.43% | [-0.65%, -0.22%] | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | +0.30% | [+0.09%, +0.52%] | false | false |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +0.16% | [-0.04%, +0.36%] | false | true |

Decision: do **not** proceed to Stage E direct-output switching yet. The Stage D direct-row observer is SMP-neutral and useful, but it is not a semantically complete output path. It still observes around `metrics.Serie` rather than replacing materialization, and it must account for post-sink enrichment (for example host tags) and exact v3 wire compatibility before a comparison image should send new-path payloads.

Updated artifacts:

- `summary.csv`
- `selected_metrics.sql`
- `selected_metrics.csv`
- `README.md`
- `report.html`
- `comp/dogstatsd/SMP_EXPERIMENT_PLAN.md`

## Stage E direct active serializer experiment

Commit `9498d5fee95` adds an intentionally unsafe local-only direct serializer path. It is enabled with:

```text
DD_DOGSTATSD_EXPERIMENTAL_DIRECT_SERIALIZER=true
```

When enabled, `AgentDemultiplexer.flushToSerializer` bypasses `IterableSeries` / `IterableSketches` channel traversal and invokes `Serializer.SendDirectSeriesAndSketches`, which writes producer rows directly into the existing v2/v3 serializer pipeline builders. This is not production-ready: it supports v2/v3 protobuf series only and still consumes existing `metrics.Serie` / `metrics.SketchSeries` rows rather than eliminating aggregator materialization.

Verification before image build:

```bash
dda inv test --targets=./pkg/serializer/internal/metrics --timeout=300
dda inv test --targets=./pkg/aggregator,./pkg/serializer/internal/metrics --timeout=300
```

Result: both passed; combined run reported 246/246 tests passed.

Built optimized Linux/arm64 image:

```text
image: datadog/agent-dev:smp-dsd-direct-active
commit: 9498d5fee95
image id: sha256:4ea5dddf91170749e4b1a804698539f5994ed92ea6fb5fe24dd58d575adab606
agent version: Agent 7.81.0-devel - Meta: git.37.9498d5f - Commit: 9498d5fee95 - Serialization version: v5.0.196 - Go version: go1.25.10
```

Created local-only case copies under `local-experiment-direct-active/` with `DD_DOGSTATSD_EXPERIMENTAL_DIRECT_SERIALIZER=true`. Smoke test passed on `uds_dogstatsd_to_api_v3_endpoint_fixed`; capture telemetry included `serializer.series_pipeline_duration_ns{phase:direct_series}` and `{phase:direct_sketches}`.

Stage E SMP compared Stage D direct-row shadow (`datadog/agent-dev:smp-dsd-direct-row`) against direct-active (`datadog/agent-dev:smp-dsd-direct-active`) using the local direct-active cases.

| Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---|---:|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3` | ingress throughput | -0.21% | [-0.43%, +0.01%] | 77.0% | false | false |
| `uds_dogstatsd_to_api` | ingress throughput | -0.07% | [-0.39%, +0.24%] | 23.6% | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | -0.94% | [-1.15%, -0.72%] | 100.0% | false | true |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +0.56% | [+0.27%, +0.84%] | 98.7% | false | true |

Stage E decision: active direct serialization is locally non-regressing and improves the corrected current-v3 endpoint plus memory case. It does not prove a broad throughput win for v2 or the source v3-labeled case. The next experiment should remove more aggregator materialization or make the direct output row representation feed the v3 builder without going through `metrics.Serie` mutation.

## Stage F direct series row experiment

Commit `c7327ad816c` adds the next active pipeline step behind a separate local-only env gate:

```text
DD_DOGSTATSD_EXPERIMENTAL_DIRECT_SERIALIZER=true
DD_DOGSTATSD_EXPERIMENTAL_DIRECT_ROWS=true
```

When both are enabled, DogStatsD `TimeSampler.flushSeries` detects a `metrics.SerieRowSink` and emits normalized `metrics.SerieRow` values directly instead of appending fully populated `*metrics.Serie` objects for the DogStatsD time-sampler series path. The row type carries serializer-visible identity/tags/resources without requiring the serializer to mutate shared `*Serie` values for device/resource projection. Check-sampler series and sketches still use the existing `*metrics.Serie` / `*metrics.SketchSeries` paths.

Verification before image build:

```bash
dda inv test --targets=./pkg/metrics,./pkg/serializer/internal/metrics,./pkg/aggregator --timeout=300
```

Result: passed; unified test report showed 320/320 tests passed.

Built optimized Linux/arm64 image:

```text
image: datadog/agent-dev:smp-dsd-direct-rows
commit: c7327ad816c
image id: sha256:d3e3444dc310dab44cf4d628b6b7d514da62f018a82cf9f35e6b49e8c5eac095
agent version: Agent 7.81.0-devel - Meta: git.39.c7327ad - Commit: c7327ad816c - Serialization version: v5.0.196 - Go version: go1.25.10
```

First build attempt failed because the Linux dev-env container saw the mounted repository as a dubious git directory. Rerun succeeded after adding:

```bash
git config --global --add safe.directory /Users/luke.steensen/code/datadog-agent
```

Created local-only case copies under `local-experiment-direct-rows/` by extending the Stage E direct-active cases with `DD_DOGSTATSD_EXPERIMENTAL_DIRECT_ROWS=true`.

Smoke test passed on `uds_dogstatsd_to_api_v3_endpoint_fixed`. Capture telemetry confirmed the row path:

- `dogstatsd_direct_row_shadow.duration_ns{phase:series_rows}`
- `serializer.series_pipeline_duration_ns{phase:direct_series_rows}`
- `serializer.segment_shadow_duration_ns{phase:direct_series_rows}`
- `dogstatsd_pipeline.items{kind:series_rows}`

Stage F compared Stage E direct-active (`datadog/agent-dev:smp-dsd-direct-active`, `9498d5fee95`) against direct-rows (`datadog/agent-dev:smp-dsd-direct-rows`, `c7327ad816c`) using the local direct-rows cases.

| Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---|---:|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3` | ingress throughput | +0.22% | [-0.03%, +0.47%] | 75.0% | false | true |
| `uds_dogstatsd_to_api` | ingress throughput | -0.13% | [-0.36%, +0.10%] | 54.0% | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | -0.21% | [-0.44%, +0.01%] | 77.6% | false | true |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | -0.12% | [-0.37%, +0.13%] | 45.7% | false | false |

The first `uds_dogstatsd_to_api_v3_endpoint_fixed` run timed out at the pi tool boundary while replicate 3 was still running; its partial log is retained as `cases/stageF-direct-active-vs-direct-rows-uds_dogstatsd_to_api_v3_endpoint_fixed.log` and excluded. Containers were stopped, then the case was rerun successfully as `cases/stageF-direct-active-vs-direct-rows-uds_dogstatsd_to_api_v3_endpoint_fixed-rerun1.log`.

Stage F decision: the active DogStatsD series row path is locally non-regressing and essentially neutral. It proves we can move the time-sampler series handoff to a serializer-visible row model without losing SMP performance, but it does not yet produce a large speedup. The remaining materialization cost is inside metric flush/dedup itself and sketches/check-sampler rows are still on old structs.

## Stage G direct metric row flush experiment

Commit `638b79c3bba` adds `metrics.SerieRowFragment` and a row-oriented `ContextMetricsFlusher` path behind:

```text
DD_DOGSTATSD_EXPERIMENTAL_DIRECT_SERIALIZER=true
DD_DOGSTATSD_EXPERIMENTAL_DIRECT_ROWS=true
DD_DOGSTATSD_EXPERIMENTAL_DIRECT_METRIC_ROWS=true
```

The experiment bypasses `*metrics.Serie` allocation during `Metric.flush` for the common scalar metric types (`Gauge`, `Count`, `Counter`, `Set`, `MonotonicCount`, `Rate`, and `MetricWithTimestamp`) and feeds lightweight row fragments into the existing Stage F direct series-row handoff. Histogram/historate still fall back through existing `*Serie` materialization.

Verification:

```bash
dda inv test --targets=./pkg/metrics,./pkg/aggregator,./pkg/serializer/internal/metrics --timeout=300
```

Result: passed; unified report showed 322/322 tests passed before the Stage G commit, and 289/289 for metrics+aggregator after the Stage H follow-up.

Built optimized Linux/arm64 image:

```text
image: datadog/agent-dev:smp-dsd-direct-metric-rows
commit: 638b79c3bba
image id: sha256:3d426f8396fee5839932767a6ba4d3299a37491adc1d58f53224be89fe6014b6
agent version: Agent 7.81.0-devel - Meta: git.42.638b79c - Commit: 638b79c3bba - Serialization version: v5.0.196 - Go version: go1.25.10
```

Smoke test passed and confirmed `dogstatsd_direct_row_shadow.duration_ns{phase:metric_rows}`.

Stage G compared direct series rows (`datadog/agent-dev:smp-dsd-direct-rows`, `c7327ad816c`) against direct metric rows (`datadog/agent-dev:smp-dsd-direct-metric-rows`, `638b79c3bba`).

| Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---|---:|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3` | ingress throughput | -0.08% | [-0.35%, +0.19%] | 29.3% | false | false |
| `uds_dogstatsd_to_api` | ingress throughput | -0.00% | [-0.24%, +0.24%] | 1.3% | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | -0.52% | [-0.76%, -0.28%] | 99.4% | false | true |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +0.02% | [-0.23%, +0.26%] | 6.8% | false | true |

A new high-rate metrics-only case was added to avoid the 100 MiB/s generator cap in the standard throughput cases:

```text
uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only
bytes_per_second: 250 MiB
kind_weights: metric=100,event=0,service_check=0
```

On that higher-pressure probe, Stage G was slightly worse:

| Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---|---:|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -0.25% | [-0.48%, -0.01%] | 82.2% | false | false |

Stage G decision: bypassing `*metrics.Serie` allocation inside scalar `Metric.flush` gives a small memory win in the memory case but no throughput win. In the uncapped/high-rate case it slightly hurts throughput. This suggests `*Serie` allocation itself is not the dominant value lever.

## Stage H unordered direct context rows upper-bound probe

Commit `c989043ddcc` adds an intentionally unsafe upper-bound path behind:

```text
DD_DOGSTATSD_EXPERIMENTAL_DIRECT_CONTEXT_ROWS=true
```

on top of the Stage G switches. It bypasses context grouping/dedup in `ContextMetricsFlusher` and flushes row fragments in timestamp/map iteration order. This can emit repeated rows for the same identity instead of merging points, so it is not wire-equivalent; it is a value probe for whether grouping/dedup removal is worth pursuing.

Built optimized Linux/arm64 image:

```text
image: datadog/agent-dev:smp-dsd-direct-context-rows
commit: c989043ddcc
image id: sha256:ba6a973a75a6114d1fcabcf0b89ce976684835836ba273e564bac7539db79a88
agent version: Agent 7.81.0-devel - Meta: git.43.c989043 - Commit: c989043ddcc - Serialization version: v5.0.196 - Go version: go1.25.10
```

Smoke test passed and confirmed `dogstatsd_direct_row_shadow.duration_ns{phase:context_rows}`.

Stage H compared direct metric rows (`638b79c3bba`) against unordered direct context rows (`c989043ddcc`) on the high-rate metrics-only probe:

| Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---|---:|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -0.51% | [-0.73%, -0.29%] | 99.7% | false | false |

Stage H decision: blindly removing grouping/dedup hurts, likely because extra rows increase downstream serializer/payload work more than the saved grouping work. This is strong negative evidence for a simple row-native rewrite being a large throughput win by itself.

Current value read: the architecture remains viable for semantics/debug/capture/replay and small memory wins, but local SMP evidence no longer supports selling the aggregator+serializer rewrite as a significant throughput/efficiency investment unless a different, more targeted bottleneck is identified by profiles or a more realistic production workload shows a much larger gap.

## 2026-05-18 Stage I: true-but-narrow columnar v3 vertical slice

Implemented and benchmarked a local-only DogStatsD columnar v3 vertical slice gated by `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3=true`.

Scope of the slice:

- v3 serializer path only.
- metrics only: on-time Gauge, Counter, Count, and Set.
- unsupported, timestamped, invalid, or `FlushFirstValue` samples fall back to the legacy path.
- normal DogStatsD parser/enrichment still runs.
- supported samples bypass `TimeSampler`, `ContextMetrics`, `metrics.Metric`, `metrics.Serie`, and iterable serializer traversal.
- direct v3 row serializer path builds payloads from `metrics.SerieRow` rows emitted by the columnar table.

Commits/images built during Stage I:

- `2ade84801cc` / `datadog/agent-dev:smp-dsd-columnar-v3` — naive parser direct insert; smoke succeeded but high-rate run was -4.74% and over-emitted rows before merge.
- `9b04b0c6104` / `datadog/agent-dev:smp-dsd-columnar-v3-merged` — merged columnar flush rows by identity; high-rate run was -5.28%.
- `0e774a353cb` / `datadog/agent-dev:smp-dsd-columnar-v3-descriptors` — reused descriptors across buckets; high-rate run was -6.68%.
- `6f3c5ae857a` / `datadog/agent-dev:smp-dsd-columnar-v3-lite-telemetry` — moved inserted-sample telemetry off the hot path; high-rate run was -6.02%.
- `7a43a9d0dae` / `datadog/agent-dev:smp-dsd-columnar-v3-batched` — handed accepted samples to shard-local columnar workers in batches; high-rate run was -8.02%.
- `87fbbc1fbb5` / `datadog/agent-dev:smp-dsd-columnar-v3-batched-nolock` — removed per-sample shard mutex from worker inserts; high-rate run was -8.61%.
- `e68c3c36110` / `datadog/agent-dev:smp-dsd-columnar-v3-bucket-cache` — cached descriptor-local current-bucket rows; high-rate run was -7.11%.

Official Stage I SMP comparisons used:

```bash
smp local run \
  --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-columnar-v3 \
  --case uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only \
  --baseline-image datadog/agent-dev:smp-dsd-direct-metric-rows \
  --comparison-image <stageI-image> \
  --replicates 3 \
  --total-samples 270
```

Key telemetry from Stage I full runs:

- `target/aggregator.dogstatsd_contexts{shard=0}` stayed effectively zero on the comparison side, confirming supported DogStatsD samples bypassed the old time-sampler context state.
- `target/dogstatsd_columnar_v3.stats{stat=inserted_samples}` was >200k/s on the comparison side.
- `target/dogstatsd_columnar_v3.stats{stat=flushed_rows}` was about 149/s, and direct v3 row serializer telemetry was present.
- DogStatsD packet pool/channel backlog and RSS were much higher on columnar variants, indicating ingest/worker backpressure rather than serializer flush time was the dominant failure mode.

Stage I verdict: negative for the performance hypothesis. The full theoretical design had not been measured by Stage A-H; Stage I measured a much closer v3-only metric-only vertical slice, and it still did not beat the direct-metric-row baseline. This does not invalidate debug/capture/lookback/replay value, but it argues strongly against pitching the current columnar replacement as a near-term throughput win.

## 2026-05-18 Stage J: root cause of the Stage I negative result

Stage J tested the hypothesis that the Stage I columnar path had not actually
collapsed identity work: it forced `identity.Builder.ResolveHotPath` for every
sample, even when debug stats were disabled and `dogstatsd_pipeline_count=1`.
That meant the columnar parser path computed:

- a debug-view key with host intentionally omitted,
- a debug display tag string via `strings.Join`, and
- a shard/backend key with host included.

The Stage G direct-metric-row baseline in the same one-pipeline case did not do
that parser-side identity work; it resolved backend context later in the
TimeSampler worker. This made the Stage I comparison unfair to the actual
unified-model idea: we were paying for an unused projection, not just a shared
semantic identity.

Implemented Stage J shard-only identity optimization:

- commit `dfdb011f0d7` / image `datadog/agent-dev:smp-dsd-columnar-v3-shard-only`
- when debug is off, compute only `identity.Builder.Shard` for the batcher or
  columnar path instead of `ResolveHotPath`.
- follow-up commit `44423157b07` generalizes this so sharded batchers can also
  avoid the unused debug projection while preserving `HotPathContext.Client` for
  tests/contracts.

SMP results:

| Comparison | Case | Δ mean | Δ mean CI | Confidence | Notes |
|---|---|---:|---:|---:|---|
| direct metric rows → columnar shard-only | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +13.57% | [+13.28%, +13.86%] | 100.0% | columnar now beats Stage G baseline |
| columnar bucket-cache → columnar shard-only | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +24.07% | [+23.57%, +24.57%] | 100.0% | isolates unused debug identity as the major Stage I bottleneck |
| direct metric rows → columnar shard-only | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +2.83% | [+2.44%, +3.22%] | 100.0% | standard corrected v3 case also improves |
| columnar shard-only → skip legacy flush | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | -0.22% | [-0.57%, +0.14%] | 56.7% | empty legacy fallback flush is not the limiter |

Interpretation:

- The theoretical case is not fundamentally refuted. Stage I's negative result
  came from an implementation shortcut/mistake: computing all hot-path
  projections when only the shard/backend projection was needed.
- The unified model can win when it truly removes old `TimeSampler` backend
  resolution and avoids introducing unrelated view work.
- The current implementation is still not ideal. It still emits via
  `metrics.SerieRow`, not a native columnar protobuf builder, and it still has
  high memory/backlog costs.
- On `uds_dogstatsd_to_api_v3_endpoint_fixed`, Stage J comparison improved
  throughput by +2.83% and reduced captured agent CPU by ~4%, but increased RSS
  by ~30% and heap by ~39%.
- On the 250 MiB/s overload probe, Stage J improved throughput by +13.57%, but
  packet pool/channel backlog stayed high because the target still cannot fully
  sustain the requested generator rate.

Conclusion after Stage J: continue, but pivot the next experiments from
"is there throughput potential?" to "can we keep the throughput gain while
controlling memory/backlog and broadening compatibility?" The next high-value
experiments are descriptor/bucket memory profiling, descriptor expiry/reuse,
native columnar-to-v3 protobuf emission, and a compatibility matrix beyond the
metric-only v3 slice.

## Stage K — backpressure/bottleneck-shift probes

Question: why does the faster Stage J columnar comparison fill the Agent
`packetsIn` backlog while the slower direct-metric-row baseline does not?
Hypothesis: columnar reduces per-sample work enough to admit/process more UDS
traffic; at high offered rates, overload moves from upstream socket/lading
backpressure into the Agent's existing packet queue.

Artifacts:

- detailed note: `notes/stageK-backpressure-shift.md`
- rate sweep summary: `backpressure_rate_sweep.csv`
- CPU sweep summary: `backpressure_cpu_sweep.csv`
- captures: `captures/stageK-rate-sweep/`, `captures/stageK-cpu-sweep/`
- experiments: `local-experiment-backpressure-rate-sweep/`,
  `local-experiment-backpressure-cpu-sweep/`

Rate sweep summary, direct-metric-rows baseline vs columnar shard-only:

| Offered | Baseline Agent UDS | Comparison Agent UDS | Baseline queue batches | Comparison queue batches | Interpretation |
|---:|---:|---:|---:|---:|---|
| 180 MiB/s | 174.75 MiB/s | 176.00 MiB/s | 0.9 | 0.6 | both keep up; no material queue |
| 200 MiB/s | 183.04 MiB/s | 195.04 MiB/s | 14.7 | 41.4 | comparison admits/processes more with modest queue |
| 220 MiB/s | 193.96 MiB/s | 215.03 MiB/s | 25.9 | 55.2 | comparison still modest queue, baseline already upstream-limited |
| 240 MiB/s | 185.68 MiB/s | 215.41 MiB/s | 15.5 | 812.5 | comparison reaches internal queue bottleneck |
| 250 MiB/s | 196.93 MiB/s | 224.19 MiB/s | 29.9 | 871.4 | previous 3-rep Stage J point; comparison near full queue |

CPU allotment probe at 240 MiB/s:

| Target CPU | Baseline Agent UDS | Comparison Agent UDS | Baseline queue batches | Comparison queue batches | Interpretation |
|---:|---:|---:|---:|---:|---|
| 2 | 184.12 MiB/s | 195.31 MiB/s | 0.6 | 0.5 | both backpressure before `packetsIn`; comparison still +6% |
| 3 | 188.17 MiB/s | 229.09 MiB/s | 5.7 | 789.5 | comparison has enough headroom to shift bottleneck to Agent queue |
| 4 | 185.68 MiB/s | 215.41 MiB/s | 15.5 | 812.5 | same qualitative shift; single-rep values are noisy |

Conclusion: the backlog is consistent with a real bottleneck shift, not with the
baseline simply dropping while the comparison only buffers. The current large
DogStatsD packet queue mostly stays empty in the baseline because another
bottleneck applies backpressure before packets enter the measured Go queue. Once
columnar reduces per-sample work enough to admit more traffic, the existing
queue becomes the overload absorber. This is a useful milestone finding and a
productionization guardrail: do not accept the high-rate throughput win unless
queue occupancy, latency, and memory remain bounded under overload.

## Stage L — byte-bounded ingress log handoff

Stage L extends the database/log design one level earlier than Stage J by taking
over the large `packetsIn` queue. The first prototype is behind
`DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG=true` and uses a byte-bounded in-memory
FIFO of packet batches with a tiny listener-to-log handoff channel. The measured
configuration used `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_MAX_BYTES=16777216`.

Code commit:

- `eae5cb364ee dogstatsd: add experimental ingress log handoff`
- `7630c423017 dogstatsd: avoid unblocked ingress log telemetry`

The SMP numbers below were gathered before `7630c423017`, so they are conservative for the ingress-log path.

Verification:

- `dda inv test --targets=./comp/dogstatsd/server/impl --timeout=300`

SMP results are single-replicate local probes:

| Comparison | Case | Δ mean | Δ mean CI | Notes |
|---|---|---:|---:|---|
| columnar env-off → columnar ingress-log | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | -7.79% | [-8.42%, -7.16%] | explicit 16MiB backpressure vs large implicit channel |
| direct metric rows → columnar ingress-log | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +4.90% | [+4.37%, +5.44%] | high-rate win remains with controlled memory |
| direct metric rows → columnar ingress-log | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +1.22% | [+0.53%, +1.92%] | standard case stays neutral/slightly positive |

Most important selected metrics from the high-rate direct-row comparison:

- direct metric rows: `193.23 MiB/s` Agent UDS, `238,830/s` processed, `247.12 MiB` RSS, `63.16 MiB` heap.
- columnar ingress-log: `202.36 MiB/s` Agent UDS, `250,077/s` processed, `8.3 MiB` ingress-log occupancy, `228.91 MiB` RSS, `58.09 MiB` heap.

Conclusion: the first ingress-log prototype gives up the unconstrained Stage J
overload-admission peak, but that is the point of the experiment. With explicit
bounded backpressure, the columnar path still beats direct metric rows and no
longer pays the huge packet/channel backlog memory cost. This is a strong signal
that the raw ingress log/ring is the right long-term replacement for the live
packet queue, but the prototype still needs a true preallocated byte ring and
parser cursors to remove the extra pump/channel hop.

## 2026-05-18/19 Stage M implementation and SMP

Implemented M1 sharded packet-batch ingress log:

- Commit `057442d4163 dogstatsd: add sharded ingress log handoff`
- Follow-up telemetry commits `e61dba295aa` and `d4ed40c5185`
- Gate: `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_SHARDED=true`
- Listener packet-buffer flushes write directly into per-worker ingress-log shards; workers drain their shard directly.

Implemented M2 raw UDS datagram ingress ring:

- Commit `052f776aef1 dogstatsd: add raw UDS ingress ring`
- Follow-up telemetry commit `53a887497f6 dogstatsd: share raw ingress ring collectors`
- Gate: `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS=true`
- UDS datagram listener reserves fixed preallocated ring slots, reads directly into ring storage, commits record metadata, and workers parse/release raw records.

Verification:

```bash
dda inv test --targets=./comp/dogstatsd/packets,./comp/dogstatsd/listeners,./comp/dogstatsd/server/impl --timeout=300
```

Final rerun result: `DONE 324 tests in 7.074s`, `302 total`, `302 passed`.

Built Stage M optimized linux/arm64 image from commit `53a887497f6` in `datadog/agent-dev-env-linux:latest` after marking the mounted repo as a safe Git directory and running:

```bash
dda inv agent.build --build-exclude=systemd --no-development
patchelf --set-rpath /opt/datadog-agent/embedded/lib bin/agent/agent
```

Image tags:

- `datadog/agent-dev:smp-dsd-columnar-v3-sharded-log`
- `datadog/agent-dev:smp-dsd-columnar-v3-raw-uds-ring`

Both tags point at image `sha256:abfd3f2cb673cbede15f672e9738c2886d5b564511f16ceb3d87e2fe6c8023eb` created `2026-05-19T00:53:33.26418897Z`.

SMP caveat discovered: early raw-ring runs were invalid because UDP was still enabled, causing the raw-ring gate to disable itself. Fixed local raw cases with `dogstatsd_port: 0`; only post-fix raw logs were used in the Stage M summary.

Valid single-replicate SMP results:

| Comparison | Case | Δ mean | Δ mean CI | Confidence | Log |
|---|---|---:|---:|---:|---|
| Stage L ingress-log -> M1 sharded log | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | -3.77% | [-4.54%, -2.99%] | 100.0% | `cases/stageM-stageL-vs-sharded-log-uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only.log` |
| direct metric rows -> M1 sharded log | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | -0.09% | [-0.77%, +0.59%] | 13.5% | `cases/stageM-direct-metric-rows-vs-sharded-log-uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only.log` |
| direct metric rows -> M2 raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +0.80% | [+0.14%, +1.46%] | 87.9% | `cases/stageM-direct-metric-rows-vs-raw-uds-ring-uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only.log` |
| direct metric rows -> M2 raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +1.16% | [+0.77%, +1.55%] | 100.0% | `cases/stageM-direct-metric-rows-vs-raw-uds-ring-uds_dogstatsd_to_api_v3_endpoint_fixed.log` |

Generated `stageM_selected_metrics.csv` from copied parquet captures. Key read: M2 raw UDS ring removed DogStatsD packet-pool backlog entirely for the UDS datagram path (`packet_pool_avg=0`) while slightly beating direct metric rows and reducing RSS/heap in both high-rate and standard corrected-v3 runs.

## 2026-05-19 Stage N compact raw UDS ingress ring

Implemented the next ceiling probe after M2:

- Commit `4b2c4b6b7e6 dogstatsd: add compact raw UDS ingress ring`
- Gate: `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_COMPACT=true`
- Listener reads into a reusable scratch buffer, then commit copies exactly the bytes read into a preallocated per-worker compact byte ring plus metadata ring.
- Compact raw mode has the same eligibility constraints as M2: UDS datagram only; no UDP, stream socket, named pipe, statsd forwarding, or origin detection.

Verification:

```bash
dda inv test --targets=./comp/dogstatsd/packets,./comp/dogstatsd/listeners,./comp/dogstatsd/server/impl --timeout=300
```

Result: `DONE 328 tests in 6.073s`, `306 total`, `306 passed`.

Built optimized linux/arm64 Agent binary in `datadog/agent-dev-env-linux:latest` and created image `datadog/agent-dev:smp-dsd-columnar-v3-compact-raw-uds-ring` by replacing the Agent binary in the previous raw-ring image. Version check:

```text
Agent 7.81.0-devel - Meta: git.67.4b2c4b6 - Commit: 4b2c4b6b7e6 - Serialization version: v5.0.196 - Go version: go1.25.10
```

Valid single-replicate SMP results:

| Comparison | Case | Δ mean | Δ mean CI | Confidence | Log |
|---|---|---:|---:|---:|---|
| fixed-slot raw UDS ring -> compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +3.92% | [+3.19%, +4.65%] | 100.0% | `cases/stageN-raw-uds-ring-vs-compact-raw-uds-ring-uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only.log` |
| direct metric rows -> compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +7.25% | [+6.52%, +7.97%] | 100.0% | `cases/stageN-direct-metric-rows-vs-compact-raw-uds-ring-uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only.log` |
| direct metric rows -> compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +2.17% | [+1.61%, +2.72%] | 100.0% | `cases/stageN-direct-metric-rows-vs-compact-raw-uds-ring-uds_dogstatsd_to_api_v3_endpoint_fixed.log` |
| fixed-slot raw UDS ring -> compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +0.05% | [+0.01%, +0.09%] | 89.9% | `cases/stageN-raw-uds-ring-vs-compact-raw-uds-ring-uds_dogstatsd_to_api_v3_endpoint_fixed.log` |

Generated `stageN_selected_metrics.csv`. Key read: the compact ring is the best bounded-ingress high-rate result so far: +7.25% vs direct metric rows and +3.92% vs fixed-slot raw, with packet-pool backlog still zero. Standard corrected-v3 throughput also remains positive vs direct rows, but memory signals are mixed and need more repetitions/profiling.

## 2026-05-19 Stage O batched compact raw ingress drains

Implemented the next ceiling probe after Stage N:

- Commit `dc29e8fff7c dogstatsd: batch raw ingress ring drains`
- Gate: `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_BATCH_DRAIN=true`
- Batch size gate: `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_BATCH_DRAIN_SIZE=32`
- Workers use `TryNextBatch` / `ReleaseBatch` to process up to 32 raw-ring records per shard lock acquisition.
- Tested with Stage N compact raw UDS ingress ring enabled.

Verification:

```bash
dda inv test --targets=./comp/dogstatsd/packets,./comp/dogstatsd/listeners,./comp/dogstatsd/server/impl --timeout=300
```

Result: `DONE 330 tests in 7.376s`, `308 total`, `308 passed`.

Built optimized linux/arm64 Agent binary in `datadog/agent-dev-env-linux:latest` and created image `datadog/agent-dev:smp-dsd-columnar-v3-compact-raw-uds-ring-batch-drain` by replacing the Agent binary in the Stage N image. Version check:

```text
Agent 7.81.0-devel - Meta: git.69.dc29e8f - Commit: dc29e8fff7c - Serialization version: v5.0.196 - Go version: go1.25.10
```

Valid single-replicate SMP results:

| Comparison | Case | Δ mean | Δ mean CI | Confidence | Log |
|---|---|---:|---:|---:|---|
| compact raw UDS ring -> batch-drain compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +1.77% | [+1.23%, +2.31%] | 100.0% | `cases/stageO-compact-raw-uds-ring-vs-batch-drain-uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only.log` |
| direct metric rows -> batch-drain compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +7.39% | [+6.74%, +8.04%] | 100.0% | `cases/stageO-direct-metric-rows-vs-compact-raw-batch-drain-uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only.log` |
| compact raw UDS ring -> batch-drain compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +0.18% | [+0.01%, +0.35%] | 83.4% | `cases/stageO-compact-raw-uds-ring-vs-batch-drain-uds_dogstatsd_to_api_v3_endpoint_fixed.log` |
| direct metric rows -> batch-drain compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +1.33% | [+1.00%, +1.66%] | 100.0% | `cases/stageO-direct-metric-rows-vs-compact-raw-batch-drain-uds_dogstatsd_to_api_v3_endpoint_fixed.log` |

Generated `stageO_selected_metrics.csv`. Key read: batched worker drains add a real but smaller high-rate step (+1.77% over Stage N compact). The best high-rate run reached 211.66 MiB/s Agent UDS bytes and 261,559 processed metrics/s with packet-pool backlog still zero. The next ceiling should remove the listener scratch-to-ring copy with a no-copy direct reservation or size-class/slabbed ring.

## 2026-05-19 Stage P direct compact raw ingress ring

Implemented and tested a simple no-copy direct-reservation ceiling probe:

- Commit `83d06509167 dogstatsd: add direct compact raw UDS ring`
- Gate: `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_DIRECT_COMPACT=true`
- The listener reserves a max-size contiguous span in the compact raw ring before `ReadFromUnix`, reads directly into ring-owned storage, then commit publishes the actual byte length and reclaims the unused tail.
- Stage P keeps Stage O worker-side batch drains enabled with `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_BATCH_DRAIN=true` and size `32`.

Verification:

```bash
dda inv test --targets=./comp/dogstatsd/packets,./comp/dogstatsd/listeners,./comp/dogstatsd/server/impl --timeout=300
```

Result: `DONE 334 tests in 5.973s`, `312 total`, `312 passed`.

Built optimized linux/arm64 Agent binary in `datadog/agent-dev-env-linux:latest` and created image `datadog/agent-dev:smp-dsd-columnar-v3-direct-compact-raw-uds-ring` by replacing the Agent binary in the Stage O image. Version check:

```text
Agent 7.81.0-devel - Meta: git.71.83d0650 - Commit: 83d06509167 - Serialization version: v5.0.196 - Go version: go1.25.10
```

Valid single-replicate SMP results:

| Comparison | Case | Δ mean | Δ mean CI | Confidence | Log |
|---|---|---:|---:|---:|---|
| compact batch-drain raw UDS ring -> direct-compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | -3.30% | [-3.90%, -2.69%] | 100.0% | `cases/stageP-compact-batch-drain-vs-direct-compact-uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only.log` |
| direct metric rows -> direct-compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +4.46% | [+3.77%, +5.15%] | 100.0% | `cases/stageP-direct-metric-rows-vs-direct-compact-uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only.log` |
| compact batch-drain raw UDS ring -> direct-compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | -0.03% | [-0.11%, +0.06%] | 33.6% | `cases/stageP-compact-batch-drain-vs-direct-compact-uds_dogstatsd_to_api_v3_endpoint_fixed.log` |
| direct metric rows -> direct-compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +1.78% | [+1.03%, +2.52%] | 99.7% | `cases/stageP-direct-metric-rows-vs-direct-compact-uds_dogstatsd_to_api_v3_endpoint_fixed.log` |

Generated `stageP_selected_metrics.csv`. Key read: the simple no-copy direct-reservation variant is worse than Stage O at high rate. It removes the copy, but requiring a max-size contiguous reservation before every socket read appears to dominate. Do not treat this as the next throughput path; Stage N/O remains the best bounded-ingress design tested so far. Next candidates: true size-class/slabbed storage or listener/syscall batching, plus oldest-age/lag telemetry.

## 2026-05-19 — Stage Q native columnar-v3 serializer

- Implemented native columnar-to-v3 payload row path behind `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_NATIVE_SERIALIZER=true`.
- Added `V3MetricPointRow`, serializer direct sink support, native v3 row writer, and demux native flush path.
- Fixed initial one-row-per-bucket payload cardinality by merging points per descriptor across buckets.
- Added a reasonable v2/direct-series bridge behind `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_DIRECT_SERIES_SERIALIZER=true`.
- Verification:
  - `dda inv test --targets=./pkg/metrics,./pkg/serializer/internal/metrics,./pkg/aggregator --timeout=300`
  - relevant suites passed locally (`397 passed` before v2/direct-series integration; `222 passed` for the follow-up aggregator run).
- Final image:
  - `datadog/agent-dev:smp-dsd-columnar-v3-native-columnar-v3`
  - `sha256:b6b4c8aeaa277952d6f13608ef1654fd2d4ba5e23c924c8676a693fba87e0e4b`
  - `Agent 7.81.0-devel - Meta: git.73.947dc3f - Commit: 947dc3f2ec3`
- Stage Q vs Stage O compact/batch:
  - high-rate fixed-v3: `+0.80%`, CI `[-0.10%, +1.71%]`, confidence `74.7%`.
  - standard fixed-v3: `-0.65%`, CI `[-1.29%, -0.02%]`, confidence `81.1%`.
  - Payload-size telemetry remained equivalent.
- Final main honesty probes:
  - v3 high-rate: `+8.10%`, CI `[+7.47%, +8.73%]`, confidence `100.0%`.
  - v3 standard: `+1.67%`, CI `[+1.05%, +2.28%]`, confidence `99.9%`.
  - v2 high-rate direct-series bridge: `+9.77%`, CI `[+8.76%, +10.78%]`, confidence `100.0%`.
  - v2 standard direct-series bridge: `+3.27%`, CI `[+2.50%, +4.03%]`, confidence `100.0%`.
  - origin-on v3 standard: `+2.68%`, CI `[+1.69%, +3.66%]`, confidence `99.9%`; raw ingress disabled itself, so this is not raw-ring origin support.
- Decision: native v3 serialization completes the architecture and preserves payload size, but it is not a large standalone Stage O improvement. Freeze design exploration; move to broad honesty gates, feature-cost measurements, memory profiling, and raw-ring lag/oldest-age telemetry.

## 2026-05-19 — raw-ring lag/backpressure telemetry

- Commit: `b99255eb31e` (`dogstatsd: add raw ingress lag telemetry`).
- Added `dogstatsd_ingress_ring.consumer_lag_records{shard}` and `consumer_lag_bytes{shard}`.
- Added `dogstatsd_ingress_ring.oldest_record_timestamp_ns{shard}` and `oldest_record_age_ns{shard}`.
- Added backpressure counters via `dogstatsd_ingress_ring.stats{stat:blocked_reservations}`, `blocked_appends`, and `backpressure_events`.
- Expanded `blocked_ns{shard}` meaning to cover both reservation and compact append blocking.
- Verification: `dda inv test --targets=./comp/dogstatsd/packets,./comp/dogstatsd/listeners,./comp/dogstatsd/server/impl --timeout=300` → `315 total`, all passed.
- Next SMP honesty gates should reject wins where consumer lag or oldest age grows without bound.

## 2026-05-19 — initial 3-replicate post-telemetry honesty gates

- Image: `datadog/agent-dev:smp-dsd-columnar-v3-raw-lag-telemetry`, `sha256:d6432a0a29cb88aa1a5918476debb1c0a722515a8d65ce597230563b1b9292bb`, commit `b99255eb31e`.
- `main` -> Stage Q + raw lag telemetry, high-rate fixed-v3: `+3.62%`, CI `[+3.35%, +3.90%]`, confidence `100.0%`.
- `main` -> Stage Q + raw lag telemetry, standard fixed-v3: `+2.25%`, CI `[+1.94%, +2.55%]`, confidence `100.0%`.
- High-rate raw ring means: `9.16 MiB` consumer lag avg, `15.87 MiB` max, oldest age `89.7 ms` avg / `196.3 ms` max, blocked `406.1 ms/s`.
- Standard raw ring means: `0.39 MiB` consumer lag avg, `7.69 MiB` max, oldest age `4.0 ms` avg / `276.2 ms` max, blocked `152.3 ms/s`.
- Interpretation: high-rate remains positive but is clearly bounded by explicit raw-ring backpressure; standard remains positive but memory is still materially higher.

## 2026-05-19 — remaining 3-replicate post-telemetry honesty matrix

Ran the remaining local matrix against `datadog/agent-dev:smp-dsd-main` with the raw-lag telemetry image:

| Comparison | Δ mean | Δ mean CI | Confidence | Read |
|---|---:|---:|---:|---|
| v2/direct-series high-rate UDS, origin off | +3.39% | [+3.08%, +3.70%] | 100.0% | positive but bounded by raw-ring backpressure |
| v2/direct-series standard UDS, origin off | +1.51% | [+1.22%, +1.80%] | 100.0% | positive; memory higher |
| v3 high-rate UDS, origin on | +2.32% | [+2.03%, +2.61%] | 100.0% | raw ingress disabled itself; columnar/native remains positive |
| v3 standard UDS, origin on | +1.40% | [+1.03%, +1.77%] | 100.0% | raw ingress disabled itself; columnar/native remains positive |
| v3 high-rate UDP, raw disabled | -0.06% | [-0.28%, +0.17%] | 25.4% | neutral; raw ingress disabled itself because UDP is enabled |
| v3 standard UDP, raw disabled | +0.06% | [-0.38%, +0.51%] | 14.3% | neutral; raw ingress disabled itself because UDP is enabled |
| v3 standard UDS, mixed metric types | -0.01% | [-0.07%, +0.06%] | 10.6% | neutral; unsupported metric types fall back |

Generated:

- `honesty3_matrix_effects.csv`
- `honesty3_matrix_selected_metrics.csv`
- `notes/raw-ring-honesty-matrix.md`

Interpretation:

- UDS origin-off v2/v3 remains positive, but high-rate cases are bounded-backpressure runs.
- Origin-on and UDP cases are raw-disabled fallback checks, not raw-ring origin/OOB or UDP support.
- UDP local cases overdrive the listener and drop heavily, so they are compatibility checks only.
- Mixed metric types are neutral with about 35k/s columnar `metric_type` fallbacks; this validates fallback compatibility, not native support for timers/distributions/histograms.
- Standard UDS memory remains higher and is still the next proof blocker.

## Stage R memory hygiene and telemetry cost controls

Implemented Stage R after the raw-lag honesty matrix showed standard UDS memory remained higher than main.

Code commits:

- `f05e7fa378f` — `dogstatsd: reduce columnar memory overhead`
- `cf85e2601ce` — `dogstatsd: make columnar descriptor interning optional`
- `ab6db258799` — `dogstatsd: reuse columnar flush buffers`

Validation commands:

```bash
dda inv test --targets=./pkg/aggregator,./pkg/serializer/internal/metrics --timeout=300
dda inv test --targets=./comp/dogstatsd/server/impl --timeout=300
```

Both passed.

Built optimized Linux/arm64 images:

- `datadog/agent-dev:smp-dsd-columnar-v3-memory-hygiene` — first pass, descriptor interning always on, image ID `sha256:52eb5568e09486a15dbf61b011060b91d92c285f5fdd7f176034f5be8ee10c67`.
- `datadog/agent-dev:smp-dsd-columnar-v3-memory-hygiene-defaults` — descriptor interning default-off, image ID `sha256:4d93e46ed0501b9a38c3a46f7e72f33a1622437710925ecc8a374695bf4ad764`.
- `datadog/agent-dev:smp-dsd-columnar-v3-memory-hygiene-reuse` — final Stage R with buffer reuse, image ID `sha256:f0594de5eb3e9db47bfba9ef41ff3c999dc6cb58be2a396632de8237561ad394`.

SMP runs completed with `--replicates 3 --total-samples 270`:

```bash
smp local run --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-stageQ-native-serializer --case uds_dogstatsd_to_api_v3_endpoint_fixed_compact_batch --baseline-image datadog/agent-dev:smp-dsd-columnar-v3-raw-lag-telemetry --comparison-image datadog/agent-dev:smp-dsd-columnar-v3-memory-hygiene
smp local run --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-stageQ-native-serializer --case uds_dogstatsd_to_api_v3_endpoint_fixed_compact_batch --baseline-image datadog/agent-dev:smp-dsd-columnar-v3-raw-lag-telemetry --comparison-image datadog/agent-dev:smp-dsd-columnar-v3-memory-hygiene-defaults
smp local run --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-stageQ-native-serializer --case uds_dogstatsd_to_api_v3_endpoint_fixed_compact_batch --baseline-image datadog/agent-dev:smp-dsd-columnar-v3-memory-hygiene-defaults --comparison-image datadog/agent-dev:smp-dsd-columnar-v3-memory-hygiene-reuse
smp local run --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-stageQ-native-serializer --case uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only_compact_batch --baseline-image datadog/agent-dev:smp-dsd-columnar-v3-raw-lag-telemetry --comparison-image datadog/agent-dev:smp-dsd-columnar-v3-memory-hygiene-reuse
smp local run --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-stageQ-native-serializer --case uds_dogstatsd_to_api_v3_endpoint_fixed_compact_batch --baseline-image datadog/agent-dev:smp-dsd-main --comparison-image datadog/agent-dev:smp-dsd-columnar-v3-memory-hygiene-reuse
```

Results are summarized in:

- `stageR_memory_hygiene_effects.csv`
- `stageR_memory_hygiene_selected_metrics.csv`
- `notes/stageR-memory-hygiene.md`

Key read:

- Direct-row and segment-shadow proof telemetry are now optional. They were useful proof tools, but not needed for normal honesty runs.
- First-pass default-on descriptor/tagset interning was rejected: the local standard workload created about `7.68` tagsets/s and reused only about `0.16` tagsets/s, and RSS worsened.
- Final Stage R high-rate UDS improved `+1.50%` vs the raw-lag image, still with bounded raw-ring backpressure.
- Final Stage R standard UDS improved `+2.22%` vs main, but still used about `+45.6 MiB` RSS and `+21.5 MiB` heap alloc vs main. Memory remains unresolved; next step is heap profiling and direct serializer/payload-builder allocation analysis.

Confirmed no Docker containers left running after Stage R.

## Stage S heap profiling and v3 serializer allocation analysis

Collected Agent heap profiles for standard v3 UDS main vs Stage R reuse. The built-in SMP `--pprof-port 5000` sidecar still failed with `wget: can't connect to remote host: Connection refused` because the Agent pprof server binds to target-container localhost. Manual collection succeeded by running `alpine` with `--network container:<target>` and fetching `http://127.0.0.1:5000/debug/pprof/heap`.

Profile artifacts:

- `profiles/stageR-agent-heap/manual/baseline_mid.heap`
- `profiles/stageR-agent-heap/manual/baseline_late.heap`
- `profiles/stageR-agent-heap/manual/comparison_mid.heap`
- `profiles/stageR-agent-heap/manual/comparison_late.heap`
- pprof summaries in the same directory

SMP profiling run:

```bash
smp local run \
  --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-stageQ-native-serializer \
  --case uds_dogstatsd_to_api_v3_endpoint_fixed_compact_batch \
  --baseline-image datadog/agent-dev:smp-dsd-main \
  --comparison-image datadog/agent-dev:smp-dsd-columnar-v3-memory-hygiene-reuse \
  --replicates 1 \
  --total-samples 150 \
  --no-rm
```

Read:

- Late pprof in-use heap was `162.14 MiB` for main and `173.31 MiB` for Stage R (`+11.17 MiB`), smaller than the earlier `~+45.6 MiB` RSS gap.
- Stage R's compact raw ring retained `18.72 MiB`, while main's legacy packet pool retained `32.74 MiB`.
- Whole-Agent allocation was dominated by parser string interning/tag parsing, not the direct serializer (`stringInterner.LoadOrStore` around `17.64 GiB` alloc-space in the Stage R profile).
- Serializer-focused Stage R allocation over the 150-sample run was about `64 MiB` through columnar-v3 direct point-row flush, with visible low-risk row escape costs in the direct callback/sink path.

Implemented follow-up code changes:

- Pass `*metrics.V3MetricPointRow` through `V3MetricPointRowSink` to remove row-by-value escapes in the native direct serializer path.
- Add a direct common-case `payloadsBuilderV3.writeSerie` path to avoid allocating a temporary `SerieRow` for no-special-resource-tag series.
- Add `BenchmarkV3PayloadBuilderAllocation` to preserve the focused v3 payload-builder allocation profile.

Focused benchmark result after the fixes:

- `writeSerie`, reused identity: `~704 KiB`, `551 allocs/op` (down from `~2.41 MiB`, `8,743 allocs/op`).
- `writeSerie`, unique identity: `~5.25 MiB`, `929 allocs/op` (down from `~6.95 MiB`, `9,123 allocs/op`).
- `writeV3MetricPointRow`, unique identity: unchanged at `~5.25 MiB`, `929 allocs/op`; this is the current payload-builder floor.

Validation:

```bash
dda inv test --targets=./pkg/metrics,./pkg/aggregator,./pkg/serializer/internal/metrics,./pkg/serializer --timeout=300
```

Result: passed (`400` tests).

Confirmed no Stage S Docker containers left running.

## Stage T parser string/tagset interning

Implemented parser-side memory/allocation improvements after Stage S identified `stringInterner.LoadOrStore` and `parseTags` as the dominant allocation sites.

Profile context from Stage S standard v3 UDS comparison capture:

- `stringInterner.LoadOrStore`: about `17.64 GiB` alloc-space in Stage R profile.
- `parseTags` tag-slice allocation: about `2.40 GiB` alloc-space.
- Runtime telemetry maxima per worker: about `53M` misses, `60M` hits, and `12,936` full resets with a `4096`-entry interner.

Changes:

- Replaced full-reset parser string interner with bounded recent/protected SLRU-style maps and ring eviction.
- Added `dogstatsd.string_interner_evictions` telemetry; existing reset telemetry should no longer increase on the new path.
- Made `extractTagsMetadata` avoid mutating normal tagsets; it mutates only when removing metadata tags.
- Added opt-in exact raw-tagset cache:
  - `DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true`
  - `DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER_SIZE=<entries>`
  - second-sighting admission via hash doorkeeper;
  - special metadata tagsets are not admitted;
  - hot per-hit telemetry deliberately avoided after it showed allocation in microbenchmarks.

Focused benchmark:

```bash
dda inv test --targets=./comp/dogstatsd/server/impl \
  --test-run-name='^$' \
  --extra-args='-bench=BenchmarkParseTagsRepeatedTagset -benchmem -benchtime=3s' \
  --timeout=300
```

Best post-fix result:

- default repeated tagset parse: `145.9 ns/op`, `112 B/op`, `1 alloc/op`.
- exact tagset interner hit: `15.64 ns/op`, `0 B/op`, `0 alloc/op`.

Validation:

```bash
dda inv test --targets=./comp/dogstatsd/server/impl,./comp/dogstatsd/listeners,./comp/dogstatsd/packets --timeout=300
```

Result: passed (`318` tests).

Macro SMP has not yet been run for Stage T. Next run should build a post-Stage-T image, compare default Stage T against Stage R/S for SLRU-only impact, and run the exact tagset cache as an explicit feature-cost comparison before making it default.

## Stage T SMP validation

Built Stage T image from commit `d78e87470fd`:

- `datadog/agent-dev:smp-dsd-columnar-v3-parser-interning`
  - ID: `sha256:9bce78118472178a9d815f9bd7a8e50c80b46d4cf92f502969a2b29d66ab3012`
- `datadog/agent-dev:smp-dsd-columnar-v3-parser-interning-tagset`
  - ID: `sha256:99c37921c6bed2145e1a848499a8e83885f5ee5bfff23601da0f47890c00b93b`
  - same image committed with `DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true`

Build notes:

- Building directly on macOS produced Darwin `*.dylib` rtloader outputs; image build must run inside the Linux/arm64 dev container.
- Docker bind-mounting Colima's host socket path into the dev container failed; mounting `/var/run/docker.sock` from the Colima VM and setting `DOCKER_HOST=unix:///var/run/docker.sock` worked.
- The original Docker context included `.git`, `reports/`, and captures; an untracked local `.dockerignore` was used to keep the context small after an earlier `COPY .` failed with `no space left on device`.

Commands used the Stage O compact-batch experiment directory and `--replicates 3 --total-samples 270`.

Results:

| Comparison | Case | Δ mean | CI | Notes |
|---|---|---:|---:|---|
| Stage T default vs `main` | standard v3 UDS | `+2.98%` | `[+2.60%, +3.36%]` | improvement |
| Stage T default vs `main` | high-rate v3 UDS metrics-only | `+4.88%` | `[+4.52%, +5.23%]` | improvement; high-rate bounded-backpressure wording still applies |
| Stage T tagset cache vs Stage T default | standard v3 UDS | `+0.02%` | `[-0.03%, +0.08%]` | throughput neutral, lower RSS in this paired run |
| Stage T tagset cache vs Stage T default | high-rate v3 UDS metrics-only | `+21.17%` | `[+21.01%, +21.32%]` | large parser/backpressure relief; SMP sets `Regression=true` only because absolute delta exceeds ±20% threshold |
| Stage T tagset cache vs `main` | standard v3 UDS | `+1.60%` | `[+1.43%, +1.77%]` | improvement, lower paired RSS |
| Stage T tagset cache vs `main` | high-rate v3 UDS metrics-only | `+28.42%` | `[+28.21%, +28.64%]` | high-rate parser/backpressure relief; same threshold caveat |

Selected reads:

- `main` still hit tens of thousands of full string-interner resets per worker in these runs; Stage T reset counts stayed at zero and used individual `string_interner_evictions`.
- Stage T default improves throughput but did not solve standard RSS by itself: in one standard paired run, average RSS was `~491 MiB` for Stage T default vs `~439 MiB` for `main`.
- The tagset cache reduced per-worker string interner misses from tens/hundreds of millions to about `47k` in these repeated-tagset SMP workloads.
- Standard direct tagset vs `main` improved throughput by `+1.60%` and had lower average RSS (`~386.8 MiB` vs `~461.6 MiB`).
- High-rate tagset vs Stage T default improved throughput by `+21.17%` and reduced raw-ring pressure: max lag dropped from full (`~8.39 MiB`, `~2200` records) to about `~2.84 MiB`, `~602` records in the feature-cost run.

Conclusion: SLRU string interning is a useful default reset-churn fix. The exact raw-tagset cache is the stronger repeated-tagset optimization and should remain opt-in until mostly-unique/adversarial tagset feature-cost runs validate admission and retained-memory behavior.
