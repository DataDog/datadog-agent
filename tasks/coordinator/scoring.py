"""Score a q.eval-scenarios report against a Baseline.

Reads the main report JSON produced by `q.eval-scenarios`, extracts
per-scenario F1/P/R/FPs, diffs against baseline, and reports gates.

Deterministic: no LLM, no randomness.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path

from .config import CONFIG
from .schema import Baseline, ScenarioResult


@dataclass
class ScenarioDelta:
    scenario: str
    baseline: ScenarioResult
    observed: ScenarioResult
    df1: float
    dprecision: float
    drecall: float
    dfps: int


@dataclass
class ScoringResult:
    detector: str  # "bocpd" | "scanmw" | "scanwelch" or "config" for multi-component
    mean_f1: float
    total_fps: int
    per_scenario: dict[str, ScenarioResult]
    baseline_mean_f1: float
    baseline_total_fps: int
    mean_df1: float
    total_dfps: int
    per_scenario_delta: dict[str, ScenarioDelta]

    # Gate outcomes
    strict_regressions: list[str]  # scenario names that regressed > tau on f1
    recall_floor_violations: list[str]  # scenario names where recall dropped below baseline-τ
    fp_reduction_pct: float  # (baseline - observed) / baseline


def load_report(path: Path) -> tuple[float, dict[str, ScenarioResult]]:
    """Parse a q.eval-scenarios main report. Returns (mean_f1, per_scenario)."""
    with Path(path).open() as f:
        report = json.load(f)
    mean = float(report.get("score", 0.0))
    per_scenario = {}
    for name, m in (report.get("metadata") or {}).items():
        per_scenario[name] = ScenarioResult(
            f1=float(m.get("f1", 0.0)),
            precision=float(m.get("precision", 0.0)),
            recall=float(m.get("recall", 0.0)),
            num_baseline_fps=int(m.get("num_baseline_fps", 0)),
        )
    return mean, per_scenario


def score_against_baseline(
    report_path: Path,
    baseline: Baseline,
    detector: str,
    catastrophe_f1_drop: float = CONFIG.catastrophe_f1_drop,
    catastrophe_recall_drop: float = CONFIG.catastrophe_recall_drop,
    train_scenarios: set[str] | None = None,
) -> ScoringResult:
    """Score a q.eval-scenarios report against the FROZEN baseline.

    Gate semantics (intentionally coarse):
      - strict_regressions: scenarios where ΔF1 < -catastrophe_f1_drop
        vs baseline. This is a "did the detector break" catastrophe filter,
        not a statistical discrimination test. At N=5 baseline runs the σ
        estimate's own CI is too wide to make per-scenario 3σ gating
        meaningful, and F1 is bounded/skewed so "3σ Gaussian" doesn't apply.
        Keep the numeric gate blunt and honest; let the LLM reviewer do
        the nuanced work.
      - recall_floor_violations: scenarios where baseline recall was
        non-trivial AND observed recall dropped by > catastrophe_recall_drop.

    Gates always compare vs frozen baseline — no rolling reference. The
    rolling reference allowed a ratchet where noise-driven lucky ships
    permanently elevated the floor, letting subsequent candidates that
    were strictly worse than baseline pass the gate. Shipping is now a
    "no-worse-than-baseline" contract, period.
    """
    mean_f1, observed = load_report(report_path)
    bd = baseline.detectors[detector]

    deltas: dict[str, ScenarioDelta] = {}
    strict_regressions = []
    recall_violations = []
    total_observed_fps = 0
    total_baseline_fps = 0

    for s_name, obs in observed.items():
        base = bd.scenarios.get(s_name)
        # Only apply gates to scenarios in the train set; lockbox scenarios
        # (if any are in the report) are observed but not gated against.
        in_train = train_scenarios is None or s_name in train_scenarios
        if base is None:
            # Skip scenarios with no baseline counterpart so both totals
            # stay apples-to-apples.
            continue
        total_observed_fps += obs.num_baseline_fps
        total_baseline_fps += base.num_baseline_fps
        d = ScenarioDelta(
            scenario=s_name,
            baseline=base,
            observed=obs,
            df1=obs.f1 - base.f1,
            dprecision=obs.precision - base.precision,
            drecall=obs.recall - base.recall,
            dfps=obs.num_baseline_fps - base.num_baseline_fps,
        )
        deltas[s_name] = d
        if not in_train:
            continue
        # Catastrophe filter: an absolute ΔF1 cliff vs baseline. Chosen to
        # catch "detector broke" (−0.10 on one scenario is a real regression
        # at any realistic σ) without pretending to discriminate noise.
        if d.df1 < -catastrophe_f1_drop:
            strict_regressions.append(s_name)
        if base.recall > CONFIG.recall_floor_baseline_min and d.drecall < -catastrophe_recall_drop:
            recall_violations.append(s_name)
    fp_reduction_pct = 0.0
    if total_baseline_fps > 0:
        fp_reduction_pct = (total_baseline_fps - total_observed_fps) / total_baseline_fps

    return ScoringResult(
        detector=detector,
        mean_f1=mean_f1,
        total_fps=total_observed_fps,
        per_scenario=observed,
        baseline_mean_f1=bd.mean_f1,
        baseline_total_fps=total_baseline_fps,
        mean_df1=mean_f1 - bd.mean_f1,
        total_dfps=total_observed_fps - total_baseline_fps,
        per_scenario_delta=deltas,
        strict_regressions=strict_regressions,
        recall_floor_violations=recall_violations,
        fp_reduction_pct=fp_reduction_pct,
    )
