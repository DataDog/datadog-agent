# Evaluate Jev E2E test selection

Jev is an evaluation-only alternative to the coverage-based dynamic-test selector.
It does **not** change which tests CI runs. Both selectors use
`DatadogDynTestEvaluator` for executed-test lookup, comparisons and metrics.

## Run locally

From the repository root, use a pipeline with completed E2E jobs whose full SHA
matches the checked-out `HEAD`. Keep the E2E sources and CI configuration unchanged
from that commit. Fetch the comparison branch if necessary.

Required access:

- `DD_API_KEY` and `DD_APP_KEY` for the Datadog organization holding Agent CI
  Visibility data (org 2 in CI), with permission to read test events. Set `DD_SITE`
  if not using `datadoghq.com`.
- The standard GitLab task authentication (the same as other `dda inv` GitLab
  tasks), and access to the internal GitLab/AI Gateway endpoints.
- Preinstalled `authanywhere` for the AI Gateway's `rapid-ai-platform` audience.
  This task does not install it. `AI_GATEWAY_TOKEN`, or `JEV_TOKEN_CMD` and
  `JEV_DC`, can override token acquisition/datacenter.
- Optional `GITHUB_TOKEN` with repository read access, to include the PR title
  and description. The source diff is still available without it.

```bash
dda inv dyntest.evaluate-index \
  --selector=jev \
  --pipeline-id=<completed-pipeline-id-for-HEAD> \
  --no-send-stats
```

The task defaults `--commit-sha` to `HEAD` and validates it against the pipeline.
No S3 bucket or AWS credentials are needed for Jev. `--no-send-stats` suppresses
both evaluation metrics and Datadog events; it still reads CI Visibility and
calls Jev. Jev sends the PR diff and test source to the internal AI Gateway.
The existing `dda` CLI may still emit its own command telemetry.

The executed-tests queries run over the last three days: evaluate a pipeline
no older than that. An empty evaluation fails rather than publishing
misleading zero-miss statistics.

For an input-only preview, without GitLab, CI Visibility or Jev requests:

```bash
dda run i python -c 'from tasks.libs.dynamic_test.jev.jev_e2e_selector import select_suite; \
    select_suite("fleet", test="TestFleetConfig", dry_run=True)'
```

There is no command-line wrapper: the selector is a library called in-process
by the evaluation.
This preview can still read GitHub/DDCI PR metadata when configured.

## Integration and interpretation

- `JevDynTestExecutor` is a plain `DynTestExecutor`: its index is a static
  job -> candidate tests map loaded from a file committed in the repo
  (`tasks/libs/dynamic_test/jev/job_test_candidates.json`), restricted to the
  pipeline's completed e2e jobs (GitLab API, latest attempts). The coverage
  executors load their index from S3; this one loads its own from Git. Jobs
  absent from the file are reported and not evaluated - never a failure.
- The shared `DatadogDynTestEvaluator` runs the Jev executor like any
  other, with no Jev-specific knowledge: it owns the executed-tests
  queries (CI Visibility, pipeline- and job-scoped - no env facet: the e2e
  jobs tag their events `env:nativetest` while the go test jobs use the
  default), marks flaky failures unreliable straight from the events (as
  the coverage evaluation always has - allow-failure jobs count like every
  other job), and computes misses with the same logic. The `index_kind:jev`
  tag identifies Jev metrics.
- Only explicit, valid Jev skip decisions remove tests. Transport, authentication,
  timeout or parsing failures run the affected tests. Duplicate bare test names
  run conservatively if any occurrence should run.
- The candidates file does not need to track CI changes. Regenerate it with
  `dda inv dyntest.generate-jev-job-index --pipeline-id=<id> [--pipeline-id ...]`
  (one CI Visibility query per pipeline, unions the executed tests per job);
  pick pipelines where the e2e jobs ran the widest for the most complete map.
  A test the coverage selection skipped in every generation pipeline stays
  missing from its job's candidates.

The reported miss count concerns **observed executions**. Zero observed misses
is not proof that skipping tests is safe. Use a pipeline that ran the full E2E
suite when comparing selector recall. The per-job universe is the job's
static candidates, so `predicted` includes runnable-but-not-executed tests
(visible as `+` lines: over-selection, and the Jev-vs-coverage disagreement
on what the coverage selection skipped).

## Regression tests

```bash
dda inv invoke-unit-tests.run --tests=dyntest,jev_selection,jev_tools,evaluator
```
