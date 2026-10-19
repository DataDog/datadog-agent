---
name: anomalydetection-run-gensim-scenario-on-vm
description: >-
  Record gensim episodes with the anomaly-detection recorder on either a remote Linux VM (--backend vm, default)
  or an AWS EKS cluster (--backend eks) and fetch validated Parquet recordings for the Agent Anomaly Detection
  team's testbench evals. Use for "record a gensim scenario on a VM", "record gensim episodes on EKS",
  "set up the gensim VM", or "record a batch of episodes and fetch the parquet". Not a general-purpose gensim
  or Agent benchmarking workflow.
allowed-tools: Bash, Read, AskUserQuestion
---

# Record gensim episodes (VM or EKS)

Owner: Agent Anomaly Detection (`@DataDog/agent-anomaly-detection`).

Accept `--backend vm|eks` (default `vm` when omitted so existing invocations keep working). Use `--backend eks` for
controlled, repeatable multi-episode benchmark batches, and `--backend vm` for interactive debugging or lightweight
single-node experiments.

Run the bundled helpers one at a time and check their exit status. They do not automatically tear down infrastructure so
you can reuse a healthy VM or EKS cluster across a batch. Do not claim a live AWS or VM run unless the user explicitly
authorized one and you actually performed it.

## 1. Collect inputs, authorization, and backend choice

| Backend | When to choose | Execution model |
|---|---|---|
| `vm` (default) | Interactive debugging, single-worker kind inspection, quick shortened trials | SSH helpers in `scripts/vm.sh`, `scripts/scenario.sh`, `scripts/fetch.sh` |
| `eks` (recommended for final batches) | Controlled, repeatable benchmark batches with S3 artifact archival | `dda inv aws.eks.gensim.{submit,status,fetch,stop-all,destroy}` |

Ask for missing inputs and resolve ambiguity before changing resources:

| Input | Requirement |
|---|---|
| Backend & target | `--backend vm` (disposable `user@host` or authorized new AWS VM) or `--backend eks` (authorized EKS stack name, default `gensim-eks`, plus S3 bucket) |
| Scenario | Directory in the `gensim-episodes` repo containing `play-episode.sh` and `chart/`; episode from `episodes/*.yaml` |
| Credentials | For `vm`, local env file defining `DD_API_KEY` and `DD_APP_KEY` for dddev; for `eks`, configured e2e AWS sandbox profile and dddev keys. Do not read, echo, or print secret values |
| Recorder image | `datadog/agent-dev:<tag>` from CI job `dev_branch-recorder-full`; confirm pipeline and amd64 platform |
| Run settings | Flush interval (default `1m`), retention (default `90m`), recording-only mode (default `true` for recording), VM full/shortened mode or EKS mode (`record-parquet`, `live-anomaly-detection`, `live-and-record`) |
| Output | Fresh local absolute directory per episode (e.g. `/tmp/vm-scenarios/<episode>` or `/tmp/eks-scenarios/<episode>`); never overwrite an existing directory. If the user gives a custom name (e.g. `kafka-partition-saturation2`), use it as the directory name; `--episode`/`--scenario` on `fetch` still use the real names |
| Batch / teardown | Episodes to run and resources to remove afterwards; retain reusable infrastructure until the batch finishes |

Retention must exceed the episode duration plus the final flush wait (and the full batch on VM). Do not operate on
production machines or clusters: these steps deploy workloads and create dddev monitors/events. Confirm disposable
host/stack authorization and dddev credentials before mutating resources. The VM helpers require `--disposable-vm` to
record that authorization; the flag is not a substitute for obtaining it.

Set `SKILL` to this skill's absolute directory. Local validation requires Bash, Python 3, and PyArrow
(`uv run --with pyarrow ...`).

## 2A. VM happy path (`--backend vm`)

Use at least 4 vCPU / 16 GiB and about 20 GB free disk. Set `VM` to the confirmed `user@host` (and `SSH_PORT` if
non-default).

1. Provision or probe the VM (run `dda inv e2e.setup --team=<team>` first if AWS e2e setup is missing):

```bash
bash "$SKILL/scripts/vm.sh" create-vm --disposable-vm "$STACK" t3.2xlarge
bash "$SKILL/scripts/vm.sh" probe "$VM"
```

