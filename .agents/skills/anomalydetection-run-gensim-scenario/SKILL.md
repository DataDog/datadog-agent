---
name: anomalydetection-run-gensim-scenario
description: >-
  Record gensim episodes with the anomaly-detection recorder on either an AWS EKS cluster (--backend eks)
  or a remote Linux VM (--backend vm) and fetch validated Parquet recordings for the Agent Anomaly Detection
  team's testbench evals. Use for "record a gensim scenario", "record a gensim scenario on a VM", "record gensim
  episodes on EKS", "set up the gensim VM", or "record a batch of episodes and fetch the parquet". Not a
  general-purpose gensim or Agent benchmarking workflow.
allowed-tools: Bash, Read, AskUserQuestion
---

# Record gensim episodes (VM or EKS)

Owner: Agent Anomaly Detection (`@DataDog/agent-anomaly-detection`).

Accept `--backend vm|eks`. If the user does not specify a backend, **ask them to choose between `eks` and `vm`**:
- `eks` is for production benchmark recordings and automatically uploads the recorded archive to S3.
- `vm` is for interactive debugging or lightweight single-node experiments.

Run the bundled helpers one at a time from the `datadog-agent` worktree and check their exit status (avoid chaining
`cd <dir> && ...`, as user shell `cd` hooks can return non-zero in non-interactive shells; use `GENSIM_REPO_PATH` or
`git -C <repo>` instead). They do not automatically tear down infrastructure so you can reuse a healthy VM or EKS
cluster across a batch. Do not claim a live AWS or VM run unless the user explicitly authorized one and you actually
performed it.

## 1. Collect inputs, authorization, and backend choice

| Backend | When to choose | Execution model |
|---|---|---|
| `eks` (production) | Production benchmark recordings and batches; automatically uploads recordings to S3 | `dda inv aws.eks.gensim.{submit,status,fetch,stop-all,destroy}` |
| `vm` | Interactive debugging, single-worker kind inspection, quick shortened trials | SSH helpers in `scripts/vm.sh`, `scripts/scenario.sh`, `scripts/fetch.sh` |

Ask for missing inputs in a single round trip before changing resources (`AskUserQuestion` requires `>= 2` options per
question):
1. **Backend**: `eks` (production, uploads to S3) or `vm` (interactive debugging)
2. **Recorder image tag**: `datadog/agent-dev:<tag>` from `dev_branch-recorder-full`
3. **Target & S3 bucket**: EKS stack (default `gensim-eks`) + S3 bucket (default `qbranch-gensim-recordings`), or VM `user@host` / stack
4. **Credentials env file**: local file defining `DD_API_KEY` and `DD_APP_KEY` for dddev
5. **Authorization & clean clone**: live-run approval, plus approval to clone `gensim-episodes` to `/tmp/gensim-episodes-clean` if the local checkout is dirty

| Input | Requirement |
|---|---|
| Backend & target | Ask if not provided: `--backend eks` (production recordings, automatically uploads to S3; authorized EKS stack name, default `gensim-eks`, plus S3 bucket) or `--backend vm` (disposable `user@host` or authorized new AWS VM) |
| Scenario | Directory in the `gensim-episodes` repo containing `play-episode.sh` and `chart/`; episode from `episodes/*.yaml`. A path under `observer-scenarios*` is an old recording output (`episode.json` + `parquet/`); read its `episode.json` to map back to `<episode-dir>:<scenario>` in `gensim-episodes` |
| Credentials | For `vm`, local env file defining `DD_API_KEY` and `DD_APP_KEY` for dddev; for `eks`, configured e2e AWS sandbox profile and dddev keys. Do not read, echo, or print secret values |
| Recorder image | `datadog/agent-dev:<tag>` from CI job `dev_branch-recorder-full` (confirm pipeline and amd64 platform). If no tag is known, point the user at the CI Visibility Explorer (`ci_level:job env:prod @git.repository.id:gitlab.ddbuild.io/DataDog/datadog-agent @ci.pipeline.name:DataDog/datadog-agent @ci.provider.instance:gitlab-ci @ci.job.name:dev_branch-recorder-full @ci.status:success`): `https://app.datadoghq.com/ci/ci-cd/explorer/executions?query=ci_level%3Ajob%20env%3Aprod%20%40git.repository.id%3Agitlab.ddbuild.io%2FDataDog%2Fdatadog-agent%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent%20%40ci.provider.instance%3Agitlab-ci%20%40ci.job.name%3Adev_branch-recorder-full%20%40ci.status%3Asuccess&agg_m=count&agg_m_source=base&agg_t=count&ci_cd_explorer_tab=pipelines&fromUser=true&refresh_mode=sliding&paused=false` |
| Run settings | `FLUSH` (default `1m`, `FLUSH_SECONDS=60`), `RETENTION` (default `90m`), `RECORDING_ONLY` (`true`/`false`, default `true` for recording), `RUN_MODE` (`full` or `shortened` on VM), or `EKS_MODE` (`record-parquet`, `live-anomaly-detection`, `live-and-record`) |
| Output | Any fresh nonexistent local absolute path per episode (e.g. `/tmp/scenarios/kafka`, `/tmp/vm-scenarios/<episode>`, or `/tmp/eks-scenarios/<episode>`); never overwrite an existing directory. `--episode`/`--scenario` on `fetch` still use the canonical `gensim-episodes` names |
| Batch / teardown | Episodes to run and resources to remove afterwards; retain reusable infrastructure until the batch finishes |

