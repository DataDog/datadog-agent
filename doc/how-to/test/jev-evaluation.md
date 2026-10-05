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

- `JevDynTestExecutor` uses the shared GitLab client to paginate the selected
  pipeline's completed jobs. The resolved GitLab configuration and existing
  matrix expansion helper supply each job's `TARGETS` and `EXTRA_PARAMS`.
- A normal in-memory `DynamicTestIndex` records the candidate root tests per job.
  It is independent of coverage data, including for new tests. Cleanup/unit-test
  jobs are excluded. Root `--run`/`--skip` filters and nested suite paths are
  honored; a subtest-only skip does not exclude the whole root suite.
- The shared evaluator queries the latest job attempt. Skipped tests are not
  counted as executed. Flaky failures and allow-failure jobs are not critical
  misses. The `index_kind:jev` tag identifies Jev metrics.
- Only explicit, valid Jev skip decisions remove tests. Transport, authentication,
  timeout or parsing failures run the affected tests. Duplicate bare test names
  run conservatively if any occurrence should run.

The reported miss count concerns **observed executions**, not tests that the
coverage selector already skipped or jobs that never ran. Zero observed misses
is not proof that skipping tests is safe. Use a pipeline that ran the full E2E
suite when comparing selector recall. Candidate discovery is source-based rather
than build-tag/runtime-aware, so predicted counts can include tests that the
particular platform or runtime setup would skip.

## Regression tests

```bash
dda inv invoke-unit-tests.run --tests=dyntest,jev_selection,jev_tools,evaluator
```
