"""Run q.eval-scenarios as a subprocess and return the report path.

Local-mode T0. Workspace dispatch (T2+) is a follow-up.
"""

from __future__ import annotations

import subprocess
from dataclasses import dataclass
from pathlib import Path


@dataclass
class EvalRun:
    ok: bool
    returncode: int
    report_path: Path
    stdout: str
    stderr: str


def run_scenarios(
    detector: str,
    report_path: Path,
    scenario_output_dir: Path,
    root: Path = Path("."),
    timeout_seconds: int = 3600,
    rebuild: bool = True,
    config_path: Path | None = None,
) -> EvalRun:
    """Run q.eval-scenarios with the given --only detector, writing the
    main report to `report_path`. Returns an EvalRun describing outcome.
    """
    scenario_output_dir.mkdir(parents=True, exist_ok=True)
    report_path.parent.mkdir(parents=True, exist_ok=True)

    cmd = [
        "dda", "inv", "q.eval-scenarios",
        "--only", detector,
        "--main-report-path", str(report_path),
        "--scenario-output-dir", str(scenario_output_dir),
    ]
    if not rebuild:
        cmd.append("--no-build")
    if config_path is not None:
        cmd += ["--config", str(config_path)]

    try:
        proc = subprocess.run(
            cmd, cwd=root, capture_output=True, text=True, timeout=timeout_seconds
        )
        return EvalRun(
            ok=(proc.returncode == 0 and report_path.exists()),
            returncode=proc.returncode,
            report_path=report_path,
            stdout=proc.stdout,
            stderr=proc.stderr,
        )
    except subprocess.TimeoutExpired as e:
        return EvalRun(
            ok=False,
            returncode=-1,
            report_path=report_path,
            stdout=e.stdout or "",
            stderr=f"timeout after {timeout_seconds}s",
        )
