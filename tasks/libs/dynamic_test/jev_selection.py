"""Jev-based (System One) E2E test selection for the dynamic-test evaluation.

JevDynTestExecutor is a real DynTestExecutor: its index is the pipeline's
observed execution map (job -> executed tests), restricted to the E2E test
filetree (test/new-e2e/tests). The coverage executors load their index from S3;
this one loads its own with one pipeline-wide CI Visibility query (the events
carry their job). Predictions come from the Jev selector over the suites whose
tests executed. No CI-configuration or coverage data is involved; only
explicit, successful Jev skip decisions may remove tests - errors and unknown
tests run conservatively.
"""

from __future__ import annotations

import os
import time
from pathlib import Path

from tasks.libs.ciproviders.gitlab_api import get_pipeline
from tasks.libs.common.datadog_api import get_ci_test_events
from tasks.libs.dynamic_test.evaluator import DatadogDynTestEvaluator, ExecutedTest, executed_tests_from_events
from tasks.libs.dynamic_test.executor import DynTestExecutor
from tasks.libs.dynamic_test.index import DynamicTestIndex, IndexKind
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


class JevDynTestExecutor(DynTestExecutor):
    """Jev executor whose index is the pipeline's observed execution map.

    init_index() lazily loads the index like every DynTestExecutor - here from
    one pipeline-wide CI Visibility query instead of S3:

    - jobs: every completed (success/failed) new-e2e job of the pipeline, from
      the GitLab API (each status queried separately; python-gitlab collapses
      list-valued scopes), with the latest-attempt job id and allow-failure flag
    - indexed tests per job: the tests that actually executed in it (latest
      attempts only), restricted to filetree entry points
    - predictions: the Jev selector's run-set over the suites whose tests
      executed (the selector gathers its own PR context from this checkout)
    """

    def __init__(
        self,
        ctx,
        commit_sha: str,
        pipeline_id: str,
        require_pipeline_commit: bool = True,
        test_env: str = "nativetest",
        lookback_days: int = 3,
    ):
        super().__init__(ctx, None, IndexKind.JEV, commit_sha)
        self.pipeline_id = pipeline_id
        # False (local experiments, --ignore-sha-mismatch): allow evaluating a
        # pipeline whose commit differs from the checkout - the Jev decisions
        # are then computed from the current checkout's PR context.
        self.require_pipeline_commit = require_pipeline_commit
        self.test_env = test_env
        self.lookback_days = lookback_days
        self.jobs: list[str] = []
        self.job_ids: dict[str, str] = {}
        self.unreliable_jobs: set[str] = set()
        # job -> executed tests, as fetched during the index build: the
        # evaluator reads these instead of re-querying CI Visibility
        self.executed: dict[str, list[ExecutedTest]] = {}
        self._entry_points: set[str] | None = None
        self._run: set[str] | None = None

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
        started = time.monotonic()
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

        # The index: executed tests of the whole pipeline in one query - the
        # events carry their job, so no job-to-test mapping to reconstruct.
        # Keeping only the latest attempts also drops canceled or non-e2e jobs.
        query = f"env:{self.test_env} @ci.pipeline.name:DataDog/datadog-agent @ci.pipeline.id:{self.pipeline_id}"
        tests = executed_tests_from_events(get_ci_test_events(query, self.lookback_days), self.unreliable_jobs)
        latest = set(self.job_ids.values())
        for test in tests:
            if test.job_id is not None and str(test.job_id) in latest:
                self.executed.setdefault(test.job_name, []).append(test)

        universe = self.entry_points()
        index = DynamicTestIndex()
        known: set[str] = set()
        unknown: set[str] = set()
        for job, job_tests in self.executed.items():
            names = {test.name for test in job_tests}
            unknown |= names - universe
            index.add_tests(job, "executed", names & universe)
            known |= names & universe
        if unknown:
            print(
                f"[jev] {len(unknown)} executed tests are not filetree entry points (not decidable): {sorted(unknown)}"
            )
        if empty := sorted(set(self.jobs) - self.executed.keys()):
            print(f"[jev] {len(empty)} completed E2E jobs executed no tests (not evaluated): {', '.join(empty)}")
        self._run = None
        self._index = index
        print(
            f"[jev] index: {len(index.get_jobs())} jobs with executed tests, {len(known)} tests, "
            f"fetched in {time.monotonic() - started:.0f}s"
        )

    def entry_points(self) -> set[str]:
        if self._entry_points is None:
            self._entry_points = all_entry_points()
        return self._entry_points

    def _jev_run(self) -> set[str]:
        """Lazily: the entry points Jev would RUN, over the suites that executed.

        Only suites with something to decide are evaluated. Tests the selector
        did not decide about (missing or failed-open decisions) run, and a bare
        name occurring in several suites runs if any occurrence runs.
        """
        if self._run is None:
            names: set[str] = set()
            for job in self.index().get_jobs():
                names |= self.index().get_indexed_tests_for_job(job)
            suites = {suite: entries for suite, entries in suite_entry_points().items() if entries & names}
            print(f"[jev] deciding {len(names)} tests with Jev; suites: {', '.join(sorted(suites))}")
            run: set[str] = set()
            for suite, entries in sorted(suites.items()):
                summary = jev_selection(suite)
                skip = set(summary.get("skip", [])) - set(summary.get("run", []))
                run.update(entries - skip)
                print(f"[jev] {suite}: {len(entries - skip)} run / {len(entries & skip)} skip")
            self._run = run
        return self._run

    def tests_to_run_per_job(self, changes: list[str]) -> dict[str, set[str]]:
        # changes are ignored: the Jev selector gathers its own PR context.
        run = self._jev_run()
        return {job: self.index().get_indexed_tests_for_job(job) & run for job in self.index().get_jobs()}

    def tests_to_run(self, job_name: str, changes: list[str]) -> set[str]:
        return self.index().get_indexed_tests_for_job(job_name) & self._jev_run()

    def tests_to_skip(self, job_name: str, changes: list[str]) -> set[str]:
        return self.index().get_indexed_tests_for_job(job_name) - self._jev_run()

    def triggering_paths(self, job_name: str, test_name: str) -> list[str]:
        # No coverage information behind the Jev selection
        return []


class JevDynTestEvaluator(DatadogDynTestEvaluator):
    """The Jev evaluation: executed tests come from the executor's index.

    list_tests_for_job reads the tests already fetched by the executor's index
    build (one pipeline-wide query): no per-job CI Visibility query. Everything
    else - the evaluate() flow, miss logic, summary, metrics - is the shared
    evaluator's, running unmodified.
    """

    def list_tests_for_job(self, job_name: str) -> list[ExecutedTest]:
        return self.executor.executed.get(job_name, [])
