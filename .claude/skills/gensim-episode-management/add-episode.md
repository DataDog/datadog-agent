# Adding a Gensim Episode to gensim-eks

Step-by-step process for taking an existing episode from the gensim-episodes repo
and running it on the gensim-eks cluster with the observer agent.

## Step 1: Locate and Validate the Episode

### Find it

Episodes live in `synthetics/` (hand-crafted) or `postmortems/` (incident-derived):

```bash
cd ~/dev/gensim-episodes
ls synthetics/$EPISODE 2>/dev/null || ls postmortems/$EPISODE 2>/dev/null
```

If it was deleted, check git history:
```bash
git log --all --diff-filter=D --name-only -- "*$EPISODE*" | head -20
```

Recover with `git checkout <commit-before-deletion>~1 -- <path>`.

### Find the scenario name

```bash
ls gensim-episodes/*/EPISODE/episodes/
# filename without .yaml is the scenario name
```

### Run pre-flight checks

Check all of these before submitting:

| Check | Command | Fix |
|-------|---------|-----|
| Internal registry refs | `grep -rl "registry.ddbuild.io" */EPISODE/services/` | `sed -i '' 's\|registry.ddbuild.io/images/mirror/\|\|g'` |
| Runtime docker build | `grep -E "docker.*build\|compose.*build\|kind load" */EPISODE/play-episode.sh` | **Blocker** -- needs refactoring |
| Port-forward usage | `grep "port-forward" */EPISODE/play-episode.sh` | Works from orchestrator pod, just note it |
| Python requests in exec | `grep "import requests" */EPISODE/play-episode.sh` | Target pod needs requests lib |
| Dirty gensim checkout | `git status --porcelain` | Commit changes (push not required) |

### Check the ground truth

Read `scenario.yaml` for `true_positives` and `false_positives`. Note which metrics
are expected to change and which should stay stable. You'll validate these after the
run completes.

Remember the trace naming mismatch: scenario.yaml uses backend names
(`trace.redis.command.errors`) but the observer uses flat names
(`trace.errors{operation:redis.command}`). See SKILL.md for the translation.

## Step 2: Submit the Run

### Basic command

```bash
dda inv aws.eks.gensim.submit \
  --image=docker.io/datadog/agent-dev:<tag> \
  --episodes=<episode>:<scenario> \
  --mode=live-and-record \
  --s3-bucket=qbranch-gensim-recordings
```

### With --skip-build (cached images)

When images are already in ECR from a previous run, or when another user created
the Pulumi stack:

```bash
dda inv aws.eks.gensim.submit \
  --image=docker.io/datadog/agent-dev:<tag> \
  --episodes=<episode>:<scenario> \
  --mode=live-and-record \
  --s3-bucket=qbranch-gensim-recordings \
  --skip-build
```

`--skip-build` skips the build VM but still passes the ECR registry to helm charts.

### Batch runs

Comma-separate multiple `episode:scenario` pairs. They run sequentially, ~33 min each:

```bash
--episodes="ep1:scen1,ep2:scen2,ep3:scen3"
```

### Mode reference

| Mode | Analysis | Recording | Use case |
|------|----------|-----------|----------|
| `record-parquet` | No | Yes | Offline testbench evaluation |
| `live-anomaly-detection` | Yes | No | Live demo |
| `live-and-record` | Yes | Yes | Evaluation (recommended) |

## Step 3: Monitor the Run

### Check status

```bash
KUBECONFIG=<kubeconfig> kubectl --context aws \
  get configmap gensim-run-status -n default \
  -o jsonpath='{.data.status}' | python3 -m json.tool
```

### Expected timeline (per episode)

| Offset | Phase |
|--------|-------|
| +0 min | episode-install (helm chart + agent) |
| +2-4 min | episode-running: warmup (2 min) |
| +4-6 min | baseline (10 min) |
| +14-16 min | disruption (10 min) |
| +16-18 min | first observer anomaly events |
| +24-26 min | cooldown (10 min) |
| +34-36 min | episode done, parquet collection |

### Check events during the run

```bash
pup events search --query 'source:agent-q-branch-observer' --from 30m
```

If zero events 5+ minutes into disruption, check the orchestrator logs
(see `debug-infra.md`).

## Step 4: Validate Results

### 4.1 Monitors

Verify each episode's monitors transitioned OK -> Alert -> OK:

```bash
pup monitors list --name 'gensim'
```

Look for monitors tagged with the episode name. They should be in OK state
(recovered after cooldown).

### 4.2 Observer Events

Count events and check which metrics were detected:

```bash
pup events search --query 'source:agent-q-branch-observer' --from '<start>' --to '<end>'
```

Parse the event messages to extract detected metric names. Cross-reference against
the episode's `scenario.yaml` TP/FP lists, applying the trace metric name translation.

### 4.3 Metric Signal Verification

For each TP metric in scenario.yaml, query Datadog to confirm the disruption signal
is actually present:

```bash
pup metrics query --query 'avg:<metric>{env:<episode-env-tag>}' \
  --from '<baseline-start>' --to '<cooldown-end>'
```

The env tag follows the pattern `gensim-<episode-name-lowercased-hyphenated>`.

Check that:
- TP metrics show clear change during disruption window
- FP metrics remain stable throughout

### 4.4 Parquet Recordings

If mode was `live-and-record` or `record-parquet`:

```bash
aws-vault exec sso-agent-sandbox-account-admin -- \
  aws s3 ls s3://qbranch-gensim-recordings/ | grep <episode>
```

The zip contains four parquet types:
- `observer-metrics-*.parquet` -- check metrics, DogStatsD (NOT trace-derived metrics)
- `observer-trace-stats-*.parquet` -- pre-aggregated trace stats (source for trace metrics)
- `observer-traces-*.parquet` -- raw spans
- `observer-logs-*.parquet` -- log observations

**Heads up**: Derived trace metrics (`trace.hits`, `trace.duration`, etc.) are NOT in
the metrics parquet. They live in engine memory only, derived from trace-stats at
runtime. The trace-stats parquet is the source of truth -- testbench replay re-derives
them via `processStatsView`.

## Checklist Summary

Before submitting:
- [ ] Episode exists with play-episode.sh, chart/, docker-compose.yaml
- [ ] Scenario name identified from episodes/*.yaml
- [ ] No `registry.ddbuild.io` refs in Dockerfiles
- [ ] No runtime `docker compose build` in play-episode.sh
- [ ] Gensim-episodes checkout is committed (not dirty)

After completion:
- [ ] Run status shows `done` for all episodes
- [ ] Monitors created and transitioned OK -> Alert -> OK
- [ ] Observer events fired during disruption window
- [ ] TP metrics show expected signal in Datadog
- [ ] FP metrics remain stable
- [ ] Parquet uploaded to S3
