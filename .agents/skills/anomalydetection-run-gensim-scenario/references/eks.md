# EKS backend architecture, troubleshooting, and live acceptance checklist

Read this reference only when running `--backend eks` with non-default settings, recovering from preflight/submit/fetch
errors, troubleshooting EKS orchestrator or collection failures, or preparing the user-run live AWS acceptance checklist.

## Multi-pod recording contract

On EKS (`aws/gensim-eks`), the Datadog Agent runs as a DaemonSet across Linux worker nodes:

- Each recorder Agent pod writes Parquet v1 files (`observer-metrics-*.parquet`, `observer-logs-*.parquet`) and optional
  sidecars (`advances.jsonl`, `detect_digests.jsonl`) into `/tmp/observer-metrics`.
- Before each episode, the orchestrator Job (`gensim-orchestrator`) snapshots all recorder Agent pods (`agent-before.json`),
  verifies every pod is Ready with `restartCount == 0`, and confirms `/tmp/observer-metrics` exists in the `agent` container.
- After `play-episode.sh` finishes, the orchestrator waits until one full `flush_interval` after the episode `end_time`,
  takes `agent-after.json`, verifies pod/node/container identities and zero restarts, and copies `/tmp/observer-metrics`
  from every expected recorder pod (`-c agent`).
- To keep all files in the flat `parquet/` directory (required because testbench v1 log discovery reads the immediate
  directory rather than walking subdirectories) without cross-pod filename collisions:
  - Metric and log Parquet files are renamed `<original-stem>-<sanitized-pod>.parquet`, preserving the
    `observer-metrics-*` / `observer-logs-*` prefixes and chronological flush-timestamp sort order.
  - Non-Parquet sidecars are renamed `<sanitized-pod>-<original-name>`. Context dictionaries (`contexts.parquet`) are not
    merged across pods.
  - `recording-manifest.json` records the original filename, source pod, byte size, and SHA-256 checksum for every file.

## Intentional Helm values differences (VM vs EKS)

`test/e2e-framework/scenarios/aws/gensim-eks/agent-values.yaml.tmpl` matches `assets/recorder-values.yaml` on all
`DD_ANOMALY_DETECTION_*` and `anomaly_detection.recording.*` settings, with intentional environment differences:

- `agents.image.pullPolicy: Always` on EKS (worker nodes pull from a container registry) vs `Never` on kind (loaded from
  a local Docker archive).
- `datadog.apiKeyExistingSecret` and `datadog.appKeyExistingSecret` reference `gensim-secrets` managed by Pulumi.
- `dogstatsd.useHostPort: true`, `apm.portEnabled: true`, `clusterAgent.admissionController.mutateUnlabelled: true`, and
  explicit container CPU/memory requests and limits for multi-node EKS workloads.

## Preflight, submit, and fetch recovery procedures

- **Dirty `gensim-episodes` checkout (`submit` refuses uncommitted changes)**: never reset or stash the user's working
  tree. With user approval, clone it locally (`git clone --local <repo> /tmp/gensim-episodes-clean`) and pass
  `GENSIM_REPO_PATH=/tmp/gensim-episodes-clean`.
- **Missing local SSH keypair on `dda inv e2e.setup`**: do not delete the remote AWS keypair; try `dda inv aws.eks.gensim.submit`
  directly (EKS does not require the EC2 SSH key) and report the keypair warning.
- **Interrupted Pulumi submit**: if a background `submit` was interrupted, verify no `pulumi` CLI process is running
  before re-submitting; idle `pulumi-resource-*` plugin processes alone do not indicate an active update.
- **Re-running validation on an already downloaded `$OUT`**: `dda inv aws.eks.gensim.fetch` automatically runs
  `uv run --no-project --with pyarrow python3` when `python3` lacks `pyarrow`. If `uv` was unavailable or validation failed
  after `artifact.zip` was downloaded to `$OUT`, install `pyarrow` in a local venv
  (`uv venv /tmp/pyarrow-venv && uv pip install --python /tmp/pyarrow-venv/bin/python pyarrow`) and re-run the validator
  against `$OUT` (omitting `flush_seconds` so it infers `flush_interval_seconds` from `recording-manifest.json`, or passing
  `"$FLUSH_SECONDS"` explicitly):
  `PATH=/tmp/pyarrow-venv/bin:$PATH python3 "$SKILL/scripts/recording.py" validate "$OUT" "$FLUSH_SECONDS" --backend eks --run-id "$RUN_ID" --scenario-dir "$SCENARIO_ID" --episode "$EPISODE" --namespace default`.

## Troubleshooting EKS runs

- **Cluster busy**: `dda inv aws.eks.gensim.submit` refuses to start while `job/gensim-orchestrator` is active. Inspect
  progress with `dda inv aws.eks.gensim.status` or stop the active run with `dda inv aws.eks.gensim.stop-all`.
- **Missing `/tmp/observer-metrics` on pod**: the Agent image was built without `anomalydetection_recorder` (or is a
  standard release image). Rebuild or select an image from CI job `dev_branch-recorder-full`.
- **`status` shows `[failed]` with `collection=failed` or `upload=failed`**: inspect orchestrator logs
  (`kubectl logs job/gensim-orchestrator -n <namespace>`) and the per-episode `error` field in `dda inv aws.eks.gensim.status`.
  Do not use an unvalidated or failed run for benchmark evaluation.
- **Multiple scenarios from one episode directory**: supported in a single submission
  (for example `--episodes=authcore-pgbouncer:pool-saturation,authcore-pgbouncer:conn-spike`); `run.go` groups scenario
  YAMLs per episode ConfigMap and tags each S3 zip with `run_id`, `episode`, and `scenario`.

## User-run live AWS acceptance checklist

Do not run live AWS commands from an implementation session unless the user explicitly asks to execute a live run.
When handing off for live validation, provide these checks for the user to run:

1. Submit one shortened or short episode on a dedicated stack:
   `dda inv aws.eks.gensim.submit --stack-name=<stack> --image=docker.io/datadog/agent-dev:<tag> --episodes=<episode>:<scenario> --s3-bucket=qbranch-gensim-recordings`
2. Monitor `dda inv aws.eks.gensim.status --stack-name=<stack>` and confirm `collection=succeeded`, `upload=succeeded`,
   `validation=pending`, and `pods=<worker_count>`.
3. Fetch and validate locally into a fresh directory:
   `dda inv aws.eks.gensim.fetch --stack-name=<stack> --episode=<episode> --scenario=<scenario> --output-dir=/tmp/eks-scenarios/<scenario>`
4. Inspect `/tmp/eks-scenarios/<scenario>/validation.json` and `recording-manifest.json`; confirm `usable: true`,
   `states.validation: "passed"`, and retained metric and log files from every recorder pod.
5. Replay in the testbench:
   `dda inv anomalydetection.launch-testbench --scenarios-dir /tmp/eks-scenarios --build`
6. Submit a multi-episode batch on the same cluster (including two scenarios from the same episode directory) to confirm
   cluster reuse, clean teardown between episodes, and distinct S3 artifact keys.
7. Confirm that a simulated pod restart or bad image reports `[failed]` in `status` rather than `[done]`.
8. Compare signal coverage (metric series and log streams) against a matching VM recording before freezing artifacts for
   detector evaluation.
