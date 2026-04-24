"""Overfit telltale: Spearman rank-correlation between train-set ranking
and lockbox-set ranking of shipped candidates.

Reads scores from EXISTING experiment records — does NOT re-run any
evals. This matters: a candidate's lockbox score must be measured at
the moment it shipped (when its own commit was HEAD). Re-evaluating
later would score the current cumulative state, not the candidate's
contribution.

The driver's ship-time eval already runs q.eval-scenarios over ALL 10
scenarios (both train and lockbox). Train scenarios are gated;
lockbox scenarios are observed-but-not-gated. Their F1 values are
recorded in exp.per_scenario. We just read them here.

Runs every CONFIG.overfit_check_every_n_ships ships. When Spearman ρ
between train-rank and lockbox-rank of shipped candidates drops below
CONFIG.overfit_spearman_threshold, emits a `tripwire` coord-out —
the coordinator is ranking candidates by train-set noise, not real
signal.

Lockbox scores NEVER surface in agent prompts: Python consumes them
for this telltale only.
"""

from __future__ import annotations

from pathlib import Path

from . import coord_out, journal
from .config import CONFIG
from .db import save_db
from .schema import CandidateStatus, ScenarioResult


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
    mx = sum(rx) / n
    my = sum(ry) / n
    num = sum((rx[i] - mx) * (ry[i] - my) for i in range(n))
    dx = sum((rx[i] - mx) ** 2 for i in range(n)) ** 0.5
    dy = sum((ry[i] - my) ** 2 for i in range(n)) ** 0.5
    if dx == 0 or dy == 0:
        return None
    return num / (dx * dy)


def _mean_over(per_scenario: dict[str, ScenarioResult], scope: set[str]) -> float | None:
    vals = [sr.f1 for name, sr in per_scenario.items() if name in scope]
    if not vals:
        return None
    return sum(vals) / len(vals)


def _count_ships(db) -> int:
    return sum(1 for c in db.candidates.values() if c.status == CandidateStatus.SHIPPED)


def should_check(db) -> bool:
    ships = _count_ships(db)
    if ships < CONFIG.overfit_min_ships_required:
        return False
    return (ships % CONFIG.overfit_check_every_n_ships) == 0


def maybe_run_overfit_check(db, root: Path) -> None:
    """Runs after every ship. Fires the check when cadence matches.

    Best-effort: any failure is journalled, coordinator keeps going.
    """
    if not should_check(db) or db.split is None:
        return

    train = set(db.split.train)
    lockbox = set(db.split.lockbox)

    # Per shipped candidate: its LATEST eval's train-mean F1 and
    # lockbox-mean F1, both already present in exp.per_scenario from the
    # ship-time q.eval-scenarios run (which evaluates all 10 scenarios).
    train_scores: list[float] = []
    lockbox_scores: list[float] = []
    evaluated_ids: list[str] = []

    for cand in db.candidates.values():
        if cand.status != CandidateStatus.SHIPPED:
            continue
        exps = [
            e for e in db.experiments.values()
            if e.candidate_id == cand.id and e.per_scenario
        ]
        if not exps:
            continue
        latest = max(exps, key=lambda e: e.completed_at or e.started_at or "")
        train_mean = _mean_over(latest.per_scenario, train)
        lockbox_mean = _mean_over(latest.per_scenario, lockbox)
        if train_mean is None or lockbox_mean is None:
            continue
        train_scores.append(train_mean)
        lockbox_scores.append(lockbox_mean)
        evaluated_ids.append(cand.id)

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
            f"Consider pausing to audit: {', '.join(evaluated_ids)}",
            requires_ack=True,
            root=root,
        )
        save_db(db, root)
