"""Jev-based (System One) E2E test selection for the dynamic-test evaluation.

The decidable universe is the E2E test filetree (test/new-e2e/tests), and each
evaluated job's universe is the tests that actually executed in it (CI
Visibility). No CI-configuration or coverage data is involved: reconstructing
per-job TARGETS/EXTRA_PARAMS from the CI config is fragile (rule evaluation is
context-dependent, the lint endpoint alphabetizes matrix variables) and can
never cover jobs generated outside the repo config. Only explicit, successful
Jev skip decisions may remove tests; errors and unknown tests run
conservatively.
"""

from __future__ import annotations

import os
import time
from pathlib import Path

from tasks.libs.ciproviders.gitlab_api import get_pipeline
from tasks.libs.dynamic_test.evaluator import DatadogDynTestEvaluator, EvaluationResult
from tasks.libs.dynamic_test.executor import DynTestExecutor
from tasks.libs.dynamic_test.index import IndexKind
from tasks.libs.dynamic_test.jev.jev_e2e_selector import select_suite
from tasks.libs.dynamic_test.jev.test_discovery import E2E_TESTS_DIR, list_suites

_REPO_ROOT = Path(__file__).resolve().parents[3]


class NothingToEvaluateError(RuntimeError):
    """The pipeline has no completed E2E test jobs to evaluate (not an error)."""


def jev_selection(suite: str) -> dict:
    """Run the Jev selector for a suite in-process; return {} on any failure.

    Jev calls run concurrently inside the selector, with a per-request timeout.
    Any failure fails open to run all the suite's tests.
    """
    try:
        summary = select_suite(
            suite,
            dc=os.environ.get("JEV_DC", "us1.ddbuild.io"),
            token_cmd=os.environ.get("JEV_TOKEN_CMD"),
        )
        if not isinstance(summary, dict) or not all(
            isinstance(summary.get(key), list) and all(isinstance(name, str) for name in summary[key])
            for key in ("run", "skip")
        ):
            raise ValueError("invalid selector summary")
        decisions = summary.get("decisions", [])
        if not isinstance(decisions, list) or not all(isinstance(row, dict) for row in decisions):
            raise ValueError("invalid selector decisions")
        errors = [row["error"] for row in decisions if "error" in row]
        if errors:
            print(f"[jev] {suite}: {len(errors)} decisions failed open; first error: {errors[0]}")
        return summary
    except Exception as e:
        print(f"[jev] selection failed for {suite}, running all its tests: {e}")
        return {}


def suite_entry_points() -> dict[str, set[str]]:
    """Suite name -> its test entry points, discovered from the filetree."""
    root = _REPO_ROOT / E2E_TESTS_DIR
    return {
        suite: {name for name, _, _ in list_suites(str(root / suite))}
        for suite in sorted(os.listdir(root))
        if (root / suite).is_dir()
    }


def all_entry_points() -> set[str]:
    """Every E2E test entry point under test/new-e2e/tests, across all suites."""
    entries = set()
    for names in suite_entry_points().values():
        entries |= names
    print(f"[jev] e2e test universe: {len(entries)} entry points across all suites")
    return entries


class JevTestUniverse:
    """Index-like object: the completed E2E jobs of the evaluated pipeline.

    Per-job test universes are NOT defined here: JevDynTestEvaluator restricts
    each job to the tests that actually executed in it. The base
    DynTestEvaluator.evaluate() cannot run against this index.
    """

    def __init__(self, jobs: list[str]):
        self.jobs = list(jobs)


