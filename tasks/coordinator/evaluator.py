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
    scenarios: str = "",
    extra_components: list[str] | None = None,
) -> EvalRun:
    """Run q.eval-scenarios with the given --only detector, writing the
    main report to `report_path`. Returns an EvalRun describing outcome.

    `scenarios`: comma-separated scenario name list; defaults to all.
    Used by the per-iter sanity sentinel (single scenario, fast turnaround).

    `extra_components`: additional component names to enable alongside
    `detector` in --only. Used for correlator/filter candidates that
    consume upstream detector firings — running them standalone produces
    zero detections (PR 50045/50046 fallout). Driver passes the canonical
    detectors (bocpd, scanmw, scanwelch) when candidate.kind != detector.
    """
    scenario_output_dir.mkdir(parents=True, exist_ok=True)
    report_path.parent.mkdir(parents=True, exist_ok=True)

    only_csv = detector
    if extra_components:
        # Dedupe + preserve "candidate first" ordering for log readability.
        seen = {detector}
        extras = [c for c in extra_components if c not in seen and not seen.add(c)]
        only_csv = ",".join([detector, *extras])

    cmd = [
        "dda", "inv", "q.eval-scenarios",
        "--only", only_csv,
        "--main-report-path", str(report_path),
        "--scenario-output-dir", str(scenario_output_dir),
    ]
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
