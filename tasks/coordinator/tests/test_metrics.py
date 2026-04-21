from pathlib import Path

from coordinator.db import empty_db, save_db, state_dir
from coordinator.metrics import regenerate, render
from coordinator.schema import (
    Baseline,
    BaselineDetector,
    Candidate,
    CandidateStatus,
    Experiment,
    ExperimentStatus,
    Phase,
    ReviewDecision,
    ReviewVerdict,
    ScenarioResult,
    Tier,
)


def test_render_empty():
    db = empty_db()
    md = render(db)
    assert "# Coordinator metrics" in md
    assert "**Phase**: 0" in md
    assert "**Iterations completed**: 0" in md


def test_render_with_baseline_and_candidate():
    db = empty_db()
    db.baseline = Baseline(
        sha="abc",
        generated_at="2026-04-20T00:00:00",
        detectors={
            "bocpd": BaselineDetector(
                mean_f1=0.186,
                total_fps=41,
                scenarios={
                    "213_pagerduty": ScenarioResult(
                        f1=0.0, precision=0.0, recall=0.0, num_baseline_fps=0
                    ),
                },
            ),
        },
    )
    db.candidates["A"] = Candidate(
        id="A",
        description="tighten scan gates",
        source="seed",
        target_components=["scanmw"],
        phase=Phase.ONE,
        status=CandidateStatus.PROPOSED,
    )
    db.experiments["exp-1"] = Experiment(
        id="exp-1",
        candidate_id="A",
        phase=Phase.ONE,
        tier=Tier.T0,
        commit_sha="deadbeef",
        config_path="",
        scenario_set=[],
        status=ExperimentStatus.DONE,
        score=0.25,
        num_baseline_fps_sum=200,
        review=ReviewVerdict(
            unanimous_approve=True,
            decisions=[
                ReviewDecision(persona="skeptic", approve=True, rationale="ok"),
                ReviewDecision(persona="conservative", approve=True, rationale="ok"),
            ],
        ),
    )
    md = render(db)
    assert "abc" in md
    assert "0.1860" in md
    assert "| A |" in md
    assert "exp-1" in md
    assert "✓" in md


def test_regenerate_writes_file(tmp_path: Path):
    db = empty_db()
    save_db(db, tmp_path)
    regenerate(db, tmp_path)
    out = state_dir(tmp_path) / "metrics.md"
    assert out.exists()
    assert "Coordinator metrics" in out.read_text()
