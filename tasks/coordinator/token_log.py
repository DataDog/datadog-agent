"""Append-only per-SDK-call token log.

Previously token counts lived in a process-global dict accumulated across
an iteration and only persisted at end-of-iteration via `save_db`. Any
mid-iteration crash/kill (user Ctrl-C on tmux, upstream conflict halt,
ssh drop) wiped the counter — every Opus token that had been billed up
to that point vanished from the coordinator's view.

This module is the fix: every SDK call immediately appends one JSON line
to `.coordinator/tokens.jsonl` with its usage. Cumulative = sum over the
file. Durable across restarts, accurate across crashes, per-purpose
attribution for analysis.

Schema (one JSON object per line):
    {
        "ts": "2026-04-23T17:15:22",
        "iter": 8,                    # may be null if outside a run_iteration
        "model": "claude-opus-4-7",   # concrete model id
        "family": "opus",             # "opus" | "sonnet" | "unknown"
        "purpose": "implement",       # "implement"|"review"|"propose"|"inbox"
        "input": 182341,
        "output": 6120,
        "success": true,              # false when the SDK raised after some tokens
    }
"""

from __future__ import annotations

import datetime as _dt
import json
from pathlib import Path
from typing import Iterable, Iterator

from .db import state_dir

TOKEN_LOG_FILENAME = "tokens.jsonl"


def _path(root: Path) -> Path:
    return state_dir(root) / TOKEN_LOG_FILENAME


def append(
    *,
    root: Path,
    model: str,
    family: str,
    purpose: str,
    input_tok: int,
    output_tok: int,
    iter_num: int | None = None,
    success: bool = True,
) -> None:
    """Append one per-call record. Atomic on POSIX (a-mode open + short
    write). If input_tok and output_tok are both zero, still log so the
    record of the call is preserved (useful for debugging why no tokens
    were reported)."""
    p = _path(root)
    p.parent.mkdir(parents=True, exist_ok=True)
    rec = {
        "ts": _dt.datetime.now().isoformat(timespec="seconds"),
        "iter": iter_num,
        "model": model or "",
        "family": family,
        "purpose": purpose,
        "input": int(input_tok),
        "output": int(output_tok),
        "success": bool(success),
    }
    with p.open("a") as f:
        f.write(json.dumps(rec) + "\n")


def read(root: Path) -> list[dict]:
    """Return all records. Skips unparseable lines (partial writes, etc.)."""
    p = _path(root)
    if not p.exists():
        return []
    out: list[dict] = []
    with p.open() as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                out.append(json.loads(line))
            except json.JSONDecodeError:
                continue
    return out


def filter_by_iter(records: list[dict], iter_num: int) -> list[dict]:
    return [r for r in records if r.get("iter") == iter_num]


def filter_since(records: list[dict], ts_iso: str) -> list[dict]:
    return [r for r in records if (r.get("ts") or "") >= ts_iso]


# Per-million-token list prices (as of April 2026). Real bill runs higher
# because prompt-cache write tokens + retries may under-surface.
_PRICING = {
    "opus":    {"in": 15.00, "out": 75.00},
    "sonnet":  {"in":  3.00, "out": 15.00},
    "unknown": {"in":  3.00, "out": 15.00},
}


def model_family(model: str | None) -> str:
    if not model:
        return "unknown"
    m = model.lower()
    if "opus" in m:
        return "opus"
    if "sonnet" in m or "haiku" in m:
        return "sonnet"
    return "unknown"


def sum_by_family(records: Iterable[dict]) -> dict[str, dict[str, int]]:
    out: dict[str, dict[str, int]] = {
        "opus": {"in": 0, "out": 0},
        "sonnet": {"in": 0, "out": 0},
        "unknown": {"in": 0, "out": 0},
    }
    for r in records:
        fam = r.get("family") or "unknown"
        if fam not in out:
            out[fam] = {"in": 0, "out": 0}
        out[fam]["in"] += int(r.get("input", 0) or 0)
        out[fam]["out"] += int(r.get("output", 0) or 0)
    return out


def sum_total(records: Iterable[dict]) -> tuple[int, int]:
    ti = to = 0
    for r in records:
        ti += int(r.get("input", 0) or 0)
        to += int(r.get("output", 0) or 0)
    return ti, to


def cost_estimate(records: Iterable[dict]) -> float:
    totals = sum_by_family(records)
    cost = 0.0
    for fam, t in totals.items():
        prices = _PRICING.get(fam, _PRICING["unknown"])
        cost += (t["in"] / 1_000_000) * prices["in"]
        cost += (t["out"] / 1_000_000) * prices["out"]
    return cost


def format_summary(records: list[dict]) -> str:
    """Human-readable one-liner for PR comments."""
    total_in, total_out = sum_total(records)
    total_toks = total_in + total_out
    cost = cost_estimate(records)
    by_fam = sum_by_family(records)
    opus_toks = by_fam["opus"]["in"] + by_fam["opus"]["out"]
    sonnet_toks = by_fam["sonnet"]["in"] + by_fam["sonnet"]["out"]
    mix = ""
    if total_toks > 0:
        opus_pct = 100 * opus_toks / total_toks
        sonnet_pct = 100 * sonnet_toks / total_toks
        mix = f" Model mix: Opus {opus_pct:.0f}%, Sonnet {sonnet_pct:.0f}%."
    return (
        f"{total_toks:,} tokens ({total_in:,} in / {total_out:,} out), "
        f"~${cost:.2f} list-price est.{mix}"
    )
