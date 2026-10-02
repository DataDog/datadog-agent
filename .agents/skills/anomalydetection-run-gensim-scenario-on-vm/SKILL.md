---
name: anomalydetection-run-gensim-scenario-on-vm
description: >-
  Record a gensim episode with the anomaly-detection recorder on a remote Linux VM (kind cluster + Datadog Agent
  recorder image) and fetch the recording locally for the testbench. Use for "record a gensim scenario on a VM",
  "set up the gensim VM", "run the kafka-partition-saturation episode and fetch the parquet".
allowed-tools: Bash, Read, AskUserQuestion
argument-hint: "[scenario path in gensim-episodes] [episode name] [VM ssh target]"
---

> **Scope:** this skill is only for the Agent Anomaly Detection team. It uses the recorder component to record data
> from the Agent into parquet files, in order to run our evals. It is not a general-purpose gensim or Agent
> benchmarking procedure.

Runs a gensim episode on a remote x86_64 Ubuntu VM instead of a local Lima VM, records it with the
`recorder-full` Agent image, and copies the part of the recording that covers the episode to the local machine.

Documentation this skill is derived from (read them if a step fails):

- Recorder + gensim: https://datadoghq.atlassian.net/wiki/spaces/agent/pages/7272235685/Using+the+recorder+to+record+gensim+staging+scenarios
- Gensim hands-on (Lima + kind setup): https://datadoghq.atlassian.net/wiki/spaces/agent/pages/6238700466/Gensim+Hands+On
- Lima setup script this replaces: `q_branch/setup-k8s-host.sh`, `q_branch/gadget-k8s-host.lima.yaml`
- Scenarios: https://github.com/DataDog/gensim-episodes

Validation status: every step was run end to end on a 2 vCPU / 3.7 GiB Ubuntu 22.04 x86_64 VM (shortened episode,
see step 7), except creating the VM with `aws.create-vm` (step 2). Report any deviation to the user.

Notation: `<vm>` is the ssh target (`user@host`, add `-p <port>` where needed); commands marked "on the VM" run through
`ssh <vm> '...'`. `kubectl` and `helm` run on the VM, no tunnel to the cluster is needed.

## Step 1 — Collect inputs

Ask with `AskUserQuestion` for everything not already given. Never guess these, and never print secret values.

| Input | Notes |
|---|---|
| Env file (local path) | Must define `DD_API_KEY` and `DD_APP_KEY` (dddev org). **Never read, cat or echo it**; check only variable names and non-emptiness (step 3) |
| VM | An existing `user@host` (and ssh port), or create one with `dda inv aws.create-vm` (step 2) |
| Scenario | Directory under `postmortems/<id>/` in a local `gensim-episodes` checkout, and an episode name from its `episodes/*.yaml`. The episode is not a directory |
| `gensim-episodes` checkout | Local path; if none, `git clone git@github.com:DataDog/gensim-episodes.git` |
| Recorder image | `datadog/agent-dev:<tag>` built by CI job `dev_branch-recorder-full` (tag looks like `<branch>-recorder-full`), or ask which pipeline to use. Must be amd64 for an x86_64 VM |
| Output directory | Where to store the recording, default `/tmp/vm-scenarios/<episode-name>` |
| Run length | Full episode (about 32 min) or shortened test run (about 4 min, step 7) |
| Recording settings | Flush interval (default `1m`) and retention (default `90m`, must be longer than the episode plus the final wait of step 8) |
| Cluster name | `clusterName` for the Agent, `<name>-gensim-cluster` |
| Teardown | Whether to uninstall / delete the cluster / destroy the VM when done |

Also confirm any other ambiguity (several scenarios, custom images, non-default namespace) before acting.

## Step 2 — VM

Size matters. A 2 vCPU / 3.7 GiB VM can run kind + the Agent + an 11-service scenario, but it is saturated (load
average ~10 on 2 vCPUs, Agent probes time out): the recording is degraded. Use at least 4 vCPU / 16 GiB.

To create one (needs the e2e AWS setup, see `doc/how-to/test/e2e/`; do not run the interactive setup):

```bash
dda inv aws.create-vm --instance-type=t3.2xlarge --no-install-agent --stack-name=<name>
```

`--no-install-agent` avoids a host Agent competing for resources. Take the ssh user/host from the task output.
Destroy later with `dda inv aws.destroy-vm --stack-name=<name>`.

Probe the VM before changing it (the first connection may need `-o StrictHostKeyChecking=accept-new`):

```bash
ssh <vm> 'uname -m; nproc; free -h; df -h /; for t in docker kind kubectl helm python3 curl; do printf "$t: "; command -v $t || echo missing; done; sudo -n true && echo sudo-ok'
```

