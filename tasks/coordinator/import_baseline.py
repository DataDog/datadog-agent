"""Import the M0.1 baseline from per-detector report JSONs into db.yaml.

Usage:
  # Current 3 detectors:
  python -m coordinator.import_baseline \\
      --detector bocpd=.../bocpd/report.json \\
      --detector scanmw=.../scanmw/report.json \\
      --detector scanwelch=.../scanwelch/report.json \\
      --sha $(git rev-parse --short HEAD)

  # Adding a new detector is just another --detector flag:
  python -m coordinator.import_baseline \\
      --detector bocpd=... \\
      --detector scanmw=... \\
      --detector scanwelch=... \\
      --detector my-new-detector=/path/to/my-new-detector/report.json \\
      --sha $(git rev-parse --short HEAD)

Each `--detector NAME=PATH` imports the q.eval-scenarios main report at
PATH into `db.baseline.detectors[NAME]`. No hardcoded detector list.
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


def _parse_detector_arg(raw: str) -> tuple[str, Path]:
    if "=" not in raw:
        raise argparse.ArgumentTypeError(
            f"--detector must be NAME=PATH (got: {raw!r})"
        )
    name, _, path = raw.partition("=")
    name = name.strip()
    path = path.strip()
    if not name or not path:
        raise argparse.ArgumentTypeError(
            f"--detector NAME=PATH requires both parts (got: {raw!r})"
        )
    return name, Path(path)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="import_baseline")
    parser.add_argument("--root", default=".")
    parser.add_argument(
        "--detector",
        action="append",
        required=True,
        type=_parse_detector_arg,
        metavar="NAME=PATH",
        help="Repeatable: detector name + path to its q.eval-scenarios report.json. "
             "Specify once per detector; no hardcoded list.",
    )
    parser.add_argument("--sha", required=True)
    args = parser.parse_args(argv)

    root = Path(args.root)
    db = load_db(root)

    detectors: dict[str, BaselineDetector] = {}
    for name, path in args.detector:
        detectors[name] = build_baseline_detector(path)

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
