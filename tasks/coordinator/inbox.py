"""Inbox + ACK protocol.

User writes free-form markdown to .coordinator/inbox.md at any time.
Coordinator at iteration start:
  1. atomic-renames inbox.md → inbox.md.reading
  2. parses, archives each message to inbox-archive/<ts>.md
  3. appends ACK entry to ack.log
  4. removes inbox.md.reading (leaves inbox.md empty / absent)

This prevents truncation races where the coordinator reads-then-clears
and a concurrent user write is lost.
"""

from __future__ import annotations

import contextlib
import datetime as _dt
import fcntl
import os
import uuid
from dataclasses import dataclass
from pathlib import Path

from .db import state_dir


INBOX_NAME = "inbox.md"
INBOX_READING = "inbox.md.reading"
INBOX_LOCK = "inbox.lock"
ARCHIVE_DIR = "inbox-archive"
ACK_LOG = "ack.log"


@contextlib.contextmanager
def inbox_lock(root: Path):
    """Exclusive flock guarding all inbox mutations.

    Serialises claim_inbox (rename → .reading), ack_and_archive (rename +
    ack append), recover_orphan_reading, and github_in._append_to_inbox.
    Without this, a gh-poll append racing a claim_inbox rename writes into
    the renamed inode and is silently archived as orphan content.
    """
    sd = state_dir(root)
    sd.mkdir(parents=True, exist_ok=True)
    lock_path = sd / INBOX_LOCK
    fp = open(lock_path, "w")
    try:
        fcntl.flock(fp.fileno(), fcntl.LOCK_EX)
        yield
    finally:
        try:
            fcntl.flock(fp.fileno(), fcntl.LOCK_UN)
        finally:
            fp.close()


@dataclass
class InboxMessage:
    id: str
    arrived_at_mtime: float
    content: str


def _inbox_path(root: Path) -> Path:
    return state_dir(root) / INBOX_NAME


def _reading_path(root: Path) -> Path:
    return state_dir(root) / INBOX_READING


def _archive_dir(root: Path) -> Path:
    return state_dir(root) / ARCHIVE_DIR


def _ack_log(root: Path) -> Path:
    return state_dir(root) / ACK_LOG


def recover_orphan_reading(root: Path = Path(".")) -> bool:
    """If a prior crash left `inbox.md.reading` behind, archive it so the
    next drain isn't silently short-circuited.

    Returns True if an orphan was recovered. Safe to call on every startup.
    """
    with inbox_lock(root):
        p = _reading_path(root)
        if not p.exists():
            return False
        archive = _archive_dir(root)
        archive.mkdir(parents=True, exist_ok=True)
        ts = _dt.datetime.now().strftime("%Y%m%dT%H%M%S")
        dest = archive / f"{ts}-orphan-reading.md"
        os.rename(p, dest)
        return True


def claim_inbox(root: Path = Path(".")) -> InboxMessage | None:
    """Atomic-rename inbox.md → inbox.md.reading; return parsed message or None.

    Returns None if inbox is empty or missing.
    Caller must call `ack_and_archive()` to complete the protocol.
    """
    with inbox_lock(root):
        src = _inbox_path(root)
        dst = _reading_path(root)
        if not src.exists():
            return None
        try:
            os.rename(src, dst)
        except FileNotFoundError:
            return None
        content = dst.read_text()
        if not content.strip():
            # empty; just remove
            dst.unlink()
            return None
        return InboxMessage(
            id=uuid.uuid4().hex[:12],
            arrived_at_mtime=dst.stat().st_mtime,
            content=content,
        )


def ack_and_archive(
    msg: InboxMessage,
    interpretation: str,
    planned_change: str,
    root: Path = Path("."),
) -> str:
    """Write ACK entry, archive the reading-file, return ack id.

    Ordering: append ack.log FIRST, then rename the reading-file. If we
    crash between ack-append and rename, recover_orphan_reading archives
    the residue on next startup — the ack is already durable and no
    duplication occurs because the reading-file is renamed (not re-read).
    If we inverted this order (rename first), a crash before ack-append
    would leave the archive without a trace — silent loss of the message
    in the ack trail.
    """
    archive = _archive_dir(root)
    archive.mkdir(parents=True, exist_ok=True)
    ts = _dt.datetime.now().strftime("%Y%m%dT%H%M%S")
    archived = archive / f"{ts}-{msg.id}.md"

    with inbox_lock(root):
        ack = _ack_log(root)
        ack.parent.mkdir(parents=True, exist_ok=True)
        now = _dt.datetime.now().isoformat(timespec="seconds")
        entry = (
            f"--- ack {msg.id} ---\n"
            f"acked_at: {now}\n"
            f"archived: {archived}\n"
            f"echo: |\n{_indent(msg.content)}\n"
            f"interpretation: {interpretation}\n"
            f"planned_change: {planned_change}\n\n"
        )
        with ack.open("a") as f:
            f.write(entry)
        os.rename(_reading_path(root), archived)
    return msg.id


def _indent(text: str, prefix: str = "  ") -> str:
    return "\n".join(prefix + line for line in text.rstrip().splitlines())
