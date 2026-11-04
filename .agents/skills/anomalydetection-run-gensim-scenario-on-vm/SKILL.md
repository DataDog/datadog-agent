---
name: anomalydetection-run-gensim-scenario-on-vm
description: >-
  Record gensim episodes on a remote Linux VM with the anomaly-detection recorder and fetch Parquet recordings
  for the Agent Anomaly Detection team's testbench evals. Use for "record a gensim scenario on a VM",
  "set up the gensim VM", or "record a batch of episodes and fetch the parquet". Not a general-purpose
  gensim or Agent benchmarking workflow.
allowed-tools: Bash, Read, AskUserQuestion
---

# Record gensim episodes on a VM

Owner: Agent Anomaly Detection (`@DataDog/agent-anomaly-detection`).

Run the bundled shell helpers one at a time and check their exit status. Inspect the relevant function when
troubleshooting or adapting another Linux image. They do not automatically tear down resources: reuse a healthy VM,
cluster, and recorder for a batch instead of provisioning for every episode.

Do not claim a live VM run unless you actually performed one.

## 1. Collect inputs and authorization

Ask for missing inputs and resolve ambiguity before changing resources:

| Input | Requirement |
|---|---|
| VM | Disposable `user@host` the user authorizes you to repurpose, or an authorized new AWS VM |
| Scenario | Directory in the `gensim-episodes` repo containing `play-episode.sh` and `chart/`; episode from `episodes/*.yaml` |
| Env file | Local file defining `DD_API_KEY` and `DD_APP_KEY` for dddev; do not read, echo, or print values |
| Recorder image | `datadog/agent-dev:<tag>` from CI job `dev_branch-recorder-full`; confirm pipeline and amd64 platform |
| Run settings | Full/shortened mode, cluster name, flush (default `1m`), retention (default `90m`), optional recording-only mode |
| Output | Fresh absolute directory per run, default `/tmp/vm-scenarios/<episode>`; do not overwrite an existing directory |
| Batch / teardown | Episodes to run and resources to remove afterwards; retain resources until the batch finishes |

Use at least 4 vCPU / 16 GiB and about 20 GB free disk.
Retention must exceed the whole batch plus the final flush wait; fetch each episode immediately to avoid
eviction. Do not operate on production machines or clusters: these steps install software, change host settings,
deploy workloads, and create dddev monitors/events. Confirm the disposable host and dddev credentials first.
The helpers require `--disposable-vm` to record that authorization; the flag is not a substitute for obtaining it.

Set `SKILL` to this skill's absolute directory, and `VM` to the confirmed SSH target.
Local dependencies: Bash, SSH/SCP, rsync, Python 3, and PyArrow for fetching (use `uv run --with pyarrow bash ...`).
Set `SSH_PORT` for a non-default SSH port. Use the user's SSH configuration; verify unknown host fingerprints instead
of disabling host-key checking.

## 2. Provision or reuse the VM and cluster

If creating a VM, use the repo's non-interactive e2e AWS setup first (`doc/how-to/test/e2e/`); ask for the GitHub team
if needed for `dda inv e2e.setup --team=<team>`. From the Agent checkout:

```bash
bash "$SKILL/scripts/vm.sh" create-vm --disposable-vm "$STACK" t3.2xlarge
```

Use the SSH target printed by `dda inv aws.create-vm`. `--no-install-agent` avoids a competing host Agent.
For a created or reused VM:

```bash
bash "$SKILL/scripts/vm.sh" probe "$VM"
```

Require x86_64, passwordless sudo, Docker without sudo, kind, kubectl, Helm, Python 3 with PyYAML, and inotify limits
of at least 524288 watches / 1024 instances. Stop on missing prerequisites. Load
[the bootstrap reference](references/bootstrap.md) only for missing tools, Docker permissions, or low inotify limits.
Prefer a pre-provisioned VM; a custom AMI is an optional future optimization, not part of this run.

Create a cluster only if `gadget-dev` does not already exist:

```bash
bash "$SKILL/scripts/vm.sh" create-cluster --disposable-vm "$VM"
```

The [kind asset](assets/kind.yaml) declares one worker so signals stay in one recorder directory. An existing cluster
is refused rather than replaced. Reuse it only after checking it is the intended disposable cluster and its nodes
are Ready: run `bash "$SKILL/scripts/vm.sh" reuse-cluster --disposable-vm "$VM"` to export its dedicated kubeconfig
without recreating it. Helpers use `~/gensim/kubeconfig` and require `kind-gadget-dev`; direct kubectl/Helm commands
must use that kubeconfig and explicit context too. Do not import an unrelated current production context.

## 3. Deploy the recorder once

Use the settings authorized in step 1:

```bash
bash "$SKILL/scripts/vm.sh" deploy-recorder --disposable-vm "$VM" "$ENV_FILE" "$TAG" "$CLUSTER_NAME" 1m 90m false
```

Expected outcome: the recorder runs in the intended cluster with [the configured values](assets/recorder-values.yaml)
and a mode-600 env file. Credentials must not appear in command arguments or output.
Secrets remain on the disposable VM until removal; do not print the env file or enable shell tracing.

Confirm exactly one recorder Agent pod, all containers Ready, zero restarts, and metrics/log Parquet files after a
flush interval. Check host load and probe warnings; stop and resize a saturated VM instead of accepting a degraded run.
Reusing the recorder avoids image loads and restarts. Redeployment can discard pod-local data; fetch first.

## 4. Prepare and run episodes

Copy each scenario into a fresh remote directory; only the VM copy is modified:

```bash
bash "$SKILL/scripts/scenario.sh" prepare --disposable-vm "$VM" "$SCENARIO_ID" "$SCENARIO_DIR"
bash "$SKILL/scripts/scenario.sh" run --disposable-vm "$VM" "$SCENARIO_ID" "$EPISODE" "$RELEASE" full qbranch_gensim
```

