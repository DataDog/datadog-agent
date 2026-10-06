"""Batch replay of the Jev evaluation over many pipelines.

For each targeted pipeline: the same executor index as the live evaluation
(GitLab completed e2e jobs + the committed candidates file), the executed
tests from ONE pipeline-wide CI Visibility query (the live evaluation's
per-job queries are impractical at scale), and the shared evaluate() flow
through a data-source override. The Jev run-set is decided ONCE from the
current checkout's PR context and shared across all pipelines: the decisions
answer "what would Jev skip for THIS change", so replay is meaningful for
pipelines of the same PR/branch as the checkout - every pipeline of the
branch that ran e2e jobs is one sample of the outcome.

The comparison: per pipeline and in aggregate, the tests that executed
(with their pass/fail outcome) against Jev's predictions. A miss is a
failing executed test Jev would have skipped - the safety signal.
Occurrences are counted across pipelines: a test executed in N pipelines
has N chances to be missed.
"""

from __future__ import annotations

import json
from collections import Counter
from dataclasses import asdict, dataclass, field

from tasks.libs.ciproviders.gitlab_api import get_gitlab_api, get_pipeline
from tasks.libs.common.datadog_api import get_ci_test_events
from tasks.libs.dynamic_test.evaluator import DatadogDynTestEvaluator, EvaluationResult, executed_tests_from_events
from tasks.libs.dynamic_test.jev_selection import JevDynTestExecutor, NothingToEvaluateError, jev_run_for
from tasks.libs.dynamic_test.telemetry import ConsoleTelemetryHandler

# CI Visibility retention is about a month: older pipelines have no events
# left to replay. The e2e jobs tag their events env:nativetest; filtering on
# it keeps the query to the e2e events (an order of magnitude faster).
_QUERY_WINDOW_DAYS = 30


class BulkDatadogDynTestEvaluator(DatadogDynTestEvaluator):
    """The shared evaluation flow, with its executed-tests data source
    overridden to a pre-fetched pipeline-wide query.

    list_tests_for_job is the evaluator's abstract data-source method: one
    query per pipeline instead of one per job, the only practical shape when
    replaying many pipelines.
    """

    def __init__(self, *args, executed_by_job: dict[str, list], **kwargs):
        super().__init__(*args, **kwargs)
        self.executed_by_job = executed_by_job

    def list_tests_for_job(self, job_name: str) -> list:
        return self.executed_by_job.get(job_name, [])


@dataclass
class PipelineReplay:
    """One pipeline's replay outcome."""

    pipeline_id: str
    jobs: int = 0
    executed: int = 0
    predicted: int = 0  # candidates Jev would keep (executed or not)
    skipped_executed: int = 0  # executed tests Jev would have skipped
    misses: list[str] = field(default_factory=list)  # failing executed tests Jev would have skipped


def recent_pipeline_ids(ref: str, limit: int) -> list[str]:
    """The most recent pipeline ids of a branch, newest first."""
    project = get_gitlab_api().projects.get("DataDog/datadog-agent", lazy=True)
    return [str(p.id) for p in project.pipelines.list(ref=ref, per_page=limit, sort="desc")]


def _executed_by_job(pipeline_id: str, job_ids: set[str]) -> dict[str, list]:
    """Executed tests grouped by job (latest attempts only): one pipeline-wide query."""
    query = f"env:nativetest @ci.pipeline.name:DataDog/datadog-agent @ci.pipeline.id:{pipeline_id}"
    grouped: dict[str, list] = {}
    for test in executed_tests_from_events(get_ci_test_events(query, _QUERY_WINDOW_DAYS)):
        if test.job_id is not None and str(test.job_id) in job_ids:
            grouped.setdefault(test.job_name, []).append(test)
    return grouped


def _summarize(pipeline_id: str, results: list[EvaluationResult]) -> PipelineReplay:
    replay = PipelineReplay(pipeline_id=pipeline_id)
    for result in results:
        replay.jobs += 1
        replay.executed += len(result.actual_executed_tests)
        replay.predicted += len(result.predicted_executed_tests)
        replay.skipped_executed += len(result.actual_executed_tests - result.predicted_executed_tests)
        replay.misses.extend(sorted(result.not_executed_failing_tests))
    return replay


def replay_jev(ctx, pipeline_ids: list[str], output: str = "") -> dict:
    """Replay the Jev evaluation over the given pipelines; returns the aggregate."""
    # One executor index per pipeline (GitLab completed e2e jobs + the
    # committed candidates); pipelines without completed e2e test jobs are
    # skipped, not failures
    built = []
    skipped = []
    for pipeline_id in pipeline_ids:
        pipeline = get_pipeline("DataDog/datadog-agent", pipeline_id)
        try:
            # commit_sha = the pipeline's own SHA: this is a replay of that
            # pipeline, not a checkout-mismatch warning case
            executor = JevDynTestExecutor(ctx, pipeline.sha, pipeline_id)
            executor.init_index()
        except NothingToEvaluateError as e:
            skipped.append({"pipeline_id": pipeline_id, "reason": str(e)})
            continue
        built.append((pipeline_id, executor))

    # The Jev run-set: decided ONCE from the current checkout's PR context,
    # over the union of every pipeline's candidates
    names: set[str] = set()
    for _, executor in built:
        for job in executor.index().get_jobs():
            names |= executor.index().get_indexed_tests_for_job(job)
    run = jev_run_for(names)

    replays = []
    miss_counts: Counter[str] = Counter()
    for pipeline_id, executor in built:
        # The shared once-decided run-set: the decisions depend on the
        # checkout, not on the pipeline
        executor._run = run
        executed_by_job = _executed_by_job(pipeline_id, set(executor.job_ids.values()))
        evaluator = BulkDatadogDynTestEvaluator(
            ctx,
            executor.kind,
            executor,
            pipeline_id,
            executed_by_job=executed_by_job,
            telemetry_handler=ConsoleTelemetryHandler(),
        )
        evaluator.index = executor.index()  # normally set by initialize(); a replay sends no telemetry events
        replay = _summarize(pipeline_id, evaluator.evaluate([]))
        replays.append(replay)
        miss_counts.update(replay.misses)
        print(
            f"[replay] pipeline {pipeline_id}: {replay.jobs} jobs, {replay.executed} executed, "
            f"{replay.skipped_executed} would-skip, {len(replay.misses)} misses"
            + (f" {replay.misses}" if replay.misses else "")
        )

    executed = sum(r.executed for r in replays)
    skipped_executed = sum(r.skipped_executed for r in replays)
    summary = {
        "pipelines_evaluated": len(replays),
        "pipelines_skipped": skipped,
        "executed_test_occurrences": executed,
        "executed_would_skip": skipped_executed,
        "miss_occurrences": sum(len(r.misses) for r in replays),
        "missed_tests": dict(miss_counts.most_common()),
        "pipelines": [asdict(r) for r in replays],
    }
    print(f"[replay] {len(replays)} pipelines evaluated, {len(skipped)} skipped")
    if replays:
        pct = 100 * skipped_executed / executed if executed else 0
        print(
            f"[replay] executed: {executed} | would skip {skipped_executed} ({pct:.1f}%) | misses {summary['miss_occurrences']}"
        )
        if miss_counts:
            print("[replay] failing tests Jev would have skipped (occurrences):")
            for test, count in miss_counts.most_common():
                print(f"    {test}  {count}x")
    if output:
        with open(output, "w", encoding="utf-8") as f:
            json.dump(summary, f, indent=2)
        print(f"[replay] results -> {output}")
    return summary
