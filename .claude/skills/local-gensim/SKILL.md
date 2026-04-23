---
name: local-gensim
description: >
  Running gensim episodes locally on Kind clusters for observer evaluation.
  Use this skill when the user wants to run episodes locally instead of on EKS,
  set up a Kind cluster for testing, collect parquet recordings, or monitor
  local runs. Trigger on mentions of "local episode", "kind cluster",
  "run-local-episode", "local gensim", "food-delivery-redis locally",
  "test observer changes locally", or wanting to evaluate observer without EKS.
  Complements the gensim-episode-management skill (which covers EKS runs).
---

# Local Gensim Episode Runner

Run gensim episodes on a local Kind cluster instead of EKS. Same episode
format, same parquet output, compatible with `q.eval-scenarios`.

## Quick start

```bash
dda inv q.run-local-episode \
  --episode=food-delivery-redis \
  --image=datadog/agent-dev:my-branch-tag \
  --mode=live-and-record
```

## Prerequisites

- `kind`, `kubectl`, `helm`, `docker` installed and in PATH
- DD API + app keys in environment. The task checks `DDDEV_API_KEY` /
  `DDDEV_APP_KEY` first (org-specific convention), then falls back to
  `DD_API_KEY` / `DD_APP_KEY` (standard). The app key must be in `ddapp_*`
  format — UUID-format keys will fail monitor creation with "Unauthorized".
- gensim-episodes repo at `~/dd/gensim-episodes` (or set `--gensim-path`)
- Agent image — either CI-built (`datadog/agent-dev:branch-tag`) or local
  (`dda inv agent.hacky-dev-image-build --trace-agent --target-image observer-agent`)

## Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--episode` | `food-delivery-redis` | Episode name |
| `--scenario` | auto from episode | Scenario file name (without .yaml) |
| `--image` | required | Agent Docker image |
| `--mode` | `live-and-record` | `live-and-record`, `record-parquet`, or `live-anomaly-detection` |
| `--skip-build` | false | Skip docker compose build + kind load (images already in cluster) |
| `--skip-teardown` | false | Leave episode + agent running after completion |
| `--cluster-name` | `observer-local` | Kind cluster name |
| `--output-dir` | `comp/observer/scenarios/<name>/parquet` | Where to save parquets |
| `--gensim-path` | `~/dd/gensim-episodes` | Path to gensim-episodes repo |

## What it does (7 phases)

1. **Ensure Kind cluster** — auto-creates if missing, reuses if exists
2. **Build/load images** — `docker compose build` for episode services, `kind load` each + agent image
3. **Create K8s secrets** — `datadog-secret` from DD credentials
4. **Install episode chart** — Helm install (services, Redis, Postgres, etc.)
5. **Install Datadog Agent** — Helm install with observer config (HF checks, recording, analysis)
6. **Run episode** — `play-episode.sh run-episode <scenario>` (~34 min: warmup → baseline → disruption → cooldown)
7. **Collect parquet** — `kubectl cp` from agent pod to output dir

## Monitoring a run

```bash
# TUI with mode picker (probes local Kind + remote EKS)
uv run q_branch/gensim-status.py

# Or skip picker, go straight to local
uv run q_branch/gensim-status.py --local
```

The TUI shows episode status, phase, pod health, and tails the episode log.

## After completion

```bash
# Copy episode.json (ground truth timestamps) if not already there
cp ~/dd/gensim-episodes/synthetics/food-delivery-redis-cpu-saturation/results/redis-cpu-saturation-1.json \
   comp/observer/scenarios/food_delivery_redis/episode.json

# Optionally compact parquets (reduces disk usage ~98%)
dda inv q.compact-parquets --scenario=food_delivery_redis

# Score
dda inv q.eval-scenarios --scenario=food_delivery_redis
```

## Fastest iteration (images already loaded)

```bash
dda inv q.run-local-episode \
  --episode=food-delivery-redis \
  --image=observer-agent:latest \
  --mode=live-and-record \
  --skip-build
```

## Available episodes

The canonical episode corpus is in `q_branch/gensim-eval-scenarios.json`
(added in #48607, merged to main). It lists episode/scenario pairs with
pinned tree SHAs for reproducibility. For EKS runs, use
`--episode-manifest=./q_branch/gensim-eval-scenarios.json`.

For the local runner, the `_EPISODE_PATHS` dict in `tasks/q.py` maps short
names to directory paths within the gensim-episodes repo. Add new episodes
there. Episode directories live under `~/dd/gensim-episodes/synthetics/`
and `~/dd/gensim-episodes/postmortems/`, each containing `play-episode.sh`,
`chart/`, `episodes/*.yaml`, and optionally `docker-compose.yaml`.

## Known issues

### dd_url doesn't route event platform traffic

Setting `dd_url` only routes metrics. Container lifecycle (`contlcycle`),
container image, and log event platform pipelines use separate endpoints.
Workaround: explicit per-pipeline `logs_dd_url` overrides in agent config.
Root cause tracked in DataDog/datadog-agent#48814.

### App key format

The app key must be in `ddapp_*` format. If monitor creation fails with
"Unauthorized", check which key format is in your environment. Stale tmux
sessions may have an old UUID-format key cached — re-export with the
correct value.

### Python output buffering

When piping through `tee`, Python's stdout buffering can make logs appear
stuck at "Synchronizing dependencies". The task is actually running — verify
via `kubectl get pods` or `ps aux | grep play-episode`.

### Cleanup on failure

Phases 3-7 are wrapped in try/finally. If a phase fails, previously-installed
Helm releases are cleaned up automatically (unless `--skip-teardown`).

### Cleanup

```bash
kind delete cluster --name observer-local
```

## Related skills

- **gensim-episode-management** — for EKS-based runs
- **observer-eval** — for scoring results after collection
- **inspect-agent-egress-traffic** — for proxy-dumper egress inspection
