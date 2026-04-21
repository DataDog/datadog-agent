"""Central configuration constants.

Mirrors the `config` block in ~/.claude/plans/ad-harness.allium. Modules
import `CONFIG` (an immutable dataclass) instead of hardcoding magic
numbers. Tuning any of these is a single-file change.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class Config:
    # Gates
    tau_default: float = 0.05
    recall_floor_baseline_min: float = 0.05

    # Plateau / phase exit
    plateau_patience: int = 5
    early_halt_iterations: int = 3  # M0.9 early-halt gate

    # Diversity policy (scheduler)
    stuck_threshold: int = 3
    ban_duration: int = 5

    # Review
    review_personas_phase1: int = 2
    review_unanimity_required: bool = True

    # Perf
    perf_ns_regress_limit: float = 0.10
    perf_allocs_regress_limit: float = 0.20
    perf_bytes_regress_limit: float = 0.20

    # Budget
    budget_milestones: tuple[float, ...] = (0.5, 0.8)
    default_wall_hours_ceiling: float | None = None  # None = no ceiling

    # MVP
    mvp_delta_sigma_multiplier: int = 2

    # Pending validation max age. Beyond this, a still-`pending` validation
    # is marked `abandoned` and the coordinator stops polling it (workspace
    # may have been killed, redeployed, or reimaged).
    validation_max_age_hours: float = 48.0

    # SDK retry policy (transient errors only).
    sdk_retry_max_attempts: int = 3
    sdk_retry_base_seconds: float = 2.0  # exponential backoff: 2s, 4s, 8s

    # Model routing. Deep-thinking tasks (implement / review / propose) use
    # Opus; lightweight tasks (interpret an inbox message) use Sonnet to
    # save tokens. Set to empty string to fall back to SDK default.
    model_deep: str = "claude-opus-4-7"
    model_light: str = "claude-sonnet-4-6"


CONFIG = Config()
