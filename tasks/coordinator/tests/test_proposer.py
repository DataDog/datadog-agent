"""Proposer parsing + materialization (no SDK required)."""

from pathlib import Path

from coordinator.db import empty_db, state_dir
from coordinator.proposer import (
    build_proposer_prompt,
    materialize_candidates,
    parse_proposer_output,
)
from coordinator.schema import (
    Baseline,
    BaselineDetector,
    ScenarioResult,
)


def test_parse_fenced_yaml():
    text = """Some preamble text.

```yaml
candidates:
  - id: cand-1
    description: first idea
    approach_family: threshold-tune
    target_components: [scanmw]
    phase: "1"
    parent_candidates: []
  - id: cand-2
    description: second idea
    approach_family: correlator-new
    target_components: [time_cluster]
    phase: "1"
    parent_candidates: [exp-0001]
```

Trailing text is ignored.
"""
    out = parse_proposer_output(text)
    assert len(out) == 2
    assert out[0]["id"] == "cand-1"
    assert out[1]["parent_candidates"] == ["exp-0001"]


def test_parse_bare_yaml():
    text = """
candidates:
  - id: c1
    description: test
    approach_family: f
    target_components: [scanmw]
    phase: "1"
"""
    out = parse_proposer_output(text)
    assert len(out) == 1


def test_parse_garbage_returns_empty():
    assert parse_proposer_output("no yaml here at all") == []
    assert parse_proposer_output("") == []


def test_materialize_deduplicates_ids(tmp_path: Path):
    state_dir(tmp_path).mkdir(parents=True)
    db = empty_db()
    from coordinator.schema import Candidate, CandidateStatus, Phase

    db.candidates["taken"] = Candidate(
        id="taken", description="", source="seed",
        target_components=[], phase=Phase.ONE,
        status=CandidateStatus.PROPOSED,
    )
    proposals = [
        {
            "id": "taken",  # collides
            "description": "proposal",
            "approach_family": "f",
            "target_components": ["scanmw"],
            "phase": "1",
        },
        {
            "id": "fresh",
            "description": "proposal",
            "approach_family": "f",
            "target_components": ["scanmw"],
            "phase": "1",
        },
    ]
    out = materialize_candidates(db, proposals, tmp_path)
    ids = [c.id for c in out]
    assert "fresh" in ids
    # Colliding id was renamed
    assert any(cid.startswith("taken-") for cid in ids)
    # Files written
    candidates_dir = state_dir(tmp_path) / "candidates"
    files = sorted(p.name for p in candidates_dir.glob("*.yaml"))
    assert len(files) == 2


def test_prompt_includes_baseline_and_bans(tmp_path: Path):
    db = empty_db()
    db.baseline = Baseline(
        sha="abc",
        generated_at="2026-04-20T00:00:00",
        detectors={
            "scanmw": BaselineDetector(
                mean_f1=0.12,
                total_fps=326,
                scenarios={
                    "s1": ScenarioResult(f1=0.1, precision=0.1, recall=0.1, num_baseline_fps=10),
                },
            )
        },
    )
    prompt = build_proposer_prompt(db, n_candidates=3, banned_families={"threshold-tune"})
    assert "abc" in prompt
    assert "threshold-tune" in prompt
    assert "Forbidden" in prompt
    assert "approach_family" in prompt