Retention must exceed the episode duration plus the final flush wait (and the full batch on VM). Do not operate on
production machines or clusters: these steps deploy workloads and create dddev monitors/events. Confirm disposable
host/stack authorization and dddev credentials before mutating resources. The VM helpers require `--disposable-vm` to
record that authorization; the flag is not a substitute for obtaining it.

Set `SKILL` to this skill's absolute directory. Local validation requires Bash, Python 3, and PyArrow
(`uv run --no-project --with pyarrow ...`).

## 2A. VM happy path (`--backend vm`)

Use at least 4 vCPU / 16 GiB and about 20 GB free disk. Set `VM` to the confirmed `user@host` (and `SSH_PORT` if
non-default), plus `FLUSH` (default `1m`), `FLUSH_SECONDS` (matching `FLUSH` in seconds, default `60`), `RETENTION`
(default `90m`), `RECORDING_ONLY` (default `true`), and `RUN_MODE` (`full` or `shortened`).

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
bash "$SKILL/scripts/vm.sh" deploy-recorder --disposable-vm "$VM" "$ENV_FILE" "$TAG" "$CLUSTER_NAME" "$FLUSH" "$RETENTION" "$RECORDING_ONLY"
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
bash "$SKILL/scripts/scenario.sh" run --disposable-vm "$VM" "$SCENARIO_ID" "$EPISODE" "$RELEASE" "$RUN_MODE" qbranch_gensim
uv run --no-project --with pyarrow bash "$SKILL/scripts/fetch.sh" "$VM" "$SCENARIO_ID" "$EPISODE" "$OUT" "$FLUSH_SECONDS"
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
one full flush interval past `end_time`, and uploads a zip archive to S3. Load [the EKS reference](references/eks.md)
only when:
- the local `gensim-episodes` checkout is dirty or `e2e.setup` fails on a missing local SSH keypair,
- a previous Pulumi submit was interrupted or the cluster reports busy,
- multi-pod collection/upload fails or local validation needs to be re-run against an already downloaded `$OUT`, or
- inspecting VM vs EKS Helm differences or running the live acceptance checklist.

0. **Authenticate to AWS once, before any other step** (especially before launching several `submit` calls in
parallel). If the SSO session is expired, every `dda`/`aws-vault` call opens its own browser login and hangs. Ask the
user to run `! aws sso login --profile sso-agent-sandbox-account-admin-8h`, then verify with
`aws-vault exec sso-agent-sandbox-account-admin-8h -- aws sts get-caller-identity --query Account --output text`.
When launching a batch from a shell loop, stagger the starts (e.g. `sleep 15`) and redirect each `dda` call's stdin
from `/dev/null`, otherwise it consumes the loop's input and only the first stack launches.