Needs `x86_64` (or adapt every `amd64` below), passwordless sudo, and ~20 GB of free disk.

## Step 3 — Copy the env file

```bash
ssh <vm> 'mkdir -p ~/gensim'
scp <env-file> <vm>:~/gensim/env
ssh <vm> 'chmod 600 ~/gensim/env && printf "DD_ENV=qbranch_gensim\nKUBE_NAMESPACE=default\n" >> ~/gensim/env'
ssh <vm> 'awk -F= "{print \$1\": \" (length(\$2)>0 ? \"set\" : \"EMPTY\")}" ~/gensim/env'
```

The file must end with a newline before appending. Stop if a key reports `EMPTY`. `DD_ENV` tags the data and the monitor
the episode creates; ask if the user wants another value.

## Step 4 — Install tooling and create the cluster

On the VM. Versions match `q_branch/gadget-k8s-host.lima.yaml`.

```bash
# Docker CE (skip if `docker` already exists)
sudo apt-get update -qq && sudo apt-get install -y -qq curl ca-certificates gpg apt-transport-https
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor --yes -o /etc/apt/keyrings/docker.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
sudo apt-get update -qq && sudo apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl enable --now docker

sudo usermod -aG docker "$USER"

# kubectl
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.32/deb/Release.key | sudo gpg --dearmor --yes -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.32/deb/ /" | sudo tee /etc/apt/sources.list.d/kubernetes.list >/dev/null
sudo apt-get update -qq && sudo apt-get install -y -qq kubectl

# kind and helm
sudo curl -fsSLo /usr/local/bin/kind https://kind.sigs.k8s.io/dl/v0.27.0/kind-linux-amd64 && sudo chmod +x /usr/local/bin/kind
curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | sudo DESIRED_VERSION=v3.20.2 bash

# inotify limits (kind / Agent fail with "too many open files" otherwise); warnings on unrelated keys are harmless
printf "fs.inotify.max_user_watches=524288\nfs.inotify.max_user_instances=1024\n" | sudo tee /etc/sysctl.d/99-kind.conf >/dev/null
sudo sysctl --system >/dev/null

# play-episode.sh needs python3 + pyyaml + curl
python3 -c "import yaml" 2>/dev/null || sudo apt-get install -y -qq python3-yaml
```

**Close the ssh multiplexed connection from the local machine**: `ssh -O exit <vm>`. With `ControlMaster auto` the open
session keeps its old groups and kind fails with "permission denied" on the docker socket. Then, in a new session:

```bash
helm repo add datadog https://helm.datadoghq.com --force-update && helm repo update
docker info >/dev/null && echo docker-ok     # must work without sudo
```

Create the cluster with **one worker only** (two workers split signals across two parquet directories):

```bash
kind create cluster --name gadget-dev --config /dev/stdin <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  apiServerAddress: "127.0.0.1"
  apiServerPort: 6443
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 30000
        hostPort: 30000
      - containerPort: 30080
        hostPort: 30080
      - containerPort: 30443
        hostPort: 30443
  - role: worker
EOF
kubectl wait --for=condition=Ready node --all --timeout=180s
```

The context is `kind-gadget-dev`.

## Step 5 — Deploy the recorder Agent

Load the image. **Do not use `kind load docker-image`**: with Docker 29's multi-arch image store it fails with
`ctr: content digest ... not found`. Save a single platform and load the archive:

```bash
IMAGE=datadog/agent-dev:<tag>
docker pull --platform linux/amd64 "$IMAGE"
docker image save --platform linux/amd64 -o /tmp/agent-recorder.tar "$IMAGE"
kind load image-archive /tmp/agent-recorder.tar --name gadget-dev && rm /tmp/agent-recorder.tar
docker exec gadget-dev-worker crictl images | grep agent-dev
```

Create the secret without echoing the key:

```bash
set -a; . ~/gensim/env; set +a
kubectl create secret generic datadog-secret --from-literal api-key="$DD_API_KEY"
```

Write the values (set `NAME`, `TAG`, `FLUSH`, `RETENTION` from step 1):

