# GenSim EKS Evaluator - Executive Summary

## Requirements Summary

The observer team evaluates agent builds by running gensim episodes --
realistic incident scenarios -- on EKS. Each run produces two outputs:
parquet data for offline testbench replay/scoring, and live anomaly
detection behavior verified against monitors. A developer submits an image
and episode list; the system handles infrastructure, runs episodes serially
with clean isolation, uploads results to S3 with version metadata (agent
image, gensim SHA, episode), and reports to Datadog. Weekly automation is
planned but blocked on private repo access for gensim-episodes.

## Technical Summary

A persistent EKS cluster (Pulumi) hosts a single-tenant orchestrator Job.
For each episode: deploy agent via Helm with observer-recorder config,
install episode chart with post-renderer, run `play-episode.sh`, collect
parquet via `kubectl cp`, upload to S3 at
`<image-tag>/<episode>/<gensim-sha>/<date>/`, emit DD events + metrics,
then tear down agent + episode for clean isolation. Invoke tasks on the
developer's laptop handle cluster lifecycle (idempotent), submission, and
status polling. Currently laptop-only; target is in-cluster autonomy via
episode container images in ECR.

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-GE-001:** Submit an Evaluation Run | 🟢 Implemented | `inv aws.eks.gensim.submit` with multi-episode support, implicit cluster create, run ID, busy-cluster guard. |
| **REQ-GE-002:** Check Evaluation Status | 🟢 Implemented | `inv aws.eks.gensim.status` reads gensim-run-status ConfigMap. |
| **REQ-GE-003:** Serial Episode Execution | 🟢 Implemented | Orchestrator Job loops episodes serially. Submit guard prevents concurrent runs. |
| **REQ-GE-004:** Clean Isolation Between Episodes | 🟢 Implemented | helm uninstall agent + episode between iterations; wait for pod termination. |
| **REQ-GE-005:** Collect and Upload Results | 🟢 Implemented | Parquet collection + S3 upload with path convention `<image-tag>/<episode--scenario>/<gensim-sha>/<date>/`. |
| **REQ-GE-006:** Tag Runs with Version Metadata | 🟢 Implemented | gensim SHA captured from clean checkout, included in S3 paths, DD events, DD metric tags. |
| **REQ-GE-007:** Report Run Metadata to Datadog | 🟢 Implemented | DD events + metrics (duration_seconds, parquet_files) emitted per episode. |
| **REQ-GE-008:** Cluster Lifecycle | 🟢 Implemented | Persistent cluster, idempotent submit, `inv aws.eks.gensim.destroy`. |
| **REQ-GE-009:** Weekly Automated Evaluation | ❌ Blocked | Blocked: private repo access for gensim-episodes. Target: episode container image in ECR. |
| **REQ-GE-010:** Scoring Integration | ❌ Not Started | Deferred until testbench scoring and observer event emission are ready. |

**Progress:** 8 of 10 implemented, 1 blocked, 1 deferred
