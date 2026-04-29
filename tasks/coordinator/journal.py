"""Append-only structured event log.

Every coordinator decision writes a JSONL record. The journal is a human-
readable audit trail; db.yaml is the machine-readable source of truth.
"""

from __future__ import annotations

import datetime as _dt
import json
from pathlib import Path
from typing import Any

from .db import state_dir

JOURNAL_NAME = "journal.jsonl"


def _path(root: Path) -> Path:
    return state_dir(root) / JOURNAL_NAME


def append(event_type: str, data: dict[str, Any], root: Path = Path(".")) -> None:
    """Append a structured event to the journal."""
    p = _path(root)
    p.parent.mkdir(parents=True, exist_ok=True)
    record = {
        "ts": _dt.datetime.now().isoformat(timespec="seconds"),
        "type": event_type,
        **data,
    }
    with p.open("a") as f:
        f.write(json.dumps(record) + "\n")
