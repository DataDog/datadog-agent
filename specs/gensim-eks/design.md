# GenSim EKS Evaluator - Technical Design

## Architecture Overview

Two layers: a persistent EKS cluster (Pulumi-managed) and an in-cluster
orchestrator Job that executes episode queues serially. Invoke tasks on the
developer's laptop are thin clients.

```
Developer laptop                    EKS Cluster
+---------------------+            +----------------------------------+
| inv gensim.submit   |--Pulumi--->| Persistent:                      |
|   --image=X         |  kubectl   |   EC2 node group                 |
|   --episodes=A:a,.. |            |   S3 IAM policy                  |
|                     |            |   RBAC (runner SA)               |
| inv gensim.status   |--kubectl-->|                                  |
|                     |            | Per-evaluation:                  |
| inv gensim.destroy  |--Pulumi--->|   Orchestrator Job               |
+---------------------+            |     for each episode:            |
                                   |       helm install agent         |
                                   |       helm install episode chart |
                                   |       play-episode.sh            |
                                   |       kubectl cp parquet         |
                                   |       aws s3 cp to S3            |
                                   |       curl DD events + metrics   |
                                   |       helm uninstall episode     |
                                   |       helm uninstall agent       |
                                   +----------------------------------+
```

## REQ-GE-001 / REQ-GE-008: Cluster Lifecycle + Submit

The `submit` command replaces the current separate `create` + manual run:

1. Check if Pulumi stack `gensim-eks` exists and is healthy (kubeconfig
   on disk, `kubectl cluster-info` succeeds).
2. If not, run `pulumi up` (idempotent -- skips existing resources).
   Persistent layer only: cluster, node group, IAM, RBAC. No agent, no
   episode chart.
3. Check for an existing `gensim-orchestrator` Job in Running state. If
   found, refuse the submission and report the active run's status.
4. Create the orchestrator Job with the submitted image and episode list.
5. Return the run ID (format: `eval-<date>-<short-hash>`).

**Invoke tasks:**
- `inv aws.eks.gensim.submit --image=X --episodes=A:a,B:b`
- `inv aws.eks.gensim.status`
- `inv aws.eks.gensim.destroy`

**Location:** `tasks/e2e_framework/aws/gensim_eks.py`

### Persistent vs Per-Evaluation Resources

| Persistent (Pulumi stack) | Per-evaluation (orchestrator) |
|---------------------------|-------------------------------|
| EKS cluster + EC2 nodes | Agent DaemonSet (helm) |
| S3 upload IAM policy | Episode Helm chart |
| Runner ServiceAccount + RBAC | Runner/orchestrator Job |
| Cluster security groups | Episode ConfigMaps/Secrets |

## REQ-GE-002: Status Reporting

The orchestrator updates a ConfigMap `gensim-run-status` with JSON after
each phase transition:

```json
{
  "runId": "eval-20260306-abc123",
  "image": "docker.io/datadog/agent-dev:my-branch",
  "gensimSha": "abc1234def5",
  "episodes": [
    {"episode": "authcore-pgbouncer", "scenario": "pool-saturation",
     "status": "done", "parquetFiles": 230, "durationSeconds": 1980},
    {"episode": "episode-two", "scenario": "scenario-a",
     "status": "running", "phase": "disruption"},
    {"episode": "episode-three", "scenario": "scenario-b",
     "status": "queued"}
  ]
}
```

The `status` invoke task reads this ConfigMap via kubectl and renders it.

**Location:** `tasks/e2e_framework/aws/gensim_eks.py` (status task),
orchestrator script (ConfigMap updates)

## REQ-GE-003 / REQ-GE-004: Serial Execution + Isolation

Single-tenant by design: the submit task checks for an existing
`gensim-orchestrator` Job before creating a new one. If one is active,
the submission is refused with the active run's status. No queueing --
a small team with weekly cadence doesn't need it.

The orchestrator is a single Kubernetes Job that loops through the episode
list. For each episode:

1. `helm install` the Datadog chart with the submitted image +
   observer.recording.* config (same values as current run.go).
2. Wait for agent DaemonSet to be Ready (3/3 containers).
3. `helm install` episode chart with post-renderer (strip stub agent,
   patch imagePullPolicy).
4. Execute `play-episode.sh run-episode <scenario>`.
5. Collect parquet from agent pod via `kubectl cp`.
6. Upload results + parquet to S3 (REQ-GE-005).
7. Emit DD event + metrics (REQ-GE-007).
8. Update status ConfigMap.
9. `helm uninstall` episode chart.
10. `helm uninstall` agent chart. Wait for all pods to terminate.
11. Proceed to next episode.

