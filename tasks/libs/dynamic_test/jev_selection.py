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

import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

from tasks.libs.ciproviders.gitlab_api import get_pipeline
from tasks.libs.dynamic_test.evaluator import DatadogDynTestEvaluator, EvaluationResult
from tasks.libs.dynamic_test.executor import DynTestExecutor
from tasks.libs.dynamic_test.index import IndexKind
from tasks.libs.dynamic_test.jev.test_discovery import E2E_TESTS_DIR, list_suites

_REPO_ROOT = Path(__file__).resolve().parents[3]


class NothingToEvaluateError(RuntimeError):
    """The pipeline has no completed E2E test jobs to evaluate (not an error)."""


def jev_selection(suite: str) -> dict:
    """Run the selector for a relative E2E suite path; return {} on failure."""
    with tempfile.TemporaryDirectory(prefix="jev-selection-") as tmp:
        output = Path(tmp) / "decisions.json"
        cmd = [
            sys.executable,
            "-m",
            "tasks.libs.dynamic_test.jev.jev_e2e_selector",
            "--suite",
            suite,
            "--output",
            str(output),
            "--dc",
            os.environ.get("JEV_DC", "us1.ddbuild.io"),
        ]
        if token_cmd := os.environ.get("JEV_TOKEN_CMD"):
            cmd += ["--token-cmd", token_cmd]
        try:
            # Calls run concurrently in the selector, with a per-request timeout.
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=1800, cwd=_REPO_ROOT)
            if result.returncode:
                raise RuntimeError(result.stderr.strip()[-500:] or "selector failed with no output")
            with output.open() as f:
                summary = json.load(f)
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
        except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as e:
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

    def __init__(self, ctx, commit_sha: str, pipeline_id: str):
        super().__init__(ctx, None, IndexKind.JEV, commit_sha)
        self.pipeline_id = pipeline_id
        self.jobs: list[str] = []
        self.job_ids: dict[str, str] = {}
        self.unreliable_jobs: set[str] = set()
        self._entry_points: set[str] | None = None

    def init_index(self):
        pipeline = get_pipeline("DataDog/datadog-agent", self.pipeline_id)
        if pipeline.sha != self.commit_sha:
            raise RuntimeError("The evaluated pipeline SHA must match --commit-sha and the checked-out HEAD")
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
        executed_per_job: dict[str, list] = {}
        for job in executor.jobs:
            if tests := self.list_tests_for_job(job):
                executed_per_job[job] = tests
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
        run = executor.jev_run(names & universe) if names & universe else set()
        return [
            self._evaluate_job(job, tests, run, indexed_tests=universe & {test.name for test in tests})
            for job, tests in executed_per_job.items()
        ]
