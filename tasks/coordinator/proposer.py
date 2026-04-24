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


def _top_scenario_deltas(exp, baseline, k: int = 5) -> list[dict[str, Any]]:
    """Biggest absolute per-scenario F1 swings vs baseline for this experiment.

    Keys in per_scenario are "<detector>/<scenario>" (from _merge_scorings).
    We extract the detector-appropriate baseline scenario to compute delta.
    Surfaces the top-K by |ΔF1| so the proposer sees which scenarios
    the candidate helped or broke, not just the aggregate score_delta.
    """
    if baseline is None or not exp.per_scenario:
        return []
    rows: list[tuple[float, dict[str, Any]]] = []
    for key, sr in exp.per_scenario.items():
        # key format: "<detector>/<scenario>"; pre-multi-detector runs
        # just stored "<scenario>" — handle both.
        if "/" in key:
            det, scen = key.split("/", 1)
        else:
            det, scen = "", key
        base_det = baseline.detectors.get(det) if det else None
        if base_det is None:
            # Fall back to any detector that has this scenario (legacy rows).
            for cand_det in baseline.detectors.values():
                if scen in cand_det.scenarios:
                    base_det = cand_det
                    break
        if base_det is None or scen not in base_det.scenarios:
            continue
        base = base_det.scenarios[scen]
        delta = sr.f1 - base.f1
        rows.append((
            abs(delta),
            {
                "key": key,
                "base_f1": round(base.f1, 3),
                "obs_f1": round(sr.f1, 3),
                "df1": round(delta, 3),
                "drecall": round(sr.recall - base.recall, 3),
            },
        ))
    rows.sort(key=lambda r: r[0], reverse=True)
    return [r[1] for r in rows[:k]]


