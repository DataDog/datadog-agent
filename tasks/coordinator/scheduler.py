"""Candidate selection policy with diversity enforcement.

Keeps the coordinator from spending hours tuning one approach. Two rules:

1. **Prefer families not already saturated**. Track recent experiments by
   `candidate.approach_family`. If a family has had N consecutive iterations
   without score improvement, block it from scheduling until the ban window
   expires or another family produces a gain.

2. **Prefer candidates informed by prior wins**. Candidates with
   `parent_candidates` pointing at high-scoring past experiments are
   scheduled first within the allowed families.
"""

from __future__ import annotations

from dataclasses import dataclass

from .config import CONFIG
from .schema import Candidate, CandidateStatus, Db


# Re-exported for readability; live values come from config.CONFIG.
STUCK_THRESHOLD = CONFIG.stuck_threshold
BAN_DURATION = CONFIG.ban_duration


@dataclass
class ScheduleDecision:
    candidate: Candidate | None
    banned_families: set[str]
    reason: str


def _family_consecutive_nonimproving(db: Db, family: str) -> int:
    """Count consecutive recent iterations on this family without a
    meaningful improvement over the global best.

    Effect-size aware: an experiment "improves" only if its score clears
    `db.phase_state.best_score + ε`. The raw strict-greater comparison
    previously used let noisy +0.001 bumps reset the counter, keeping
    dead-end families alive indefinitely — while genuine small-effect
    winners got banned when 5 draws happened to be flat.

    Walks back from the most recent experiment; the streak ends at the
    first experiment that clears the ε threshold OR at a family switch.
    """
    epsilon = CONFIG.plateau_epsilon
    best = db.phase_state.best_score
    count = 0
    for exp in reversed(list(db.experiments.values())):
        cand = db.candidates.get(exp.candidate_id)
        if cand is None:
            continue
        if cand.approach_family != family:
            # Hit a different family; streak broken.
            break
        if exp.score is not None and exp.score > best + epsilon:
            break
        count += 1
    return count


def stuck_families(db: Db) -> set[str]:
    """Return the set of approach families currently banned from scheduling.

    Union of two sources:
      - Per-family "stuck" detection (K consecutive non-improving iters).
      - Persistent `db.pivot_banned_families` — families banned by an
        autopilot phase-plateau pivot. Never re-tried for the rest of
        the run.
    """
    # Persistent bans from prior plateau pivots.
    banned: set[str] = set(db.pivot_banned_families)

    # Gather families seen in recent experiments
    recent_families: list[str] = []
    for exp in list(db.experiments.values())[-BAN_DURATION - STUCK_THRESHOLD :]:
        cand = db.candidates.get(exp.candidate_id)
        if cand is not None:
            recent_families.append(cand.approach_family)
    for fam in set(recent_families):
        if fam == "unspecified":
            continue
        if _family_consecutive_nonimproving(db, fam) >= STUCK_THRESHOLD:
            banned.add(fam)
    return banned


def pick_next_candidate(db: Db) -> ScheduleDecision:
    """Pick the next candidate to work on, enforcing diversity.

    Policy:
      1. Collect PROPOSED candidates in current phase.
      2. Remove those whose family is currently banned (stuck).
      3. Prefer candidates whose parent_candidates include past SHIPPED wins.
      4. Within that, first-seen wins (deterministic).
    """
    banned = stuck_families(db)
    proposed = [
        c
        for c in db.candidates.values()
        if c.phase == db.phase_state.current_phase and c.status == CandidateStatus.PROPOSED
    ]
    if not proposed:
        return ScheduleDecision(
            candidate=None,
            banned_families=banned,
            reason="queue empty; invoke proposer",
        )

    allowed = [c for c in proposed if c.approach_family not in banned]
    if not allowed:
        # All allowed families are banned. Let the proposer generate
        # a fresh family rather than violate the diversity rule.
        return ScheduleDecision(
            candidate=None,
            banned_families=banned,
            reason=f"all proposed families banned {sorted(banned)}; invoke proposer for a new family",
        )

    # Rank: candidates referencing a SHIPPED parent first, else insertion order.
    shipped_ids = {
        c.id for c in db.candidates.values() if c.status == CandidateStatus.SHIPPED
    }

    def _informed_by_win(c: Candidate) -> int:
        return -len(set(c.parent_candidates) & shipped_ids)  # negative = earlier

    allowed.sort(key=lambda c: (_informed_by_win(c), c.proposed_at, c.id))
    return ScheduleDecision(
        candidate=allowed[0],
        banned_families=banned,
        reason=("banned families: " + ", ".join(sorted(banned))) if banned else "ok",
    )
