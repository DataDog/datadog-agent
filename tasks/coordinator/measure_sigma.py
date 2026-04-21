"""Populate per-scenario F1 σ by running the baseline N times and
computing std-dev across seeds.

Rationale: scoring gates (strict regression, recall floor) need a
per-scenario noise threshold. A scalar τ=0.05 misses the fact that
scenario F1 variance spans ~10x across scenarios — e.g. `221_base`
has σ ≈ 0.15 while `food_delivery_redis` is < 0.02. Gating on τ
either ships noise as "improvement" (if τ > σ_s) or rejects real
gains as noise (if τ < σ_s).

After running this, `scoring.score_against_baseline` uses `3 * σ_s`
per scenario instead of the scalar, for both strict-regression and
recall-floor gates.

Usage (from repo root):

  # N=5 repeats per detector (default), writes σ into db.yaml.
  PYTHONPATH=tasks python -m coordinator.measure_sigma --seeds 5

  # Or measure σ for a single detector to save time:
  PYTHONPATH=tasks python -m coordinator.measure_sigma --seeds 5 --only scanmw

This is a one-time setup step (repeat after any substantial pipeline
change). Each run reuses existing q.eval-scenarios machinery locally;
expect ~5 × 6 min per detector = ~30 min per detector.
"""

from __future__ import annotations

import argparse
import json
import statistics
import subprocess
import sys
import tempfile
from pathlib import Path

from .db import load_db, save_db
from .schema import ScenarioResult


def _run_baseline(detector: str, report_path: Path, scenario_output_dir: Path) -> bool:
    cmd = [
        "dda",
        "inv",
        "q.eval-scenarios",
        "--only",
        detector,
        "--no-build",
        "--main-report-path",
        str(report_path),
        "--scenario-output-dir",
        str(scenario_output_dir),
    ]
    scenario_output_dir.mkdir(parents=True, exist_ok=True)
    report_path.parent.mkdir(parents=True, exist_ok=True)
    proc = subprocess.run(cmd, capture_output=True, text=True)
    return proc.returncode == 0 and report_path.exists()


def _parse_report(path: Path) -> dict[str, float]:
    with path.open() as f:
        r = json.load(f)
    return {
        name: float(m.get("f1", 0.0)) for name, m in (r.get("metadata") or {}).items()
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="measure_sigma")
    parser.add_argument("--root", default=".")
    parser.add_argument("--seeds", type=int, default=5,
                        help="number of baseline repeats per detector (default: 5)")
    parser.add_argument("--only", default="",
                        help="comma-sep detectors to measure (default: every "
                             "detector present in db.baseline)")
    args = parser.parse_args(argv)

    db = load_db(Path(args.root))
    if db.baseline is None:
        print("error: no baseline in db.yaml. Run import_baseline first.", file=sys.stderr)
        return 1

    # Default to whatever was imported into db.baseline — no hardcoded list.
    detectors = (
        [d.strip() for d in args.only.split(",") if d.strip()]
        if args.only
        else list(db.baseline.detectors.keys())
    )

    tmp_root = Path(tempfile.mkdtemp(prefix="measure_sigma_"))
    print(f"writing intermediate reports to {tmp_root}")

    for det in detectors:
        if det not in db.baseline.detectors:
            print(f"skip {det}: no baseline loaded")
            continue
        print(f"\n=== {det}: {args.seeds} baseline repeats ===")
        per_scenario_runs: dict[str, list[float]] = {}
        for i in range(args.seeds):
            rp = tmp_root / f"{det}-run{i}.json"
            sd = tmp_root / f"{det}-run{i}"
            if not _run_baseline(det, rp, sd):
                print(f"  run {i}: FAILED")
                continue
            f1s = _parse_report(rp)
            for s, f1 in f1s.items():
                per_scenario_runs.setdefault(s, []).append(f1)
            print(f"  run {i}: mean F1 = {sum(f1s.values()) / max(len(f1s), 1):.4f}")

        updated = 0
        bd = db.baseline.detectors[det]
        for s_name, values in per_scenario_runs.items():
            if len(values) < 2:
                continue
            sigma = statistics.stdev(values)
            if s_name in bd.scenarios:
                bd.scenarios[s_name].f1_sigma = sigma
                updated += 1
                print(f"  {s_name}: σ = {sigma:.4f} (n={len(values)})")
        print(f"  updated σ on {updated} scenarios for {det}")

    save_db(db, Path(args.root))
    print("\ndone. db.yaml updated.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
