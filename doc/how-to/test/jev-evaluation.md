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
  --lookback-days=7 \
  --no-send-stats
```

The task defaults `--commit-sha` to `HEAD` and validates it against the pipeline.
No S3 bucket or AWS credentials are needed for Jev. `--no-send-stats` suppresses
both evaluation metrics and Datadog events; it still reads CI Visibility and
calls Jev. Jev sends the PR diff and test source to the internal AI Gateway.
The existing `dda` CLI may still emit its own command telemetry.

Use `--test-env` to override `nativetest` (Jev's default, matching the E2E template).
The coverage selector retains its existing `prod` default. The default query
window is three days; increase `--lookback-days` when evaluating an older pipeline,
subject to CI Visibility retention. An empty evaluation fails rather than
publishing misleading zero-miss statistics.

For an input-only preview, without GitLab, CI Visibility or Jev requests:

```bash
dda run i python -m tasks.libs.dynamic_test.jev.jev_e2e_selector \
  --suite=fleet --test=TestFleetConfig --dry-run
```

This preview can still read GitHub/DDCI PR metadata when configured.

## Integration and interpretation

- `JevDynTestExecutor` loads the pipeline's completed (success/failed) E2E jobs
  from the shared GitLab client (each status queried separately, all pages).
  No CI-configuration parsing is involved.
- The decidable universe is the E2E test filetree (`test/new-e2e/tests`), and
  each job's universe is the tests that actually executed in it (CI
  Visibility). Cleanup/unit-test jobs without executions are not evaluated;
  executed tests that are not filetree entry points are not decidable.
- `JevDynTestEvaluator` shares the executed-test queries, flaky/allow-failure
  handling, miss logic and telemetry with the coverage evaluation. The
  `index_kind:jev` tag identifies Jev metrics.
- Only explicit, valid Jev skip decisions remove tests. Transport, authentication,
  timeout or parsing failures run the affected tests. Duplicate bare test names
  run conservatively if any occurrence should run.

The reported miss count concerns **observed executions**. Zero observed misses
is not proof that skipping tests is safe. Use a pipeline that ran the full E2E
suite when comparing selector recall. The per-job efficiency is measured over
executed tests (i.e., on top of the coverage `--impacted` selection), and
suites are asked for Jev decisions only when one of their tests executed.

## Regression tests

```bash
dda inv invoke-unit-tests.run --tests=dyntest,jev_selection,jev_tools,evaluator
```
