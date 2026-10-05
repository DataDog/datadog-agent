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
