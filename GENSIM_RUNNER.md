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
- **ECR for images** — custom service images are built locally and pushed to ECR;
  the episode Helm chart's `imageRegistry` value handles the rest
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

### M2 — ECR image build/push + episode chart 🔄 (in progress)
Build custom service images locally, push to ECR, deploy the episode Helm chart to the cluster.

**Goal:** confirm all episode service pods reach `Running` state on EKS.

**What happens:**
1. `docker-compose build` in the episode directory
2. ECR repos created (idempotent), images tagged and pushed
3. Pulumi deploys the episode Helm chart with `imageRegistry=<ecr-url>`
4. `imagePullPolicy: Never` patched to `IfNotPresent` via Helm post-renderer

**Success:** `kubectl get pods -A` shows all episode services `Running`.
`load-generator-surge` at `0/0` replicas is correct (scaled up during disruption).

---

### M3 — Datadog Agent deployed + metrics flowing ⬜
Deploy the full DaemonSet-based DD agent (with `datadog-values.yaml`), remove the episode
chart's built-in stub agent.

**Goal:** agent pod running, metrics visible in DD under the episode's env tag.

**What happens:**
- `gensim-episodes/postmortems/datadog-values.yaml` applied (sets `clusterName`, `kubelet.tlsVerify: false`)
- Episode chart's built-in `datadog-agent` Deployment/Service/ServiceAccount deleted
- Full agent DaemonSet takes over

**Success:** DD metrics flowing for the episode's env tag.

---

### M4 — Kubernetes Job runs play-episode.sh autonomously ⬜
Create RBAC, mount `play-episode.sh` + episode YAMLs via ConfigMap, launch a Job.

**Goal:** all four phases complete without intervention. DD monitors transition Alert→OK
during disruption.

**What happens:**
- ServiceAccount + ClusterRole (scale deployments, get/list pods) created
- Secret with DD credentials created
- ConfigMap with `play-episode.sh` + `episodes/<scenario>.yaml` created
- Job runs `alpine/k8s` image, mounts ConfigMap at `/episode/`, executes `play-episode.sh`
- `dda inv aws.eks.gensim.create` returns as soon as Job is confirmed running

**Success:** `kubectl logs -f job/gensim-runner` shows warmup → baseline → disruption → cooldown.
Result JSON written.

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
