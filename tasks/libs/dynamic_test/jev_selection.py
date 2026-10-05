"""Jev selection adapter for the existing dynamic-test executor/evaluator.

The evaluation universe comes from the evaluated pipeline's jobs and their
resolved TARGETS/EXTRA_PARAMS, not from coverage data. Only explicit, successful
Jev skip decisions may remove tests; errors and unknown tests run conservatively.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shlex
import subprocess
import sys
import tempfile
from pathlib import Path
from string import Template

from tasks.libs.ciproviders.gitlab_api import (
    get_pipeline,
    post_process_gitlab_ci_configuration,
    resolve_gitlab_ci_configuration,
)
from tasks.libs.dynamic_test.executor import DynTestExecutor
from tasks.libs.dynamic_test.index import DynamicTestIndex, IndexKind
from tasks.libs.dynamic_test.jev.test_discovery import E2E_TESTS_DIR, list_suites

_REPO_ROOT = Path(__file__).resolve().parents[3]


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


def _suite_path(target: str) -> str:
    """Keep nested suite paths (installer/unix), rather than just the basename."""
    target = target.removeprefix("./").removesuffix("/...").rstrip("/")
    if not target.startswith("tests/") or ".." in Path(target).parts or "$" in target:
        raise ValueError(f"Unsupported E2E TARGETS value: {target}")
    return target.removeprefix("tests/")


def _expand_variables(value: str, variables: dict) -> str:
    """Expand job/matrix variables while preserving regex anchors such as '$'."""
    for _ in range(10):
        expanded = Template(value).safe_substitute(variables)
        if expanded == value:
            return expanded
        value = expanded
    raise ValueError("Cyclic CI variable expansion")


def _candidates_for_job(extra_params: str, universe: set[str]) -> set[str]:
    """Apply Go's root-test run/skip regexes from the resolved job variables.

    Subtest-only skips must not remove a root suite. Go splits run patterns at
    '/', so TestFleetConfig$/TestConfig still selects the TestFleetConfig root.
    """
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--run", default="")
    parser.add_argument("--skip", default="")
    args, _ = parser.parse_known_args(shlex.split(extra_params))
    if any(re.search(r"\$(?:\w+|\{)", pattern) for pattern in (args.run, args.skip)):
        raise ValueError("Unresolved CI variable in the job's run/skip filters")
    run = args.run.split("/", 1)[0]
    skip = args.skip if "/" not in args.skip else ""
    return {name for name in universe if re.search(run, name) and not (skip and re.search(skip, name))}


class JevDynTestExecutor(DynTestExecutor):
    """Evaluation-only Jev executor, with an in-memory DynamicTestIndex.

    Uses the shared GitLab helpers for pipeline jobs and CI configuration. The
    shared Datadog evaluator owns all executed-test queries and comparisons.
    """

    def __init__(self, ctx, commit_sha: str, pipeline_id: str):
        super().__init__(ctx, None, IndexKind.JEV, commit_sha)
        self.pipeline_id = pipeline_id
        self.job_ids: dict[str, str] = {}
        self.unreliable_jobs: set[str] = set()
        self._suites: set[str] = set()
        self._jev_run_tests: set[str] | None = None

    def init_index(self):
        pipeline = get_pipeline("DataDog/datadog-agent", self.pipeline_id)
        if pipeline.sha != self.commit_sha:
            raise RuntimeError("The evaluated pipeline SHA must match --commit-sha and the checked-out HEAD")
        jobs = [
            job
            for job in pipeline.jobs.list(scope=["success", "failed"], iterator=True)
            if job.name.startswith("new-e2e")
        ]
        if not jobs:
            raise RuntimeError(f"No completed E2E jobs in pipeline {self.pipeline_id}")
        config = post_process_gitlab_ci_configuration(resolve_gitlab_ci_configuration(self.ctx), expand_matrix=True)
        index = DynamicTestIndex()
        self.job_ids.clear()
        self.unreliable_jobs.clear()
        self._suites.clear()
        self._jev_run_tests = None
        for job in jobs:
            # Non-matrix parallel jobs have a ' 1/N' suffix, not separate configs.
            job_config = config.get(job.name) or config.get(re.sub(r" \d+/\d+$", "", job.name))
            if not job_config:
                raise RuntimeError(f"Cannot find CI configuration for {job.name}")
            variables = {**config.get("variables", {}), **job_config.get("variables", {})}
            targets = _expand_variables(variables.get("TARGETS", ""), variables)
            if not targets:
                # new-e2e-unit-tests and cleanup jobs are not E2E test runs.
                continue
            candidates = set()
            for target in targets.split(","):
                suite = _suite_path(target.strip())
                directory = _REPO_ROOT / E2E_TESTS_DIR / suite
                if not directory.is_dir():
                    raise RuntimeError(f"E2E suite does not exist: {directory}")
                self._suites.add(suite)
                candidates.update(name for name, _, _ in list_suites(str(directory)))
            candidates = _candidates_for_job(
                _expand_variables(variables.get("EXTRA_PARAMS", ""), variables), candidates
            )
            index.add_tests(job.name, "jev", candidates)
            self.job_ids[job.name] = str(job.id)
            if job.allow_failure:
                self.unreliable_jobs.add(job.name)
        if not index.get_jobs():
            raise RuntimeError(f"No completed E2E test jobs in pipeline {self.pipeline_id}")
        self._index = index
        print(
            f"[jev] universe: {len(index.get_jobs())} E2E jobs, {len(self._suites)} suites in pipeline {self.pipeline_id}"
        )

    def _jev_run(self) -> set[str]:
        self.index()
        if self._jev_run_tests is None:
            run = set()
            for suite in sorted(self._suites):
                discovered = {name for name, _, _ in list_suites(str(_REPO_ROOT / E2E_TESTS_DIR / suite))}
                summary = jev_selection(suite)
                # Unknown/missing decisions run. Duplicate bare names across suites
                # run if ANY occurrence runs, matching the index's bare-name keys.
                skip = set(summary.get("skip", [])) - set(summary.get("run", []))
                run.update(discovered - skip)
                print(f"[jev] {suite}: {len(discovered - skip)} run / {len(discovered & skip)} skip")
            self._jev_run_tests = run
        return self._jev_run_tests

    def tests_to_run_per_job(self, changes: list[str]) -> dict[str, set[str]]:
        run = self._jev_run()
        return {job: self.index().get_indexed_tests_for_job(job) & run for job in self.index().get_jobs()}

    def tests_to_run(self, job_name: str, changes: list[str]) -> set[str]:
        return self.index().get_indexed_tests_for_job(job_name) & self._jev_run()

    def tests_to_skip(self, job_name: str, changes: list[str]) -> set[str]:
        return self.index().get_indexed_tests_for_job(job_name) - self._jev_run()

    def triggering_paths(self, job_name: str, test_name: str) -> list[str]:
        return []