def _recent_experiments(db: Db, n: int = 10) -> list[dict[str, Any]]:
    """Compact summary of the last N experiments for the proposer prompt.

    This is the proposer's "research memory" — what it sees on iter N+1 to
    decide where to go next. Beyond aggregate score/approval, we include:
      - impl_summary: the implementation agent's DONE: line, so the proposer
        can see WHAT was tried (not just the family tag).
      - auto_reject_reason: which specific gates fired, so the proposer
        knows which scenarios a candidate broke.
      - top_scenario_deltas: the 5 biggest |ΔF1| swings vs baseline, so
        the proposer can see patterns like "RuLSIF helps low-baseline
        scenarios but kills 703_shopify" and adapt accordingly.
    """
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
                "impl_summary": exp.impl_summary,
                "auto_reject_reason": exp.auto_reject_reason,
                "top_scenario_deltas": _top_scenario_deltas(exp, db.baseline),
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
    pivot_clause = ""
    if banned_families:
        ban_clause = (
            f"\n**Forbidden approach_family values**: {sorted(banned_families)}. "
            "These families have been tried and either ran multiple "
            "consecutive non-improving iterations OR were exhausted by a "
            "phase-plateau pivot. Do NOT propose anything in these families.\n"
        )
    pivot_ban = getattr(db, "pivot_banned_families", []) or []
    if pivot_ban:
        pivot_clause = (
            f"\n**Phase-plateau pivots so far**: "
            f"{getattr(db, 'pivot_count', 0)}. "
            f"Cumulative banned: {pivot_ban}.\n"
            "The harness has ALREADY plateaued on the above direction(s). "
            "This call is a PIVOT — the previous families saturated the "
            "gain available from them. Do not propose minor variations "
            "on rejected approaches. Look at the recent-experiments block "
            "below for what the reviewers rejected and why, and propose "
            "STRUCTURALLY different mechanisms (different problem framing, "
            "different stage in the pipeline, different mathematical "
            "formulation). Lean hard on breadth over depth this round.\n"
        )

    return f"""You are the proposer subagent for an anomaly-detection
research harness. Your job is to brainstorm {n_candidates} **genuinely
novel** candidate changes that might improve anomaly detection on the
observer pipeline.

This harness is explicitly for exploration. The observer pipeline currently
has ZERO detectors and ZERO correlators — only wiring, storage, extractors,
and interfaces. Your job is to invent the detectors/correlators from
scratch. Shallow threshold-tuning on whatever you first land is the LEAST
interesting thing you can do here — saturate fast and pivot.

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

- **Register detectors in `comp/observer/impl/component_catalog.go`.**
  Implement the `Detector` or `SeriesDetector` interface from
  `comp/observer/def/component.go`, give the detector a stable name,
  add an entry to `defaultCatalog()`. The name must match the
  candidate's `target_components[0]`; the coordinator uses that name
  for `q.eval-scenarios --only <name>` and for baseline gating. Pick a
  name and keep it stable across iterations of the same family.

- **Evolve or replace without doubling work.** Once a detector is
  shipped, later candidates can refine it in place or swap the guts
  while keeping the registered name. Wholesale replacement can
  catastrophically regress scenarios the original aced (see `recent
  experiments` — replacements tend to show big +ΔF1 on scenarios the
  original missed AND big -ΔF1 on scenarios it aced).

  **CRITICAL PERF CONSTRAINT**: this is a streaming detector on
  production infrastructure. Do NOT propose "run both algorithms in
  parallel on every tick and OR the outputs" — that's 2× CPU and
  memory on every data point, unacceptable in prod. Patterns that
  preserve the original's wins WITHOUT doubling work:
  * **Post-filter on the original's detections.** The original runs
    unchanged; the novel algorithm fires ONLY when the original
    emits, and decides whether to pass it through. On the 99%+ of
    ticks where the original is silent, the cost is zero. Classic
    example: watchdog's AnomalyRank filter.
  * **Cheap pre-gate feeding a single algorithm.** A lightweight
    signal-shape test (variance, autocorrelation, bimodality) decides
    which algorithm to run for THIS stream. Constant per-tick
    overhead; picks one algo per stream, not both.
  * **Shared-feature reuse.** If both algos work from the same rolling
    stats (mean/variance/rank sketches), compute those ONCE and run
    two lightweight decision heads. The decision heads must be O(1)
    to O(k) in k = sketch size, NOT O(window) each.

  Only full-replace a detector if the recent experiments show the
  original is broadly weak across the scenario set AND the new algo
  is cheaper per-tick than the original (or the same).

- **State the perf budget.** Every candidate description must estimate
  per-tick CPU cost and memory footprint relative to the baseline
  detector. "~1.2× CPU, same memory" or "adds O(k) per tick, k=64
  sketch" — a concrete bound the reviewer can sanity-check.

The eval framework is OFF LIMITS. Do NOT modify `tasks/q.py`,
`tasks/libs/q`, `q.eval-scenarios` orchestration, or the testbench
registry. The scenario list is a fixed evaluation boundary. All
innovation happens INSIDE `comp/observer/`. Detector names are
INVENTED by you — pick one and keep it stable across iterations so
the coordinator's baseline/gate machinery lines up.

- **Adapted research from related systems.** Datadog's watchdog uses
  AnomalyRank. Netflix's SURUS does robust PCA on streams. NAB has a battery
  of algorithms. Cross-pollinate.

Conservative, incremental threshold tweaks are allowed but should be rare —
only propose one of those if the prior-work list suggests a specific small
win is untapped. Bias the pool toward structurally-different approaches.

## Current baseline
{yaml.safe_dump(baseline, sort_keys=False) if baseline else '(no baseline loaded)'}

## Recent experiments — your research memory

Read these carefully. Each entry shows what was tried (`impl_summary` =
the implementation agent's DONE: line), the outcome (`approved` /
`auto_reject_reason`), and the **biggest per-scenario F1 swings**
(`top_scenario_deltas`). Look for patterns: which scenarios does a
given algorithmic family reliably help or hurt? If the prior work shows
an attempt that catastrophically broke a specific scenario the baseline
aced, the next candidate should PRESERVE that scenario (via an additive
pattern; see Guidelines). If it broadly failed across the set, pivot to
a genuinely different family.

{yaml.safe_dump(recent, sort_keys=False) if recent else '(no experiments yet)'}

## Existing approach families
{existing_families or '(none)'}
{ban_clause}{pivot_clause}
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
  (~25 tool hops). If the idea is too big to implement in that budget,
  break it into phases and propose phase 1 here.

## Proposer-implementer split (IMPORTANT)

You are doing BOTH the brainstorming AND the detailed planning. The
implementer that runs after you is **Sonnet** with only ~25 tool turns
— it cannot design, it can only execute your plan. If your plan is
vague, Sonnet will either misimplement or crash trying to redesign in
flight. You are Opus. You have Read/Grep/Glob access. Use them to
ground your plan in the actual code BEFORE writing the plan.

For each candidate you propose, the `implementation_plan` field must
be detailed enough that a competent Sonnet instance can execute it
mechanically. Concretely:

1. **Files to modify / create** — exact paths under `comp/observer/`.
   Read the file first (before writing the plan) and cite the specific
   function or line range being replaced.
2. **Interface to preserve** — which interface (`SeriesDetector` or
   `Detector` from `comp/observer/def/component.go`), which factory
   signature, which catalog entry. Do NOT ask Sonnet to infer.
3. **Algorithm steps** — numbered, concrete. Cite papers if
   literature-derived, but the steps must be translatable to Go
   without further design choices (e.g. "maintain a ring buffer of
   size N=window/2" not "use a ring buffer").
4. **Data structures** — specify Go-level types and shapes. Not
   "a map" — "`map[seriesKey]*ringState` where ringState has the
   fields: buf []float64, head int, n int".
5. **Integration / wiring** — where in the detector's existing
   Detect() path your new logic inserts. Line ranges help.
6. **Tests** — what new tests to add (input shape + expected behavior)
   and what existing tests must still pass. If an existing test needs
   an assertion update to match the new contract, say so EXPLICITLY
   with rationale.
7. **Perf** — stated per-tick cost (O-notation + concrete constant if
   you know it) and memory footprint per series.
8. **Forbidden**: do not suggest the implementer use the `Task` tool
   to sub-delegate. Sonnet's toolset is Read/Edit/Write/Bash/Grep/Glob
   only. If you can't express the change as a plan Sonnet can follow
   with that toolset, the candidate is too big — break it into phases.

A good `implementation_plan` is 30-80 lines. A 5-line plan is a
proposal, not a plan — Sonnet will fail on it.

## Output
Return a YAML document with a top-level key `candidates` containing a list.
Each entry has these fields:
- `id`: short kebab-case tag
- `description`: 1-3 sentences on the idea + motivation
- `approach_family`: short tag (e.g. "kernel-two-sample", "spectral-residual")
- `target_components`: list of detectors this affects
- `phase`: "1"
- `parent_candidates`: list of past experiment_ids that informed this
- `implementation_plan`: the detailed plan (see above)

Do not include any other prose — just the YAML block.
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
            implementation_plan=str(prop.get("implementation_plan", "") or "").strip(),
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
        purpose="propose",
        root=root,
        allowed_tools=["Read", "Grep", "Glob"],  # read-only exploration
        cwd=str(root),
    )
    proposals = parse_proposer_output(text)
    return materialize_candidates(db, proposals, root)
