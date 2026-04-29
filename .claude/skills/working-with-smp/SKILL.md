---
name: working-with-smp
description: Run, observe, and triage Single-Machine-Performance (SMP) regression detector experiments for datadog-agent. Use when the user wants to validate a perf/memory/CPU change against SMP, iterate on a branch with multiple SMP runs, bump a comparison image tag, manually trigger the gitlab dev_branch-full job, or use Datadog's profiling MCP to find allocation/heap hotspots from an SMP run. Triggers include "run SMP", "next SMP iteration", "bump COMPARISON_TAG", "trigger dev_branch-full", "profile the SMP run", and "diff the comparison vs baseline allocs".
---

# Working with SMP / Regression Detector

The SMP regression detector runs the agent under controlled workloads (lading-driven log/metric/trace/etc. generators), measures CPU + RSS + a few other resource metrics, and posts a baseline-vs-comparison report on the PR. Its bounds-checks (memory, CPU, intake_connections, etc.) are how we gate observer work that touches hot paths.

There are **two delivery paths** that look superficially the same but have very different mechanics. Pick the right one before doing anything else.

---

## Decision: which SMP path do I want?

| Situation | Use |
|---|---|
| Branch is on or near current `main`, and you want SMP to gate the PR automatically. | **Agent native SMP CI** (default, no extra steps). |
| Branch's merge-base is too old for SMP's baseline image to exist in ECR. CI prints "Merge-base of this branch is too old for SMP". | **smp-playground**. |
| The image under test is fixed (e.g. published `agent-dev:foo-full`) and you want to **vary the experiment**: tweak `lading.yaml`, `datadog.yaml`, bounds, replica count, etc. | **smp-playground** — it skips the image build entirely and goes straight to a run against whatever `COMPARISON_TAG` says. |
| You want to compare two arbitrary published images. | **smp-playground** — set `BASELINE_TAG` and `COMPARISON_TAG` to the two images. |

Native CI does the *whole* loop (build image → publish → run SMP → comment). smp-playground does the *back half* (run SMP → comment) against an image you've already published. The skill below covers both because most observer work right now uses smp-playground (long-lived dev branch, old merge-base).

---

## Path A — Agent native SMP CI

Implemented by `.gitlab/test/functional_test/regression_detector.yml` (the merge-base check) plus `.gitlab/childs/smp-regression-child-pipeline.yml` (the runs themselves). Triggered automatically on every PR push.

1. Push your branch / open PR. CI builds the image as part of normal pipeline (`docker_build_agent7_full[_arm64]`).
2. `single_machine_performance-regression_detector-merge_base_check` runs on the parent pipeline, finds a baseline SHA, validates the baseline image exists in ECR.
3. If valid → child pipeline runs the experiments. Result lands as a PR comment summarising bounds-checks pass/fail and an optimization-goal Δ%.

If `merge_base_check` fails with **"Merge-base of this branch is too old for SMP"**, your branch is too far behind `main` for SMP to find a baseline image in ECR. Either rebase onto current `main` (preferred when feasible) or switch to Path B.

---

## Path B — smp-playground (manual image, manual loop)

This is the iterate-fast path used during observer perf reduction work. The loop is: **edit code → build agent-dev image → bump COMPARISON_TAG in smp-playground PR → SMP run posts comment → triage with profiling MCP → repeat**.

### B.1 Push agent code, get an image tag

```bash
cd ~/dd/beta-datadog-agent
# Pre-push hook runs go-test which is slow; --no-verify is typical here
git push --no-verify origin-https HEAD:<branch>
SHORT_SHA=$(git rev-parse --short=8 HEAD)   # e.g. 863d8749
```

The agent gitlab pipeline starts on push. Wait for `docker_build_agent7_full` and `docker_build_agent7_full_arm64` to be SUCCESS. Poll via:

```bash
gh pr view <PR> --repo DataDog/datadog-agent --json statusCheckRollup \
  --jq '.statusCheckRollup[] | select(.context != null and (.context | test("docker_build_agent7_full(_arm64)?$"))) | {context, state}'
```

