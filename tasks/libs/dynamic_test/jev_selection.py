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
import shutil
import subprocess
import tempfile

_REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))
_SELECTOR = os.path.join(_REPO_ROOT, "tools", "jev", "jev_e2e_selector.py")


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


class JevDynTestExecutor:
    """DynTestExecutor-compatible prediction source backed by Jev, for DynTestEvaluator.

    Wraps a coverage-based DynTestExecutor: the coverage index provides the
    job/test universe (which jobs to evaluate, and which tests each job can
    run), so the evaluator measures the Jev selection and the coverage
    selection on exactly the same test universe with the same metrics - only
    the prediction differs. Used by the evaluate-jev-index task.
    """

    def __init__(self, coverage_executor):
        self._coverage = coverage_executor
        self._jev_run_tests = None

    # --- executor interface used by DynTestEvaluator ---------------------------------

    @property
    def commit_sha(self):
        return self._coverage.commit_sha

    def init_index(self):
        self._coverage.init_index()

    def index(self):
        return self._coverage.index()

    def tests_to_run_per_job(self, changes: list[str]) -> dict:
        # NOTE: the Jev prediction ignores `changes`; the selector gathers its
        # own, richer PR context (diff, description, team, test code).
        run = self._jev_run()
        return {job: run for job in self.index().to_dict().keys()}

    def tests_to_skip(self, job_name: str, changes: list[str]) -> set:
        indexed = set(self.index().get_indexed_tests_for_job(job_name) or [])
        return indexed - self._jev_run()

    # ---------------------------------------------------------------------------------

    def _jev_run(self) -> set:
        if self._jev_run_tests is None:
            self._jev_run_tests = jev_tests_to_run_all()
        return self._jev_run_tests
