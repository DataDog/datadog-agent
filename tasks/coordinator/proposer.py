"""Proposer subagent: brainstorms new candidates from prior results.

Triggered when the scheduler runs out of PROPOSED candidates (or when all
proposed candidates are in banned families). Reads the last N experiments,
their verdicts/rationales, and the current baseline, then asks Claude to
produce K fresh candidates in YAML form (same shape the seed loader expects).
"""

from __future__ import annotations

import datetime as _dt
import uuid
from pathlib import Path
from typing import Any

import yaml

from .db import state_dir
from .schema import Candidate, CandidateStatus, Db, Phase


CANDIDATES_DIR = "candidates"


def _recent_experiments(db: Db, n: int = 10) -> list[dict[str, Any]]:
    """Compact summary of the last N experiments for the proposer prompt."""
    summaries: list[dict[str, Any]] = []
    for exp in list(db.experiments.values())[-n:]:
        cand = db.candidates.get(exp.candidate_id)
        review = exp.review
        summaries.append(
            {
                "experiment_id": exp.id,
                "candidate_id": exp.candidate_id,
                "approach_family": cand.approach_family if cand else "unknown",
                "status": exp.status.value,
                "score": exp.score,
                "num_baseline_fps_sum": exp.num_baseline_fps_sum,
                "approved": bool(review and review.unanimous_approve),
                "review_rationales": (
                    [d.rationale for d in review.decisions] if review else []
                ),
            }
        )
    return summaries


def _baseline_summary(db: Db) -> dict[str, Any] | None:
    if db.baseline is None:
        return None
    return {
        "sha": db.baseline.sha,
        "detectors": {
            name: {"mean_f1": d.mean_f1, "total_fps": d.total_fps}
            for name, d in db.baseline.detectors.items()
        },
    }


def build_proposer_prompt(
    db: Db,
    n_candidates: int,
    banned_families: set[str],
) -> str:
    baseline = _baseline_summary(db)
    recent = _recent_experiments(db)
    existing_families = sorted(
        {c.approach_family for c in db.candidates.values() if c.approach_family}
    )

    ban_clause = ""
    if banned_families:
        ban_clause = (
            f"\n**Forbidden approach_family values**: {sorted(banned_families)}. "
            "These families have run multiple consecutive non-improving "
            "iterations. Pick a genuinely different family.\n"
        )

    return f"""You are the proposer subagent for an anomaly-detection
improvement harness. Your job is to brainstorm {n_candidates} new candidate
changes that could reduce false positives or improve F1 on the observer
pipeline, based on the prior experiment history below.

## Current baseline
{yaml.safe_dump(baseline, sort_keys=False) if baseline else '(no baseline loaded)'}

## Recent experiments (chronological, oldest first)
{yaml.safe_dump(recent, sort_keys=False) if recent else '(no experiments yet)'}

## Existing approach families
{existing_families or '(none)'}
{ban_clause}
## Guidelines
- Each candidate must target `comp/observer/` code; no infrastructure.
- Prefer approaches informed by what PRIOR experiments revealed (cite the
  experiment id in the description if you're building on a past result).
- Assign a short `approach_family` tag (e.g. "threshold-tune",
  "anomaly-rank-filter", "correlator-new", "detector-swap",
  "scan-gate-internal", "signal-class-routing", "baseline-window-tune",
  "new-feature-gate", etc.).
- Diversity: do NOT all fall in the same family. Spread across at least
  {min(n_candidates, 3)} distinct families.
- Mention the primary target_components: one of `bocpd`, `scanmw`,
  `scanwelch`, or a correlator name.
- Include explicit success criteria (F1 / FP / recall targets) and
  fallback behavior where relevant.

## Output
Return a YAML document with a top-level key `candidates` containing a list.
Each entry has these fields: id, description, approach_family,
target_components, phase (use "1"), parent_candidates (list of past
experiment_ids that informed this candidate; may be empty). Do not include
any other prose — just the YAML block.
"""


def parse_proposer_output(yaml_text: str) -> list[dict[str, Any]]:
    """Extract the list of candidates from the proposer's YAML response."""
    import re

    # Prefer fenced ```yaml blocks
    m = re.search(r"```(?:yaml)?\s*\n(.*?)```", yaml_text, re.DOTALL)
    blob = m.group(1) if m else yaml_text
    try:
        data = yaml.safe_load(blob) or {}
    except yaml.YAMLError:
        return []
    cands = data.get("candidates") if isinstance(data, dict) else None
    return list(cands) if isinstance(cands, list) else []


def materialize_candidates(
    db: Db,
    proposals: list[dict[str, Any]],
    root: Path,
) -> list[Candidate]:
    """Turn proposer YAML entries into Candidate objects + write to candidates/ dir.

    Skips entries with duplicate IDs. Assigns fresh uuid suffix if the
    proposer used a colliding id.
    """
    out: list[Candidate] = []
    candidates_dir = state_dir(root) / CANDIDATES_DIR
    candidates_dir.mkdir(parents=True, exist_ok=True)
    now = _dt.datetime.now().isoformat(timespec="seconds")

    for prop in proposals:
        cid = prop.get("id") or f"proposed-{uuid.uuid4().hex[:8]}"
        if cid in db.candidates:
            cid = f"{cid}-{uuid.uuid4().hex[:6]}"
        cand = Candidate(
            id=cid,
            description=str(prop.get("description", "")).strip(),
            source="coordinator-proposed",
            target_components=list(prop.get("target_components", [])),
            phase=Phase(str(prop.get("phase", db.phase_state.current_phase.value))),
            status=CandidateStatus.PROPOSED,
            proposed_at=now,
            approach_family=str(prop.get("approach_family", "unspecified")),
            parent_candidates=list(prop.get("parent_candidates", [])),
        )
        # Write YAML snapshot for audit
        snapshot = {
            "id": cand.id,
            "description": cand.description,
            "source": cand.source,
            "target_components": cand.target_components,
            "phase": cand.phase.value,
            "approach_family": cand.approach_family,
            "parent_candidates": cand.parent_candidates,
            "proposed_at": cand.proposed_at,
        }
        (candidates_dir / f"{cand.id}.yaml").write_text(
            yaml.safe_dump(snapshot, sort_keys=False)
        )
        out.append(cand)
    return out


def propose(
    db: Db,
    root: Path,
    n_candidates: int = 3,
    banned_families: set[str] | None = None,
) -> list[Candidate]:
    """SDK entry point: run the proposer. Returns new candidates (unsaved).

    Caller is responsible for inserting them into db and calling save_db.
    """
    from . import sdk

    prompt = build_proposer_prompt(db, n_candidates, banned_families or set())
    query, ClaudeAgentOptions = sdk._import_sdk()
    text = sdk._collect_text(
        query(
            prompt=prompt,
            options=ClaudeAgentOptions(
                allowed_tools=["Read", "Grep", "Glob"],  # read-only exploration
                cwd=str(root),
            ),
        )
    )
    proposals = parse_proposer_output(text)
    return materialize_candidates(db, proposals, root)