Require x86_64, passwordless sudo, Docker without sudo, kind, kubectl, Helm, Python 3 with PyYAML, and inotify limits of
at least 524288 watches / 1024 instances. Load [the bootstrap reference](references/bootstrap.md) only when `probe`
reports missing tools, Docker permissions, or low inotify limits.

2. Create or reuse the `gadget-dev` kind cluster (`assets/kind.yaml` uses one worker so signals stay in one pod) and
deploy the recorder (`assets/recorder-values.yaml`):

```bash
bash "$SKILL/scripts/vm.sh" create-cluster --disposable-vm "$VM"
bash "$SKILL/scripts/vm.sh" deploy-recorder --disposable-vm "$VM" "$ENV_FILE" "$TAG" "$CLUSTER_NAME" 1m 90m true
```

If `gadget-dev` already exists on the authorized disposable VM and its nodes are Ready, run
`bash "$SKILL/scripts/vm.sh" reuse-cluster --disposable-vm "$VM"` instead of `create-cluster`. Helpers use
`~/gensim/kubeconfig` and require `kind-gadget-dev`.
Expected outcome: the recorder runs in the intended cluster with [the configured values](assets/recorder-values.yaml)
and a mode-600 env file. Credentials must not appear in command arguments or output. Confirm one Ready recorder Agent
pod with zero restarts and metrics/log Parquet files after a flush interval.

3. Prepare the scenario, run the episode, and fetch/validate into a fresh local directory:

```bash
bash "$SKILL/scripts/scenario.sh" prepare --disposable-vm "$VM" "$SCENARIO_ID" "$SCENARIO_DIR"
bash "$SKILL/scripts/scenario.sh" run --disposable-vm "$VM" "$SCENARIO_ID" "$EPISODE" "$RELEASE" full qbranch_gensim
uv run --with pyarrow bash "$SKILL/scripts/fetch.sh" "$VM" "$SCENARIO_ID" "$EPISODE" "$OUT" 60
```

Expected outcome from `prepare`: a fresh scenario copy ready to run on the VM, without a competing Agent or stale
results. Reuse the prepared copy across episodes of the same scenario; uninstall the previous scenario release before
switching scenarios.
Expected outcome from `run`: `results/<episode>-1.json`, `results/<episode>.log`, and Agent status snapshots for
recording validation. Retain the tool session and wait for completion; episodes survive SSH disconnections, so inspect
the log and running workloads before retrying. Use `shortened` instead of `full` only when requested (recopy before a
full run).

## 2B. EKS happy path (`--backend eks`)

EKS orchestrates episodes in-cluster via `job/gensim-orchestrator`, collects from every recorder Agent pod after waiting
one full flush interval past `end_time`, and uploads a zip archive to S3. Load
[the EKS reference](references/eks.md) when troubleshooting multi-pod collection, inspecting Helm differences, or
running the live acceptance checklist.

1. Submit the episode or batch (requires a clean `gensim-episodes` checkout and a recorder-enabled `agent-dev` image).
`--episodes` is `<dir under postmortems/ or synthetics/>:<episodes/*.yaml name without extension>`, e.g.
`703_shopify_bfcm_incident:kafka-partition-saturation`, not the human scenario name. A user-supplied "scenario" path in
`observer-scenarios*` is usually an old recording output (`episode.json` + `parquet/`), not a source; read its
`episode.json` for the episode name and locate the source in `gensim-episodes`.

If the user's `gensim-episodes` checkout is dirty (`submit` refuses it), never reset or stash it. Clone it to a fresh
directory (`git clone --local <repo> /tmp/gensim-episodes-clean`) and pass `GENSIM_REPO_PATH=/tmp/gensim-episodes-clean`.

A first submit provisions the EKS cluster, node group and image builder and takes well over 10 minutes. **Never run it in
the foreground or under a short `timeout`**: killing it mid-update leaves a partial Pulumi state. Run it with
`run_in_background` (no timeout), redirect output to a log file, and poll `status` or the log. If a run was interrupted,
check that no `pulumi` process is active before re-submitting; the orphan `pulumi-resource-*` plugin processes alone do
not mean an update is running.

