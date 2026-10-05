"""Jev-based (System One) test selection, integrated with the dynamic tests.

Provides JevDynTestExecutor, a DynTestExecutor implementation whose
selection decisions come from the standalone Jev selector
(tools/jev/jev_e2e_selector.py) instead of the coverage index: usable
wherever a DynTestExecutor is accepted, e.g. the evaluate-index task with
--selector jev (see tasks/dyntest.py). Always fails open: any error returns
an empty selection, so nothing is skipped.

Tokens: an installed authanywhere is preferred (CI jobs install it via the
.install_authanywhere template, laptops via `brew install datadog/tap/ddr &&
brew install authanywhere`); $JEV_TOKEN_CMD/$JEV_DC override, and the linux
binary is downloaded as a last resort.
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

try:
    # Real base class when used from the invoke tasks (CI image, invoke installed)
    from tasks.libs.dynamic_test.executor import DynTestExecutor
except ImportError:  # standalone use (no invoke): duck-type the same interface
    DynTestExecutor = object

_SELECTOR = os.path.join(_TOOLS_JEV, "jev_e2e_selector.py")


def _auth(tmp: str) -> tuple[str, str]:
    """(token command, AI Gateway dc) for the Jev selector.

    Prefers an installed authanywhere (CI: the .install_authanywhere template
    puts it on the workspace PATH; laptops: brew), falls back to
    $JEV_TOKEN_CMD/$JEV_DC, and downloads the linux binary as a last resort
    (CI jobs without the install step).
    """
    if os.environ.get("JEV_TOKEN_CMD"):
        return os.environ["JEV_TOKEN_CMD"], os.environ.get("JEV_DC", "us1.ddbuild.io")
    if shutil.which("authanywhere"):
        return "authanywhere --audience rapid-ai-platform --raw --dc us1.ddbuild.io", "us1.ddbuild.io"
    # last resort: download the linux binary
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
    return f"{path} --audience rapid-ai-platform --raw --dc us1.ddbuild.io", "us1.ddbuild.io"


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
        token_cmd, dc = _auth(tmp)
        cmd = [
            "python3",
            _SELECTOR,
            "--suite",
            suite,
            "--output",
            out,
            "--dc",
            dc,
            "--token-cmd",
            token_cmd,
        ]
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


class JevDynTestExecutor(DynTestExecutor):
    """A DynTestExecutor implementation whose decisions come from the Jev
    selector (AI Gateway System One) instead of the coverage index.

    Drop-in alternative to the coverage-based executor, usable wherever a
    DynTestExecutor is accepted (the dynamic tests evaluation, or the
    --impacted selection) without modifying the callers:

    - same constructor shape; `backend` is unused (kept for interface
      compatibility, the Jev selection has no stored index)
    - the "index" is the full e2e test universe: every job that ran in the
      evaluated pipeline (GitLab API, full matrix names) and every test
      entry point under test/new-e2e/tests - NOT restricted to the tests
      the coverage index knows about
    - predictions (tests_to_run / tests_to_skip) come from the Jev selector,
      which decides on all of them, including tests without coverage data or
      brand new tests
    """

    # Extra telemetry tags identifying this selection in the evaluation
    # stats (the coverage executors carry none) - see evaluate_index
    telemetry_tags = ["selector:jev", "universe:all-e2e-tests"]

    def __init__(self, ctx, backend, kind, commit_sha, pipeline_id=None):
        if DynTestExecutor is not object:
            super().__init__(ctx, backend, kind, commit_sha)
        else:  # duck-typed fallback (no invoke available): mirror the base attributes
            self.ctx, self.backend, self.kind = ctx, backend, kind
            self.commit_sha = commit_sha
            self._index = None
        self.pipeline_id = pipeline_id or os.getenv("CI_PIPELINE_ID") or ""
        self._jev_run_tests = None

    def init_index(self):
        """Build the full test universe instead of loading a stored index."""
        self._index = JevTestUniverse(self.pipeline_id).build()

    def index(self):
        # same lazy semantics as the base class (redefined so the duck-typed
        # fallback without invoke also has it)
        if self._index is None:
            self.init_index()
        return self._index

    def tests_to_run_per_job(self, changes: list[str]) -> dict:
        # NOTE: the Jev prediction ignores `changes`; the selector gathers its
        # own, richer PR context (diff, description, team, test code).
        run = self._jev_run()
        return {job: run for job in self.index().to_dict().keys()}

    def tests_to_run(self, job_name: str, changes: list[str]) -> set:
        candidates = self.index().get_indexed_tests_for_job(job_name) or set()
        return set(candidates) & self._jev_run()

    def tests_to_skip(self, job_name: str, changes: list[str]) -> set:
        candidates = self.index().get_indexed_tests_for_job(job_name) or set()
        return set(candidates) - self._jev_run()

    def triggering_paths(self, job_name: str, test_name: str) -> list:
        # No coverage information behind the Jev selection
        return []

    def _jev_run(self) -> set:
        if self._jev_run_tests is None:
            self._jev_run_tests = jev_tests_to_run_all()
        return self._jev_run_tests
