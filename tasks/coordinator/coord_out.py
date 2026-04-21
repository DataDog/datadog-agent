"""Coord-out channel: coordinator → user notifications.

Distinct from `inbox.md` (user → coordinator). Messages are appended to
`coord-out.md`. At iteration start the coordinator checks whether any
`requires_ack: true` messages are unacked and halts non-search work
until they are. (MVP: we emit messages but don't yet hard-block; user
acks by editing the file or running a helper.)

Message types:
  - budget_milestone: 50% / 80% wall-budget reached.
  - phase_exit: current phase plateaued.
  - strict_regression: a candidate was auto-rejected for regressing train scenarios.
  - tripwire: lockbox overfit alarm.
  - proposer_pick: (optional) noteworthy proposer choice.
"""

from __future__ import annotations

import datetime as _dt
from dataclasses import dataclass
from pathlib import Path

from . import github_out
from .db import state_dir

COORD_OUT_NAME = "coord-out.md"


@dataclass
class CoordOutMessage:
    ts: str
    type: str
    content: str
    requires_ack: bool


def _path(root: Path) -> Path:
    return state_dir(root) / COORD_OUT_NAME


def emit(
    msg_type: str,
    content: str,
    requires_ack: bool = False,
    root: Path = Path("."),
) -> CoordOutMessage:
    """Append a message to coord-out.md and (if configured) post a GitHub
    PR comment on the run-log PR.

    coord-out.md is the source of truth; GitHub is a notification channel.
    GitHub failures never raise — they're recorded in the journal.
    """
    now = _dt.datetime.now().isoformat(timespec="seconds")
    msg = CoordOutMessage(ts=now, type=msg_type, content=content, requires_ack=requires_ack)
    p = _path(root)
    p.parent.mkdir(parents=True, exist_ok=True)
    header = f"## {now}  `{msg_type}`" + ("  **[REQUIRES ACK]**" if requires_ack else "")
    with p.open("a") as f:
        f.write(f"\n{header}\n\n{content.rstrip()}\n")

    if github_out.is_configured():
        ok, detail = github_out.post(msg_type, content, requires_ack)
        if not ok:
            # Lazy import to avoid journal-importing-coord_out import cycle.
            from . import journal

            journal.append(
                "github_post_failed",
                {"type": msg_type, "detail": detail},
                root,
            )

    return msg
