# Gensim Episode Runner — EKS Approach

## Background

[Maxime's prototype](test/e2e-framework/scenarios/aws/gensim/) proved the concept: provision ephemeral
infrastructure, deploy a gensim episode, run it autonomously so the developer can walk away.
His implementation uses **Kind-on-EC2** — a single VM running a Kind cluster inside Docker,
with a `gensim_runner.sh` bash script that:
1. Builds episode service images directly on the VM
2. Loads them into Kind via `kind load docker-image`
3. Calls `play-episode.sh` to run the episode phases
4. Collects parquet files via `docker exec` into the Kind control-plane container (kubectl workaround)
5. Uploads results to S3

This is pragmatic and works, but has limitations: single-node virtualised cluster, Kind-specific
workarounds (no kubectl on VM, `imagePullPolicy: Never`), and ~8-10 min provisioning time.

## This Work: EKS Approach

Replace Kind with a real EKS cluster. Key strategic differences:

- **No runner VM** — `play-episode.sh` runs as a Kubernetes Job inside the cluster itself,
  with native kubectl access via ServiceAccount RBAC
- **No `gensim_runner.sh`** — the Kind-specific glue script is eliminated entirely;
  `play-episode.sh` is the sole coordinator
- **ECR for images** — custom service images are built on an EC2 build VM (Amazon Linux ECS
  AMI, Docker pre-installed) and pushed to ECR via instance IAM role; the episode Helm chart's
  `imageRegistry` value handles the rest. Images are cached in ECR by content hash to skip
  rebuilds when source is unchanged.
- **Production-grade cluster** — multi-node, real cloud LBs, proper kubelet, no Kind quirks

### Key implementation detail: `imagePullPolicy: Never`

Episode Helm charts hardcode `imagePullPolicy: Never` for Kind compatibility. On EKS, images
must be pulled from ECR. We patch this transparently at deploy time via a Helm post-renderer
(a one-line `sed` script written to a temp file and passed to `helm install --post-renderer`).
This is tracked as a known issue to fix upstream in gensim-episodes.

---

## Invoke commands

```bash
# Create cluster (M1 — cluster only, no episode)
dda inv aws.eks.gensim.create

# Create cluster + deploy episode (M2+)
dda inv aws.eks.gensim.create --episode=<episode-dir-name>

# Destroy
dda inv aws.eks.gensim.destroy
```

---

## Milestones

### M1 — EKS skeleton ✅
Provision an EKS cluster and export the kubeconfig. No workloads.

**Goal:** flush out access issues (AppGate VPN, IAM, invoke task wiring).

**Files:**
- `test/e2e-framework/scenarios/aws/gensim-eks/run.go`
- `tasks/e2e_framework/aws/gensim_eks.py`
- `test/e2e-framework/registry/scenarios.go` (registered as `aws/gensim-eks`)

**Success:** `kubectl get nodes` returns healthy nodes. Destroy tears down cleanly.

---

### M2 — ECR image build/push + episode chart ✅
Build custom service images on an EC2 build VM, push to ECR, deploy the episode Helm chart.

**Goal:** confirm all episode service pods reach `Running` state on EKS with a healthy baseline.

**What happens:**
1. EC2 build VM (Amazon Linux ECS AMI) provisioned alongside the EKS cluster
2. `docker buildx bake` builds all episode service images natively on x86_64
3. Images pushed to ECR (with content-hash cache tag to skip rebuilds on unchanged source)
4. Episode Helm chart deployed with `imageRegistry=<ecr-url>`
5. `imagePullPolicy: Never` patched to `Always` via Helm post-renderer

**Key learnings during implementation:**
- EKS framework always creates Fargate nodes by default (for CoreDNS speed + sidecar injection testing). Added `WithoutFargate()` option — gensim uses it so DaemonSets schedule cleanly.
- `DependsOn` in Pulumi is ordering-only; used `Triggers` (content hash of services/) to force rebuild command re-run when source changes.
- `docker-compose` not available on ECS AMI; replaced with `docker buildx bake` which understands `docker-compose.yaml` natively and is pre-installed with Docker 25.

**Validated:**
```
dispatch_rps=6.0  actual_rps=6.0  success=100.0%  avg_resp=0.33-0.53s
```
Pool at ~60% utilization at baseline. PgBouncer `wait=0us`. svc-login all successful.

---

### M3 — Datadog Agent deployed + metrics flowing ✅
Deploy the full DaemonSet-based DD agent (with `datadog-values.yaml`), remove the episode
chart's built-in stub agent.

**Goal:** agent pod running, collecting data, stub agent removed.

**What happens:**
- `gensim-episodes/postmortems/datadog-values.yaml` applied via `WithHelmValuesFile`
  (sets `clusterName`, `kubelet.tlsVerify: false` — required on EKS, self-signed kubelet cert)
- `helm.NewKubernetesAgent` deploys the full DaemonSet-based agent, gated on `awsEnv.AgentDeploy()`
- Post-pulumi: Python invoke task runs `kubectl delete` to remove episode chart's stub agent

**Key learning:** `WithHelmValues` (string asset) has a local-backend serialisation bug in
pulumi-kubernetes that causes `unsupported type for 'valueYamlFiles' arg: []interface {}`.
Added `WithHelmValuesFile` (file asset) to `kubernetesagentparams` as the fix.

**Validated:** 1 agent pod Running on EC2 node, stub gone, agent collecting and forwarding
(403s expected with dummy API key — real key needed for metrics to flow to DD).

---

### M4 — Kubernetes Job runs play-episode.sh autonomously ✅
Create RBAC, mount `play-episode.sh` + episode YAMLs via ConfigMap, launch a Job.

**Goal:** all four phases complete without intervention. DD monitors transition Alert→OK
during disruption.

**What happens:**
- ServiceAccount + ClusterRole (scale deployments, get/list pods) created
- Secret with DD credentials created
- ConfigMap with `play-episode.sh` + `episodes/<scenario>.yaml` created
- Job runs `alpine/k8s` image, mounts ConfigMap at `/episode/`, executes `play-episode.sh`
- `dda inv aws.eks.gensim.create` returns as soon as Job is confirmed running

**Key learnings during implementation:**
- `kubectl scale` uses `patch` not `update` on `deployments/scale`. ClusterRole needs both verbs; missing `patch` fails silently due to `|| true` in play-episode.sh.
- Jobs are immutable in Kubernetes — template spec changes require delete + recreate, not in-place update.
- ConfigMap volumes are read-only; play-episode.sh hardcodes `RESULTS_DIR="${SCRIPT_DIR}/results"` so an `emptyDir` volume must be mounted at `/episode/results`.

**Validated end-to-end (2026-03-05):**
- Warmup 3m → baseline OK → disruption: surge×5 scaled up → pgbouncer monitor Alert at 510s → cooldown → monitor OK at 540s → result JSON written
- Total runtime: ~33 minutes, pod status: `Succeeded`

**Gap exposed → M5:** result JSON written to emptyDir is lost when pod completes. S3 upload required before exit.

---

### M5 — S3 upload + destroy + full parity ⬜
Complete the artifact pipeline and harden the workflow.

**Goal:** end-to-end identical observable outcome to the Kind path — results in S3, cluster gone.

**What happens:**
- Job uploads results zip to S3 (via EKS node IAM role — same pattern as Kind's EC2 role)
- `destroy` tears down EKS cluster + ECR repos
- Graceful handling of episodes without `docker-compose.yaml` (public images only)
- Stack naming convention matches existing gensim pattern

**Success:** `s3://qbranch-gensim-recordings/gensim-results-<episode>-<date>.zip` exists.
Cluster destroyed. Ready to hand off to junior engineers.
