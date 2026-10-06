"""Batch replay of the Jev evaluation over many pipelines.

Branch-independent: from ANY checkout, each targeted pipeline is replayed with
the decisions computed from ITS OWN commit - a temporary git worktree checked
out at the pipeline's SHA provides the PR context (diff vs the merge base,
test sources, suite definitions) that the selector reads; the current
checkout only provides the tooling and the committed candidates file.

Per pipeline: the same executor index as the live evaluation (GitLab
completed e2e jobs + the committed candidates file), the executed tests from
ONE pipeline-wide CI Visibility query (the live evaluation's per-job queries
are impractical at scale), and the shared evaluate() flow through a
data-source override. Pipelines without completed e2e test jobs, or whose
commit can no longer be fetched, are skipped with a report, never failures.

The comparison: per pipeline and in aggregate, the tests that executed (with
their pass/fail outcome) against Jev's predictions. A miss is a failing
executed test Jev would have skipped - the safety signal. Occurrences are
counted across pipelines: a test executed in N pipelines has N chances to be
missed.
"""

from __future__ import annotations

import json
import os
import subprocess
import tempfile
from collections import Counter
from contextlib import contextmanager
from dataclasses import asdict, dataclass, field
from pathlib import Path

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


def _git(*args: str, cwd: str | None = None) -> str:
    res = subprocess.run(["git", *args], capture_output=True, text=True, cwd=cwd, timeout=300)
    if res.returncode != 0:
        raise RuntimeError(f"git {' '.join(args)} failed: {res.stderr.strip()}")
    return res.stdout.strip()


def _commit_present(sha: str) -> bool:
    return (
        subprocess.run(["git", "cat-file", "-e", f"{sha}^{{commit}}"], capture_output=True, timeout=60).returncode == 0
    )


def _fetch_pipeline_commit(pipeline) -> None:
    """Fetch the pipeline's commit: by its ref first (the branch tip contains
    the commit unless history was rewritten), by its SHA as a fallback."""
    last_error = ""
    for ref in (pipeline.ref, pipeline.sha):
        try:
            _git("fetch", "origin", ref)
        except RuntimeError as e:
            last_error = str(e)
            continue
        if _commit_present(pipeline.sha):
            return
    raise RuntimeError(
        f"cannot fetch commit {pipeline.sha[:12]} ({last_error or 'not reachable from the fetched refs'})"
    )


@contextmanager
def _worktree():
    """A throwaway worktree, checked out per replayed pipeline's commit.

    One worktree for the whole replay (a full checkout per pipeline would be
    wasteful): created detached at HEAD, then 'git checkout --detach <sha>'
    before each pipeline's selection.
    """
    base = tempfile.mkdtemp(prefix="jev-replay-worktree-")
    path = os.path.join(base, "checkout")
    created = False
    try:
        _git("worktree", "add", "--detach", path)
        created = True
        yield Path(path)
    finally:
        if created:
            _git("worktree", "remove", "--force", path)
        _git("worktree", "prune")
        os.rmdir(base)


@contextmanager
def _selector_context(worktree: Path, ref: str):
    """Run the selector from the worktree, as the pipeline's own CI would:

    - the process CWD moves into the worktree: the selector's file reads
      (test sources, suite definitions) and git commands (the PR diff vs the
      merge base) all target the pipeline's commit
    - CI_COMMIT_REF_NAME is set to the pipeline's ref: a detached worktree
      cannot tell its branch to git, and the PR lookup needs it (the same
      mechanism the selector uses in real CI)
    """
    previous_cwd = os.getcwd()
    previous_ref_name = os.environ.get("CI_COMMIT_REF_NAME")
    os.chdir(worktree)
    os.environ["CI_COMMIT_REF_NAME"] = ref
    try:
        yield
    finally:
        os.chdir(previous_cwd)
        if previous_ref_name is None:
            os.environ.pop("CI_COMMIT_REF_NAME", None)
        else:
            os.environ["CI_COMMIT_REF_NAME"] = previous_ref_name


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
    """Replay the Jev evaluation over the given pipelines; returns the aggregate.

    Works from any checkout: each pipeline's Jev decisions are computed from
    its own commit, inside a worktree checked out at the pipeline's SHA.
    """
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
        built.append((pipeline, executor))

    replays = []
    miss_counts: Counter[str] = Counter()
    # The run-set cache for this replay: the same commit deciding the same
    # candidate set always produces the same decisions (retried pipelines)
    run_sets: dict[tuple[str, tuple[str, ...]], set[str]] = {}
    with _worktree() as worktree:
        for pipeline, executor in built:
            try:
                if not _commit_present(pipeline.sha):
                    _fetch_pipeline_commit(pipeline)
                # The Jev decisions for THIS pipeline's commit: the selector
                # runs from the worktree checked out at the pipeline's SHA,
                # over the union of the pipeline jobs' candidates
                names: set[str] = set()
                for job in executor.index().get_jobs():
                    names |= executor.index().get_indexed_tests_for_job(job)
                cache_key = (pipeline.sha, tuple(sorted(names)))
                run = run_sets.get(cache_key)
                if run is None:
                    _git("checkout", "--detach", "--force", pipeline.sha, cwd=str(worktree))
                    with _selector_context(worktree, pipeline.ref):
                        run = jev_run_for(names, root=worktree)
                    run_sets[cache_key] = run
                executor._run = run  # the decisions of this pipeline's commit

                executed_by_job = _executed_by_job(str(pipeline.id), set(executor.job_ids.values()))
                evaluator = BulkDatadogDynTestEvaluator(
                    ctx,
                    executor.kind,
                    executor,
                    str(pipeline.id),
                    executed_by_job=executed_by_job,
                    telemetry_handler=ConsoleTelemetryHandler(),
                )
                # Normally set by initialize(); a replay sends no telemetry events
                evaluator.index = executor.index()
                replay = _summarize(str(pipeline.id), evaluator.evaluate([]))
            except RuntimeError as e:
                skipped.append({"pipeline_id": str(pipeline.id), "reason": str(e)})
                continue
            replays.append(replay)
            miss_counts.update(replay.misses)
            print(
                f"[replay] pipeline {pipeline.id} (at {pipeline.sha[:12]}, {pipeline.ref}): "
                f"{replay.jobs} jobs, {replay.executed} executed, "
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
            f"[replay] executed: {executed} | would skip {skipped_executed} ({pct:.1f}%)"
            f" | misses {summary['miss_occurrences']}"
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