```bash
GENSIM_REPO_PATH="${GENSIM_REPO_PATH:-}" dda inv aws.eks.gensim.submit \
  --stack-name="$STACK" \
  --image="docker.io/datadog/agent-dev:$TAG" \
  --episodes="$EPISODE_DIR:$EPISODE" \
  --s3-bucket="$S3_BUCKET" \
  --flush-interval="$FLUSH" \
  --retention="$RETENTION" > /tmp/gensim-submit-"$STACK".log 2>&1   # run in background
```

Source the user's env file (`set -a; . "$ENV_FILE"; set +a`) in the same shell invocation without printing it. If
`dda inv e2e.setup` fails only on a missing local keypair file, do not delete the remote keypair; try `submit` first
(EKS does not need the SSH key) and report the issue.

2. Monitor in-cluster execution:

```bash
dda inv aws.eks.gensim.status --stack-name="$STACK"
```

`status` separates monitor outcome (`monitor`), multi-pod Parquet collection (`collection`), S3 upload (`upload`), and
local validation (`validation`). An uploaded S3 artifact (`upload=succeeded`, `validation=pending`) is **not yet a
validated benchmark recording**.

3. Fetch the uploaded artifact into a fresh local directory and run the shared PyArrow validator.
`uv run --with pyarrow dda inv ...` does **not** reliably expose `pyarrow` to the validator (`dda` overrides `PATH`, so the
validator's `python3` fails with `No module named 'pyarrow'` and leaves a partial `$OUT`). If that happens, the artifact is
already downloaded in `$OUT`: create a venv (`uv venv /tmp/pyarrow-venv && uv pip install --python /tmp/pyarrow-venv/bin/python pyarrow`)
and run the validator directly with it first on `PATH`:
`PATH=/tmp/pyarrow-venv/bin:$PATH python3 "$SKILL/scripts/recording.py" validate "$OUT" 60 --backend eks --run-id "$RUN_ID" --scenario-dir "$SCENARIO_ID" --episode "$EPISODE" --namespace default`.

```bash
uv run --with pyarrow dda inv aws.eks.gensim.fetch \
  --stack-name="$STACK" \
  --run-id="$RUN_ID" \
  --episode="$SCENARIO_ID" \
  --scenario="$EPISODE" \
  --s3-bucket="$S3_BUCKET" \
  --output-dir="$OUT"
```

## 3. Shared validation contract and testbench replay

Both `scripts/fetch.sh` (VM) and `dda inv aws.eks.gensim.fetch` (EKS) run
`scripts/recording.py validate`:

- Creates `$OUT` exclusively so existing data is never overwritten.
- Verifies safe archive paths, Agent pod readiness and zero restarts (`agent-before.json` / `agent-after.json`), and
  manifest SHA-256 checksums. Legacy VM recordings without before/after snapshots remain fetchable with
  `agent_history_verified: false`.
- Preserves whole-file trimming: retains `.parquet` files whose `Time` min/max overlaps `start_time..end_time` across
  seconds, milliseconds, microseconds, and nanoseconds.
- Requires nonempty in-window metrics (`observer-metrics-*.parquet`) and logs (`observer-logs-*.parquet`) from every
  expected recorder pod. Sparse timestamps produce `coverage_warnings`, not automatic failure; inspect warnings before
  claiming complete coverage. Monitor `"success": false` in `episode.json` does not invalidate a recording.
- Writes `episode.json`, `validation.json`, `recording-manifest.json`, and `parquet/` (preserving untrimmed sidecars
  such as `advances.jsonl` and `detect_digests.jsonl`). On failure, retains partial output in `$OUT` for diagnosis.

Replay only after local validation succeeds (`usable: true` in `validation.json` and `states.validation: "passed"` in
`recording-manifest.json`):

```bash
dda inv anomalydetection.launch-testbench --scenarios-dir "$(dirname "$OUT")" --build
```

## 4. Teardown and report

Perform only authorized teardown after all episodes in the batch have been fetched and validated:

- **VM**: uninstall selected Helm releases (`KUBECONFIG="$HOME/gensim/kubeconfig" helm --kube-context kind-gadget-dev --namespace default uninstall <release>`), delete kind only when requested (`kind delete cluster --name gadget-dev`), and destroy the VM only when requested (`dda inv aws.destroy-vm --stack-name=<stack>`).
- **EKS**: stop active workloads while keeping the cluster with `dda inv aws.eks.gensim.stop-all --stack-name=<stack>`; destroy the cluster only when requested with `dda inv aws.eks.gensim.destroy --stack-name=<stack>`.

Use this final-response template per episode:

| Field | Report |
|---|---|
| Episode | Backend (`vm` or `eks`), scenario id, episode, run ID, mode |
| Target / image | VM SSH target or EKS stack/namespace, recorder pod count, exact image tag and observed image IDs |
| Output / artifact | Local `$OUT` path (and S3 artifact key on EKS), kept metrics/log file and row counts, removed file count |
| Window | Start/end, flush interval, observed coverage from `validation.json` |
| Quality / states | `episode_outcome`, `collection`, `upload`, `validation` (`passed`/`failed`), sample gaps, restarts; distinguish `unknown` from `verified` |
| Teardown | Releases/workloads/cluster/VM removed vs retained |

Example 1 (VM): "Record a shortened `kafka-partition-saturation` episode on `ubuntu@10.0.1.23` (`--backend vm`) with
`datadog/agent-dev:my-branch-recorder-full`, use `~/dddev.env`, save under `/tmp/vm-scenarios`, and keep the VM."
Result: `backend=vm`, reused `ubuntu@10.0.1.23` (1 recorder pod), output `/tmp/vm-scenarios/kafka-partition-saturation`,
4 metrics files (1000 rows) and 4 log files (200 rows) retained, 2 out-of-window files removed, `validation=passed`,
`agent_history_verified=true`, VM and kind cluster retained.

Example 2 (EKS): "Record `authcore-pgbouncer:pool-saturation` with `--backend eks` on stack `gensim-eks` using
`docker.io/datadog/agent-dev:my-branch-recorder-full`, upload to `qbranch-gensim-recordings`, fetch to
`/tmp/eks-scenarios/pool-saturation`, and keep the cluster."
Result: `backend=eks`, `run_id=eval-20261015T120000Z-a1b2c3d-9f8e7d`, 2 recorder pods on `gensim-eks`, S3 artifact
`gensim-results-eval-20261015T120000Z-a1b2c3d-9f8e7d-authcore-pgbouncer-pool-saturation.zip` (`collection=succeeded`,
`upload=succeeded`), fetched to `/tmp/eks-scenarios/pool-saturation` with 8 metrics files and 8 log files retained across
both pods, SHA-256 checksums verified, `validation=passed`, EKS cluster retained for the next batch.
Fill these fields from real observations, not the examples.

## Troubleshooting references

Read only for a failed or unfamiliar step:

- Recorder configuration: https://datadoghq.atlassian.net/wiki/spaces/agent/pages/7272235685/Using+the+recorder+to+record+gensim+staging+scenarios
- Lima/kind setup: https://datadoghq.atlassian.net/wiki/spaces/agent/pages/6238700466/Gensim+Hands+On
- Original setup in the `gensim-episodes` repo: `q_branch/setup-k8s-host.sh`, `q_branch/gadget-k8s-host.lima.yaml`.
- [VM Ubuntu bootstrap](references/bootstrap.md) — load only when `vm.sh probe` fails on missing tools or sysctl limits.
- [EKS multi-pod recording & live acceptance checklist](references/eks.md) — load only for EKS troubleshooting or live acceptance handoff.
- Docker permission denied after bootstrap: close the multiplexed SSH connection and reconnect to pick up group membership.
- Exec format errors: confirm an amd64 image for the x86_64 VM.
- Docker 29 image-load errors: use a single-platform image archive instead of `kind load docker-image`.
- Multiple recorder pods or restarting containers: stop, fix topology/resources, and start a new recording.

To validate helper changes without provisioning resources, run
`uv run --no-project --with pyarrow --with pyyaml bash "$SKILL/scripts/test.sh"` and ShellCheck on `scripts/*.sh`.
The tests mock SSH/SCP/dda/kubectl/helm/aws and use generated Parquet fixtures; the user performs live AWS testing.
