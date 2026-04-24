"""Offline marginal re-evaluation of shipped candidates.

Post-run audit. For each shipped candidate, check out the candidate's
commit AND its parent, run `q.eval-scenarios` at N seeds on each, and
report the marginal ΔF1 (this ship - its parent state) with confidence
intervals per scenario.

Why this is needed: during the run, `experiment.per_scenario` records
the cumulative state at ship time (i.e. this ship + all prior ships
combined). That answers "where are we now?" but not "did this specific
candidate help?" — if ship 5 regressed by 0.03 but shipped 1-4
collectively gained 0.10, ship 5 looks positive vs baseline but is a
marginal loss. Without this re-eval, we can't tell.

Independence relies on the one-component-per-candidate policy enforced
in proposer.materialize_candidates and seed_candidates._load_one. If
a shipped candidate was somehow multi-component, the marginal view
still shows its combined effect, but cherry-picking it standalone
may be non-trivial.

Output: JSON report `reeval-ships.json` under --out with fields:
  - per_candidate: [{id, sha, parent_sha, target_component,
                     scenario_deltas: {scenario: {mean_df1, ci_low,
                     ci_high, n_seeds, train_or_lockbox}}}]
  - summary: {n_ships, n_lockbox_positive, n_train_positive}

Reads db.yaml for the ship list and baseline split; reads eval-results
from the runs done here. Does NOT modify db.yaml.

Usage:
  # From repo root, with driver checked out somewhere stable:
  PYTHONPATH=tasks python -m coordinator.reeval_ships \
      --root . --seeds 20 --out ./reeval-ships.json

  # Dry-run: plan what would be evaluated, no subprocess calls.
  PYTHONPATH=tasks python -m coordinator.reeval_ships --dry-run

Cost: N seeds × 2 shas × ~6min per eval. For 5 ships × 20 seeds × 2 =
200 evals ≈ 20h on a single workspace. Parallelise across workspaces
if needed by splitting the ship list with --only.
"""

from __future__ import annotations

import argparse
import json
import math
import statistics
import subprocess
import sys
import tempfile
from pathlib import Path

from .db import load_db
from .schema import CandidateStatus


def _git(args: list[str], cwd: Path) -> subprocess.CompletedProcess:
    return subprocess.run(
        ["git", *args], cwd=cwd, capture_output=True, text=True
    )


def _current_branch(cwd: Path) -> str:
    return _git(["rev-parse", "--abbrev-ref", "HEAD"], cwd).stdout.strip()


def _parent_sha(sha: str, cwd: Path) -> str | None:
    r = _git(["rev-parse", f"{sha}^"], cwd)
    if r.returncode != 0:
        return None
    return r.stdout.strip() or None


def _build_once(cwd: Path) -> bool:
    """Build testbench + scorer once before the re-eval loop."""
    r = subprocess.run(
        ["dda", "inv", "q.build-testbench", "q.build-scorer"],
        cwd=cwd, capture_output=True, text=True,
    )
    return r.returncode == 0


def _eval_at_sha(
    sha: str, detector: str, seed_idx: int, work_root: Path, repo_root: Path
) -> dict[str, float] | None:
    """Checkout `sha`, build, run q.eval-scenarios for `detector`. Return
    {scenario_name: f1} or None on failure.

    Caller is responsible for restoring the original branch after the
    full loop; we don't restore per-call because that's N× slower.
    """
    r = _git(["checkout", "--quiet", sha], repo_root)
    if r.returncode != 0:
        print(f"  checkout {sha[:10]} failed: {r.stderr.strip()[:200]}",
              file=sys.stderr)
        return None
    # Rebuild — each sha has potentially different detector code.
    if not _build_once(repo_root):
        print(f"  build failed at {sha[:10]}", file=sys.stderr)
        return None
    report_path = work_root / f"{sha[:10]}-seed{seed_idx}.json"
    scenario_dir = work_root / f"{sha[:10]}-seed{seed_idx}"
    scenario_dir.mkdir(parents=True, exist_ok=True)
    cmd = [
        "dda", "inv", "q.eval-scenarios",
        "--only", detector,
        "--no-build",  # just built above
        "--main-report-path", str(report_path),
        "--scenario-output-dir", str(scenario_dir),
    ]
    r = subprocess.run(cmd, cwd=repo_root, capture_output=True, text=True)
    if r.returncode != 0 or not report_path.exists():
        print(f"  eval failed at {sha[:10]} seed {seed_idx}",
              file=sys.stderr)
        return None
    with report_path.open() as f:
        report = json.load(f)
    return {
        name: float(m.get("f1", 0.0))
        for name, m in (report.get("metadata") or {}).items()
    }