1. Submit the episode or batch in the background (first-time cluster provisioning takes >10 minutes; never run under a
short foreground timeout). `--episodes` is `<dir under postmortems/ or synthetics/>:<episodes/*.yaml name without extension>`,
e.g. `703_shopify_bfcm_incident:kafka-partition-saturation`. Source `"$ENV_FILE"` (`set -a; . "$ENV_FILE"; set +a`) in
the same shell without printing secrets, and append `--no-recording-only` when `RECORDING_ONLY=false`:

```bash
GENSIM_REPO_PATH="${GENSIM_REPO_PATH:-}" dda inv aws.eks.gensim.submit \
  --stack-name="$STACK" \
  --image="docker.io/datadog/agent-dev:$TAG" \
  --episodes="$SCENARIO_ID:$EPISODE" \
  --s3-bucket="$S3_BUCKET" \
  --mode="${EKS_MODE:-record-parquet}" \
  --flush-interval="$FLUSH" \
  --retention="$RETENTION" > /tmp/gensim-submit-"$STACK".log 2>&1   # run in background
```

2. Monitor in-cluster execution until `collection=succeeded` and `upload=succeeded`:

```bash
dda inv aws.eks.gensim.status --stack-name="$STACK"
```

`status` separates monitor outcome (`monitor`), multi-pod Parquet collection (`collection`), S3 upload (`upload`), and
local validation (`validation`). An uploaded S3 artifact (`upload=succeeded`, `validation=pending`) is **not yet a
validated benchmark recording**.

3. Fetch the uploaded artifact into a fresh nonexistent local directory (`$OUT`, e.g. `/tmp/scenarios/kafka` or
`/tmp/eks-scenarios/<episode>`) and run the shared PyArrow validator (`fetch` automatically invokes
`uv run --no-project --with pyarrow python3` when the active `python3` lacks `pyarrow`, and infers the flush interval
from `recording-manifest.json` unless `--flush-interval="$FLUSH"` is passed):

```bash
dda inv aws.eks.gensim.fetch \
  --stack-name="$STACK" \
  --run-id="$RUN_ID" \
  --episode="$SCENARIO_ID" \
  --scenario="$EPISODE" \
  --s3-bucket="$S3_BUCKET" \
  --flush-interval="$FLUSH" \
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

After all episodes in the batch have been uploaded, fetched, and validated:

- **Clean `gensim-episodes` generated files**: run `git -C <gensim-repo> status` on the repo used for `submit` (the clean
  clone, or the user's checkout if used directly) to ensure no generated files (`results/*.json`, deleted chart
  templates) remain. In the clean clone, restore generated changes with `git -C <clone> checkout -- <generated-files>`.
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
| Teardown | Releases/workloads/cluster/VM removed vs retained; `gensim-episodes` repo left clean |

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
both pods, SHA-256 checksums verified, `validation=passed`, `gensim-episodes` repo verified clean, EKS cluster retained
for the next batch.
Fill these fields from real observations, not the examples.

## Troubleshooting references

Read only for a failed or unfamiliar step:

- Recorder configuration: https://datadoghq.atlassian.net/wiki/spaces/agent/pages/7272235685/Using+the+recorder+to+record+gensim+staging+scenarios
- Lima/kind setup: https://datadoghq.atlassian.net/wiki/spaces/agent/pages/6238700466/Gensim+Hands+On
- Original setup in the `gensim-episodes` repo: `q_branch/setup-k8s-host.sh`, `q_branch/gadget-k8s-host.lima.yaml`.
- [VM Ubuntu bootstrap](references/bootstrap.md) — load only when `vm.sh probe` fails on missing tools or sysctl limits.
- [EKS multi-pod recording & live acceptance checklist](references/eks.md) — load only for EKS recovery, troubleshooting, or live acceptance handoff.
- Docker permission denied after bootstrap: close the multiplexed SSH connection and reconnect to pick up group membership.
- Exec format errors: confirm an amd64 image for the x86_64 VM.
- Docker 29 image-load errors: use a single-platform image archive instead of `kind load docker-image`.
- Multiple recorder pods or restarting containers: stop, fix topology/resources, and start a new recording.

To validate helper changes without provisioning resources, run
`uv run --no-project --with pyarrow --with pyyaml bash "$SKILL/scripts/test.sh"` and ShellCheck on `scripts/*.sh`.
The tests mock SSH/SCP/dda/kubectl/helm/aws and use generated Parquet fixtures; the user performs live AWS testing.
