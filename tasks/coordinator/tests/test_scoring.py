import json
from pathlib import Path

from coordinator.schema import Baseline, BaselineDetector, ScenarioResult
from coordinator.scoring import load_report, score_against_baseline


def _make_baseline(sha: str = "abc") -> Baseline:
    return Baseline(
        sha=sha,
        generated_at="2026-04-20T00:00:00",
        system=BaselineDetector(
            mean_f1=0.121,
            total_fps=326,
            scenarios={
                "213_pagerduty": ScenarioResult(f1=0.655, precision=0.493, recall=0.974, num_baseline_fps=1),
                "food_delivery_redis": ScenarioResult(f1=0.235, precision=0.143, recall=0.666, num_baseline_fps=4),
                "093_cloudflare": ScenarioResult(f1=0.015, precision=0.008, recall=0.841, num_baseline_fps=109),
            },
        ),
    )


def _write_report(path: Path, per_scenario: dict[str, dict], mean_f1: float):
    path.write_text(
        json.dumps(
            {
                "score": mean_f1,
                "metadata": per_scenario,
            }
        )
    )


def test_load_report(tmp_path: Path):
    report = tmp_path / "r.json"
    _write_report(
        report,
        {
            "213_pagerduty": {"f1": 0.655, "precision": 0.493, "recall": 0.974, "num_baseline_fps": 1},
        },
        mean_f1=0.655,
    )
    mean, per = load_report(report)
    assert mean == 0.655
    assert per["213_pagerduty"].num_baseline_fps == 1


def test_no_change_no_regressions(tmp_path: Path):
    baseline = _make_baseline()
    report = tmp_path / "r.json"
    _write_report(
        report,
        {
            "213_pagerduty": {"f1": 0.655, "precision": 0.493, "recall": 0.974, "num_baseline_fps": 1},
            "food_delivery_redis": {"f1": 0.235, "precision": 0.143, "recall": 0.666, "num_baseline_fps": 4},
            "093_cloudflare": {"f1": 0.015, "precision": 0.008, "recall": 0.841, "num_baseline_fps": 109},
        },
        mean_f1=0.121,
    )
    r = score_against_baseline(report, baseline)
    assert r.mean_df1 == 0
    assert r.total_dfps == 0
    assert r.strict_regressions == []
    assert r.recall_floor_violations == []
    assert r.fp_reduction_pct == 0


def test_detects_f1_regression(tmp_path: Path):
    baseline = _make_baseline()
    report = tmp_path / "r.json"
    # 213_pagerduty drops f1 from 0.655 → 0.155 (drop of 0.50). Catastrophe
    # threshold is max(catastrophe_f1_drop=0.15, base.f1 * 0.5 = 0.3275),
    # so this scenario must drop by > 0.3275 to trip. Drop of 0.50 trips
    # cleanly. Catastrophe filter intentionally doesn't catch marginal
    # drops — the LLM reviewer is responsible for subtle regressions.
    _write_report(
        report,
        {
            "213_pagerduty": {"f1": 0.155, "precision": 0.493, "recall": 0.974, "num_baseline_fps": 1},
            "food_delivery_redis": {"f1": 0.235, "precision": 0.143, "recall": 0.666, "num_baseline_fps": 4},
            "093_cloudflare": {"f1": 0.015, "precision": 0.008, "recall": 0.841, "num_baseline_fps": 109},
        },
        mean_f1=0.135,
    )
    r = score_against_baseline(report, baseline)
    assert "213_pagerduty" in r.strict_regressions


def test_detects_fp_reduction(tmp_path: Path):
    baseline = _make_baseline()
    report = tmp_path / "r.json"
    # Halve the big 109-FP scenario
    _write_report(
        report,
        {
            "213_pagerduty": {"f1": 0.655, "precision": 0.493, "recall": 0.974, "num_baseline_fps": 1},
            "food_delivery_redis": {"f1": 0.235, "precision": 0.143, "recall": 0.666, "num_baseline_fps": 4},
            "093_cloudflare": {"f1": 0.015, "precision": 0.008, "recall": 0.841, "num_baseline_fps": 54},
        },
        mean_f1=0.302,
    )
    r = score_against_baseline(report, baseline)
    assert r.total_dfps == -55
    assert r.fp_reduction_pct > 0.15


def test_recall_floor_skipped_when_baseline_low(tmp_path: Path):
    baseline = _make_baseline()
    # Baseline food_delivery_redis has recall 0.666 > 0.05, so a drop is caught.
    # But if baseline recall were <0.05 we'd skip — test that with a custom baseline.
    baseline.system.scenarios["food_delivery_redis"].recall = 0.02
    report = tmp_path / "r.json"
    _write_report(
        report,
        {
            "213_pagerduty": {"f1": 0.655, "precision": 0.493, "recall": 0.974, "num_baseline_fps": 1},
            "food_delivery_redis": {"f1": 0.235, "precision": 0.143, "recall": 0.000, "num_baseline_fps": 4},
            "093_cloudflare": {"f1": 0.015, "precision": 0.008, "recall": 0.841, "num_baseline_fps": 109},
        },
        mean_f1=0.121,
    )
    r = score_against_baseline(report, baseline)
    # food_delivery_redis dropped but baseline < 0.05, so not flagged
    assert "food_delivery_redis" not in r.recall_floor_violations