def _mean_ci(values: list[float]) -> tuple[float, float, float]:
    """Return (mean, ci_low, ci_high) for a 95% CI (Student-t; n ≥ 2)."""
    n = len(values)
    if n < 2:
        return (values[0] if values else 0.0, 0.0, 0.0)
    mean = statistics.fmean(values)
    stdev = statistics.stdev(values)
    # t(0.975, df=n-1) — use a table for small n. For n >= 20, t ≈ 2.09.
    # For 20 seeds per the recommendation, approximate 2.09; for smaller
    # n, fall back to 2.26 (n=9) worst case. Good enough for a flag
    # column — this isn't a thesis.
    t_lookup = {2: 12.71, 3: 4.30, 4: 3.18, 5: 2.78, 10: 2.26, 20: 2.09, 30: 2.05}
    keys = sorted(t_lookup.keys())
    t = t_lookup[20]
    for k in keys:
        if n <= k:
            t = t_lookup[k]
            break
    half = t * stdev / math.sqrt(n)
    return mean, mean - half, mean + half


def _diff_with_ci(
    ship_f1s: list[float], parent_f1s: list[float]
) -> tuple[float, float, float]:
    """Per-scenario marginal mean Δ + 95% CI. Independent-samples, equal-n
    assumed; not paired (each seed draws a fresh run).
    """
    n_s = len(ship_f1s)
    n_p = len(parent_f1s)
    if n_s < 2 or n_p < 2:
        mean_diff = (statistics.fmean(ship_f1s) if ship_f1s else 0.0) - \
                    (statistics.fmean(parent_f1s) if parent_f1s else 0.0)
        return mean_diff, 0.0, 0.0
    m_s = statistics.fmean(ship_f1s)
    m_p = statistics.fmean(parent_f1s)
    var_s = statistics.variance(ship_f1s)
    var_p = statistics.variance(parent_f1s)
    se = math.sqrt(var_s / n_s + var_p / n_p)
    # Conservative df = min(n_s, n_p) - 1; t ≈ 2.09 at df ≥ 19.
    t = 2.09 if min(n_s, n_p) >= 20 else 2.26
    half = t * se
    diff = m_s - m_p
    return diff, diff - half, diff + half


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="reeval_ships")
    parser.add_argument("--root", default=".",
                        help="Repo root (must contain .coordinator/db.yaml).")
    parser.add_argument("--seeds", type=int, default=20,
                        help="Eval repeats per (sha, parent_sha). Default 20.")
    parser.add_argument("--only", default="",
                        help="Comma-separated candidate IDs to restrict the "
                             "re-eval set. Default: all shipped candidates.")
    parser.add_argument("--out", default="./reeval-ships.json",
                        help="Output JSON report path.")
    parser.add_argument("--dry-run", action="store_true",
                        help="Print the evaluation plan without running.")
    args = parser.parse_args(argv)

    repo_root = Path(args.root).resolve()
    db = load_db(repo_root)
    if db.baseline is None:
        print("error: db.yaml has no baseline loaded.", file=sys.stderr)
        return 1

    # Reconstruct ship list from experiments with a real commit_sha, not
    # the "pending" sentinel used by the crash-recovery path.
    ships: list[tuple[str, str, str]] = []  # (candidate_id, sha, detector)
    only = set(x.strip() for x in args.only.split(",") if x.strip())
    for cand in db.candidates.values():
        if cand.status != CandidateStatus.SHIPPED:
            continue
        if only and cand.id not in only:
            continue
        # Find this candidate's shipping experiment.
        cand_exps = [
            e for e in db.experiments.values()
            if e.candidate_id == cand.id and e.commit_sha and e.commit_sha != "pending"
        ]
        if not cand_exps:
            continue
        exp = max(cand_exps, key=lambda e: e.completed_at or e.started_at or "")
        detector = (cand.target_components or ["scanmw"])[0]
        ships.append((cand.id, exp.commit_sha, detector))

    if not ships:
        print("no shipped candidates with commit shas; nothing to re-eval.")
        return 0

    train = set(db.split.train) if db.split else set()
    lockbox = set(db.split.lockbox) if db.split else set()

    print(f"plan: {len(ships)} ships × {args.seeds} seeds × 2 shas "
          f"= {len(ships) * args.seeds * 2} evals")
    for cid, sha, det in ships:
        parent = _parent_sha(sha, repo_root)
        print(f"  {cid}  ({det})  {sha[:10]}  parent={parent and parent[:10]}")

    if args.dry_run:
        return 0

    # Guard: current branch must be claude/observer-improvements or we're
    # likely to leave the caller in a weird state. (We checkout by sha;
    # returning to a known branch at the end is the caller's job if they
    # care.)
    orig_branch = _current_branch(repo_root)
    print(f"\ncurrent branch: {orig_branch} (will be left in detached HEAD)")

    work_root = Path(tempfile.mkdtemp(prefix="reeval_ships_"))
    print(f"work dir: {work_root}\n")

    results = []
    for cid, sha, det in ships:
        parent = _parent_sha(sha, repo_root)
        if parent is None:
            print(f"skip {cid}: no parent sha")
            continue
        print(f"=== {cid}  ({det})  {sha[:10]} vs {parent[:10]} ===")

        # Run `seeds` evals at each sha. Parent first (warmer cache for
        # ship; marginal impact is small either way).
        parent_runs: dict[str, list[float]] = {}
        ship_runs: dict[str, list[float]] = {}
        for i in range(args.seeds):
            f1s = _eval_at_sha(parent, det, i, work_root, repo_root)
            if f1s is not None:
                for s, v in f1s.items():
                    parent_runs.setdefault(s, []).append(v)
            f1s = _eval_at_sha(sha, det, i, work_root, repo_root)
            if f1s is not None:
                for s, v in f1s.items():
                    ship_runs.setdefault(s, []).append(v)

        scenario_deltas: dict[str, dict] = {}
        for scen in sorted(set(parent_runs) | set(ship_runs)):
            p_vals = parent_runs.get(scen, [])
            s_vals = ship_runs.get(scen, [])
            diff, lo, hi = _diff_with_ci(s_vals, p_vals)
            if scen in train:
                split_tag = "train"
            elif scen in lockbox:
                split_tag = "lockbox"
            else:
                split_tag = "unknown"
            scenario_deltas[scen] = {
                "mean_df1": round(diff, 4),
                "ci_low": round(lo, 4),
                "ci_high": round(hi, 4),
                "n_parent": len(p_vals),
                "n_ship": len(s_vals),
                "split": split_tag,
            }
        results.append({
            "candidate_id": cid,
            "sha": sha,
            "parent_sha": parent,
            "detector": det,
            "scenario_deltas": scenario_deltas,
        })

    # Summarise. A ship "generalizes" if its lockbox-scenarios have
    # CI_low > 0 on any scenario (i.e. real marginal gain on held-out
    # data). "Train win" if any train scenario has CI_low > 0.
    n_lockbox_pos = sum(
        1 for r in results
        if any(sd["split"] == "lockbox" and sd["ci_low"] > 0
               for sd in r["scenario_deltas"].values())
    )
    n_train_pos = sum(
        1 for r in results
        if any(sd["split"] == "train" and sd["ci_low"] > 0
               for sd in r["scenario_deltas"].values())
    )

    out = {
        "n_ships": len(results),
        "n_train_positive": n_train_pos,
        "n_lockbox_positive": n_lockbox_pos,
        "seeds_per_sha": args.seeds,
        "per_candidate": results,
    }
    Path(args.out).write_text(json.dumps(out, indent=2))
    print(f"\nwrote {args.out}")
    print(f"summary: {len(results)} ships, "
          f"{n_train_pos} with ≥1 train CI_low>0, "
          f"{n_lockbox_pos} with ≥1 lockbox CI_low>0")
    return 0


if __name__ == "__main__":
    sys.exit(main())
