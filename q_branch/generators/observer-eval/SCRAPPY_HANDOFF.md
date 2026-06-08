# Scrappy live-detection in dd-agent — HANDOFF

This branch makes a custom dd-agent run the **real Scrappy inference detector** (trained
anomaly-detection model, native AVX2 engine) live inside the observer runtime against real GenSim
episodes via gs-flow, scoring the metric/log surface every tick and (when it fires) emitting Datadog
events. It's a working snapshot for testing/iteration.

**Self-contained**: `vocab.json` and `scrappy-infer` are committed under
`q_branch/generators/observer-eval/scrappy-assets/`. You do NOT need the `scrappy` repo or the
original laptop. The 140MB `model.scrappy` is fetched on-demand from the durable GAR image via
`scrappy-assets/fetch-model.sh`.

## TL;DR — fastest way to play with it (NO rebuild, weights already baked in)
The agent image on GAR contains the patched agent binary + model + vocab + native engine:
```
us-east1-docker.pkg.dev/dd-plt-simulation-environment/gensim-images/agent-dev:scrappy-detect-20260605-sc5fix
@sha256:f0da6d359b95bb3f2d8c72a56c1c853a53bf6dc489cc04746eebc77d3fa72a53
```
Run an episode through gs-flow from the generator dir (`q_branch/generators/observer-eval`):
```bash
export DD_SITE=gensim.datadoghq.com
export GENSIM_REPO_PATH=$HOME/go/src/github.com/DataDog/gensim-episodes
export DD_API_KEY=...   # GenSim org key   (DD_APP_KEY=... too)
export CLOUDSDK_CORE_ACCOUNT=gensim-integration@dd-plt-simulation-environment.iam.gserviceaccount.com
./submit.sh \
  --image us-east1-docker.pkg.dev/dd-plt-simulation-environment/gensim-images/agent-dev:scrappy-detect-20260605-sc5fix \
  --episodes "494_honeycomb_october_2017_kafka_bug:kafka-split-brain-zk-partition" \
  --mode scrappy-detect
```
Then poll `GET https://gs-flow.us1.staging.dog/api/v1/jobs/<JOB_ID>` (auth header
`-H "$(ddtool auth token sims --datacenter us1.staging.dog --http-header)"`), and on completion pull
`/logs` + `/artifacts` (the agent writes `/tmp/scrappy-scores.csv` per tick).

Detector config delivered via the entrypoint kubectl patch (override per run as needed):
`...SCRAPPY_DETECTOR_ENABLED=true`, `MODEL_PATH=/opt/scrappy/model.scrappy`,
`VOCAB_PATH=/opt/scrappy/vocab.json`, `CONTEXT_WINDOW=4096`, `TICK_WINDOW=30` (score every 30s over
the past 30s of surface), `THRESHOLD=0.1` (the value the offline evals used — see results below),
`SCORES_OUTPUT=/tmp/scrappy-scores.csv`, `EVENT_REPORTER_SENDING_ENABLED=true`; agent pod memory
limit bumped to 8192Mi.

## Model weights & native engine — in-repo layout (clone-and-go)

All three required assets live under `q_branch/generators/observer-eval/scrappy-assets/`:

| Asset | Status | Size | Notes |
|-------|--------|------|-------|
| `vocab.json` | **committed** | 59,802 B | 3594-token BPE vocab; MUST match the v0.3 model |
| `scrappy-infer` | **committed** | 55,832 B | Native AVX2/FMA linux/amd64 ELF engine |
| `model.scrappy` | **fetched on demand** | 139,883,748 B | Too large for git (no LFS); pull via script |

### Fetching model.scrappy (required before rebuild)

`model.scrappy` is NOT committed — it is 140MB and the repo has no LFS.  It is baked into the
durable GAR image at `/opt/scrappy/model.scrappy`. Fetch it with:

```bash
cd q_branch/generators/observer-eval
./scrappy-assets/fetch-model.sh
# Pulls from: us-east1-docker.pkg.dev/dd-plt-simulation-environment/gensim-images/agent-dev:scrappy-detect-20260605-sc5fix
# Expects: docker (colima context) + gcloud auth
```

The script does: `docker pull` → `docker create` → `docker cp :/opt/scrappy/model.scrappy` →
`docker rm`. Idempotent (skips if already present). Output: `scrappy-assets/model.scrappy`
(139,883,748 bytes).

Provenance: converted from checkpoint `v0.3-run-001/epoch_005.pt` via the scrappy repo conversion
tooling. Vocab 3594 ↔ v0.3 model — they MUST be kept in sync.

## Rebuilding the agent image (proven path)

From `q_branch/generators/observer-eval/`, run the self-contained build script:

```bash
./build-agent-image.sh scrappy-detect-$(date +%Y%m%d)-yourname
```

This script:
1. Fetches `model.scrappy` via `scrappy-assets/fetch-model.sh` if not already present.
2. Cross-compiles `./cmd/agent` for linux/amd64 inside `dda-linux-container-default`
   (CGO with `x86_64-linux-gnu-gcc`, tags = default-minus-python).
3. Copies the resulting binary to `scrappy-assets/agent` (gitignored).
4. Generates `scrappy-assets/Dockerfile` (gitignored).
5. Runs `docker buildx build --platform linux/amd64 ... --push` to push to GAR.

