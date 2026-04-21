"""Tests for config, DataSplit, coord_out, budget, lockbox-aware scoring."""

import time
from pathlib import Path

from coordinator.budget import WallTimer, check_milestones
from coordinator.config import CONFIG
from coordinator.coord_out import _path as coord_out_path
from coordinator.coord_out import emit
from coordinator.db import empty_db, load_db, save_db, state_dir
from coordinator.schema import (
    Baseline,
    BaselineDetector,
    BudgetState,
    DataSplit,
    ScenarioResult,
)
from coordinator.scoring import score_against_baseline
from coordinator.seed_split import compute_sealed_hash, make_split


# --- config --- -------------------------------------------------------------

def test_config_values():
    assert CONFIG.tau_default == 0.05
    assert CONFIG.plateau_patience == 5
    assert CONFIG.stuck_threshold == 3
    assert CONFIG.review_personas_phase1 == 2
    assert 0.5 in CONFIG.budget_milestones
    assert 0.8 in CONFIG.budget_milestones


# --- DataSplit --- ----------------------------------------------------------

def test_sealed_hash_deterministic_by_membership():
    h1 = compute_sealed_hash(["a", "b", "c"])
    h2 = compute_sealed_hash(["c", "b", "a"])  # order-insensitive
    assert h1 == h2


def test_sealed_hash_changes_on_membership_change():
    h1 = compute_sealed_hash(["a", "b"])
    h2 = compute_sealed_hash(["a", "b", "c"])
    assert h1 != h2


def test_make_split_sets_have_no_overlap():
    split = make_split(["a", "b"], ["c", "d"])
    assert split.as_train_set() & split.as_lockbox_set() == set()


def test_datasplit_roundtrip_in_db(tmp_path: Path):
    db = empty_db()
    db.split = make_split(["x", "y", "z"], ["p", "q"])
    save_db(db, tmp_path)
    reloaded = load_db(tmp_path)
    assert reloaded.split is not None
    assert reloaded.split.train == ["x", "y", "z"]
    assert reloaded.split.lockbox == ["p", "q"]
    assert reloaded.split.sealed_hash == db.split.sealed_hash


# --- coord_out --- ----------------------------------------------------------

def test_emit_appends(tmp_path: Path):
    state_dir(tmp_path).mkdir(parents=True)
    msg = emit("test_type", "hello", requires_ack=False, root=tmp_path)
    body = coord_out_path(tmp_path).read_text()
    assert "test_type" in body
    assert "hello" in body
    assert msg.type == "test_type"


def test_emit_requires_ack_marker(tmp_path: Path):
    state_dir(tmp_path).mkdir(parents=True)
    emit("budget_milestone", "halt?", requires_ack=True, root=tmp_path)
    body = coord_out_path(tmp_path).read_text()
    assert "[REQUIRES ACK]" in body


def test_emit_multiple_appends(tmp_path: Path):
    state_dir(tmp_path).mkdir(parents=True)
    emit("a", "first", root=tmp_path)
    emit("b", "second", root=tmp_path)
    body = coord_out_path(tmp_path).read_text()
    assert "first" in body
    assert "second" in body


# --- budget --- -------------------------------------------------------------

def test_wall_timer_accumulates():
    budget = BudgetState(
        wall_hours_used=0.0,
        wall_hours_ceiling=None,
        api_tokens_used=0,
        api_token_ceiling=None,
    )
    with WallTimer(budget):
        time.sleep(0.05)
    assert budget.wall_hours_used > 0
    assert budget.wall_hours_used < 0.001  # Way under 1 hour.


def test_check_milestones_no_ceiling_returns_empty(tmp_path: Path):
    state_dir(tmp_path).mkdir(parents=True)
    budget = BudgetState(
        wall_hours_used=5.0,
        wall_hours_ceiling=None,
        api_tokens_used=0,
        api_token_ceiling=None,
    )
    assert check_milestones(budget, tmp_path) == []


def test_check_milestones_fires_at_half(tmp_path: Path):
    state_dir(tmp_path).mkdir(parents=True)
    budget = BudgetState(
        wall_hours_used=5.0,
        wall_hours_ceiling=10.0,
        api_tokens_used=0,
        api_token_ceiling=None,
    )
    msgs = check_milestones(budget, tmp_path)
    assert len(msgs) == 1
    assert 0.5 in budget.milestones_notified
    # Idempotent — second call does not re-fire.
    assert check_milestones(budget, tmp_path) == []


def test_check_milestones_fires_half_and_eighty(tmp_path: Path):
    state_dir(tmp_path).mkdir(parents=True)
    budget = BudgetState(
        wall_hours_used=9.0,
        wall_hours_ceiling=10.0,
        api_tokens_used=0,
        api_token_ceiling=None,
    )
    msgs = check_milestones(budget, tmp_path)
    assert len(msgs) == 2
    assert set(budget.milestones_notified) == {0.5, 0.8}


# --- lockbox-aware scoring --- ---------------------------------------------

def _baseline_two_buckets() -> Baseline:
    return Baseline(
        sha="x",
        generated_at="t",
        detectors={
            "scanmw": BaselineDetector(
                mean_f1=0.3,
                total_fps=100,
                scenarios={
                    "train_a": ScenarioResult(f1=0.3, precision=0.5, recall=0.7, num_baseline_fps=10),
                    "train_b": ScenarioResult(f1=0.4, precision=0.6, recall=0.9, num_baseline_fps=20),
                    "lockbox_c": ScenarioResult(f1=0.5, precision=0.6, recall=0.9, num_baseline_fps=30),
                },
            )
        },
    )


def test_scoring_gates_only_train_scenarios(tmp_path: Path):
    import json

    report = tmp_path / "r.json"
    # Tank F1 on BOTH train_a and lockbox_c.
    report.write_text(
        json.dumps(
            {
                "score": 0.15,
                "metadata": {
                    "train_a": {"f1": 0.0, "precision": 0.5, "recall": 0.7, "num_baseline_fps": 10},
                    "train_b": {"f1": 0.4, "precision": 0.6, "recall": 0.9, "num_baseline_fps": 20},
                    "lockbox_c": {"f1": 0.0, "precision": 0.6, "recall": 0.9, "num_baseline_fps": 30},
                },
            }
        )
    )
    r = score_against_baseline(
        report,
        _baseline_two_buckets(),
        "scanmw",
        tau=0.05,
        train_scenarios={"train_a", "train_b"},
    )
    # train_a regressed — flagged. lockbox_c regressed too, but NOT gated.
    assert "train_a" in r.strict_regressions
    assert "lockbox_c" not in r.strict_regressions


def test_scoring_train_only_deltas_still_include_lockbox_view(tmp_path: Path):
    import json

    report = tmp_path / "r.json"
    report.write_text(
        json.dumps(
            {
                "score": 0.3,
                "metadata": {
                    "train_a": {"f1": 0.3, "precision": 0.5, "recall": 0.7, "num_baseline_fps": 10},
                    "lockbox_c": {"f1": 0.5, "precision": 0.6, "recall": 0.9, "num_baseline_fps": 30},
                },
            }
        )
    )
    r = score_against_baseline(
        report,
        _baseline_two_buckets(),
        "scanmw",
        tau=0.05,
        train_scenarios={"train_a"},
    )
    # Both scenarios have deltas for inspection, neither regressed.
    assert "train_a" in r.per_scenario_delta
    assert "lockbox_c" in r.per_scenario_delta
    assert r.strict_regressions == []
