"""Jev-based (System One) test selection, integrated with the dynamic tests.

Runs the standalone selector (tools/jev/jev_e2e_selector.py) for the suite of
the current e2e job and returns its run/skip decision. Used by the
new-e2e-tests.run --impacted path, gated by the JEV_SELECTION environment
variable:

- JEV_SELECTION=shadow: compute, log and measure the Jev decision, but do not
  enforce it (comparison only - the same rollout strategy the coverage-based
  selection used in its PR #39364 evaluation phase)
- JEV_SELECTION=enforce: additionally return the skips so the caller extends
  the go test --skip list

Always fails open: any error returns an empty selection, so the e2e job runs
its static test set.

Local testing (macOS, no linux authanywhere): override with
  JEV_TOKEN_CMD='ddtool auth token rapid-ai-platform --datacenter us1.staging.dog'
  JEV_DC='us1.staging.dog'
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

_REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))
_TOOLS_JEV = os.path.join(_REPO_ROOT, "tools", "jev")
if _TOOLS_JEV not in sys.path:  # standalone selector modules (stdlib only)
    sys.path.insert(0, _TOOLS_JEV)

from executed_lookup import _gitlab_pipeline_jobs  # noqa: E402
from test_discovery import E2E_TESTS_DIR, list_suites  # noqa: E402

_SELECTOR = os.path.join(_TOOLS_JEV, "jev_e2e_selector.py")


def _token_cmd(tmp: str) -> str:
    """Command producing the raw AI Gateway internal auth token."""
    if os.environ.get("JEV_TOKEN_CMD"):
        return os.environ["JEV_TOKEN_CMD"]
    # CI: download authanywhere (see the DDCI Metadata / Authanywhere docs)
    arch = "amd64" if os.uname().machine == "x86_64" else "arm64"
    path = os.path.join(tmp, "authanywhere")
    subprocess.run(
        [
            "curl",
            "-sSfL",
            "-o",
            path,
            f"https://binaries.ddbuild.io/dd-source/authanywhere/LATEST/authanywhere-linux-{arch}",
        ],
        check=True,
        timeout=60,
    )
    os.chmod(path, 0o755)
    return f"{path} --audience rapid-ai-platform --raw --dc us1.ddbuild.io"


def jev_selection(targets: list[str], team: str | None = None) -> dict:
    """Run the Jev selector for the job's e2e suite.

    Returns the selector's summary dict ({"suite", "run": [...], "skip": [...],
    "decisions": [...]}), or an empty dict on any error (fail open).
    """
    if not targets:
        return {}
    # the e2e jobs run with targets like ./tests/fleet (or ./tests/installer/unix)
    suite = os.path.basename(targets[0].rstrip("/"))
    tmp = tempfile.mkdtemp(prefix="jev-selection-")
    try:
        out = os.path.join(tmp, "decisions.json")
        cmd = [
            "python3",
            _SELECTOR,
            "--suite",
            suite,
            "--output",
            out,
            "--token-cmd",
            _token_cmd(tmp),
        ]
        if os.environ.get("JEV_DC"):
            cmd += ["--dc", os.environ["JEV_DC"]]
        if team:
            cmd += ["--team", team]
        res = subprocess.run(cmd, capture_output=True, text=True, timeout=300, cwd=_REPO_ROOT)
        if res.returncode != 0:
            raise RuntimeError(res.stderr.strip()[-500:] or "selector failed with no output")
        with open(out) as f:
            return json.load(f)
    except Exception as e:
        print(f"[jev] selection failed, failing open: {e}")
        return {}
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def jev_tests_to_skip(targets: list[str], team: str | None = None) -> tuple[list[str], dict]:
    """(to_skip, stats) for the dynamic tests integration: the entry points Jev
    would skip, plus the full decision summary for logging/measuring."""
    summary = jev_selection(targets, team)
    if not summary:
        return [], summary
    return summary.get("skip", []), summary


def jev_tests_to_run_all() -> set:
    """Run the Jev selection over every e2e suite; returns the entry points Jev
    would RUN (bare test names, the same keying the dynamic test index uses)."""
    run = set()
    suites_root = os.path.join(_REPO_ROOT, "test", "new-e2e", "tests")
    for suite in sorted(os.listdir(suites_root)):
        if not os.path.isdir(os.path.join(suites_root, suite)):
            continue
        summary = jev_selection([f"./tests/{suite}"])
        if summary:
            run.update(summary.get("run", []))
            print(f"[jev] suite {suite}: {len(summary.get('run', []))} run / {len(summary.get('skip', []))} skip")
    return run


def all_e2e_entry_points() -> set:
    """Every test entry point defined under test/new-e2e/tests, across all suites."""
    entries = set()
    root = os.path.join(_REPO_ROOT, E2E_TESTS_DIR)
    for suite in sorted(os.listdir(root)):
        suite_dir = os.path.join(root, suite)
        if not os.path.isdir(suite_dir):
            continue
        entries.update(name for name, _, _ in list_suites(suite_dir))
    print(f"[jev] e2e test universe: {len(entries)} entry points across all suites")
    return entries


def _candidates_for_job(job_name: str, universe: set) -> set:
    """Tests a job can run: entry points named in the job's --run pattern
    (matrix job names embed their EXTRA_PARAMS), or the whole universe when
    the job has no run pattern."""
    m = re.search(r"--run\s+\"?([^\"\]]+)", job_name)
    if m:
        return {t for t in universe if t in m.group(1)}
    return universe


class JevTestUniverse:
    """Index-like object for DynTestEvaluator, WITHOUT the coverage index
    restriction: every e2e job that ran in the pipeline is evaluated, and the
    decidable universe is every test entry point under test/new-e2e/tests
    (not only the tests with coverage data)."""

    def __init__(self, pipeline_id: str):
        self.pipeline_id = pipeline_id
        self._jobs: dict = {}

    def build(self) -> "JevTestUniverse":
        universe = all_e2e_entry_points()
        for job in _gitlab_pipeline_jobs(self.pipeline_id):
            if not job["name"].startswith("new-e2e"):
                continue
            self._jobs[job["name"]] = _candidates_for_job(job["name"], universe)
        print(f"[jev] universe: {len(self._jobs)} new-e2e jobs from pipeline {self.pipeline_id}")
        return self

    def to_dict(self) -> dict:
        return {job: sorted(tests) for job, tests in self._jobs.items()}

    def get_indexed_tests_for_job(self, job: str) -> set:
        return self._jobs.get(job, set())


class JevDynTestExecutor:
    """DynTestExecutor-compatible prediction source backed by Jev, for DynTestEvaluator.

    Unlike the coverage-based executor, the test universe is NOT restricted to
    the tests present in the coverage index: every e2e job that ran in the
    evaluated pipeline is considered (from the GitLab API, with their full
    matrix names), and every entry point under test/new-e2e/tests is decidable.
    The prediction (tests_to_run_per_job) comes from the Jev selector
    (tools/jev), which can decide on all of them - including tests without
    coverage data or brand new tests.
    """

    def __init__(self, pipeline_id: str):
        self.pipeline_id = pipeline_id
        self.commit_sha = os.getenv("CI_COMMIT_SHA") or ""
        self._universe: JevTestUniverse | None = None
        self._jev_run_tests = None

    # --- executor interface used by DynTestEvaluator ---------------------------------

    def init_index(self):
        self._universe = JevTestUniverse(self.pipeline_id).build()

    def index(self):
        return self._universe

    def tests_to_run_per_job(self, changes: list[str]) -> dict:
        # NOTE: the Jev prediction ignores `changes`; the selector gathers its
        # own, richer PR context (diff, description, team, test code).
        run = self._jev_run()
        return {job: run for job in self._universe.to_dict().keys()}

    def tests_to_skip(self, job_name: str, changes: list[str]) -> set:
        return set(self._universe.get_indexed_tests_for_job(job_name) or []) - self._jev_run()

    # ---------------------------------------------------------------------------------

    def _jev_run(self) -> set:
        if self._jev_run_tests is None:
            self._jev_run_tests = jev_tests_to_run_all()
        return self._jev_run_tests