**Prereqs**: `dda env dev start` running, colima docker context active, gcloud SA
`gensim-integration@dd-plt-simulation-environment.iam.gserviceaccount.com` credentialed.

Host is arm64 (Apple Silicon); cross-compile the Go agent for linux/amd64 in the dev container.
The `dda inv ... hacky-dev-image-build` path is broken by a py3.12-vs-3.13 toolchain skew —
do NOT use it.

Manual overlay Dockerfile (if you need to rebuild manually):
```dockerfile
FROM datadog/agent:7
COPY agent         /opt/datadog-agent/bin/agent/agent
COPY scrappy-infer /opt/scrappy/scrappy-infer
COPY model.scrappy /opt/scrappy/model.scrappy
COPY vocab.json    /opt/scrappy/vocab.json
RUN chmod +x /opt/datadog-agent/bin/agent/agent /opt/scrappy/scrappy-infer
```

## Running an episode

From `q_branch/generators/observer-eval/`:
```bash
./submit.sh --image <GAR ref> --episodes <episode-id> --mode scrappy-detect
```

Example:
```bash
./submit.sh \
  --image us-east1-docker.pkg.dev/dd-plt-simulation-environment/gensim-images/agent-dev:scrappy-detect-20260605-sc5fix \
  --episodes "494_honeycomb_october_2017_kafka_bug:kafka-split-brain-zk-partition" \
  --mode scrappy-detect
```

## Key code changes in this branch (datadog-agent)
Session work (live-detection enablement):
- `comp/observer/impl/scrappy_detector.go` (NEW) — the Detector: tokenizes the surface, scores via the
  native backend, emits anomaly when P(alert) ≥ threshold. Adds `tick_window` (score every N s over the
  past N s) and a `truncated` scores-CSV column (token_count > context_window 4096).
- `pkg/config/setup/config.go` — registers `observer.components.scrappy_detector.*` config keys
  (enabled/vocab_path/model_path/threshold/context_window/scores_output/tick_window). WITHOUT this the
  detector silently never enables (this was the original "doesn't run" bug).
- `comp/observer/impl/notify.go` — events client now pins the server URL to `https://api.<site>` and
  clears `OperationServers`, so the datadog client's hardcoded `site` enum no longer rejects
  `gensim.datadoghq.com` (this silently blocked all observer event emission).
- `q_branch/generators/observer-eval/` — `entrypoint.sh` adds `scrappy-detect` mode (env patch,
  container-name-correct evidence collection, 8192Mi memory patch, episode artifact harvest),
  `agent-values.yaml.tmpl` scrappy block, `submit.sh` gs-flow auth header, `build.sh` non-fatal
  service-image build.
- `comp/observer/impl/{reporter_scrappy.go, scrappy_inference.go, scrappy_scorer.py,
  scrappy_bench_test.py, scrappy_eval_config.json}` (NEW) — reporter, native/torch inference backends,
  offline scorer + eval config.
- `q_branch/generators/observer-eval/scrappy-assets/` — `vocab.json` + `scrappy-infer` committed;
  `fetch-model.sh` pulls 140MB model from GAR; `build-agent-image.sh` + this handoff doc give a
  clone-and-go build/run experience.

Note: the branch also carries pre-existing scrappy-collector / observer WIP (component_catalog.go,
observer.go, score.go, scrappy_collector.go, scrappy_tokenizer.go, telemetry.go, testbench.go,
engine.go, scheduler.go, output.go, events.go, cmd/observer-scorer, tasks/libs/q/eval.py) that was
already in the working tree — committed here to give a complete, buildable snapshot.

## Current results (as of handoff)
- ✅ Detector loads the real model live (vocab 3594, 155 tensors, 108M params, native AVX2 backend,
  stateful SSM across ticks), scores every 30s tick on a full surface (590–735 series/tick).
- ✅ Full-context inference latency: mean ~12.8s, p95 ~17.7s per tick — within a 30s detection budget
  (a 1s tick budget is infeasible). Memory: needs ≥8Gi (startup spike → 1 OOM-retry, then stable).
- ⚠️ Detection efficacy: on cyclic-routing-flood at threshold 0.5 it did NOT fire (disruption
  p_alert max 0.118). The offline evals used **threshold 0.1** (kafka-split-brain F1 0.998,
  cyclic-routing 0.958 — scrappy `VIABILITY_REPORT.md`). A kafka-split-brain run at threshold 0.1 is
  the current test of whether offline efficacy translates live.
- Per-tick scores CSV columns:
  `timestamp,p_alert,p_normal,prediction,tick_tokens,series_count,inference_ms,salience,truncated`.

## Caveats
- All changes are a working-tree snapshot (a mix of this session's work + pre-existing branch WIP).
- gensim-episodes has 3 uncommitted kafka-episode Dockerfile tweaks (QEMU build workarounds, separate
  repo on `main`) — only needed to build kafka service images locally; the gs-flow DinD node builds
  them at run time regardless. Not pushed here.
- The forwarder logs a `app.gensim.datadoghq.com.` trailing-dot TLS warning (agent site→URL quirk);
  it does not block the events path after the notify.go fix, but is worth cleaning up.
