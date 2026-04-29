"""Score a q.eval-scenarios report against a Baseline.

System-level: one report, one baseline, one ScoringResult per iter.
Per-detector standalone scoring mismeasured anything where detectors
interact and didn't generalize to correlators/filters at all. The
catalog's `defaultEnabled` set is the prod-realistic pipeline; that's
what we score.

Deterministic: no LLM, no randomness.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path

from .config import CONFIG
from .schema import Baseline, BaselineMetrics, ScenarioResult


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
    detector: str  # always "system" now; kept as a field for log/PR clarity
    mean_f1: float
    total_fps: int
    per_scenario: dict[str, ScenarioResult]
    baseline_mean_f1: float
    baseline_total_fps: int
    mean_df1: float
    total_dfps: int
    per_scenario_delta: dict[str, ScenarioDelta]

    strict_regressions: list[str]
    recall_floor_violations: list[str]
    fp_reduction_pct: float


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


def merge_best_historical(
    base: Baseline,
    shipped_per_scenario: dict[str, ScenarioResult],
) -> Baseline:
    """Element-wise best-of merge of a shipped iter into the baseline.

    Per scenario:
      - f1, precision, recall: max(prior, shipped)
      - num_baseline_fps:      min(prior, shipped) (lower is better)
    Returns a NEW Baseline (does not mutate input). Best-historical can
    only ratchet up or sideways, never down.
    """
    new_scens: dict[str, ScenarioResult] = dict(base.system.scenarios)
    for scen, shipped in shipped_per_scenario.items():
        prior = new_scens.get(scen)
        if prior is None:
            new_scens[scen] = shipped
            continue
        new_scens[scen] = ScenarioResult(
            f1=max(prior.f1, shipped.f1),
            precision=max(prior.precision, shipped.precision),
            recall=max(prior.recall, shipped.recall),
            num_baseline_fps=min(prior.num_baseline_fps, shipped.num_baseline_fps),
            f1_sigma=prior.f1_sigma,
        )
    new_mean = (
        sum(s.f1 for s in new_scens.values()) / len(new_scens)
        if new_scens else 0.0
    )
    new_total_fps = sum(s.num_baseline_fps for s in new_scens.values())
    return Baseline(
        sha=base.sha,
        generated_at=base.generated_at,
        system=BaselineMetrics(
            mean_f1=new_mean, total_fps=new_total_fps, scenarios=new_scens,
        ),
    )


def score_against_baseline(
    report_path: Path,
    baseline: Baseline,
    catastrophe_f1_drop: float = CONFIG.catastrophe_f1_drop,
    catastrophe_recall_drop: float = CONFIG.catastrophe_recall_drop,
    train_scenarios: set[str] | None = None,
) -> ScoringResult:
    """Score a system-level q.eval-scenarios report against the FROZEN baseline."""
    mean_f1, observed = load_report(report_path)
    bs = baseline.system

    deltas: dict[str, ScenarioDelta] = {}
    strict_regressions = []
    recall_violations = []
    total_observed_fps = 0
    total_baseline_fps = 0

    for s_name, obs in observed.items():
        base = bs.scenarios.get(s_name)
        in_train = train_scenarios is None or s_name in train_scenarios
        if base is None:
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
        threshold = max(catastrophe_f1_drop, base.f1 * 0.5)
        if d.df1 < -threshold:
            strict_regressions.append(s_name)
        if (base.f1 >= CONFIG.catastrophe_relative_min
                and obs.f1 < CONFIG.catastrophe_relative_ratio * base.f1):
            if s_name not in strict_regressions:
                strict_regressions.append(s_name)
        if base.recall > CONFIG.recall_floor_baseline_min and d.drecall < -catastrophe_recall_drop:
            recall_violations.append(s_name)
    fp_reduction_pct = 0.0
    if total_baseline_fps > 0:
        fp_reduction_pct = (total_baseline_fps - total_observed_fps) / total_baseline_fps

    return ScoringResult(
        detector="system",
        mean_f1=mean_f1,
        total_fps=total_observed_fps,
        per_scenario=observed,
        baseline_mean_f1=bs.mean_f1,
        baseline_total_fps=total_baseline_fps,
        mean_df1=mean_f1 - bs.mean_f1,
        total_dfps=total_observed_fps - total_baseline_fps,
        per_scenario_delta=deltas,
        strict_regressions=strict_regressions,
        recall_floor_violations=recall_violations,
        fp_reduction_pct=fp_reduction_pct,
    )