```bash
NAME=<name>; TAG=<tag>; FLUSH=1m; RETENTION=90m
cat > ~/datadog-values.yaml <<EOF
datadog:
  apiKeyExistingSecret: "datadog-secret"
  clusterName: "${NAME}-gensim-cluster"
  logLevel: "INFO"
  apm:
    instrumentation:
      enabled: true
  logs:
    enabled: true
    containerCollectAll: true
  processAgent:
    enabled: true
    processCollection: true
  kubelet:
    tlsVerify: false
  clusterChecks:
    enabled: true
  env:
  - name: DD_ANOMALY_DETECTION_RECORDING_ENABLED
    value: "true"
  - name: DD_ANOMALY_DETECTION_RECORDING_OUTPUT_DIR
    value: "/tmp/observer-metrics"
  - name: DD_ANOMALY_DETECTION_RECORDING_FLUSH_INTERVAL
    value: "${FLUSH}"
  - name: DD_ANOMALY_DETECTION_RECORDING_RETENTION
    value: "${RETENTION}"
agents:
  image:
    repository: datadog/agent-dev
    tag: ${TAG}
    pullPolicy: Never
    doNotCheckTag: true
clusterAgent:
  enabled: true
  replicas: 1
EOF
helm upgrade --install datadog-agent -f ~/datadog-values.yaml datadog/datadog
kubectl rollout status daemonset/datadog-agent --timeout=180s
```

Optional: `DD_ANOMALY_DETECTION_RECORDING_ONLY=true` records without detecting. Prefer the
`DD_ANOMALY_DETECTION_RECORDING_*` variables of the recorder page over the `DD_OBSERVER_*` ones from the older
hands-on page. Check the recorder works:

```bash
kubectl get pods   # agent 3/3, cluster agent 1/1, 0 restarts
POD=$(kubectl get pod -l app.kubernetes.io/component=agent -o name | head -1)
kubectl exec $POD -c agent -- ls /tmp/observer-metrics   # first parquet files appear after one flush interval (1m by default)
```

Recording starts when the Agent starts, so the files cover more than the episode; step 8 trims them. Changing the values
and re-running `helm upgrade` restarts the Agent and starts a clean recording.

## Step 6 — Load the scenario images

From the local machine:

```bash
rsync -a --exclude results --exclude run-result.json <gensim-episodes>/postmortems/<id> <vm>:~/gensim/
```

On the VM, in `~/gensim/<id>`:

```bash
cd ~/gensim/<id>
rm -f chart/templates/datadog-agent.yaml    # the episode must not deploy its own Agent
nohup sh -c 'docker compose build && echo BUILD_DONE' > /tmp/build.log 2>&1 < /dev/null &
until grep -q -E 'BUILD_DONE|ERROR|failed' /tmp/build.log; do sleep 5; done; tail -3 /tmp/build.log
docker image save -o /tmp/scenario.tar $(docker compose config --images)     # archive: same Docker 29 caveat
kind load image-archive /tmp/scenario.tar --name gadget-dev && rm /tmp/scenario.tar
docker exec gadget-dev-worker crictl images | grep -c gensim    # expect one per service
```

Building ~11 images takes a few minutes. Stop if the log reports an error. If the scenario has no `docker-compose.yaml`,
its images are public and nothing needs building.

## Step 7 — Run the episode

```bash
cd ~/gensim/<id>
set -a; . ~/gensim/env; set +a
helm install <release> ./chart --set datadog.env="$DD_ENV"
kubectl wait --for=condition=Ready pod --all --timeout=300s
kubectl get events --field-selector type=Warning --sort-by=.lastTimestamp | tail    # probe warnings during startup are expected
cut -d' ' -f1-3 /proc/loadavg
./play-episode.sh list-episodes
```

Check the Agent has 0 restarts before playing: a restarted Agent container loses `/tmp/observer-metrics`, and a
CPU-starved Agent produces gaps. If the VM is saturated, stop and resize rather than record.

