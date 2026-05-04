"""Run q.eval-scenarios as a subprocess and return the report path.

System-level eval is the default: no `--only` flag, the testbench's
catalog `defaultEnabled` set defines what runs. Per-detector standalone
eval mismeasured anything that interacts with siblings.

Single-detector mode (`detector=` argument) is retained for the per-iter
sanity sentinel — it wants to verify one specific detector + one known
scenario reproduces a known F1, which needs `--only`.
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
    report_path: Path,
    scenario_output_dir: Path,
    root: Path = Path("."),
    timeout_seconds: int = 3600,
    rebuild: bool = True,
    config_path: Path | None = None,
    scenarios: str = "",
    detector: str = "",
) -> EvalRun:
    """Run q.eval-scenarios, writing the main report to `report_path`.

    System-level mode (default, `detector=""`): no `--only` flag, runs
    whatever the catalog has `defaultEnabled: true`. This is what
    coordinator candidate iters use.

    Single-detector mode (`detector="bocpd"`): adds `--only <detector>`,
    used by the sanity sentinel.
    """
    scenario_output_dir.mkdir(parents=True, exist_ok=True)
    report_path.parent.mkdir(parents=True, exist_ok=True)

    cmd = [
        "dda", "inv", "q.eval-scenarios",
        "--main-report-path", str(report_path),
        "--scenario-output-dir", str(scenario_output_dir),
    ]
    if detector:
        cmd += ["--only", detector]
    if not rebuild:
        cmd.append("--no-build")
    if config_path is not None:
        cmd += ["--config", str(config_path)]
    if scenarios:
        cmd += ["--scenarios", scenarios]

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
