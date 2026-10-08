"""Jev-based (System One) E2E test selection for the dynamic-test evaluation.

JevDynTestExecutor is a plain DynTestExecutor: its index is a static job ->
candidate tests map, loaded from a file committed in the repo
(tasks/libs/dynamic_test/jev/job_test_candidates.json, generated once from
CI Visibility) and indexing the ENTIRE file - every job it knows, whether or
not that job ran in the evaluated pipeline - so the evaluation measures Jev's
over-selection: which tests and jobs Jev would run but the pipeline did not.
The coverage executors load their index from S3; this one loads its own from
Git. Predictions come from the Jev selector over the suites whose tests are
candidates. The shared DatadogDynTestEvaluator runs it like any other executor:
it owns the executed-tests queries (only from the pipeline) and the miss
logic. Only explicit, successful Jev skip decisions may remove tests -
errors and unknown tests run conservatively.
"""

from __future__ import annotations

import json
import os
from pathlib import Path

from tasks.libs.ciproviders.gitlab_api import get_pipeline
from tasks.libs.common.datadog_api import get_ci_test_events
from tasks.libs.dynamic_test.evaluator import executed_tests_from_events
from tasks.libs.dynamic_test.executor import DynTestExecutor
from tasks.libs.dynamic_test.index import DynamicTestIndex, IndexKind
from tasks.libs.dynamic_test.jev.jev_e2e_selector import select_suite
from tasks.libs.dynamic_test.jev.test_discovery import E2E_TESTS_DIR, list_suites

_REPO_ROOT = Path(__file__).resolve().parents[3]

# The committed job -> candidate tests map, generated once from CI Visibility
# (dyntest.generate-jev-job-index / generate_job_candidates below)
JOB_CANDIDATES_FILE = Path(__file__).resolve().parent / "jev" / "job_test_candidates.json"


class NothingToEvaluateError(RuntimeError):
    """The pipeline has no completed E2E test jobs to evaluate (not an error)."""


