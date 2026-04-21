from pathlib import Path

import pytest

from coordinator.db import empty_db, load_db, save_db
from coordinator.schema import (
    Baseline,
    BaselineDetector,
    Candidate,
    CandidateStatus,
    Phase,
    ScenarioResult,
)


def test_empty_db_roundtrip(tmp_path: Path):
    db = empty_db()
    save_db(db, tmp_path)
    reloaded = load_db(tmp_path)
    assert reloaded.schema_version == 1
    assert reloaded.baseline is None
    assert reloaded.experiments == {}
    assert reloaded.phase_state.current_phase == Phase.ZERO


def test_baseline_roundtrip(tmp_path: Path):
    db = empty_db()
    db.baseline = Baseline(
        sha="abc123",
        generated_at="2026-04-20T00:00:00",
        detectors={
            "bocpd": BaselineDetector(
                mean_f1=0.186,
                total_fps=41,
                scenarios={
                    "213_pagerduty": ScenarioResult(
                        f1=0.0, precision=0.0, recall=0.0, num_baseline_fps=0
                    ),
                    "221_base": ScenarioResult(
                        f1=0.672, precision=1.0, recall=0.506, num_baseline_fps=0
                    ),
                },
            ),
        },
    )
    save_db(db, tmp_path)
    reloaded = load_db(tmp_path)
    assert reloaded.baseline is not None
    assert reloaded.baseline.sha == "abc123"
    assert reloaded.baseline.detectors["bocpd"].mean_f1 == pytest.approx(0.186)
    assert reloaded.baseline.detectors["bocpd"].scenarios["221_base"].f1 == pytest.approx(0.672)


def test_candidate_roundtrip(tmp_path: Path):
    db = empty_db()
    db.candidates["A-tighten"] = Candidate(
        id="A-tighten",
        description="tighten scan triple-gate thresholds",
        source="seed",
        target_components=["scanmw", "scanwelch"],
        phase=Phase.ONE,
        status=CandidateStatus.PROPOSED,
        proposed_at="2026-04-20T00:00:00",
    )
    save_db(db, tmp_path)
    reloaded = load_db(tmp_path)
    c = reloaded.candidates["A-tighten"]
    assert c.phase == Phase.ONE
    assert c.status == CandidateStatus.PROPOSED
    assert "scanmw" in c.target_components


def test_atomic_write_does_not_leave_tmp(tmp_path: Path):
    db = empty_db()
    save_db(db, tmp_path)
    tmps = list(tmp_path.glob(".coordinator/.db-*.tmp"))
    assert tmps == []
