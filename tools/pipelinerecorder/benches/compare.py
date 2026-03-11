#!/usr/bin/env python3
"""
Compare two bench result JSON files and report regressions.

Usage:
  python3 compare.py baseline.json current.json

Exit code:
  0 — no regressions (or improvements only)
  1 — at least one metric regressed beyond the threshold

Example:
  cp bench_result.json baselines/steady_metrics_$(date +%Y%m%d).json
  python3 tools/pipelinerecorder/benches/compare.py \
      baselines/steady_metrics_20260309.json bench_result.json
"""

import argparse
import json
import sys

# Metrics to compare and their preferred direction.
METRICS = [
    ("actual_rate",        "higher_is_better"),
    ("drop_rate_pct",      "lower_is_better"),
    ("compression_ratio",  "higher_is_better"),
    ("rate_drift_pct",     "lower_is_better"),
    ("bytes_sent_total",   "higher_is_better"),
    ("vortex_bytes_on_disk", "lower_is_better"),
]

# A metric is flagged as a regression when it moves in the wrong direction
# by more than this fraction.
REGRESSION_THRESHOLD = 0.05   # 5 %


def compare(base: dict, pr: dict) -> list:
    """Return list of regressed metric names; print a summary table."""
    regressions = []

    width_key = max(len(k) for k, _ in METRICS)
    print(f"\n{'Status':12s}  {'Metric':{width_key}s}  {'Baseline':>12s}  {'Current':>12s}  {'Delta':>8s}")
    print("-" * (width_key + 50))

    for key, direction in METRICS:
        b = base.get(key)
        p = pr.get(key)

        if b is None or p is None:
            print(f"{'MISSING':12s}  {key:{width_key}s}  (not present in one or both files)")
            continue

        if b == 0:
            delta = 0.0
        else:
            delta = (p - b) / abs(b)

        if direction == "lower_is_better":
            worse = delta > REGRESSION_THRESHOLD
        else:
            worse = delta < -REGRESSION_THRESHOLD

        status = "REGRESSION" if worse else ("improved" if abs(delta) > 0.001 else "ok")
        print(f"{status:12s}  {key:{width_key}s}  {b:>12.3f}  {p:>12.3f}  {delta:>+8.1%}")

        if worse:
            regressions.append(key)

    print()
    if regressions:
        print(f"FAIL: {len(regressions)} regression(s) detected: {', '.join(regressions)}")
    else:
        print("PASS: no regressions detected.")

    return regressions


def main() -> None:
    ap = argparse.ArgumentParser(
        description="Compare two bench_result.json files for pipeline performance regressions."
    )
    ap.add_argument("baseline", help="Path to the baseline JSON (e.g. from main branch)")
    ap.add_argument("current",  help="Path to the current JSON (e.g. from a PR)")
    ap.add_argument(
        "--threshold", type=float, default=REGRESSION_THRESHOLD,
        help=f"Regression threshold as a fraction (default: {REGRESSION_THRESHOLD})"
    )
    args = ap.parse_args()

    with open(args.baseline) as f:
        base = json.load(f)
    with open(args.current) as f:
        pr = json.load(f)

    print(f"Baseline : {args.baseline}  (git: {base.get('git_sha', '?')},"
          f" workload: {base.get('workload', '?')},"
          f" {base.get('duration_secs', '?')}s)")
    print(f"Current  : {args.current}  (git: {pr.get('git_sha', '?')},"
          f" workload: {pr.get('workload', '?')},"
          f" {pr.get('duration_secs', '?')}s)")

    regressions = compare(base, pr)
    sys.exit(1 if regressions else 0)


if __name__ == "__main__":
    main()
