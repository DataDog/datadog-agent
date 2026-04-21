"""Import the M0.1 baseline from per-detector report JSONs into db.yaml.

Usage:
  python -m tasks.coordinator.import_baseline \\
      --bocpd    /Users/ella.taira/.claude/plans/baseline-m0.1-371630f2dc0/bocpd.json \\
      --scanmw   .../scanmw.json \\
      --scanwelch .../scanwelch.json \\
      --sha 371630f2dc0
"""

from __future__ import annotations

import argparse
import datetime as _dt
import sys
from pathlib import Path

from .db import load_db, save_db
from .schema import Baseline, BaselineDetector, ScenarioResult
from .scoring import load_report


def build_baseline_detector(report_path: Path) -> BaselineDetector:
    mean_f1, per_scen = load_report(report_path)
    total_fps = sum(s.num_baseline_fps for s in per_scen.values())
    return BaselineDetector(
        mean_f1=mean_f1,
        total_fps=total_fps,
        scenarios=per_scen,
    )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="import_baseline")
    parser.add_argument("--root", default=".")
    parser.add_argument("--bocpd", required=True, type=Path)
    parser.add_argument("--scanmw", required=True, type=Path)
    parser.add_argument("--scanwelch", required=True, type=Path)
    parser.add_argument("--sha", required=True)
    args = parser.parse_args(argv)

    root = Path(args.root)
    db = load_db(root)
    detectors = {
        "bocpd": build_baseline_detector(args.bocpd),
        "scanmw": build_baseline_detector(args.scanmw),
        "scanwelch": build_baseline_detector(args.scanwelch),
    }
    db.baseline = Baseline(
        sha=args.sha,
        generated_at=_dt.datetime.now().isoformat(timespec="seconds"),
        detectors=detectors,
    )
    save_db(db, root)
    for name, d in detectors.items():
        print(f"{name}: mean_f1={d.mean_f1:.4f} total_fps={d.total_fps}")
    print(f"baseline written to {root}/.coordinator/db.yaml (sha={args.sha})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
