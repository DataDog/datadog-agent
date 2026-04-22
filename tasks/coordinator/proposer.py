"""Proposer subagent: brainstorms new candidates from prior results.

Triggered when the scheduler runs out of PROPOSED candidates (or when all
proposed candidates are in banned families). Reads the last N experiments,
their verdicts/rationales, and the current baseline, then asks Claude to
produce K fresh candidates in YAML form (same shape the seed loader expects).
"""

from __future__ import annotations

import datetime as _dt
import sys
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
research harness. Your job is to brainstorm {n_candidates} **genuinely
novel** candidate changes that might improve anomaly detection on the
observer pipeline.

This harness is explicitly for exploration. Threshold-tuning on the three
existing detectors (bocpd / scanmw / scanwelch) is the LEAST interesting
thing you can do here — it finds small local wins and saturates fast.

What's actually interesting:

- **New algorithms from the literature.** You have Read/Grep/Glob access.
  Look at recent AD literature (change-point detection beyond BOCPD,
  robust statistics, density ratio estimation, spectral methods, contrastive
  anomaly scoring, ensemble methods, conformal prediction, etc.). Pick
  ideas that fit the edge constraint (bounded memory, streaming, per-series
  state). Name the paper / algorithm in the description.

- **Cross-cutting infrastructure.** New correlators that combine detector
  outputs differently, new emitter logic, new feature-engineering stages,
  seasonality-aware baseline windows, per-signal-class routing.

- **Replace an existing detector's internals.** bocpd/scanmw/scanwelch
  are starting points, not sacred. Keep the detector's registration and
  whichever interface from `comp/observer/def/component.go` it already
  implements (`SeriesDetector` or `Detector`), but swap the guts for a
  different algorithm entirely (e.g. replace BOCPD with a density-ratio
  detector while keeping the `bocpd` name). This is how you integrate a
  novel algorithm cleanly.

The eval framework is OFF LIMITS. Do NOT modify `tasks/q.py`,
`tasks/libs/q`, `q.eval-scenarios` orchestration, or the testbench
registry. The three detector names and scenario list are fixed
evaluation boundaries. All innovation happens INSIDE `comp/observer/`,
behind the three existing detector names.

- **Adapted research from related systems.** Datadog's watchdog uses
  AnomalyRank. Netflix's SURUS does robust PCA on streams. NAB has a battery
  of algorithms. Cross-pollinate.

Conservative, incremental threshold tweaks are allowed but should be rare —
only propose one of those if the prior-work list suggests a specific small
win is untapped. Bias the pool toward structurally-different approaches.

## Current baseline
{yaml.safe_dump(baseline, sort_keys=False) if baseline else '(no baseline loaded)'}

## Recent experiments (chronological, oldest first)
{yaml.safe_dump(recent, sort_keys=False) if recent else '(no experiments yet)'}

## Existing approach families
{existing_families or '(none)'}
{ban_clause}
## Guidelines
- Each candidate modifies `comp/observer/` code (and potentially
  `tasks/q.py` / `tasks/libs/q` if it needs to plumb a new detector into
  the eval framework). No other paths.
- Prefer approaches informed by prior experiments (cite the experiment id
  if building on a result) — but don't let prior work trap you in local
  minima. The proposer is explicitly allowed to *pivot*: if 3 iterations
  failed to move the needle on threshold-tune, propose something
  structurally different.
- `approach_family` is a short tag. Use concrete descriptive names
  ("bocpd-robust-median", "spectral-residual-detector",
  "density-ratio-detector", "conformal-prediction-gate",
  "anomaly-rank-postfilter", "cross-series-correlator", etc.). Avoid
  generic "tune-X" labels.
- Diversity: at least {min(n_candidates, 3)} distinct families.
  Ideally all {n_candidates} are different.
- `target_components` is a list of components this candidate modifies.
  Typically one, but multi-component changes (e.g. a new feature used
  by both scan detectors) are allowed — the post-run attribution will
  flag conflicts but won't block.
- Include explicit success criteria (F1 / FP / recall targets) and
  fallback behavior.
- For new-detector proposals: describe the algorithm precisely enough
  that the implementation agent can code it in a single iteration
  (~60 tool hops). If the idea is too big to implement in that budget,
  break it into phases and propose phase 1 here.

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
        target_components = list(prop.get("target_components", []))
        # Require at least one target_component. Multi-component candidates
        # are allowed — this is an exploratory harness; novel approaches
        # (new correlators, literature-inspired gates, cross-cutting
        # features) often span multiple detectors. The post-run
        # reeval_ships attribution is best-effort: multi-component ships
        # may produce cherry-pick conflicts, in which case we report them
        # rather than block the candidate up front.
        if len(target_components) == 0:
            print(
                f"skip {cid}: target_components must be non-empty",
                file=sys.stderr,
            )
            continue
        cand = Candidate(
            id=cid,
            description=str(prop.get("description", "")).strip(),
            source="coordinator-proposed",
            target_components=target_components,
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
    from .config import CONFIG

    prompt = build_proposer_prompt(db, n_candidates, banned_families or set())
    text = sdk._run_query(
        prompt,
        model=CONFIG.model_deep,  # brainstorming is a deep-thinking task — Opus
        allowed_tools=["Read", "Grep", "Glob"],  # read-only exploration
        cwd=str(root),
    )
    proposals = parse_proposer_output(text)
    return materialize_candidates(db, proposals, root)