Single-tenancy: Kubernetes Jobs are singleton by name. The submit task
checks for existing `gensim-orchestrator` Job before creating a new one.

The orchestrator image is `alpine/k8s` (kubectl, helm, bash, curl, jq).
AWS CLI is needed for S3 upload -- either pre-installed or added via
`apk add`.

**Location:** `test/e2e-framework/scenarios/aws/gensim-eks/run.go`
(orchestrator Job spec), orchestrator bash script (ConfigMap-mounted)

## REQ-GE-005: S3 Upload + Path Convention

S3 path: `s3://<bucket>/<image-tag>/<episode--scenario>/<gensim-sha>/<date>/`

Example:
```
s3://qbranch-gensim-recordings/
  q-branch-observer-full/
    authcore-pgbouncer--pool-saturation/
      gensim-abc1234/
        20260306/
          pool-saturation-1.json
          parquet/
            observer-metrics-20260306-201431Z.parquet
            observer-traces-20260306-201431Z.parquet
            observer-logs-20260306-201431Z.parquet
            ...
```

Parquet source: `/tmp/observer-parquet` on the agent pod (label
`app=dda-linux-datadog`).

Error handling: log ERROR to stderr if parquet collection fails, report
file count on success, continue to next episode either way.

**Location:** Orchestrator script (bash)

## REQ-GE-006: Version Metadata

Three coordinates per run:
- **Agent image tag**: from `--image` flag (e.g. `q-branch-observer-full`)
- **gensim-episodes SHA**: from `git rev-parse HEAD` in the gensim-episodes
  checkout on the developer's laptop. Passed to the orchestrator as an env
  var.
- **Episode:scenario**: from `--episodes` flag

Currently the gensim-episodes checkout lives on the developer's laptop. The
submit task validates it's a clean git working tree (`git status --porcelain`
is empty) and records the SHA. Episode charts and play-episode.sh are
shipped to the cluster as ConfigMaps (current mechanism).

Future: gensim-episodes packaged as a container image in ECR, eliminating
the laptop dependency and unblocking REQ-GE-009.

**Location:** `tasks/e2e_framework/aws/gensim_eks.py`

## REQ-GE-007: Datadog Reporting

**Events:** POST to `https://api.datadoghq.com/api/v1/events`:
- `title`: `gensim: <episode>/<scenario> <outcome>`
- `text`: Duration, alert detection time, parquet count, S3 path
- `tags`: `episode:<name>`, `scenario:<name>`, `image:<tag>`,
  `gensim_sha:<sha>`, `run_id:<id>`
- `alert_type`: `success` or `error`

**Metrics:** POST to `https://api.datadoghq.com/api/v1/series`:
- `gensim.episode.duration_seconds` (gauge)
- `gensim.episode.alert_detection_seconds` (gauge)
- `gensim.episode.parquet_files` (gauge)
- Tags: same as events

Uses `DD_API_KEY` from the existing `gensim-secrets` Secret. Emitted via
`curl` from the orchestrator script.

**Location:** Orchestrator script (bash)

## REQ-GE-009: Weekly Automation (Blocked)

**Blocker:** gensim-episodes is a private repo. The orchestrator running
in-cluster cannot clone it without credentials.

**Target solution:** CI pipeline builds a gensim-episodes container image
and pushes to ECR. The orchestrator pulls episode charts and scripts from
this image instead of receiving them via ConfigMap from the developer's
laptop. This decouples git access from the cluster.

Until solved, weekly automation requires a developer to run `submit` from
their laptop with the nightly image tag.

**Location:** Future CI pipeline + `tasks/e2e_framework/aws/gensim_eks.py`

## Key Technical Decisions

1. **Orchestrator as a Job, not a controller**: Simpler than a CRD+operator.
   Runs once per evaluation, loops through episodes, exits. No
   reconciliation loops or state machines.

2. **Helm for per-episode agent lifecycle**: Reusing the framework's Helm
   chart (with deep merge for ExtraValues) ensures identical agent config
   to what we've validated. Install/uninstall per episode gives clean
   isolation.

3. **ConfigMap for status**: Lightweight, no external dependencies. Swap to
   a CRD later if richer queuing is needed.

4. **Bash orchestrator**: Keeps it debuggable -- helm, kubectl, curl are
   the primitives. Serial execution makes error handling straightforward.

5. **Laptop-first, cluster-autonomous later**: Ship the working capture
   pipeline now with laptop-based submission. Solve private repo access
   (episode container image) as a follow-up to unblock full automation.