Both arches must finish before `dev_branch-full` becomes triggerable.

### B.2 Trigger `dev_branch-full` (the manual deploy job)

This is the gitlab job that publishes `agent-dev:<branch-slug>-<short-sha>-full` to Docker Hub. It is **manual** (`rules: !reference [.manual]`) so won't run by itself.

```bash
TOKEN=$(ddtool auth gitlab token | tail -1)
PIPELINE_ID=$(gh pr view <PR> --repo DataDog/datadog-agent --json statusCheckRollup \
  --jq '.statusCheckRollup[] | select(.context == "dd-gitlab/default-pipeline") | .targetUrl' \
  | grep -oE '[0-9]+$')

# Find the dev_branch-full job ID in this pipeline
JOB_ID=$(curl -s --header "Authorization: Bearer $TOKEN" \
  "https://gitlab.ddbuild.io/api/v4/projects/DataDog%2Fdatadog-agent/pipelines/${PIPELINE_ID}/jobs?per_page=100" \
  | python3 -c "import json,sys
for j in json.load(sys.stdin):
    if j.get('name') == 'dev_branch-full':
        print(j['id']); break")

# Trigger it (status will go manual → pending → running → success in ~2 min)
curl -s -X POST --header "Authorization: Bearer $TOKEN" \
  "https://gitlab.ddbuild.io/api/v4/projects/DataDog%2Fdatadog-agent/jobs/${JOB_ID}/play" \
  | python3 -m json.tool | head
```

The token is a GitLab JWT — use `Authorization: Bearer …`, **not** `PRIVATE-TOKEN: …` (which returns 401).

### B.3 Wait for the image on Docker Hub

```bash
TAG="agent-dev:<branch-slug>-${SHORT_SHA}-full"   # e.g. sopell-q-branch-observer-allocs-f15c3499-full
while true; do
  hub=$(curl -sI -o /dev/null -w '%{http_code}' \
    "https://hub.docker.com/v2/repositories/datadog/${TAG/:/\/tags/}/")
  echo "$(date +%H:%M:%S) hub=$hub"
  [ "$hub" = "200" ] && break
  sleep 60
done
```

Image typically lands ~2 min after `dev_branch-full` starts running.

### B.4 Bump COMPARISON_TAG in smp-playground

The smp-playground PR's `regression-action` workflow re-runs SMP every time the PR is updated. So a one-line bump = a new SMP run.

```bash
cd /home/bits/dd/smp-playground   # (or your worktree of it)
sed -i "s/^COMPARISON_TAG=.*/COMPARISON_TAG=\"<branch-slug>-${SHORT_SHA}-full\"/" \
  experiments/regression/agent/regression.env
git diff experiments/regression/agent/regression.env   # sanity check
git -c user.email=YOU@datadoghq.com -c user.name='you' commit -am "Repoint COMPARISON_TAG at <change> image (${SHORT_SHA})"
git push origin-https HEAD
```

### B.5 Wait for the SMP run to post a comment

The `regression-action` GitHub Action posts a single PR comment summarising:

- bounds-checks pass/fail per experiment
- Δ% on the optimization goal vs baseline
- a Datadog dashboard link with `tpl_var_run-id[0]=<JOB_ID>` — **save this `JOB_ID`**, it's how you scope every profiling MCP call.

Typical run takes ~20–30 minutes. Read the comment with `gh pr view <smp-playground PR> --comments`.

### B.6 Triage with the Datadog profiling MCP

Once you have the `JOB_ID`, use the profiling MCP tools (`datadog-mcp` family — `Datadog_profiling_*` etc.) to find what's still leaking. Confirmed working calls:

| Goal | Profile type | Notes |
|---|---|---|
| Find what's still **alive in heap** at end of run (residual memory). | `go-heap-live-size` | Best for "where did the 466 MiB go?" |
| Find allocation **hotness** (rate of allocs). | `go-alloc-size` | Best for hot-path allocation churn. |
| Allocation rate weighted by **lifetime**. | `go-alloc-size-lifetime` | Useful for spotting moderately-hot allocs that don't die. |

