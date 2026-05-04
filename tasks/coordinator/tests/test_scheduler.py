"""Diversity policy: stuck-family detection + parent-win preference."""

from coordinator.db import empty_db
from coordinator.scheduler import (
    BAN_DURATION,
    STUCK_THRESHOLD,
    pick_next_candidate,
    stuck_families,
)
from coordinator.schema import (
    Candidate,
    CandidateStatus,
    Experiment,
    ExperimentStatus,
    Phase,
    Tier,
)


def _mk_cand(id: str, family: str, status: CandidateStatus = CandidateStatus.PROPOSED,
             parents: list[str] | None = None) -> Candidate:
    return Candidate(
        id=id,
        description="",
        source="test",
        target_components=["scanmw"],
        phase=Phase.ONE,
        status=status,
        approach_family=family,
        parent_candidates=parents or [],
    )


def _mk_exp(id: str, candidate_id: str, score: float) -> Experiment:
    return Experiment(
        id=id,
        candidate_id=candidate_id,
        phase=Phase.ONE,
        tier=Tier.T0,
        commit_sha="x",
        config_path="",
        scenario_set=[],
        status=ExperimentStatus.DONE,
        score=score,
    )


def test_no_ban_when_fresh():
    db = empty_db()
    db.phase_state.current_phase = Phase.ONE
    db.candidates["a1"] = _mk_cand("a1", "family-x")
    decision = pick_next_candidate(db)
    assert decision.candidate is not None
    assert decision.candidate.id == "a1"
    assert decision.banned_families == set()


def test_family_banned_after_k_non_improving():
    """K consecutive experiments in the same family with score <= best → banned."""
    db = empty_db()
    db.phase_state.current_phase = Phase.ONE
    db.phase_state.best_score = 0.10

    # Three COMPLETED experiments in family-x, none improving
    for i in range(STUCK_THRESHOLD):
        cid = f"cand-{i}"
        db.candidates[cid] = _mk_cand(cid, "family-x", status=CandidateStatus.REJECTED)
        db.experiments[f"exp-{i}"] = _mk_exp(f"exp-{i}", cid, score=0.08)

    # A fresh family-x candidate still in proposed
    db.candidates["pending"] = _mk_cand("pending", "family-x")
    decision = pick_next_candidate(db)
    assert "family-x" in decision.banned_families
    assert decision.candidate is None  # no non-banned candidates → trigger proposer


def test_family_not_banned_if_another_family_active():
    """Only CONSECUTIVE non-improving matters; mixing families resets."""
    db = empty_db()
    db.phase_state.current_phase = Phase.ONE
    db.phase_state.best_score = 0.10

    # family-x, family-y, family-x alternation — consecutive streak broken
    db.candidates["x1"] = _mk_cand("x1", "family-x", status=CandidateStatus.REJECTED)
    db.experiments["e1"] = _mk_exp("e1", "x1", 0.05)
    db.candidates["y1"] = _mk_cand("y1", "family-y", status=CandidateStatus.REJECTED)
    db.experiments["e2"] = _mk_exp("e2", "y1", 0.05)
    db.candidates["x2"] = _mk_cand("x2", "family-x", status=CandidateStatus.REJECTED)
    db.experiments["e3"] = _mk_exp("e3", "x2", 0.05)

    banned = stuck_families(db)
    assert banned == set()


def test_parent_win_preferred():
    """Candidates referencing a SHIPPED parent are scheduled first."""
    db = empty_db()
    db.phase_state.current_phase = Phase.ONE
    db.candidates["winner"] = _mk_cand("winner", "family-a", status=CandidateStatus.SHIPPED)
    db.candidates["follow-up"] = _mk_cand(
        "follow-up", "family-b", parents=["winner"]
    )
    db.candidates["unrelated"] = _mk_cand("unrelated", "family-c")
    decision = pick_next_candidate(db)
    assert decision.candidate is not None
    assert decision.candidate.id == "follow-up"


def test_unspecified_family_not_banned():
    """Candidates tagged 'unspecified' are never banned (they're untagged)."""
    db = empty_db()
    db.phase_state.current_phase = Phase.ONE
    db.phase_state.best_score = 0.10
    for i in range(STUCK_THRESHOLD + 2):
        cid = f"c{i}"
        db.candidates[cid] = _mk_cand(cid, "unspecified", status=CandidateStatus.REJECTED)
        db.experiments[f"e{i}"] = _mk_exp(f"e{i}", cid, 0.05)
    assert stuck_families(db) == set()
