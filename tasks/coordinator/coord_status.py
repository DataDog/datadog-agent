"""Human-readable status + debug-last views.

`q.coord-status` is the thing you run when you have no idea what state
the coordinator is in: alive? paused? how much spent? what crashed last?
Single command, single screen of output.

`q.coord-debug-last` is for postmortem: find the most recent crash
artifact, print it with surrounding journal events. Saves you from
ssh-ing to the workspace and grep'ing five files.

Pure functions; the q.* tasks are thin wrappers.
"""

from __future__ import annotations

import datetime as _dt
import json
import os
import subprocess
from pathlib import Path

from .db import state_dir


# ---------------------------------------------------------------------------
# Status
# ---------------------------------------------------------------------------


def _read_session_info(root: Path) -> tuple[str, str, str]:
    """(session_name, mode, pr_num) — empty strings if not found."""
    p = state_dir(root) / "session.txt"
    if not p.exists():
        return "", "", ""
    parts = p.read_text().splitlines()
    return (
        parts[0].strip() if len(parts) >= 1 else "",
        parts[1].strip() if len(parts) >= 2 else "",
        parts[2].strip() if len(parts) >= 3 else "",
    )


def _tmux_alive(session: str) -> bool:
    if not session:
        return False
    try:
        r = subprocess.run(
            ["tmux", "has-session", "-t", session],
            capture_output=True, timeout=5,
        )
        return r.returncode == 0
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return False


def _git_state(root: Path) -> dict:
    def _g(args: list[str]) -> str:
        try:
            r = subprocess.run(
                ["git", "-C", str(root), *args],
                capture_output=True, text=True, timeout=5,
            )
            return r.stdout.strip()
        except (FileNotFoundError, subprocess.TimeoutExpired):
            return ""
    return {
        "branch": _g(["rev-parse", "--abbrev-ref", "HEAD"]),
        "sha": _g(["rev-parse", "--short", "HEAD"]),
        "subject": _g(["log", "-1", "--pretty=%s"]),
    }


def _read_last_journal_events(root: Path, n: int = 8) -> list[dict]:
    p = state_dir(root) / "journal.jsonl"
    if not p.exists():
        return []
    out: list[dict] = []
    # Tail-read by reverse-line to avoid loading multi-MB journals.
    try:
        with p.open("rb") as f:
            f.seek(0, 2)
            size = f.tell()
            chunk = min(size, 32 * 1024)
            f.seek(size - chunk)
            tail = f.read().decode("utf-8", errors="replace").splitlines()
    except OSError:
        return []
    for line in tail[-n:]:
        try:
            out.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    return out


def _budget_summary(root: Path) -> dict:
    """Quick cumulative budget read from the durable token log."""
    try:
        from . import token_log
        records = token_log.read(root)
        if not records:
            return {"tokens": 0, "cost_usd": 0.0, "by_family": {}}
        cost = token_log.cost_estimate(records)
        by_fam = token_log.sum_by_family(records)
        total = sum(r.get("input_tok", 0) + r.get("output_tok", 0) for r in records)
        return {"tokens": total, "cost_usd": cost, "by_family": by_fam}
    except Exception:  # noqa: BLE001 — telemetry must not crash status
        return {"tokens": 0, "cost_usd": 0.0, "by_family": {}}


def _latest_sdk_error(root: Path) -> tuple[Path | None, str]:
    """Return (path, tail_text) of newest sdk-errors file, or (None, '')."""
    errdir = state_dir(root) / "sdk-errors"
    if not errdir.is_dir():
        return None, ""
    files = sorted(errdir.iterdir(), key=lambda p: p.stat().st_mtime, reverse=True)
    if not files:
        return None, ""
    p = files[0]
    try:
        text = p.read_text()
    except OSError:
        return p, ""
    return p, text