The full episode takes ~32 minutes: phase durations are hardcoded at the top of `play-episode.sh` (warmup 120 s, baseline
600 s, disruption 600 s, cooldown 600 s). For a **test run**, shorten them in the VM copy only (never in the user's checkout):

```bash
sed -i -E 's/^WARMUP_DURATION=.*/WARMUP_DURATION=30/; s/^BASELINE_DURATION=.*/BASELINE_DURATION=60/; s/^DISRUPTION_DURATION=.*/DISRUPTION_DURATION=90/; s/^COOLDOWN_DURATION=.*/COOLDOWN_DURATION=30/' play-episode.sh
```

Run it in the background (redirect stdin, otherwise the ssh call hangs) and wait for completion:

```bash
nohup ./play-episode.sh run-episode <episode-name> > /tmp/episode.log 2>&1 < /dev/null &
until grep -q 'All cycles complete' /tmp/episode.log || ! pgrep -f '[p]lay-episode.sh run-episode' >/dev/null; do sleep 10; done
tail -20 /tmp/episode.log
ls results/
```

`play-episode.sh` needs `DD_API_KEY` / `DD_APP_KEY` in its environment; it creates a monitor and posts events in the dddev
org. Expected behavior:

- The baseline phase waits for the monitor to reach OK (up to 300 s), so it can last longer than configured, and the
  monitor shows `No Data` at first.
- `WARNING: Monitor did not reach Alert state during disruption` is **normal**, and `"success": false` in the results file
  goes with it. It does not mean the recording failed; ignore it.
- `results/<episode-name>-1.json` is written at the end and holds `start_time`, `end_time` and the
  baseline/disruption/cooldown windows.
- `pgrep -f` matches its own ssh command line: use the `[p]` bracket form, as above.

## Step 8 — Fetch and trim the recording

**Wait for the last flush before fetching.** The Agent only writes a parquet file once per flush interval (1 minute by
default), so the end of the episode is not on disk until the next flush: wait at least one full flush interval after
`end_time` in the results file (`ls -l --time-style=full-iso /tmp/observer-metrics` in the Agent container should show a
parquet file newer than `end_time`), then, from the local machine, stream the files out (no `kubectl cp` needed):

```bash
OUT=<output-dir>            # default /tmp/vm-scenarios/<episode-name>
rm -rf $OUT; mkdir -p $OUT/parquet
ssh <vm> 'P=$(kubectl get pod -l app.kubernetes.io/component=agent -o name | head -1); kubectl exec $P -c agent -- tar cf - -C /tmp/observer-metrics .' | tar xf - -C $OUT/parquet
scp <vm>:~/gensim/<id>/results/<episode-name>-1.json $OUT/episode.json
for f in $OUT/parquet/*.parquet; do [ "$(tail -c4 $f)" = PAR1 ] || echo "truncated: $f"; done
```

Expect `observer-metrics-*.parquet`, `observer-logs-*.parquet`, `advances.jsonl`, `detect_digests.jsonl`.

**Trim to the episode.** The Agent records from its start, so the directory also holds data from before and after the
episode. The timestamp in a parquet filename is the *flush* time (close to the end of the file's data), not its start, so
do not select by filename. Select by the `Time` column inside each file and keep the files overlapping
`start_time`..`end_time` of `episode.json`. This needs `pyarrow` (`uv run --with pyarrow python3 - <<'EOF'` works):

```bash
OUT=<output-dir> python3 - <<'EOF'
import glob, json, os, datetime as dt
import pyarrow.parquet as pq, pyarrow.compute as pc
out = os.environ["OUT"]
ep = json.load(open(f"{out}/episode.json"))
ts = lambda s: dt.datetime.strptime(s, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=dt.timezone.utc).timestamp()
start, end = ts(ep["start_time"]), ts(ep["end_time"])
def seconds(v):                      # Time may be s, ms, us or ns
    for d in (1, 1e3, 1e6, 1e9):
        if 1e9 < v / d < 4e9:
            return v / d
kept = dropped = 0
for f in sorted(glob.glob(f"{out}/parquet/*.parquet")):
    t = pq.read_table(f, columns=["Time"]).column(0)
    mm = pc.min_max(t).as_py() if len(t) else None
    if mm and seconds(mm["max"]) >= start and seconds(mm["min"]) <= end:
        kept += 1
    else:
        os.remove(f); dropped += 1
print(f"kept {kept}, removed {dropped}")
EOF
```

The first and last kept files straddle the window, so they can hold up to one flush interval of data outside it. `advances.jsonl` and
`detect_digests.jsonl` span the whole Agent run and are left untouched; mention it to the user. The unfiltered recording
stays in the Agent pod until it is deleted or restarted.

Without `episode.json` (episode not played) the data is not a usable scenario and cannot be trimmed. Say so.

Replay: `dda inv anomalydetection.launch-testbench --scenarios-dir <parent of OUT> --build` (point at the directory that
contains `<episode-name>/`, not at the episode itself).

## Step 9 — Teardown

Only what the user chose in step 1:

```bash
helm uninstall <release>; helm uninstall datadog-agent       # on the VM
kind delete cluster --name gadget-dev                        # on the VM
dda inv aws.destroy-vm --stack-name=<name>                   # VMs created in step 2, from the local machine
```

## Pitfalls

| Symptom | Cause / fix |
|---|---|
| kind: permission denied on `/var/run/docker.sock` | ssh multiplexing kept the old session: `ssh -O exit <vm>` |
| `kind load docker-image` → `content digest ... not found` | Docker 29 image store: use `docker image save --platform` + `kind load image-archive` |
| Image does not start / exec format error | arm64 image on an x86_64 VM: use the amd64 variant |
| Agent probes time out, load average >> vCPUs | VM too small: resize before recording |
| kind or Agent "too many open files" | inotify limits not raised |
| Signals split across parquet directories | More than one worker node: keep one |
| ssh call that starts `play-episode.sh` never returns | Background process kept stdin: add `< /dev/null` |
| `kubectl wait -l app.kubernetes.io/instance=<release>` finds nothing | Chart pods lack that label: use `--all` |
| Recording starts before / ends after the episode | Expected: trim in step 8 |
