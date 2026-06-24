# orchestrator-starvation

A self-contained kind + fakeintake lab that exercises the Datadog cluster-checks runner (CLC) and the orchestrator-explorer pipeline, and measures whether high cluster-check load on the CLC dispatches the orchestrator check late.

The original hypothesis was: a customer running ~100 Prometheus-HTTP-SD-generated cluster checks per CLC pod sees the orchestrator check getting starved of dispatch slots and shipping its manifest payloads late. This lab reproduces that scenario end-to-end and instruments the orchestrator check's actual dispatch cadence.

## TL;DR finding

**The orchestrator check goes dark in short windows because of cluster-checks-runner *rebalancing*, not worker saturation.** When the dispatcher rebalances a check between runners, the destination runner’s `clcRunnerStats` is empty for that check until the first post-move `Run()` completes, so the next call to `currentDistribution()` sees `WorkersNeeded = 0` for that runner and treats the check as a free win to move again. On every rebalance tick the orchestrator check is therefore an attractive target: it gets shuffled, its stats reset, the next rebalance moves it again, and the manifest pipeline produces ABSENT samples in the gap between schedule and first-Run. See `runs/queue-starvation-confirmed-*/FINDINGS.md` and `runs/rebalance-stats-derived-*/FINDINGS.md`.

Reproduction recipe: 3 CLCRs × `DD_CHECK_RUNNERS=1`, `--prom-endpoints 200`, `rebalance_period=5s`, `min_percentage_improvement=1`. Result: ~70% ABSENT in the orchestrator-pipeline payload stream, ~13–29s gap between schedule and first Run, repeated cancel/reschedule storm on every rebalance tick.

The SME-suggested fix (`orchestratorExplorer.conf.cluster_check: false` — run orch on the DCA directly instead of on a CLCR) is A/B-validated in `runs/sme-fix-validation-*/FINDINGS.md`: 0% ABSENT, 0 cancels, ~10s cadence.

Worker saturation (`DD_CHECK_RUNNERS` sweep) is *orthogonal but compounding*: as exec_time per check rises, `WorkersNeeded` pegs at the 1.0 cap, which makes orch the only movable check on the cluster and feeds the rebalance loop. See `runs/check-runners-sweep-*/FINDINGS.md`.

Under *low* load the original negative finding still holds: with 1 CLCR and modest noise, the orchestrator check holds its 15s `min_collection_interval` to within 10ms. The lab covers both regimes.

## Architecture

A single-node `kind` cluster running:

- **datadog-operator** (chart 2.20.0 / app 1.24.0 by default).
- **DatadogAgent** CR with the node agent, cluster agent, and cluster-checks runner(s) all configured. The CR is built per-run from a Python template in this script.
- **fakeintake** as the metrics + orchestrator-payload sink (port-forwarded for offline querying).
- **Noise generator**: a single `slow-metrics` Deployment plus N prometheus-annotated `Service` objects in the `noise` namespace; each Service spawns an `openmetrics` cluster check via prometheus-HTTP-SD that takes ~5–10s per scrape attempt and always fails (the Service has no backing Pod). N is set by `--prom-endpoints`.
- **Workload pods**: optional `pause` containers in the `workload` namespace, used to stress the *node-agent* `orchestrator_pod` check via the kubelet (not the CLC). Sized by `--workload-pods`.
- **Workload Deployments**: optional N standalone tiny `pause` Deployments, used to stress the *CLC* orchestrator check via the apiserver. Sized by `--workload-deployments`.
- **OOTB CRDs + workload CRs**: optional installation of the 13 builtin CRDs covered by `DD_ORCHESTRATOR_EXPLORER_CUSTOM_RESOURCES_OOTB_ENABLED` (Argo, Flux, Karpenter) plus N seeded `GitRepository` CRs, used to stress the orchestrator check’s custom-resource collectors. Sized by `--ootb-crds` and `--workload-crs`.

A few kind specifics matter:

