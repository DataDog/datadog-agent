"""Regenerate the metrics.md dashboard from db.yaml state.

No LLM. Pure data → markdown. Written at the end of every iteration so
the user can check harness health without attaching to tmux or the SDK.
"""

from __future__ import annotations

from pathlib import Path

from .db import state_dir
from .schema import Baseline, CandidateStatus, Db, ExperimentStatus

METRICS_NAME = "metrics.md"


def _path(root: Path) -> Path:
    return state_dir(root) / METRICS_NAME


def render(db: Db) -> str:
    lines: list[str] = []
    lines.append("# Coordinator metrics\n")
    lines.append(f"**Phase**: {db.phase_state.current_phase.value}")
    lines.append(f"**Best score (phase)**: {db.phase_state.best_score:.4f}")
    lines.append(f"**Plateau counter**: {db.phase_state.plateau_counter}")
    lines.append(f"**Iterations completed**: {len(db.iterations)}")
    lines.append("")

    if db.baseline:
        lines.append("## Baseline")
        lines.append(f"SHA: `{db.baseline.sha}`  ·  Generated: {db.baseline.generated_at}\n")
        train_set = db.split.as_train_set() if db.split else None
        lockbox_set = db.split.as_lockbox_set() if db.split else set()
        lines.append(
            "| Detector | Mean F1 | Total FPs | Prec floor (train) | Recall floor (train) | Lockbox mean |"
        )
        lines.append("|---|---:|---:|---:|---:|---:|")
        for name, d in db.baseline.detectors.items():
            prec_floor = _min_over(d, "precision", train_set)
            rec_floor = _min_over(d, "recall", train_set)
            lockbox_mean = _mean_over(d, "f1", lockbox_set) if lockbox_set else None
            lb_str = f"{lockbox_mean:.4f}" if lockbox_mean is not None else "—"
            lines.append(
                f"| {name} | {d.mean_f1:.4f} | {d.total_fps} | "
                f"{prec_floor:.3f} | {rec_floor:.3f} | {lb_str} |"
            )
        lines.append("")

    # Harness meta
    lines.append("## Harness")
    hit, tot = _review_hit_rate(db)
    rate = f"{hit}/{tot} ({100 * hit / tot:.0f}%)" if tot else "—"
    lines.append(f"- review hit rate (unanimous-approve / reviewed): {rate}")
    shipped = sum(1 for c in db.candidates.values() if c.status == CandidateStatus.SHIPPED)
    rejected = sum(1 for c in db.candidates.values() if c.status == CandidateStatus.REJECTED)
    lines.append(f"- candidates shipped/rejected: {shipped} / {rejected}")
    if db.split:
        lines.append(f"- split: train={len(db.split.train)}, lockbox={len(db.split.lockbox)} (sealed `{db.split.sealed_hash[:10]}`)")
    lines.append("")

    lines.append("## Budget")
    lines.append(f"- wall_hours_used: {db.budget.wall_hours_used:.2f}")
    if db.budget.wall_hours_ceiling is not None:
        lines.append(f"- wall_hours_ceiling: {db.budget.wall_hours_ceiling:.2f}")
    lines.append(f"- api_tokens_used: {db.budget.api_tokens_used}")
    lines.append("")

    lines.append("## Candidates")
    if not db.candidates:
        lines.append("_(none)_")
    else:
        lines.append("| ID | Phase | Status | Description |")
        lines.append("|---|---|---|---|")
        for cid, c in db.candidates.items():
            # First non-empty line only; markdown tables don't render newlines.
            first_line = next(
                (ln.strip() for ln in c.description.splitlines() if ln.strip()),
                "",
            )
            if len(first_line) > 100:
                first_line = first_line[:97] + "..."
            desc = first_line.replace("|", "\\|")
            lines.append(f"| {cid} | {c.phase.value} | {c.status.value} | {desc} |")
    lines.append("")

    # Post-ship workspace validations (lagging data points).
    lines.append("## Post-ship validations")
    if not db.validations:
        lines.append("_(none)_")
    else:
        lines.append("| ID | Experiment | Detector | Workspace | Status | Reco | Δmax |")
        lines.append("|---|---|---|---|---|---|---:|")
        # Most recent first
        recent_vals = list(db.validations.values())[-10:]
        for v in reversed(recent_vals):
            dm = f"{v.delta_max:+.3f}" if v.delta_max is not None else "—"
            reco = v.recommendation or "—"
            lines.append(
                f"| {v.id} | {v.experiment_id} | {v.detector} | {v.workspace} "
                f"| {v.status} | {reco} | {dm} |"
            )
    lines.append("")

    lines.append("## Recent experiments (last 10)")
    recent = list(db.experiments.values())[-10:]
    if not recent:
        lines.append("_(none)_")
    else:
        lines.append("| ID | Candidate | Tier | Status | Score | ΔFPs | Approved |")
        lines.append("|---|---|---|---|---:|---:|:---:|")
        for e in recent:
            score = f"{e.score:.4f}" if e.score is not None else "—"
            dfps = str(e.num_baseline_fps_sum) if e.num_baseline_fps_sum is not None else "—"
            approved = "✓" if e.review and e.review.unanimous_approve else ("✗" if e.review else "—")
            lines.append(
                f"| {e.id} | {e.candidate_id} | {e.tier.value} | {e.status.value} | "
                f"{score} | {dfps} | {approved} |"
            )
    lines.append("")

    return "\n".join(lines)


def _min_over(detector_baseline, attr: str, scope: set[str] | None) -> float:
    """Min value of `attr` over scenarios in `scope` (or all if scope=None)."""
    vals = [
        getattr(s, attr)
        for name, s in detector_baseline.scenarios.items()
        if scope is None or name in scope
    ]
    return min(vals) if vals else 0.0


def _mean_over(detector_baseline, attr: str, scope: set[str]) -> float | None:
    vals = [
        getattr(s, attr)
        for name, s in detector_baseline.scenarios.items()
        if name in scope
    ]
    if not vals:
        return None
    return sum(vals) / len(vals)


def _review_hit_rate(db: Db) -> tuple[int, int]:
    reviewed = [e for e in db.experiments.values() if e.review is not None]
    approved = sum(1 for e in reviewed if e.review and e.review.unanimous_approve)
    return approved, len(reviewed)


def regenerate(db: Db, root: Path = Path(".")) -> None:
    p = _path(root)
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(render(db))
