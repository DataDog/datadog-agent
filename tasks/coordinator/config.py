"""Central configuration constants.

Mirrors the `config` block in ~/.claude/plans/ad-harness.allium. Modules
import `CONFIG` (an immutable dataclass) instead of hardcoding magic
numbers. Tuning any of these is a single-file change.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class Config:
    # Gates — catastrophe filters, not statistical discrimination tests.
    # N=5 baseline σ is too noisy to support per-scenario 3σ gating; F1 is
    # bounded/skewed so Gaussian assumptions don't apply. Keep gates blunt
    # and let the LLM reviewer do the nuanced work.
    catastrophe_f1_drop: float = 0.10      # scenario ΔF1 < -0.10 vs baseline → reject
    catastrophe_recall_drop: float = 0.10  # scenario Δrecall < -0.10 vs baseline → reject
    # RELATIVE catastrophe: a scenario with baseline F1=0.08 can only drop
    # 0.08 absolute — the absolute 0.10 filter never fires, yet dropping to
    # zero is still a real regression. Also reject if any scenario whose
    # baseline F1 ≥ catastrophe_relative_min drops to ≤ 50% of baseline F1.
    catastrophe_relative_min: float = 0.05   # only enforce when baseline had non-trivial F1
    catastrophe_relative_ratio: float = 0.5  # obs < 0.5 * base → reject
    recall_floor_baseline_min: float = 0.05  # only enforce recall floor when baseline had real recall

    # Total-FP ceiling: if observed total FPs across train scenarios is
    # more than fp_ceiling_ratio × baseline total, reject. The emit-
    # everything attack (rewrite detector to fire on every tick) boosts
    # recall at the cost of massive precision loss — F1 can look fine on
    # individual scenarios while total FPs 3x. The reviewer sees the
    # number but the deterministic gate should too.
    fp_ceiling_ratio: float = 1.5

    # Legacy scalar — kept for tests that still reference it; unused by the
    # live scoring path. Remove after next cleanup.
    tau_default: float = 0.05

    # Plateau / phase pivot. Effect-size aware: plateau fires when the
    # best score in a trailing window hasn't advanced by > plateau_epsilon
    # above its reference. A raw strict-greater comparison let noisy
    # +0.001 bumps keep dead-end phases alive forever while abandoning
    # real winners whose signal happened to be flat for 5 draws.
    #
    # On plateau: the driver AUTOPILOT-pivots. It bans every approach
    # family seen in the last `plateau_pivot_lookback` iterations,
    # resets the plateau_counter, and invokes the proposer with the
    # banned set so fresh candidates come from structurally different
    # directions. No user input required; inbox.md is optional steering.
    plateau_patience: int = 5
    plateau_epsilon: float = 0.01
    plateau_pivot_lookback: int = 5      # iterations back to harvest families from
    # Hard runaway stop: if we've pivoted this many times and STILL
    # haven't shipped, something is fundamentally wrong with the approach
    # surface for this problem. Halt and request human attention.
    max_pivots_before_halt: int = 4
    early_halt_iterations: int = 3  # M0.9 early-halt gate

    # Diversity policy (scheduler). K=5 per panel feedback — "competent
    # humans grind the same approach for 5-10 iterations before giving
    # up"; K=3 prevented deep exploration of promising directions.
    stuck_threshold: int = 5
    ban_duration: int = 5

    # Review: three personas running in parallel, unanimity required.
    #   leakage_auditor  — scenario/metric name leakage, threshold-snapping,
    #     implicit identity encoding, hardcoded special cases.
    #   hack_detector    — gain concentration, complexity-proportionality,
    #     proxy-gaming, prior-rejection retread.
    #   algorithm_expert — house-style enforcement (interface compliance,
    #     non-blocking ingestion, state-key pattern, license header +
    #     filename, companion test updates, helper reuse).
    # All three required because they catch orthogonal failure modes
    # (leakage / metric-gaming / convention-drift) and each has
    # evidence-cited structured output.
    # Phase 2+ personas staged in reviewer.py: Duplicate Hunter, Greybeard.
    review_personas_phase1: int = 3
    review_unanimity_required: bool = True

    # Perf
    perf_ns_regress_limit: float = 0.10
    perf_allocs_regress_limit: float = 0.20
    perf_bytes_regress_limit: float = 0.20

    # Budget. Non-None defaults so milestones actually fire and --forever
    # cannot burn unbounded spend. Operator raises the ceiling explicitly
    # if a longer run is intended. Driver refuses to start --forever with
    # either ceiling set to None.
    budget_milestones: tuple[float, ...] = (0.5, 0.8)
    default_wall_hours_ceiling: float | None = 72.0  # 3 days

    # MVP
    mvp_delta_sigma_multiplier: int = 2

    # Pending validation max age. Beyond this, a still-`pending` validation
    # is marked `abandoned` and the coordinator stops polling it (workspace
    # may have been killed, redeployed, or reimaged).
    validation_max_age_hours: float = 48.0

    # SDK retry policy (transient errors only).
    sdk_retry_max_attempts: int = 3
    sdk_retry_base_seconds: float = 2.0  # exponential backoff: 2s, 4s, 8s

    # api_token_ceiling is a PANIC BRAKE, not a budget. Real cost
    # control happens via wall-hours + per-iter cost anomaly warnings +
    # auto-pause on consecutive-anomaly streaks (see below). The token
    # ceiling exists so a runaway loop with a stuck SDK retry can't
    # silently burn five figures, but it's deliberately set far above
    # any reasonable run. Tighten via `--token-ceiling N` CLI flag if
    # you want a hard cost cap.
    api_token_ceiling: int | None = 500_000_000  # panic brake; ≈ $2.5k-7.5k

    # Per-iter cost anomaly thresholds. ANY of these triggers a
    # `cost_anomaly` tripwire PR comment for the iteration.
    # Informational by default; auto-pause kicks in only after
    # `cost_anomaly_pause_streak` consecutive anomalous iterations.
    cost_anomaly_vs_rolling_ratio: float = 2.0      # this iter > 2× rolling mean
    cost_anomaly_rolling_window: int = 5            # rolling-mean window
    cost_anomaly_absolute_usd: float = 20.0         # this iter cost > $20
    cost_anomaly_absolute_tokens: int = 5_000_000   # this iter > 5M tokens
    # Auto-pause: after N consecutive anomalous iters, touch the
    # cooperative-pause file (`.coordinator/pause`). Driver checks at
    # iter boundary; sleeps until the user removes the file.
    cost_anomaly_pause_streak: int = 3

    # Overfit detector: every N shipped candidates, evaluate all shipped
    # candidates on the lockbox (locally, not passed to any agent) and
    # compute Spearman rank-correlation between train-rank and lockbox-rank.
    # Drift below `overfit_spearman_threshold` → coord-out warning.
    # Lockbox scores are never surfaced to implementation/review prompts.
    overfit_check_every_n_ships: int = 5
    overfit_spearman_threshold: float = 0.5
    overfit_min_ships_required: int = 3

    # Model routing. Deep-thinking tasks (implement / review / propose) use
    # Opus; lightweight tasks (interpret an inbox message) use Sonnet to
    # save tokens. Set to empty string to fall back to SDK default.
    model_deep: str = "claude-opus-4-7"
    model_light: str = "claude-sonnet-4-6"


CONFIG = Config()
