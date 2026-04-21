"""Wall-hour budget tracking + milestone escalations.

Each iteration tallies its wall-time into `BudgetState.wall_hours_used`.
When we cross a milestone fraction (from `CONFIG.budget_milestones`) for
the first time, emit a coord-out message so the user can decide whether
to continue.

No hard halt for now — coord-out just flags. Caller (driver) may choose
to block on unacked messages in a future iteration.
"""

from __future__ import annotations

import time
from pathlib import Path
from typing import Iterator

from . import coord_out
from .config import CONFIG
from .schema import BudgetState


class WallTimer:
    """Context-manager that records elapsed wall-hours on exit."""

    def __init__(self, budget: BudgetState):
        self.budget = budget
        self._start: float = 0.0

    def __enter__(self) -> "WallTimer":
        self._start = time.monotonic()
        return self

    def __exit__(self, *_: object) -> None:
        elapsed_hours = (time.monotonic() - self._start) / 3600.0
        self.budget.wall_hours_used += elapsed_hours


def check_milestones(
    budget: BudgetState,
    root: Path,
) -> list[coord_out.CoordOutMessage]:
    """If the budget has crossed a new milestone, emit + record. Return new msgs."""
    ceiling = budget.wall_hours_ceiling
    if ceiling is None or ceiling <= 0:
        return []
    fraction = budget.wall_hours_used / ceiling
    new_msgs: list[coord_out.CoordOutMessage] = []
    for m in CONFIG.budget_milestones:
        if fraction >= m and m not in budget.milestones_notified:
            msg = coord_out.emit(
                "budget_milestone",
                f"{int(m * 100)}% of the wall-hour budget has been used "
                f"({budget.wall_hours_used:.2f}h / {ceiling:.2f}h). "
                "Decide whether to continue, adjust scope, or halt. "
                "Write instructions to `.coordinator/inbox.md`.",
                requires_ack=True,
                root=root,
            )
            budget.milestones_notified.append(m)
            new_msgs.append(msg)
    return new_msgs