def render_status(root: Path = Path(".")) -> str:
    """Pretty-print everything you need to triage the coordinator quickly."""
    out: list[str] = []
    sd = state_dir(root)
    pause_file = sd / "pause"

    session, mode, pr_num = _read_session_info(root)
    alive = _tmux_alive(session)
    git = _git_state(root)

    out.append("=" * 70)
    out.append("Coordinator status")
    out.append("=" * 70)
    out.append(f"  branch:   {git.get('branch', '?')} @ {git.get('sha', '?')}")
    out.append(f"  commit:   {git.get('subject', '?')[:80]}")
    if session:
        status = "ALIVE" if alive else "DEAD"
        out.append(f"  tmux:     {session}  [{status}]  (mode={mode}, pr=#{pr_num})")
    else:
        out.append("  tmux:     (no session info — was this started via q.coord-up?)")
    if pause_file.exists():
        try:
            reason = pause_file.read_text().strip().splitlines()[0][:120]
        except OSError:
            reason = ""
        out.append(f"  pause:    YES — {reason}")
    else:
        out.append("  pause:    no")

    budget = _budget_summary(root)
    out.append("")
    out.append("Budget (cumulative)")
    out.append(f"  tokens:    {budget['tokens']:,}")
    out.append(f"  cost:      ${budget['cost_usd']:,.2f}  (list-price)")
    for fam, totals in (budget.get("by_family") or {}).items():
        out.append(
            f"  {fam:8s}  in={totals.get('in', 0):,}  out={totals.get('out', 0):,}"
        )

    events = _read_last_journal_events(root, n=8)
    out.append("")
    out.append(f"Last {len(events)} journal events")
    for ev in events:
        ts = ev.get("ts", "?")
        typ = ev.get("type", "?")
        # Compact one-line per event.
        extras = {k: v for k, v in ev.items() if k not in ("ts", "type")}
        snippet = json.dumps(extras, default=str)[:140]
        out.append(f"  {ts}  {typ}  {snippet}")

    err_path, err_text = _latest_sdk_error(root)
    if err_path:
        out.append("")
        out.append(f"Latest sdk-error: {err_path.name}")
        # Inline last 25 lines.
        tail = "\n".join(err_text.splitlines()[-25:])
        out.append(tail)

    out.append("=" * 70)
    return "\n".join(out)


# ---------------------------------------------------------------------------
# Debug last
# ---------------------------------------------------------------------------


def find_last_crash(root: Path = Path(".")) -> dict:
    """Locate the most recent crash artifact + the journal context around it.

    Returns a dict with:
      path: Path to the sdk-errors file (or None)
      ts: ISO timestamp of the crash
      iter: iter number if discoverable
      candidate: candidate id if discoverable
      surrounding_events: journal entries within ±5 minutes
    """
    err_path, _ = _latest_sdk_error(root)
    if not err_path:
        return {"path": None, "events": []}
    # Filename: <YYYYMMDDTHHMMSS>-<purpose>.txt
    stem = err_path.stem
    ts_str = stem.split("-", 1)[0]
    try:
        ts = _dt.datetime.strptime(ts_str, "%Y%m%dT%H%M%S")
    except ValueError:
        ts = None

    # Walk journal forward, collect events within a 10-min window of ts.
    events: list[dict] = []
    journal_path = state_dir(root) / "journal.jsonl"
    if journal_path.exists() and ts is not None:
        delta = _dt.timedelta(minutes=5)
        try:
            with journal_path.open() as f:
                for line in f:
                    try:
                        ev = json.loads(line)
                    except json.JSONDecodeError:
                        continue
                    ev_ts_str = ev.get("ts", "")
                    try:
                        ev_ts = _dt.datetime.fromisoformat(ev_ts_str)
                    except (TypeError, ValueError):
                        continue
                    if abs((ev_ts - ts).total_seconds()) <= delta.total_seconds():
                        events.append(ev)
        except OSError:
            pass

    # Pull iter + candidate from the surrounding events if any
    iter_num = None
    candidate_id = None
    for ev in events:
        if iter_num is None and "iter" in ev:
            iter_num = ev.get("iter")
        if candidate_id is None:
            candidate_id = ev.get("candidate") or ev.get("candidate_id") or candidate_id
        if iter_num is not None and candidate_id is not None:
            break

    return {
        "path": err_path,
        "ts": ts.isoformat() if ts else "",
        "iter": iter_num,
        "candidate": candidate_id,
        "events": events,
    }


def render_debug_last(root: Path = Path(".")) -> str:
    info = find_last_crash(root)
    err_path = info.get("path")
    if not err_path:
        return "No sdk-errors files found. Either nothing has crashed (good) or the directory hasn't been written yet."

    out: list[str] = []
    out.append("=" * 70)
    out.append("Last crash")
    out.append("=" * 70)
    out.append(f"  file:      {err_path}")
    out.append(f"  ts:        {info.get('ts', '?')}")
    out.append(f"  iter:      {info.get('iter', '?')}")
    out.append(f"  candidate: {info.get('candidate', '?')}")
    out.append("")
    out.append("--- sdk-errors body ---")
    try:
        body = err_path.read_text()
    except OSError as e:
        body = f"(unreadable: {e})"
    out.append(body)
    out.append("")
    out.append(f"--- journal events within 5 min ({len(info.get('events', []))}) ---")
    for ev in info.get("events", []):
        ts = ev.get("ts", "?")
        typ = ev.get("type", "?")
        extras = {k: v for k, v in ev.items() if k not in ("ts", "type")}
        out.append(f"  {ts}  {typ}  {json.dumps(extras, default=str)[:200]}")
    out.append("=" * 70)
    return "\n".join(out)
