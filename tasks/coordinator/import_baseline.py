"""Import a system-level baseline report into db.yaml.

Usage:
  # New (system-level) mode — single report, no per-detector split:
  python -m coordinator.import_baseline \\
      --report /tmp/observer-eval-main-report.json \\
      --sha $(git rev-parse --short HEAD)

  # Legacy per-detector mode — combines multiple reports into a single
  # system-level entry as a transitional shim. Deprecated; rerun
  # `dda inv q.eval-scenarios` (no --only) and use --report instead.
  python -m coordinator.import_baseline \\
      --detector bocpd=.../bocpd/report.json \\
      --detector scanmw=.../scanmw/report.json \\
      --sha $(git rev-parse --short HEAD)
"""

from __future__ import annotations

import argparse
import datetime as _dt
import sys
from pathlib import Path

from .db import load_db, save_db
from .schema import Baseline, BaselineMetrics, ScenarioResult
from .scoring import load_report


def build_system_metrics(report_path: Path) -> BaselineMetrics:
    mean_f1, per_scen = load_report(report_path)
    total_fps = sum(s.num_baseline_fps for s in per_scen.values())
    return BaselineMetrics(mean_f1=mean_f1, total_fps=total_fps, scenarios=per_scen)


def combine_legacy_per_detector(paths: list[tuple[str, Path]]) -> BaselineMetrics:
    """Transitional: combine N per-detector reports as max(F1)/sum(FPs).

    Stand-in only — re-run `dda inv q.eval-scenarios` (no --only) for a
    real system-level report.
    """
    combined: dict[str, ScenarioResult] = {}
    for _name, path in paths:
        _, per_scen = load_report(path)
        for scen, sr in per_scen.items():
            prior = combined.get(scen)
            if prior is None:
                combined[scen] = sr
            else:
                combined[scen] = ScenarioResult(
                    f1=max(prior.f1, sr.f1),
                    precision=max(prior.precision, sr.precision),
                    recall=max(prior.recall, sr.recall),
                    num_baseline_fps=prior.num_baseline_fps + sr.num_baseline_fps,
                )
    mean_f1 = (
        sum(s.f1 for s in combined.values()) / len(combined) if combined else 0.0
    )
    total_fps = sum(s.num_baseline_fps for s in combined.values())
    return BaselineMetrics(mean_f1=mean_f1, total_fps=total_fps, scenarios=combined)


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
        "--report",
        type=Path,
        help="Path to a system-level q.eval-scenarios report.json (no --only).",
    )
    parser.add_argument(
        "--detector",
        action="append",
        type=_parse_detector_arg,
        metavar="NAME=PATH",
        help="Legacy per-detector report. Combined as a transitional shim.",
    )
    parser.add_argument("--sha", required=True)
    args = parser.parse_args(argv)

    if not args.report and not args.detector:
        parser.error("specify either --report (system) or --detector (legacy)")

    root = Path(args.root)
    db = load_db(root)

    if args.report:
        metrics = build_system_metrics(args.report)
        source = f"--report {args.report}"
    else:
        print(
            "WARNING: legacy per-detector import. Combining max(f1) / "
            "sum(fps) across detectors as a stand-in. Re-run "
            "`dda inv q.eval-scenarios` (no --only) and use --report.",
            file=sys.stderr,
        )
        metrics = combine_legacy_per_detector(args.detector)
        source = f"legacy combine of {len(args.detector)} per-detector reports"

    db.baseline = Baseline(
        sha=args.sha,
        generated_at=_dt.datetime.now().isoformat(timespec="seconds"),
        system=metrics,
    )
    save_db(db, root)
    print(
        f"system baseline: mean_f1={metrics.mean_f1:.4f} "
        f"total_fps={metrics.total_fps} ({source})"
    )
    print(f"baseline written to {root}/.coordinator/db.yaml (sha={args.sha})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