class JevDynTestExecutor(DynTestExecutor):
    """Loads the pipeline's completed E2E jobs and computes the Jev run-set.

    - jobs: every completed (success/failed) new-e2e job of the pipeline, with
      the latest-attempt job id and the allow-failure flag
    - entry_points(): every E2E test entry point in the filetree (the decidable
      universe)
    - jev_run(names): the Jev run-set for the suites containing `names`; the
      selector gathers its own PR context from this checkout
    """

    def __init__(self, ctx, commit_sha: str, pipeline_id: str, require_pipeline_commit: bool = True):
        super().__init__(ctx, None, IndexKind.JEV, commit_sha)
        self.pipeline_id = pipeline_id
        # False (local experiments, --ignore-sha-mismatch): allow evaluating a
        # pipeline whose commit differs from the checkout - the Jev decisions
        # are then computed from the current checkout's PR context.
        self.require_pipeline_commit = require_pipeline_commit
        self.jobs: list[str] = []
        self.job_ids: dict[str, str] = {}
        self.unreliable_jobs: set[str] = set()
        self._entry_points: set[str] | None = None

    def init_index(self):
        pipeline = get_pipeline("DataDog/datadog-agent", self.pipeline_id)
        if pipeline.sha != self.commit_sha:
            if self.require_pipeline_commit:
                raise RuntimeError(
                    f"Pipeline {self.pipeline_id} ran {pipeline.sha}, but the checkout is at {self.commit_sha}. "
                    "The Jev selection is computed from the pipeline commit's PR context: either check out that "
                    f"commit (git checkout {pipeline.sha}) or evaluate the pipeline of the current HEAD, or pass "
                    "--ignore-sha-mismatch to decide from the current checkout's context instead. "
                    "Note the evaluation code also comes from the checkout, so old pipelines run their old "
                    "evaluation code."
                )
            print(
                f"[jev] WARNING: pipeline {self.pipeline_id} ran {pipeline.sha}, but the checkout is at "
                f"{self.commit_sha}: the Jev decisions will be computed from the current checkout's PR "
                "context, not the pipeline's commit (--ignore-sha-mismatch)"
            )
        jobs: list = []
        # python-gitlab collapses list-valued query params (scope=["success",
        # "failed"] reaches the API as a single scope), so query each status
        # separately; iterator=True walks every page.
        for scope in ("success", "failed"):
            jobs.extend(pipeline.jobs.list(scope=scope, iterator=True))
        jobs = [job for job in jobs if job.name.startswith("new-e2e")]
        if not jobs:
            raise NothingToEvaluateError(f"No completed E2E jobs in pipeline {self.pipeline_id}")
        self.jobs = [job.name for job in jobs]
        self.job_ids = {job.name: str(job.id) for job in jobs}
        self.unreliable_jobs = {job.name for job in jobs if job.allow_failure}
        self._index = JevTestUniverse(self.jobs)
        print(f"[jev] universe: {len(self.jobs)} completed E2E jobs in pipeline {self.pipeline_id}")

    def entry_points(self) -> set[str]:
        if self._entry_points is None:
            self._entry_points = all_entry_points()
        return self._entry_points

    def jev_run(self, names: set[str]) -> set[str]:
        """The entry points Jev would RUN, decided via the suites containing `names`.

        Only suites with something to decide are evaluated. Tests the selector
        did not decide about (missing or failed-open decisions) run, and a bare
        name occurring in several suites runs if any occurrence runs.
        """
        suites = {suite: entries for suite, entries in suite_entry_points().items() if entries & names}
        print(f"[jev] suites to decide: {', '.join(sorted(suites))}")
        run: set[str] = set()
        for suite, entries in sorted(suites.items()):
            summary = jev_selection(suite)
            skip = set(summary.get("skip", [])) - set(summary.get("run", []))
            run.update(entries - skip)
            print(f"[jev] {suite}: {len(entries - skip)} run / {len(entries & skip)} skip")
        return run


class JevDynTestEvaluator(DatadogDynTestEvaluator):
    """Evaluates the Jev selection with executed tests as the per-job universe."""

    def evaluate(self, changes: list[str]) -> list[EvaluationResult]:
        # changes are ignored: the Jev selector gathers its own PR context.
        executor: JevDynTestExecutor = self.executor  # type: ignore[assignment]
        total = len(executor.jobs)
        print(f"[jev] querying executed tests for {total} jobs (CI Visibility, {self.lookback_days}d lookback)")
        started = time.monotonic()
        executed_per_job: dict[str, list] = {}
        for done, job in enumerate(executor.jobs, 1):
            if tests := self.list_tests_for_job(job):
                executed_per_job[job] = tests
            if done % 25 == 0 or done == total:
                print(
                    f"[jev] {done}/{total} jobs queried, {sum(map(len, executed_per_job.values()))} tests found, "
                    f"{time.monotonic() - started:.0f}s elapsed"
                )
        if not executed_per_job:
            return []
        if empty := sorted(set(executor.jobs) - executed_per_job.keys()):
            print(f"[jev] {len(empty)} completed E2E jobs executed no tests (not evaluated): {', '.join(empty)}")
        universe = executor.entry_points()
        names = {test.name for tests in executed_per_job.values() for test in tests}
        if unknown := names - universe:
            print(
                f"[jev] {len(unknown)} executed tests are not filetree entry points (not decidable): {sorted(unknown)}"
            )
        decidable = names & universe
        print(f"[jev] {len(executed_per_job)} jobs executed tests; deciding {len(decidable)} of them with Jev")
        run = executor.jev_run(decidable) if decidable else set()
        return [
            self._evaluate_job(job, tests, run, indexed_tests=universe & {test.name for test in tests})
            for job, tests in executed_per_job.items()
        ]