def jev_selection(suite: str, summary_model: str | None = None) -> dict:
    """Run the Jev selector for a suite in-process; return {} on any failure.

    summary_model is passed through to the selector: None uses the default
    LLM PR summary model (gpt-4o-mini, JEV_SUMMARY_MODEL override) and ""
    disables the summary so the raw diff is sent (the evaluation passes ""
    when the 'datadog-agent-jev-llm-summary' feature flag is off).
    Jev calls run concurrently inside the selector, with a per-request timeout.
    Any failure fails open to run all the suite's tests.
    """
    try:
        summary = select_suite(
            suite,
            dc=os.environ.get("JEV_DC", "us1.ddbuild.io"),
            token_cmd=os.environ.get("JEV_TOKEN_CMD"),
            summary_model=summary_model,
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


def _completed_e2e_jobs(pipeline) -> list:
    """The pipeline's completed (success/failed) new-e2e jobs, latest attempts.

    python-gitlab collapses list-valued query params (scope=["success",
    "failed"] reaches the API as a single scope), so query each status
    separately; iterator=True walks every page.
    """
    jobs: list = []
    for scope in ("success", "failed"):
        jobs.extend(pipeline.jobs.list(scope=scope, iterator=True))
    return [job for job in jobs if job.name.startswith("new-e2e")]


def generate_job_candidates(pipeline_ids: list[str]) -> dict[str, set[str]]:
    """Build the job -> candidate tests map from CI Visibility.

    One pipeline-wide query per pipeline (the events carry their job), a
    job's candidates are the tests that executed in it (latest attempts only,
    queried over the last 30 days), unioned across the pipelines and
    restricted to filetree entry points. Pick pipelines where the e2e jobs
    ran the widest (full-suite main pipelines or the largest dev pipelines)
    for the most complete map: a test the coverage selection skipped in every
    given pipeline stays missing from its job's candidates.
    """
    entry_points: set[str] = set()
    for names in suite_entry_points().values():
        entry_points |= names
    candidates: dict[str, set[str]] = {}
    for pipeline_id in pipeline_ids:
        jobs = _completed_e2e_jobs(get_pipeline("DataDog/datadog-agent", pipeline_id))
        latest = {str(job.id) for job in jobs}
        # env:nativetest is the e2e jobs' tag (the go test jobs use the
        # default env): filtering on it keeps the pipeline-wide query to the
        # e2e events only, an order of magnitude faster than unfiltered
        query = f"env:nativetest @ci.pipeline.name:DataDog/datadog-agent @ci.pipeline.id:{pipeline_id}"
        tests = executed_tests_from_events(get_ci_test_events(query, 30))
        found = 0
        for test in tests:
            if test.job_id is not None and str(test.job_id) in latest and test.name in entry_points:
                candidates.setdefault(test.job_name, set()).add(test.name)
                found += 1
        print(f"[jev] pipeline {pipeline_id}: {found} candidate tests across {len(jobs)} completed e2e jobs")
    return candidates


def _job_candidates() -> dict[str, set[str]]:
    """The committed job -> candidate tests map."""
    with open(JOB_CANDIDATES_FILE, encoding="utf-8") as f:
        return {job: set(tests) for job, tests in json.load(f).items()}


class JevDynTestExecutor(DynTestExecutor):
    """Jev executor whose index is the committed job -> candidate tests map.

    init_index() lazily loads it like every DynTestExecutor - from Git, where
    the coverage executors load theirs from S3 - and indexes the ENTIRE
    candidate file: every job it knows, whether or not that job ran in the
    evaluated pipeline. The evaluation therefore measures over-selection
    too: which tests and jobs Jev would run but the pipeline did not (jobs
    that did not run in the pipeline contribute zero executed tests, showing
    as predicted-but-not-executed). The pipeline's completed e2e jobs
    (GitLab API, latest attempts) are still fetched, for the executed-tests
    side of the comparison and to report jobs absent from the file.
    Predictions: the Jev selector's run-set over the suites whose tests are
    candidates (the selector gathers its own PR context from this checkout).
    """

    def __init__(
        self,
        ctx,
        commit_sha: str,
        pipeline_id: str,
        require_pipeline_commit: bool = True,
        summary_model: str | None = None,
    ):
        super().__init__(ctx, None, IndexKind.JEV, commit_sha)
        self.pipeline_id = pipeline_id
        # False (local experiments, --ignore-sha-mismatch): allow evaluating a
        # pipeline whose commit differs from the checkout - the Jev decisions
        # are then computed from the current checkout's PR context.
        self.require_pipeline_commit = require_pipeline_commit
        # The LLM PR summary model: None for the default, "" to disable the
        # summary (the 'datadog-agent-jev-llm-summary' feature flag is off)
        self.summary_model = summary_model
        self.jobs: list[str] = []
        self.job_ids: dict[str, str] = {}
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
        jobs = _completed_e2e_jobs(pipeline)
        if not jobs:
            raise NothingToEvaluateError(f"No completed E2E jobs in pipeline {self.pipeline_id}")
        self.jobs = [job.name for job in jobs]
        self.job_ids = {job.name: str(job.id) for job in jobs}

        candidates = _job_candidates()
        index = DynamicTestIndex()
        # The whole candidate file is indexed - every job it knows, whether or
        # not the job ran in this pipeline - so the evaluation shows Jev's
        # over-selection: tests and jobs Jev would run but the pipeline did
        # not (executed tests still come only from the pipeline, via the
        # shared evaluator)
        for job, tests in candidates.items():
            index.add_tests(job, "candidates", tests)
        missing = [job for job in self.jobs if not candidates.get(job)]
        if missing:
            print(
                f"[jev] {len(missing)} completed E2E jobs are not in the candidate index "
                f"(no candidates known, their executed tests are not evaluated): {', '.join(missing)}"
            )
        if not set(self.jobs) & set(candidates):
            raise NothingToEvaluateError(
                f"No completed E2E test jobs in the candidate index for pipeline {self.pipeline_id}"
            )
        not_ran = len(candidates) - len(set(candidates) & set(self.jobs))
        self._run = None
        self._index = index
        total = sum(len(index.get_indexed_tests_for_job(job)) for job in index.get_jobs())
        print(
            f"[jev] index: all {len(index.get_jobs())} jobs / {total} candidate tests from the committed "
            f"file ({len(self.jobs)} of them ran in this pipeline, {not_ran} did not)"
        )

    def _jev_run(self) -> set[str]:
        """Lazily: the candidate tests Jev would RUN, over the suites with candidates.

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
                summary = jev_selection(suite, self.summary_model)
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