- Pod CIDR is widened to `10.244.0.0/20` (4094 IPs) and `maxPods` is bumped to 2000 so a hundreds-of-pods workload can fit on the single control-plane node.
- The DatadogAgent CR sets `spec.global.kubelet.tlsVerify: false` because kind’s kubelet serves its API with a self-signed cert that the agent’s default kubelet client rejects. Without this, the node-agent `orchestrator_pod` check errors with *“impossible to reach Kubelet”* and per-Pod data never enters the orchestrator pipeline.

### What runs where

The orchestrator pipeline is split across two agents and you need to be precise about which one you’re asking about:

| Check | Lives on | Inputs | What `--workload-*` knob scales it |
|---|---|---|---|
| `orchestrator` (cluster-scoped) | cluster-checks runner | Deployments, ReplicaSets, Services, Nodes, Namespaces, CRDs/CRs, … | `--workload-deployments`, `--workload-crs`, `--ootb-crds` |
| `orchestrator_pod` (per-node) | node agent (via kubelet) | Pods on this node | `--workload-pods` |

If you’re trying to reproduce “CLC noise starves the orchestrator check” — the original lab thesis — only the first row matters. `--workload-pods` is for the node-agent path.

## Subcommands

```
orchestrator-starvation.py up [--prom-endpoints N] [--workload-pods N]
                              [--workload-deployments N] [--workload-crs N]
                              [--check-runners default|N] [--clcr-replicas N]
                              [--ootb-crds]
                              [--agent-image …] [--cluster-agent-image …]

orchestrator-starvation.py observe [--window 5m] [--interval 30s] [--duration …]

orchestrator-starvation.py sweep [--settle 90s] [--window 3m]
                                 [--observe-interval 20s] [--observe-duration 3m]
                                 [--runs-dir runs]

orchestrator-starvation.py report --manifest runs/sweep-<ts>-manifest.json
                                  [--output report.html]

orchestrator-starvation.py down
```

### `up`
Brings the whole stack to the configured cell state. Idempotent — will reuse an existing kind cluster + operator + fakeintake. Order matters: noise services, then OOTB CRDs (if requested), then the DatadogAgent CR, then `--workload-pods`, then `--workload-deployments`, then `--workload-crs`. CRDs go in before the DDA because the agent’s builtin-CRD discovery runs once at check-bundle init and silently drops collectors whose CRD isn’t present — there’s no retry.

### `observe`
Streams a CSV of orchestrator-pipeline activity, computed from fakeintake’s recorded payloads.

Columns:

| column | meaning |
|---|---|
| `timestamp`, `window_seconds` | Unix epoch of this sample, plus the rolling-window length used to compute the others. |
| `orchmanif_payloads_in_window` | total payloads received on `/api/v2/orchmanif` in the window. |
| `orchmanif_batches_in_window` | count of distinct 15s buckets that contain ≥ 1 payload. Coarse legacy metric, kept for back-compat — caps low and can’t catch <2× dispatch slip. |
| `orch_payloads_in_window` | total payloads on `/api/v2/orch` in the window. |
| `total_orchmanif_payloads`, `total_orch_payloads` | running lifetime totals. |
| `manif_batches` | distinct orchestrator-check Run()s detected in the window (gap-clustered, see below). |
| `manif_gap_p50_s`, `manif_gap_p95_s`, `manif_gap_max_s` | distribution of gaps **between consecutive Run()s** on the manifest pipeline, seconds. |
| `coll_batches`, `coll_gap_p50_s` … `coll_gap_max_s` | same, for the `/api/v2/orch` collector pipeline. |

**Read the gap columns.** They are the direct dispatch-cadence signal:

- p50 close to the check’s `min_collection_interval` (15s for manifests, ~10s for the collector pipeline) → dispatch is on time.
- p50 or p95 inflating above target → the orchestrator check is being dispatched late.

A “batch” is the empirical fingerprint of one Run(): the check ships its 3+ per-resource manifest payloads in rapid succession (within ~1s), then is silent until the next dispatch. We cluster timestamps with a 4s gap threshold so occasional back-pressured payloads inside one Run don’t fragment the batch, but normal idle cadence (≥10s) is clearly separated.