Expected outcome from `prepare`: a fresh scenario copy ready to run on the VM, without a competing Agent or stale
results. It refuses to overwrite a remote scenario directory. For another episode of that scenario,
reuse the prepared copy and invoke only `run`. For a same-name rerun, prepare a different scenario id to preserve results.
Uninstall the previous scenario release before switching scenarios so unrelated workloads do not contaminate the next
recording; keep the VM, cluster, and recorder.

Expected outcome from `run`: `results/<episode>-1.json`, `results/<episode>.log`, and Agent status snapshots for
recording validation. Retain the tool session and wait for completion. Episodes survive SSH disconnections;
inspect the log and running workloads before retrying instead of blindly rerunning.

Use `shortened` instead of `full` only when requested: it changes warmup/baseline/disruption/cooldown to 30/60/90/30 s
in the VM copy. Recopy the original scenario before a subsequent full run. Full episodes usually take about 32 min;
the monitor can add up to 300 s waiting for baseline OK.

An Alert-state warning or `"success": false` in the monitor results does not establish recording failure.
Judge recording success separately from fetched data. Missing `episode.json` means there is no usable scenario window.

## 5. Fetch and validate each recording

Pass the actual recorder flush interval in seconds:

```bash
uv run --with pyarrow bash "$SKILL/scripts/fetch.sh" "$VM" "$SCENARIO_ID" "$EPISODE" "$OUT" 60
```

Expected outcome: validated recordings for the episode window in a fresh output directory, ready for replay.
Fetch failures, corrupt files, and unusable recordings must stop the workflow rather than produce a successful result.
Recordings from the original procedure without episode Agent snapshots can still be fetched; report their
restart/replacement history as unknown rather than claiming it was verified.

Preserve the original trim rule: keep whole files whose minimum/maximum `Time` overlaps `start_time`..`end_time`,
not filenames (flush time). Integer timestamps may be seconds, milliseconds, microseconds, or nanoseconds.
Both metrics/log families must contain nonempty in-window data. Report observed timestamp coverage and sample gaps;
sparse timestamps are a warning, not a new rejection rule. Investigate coverage warnings before claiming a complete
recording. These checks do not prove every metric series is complete; also report restarts, saturation, and probe errors.

Outputs: `episode.json`, `validation.json`, Agent status snapshots, and `parquet/` with retained files. Boundary files
may include samples outside the window. `advances.jsonl` and `detect_digests.jsonl`, when present, remain untrimmed and
span the Agent run. Unfiltered remote files remain until retention or restart. On failure, retain partial output for
diagnosis and retry into a fresh directory; do not delete user-selected data.

Replay only after validation succeeds, with output named for the episode under a parent of scenario directories:

```bash
dda inv anomalydetection.launch-testbench --scenarios-dir "$(dirname "$OUT")" --build
```

## 6. Teardown and report

Perform only authorized teardown, after fetching all episodes. On the VM, uninstall selected releases with
`KUBECONFIG="$HOME/gensim/kubeconfig" helm --kube-context kind-gadget-dev --namespace default uninstall <release>`
(or `datadog-agent`). Delete kind only when selected: `kind delete cluster --name gadget-dev`.
Destroy a VM created in step 2 only when selected: `dda inv aws.destroy-vm --stack-name=<stack>` from the checkout.
Report resources and secrets still present when the VM is retained.

Use this final-response template per episode:

| Field | Report |
|---|---|
| Episode | Scenario id, episode, full/shortened mode |
| VM / image | SSH target, created/reused status, exact image tag and source pipeline |
| Output | Absolute path, kept metrics/log file and in-window row counts, removed file count |
| Window | Start/end, flush interval, observed coverage from `validation.json` |
| Quality | Usable/failed, sample gaps, restarts, saturation/probe errors; distinguish unknown from verified |
| Teardown | Releases/cluster/VM removed, resources and secrets retained |

Example request: "Record a shortened kafka-partition-saturation episode on `ubuntu@10.0.1.23` with
datadog/agent-dev:my-branch-recorder-full from pipeline 123, use ~/dddev.env, save under /tmp/vm-scenarios, and keep the VM."

Illustrative result: shortened episode, reused VM, specified image/pipeline, output
`/tmp/vm-scenarios/kafka-partition-saturation`; 4 metrics files / 1000 rows and 4 log files / 200 rows retained, 2 files
removed; window 10:12–10:16 UTC, flush 60 s, maximum observed gaps 15 s metrics / 30 s logs, no restarts or probe errors
observed; validation passed. VM, cluster, recorder, scenario release, and env file retained as requested.
Fill these fields from real observations, not the example.

## Troubleshooting references

Read only for a failed or unfamiliar step:

- Recorder configuration: https://datadoghq.atlassian.net/wiki/spaces/agent/pages/7272235685/Using+the+recorder+to+record+gensim+staging+scenarios
- Lima/kind setup: https://datadoghq.atlassian.net/wiki/spaces/agent/pages/6238700466/Gensim+Hands+On
- Docker permission denied after bootstrap: close the multiplexed SSH connection and reconnect to pick up group membership.
- Exec format errors: confirm an amd64 image for the x86_64 VM.
- Docker 29 image-load errors: use a single-platform image archive instead of `kind load docker-image`.
- Multiple recorder pods or restarting containers: stop, fix topology/resources, and start a new recording.

To validate helper changes without provisioning resources, run
`uv run --no-project --with pyarrow --with pyyaml bash "$SKILL/scripts/test.sh"` and ShellCheck on `scripts/*.sh`.
The tests mock SSH/SCP/dda and use generated Parquet fixtures; they do not validate a live chart or AMI.
