# Gensim EKS benchmark scenario

`aws/gensim-eks` is a manual workflow for generating Gensim episode data used
by Agent benchmark evaluations. It provisions a persistent EKS cluster and
runs an orchestrator Job that deploys the selected episodes and Agent image.

This scenario is intentionally not run by the E2E CI suite. Do not remove it
because it has no CI callers: benchmark maintainers submit runs manually when
they need new recordings or want to evaluate an Agent image.

## Prerequisites

- Complete the E2E framework setup, including access to the Agent sandbox AWS
  account.
- Have a local checkout of the `gensim-episodes` repository. Set
  `GENSIM_REPO_PATH` to its root when it is not a sibling of this repository.
- Build or otherwise make available a recorder-enabled Agent image (for example
  `docker.io/datadog/agent-dev:<tag>` from the `dev_branch-recorder-full` CI
  job). Standard release images do not include the
  `anomalydetection_recorder` build tag and are rejected in recording modes.

Each episode must have a Helm chart and the requested scenario YAML. Episodes
are selected as `episode:scenario` pairs; the task searches the `postmortems`
and `synthetics` directories in the Gensim checkout. Multiple scenarios from
the same episode directory may be submitted together in a single run.

## Submit and monitor a run

Submit one or more episodes with an Agent image:

```bash
dda inv aws.eks.gensim.submit \
  --image=docker.io/datadog/agent-dev:my-branch-recorder-full \
  --episodes=authcore-pgbouncer:pool-saturation \
  --s3-bucket=qbranch-gensim-recordings \
  --flush-interval=1m \
  --retention=90m
```

Alternatively, pass `--episode-manifest=path/to/manifest.json`. The manifest is
a JSON array of `episode` and `scenario` entries and may pin an episode `sha`
to make results comparable across runs.

The default mode, `record-parquet`, configures `anomaly_detection.recording.*`
(`DD_ANOMALY_DETECTION_RECORDING_*`) to write Parquet v1 files under
`/tmp/observer-metrics` for offline testbench replay. Pass `--recording-only`
when live detection should be disabled during recording. Use
`--mode=live-anomaly-detection` to send live anomaly events to Datadog without
Parquet recording, or `--mode=live-and-record` to enable both simultaneously.

The submit task generates a unique `run_id` (`eval-<timestamp>-<sha>-<rand>`)
and propagates it to the orchestrator Job and S3 artifact key
(`gensim-results-<run_id>-<episode>-<scenario>.zip`) so repeated same-day runs
and multiple scenarios from the same episode directory never overwrite each
other.

After each episode completes, the orchestrator waits one full `flush_interval`
past the episode `end_time`, verifies that all recorder Agent pods remained
Ready with zero restarts and unchanged topology, collects `/tmp/observer-metrics`
from every recorder Agent pod (`-c agent`), renames Parquet files with the pod
suffix (`observer-metrics-<ts>-<pod>.parquet`, `observer-logs-<ts>-<pod>.parquet`)
so testbench v1 metric and log discovery sees all pods in the flat `parquet/`
directory, and records file SHA-256 checksums and provenance in
`recording-manifest.json`.

For episodes containing `docker-compose.yaml`, the submit task builds and
pushes their images to ECR using its Amazon Linux ECS builder. Use
`--skip-build` only when the required images are already cached in ECR.

Monitor progress and inspect the individual episode states with:

```bash
dda inv aws.eks.gensim.status
```

`status` reports `monitor` (episode script outcome), `collection` (multi-pod
Parquet collection), `upload` (S3 archive upload), and `validation` separately.
Collection or upload failures mark the episode `failed` rather than `done`.

## Fetch and validate locally (two-stage workflow)

An uploaded S3 archive has `validation=pending` because full PyArrow validation
and `Time`-window trimming run on the developer machine before benchmark replay:

```bash
uv run --with pyarrow dda inv aws.eks.gensim.fetch \
  --run-id=<run_id> \
  --episode=authcore-pgbouncer \
  --scenario=pool-saturation \
  --s3-bucket=qbranch-gensim-recordings \
  --output-dir=/tmp/eks-scenarios/pool-saturation
```

`fetch` downloads the selected archive into a fresh local directory, verifies
archive safety and manifest SHA-256 checksums, validates multi-pod Agent
snapshots and Parquet `Time` overlap via `.agents/skills/anomalydetection-run-gensim-scenario-on-vm/scripts/recording.py`, and writes
`episode.json`, `validation.json`, `recording-manifest.json`, and `parquet/`.
Replay in the testbench only after `validation.json` reports `"usable": true`:

```bash
dda inv anomalydetection.launch-testbench --scenarios-dir /tmp/eks-scenarios --build
```

The submit task also prints a `kubectl logs -f` command for the orchestrator.
Use `--stack-name` and `--namespace` consistently on submit, status, and
cleanup commands when overriding their defaults.

## Cleanup

Keep the EKS cluster between runs to avoid reprovisioning it. To stop a run and
remove its workloads while retaining the cluster:

```bash
dda inv aws.eks.gensim.stop-all
```

Destroy the persistent cluster when it is no longer needed:

```bash
dda inv aws.eks.gensim.destroy
```

Both commands accept `--stack-name`; `stop-all` also accepts `--namespace`.