### `sweep`
Runs a matrix of `(prom_endpoints, workload_deployments, workload_crs, clcr_replicas, check_runners, ootb_crds)` cells, reconfiguring the live cluster between cells and capturing one observe CSV per cell. Writes `runs/sweep-<ts>-cell-NN-<label>.csv` for each cell plus a `runs/sweep-<ts>-manifest.json` index that the `report` command consumes.

The default matrix is hard-coded near the top of `cmd_sweep` and targeted at the negative result above (idle → noise-100 → noise-300 → saturated 1-worker). Edit `DEFAULT_SWEEP_CELLS` to vary other dimensions.

### `report`
Renders a sweep manifest + its CSVs into a single self-contained HTML file with one summary table and a pair of inline-SVG timeseries (manifest pipeline + collector pipeline gap distribution) per cell. No external JS or asset dependencies.

### `down`
`kind delete cluster --name orch-starve`. Brutal but fast.

## Quickstart

```bash
# Bring up a realistic loaded config (~5 min).
./orchestrator-starvation.py up \
  --prom-endpoints 100 \
  --workload-deployments 100 \
  --workload-crs 500 \
  --ootb-crds

# Watch dispatch cadence for 4 minutes.
./orchestrator-starvation.py observe --window 3m --interval 20s --duration 4m \
  > runs/manual-loaded.csv

# Or: sweep the negative-result matrix and render a report (~25 min total).
./orchestrator-starvation.py sweep
./orchestrator-starvation.py report --manifest runs/sweep-<ts>-manifest.json --output report.html

# Tear down.
./orchestrator-starvation.py down
```

## fakeintake decode notes

Modern Datadog agents (v7.x and the `nightly-full-main` dev tags this lab pulls by default) POST orchestrator data to two routes:

- `/api/v2/orch` — collector summaries.
- `/api/v2/orchmanif` — per-resource manifests.

Both are zstd-compressed protobuf (agent-payload v5) wrapped in a 16-byte framing header. fakeintake’s built-in JSON decode aggregator only knows the legacy `/api/v1/orchestrator` route, which current agents no longer hit, so `format=json` returns null for v2 endpoints. We work around this by counting raw payloads and clustering their timestamps into batches — enough for cadence analysis without needing to decode individual resource types.

If you want decoded payloads, you’ll need to add the v2 routes to fakeintake’s aggregator or post-process the raw `/fakeintake/payloads?endpoint=…` output offline with the agent-payload protobuf descriptors.

## Caveats

- The 100–300 prometheus-annotated Services are intentionally backed by *no* Pods. The `openmetrics` cluster check that prometheus-HTTP-SD generates for each Service therefore always fails with `connection refused`, which is the worst case for per-check exec time (each attempt takes the full TCP-connect timeout). This is closer to the customer scenario than a fast successful scrape would be — in production, the noise is endpoints that go intermittently unreachable, not endpoints that respond quickly.
- The lab’s default agent images are dev tags (`agent-dev:nightly-full-main-jmx`, `cluster-agent-dev:master`). Pin them with `--agent-image` / `--cluster-agent-image` if you need a stable reproduction across days.
- `--ootb-crds` installs 13 CRDs but seeds *no* controllers for them. The CRs exist purely as stored objects so the orchestrator check’s informers have non-empty caches to walk. Don’t expect Argo / Flux / Karpenter to do anything useful in this lab.
- Some OOTB collectors are knowingly skipped: `karpenter.azure.com/*` and `eks.amazonaws.com/nodeclasses` have no off-the-shelf standalone CRD manifest, and the Datadog-group CRDs not shipped by the 2.20.0 operator chart (`datadogslos`, `datadogdashboards`, `datadogagentprofiles`, `datadogpodautoscalerclusterprofiles`) aren’t installed by this script. The runner logs “no supported version found” for those at startup. The remaining ~13 collectors carry the rest.

## File layout

```
orchestrator-starvation-lab/
  orchestrator-starvation.py    — the whole lab (PEP-723 single-file uv script)
  README.md                     — this file
  runs/                         — per-run observe CSVs, sweep manifests/reports, and per-run FINDINGS.md
```
