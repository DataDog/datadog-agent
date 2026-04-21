"""Overfit telltale: periodic Spearman rank-correlation between the
coordinator's train-set ranking of shipped candidates and their true
out-of-sample ranking on the lockbox.

Runs every CONFIG.overfit_check_every_n_ships ships (when we have enough
data). Evaluates every shipped candidate against `split.lockbox` scenarios
via q.eval-scenarios. Computes Spearman ρ. If ρ < threshold, emits a
coord-out warning — the coordinator is ranking candidates by train noise,
not by real signal.

Lockbox results are recorded on a dedicated Experiment (`tier=t4`,
`scenario_set=lockbox`) so they're auditable via db.yaml. They are NEVER
included in SDK prompts (implement / review / proposer) — doing so would
leak lockbox info and defeat the holdout.

Trade-off: this check itself consumes lockbox runs. Running it too often
still means the lockbox influences scheduling indirectly (even if only
through the coord-out warning). Kept rare via `every_n_ships`.
"""

from __future__ import annotations

import datetime as _dt
import statistics
from pathlib import Path

from . import coord_out, evaluator, journal
from .config import CONFIG
from .db import save_db, state_dir
from .scoring import load_report


def _spearman(xs: list[float], ys: list[float]) -> float | None:
    """Spearman rank-correlation. Returns None if < 3 points or no variance."""
    n = len(xs)
    if n < 3 or n != len(ys):
        return None

    def _rank(vs: list[float]) -> list[float]:
        order = sorted(range(len(vs)), key=lambda i: vs[i])
        ranks = [0.0] * len(vs)
        i = 0
        while i < len(vs):
            j = i
            while j + 1 < len(vs) and vs[order[j + 1]] == vs[order[i]]:
                j += 1
            avg_rank = (i + j) / 2 + 1
            for k in range(i, j + 1):
                ranks[order[k]] = avg_rank
            i = j + 1
        return ranks

    rx = _rank(xs)
    ry = _rank(ys)
    try:
        # Pearson on ranks == Spearman
        mx = sum(rx) / n
        my = sum(ry) / n
        num = sum((rx[i] - mx) * (ry[i] - my) for i in range(n))
        dx = sum((rx[i] - mx) ** 2 for i in range(n)) ** 0.5
        dy = sum((ry[i] - my) ** 2 for i in range(n)) ** 0.5
        if dx == 0 or dy == 0:
            return None
        return num / (dx * dy)
    except ZeroDivisionError:
        return None


def _count_ships(db) -> int:
    from .schema import CandidateStatus

    return sum(1 for c in db.candidates.values() if c.status == CandidateStatus.SHIPPED)


def should_check(db) -> bool:
    """Fire on every Nth ship, once we have enough shipped candidates."""
    ships = _count_ships(db)
    if ships < CONFIG.overfit_min_ships_required:
        return False
    return (ships % CONFIG.overfit_check_every_n_ships) == 0


def run_lockbox_for_candidate(
    candidate_id: str,
    detector: str,
    lockbox_scenarios: list[str],
    root: Path,
) -> float | None:
    """Run q.eval-scenarios on the lockbox for one shipped candidate's commit.

    Returns the mean F1 on the lockbox subset (not gated; just recorded).
    None on eval failure. The candidate must already be committed on the
    scratch branch; caller is responsible for checking out its SHA first
    if comparing across multiple candidates.
    """
    out_dir = state_dir(root) / "reports" / f"lockbox-{candidate_id}"
    report_path = out_dir.parent / f"lockbox-{candidate_id}.json"
    scenarios_csv = ",".join(lockbox_scenarios)

    # Build the eval command manually because evaluator.run_scenarios
    # doesn't accept a scenario subset yet — stay minimal here.
    import subprocess, shlex

    cmd = [
        "dda", "inv", "q.eval-scenarios",
        "--only", detector,
        "--no-build",
        "--scenarios", scenarios_csv,
        "--main-report-path", str(report_path),
        "--scenario-output-dir", str(out_dir),
    ]
    out_dir.mkdir(parents=True, exist_ok=True)
    try:
        proc = subprocess.run(cmd, cwd=root, capture_output=True, text=True, timeout=3600)
    except Exception as e:
        journal.append("overfit_lockbox_eval_error", {"candidate": candidate_id, "error": str(e)}, root)
        return None
    if proc.returncode != 0 or not report_path.exists():
        journal.append("overfit_lockbox_eval_failed", {"candidate": candidate_id, "rc": proc.returncode}, root)
        return None
    mean_f1, _ = load_report(report_path)
    return mean_f1


def maybe_run_overfit_check(db, root: Path) -> None:
    """Invoked after a ship. Runs the periodic rank-corr check if due.

    This function is best-effort: any failure is journalled and the
    coordinator keeps going. The check itself is expensive (lockbox
    evals) so it's gated by should_check().
    """
    if not should_check(db):
        return
    if db.split is None:
        return

    from .schema import CandidateStatus

    shipped = [c for c in db.candidates.values() if c.status == CandidateStatus.SHIPPED]
    if len(shipped) < CONFIG.overfit_min_ships_required:
        return

    # Train score per shipped candidate = its latest experiment's .score
    train_scores: list[float] = []
    lockbox_scores: list[float] = []
    evaluated_ids: list[str] = []
    lockbox = db.split.lockbox
    for c in shipped:
        # Latest DONE experiment for this candidate
        exps = [e for e in db.experiments.values() if e.candidate_id == c.id and e.score is not None]
        if not exps:
            continue
        latest = max(exps, key=lambda e: e.completed_at or e.started_at or "")
        # Use first target component as the detector.
        detector = next((t for t in c.target_components if t in {"bocpd", "scanmw", "scanwelch"}), None)
        if detector is None:
            continue
        lb = run_lockbox_for_candidate(c.id, detector, lockbox, root)
        if lb is None:
            continue
        train_scores.append(float(latest.score))
        lockbox_scores.append(lb)
        evaluated_ids.append(c.id)

    rho = _spearman(train_scores, lockbox_scores)
    journal.append(
        "overfit_check",
        {
            "ships_evaluated": len(evaluated_ids),
            "candidate_ids": evaluated_ids,
            "train_scores": train_scores,
            "lockbox_scores": lockbox_scores,
            "spearman": rho,
        },
        root,
    )

    if rho is None:
        return
    if rho < CONFIG.overfit_spearman_threshold:
        coord_out.emit(
            "tripwire",
            f"Overfit telltale: Spearman ρ between train and lockbox rankings "
            f"of {len(evaluated_ids)} shipped candidates = {rho:.2f} "
            f"(threshold {CONFIG.overfit_spearman_threshold}). Coordinator "
            f"is ranking candidates by train-set noise, not real signal. "
            f"Consider pausing to audit shipped candidates: "
            f"{', '.join(evaluated_ids)}",
            requires_ack=True,
            root=root,
        )
    save_db(db, root)