Scope every query with the SMP run filter:

```
env:single-machine-performance service:datadog-agent job_id:<JOB_ID>
```

Add `variant:comparison` or `variant:baseline` as a tag filter to compare the two arms of the same run. **Flame-graph diff endpoint works reliably**; the timeseries endpoint with group-by-package returns empty for some profile types (try without `family:go` or fall back to flame-graph diffs).

The flame-graph output groups by `my-code` / standard library / runtime — focus on `my-code` for things you can actually fix. From there, walk down to the hottest packages and cross-reference the source files.

### B.7 Iterate

Edit code → commit → goto B.1. Each loop is ~30–45 minutes wall time, mostly waiting for the agent build and SMP run.

---

## Common pitfalls

| Symptom | Cause | Fix |
|---|---|---|
| `git push` from `~/dd/beta-datadog-agent` hangs forever. | The pre-push hook runs `go-test` (slow). | Use `git push --no-verify` for these dev branches. SSH agent-forwarding is also flaky in non-interactive shells — use `origin-https`. |
| GitLab API returns `{"message":"401 Unauthorized"}`. | Wrong header. The `ddtool` token is a JWT. | Use `Authorization: Bearer <token>`, not `PRIVATE-TOKEN: <token>`. |
| Docker Hub still 404 long after pipeline went green. | `dev_branch-full` is **manual** and was never triggered. | See B.2 — find the job ID in the pipeline's `jobs` API and POST to `/play`. |
| smp-playground PR comment never appears. | `COMPARISON_TAG` references an image that doesn't exist (typo or the `dev_branch-full` job hasn't published yet). | Confirm `curl -sI https://hub.docker.com/v2/repositories/datadog/agent-dev/tags/<tag>/` returns 200 before pushing the COMPARISON_TAG bump. |
| Native SMP fails with "Merge-base of this branch is too old". | Branch's merge-base is older than the oldest baseline image still in ECR. | Either rebase onto current `main`, or switch to Path B (smp-playground). |
| Profiling MCP timeseries endpoint returns no data. | Some profile types don't return a meaningful series with `family:go` group-by-package. | Drop the family filter, or stick with flame-graph diffs which are reliable. |
| Two consecutive SMP runs on the same `COMPARISON_TAG` give different Δ%. | Inherent variance — SMP uses 5 replicas per arm by default, but small effects fall inside noise. | Increase `NUM_REPLICAS` in `regression.env`, or trust the trend across multiple commits rather than any single Δ%. |
| Pipeline shows several FAILED checks unrelated to your change (`lint_components`, `bazel:run-go-mod-tidy`, etc.). | Long-lived dev branches drift on lint/dep checks that gate `main` but not the dev image build. | Usually fine to ignore for SMP iteration purposes; they don't block `dev_branch-full`. Fix before merging. |

---

## Reference

- **Agent SMP config**: `.gitlab/test/functional_test/regression_detector.yml`, `.gitlab/childs/smp-regression-child-pipeline.yml`
- **smp-playground**: <https://github.com/DataDog/smp-playground> — `experiments/regression/agent/regression.env` is the file you edit. `cases/<name>/{lading,datadog-agent,experiment.yaml}` define each experiment.
- **dev_branch-full job spec**: `.gitlab/deploy/dev_container_deploy/docker_linux.yml` (look for `dev_branch-full:`)
- **SMP wiki**: <https://datadoghq.atlassian.net/wiki/spaces/agent/pages/6589153525/Running+SMP+Experiments>
- **Quality gate observer budget**: <https://datadoghq.atlassian.net/wiki/spaces/agent/pages/...> — the 370 MiB / 500 mc bounds for `quality_gate_logs`-class experiments.

## When NOT to use this skill

- For a one-off check that the PR doesn't regress SMP and the branch is fresh: just rely on the native CI path (Path A) — the skill's smp-playground machinery is overkill.
- For benchmarking *outside* the lading-driven workloads: SMP isn't the right tool. See `tasks/bench.py` and the `bench_*_test.go` files in the relevant package.
