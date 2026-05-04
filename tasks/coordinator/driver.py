"""Coordinator driver: the long-running loop.

End-to-end iteration:
  1. Process inbox (SDK: interpret each message → ACK).
  2. Pick next candidate (deterministic scheduler).
  3. Implement candidate (SDK: coding agent writes changes in working tree).
  4. Run q.eval-scenarios on target detector; parse report; score vs baseline.
  5. Review (SDK: Phase-1 personas; unanimity required).
  6. Commit-or-revert; record to db.yaml; append journal; regenerate metrics.md.

Usage:
  PYTHONPATH=tasks python -m coordinator.driver --once
  PYTHONPATH=tasks python -m coordinator.driver --forever
  PYTHONPATH=tasks python -m coordinator.driver --once --dry-run
    (dry-run skips SDK calls and eval subprocess; walks the loop logic only)
"""

from __future__ import annotations

import argparse
import datetime as _dt
import fcntl
import sys
from pathlib import Path
from typing import Any

from . import budget as budget_mod
from . import coord_out, evaluator, git_ops, github_in, journal, metrics, overfit_check, scheduler, token_log, workspace_validate
from . import sdk as sdk_mod
from .config import CONFIG
from .db import empty_db, load_db, save_db, state_dir
from .inbox import ack_and_archive, claim_inbox
from .schema import (
    Candidate,
    CandidateStatus,
    Db,
    Experiment,
    ExperimentStatus,
    Iteration,
    Phase,
    RejectionDecision,
    RejectionStage,
    Tier,
)
from .scoring import score_against_baseline


def now_iso() -> str:
    return _dt.datetime.now().isoformat(timespec="seconds")


class ProtectedPathTamperHalt(Exception):
    """Raised when ground-truth fixtures changed between iterations."""


PAUSE_FILE = "pause"


def _pivot_on_plateau(db: Db, root: Path) -> bool:
    """Autopilot response to phase plateau: ban recent approach_families
    and tell the proposer to pivot structurally. Returns True if this was
    the Nth consecutive plateau without a ship and we should hard-halt.

    The coordinator is autonomous by design — when it plateaus, it
    redirects itself rather than waiting for human input. inbox.md is
    optional steering, not a prerequisite.
    """
    # Collect families seen in the last N iterations (the ones that just
    # ran out of steam). Add them to the persistent ban list.
    recent = []
    for it in db.iterations[-CONFIG.plateau_pivot_lookback:]:
        cand_id = it.candidate_id
        if not cand_id:
            continue
        cand = db.candidates.get(cand_id)
        if cand is None or not cand.approach_family:
            continue
        if cand.approach_family == "unspecified":
            continue
        recent.append(cand.approach_family)

    newly_banned = sorted(set(recent) - set(db.pivot_banned_families))
    for fam in newly_banned:
        db.pivot_banned_families.append(fam)

    # Reset plateau counter; don't reset best_score (want to keep the
    # bar high for the proposer to beat).
    db.phase_state.plateau_counter = 0
    db.pivot_count += 1

    # Check if there's been any ship across the whole run; if not AND
    # we've pivoted max_pivots_before_halt times, the problem is
    # structurally un-improvable with this setup → stop asking Opus.
    # Before actually halting, consult the oracle: it might see context
    # (a recent partial-fix landed, threshold tweak ready to ship) that
    # changes the call.
    any_shipped = any(
        c.status == CandidateStatus.SHIPPED for c in db.candidates.values()
    )
    hard_halt = (
        db.pivot_count >= CONFIG.max_pivots_before_halt and not any_shipped
    )
    if hard_halt:
        oracle_decision, oracle_rationale = _consult_oracle(
            db=db, root=root,
            trigger=f"plateau hard-halt after {db.pivot_count} pivots without ship",
            detail=(
                f"max_pivots_before_halt={CONFIG.max_pivots_before_halt} reached. "
                f"Banned families: {db.pivot_banned_families}. "
                f"Best score: {db.phase_state.best_score:.4f}."
            ),
        )
        if oracle_decision in ("continue", "pivot"):
            # Reset pivot_count so the next plateau triggers ban-and-pivot
            # rather than another hard-halt attempt.
            db.pivot_count = 0
            hard_halt = False
            coord_out.emit(
                f"oracle_{oracle_decision}",
                (
                    f"🔮 Oracle overrode plateau hard-halt → **{oracle_decision}** "
                    f"(resetting pivot_count to 0).\n\n"
                    f"**Rationale**: {oracle_rationale}"
                ),
                requires_ack=False,
                root=root,
            )
            journal.append(
                "oracle_decision",
                {"decision": oracle_decision, "rationale": oracle_rationale,
                 "context": "plateau_hard_halt"},
                root,
            )

    body = (
        f"Phase {db.phase_state.current_phase.value} plateaued after "
        f"{CONFIG.plateau_patience} consecutive non-improving iterations. "
        f"Best score so far: {db.phase_state.best_score:.4f}. "
        f"Pivot #{db.pivot_count}.\n\n"
        f"**Banned (newly added)**: {newly_banned or '(nothing new)'}\n"
        f"**Banned (cumulative)**: {db.pivot_banned_families}\n\n"
    )
    if hard_halt:
        body += (
            f"⚠️ `max_pivots_before_halt={CONFIG.max_pivots_before_halt}` "
            f"reached with zero ships across the run. The proposer has "
            f"exhausted {db.pivot_count} structurally-different families "
            f"and the gates have rejected all of them. This is a signal "
            f"that either: (a) the baseline is genuinely hard to improve "
            f"on the current scenario set, (b) the gates are too strict, "
            f"or (c) the candidate directions need human-level redirection. "
            f"Writing `.coordinator/inbox.md` with a specific steer will "
            f"be used by the proposer on the next run."
        )
    else:
        body += (
            f"Coordinator auto-pivoting: the proposer will generate new "
            f"candidates with the banned families filtered out. "
            f"(Write `.coordinator/inbox.md` to add a specific steer; "
            f"optional — the loop continues autonomously either way.)"
        )

    coord_out.emit(
        "phase_pivot" if not hard_halt else "phase_exit",
        body,
        requires_ack=hard_halt,
        root=root,
    )
    journal.append(
        "phase_pivot" if not hard_halt else "phase_exit",
        {
            "pivot_count": db.pivot_count,
            "newly_banned": newly_banned,
            "cumulative_banned": db.pivot_banned_families,
            "hard_halt": hard_halt,
        },
        root,
    )
    return hard_halt


def _wait_while_paused(root: Path) -> None:
    """Cooperative pause: if `.coordinator/pause` exists, sleep until
    the user removes it. Checked at iteration boundary, so nothing
    in flight is wasted (vs. Ctrl-C mid-iter which loses the impl work).
    """
    import time as _time
    pause_path = state_dir(root) / PAUSE_FILE
    if pause_path.exists():
        msg = f"[paused] {pause_path} exists; sleeping. `rm` to resume."
        print(msg, file=sys.stderr)
        journal.append("paused", {"reason": "pause file exists"}, root)
    while pause_path.exists():
        _time.sleep(30)
    # Optional: log resume only when we actually paused.
    journal.append("resumed", {}, root) if False else None


def _check_cost_anomaly(db: Db, root: Path, iter_num: int, records: list) -> None:
    """Detect this-iter cost anomalies and act:
      - Emit a `cost_anomaly` tripwire PR comment when ANY trigger fires.
      - Increment db.budget.consecutive_cost_anomalies on fire, reset on pass.
      - Touch `.coordinator/pause` after N consecutive fires.

    Triggers (any one):
      A. iter_cost > cost_anomaly_vs_rolling_ratio × rolling_mean(last N)
         — primary signal. Catches "this iter is weirdly more expensive
         than its peers", which is the actual thing we care about.
      B. iter_tokens > cost_anomaly_absolute_tokens
         — hard ceiling. Catches single-iter runaways even when there's
         no rolling baseline yet (iter 1) or the peers happen to also
         be expensive.
    """
    iter_records = token_log.filter_by_iter(records, iter_num)
    iter_in, iter_out = token_log.sum_total(iter_records)
    iter_toks = iter_in + iter_out
    iter_cost = token_log.cost_estimate(iter_records)

    if iter_toks == 0:
        # Nothing to evaluate (SDK call failed pre-API or this iter
        # didn't make any SDK calls). Don't reset the streak — leave it
        # untouched so a long string of impl-failed iters doesn't mask a
        # prior anomaly streak.
        return

    # Build rolling-mean baseline from prior iterations' costs.
    window = CONFIG.cost_anomaly_rolling_window
    prior_costs: list[float] = []
    for j in range(max(0, iter_num - window), iter_num):
        prev_records = token_log.filter_by_iter(records, j)
        prev_in, prev_out = token_log.sum_total(prev_records)
        if prev_in + prev_out == 0:
            continue
        prior_costs.append(token_log.cost_estimate(prev_records))
    rolling_mean = sum(prior_costs) / len(prior_costs) if prior_costs else 0.0

    triggers: list[str] = []
    if (
        rolling_mean > 0
        and iter_cost > CONFIG.cost_anomaly_vs_rolling_ratio * rolling_mean
    ):
        triggers.append(
            f"iter cost ${iter_cost:.2f} > "
            f"{CONFIG.cost_anomaly_vs_rolling_ratio}× rolling mean "
            f"${rolling_mean:.2f} (last {len(prior_costs)})"
        )
    if iter_toks > CONFIG.cost_anomaly_absolute_tokens:
        triggers.append(
            f"iter tokens {iter_toks:,} > absolute "
            f"{CONFIG.cost_anomaly_absolute_tokens:,}"
        )

    if not triggers:
        if db.budget.consecutive_cost_anomalies > 0:
            journal.append(
                "cost_anomaly_streak_reset",
                {"prev_streak": db.budget.consecutive_cost_anomalies},
                root,
            )
        db.budget.consecutive_cost_anomalies = 0
        return

    db.budget.consecutive_cost_anomalies += 1
    streak = db.budget.consecutive_cost_anomalies

    journal.append(
        "cost_anomaly",
        {
            "iter": iter_num,
            "iter_cost_usd": round(iter_cost, 4),
            "iter_tokens": iter_toks,
            "rolling_mean_usd": round(rolling_mean, 4),
            "rolling_window_n": len(prior_costs),
            "triggers": triggers,
            "streak": streak,
        },
        root,
    )

    pause_threshold = CONFIG.cost_anomaly_pause_streak
    will_pause = streak >= pause_threshold
    body = (
        f"**iter {iter_num}** cost: **${iter_cost:.2f}** "
        f"({iter_toks:,} tokens). "
        f"Rolling mean (last {len(prior_costs)}): "
        f"${rolling_mean:.2f}.\n\n"
        f"**Triggers**:\n"
        + "\n".join(f"- {t}" for t in triggers)
        + f"\n\n**Streak**: {streak} consecutive anomalous iter(s) "
        f"(auto-pause at {pause_threshold})."
    )
    if will_pause:
        pause_path = state_dir(root) / PAUSE_FILE
        pause_path.write_text(
            f"auto-paused {now_iso()} after {streak} consecutive cost anomalies "
            f"(latest iter {iter_num})\n"
        )
        body += (
            f"\n\n**Driver auto-paused** — wrote `{pause_path}`. "
            "Driver will sleep at the next iteration boundary until you "
            f"`rm {pause_path}` to resume. Investigate via `tail` on "
            "`.coordinator/journal.jsonl` and `tokens.jsonl`."
        )
    coord_out.emit("cost_anomaly", body, requires_ack=will_pause, root=root)


def _budget_footer(root: Path, iter_num: int | None = None, ceiling: int | None = None) -> str:
    """Compact token/cost summary for PR comments, sourced from the
    durable token log (`.coordinator/tokens.jsonl`). Decoupled from
    db.budget — tokens are authoritative from the instant they're
    appended, not from the end-of-iteration save_db.

    `ceiling` is the live token ceiling (typically `db.budget.api_token_ceiling`,
    which honors `--token-ceiling` overrides). Falls back to the static
    CONFIG default when callers haven't passed one (e.g. unit tests).
    """
    records = token_log.read(root)
    iter_records = (
        token_log.filter_by_iter(records, iter_num) if iter_num is not None else []
    )

    total_in, total_out = token_log.sum_total(records)
    total_toks = total_in + total_out
    cum_cost = token_log.cost_estimate(records)

    effective_ceiling = ceiling if ceiling is not None else CONFIG.api_token_ceiling
    pct = ""
    if effective_ceiling:
        pct = (
            f" ({100 * total_toks / effective_ceiling:.1f}% of "
            f"{effective_ceiling:,} ceiling)"
        )

    iter_line = ""
    if iter_num is not None:
        i_in, i_out = token_log.sum_total(iter_records)
        if i_in or i_out:
            i_cost = token_log.cost_estimate(iter_records)
            iter_line = (
                f"This iter: {i_in:,} in / {i_out:,} out (~${i_cost:.2f}). "
            )
        else:
            iter_line = "This iter: 0 tokens (SDK call failed before API). "

    # Model mix line: from the log, using current tokens of each family.
    by_fam = token_log.sum_by_family(records)
    opus_toks = by_fam["opus"]["in"] + by_fam["opus"]["out"]
    sonnet_toks = by_fam["sonnet"]["in"] + by_fam["sonnet"]["out"]
    if total_toks > 0:
        opus_pct = 100 * opus_toks / total_toks
        sonnet_pct = 100 * sonnet_toks / total_toks
        mix_line = f" Model mix: Opus {opus_pct:.0f}%, Sonnet {sonnet_pct:.0f}%."
    else:
        mix_line = ""

    return (
        f"\n\n---\n"
        f"**Budget**: {iter_line}"
        f"Run total: {total_toks:,} tokens "
        f"(~${cum_cost:.2f}){pct}.{mix_line}"
    )


def _capture_pre_revert_diff(root: Path, iter_num: int, candidate_id: str, reason: str) -> None:
    """Snapshot the implementer's working-tree changes + a build check
    BEFORE git_ops.revert_working_tree wipes them.

    Without this, every rejected iter destroys the implementer's actual
    output and we can't tell whether the rejection reason was justified
    or whether the implementer wrote something subtly broken. Stores
    output to .coordinator/rejected-diffs/<iter>-<candidate-id>.txt.
    """
    import datetime as _dt, subprocess
    out_dir = state_dir(root) / "rejected-diffs"
    out_dir.mkdir(parents=True, exist_ok=True)
    ts = _dt.datetime.now().strftime("%Y%m%dT%H%M%S")
    safe_id = candidate_id.replace("/", "_")[:80]
    out_path = out_dir / f"iter-{iter_num:04d}-{safe_id}-{ts}.txt"
    chunks: list[str] = [
        f"timestamp: {_dt.datetime.now().isoformat(timespec='seconds')}",
        f"iter: {iter_num}",
        f"candidate: {candidate_id}",
        f"reason: {reason}",
        "",
    ]
    # 1. git status — what was modified/created
    try:
        r = subprocess.run(
            ["git", "-C", str(root), "status", "--porcelain", "--", "comp/observer"],
            capture_output=True, text=True, timeout=10,
        )
        chunks.append("--- git status (comp/observer) ---")
        chunks.append(r.stdout or "(empty)")
    except Exception as e:
        chunks.append(f"git status failed: {e}")
    # 2. git diff of comp/observer/impl — actual content of the changes
    try:
        r = subprocess.run(
            ["git", "-C", str(root), "diff", "--", "comp/observer/impl"],
            capture_output=True, text=True, timeout=15,
        )
        chunks.append("\n--- git diff (comp/observer/impl, tracked) ---")
        # Cap at 50KB so a giant new-file diff doesn't blow up the dump
        chunks.append((r.stdout or "(empty)")[:50_000])
    except Exception as e:
        chunks.append(f"git diff failed: {e}")
    # 3. Untracked files under comp/observer/impl — usually new detector .go
    try:
        r = subprocess.run(
            ["git", "-C", str(root), "ls-files", "--others", "--exclude-standard",
             "--", "comp/observer/impl"],
            capture_output=True, text=True, timeout=10,
        )
        chunks.append("\n--- untracked under comp/observer/impl ---")
        chunks.append(r.stdout or "(none)")
        # Inline the content of each untracked .go file (cap each at 20KB)
        for p in (r.stdout or "").splitlines():
            p = p.strip()
            if not p or not p.endswith(".go"):
                continue
            try:
                with open(root / p) as f:
                    body = f.read(20_000)
                chunks.append(f"\n=== {p} ===")
                chunks.append(body)
            except OSError:
                continue
    except Exception as e:
        chunks.append(f"untracked listing failed: {e}")
    # 4. Build check — compiles?
    try:
        r = subprocess.run(
            ["go", "build", "./cmd/observer-testbench/"],
            cwd=root, capture_output=True, text=True, timeout=120,
        )
        chunks.append(f"\n--- go build returncode={r.returncode} ---")
        chunks.append((r.stdout + r.stderr)[-4000:])
    except Exception as e:
        chunks.append(f"go build failed to start: {e}")
    out_path.write_text("\n".join(chunks))
    journal.append(
        "rejected_diff_captured",
        {"iter": iter_num, "candidate": candidate_id, "path": str(out_path), "reason": reason[:200]},
        root,
    )


def _reject_candidate(
    *,
    db: Db,
    root: Path,
    it: Iteration,
    candidate: Candidate,
    experiment: Experiment,
    decision: RejectionDecision,
    body_md: str,
    coord_out_kind: str,
    requires_ack: bool = False,
    revert_tree: bool = True,
    pause_after: str | None = None,
    iter_num: int,
) -> None:
    """Single rejection sink. All `candidate dies here` paths funnel through this.

    Centralizes the side effects every rejection has historically duplicated:
    journal append, pre-revert diff capture, coord-out comment, set
    auto_reject_reason on the experiment, mark candidate REJECTED, revert
    working tree, end iter, save db, regenerate metrics. Adding `pause_after`
    handles the eval_silent_failure cooperative-pause case without duplicating
    revert/save logic.

    Replaces ~5 near-identical inline blocks scattered across iter_run.
    """
    # 1. Stamp the experiment with a compact reason (proposer reads this
    # via auto_reject_reason on the next iter's research-memory context).
    experiment.auto_reject_reason = decision.reason

    # 2. Journal append — full structured record (stage, evidence, signals).
    journal.append(
        f"rejected_{decision.stage.value}",
        {
            "iter": iter_num,
            "candidate": candidate.id,
            "stage": decision.stage.value,
            "reason": decision.reason,
            "evidence": decision.evidence,
            "advisory_signals": decision.advisory_signals,
        },
        root,
    )

    # 3. Capture diff BEFORE the revert wipes it.
    if revert_tree:
        _capture_pre_revert_diff(root, iter_num, candidate.id, decision.reason)

    # 4. Operator-facing PR comment.
    coord_out.emit(
        coord_out_kind,
        body_md + _budget_footer(root, iter_num, ceiling=db.budget.api_token_ceiling),
        requires_ack=requires_ack,
        root=root,
    )

    # 5. Mark candidate terminal + revert working tree.
    candidate.status = CandidateStatus.REJECTED
    if revert_tree:
        git_ops.revert_working_tree(root)

    # 6. Cooperative pause (used by eval_silent_failure path).
    if pause_after is not None:
        (state_dir(root) / "pause").write_text(pause_after)

    # 7. End iter and persist.
    it.ended_at = now_iso()
    db.iterations.append(it)
    save_db(db, root)
    metrics.regenerate(db, root)


def _archive_numeric_prefilter(db: Db, scoring) -> tuple[bool, dict[str, Any]]:
    """Cheap corpus-retention prefilter before asking the archive-merit model.

    Shipping gates compare against the effective best-historical baseline.
    Archival asks a different question: did this rejected diff materially beat
    the original system baseline with bounded FP growth, enough to preserve for
    manual corpus eval?
    """
    if db.baseline is None:
        return False, {"reason": "no_original_baseline"}
    base = db.baseline.system
    min_score = base.mean_f1 + 0.05
    fp_ceiling = max(int(base.total_fps * 2), base.total_fps + 25)
    passes = (
        scoring.mean_f1 >= min_score
        and scoring.total_fps <= fp_ceiling
    )
    return passes, {
        "original_baseline_mean_f1": base.mean_f1,
        "observed_mean_f1": scoring.mean_f1,
        "min_archive_score": min_score,
        "original_baseline_total_fps": base.total_fps,
        "observed_total_fps": scoring.total_fps,
        "archive_fp_ceiling": fp_ceiling,
    }


def _archive_candidate_commit(
    *,
    db: Db,
    root: Path,
    it: Iteration,
    candidate: Candidate,
    experiment: Experiment,
    scoring,
    archive_prefilter: dict[str, Any],
    archive_decision: dict[str, Any],
    rationale_lines: str,
    iter_num: int,
) -> None:
    """Commit a rejected-but-promising attempt as corpus, not as a ship."""
    candidate.status = CandidateStatus.ARCHIVED
    experiment.commit_sha = "pending"
    save_db(db, root)

    commit_sha = git_ops.commit_candidate(
        candidate.id,
        experiment.id,
        root,
        commit_prefix="coord-archive",
    )
    experiment.commit_sha = commit_sha
    save_db(db, root)
    pushed_ok, push_msg = git_ops.push_scratch_branch(root)
    journal.append(
        "push_attempted",
        {
            "iter": iter_num,
            "ok": pushed_ok,
            "msg": push_msg,
            "sha": commit_sha,
            "kind": "archive",
        },
        root,
    )
    push_tag = "pushed" if pushed_ok else f"push-failed ({push_msg[:60]})"
    print(
        f"[iter {iter_num}] ARCHIVED; committed {commit_sha[:10]} "
        f"({push_tag}) score {scoring.mean_f1:.4f}"
    )
    coord_out.emit(
        "iter_archived",
        (
            f"**iter {iter_num}** · `{candidate.id}` — **ARCHIVED** "
            f"(commit `{commit_sha[:10]}`, {push_tag}).\n\n"
            f"This is a research corpus commit, not a shipped candidate; "
            f"effective baseline was not updated.\n\n"
            f"Mean F1 vs original baseline "
            f"{archive_prefilter['original_baseline_mean_f1']:.4f} → "
            f"{scoring.mean_f1:.4f} "
            f"(Δ{scoring.mean_f1 - archive_prefilter['original_baseline_mean_f1']:+.4f}). "
            f"FPs {archive_prefilter['original_baseline_total_fps']} → "
            f"{scoring.total_fps}.\n\n"
            f"**Archive merit reviewer** "
            f"(confidence {float(archive_decision.get('confidence', 0.0)):.2f}): "
            f"{str(archive_decision.get('reason', ''))[:900]}\n\n"
            f"**Prior rejection context**:\n{rationale_lines}"
            + _budget_footer(root, iter_num, ceiling=db.budget.api_token_ceiling)
        ),
        requires_ack=False,
        root=root,
    )
    it.ended_at = now_iso()
    db.iterations.append(it)
    save_db(db, root)
    metrics.regenerate(db, root)


def _consult_oracle(
    *,
    db: Db,
    root: Path,
    trigger: str,
    detail: str,
) -> tuple[str, str]:
    """Build context, call the pre-pause oracle, return its decision.

    Encapsulates the journal-tail / db-summary plumbing so the call sites
    only have to provide the trigger description.
    """
    import json as _json
    journal_path = state_dir(root) / "journal.jsonl"
    recent_journal: list[dict] = []
    try:
        if journal_path.exists():
            with journal_path.open() as f:
                for line in f.readlines()[-50:]:
                    try:
                        recent_journal.append(_json.loads(line))
                    except _json.JSONDecodeError:
                        continue
    except OSError:
        pass

    recent_experiments: list[dict] = []
    for exp in list(db.experiments.values())[-10:]:
        cand = db.candidates.get(exp.candidate_id)
        recent_experiments.append({
            "iter": exp.id,
            "candidate_id": exp.candidate_id,
            "approach_family": cand.approach_family if cand else "unknown",
            "status": exp.status.value,
            "score": exp.score,
            "auto_reject_reason": exp.auto_reject_reason,
        })

    db_summary = {
        "iterations": len(db.iterations),
        "shipped": sum(
            1 for c in db.candidates.values() if c.status == CandidateStatus.SHIPPED
        ),
        "rejected": sum(
            1 for c in db.candidates.values() if c.status == CandidateStatus.REJECTED
        ),
        "pivot_count": db.pivot_count,
        "banned_families": list(db.pivot_banned_families),
        "best_score": db.phase_state.best_score,
        "consecutive_silent_failures": db.budget.consecutive_silent_failures,
        "consecutive_sentinel_failures": db.budget.consecutive_sentinel_failures,
        "consecutive_cost_anomalies": db.budget.consecutive_cost_anomalies,
    }

    return sdk_mod.consult_oracle_pre_pause(
        trigger=trigger,
        detail=detail,
        recent_journal=recent_journal,
        recent_experiments=recent_experiments,
        db_summary=db_summary,
        root=root,
    )


def _apply_oracle_decision(
    *,
    db: Db,
    root: Path,
    decision: str,
    rationale: str,
    streak_field: str,
) -> bool:
    """Apply the oracle's decision. Returns True if the caller should
    proceed to pause, False if the caller should skip the pause.

    - continue: reset the streak, post comment, return False (no pause).
    - pivot:    ban recent families, reset streak, post comment, False.
    - stop:     post comment, return True (pause).
    """
    journal.append(
        "oracle_decision",
        {"decision": decision, "rationale": rationale, "streak_field": streak_field},
        root,
    )
    if decision == "continue":
        setattr(db.budget, streak_field, 0)
        coord_out.emit(
            "oracle_continue",
            (
                f"🔮 Oracle decided **continue** (resetting `{streak_field}` "
                f"streak; no pause).\n\n"
                f"**Rationale**: {rationale}"
            ),
            requires_ack=False,
            root=root,
        )
        return False
    if decision == "pivot":
        # Ban recent families (same logic as plateau pivot, condensed).
        recent = []
        for it in db.iterations[-CONFIG.plateau_pivot_lookback:]:
            cand_id = it.candidate_id
            if not cand_id:
                continue
            cand = db.candidates.get(cand_id)
            if cand is None or not cand.approach_family or cand.approach_family == "unspecified":
                continue
            recent.append(cand.approach_family)
        newly_banned = sorted(set(recent) - set(db.pivot_banned_families))
        for fam in newly_banned:
            db.pivot_banned_families.append(fam)
        db.pivot_count += 1
        setattr(db.budget, streak_field, 0)
        coord_out.emit(
            "oracle_pivot",
            (
                f"🔮 Oracle decided **pivot** (banning recent families, "
                f"resetting `{streak_field}` streak; no pause).\n\n"
                f"**Newly banned**: {newly_banned or '(nothing new)'}\n"
                f"**Cumulative banned**: {db.pivot_banned_families}\n\n"
                f"**Rationale**: {rationale}"
            ),
            requires_ack=False,
            root=root,
        )
        return False
    # stop
    coord_out.emit(
        "oracle_stop",
        (
            f"🔮 Oracle decided **stop** (auto-pausing).\n\n"
            f"**Rationale**: {rationale}\n\n"
            f"`rm .coordinator/pause` to resume after investigating."
        ),
        requires_ack=True,
        root=root,
    )
    return True


def _silent_failure_diagnostics(
    *,
    root: Path,
    iter_num: int,
    candidate: Candidate,
    scoring,
    experiment: Experiment,
) -> str:
    """Compose a multi-section diagnostic dump for an eval_silent_failure.

    Goes into both the PR comment and (when pausing) the pause file so
    the operator doesn't have to ssh in to figure out why every scenario
    came back zero. Covers the four most common root causes:

      1. Candidate is correlator/filter run standalone → no upstream input.
      2. Detector not actually registered in the catalog (silently broken).
      3. Eval pipeline missing parquets or tools (env drift).
      4. Implementation produced zero detections (real algorithm bug).
    """
    import subprocess
    parts: list[str] = []
    parts.append(f"candidate.id   = {candidate.id}")
    parts.append(f"target_components = {candidate.target_components}")

    # Per-scenario zero count.
    zero_count = sum(
        1 for s in scoring.per_scenario.values()
        if s.f1 == 0 and s.precision == 0 and s.recall == 0
    )
    total = len(scoring.per_scenario)
    parts.append(f"zero_scenarios = {zero_count}/{total}")

    # Catalog registration grep.
    catalog = root / "comp" / "observer" / "impl" / "component_catalog.go"
    for name in candidate.target_components or [candidate.id]:
        try:
            r = subprocess.run(
                ["grep", "-c", f'"{name}"', str(catalog)],
                capture_output=True, text=True, timeout=5,
            )
            count = (r.stdout or "0").strip()
            parts.append(f"catalog grep '\"{name}\"' = {count} lines")
        except Exception as e:
            parts.append(f"catalog grep '\"{name}\"' failed: {e}")

    # Parquet existence sample.
    try:
        r = subprocess.run(
            ["bash", "-c",
             f"ls {root}/comp/observer/scenarios/*/parquet/*.parquet 2>/dev/null | wc -l"],
            capture_output=True, text=True, timeout=5,
        )
        parts.append(f"parquet count = {(r.stdout or '0').strip()}")
    except Exception as e:
        parts.append(f"parquet count failed: {e}")

    # Implementer summary tail.
    if experiment.impl_summary:
        parts.append(f"impl_summary tail: {experiment.impl_summary[-300:]}")

    parts.append(
        "HINT: system-level eval expects new components to register with "
        "defaultEnabled: true so the testbench picks them up under the "
        "no --only invocation. Verify the catalog edit landed."
    )
    return "\n".join(parts)


def _detectors_not_registered(detectors: list[str], root: Path) -> set[str]:
    """Return any detector names not present in component_catalog.go.

    Greps the catalog file for the detector name as a quoted string
    literal — `name: "<detector>"` is the registration shape used in
    `defaultCatalog()`. Misses on case mismatch, intentional: the
    implementer should match the candidate.target_components exactly.

    Returns the set of detectors NOT registered. Empty set = all good.
    """
    catalog_path = root / "comp" / "observer" / "impl" / "component_catalog.go"
    if not catalog_path.exists():
        # Catalog file missing entirely — every detector is "not registered."
        return set(detectors)
    try:
        text = catalog_path.read_text()
    except OSError:
        return set(detectors)
    missing: set[str] = set()
    for det in detectors:
        # Look for the name as a quoted string literal. `"<det>"` covers
        # both the `name: "<det>"` registration form and any other
        # quoted reference. Cheap, deterministic, no regex risk.
        if f'"{det}"' not in text:
            missing.add(det)
    return missing


def _sanity_pre_eval_sentinel(db: Db, root: Path, iter_num: int) -> tuple[bool, str]:
    """Run a single-scenario eval on the sentinel detector + scenario.

    Verifies the eval pipeline (build, testbench, scorer, scenarios) is
    functional before we burn SDK tokens implementing this iter's
    candidate. If sentinel F1 drops below `sanity_sentinel_min_f1`,
    something in the workspace env (deps, scenarios, binaries) is broken
    — abort the iter and ask a human.

    Returns (ok, detail). ok=True → continue iter; ok=False → halt.
    Caller is responsible for emitting a PR comment and journal event.

    Skipped (returns (True, "skipped")) if `sanity_sentinel_detector` is
    empty (e.g. blank-slate runs).
    """
    detector = CONFIG.sanity_sentinel_detector
    scenario = CONFIG.sanity_sentinel_scenario
    if not detector or not scenario:
        return True, "skipped"
    if not db.baseline:
        return True, "no_baseline"
    # Sentinel only meaningful pre-first-ship: once any candidate has
    # modified detector code in-tree, "vanilla bocpd reproduces baseline
    # F1" stops being a valid invariant. Post-ship, the in-tree bocpd is
    # not the bocpd that was baselined, so divergence is expected, not a
    # signal of broken eval. Silent_failure detection (post-eval all-zero
    # check) covers the env-broken case after first ship.
    any_ship = any(
        c.status == CandidateStatus.SHIPPED for c in db.candidates.values()
    )
    if any_ship:
        return True, "skipped_post_first_ship"
    if scenario not in db.baseline.system.scenarios:
        return True, f"sentinel_scenario_not_in_baseline ({scenario})"
    expected_f1 = db.baseline.system.scenarios[scenario].f1
    if db.baseline.system.mean_f1 < 0.05 or expected_f1 < 0.05:
        return True, (
            f"skipped_blank_baseline (system mean_f1={db.baseline.system.mean_f1:.3f}, "
            f"sentinel_scenario_f1={expected_f1:.3f})"
        )
    # Relative tolerance: sentinel must reproduce within 0.10 absolute of
    # the baseline's recorded F1. Earlier `max(absolute_floor, expected-0.05)`
    # broke once baselines were re-measured at lower F1 — the absolute floor
    # of 0.90 was a leftover assumption from the original Apr-23 baseline
    # where bocpd/703_shopify scored 0.987. Once a baseline is regenerated
    # against drifted upstream code (eg expected_f1=0.65), the absolute floor
    # falsely failed the sentinel every iter.
    min_f1 = max(0.0, expected_f1 - 0.10)

    report_path = state_dir(root) / "sanity" / f"iter-{iter_num:04d}-sentinel.json"
    scenario_dir = state_dir(root) / "sanity" / f"iter-{iter_num:04d}"
    run = evaluator.run_scenarios(
        detector=detector,
        report_path=report_path,
        scenario_output_dir=scenario_dir,
        root=root,
        timeout_seconds=CONFIG.sanity_sentinel_timeout_seconds,
        rebuild=True,
        scenarios=scenario,
    )
    if not run.ok:
        # Build/eval crash: env is definitely broken.
        return False, (
            f"sentinel_eval_crashed rc={run.returncode} "
            f"stderr={(run.stderr or '')[:300]}"
        )
    try:
        from .scoring import load_report
        _mean, per_scen = load_report(run.report_path)
    except (OSError, ValueError, KeyError) as exc:
        return False, f"sentinel_report_unreadable: {exc}"
    sr = per_scen.get(scenario)
    if sr is None:
        return False, f"sentinel_f1_missing in {run.report_path.name}"
    f1 = sr.f1
    if f1 < min_f1:
        return False, (
            f"sentinel F1 {f1:.3f} < threshold {min_f1:.3f} "
            f"(baseline expected {expected_f1:.3f}); workspace eval likely broken"
        )
    return True, f"sentinel_ok (f1={f1:.3f} >= {min_f1:.3f})"


def _sanity_zero_detections(scoring) -> str | None:
    """Detect "all zeros across all scenarios" — almost-certainly silent
    eval failure rather than algorithmic regression.

    Returns a string describing the failure, or None if no anomaly.
    """
    if not CONFIG.sanity_zero_detections_check:
        return None
    if not scoring or not scoring.per_scenario:
        return "scoring_empty (no per-scenario results)"
    all_zero = all(
        sr.f1 == 0.0 and sr.precision == 0.0 and sr.recall == 0.0
        for sr in scoring.per_scenario.values()
    )
    if all_zero and len(scoring.per_scenario) >= 3:
        return f"all-zero F1/precision/recall across {len(scoring.per_scenario)} scenarios"
    return None


def _snapshot_protected_paths(db: Db, root: Path) -> None:
    """Record current content hashes of scoring labels into db.yaml."""
    db.protected_path_hashes = {
        p: git_ops.tree_hash(root, p) for p in git_ops.PROTECTED_PATHS
    }


def _verify_protected_paths(db: Db, root: Path) -> None:
    """Abort iteration if any PROTECTED_PATH content hash changed.

    First-run bootstrap: no recorded hashes → take a snapshot and proceed.
    Subsequent runs compare and halt on mismatch.
    """
    if not db.protected_path_hashes:
        _snapshot_protected_paths(db, root)
        save_db(db, root)
        return
    for p in git_ops.PROTECTED_PATHS:
        current = git_ops.tree_hash(root, p)
        recorded = db.protected_path_hashes.get(p, "")
        if recorded and current != recorded:
            msg = (
                f"Protected path '{p}' changed between iterations "
                f"(recorded {recorded[:12]} vs current {current[:12]}). "
                "This is the scoring label set; any change invalidates "
                "mid-run F1 measurements. Halting. If the change is a "
                "legitimate upstream merge, delete db.protected_path_hashes "
                "and restart to re-baseline."
            )
            coord_out.emit("tripwire", msg, requires_ack=True, root=root)
            journal.append(
                "protected_path_tamper",
                {"path": p, "recorded": recorded, "current": current},
                root,
            )
            raise ProtectedPathTamperHalt(msg)


# ---------------------------------------------------------------------------
# Inbox handling (TODO #1)
# ---------------------------------------------------------------------------

def process_inbox(db: Db, root: Path, dry_run: bool) -> list[str]:
    ack_ids: list[str] = []
    while True:
        msg = claim_inbox(root)
        if msg is None:
            break
        if dry_run:
            interpretation = "[dry-run] SDK not called"
            planned_change = "[dry-run] no change"
        else:
            from . import sdk

            interpretation, planned_change = sdk.interpret_inbox_message(msg.content, root=root)
        ack_id = ack_and_archive(msg, interpretation, planned_change, root)
        journal.append(
            "inbox_ack",
            {
                "ack_id": ack_id,
                "interpretation": interpretation,
                "planned_change": planned_change,
            },
            root,
        )
        ack_ids.append(ack_id)
        # Feed the steering directive into db.user_steering_active so the
        # proposer sees it on the next invocation. Without this, inbox
        # interpretations were journaled but never reached the proposer
        # — operator steering had no effect on candidate generation.
        # Skip CI-noise interpretations (planned_change starts with
        # "no action" or similar) so the steering list doesn't fill up
        # with bot CI noise.
        if interpretation and not _looks_like_no_op_steering(planned_change):
            entry = f"[{ack_id}] {interpretation.strip()[:600]}"
            db.user_steering_active.append(entry)
            # Cap the list at the most recent 5 directives so old steering
            # doesn't pile up indefinitely.
            db.user_steering_active = db.user_steering_active[-5:]
    return ack_ids


def _looks_like_no_op_steering(planned_change: str) -> bool:
    """True if the inbox interpreter said no coordinator action is needed.

    These are typically CI noise (size gates, lint comments) that the
    interpreter correctly identified as not requiring behavioural change.
    Don't pollute db.user_steering_active with them.
    """
    if not planned_change:
        return True
    lo = planned_change.strip().lower()
    return (
        lo.startswith("no action")
        or lo.startswith("no coordinator")
        or "no coordinator behaviour change" in lo
        or "no coordinator behavior change" in lo
        or "no action is needed" in lo
    )


# ---------------------------------------------------------------------------
# Scheduling
# ---------------------------------------------------------------------------

MAX_PROPOSER_ATTEMPTS = 1  # per iteration; avoid infinite SDK loops


class UpstreamConflictHalt(Exception):
    """Raised when sync_from_upstream aborts on conflict.

    Propagates out of the iteration body and the --forever loop so the
    coordinator exits cleanly. Next invocation (after human rebase) picks
    up from the restored state.
    """


class BudgetCeilingHalt(Exception):
    """Raised when CONFIG.api_token_ceiling is exceeded."""


class GhChannelDeadHalt(Exception):
    """Raised when the gh CLI has failed too many consecutive times.

    User-facing channel (PR comments) is the only way the operator
    learns about milestones, tripwires, and phase exits. If gh auth
    expires on day 2 and we keep iterating, we burn tokens with no
    user visibility. Halt instead.
    """


_gh_warned = False


def _check_gh_health(root: Path) -> None:
    """Escalate if gh post/poll have been failing. Emits a local tripwire
    via coord_out (writes to .coordinator/coord-out.md regardless of gh
    status) at WARN threshold; raises GhChannelDeadHalt at HALT threshold.
    """
    global _gh_warned
    from . import github_out

    errors = github_out.gh_consecutive_errors()
    worst = max(errors.values()) if errors else 0
    if worst >= github_out.GH_HALT_THRESHOLD:
        raise GhChannelDeadHalt(
            f"gh consecutive errors reached halt threshold: {errors}"
        )
    if worst >= github_out.GH_WARN_THRESHOLD and not _gh_warned:
        coord_out.emit(
            "tripwire",
            f"gh CLI has failed {worst} consecutive times ({errors}). "
            "The user-facing PR-comment channel may be dead (auth expired, "
            "network, rate limit). Coordinator will continue but halt at "
            f"{github_out.GH_HALT_THRESHOLD} consecutive failures.",
            requires_ack=True,
            root=root,
        )
        _gh_warned = True
    elif worst == 0:
        # channel recovered; re-arm the warning for any future outage.
        _gh_warned = False


def pick_next_candidate_with_proposal(
    db: Db,
    root: Path,
    dry_run: bool,
) -> Candidate | None:
    """Apply scheduler diversity policy; invoke proposer if queue is dry.

    If the scheduler returns None (no PROPOSED candidates in allowed families),
    run the proposer once to generate fresh candidates, then retry.
    """
    decision = scheduler.pick_next_candidate(db)
    if decision.candidate is not None:
        if decision.banned_families:
            journal.append(
                "scheduler_banned_families",
                {"banned": sorted(decision.banned_families)},
                root,
            )
        return decision.candidate

    if dry_run:
        return None

    # Queue dry — proposer generates new candidates.
    journal.append(
        "proposer_invoked",
        {"reason": decision.reason, "banned": sorted(decision.banned_families)},
        root,
    )
    try:
        from . import proposer

        new = proposer.propose(db, root, n_candidates=3, banned_families=decision.banned_families)
    except Exception as e:
        journal.append("proposer_failed", {"error": str(e)}, root)
        return None

    for cand in new:
        db.candidates[cand.id] = cand
        journal.append(
            "candidate_proposed",
            {
                "id": cand.id,
                "approach_family": cand.approach_family,
                "parent_candidates": cand.parent_candidates,
            },
            root,
        )
    save_db(db, root)

    # Retry selection now that we have fresh candidates.
    decision = scheduler.pick_next_candidate(db)
    return decision.candidate


# ---------------------------------------------------------------------------
# Detector selection for a candidate
# ---------------------------------------------------------------------------

# Each candidate targets one "primary" detector for eval/review. For
# multi-detector candidates (e.g. "tighten scan triple-gate" touches both
# scanmw and scanwelch), we pick the first listed.

def _recent_same_family(db: Db, candidate: Candidate, limit: int = 5) -> list[dict]:
    """Pull up to `limit` most-recent same-family experiments as small dicts
    for the implementation agent's prompt. Safe when db is sparse."""
    fam = candidate.approach_family
    out: list[dict] = []
    for exp in reversed(list(db.experiments.values())):
        c = db.candidates.get(exp.candidate_id)
        if c is None or c.approach_family != fam:
            continue
        rationales = (
            [d.rationale for d in exp.review.decisions] if exp.review else []
        )
        base_mean = db.baseline.system.mean_f1 if db.baseline else None
        score_delta = (exp.score - base_mean) if (exp.score is not None and base_mean is not None) else None
        out.append(
            {
                "id": exp.id,
                "approach_family": fam,
                "approved": bool(exp.review and exp.review.unanimous_approve),
                "score_delta": score_delta,
                "rationales": rationales,
            }
        )
        if len(out) >= limit:
            break
    out.reverse()  # oldest first for readability
    return out


def known_detectors(db: Db) -> tuple[str, ...]:
    """Canonical detectors known to the current run.

    Under system-level eval the baseline is one entry; this no longer
    enumerates per-detector baseline records. Returns a static fallback
    of canonical detector names, used for the proposer's "what's
    already out there" hint. Empty on blank-mode runs.
    """
    if db.baseline is None:
        return ()
    return ("bocpd", "scanmw", "scanwelch")


def relevant_detectors(candidate: Candidate, db: Db) -> list[str]:
    """Which detectors' F1 do we measure to decide if this candidate shipped?

    A candidate can modify any file under comp/observer/. But scoring runs
    per-detector, and the panel review caught this: silently defaulting to
    ONE detector (previously scanmw) meant candidates modifying e.g. bocpd
    internals got scored against scanmw's unaffected output — ΔF1≈0 by
    construction, "improvement" or "regression" both invisible.

    Policy (with `db` to keep blank-mode runs honest):
      - Take the union of (candidate.target_components ∩ known) PLUS any
        novel target names not yet in baseline (the implementer creates
        them and registers in component_catalog.go; eval `--only <name>`
        will work after registration). This is what blank-mode runs need
        — every candidate is novel, so we must keep its proposed name(s).
      - If target_components is empty (correlator-only / pipeline-only
        changes), return the full set of known detectors so we measure
        whichever one the change might have affected.

    Always returns a non-empty list (driver code assumes that). On a
    blank run with empty target_components and no baseline detectors,
    falls back to ['<candidate-id>'] as a last resort so eval at least
    has something to point `--only` at.
    """
    target = list(candidate.target_components or [])
    if target:
        return target
    known = known_detectors(db)
    if known:
        return list(known)
    # Truly blank with no targets: use the candidate id as the detector
    # name so eval has something to dispatch with.
    return [candidate.id]


def primary_detector(candidate: Candidate, db: Db) -> str:
    """Deprecated single-detector view. Kept for callers that print a
    single string (metrics, log lines). Returns the first relevant detector.
    """
    return relevant_detectors(candidate, db)[0]




# ---------------------------------------------------------------------------
# Full iteration (TODOs 2–5)
# ---------------------------------------------------------------------------

def run_iteration(db: Db, root: Path, dry_run: bool = False) -> Db:
    iter_num = len(db.iterations)
    it = Iteration(number=iter_num, started_at=now_iso())

    # Protected-path integrity check: verify ground-truth fixtures haven't
    # been tampered with since the last iteration. Upstream merges may
    # legitimately change them (infrequent) — the coordinator halts on
    # detection and a human decides whether to re-snapshot.
    if not dry_run:
        _verify_protected_paths(db, root)

    with budget_mod.WallTimer(db.budget):
        _run_iteration_body(db, root, dry_run, iter_num, it)

    # Re-snapshot protected-path hashes AFTER the iteration so the NEXT
    # iteration's verify sees whatever upstream merges or human edits
    # happened during this iteration as the new baseline. Agent edits were
    # made impossible by the Edit/Write deny-list hook; if somehow one
    # snuck through, it's caught on the next iteration's pre-check.
    if not dry_run:
        _snapshot_protected_paths(db, root)
        save_db(db, root)

    if not dry_run:
        # Tokens are durably appended to .coordinator/tokens.jsonl by the
        # SDK on every API call — we just re-read them here to update the
        # db.yaml cached total (for metrics.md + milestone check) and
        # emit the ceiling-halt if we've crossed it. Source of truth is
        # the log file, not db.budget.
        records = token_log.read(root)
        # Just-completed iteration is at index len-1 (was appended inside
        # _run_iteration_body). Tokens are tagged with the iter_num they
        # ran under, which is len-1.
        just_ran_iter = len(db.iterations) - 1
        iter_in, iter_out = token_log.sum_total(
            token_log.filter_by_iter(records, just_ran_iter)
        )
        total_in, total_out = token_log.sum_total(records)
        db.budget.api_tokens_used = total_in + total_out
        if iter_in or iter_out:
            journal.append(
                "tokens_used",
                {
                    "iter": just_ran_iter,
                    "iter_input": iter_in,
                    "iter_output": iter_out,
                    "iter_cost_usd": round(
                        token_log.cost_estimate(
                            token_log.filter_by_iter(records, just_ran_iter)
                        ),
                        4,
                    ),
                    "cumulative_input": total_in,
                    "cumulative_output": total_out,
                    "cumulative_cost_usd": round(token_log.cost_estimate(records), 4),
                },
                root,
            )

        # Per-iter cost anomaly: tripwire (and possibly auto-pause) when
        # this iteration's spend looks abnormal versus its peers. Three
        # triggers; ANY fires a tripwire. Streak of N consecutive fires
        # → touch the cooperative-pause file so user must intervene.
        _check_cost_anomaly(
            db, root, iter_num=just_ran_iter, records=records,
        )

        new_msgs = budget_mod.check_milestones(db.budget, root)
        for m in new_msgs:
            journal.append(
                "budget_milestone_emitted",
                {"type": m.type, "requires_ack": m.requires_ack},
                root,
            )

        # Hard token ceiling: halt when cumulative log sum exceeds the
        # LIVE ceiling on db.budget (which honors --token-ceiling overrides).
        live_ceiling = db.budget.api_token_ceiling
        if (
            live_ceiling is not None
            and db.budget.api_tokens_used >= live_ceiling
        ):
            coord_out.emit(
                "budget_halt",
                f"Token ceiling reached: {db.budget.api_tokens_used:,} ≥ "
                f"{live_ceiling:,}. Coordinator halted. Restart with "
                f"`--token-ceiling N` to bump (or edit "
                f"CONFIG.api_token_ceiling for a permanent change).",
                requires_ack=True,
                root=root,
            )
            journal.append(
                "budget_halt",
                {"tokens_used": db.budget.api_tokens_used, "ceiling": live_ceiling},
                root,
            )
            save_db(db, root)
            raise BudgetCeilingHalt(
                f"token ceiling {live_ceiling} exceeded"
            )

        # Persist updated wall-hours + tokens.
        save_db(db, root)

    return db


def _run_iteration_body(
    db: Db, root: Path, dry_run: bool, iter_num: int, it: Iteration
) -> None:
    # 0. Poll pending post-ship validations (lagging data points; non-blocking).
    if not dry_run:
        transitioned = workspace_validate.poll_pending_validations(db, root)
        if transitioned:
            print(f"[iter {iter_num}] validations landed: {transitioned}")

    # 0a. Pull any new GitHub PR comments into inbox.md so they're
    # processed in the same drain below. No-op if COORD_GITHUB_PR_NUMBER
    # is unset.
    if not dry_run:
        count, detail = github_in.poll(root)
        if count > 0:
            journal.append("github_inbox_pulled", {"count": count}, root)
            print(f"[iter {iter_num}] github_in: {detail}")
        _check_gh_health(root)

    # 1. Process inbox
    it.inbox_acks = process_inbox(db, root, dry_run)

    # 1a. Sync from upstream feature branch (q-branch-observer) so the
    # coordinator picks up any newly-merged work before iterating. We're
    # running on claude/observer-improvements, not merging into upstream — this is one-way.
    # On conflict the iteration halts with a coord-out message; the human
    # resolves by hand (rebase or abandon the diverging claude/observer-improvements commit).
    if not dry_run:
        git_ops.ensure_scratch_branch(root)
        # Dirty-tree guard MUST precede the merge — otherwise stray edits
        # under WATCH_PATHS get silently auto-committed OR wiped by merge
        # --abort on conflict. startup_cleanup covers coordinator-owned
        # crashes; this catches human/untracked edits mid-run.
        if not git_ops.is_clean(root):
            print(f"[iter {iter_num}] working tree dirty (pre-sync); aborting", file=sys.stderr)
            journal.append("iteration_aborted_dirty_tree", {"iter": iter_num, "phase": "pre-sync"}, root)
            it.ended_at = now_iso()
            db.iterations.append(it)
            save_db(db, root)
            return
        sync = git_ops.sync_from_upstream(root)
        if sync.get("merged") or sync.get("ahead_count"):
            journal.append("upstream_sync", sync, root)
        if sync.get("conflict"):
            print(
                f"[iter {iter_num}] upstream sync CONFLICT; halting coordinator",
                file=sys.stderr,
            )
            journal.append("upstream_sync_conflict", sync, root)
            coord_out.emit(
                "upstream_conflict",
                f"Sync from `origin/{git_ops.UPSTREAM_BRANCH}` conflicted with "
                f"claude/observer-improvements. Merge aborted; working tree restored. "
                f"Manual resolution required. Coordinator halted — re-run "
                f"after rebase.\n\n```\n{sync.get('error') or ''}\n```",
                requires_ack=True,
                root=root,
            )
            it.ended_at = now_iso()
            db.iterations.append(it)
            save_db(db, root)
            metrics.regenerate(db, root)
            # Raise so the --forever outer loop exits instead of re-trying
            # the sync every iteration (which would conflict again + spam).
            raise UpstreamConflictHalt(sync.get("error") or "upstream conflict")
        if not sync.get("fetched"):
            # Fetch failure is recoverable (network blip); log and continue
            # with whatever upstream state we already have.
            journal.append("upstream_fetch_failed", sync, root)

    # 2. Pick next candidate (with diversity policy + proposer if queue dry)
    candidate = pick_next_candidate_with_proposal(db, root, dry_run)
    if candidate is None:
        print(f"[iter {iter_num}] no candidates in phase {db.phase_state.current_phase.value}; idle")
        it.ended_at = now_iso()
        db.iterations.append(it)
        if not dry_run:
            save_db(db, root)
            metrics.regenerate(db, root)
        return

    it.candidate_id = candidate.id
    detectors = relevant_detectors(candidate, db)
    # Why a list, not a single detector: a candidate modifying correlator
    # code or a shared feature affects MULTIPLE detectors. Gating on a
    # single "primary" detector was a panel-reviewed BLOCK — silent
    # regressions on other detectors could ship. Policy now: eval every
    # detector whose output the candidate plausibly changes, and gate on
    # the worst ΔF1 across them.

    experiment_id = f"exp-{iter_num:04d}-{candidate.id}"
    print(f"[iter {iter_num}] candidate={candidate.id} detectors={detectors}")

    if not dry_run:
        # Announce iteration start on the run-log PR. Gives observers on
        # mobile a real-time breadcrumb trail so a days-long run isn't
        # just "silence → verdict at the end".
        cand_desc = (candidate.description or "").strip().split("\n")[0][:200]
        coord_out.emit(
            "iter_start",
            (
                f"**iter {iter_num}** · `{candidate.id}` "
                f"(family `{candidate.approach_family}`)\n\n"
                f"Evaluating against detectors: `{', '.join(detectors)}`.\n\n"
                f"> {cand_desc}"
                # Cumulative-only footer (no "this iter" since no SDK call has
                # fired yet) — lets observers see run-to-date spend at a glance.
                + _budget_footer(root, iter_num=None, ceiling=db.budget.api_token_ceiling)
            ),
            requires_ack=False,
            root=root,
        )

    # 2a. Capture pre-SHA (scratch branch already ensured + sync'd + dirty-check
    # passed at step 1a). Post-sync SHA is the correct baseline for the
    # candidate's commit.
    pre_sha = git_ops.head_sha(root) if not dry_run else "dry-run"

    # 2b. Sanity sentinel: re-run a known-good detector+scenario eval to
    # catch a sick eval pipeline (missing dda/invoke/go deps, drifted
    # binary, broken scenarios dir) BEFORE we burn implementation tokens.
    # If the sentinel can't reproduce baseline F1, the iter is aborted
    # and a human is asked to fix the workspace. See CONFIG.sanity_*.
    if not dry_run:
        ok, detail = _sanity_pre_eval_sentinel(db, root, iter_num)
        journal.append("sanity_check_pre", {"iter": iter_num, "ok": ok, "detail": detail}, root)
        if ok:
            # Reset streak on first successful sentinel.
            if db.budget.consecutive_sentinel_failures > 0:
                db.budget.consecutive_sentinel_failures = 0
        else:
            db.budget.consecutive_sentinel_failures += 1
            streak = db.budget.consecutive_sentinel_failures
            pause_threshold = CONFIG.silent_failure_pause_streak  # share same threshold
            should_pause = streak >= pause_threshold

            print(
                f"[iter {iter_num}] SANITY FAILED: {detail} "
                f"(streak {streak}/{pause_threshold})",
                file=sys.stderr,
            )

            if should_pause:
                # Streak hit threshold — consult the oracle before pausing.
                decision, rationale = _consult_oracle(
                    db=db, root=root,
                    trigger=f"sentinel_failure streak {streak}/{pause_threshold}",
                    detail=detail,
                )
                actually_pause = _apply_oracle_decision(
                    db=db, root=root,
                    decision=decision, rationale=rationale,
                    streak_field="consecutive_sentinel_failures",
                )
                if actually_pause:
                    coord_out.emit(
                        "eval_env_drift",
                        (
                            f"**iter {iter_num}** · `{candidate.id}` — sentinel "
                            f"FAILED **{streak} consecutive** iters. Oracle decided **stop**.\n\n"
                            f"**Detail**: `{detail[:600]}`\n\n"
                            f"**Oracle rationale**: {rationale}"
                            + _budget_footer(root, iter_num=None, ceiling=db.budget.api_token_ceiling)
                        ),
                        requires_ack=True,
                        root=root,
                    )
                    (state_dir(root) / "pause").write_text(
                        f"auto-paused: sentinel streak={streak} at iter {iter_num}\n"
                        f"oracle: {rationale}\n{detail}\n"
                    )
            else:
                # Skip-and-continue. The sentinel is a soft signal (it
                # mostly fires when a recent ship modified the sentinel
                # detector). Burning a full iter on it isn't worth it;
                # streak-3 protects against actual env breakage.
                coord_out.emit(
                    "sanity_skipped",
                    (
                        f"**iter {iter_num}** · `{candidate.id}` — sanity sentinel "
                        f"failed (streak {streak}/{pause_threshold}); skipping "
                        f"this iter and continuing.\n\n"
                        f"**Detail**: `{detail[:300]}`"
                    ),
                    requires_ack=False,
                    root=root,
                )
            it.ended_at = now_iso()
            db.iterations.append(it)
            save_db(db, root)
            metrics.regenerate(db, root)
            return

    # 3. Implement (SDK)
    impl_summary = "[dry-run] SDK not called"
    if not dry_run:
        from . import sdk

        try:
            impl_summary = sdk.implement_candidate(
                candidate, root,
                prior_experiments=_recent_same_family(db, candidate),
                iter_num=iter_num,
            )
        except Exception as e:
            print(f"[iter {iter_num}] implementation failed: {e}", file=sys.stderr)
            journal.append(
                "implementation_failed",
                {"iter": iter_num, "candidate": candidate.id, "error": str(e)},
                root,
            )
            git_ops.revert_working_tree(root)
            # Mark REJECTED so the scheduler doesn't re-pick this same
            # candidate forever. An implementation crash usually means
            # the prompt / candidate spec is somehow un-implementable
            # (or the SDK itself is sick — but re-picking won't help
            # either way).
            candidate.status = CandidateStatus.REJECTED
            stderr_tail = sdk_mod.tail_sdk_error_for_pr(str(e))
            tail_block = f"\n\n**Captured CLI stderr (tail)**:\n{stderr_tail}" if stderr_tail else ""
            coord_out.emit(
                "iter_impl_failed",
                (
                    f"**iter {iter_num}** · `{candidate.id}` — "
                    f"implementation agent crashed. Candidate marked "
                    f"REJECTED to prevent loop.\n\n"
                    f"Error: `{str(e)[:400]}`"
                    + tail_block
                    + f"\n\nWorking tree reverted; moving on."
                    + _budget_footer(root, iter_num, ceiling=db.budget.api_token_ceiling)
                ),
                requires_ack=False,
                root=root,
            )
            it.ended_at = now_iso()
            db.iterations.append(it)
            save_db(db, root)
            return

    journal.append(
        "implementation_done",
        {"iter": iter_num, "candidate": candidate.id, "summary": impl_summary},
        root,
    )

    # 3a. Pre-eval registration check: verify each target detector is
    # actually registered in comp/observer/impl/component_catalog.go.
    # Without this, q.eval-scenarios --only <name> matches nothing,
    # eval runs with no detector enabled, returns an empty report, and
    # the eval_silent_failure gate fires too late ($30+ wasted per iter).
    # Common cause: the implementer (Sonnet) hits max_turns before
    # finishing the catalog registration step. Catching it here saves
    # the eval cost AND gives the proposer a precise rejection reason.
    if not dry_run:
        unregistered = _detectors_not_registered(detectors, root)
        if unregistered:
            # No retry: the implementer prompt now constrains output names
            # via expected_names, so a miss here means the implementer
            # ignored a hard constraint — that's a signal worth respecting,
            # not papering over with a Sonnet rename call (which suppresses
            # the auditor's ability to notice the implementer went off-spec).
            reason = (
                f"detector_not_registered: {sorted(unregistered)} — "
                f"the implementer wrote code but did not register the "
                f"detector(s) in comp/observer/impl/component_catalog.go "
                f"under the expected name, so eval would run with nothing "
                f"to measure"
            )
            print(f"[iter {iter_num}] AUTO-REJECTED ({reason})", file=sys.stderr)
            # Minimal Experiment record so the proposer's research memory
            # sees this rejection on the next iter.
            experiment = Experiment(
                id=experiment_id,
                candidate_id=candidate.id,
                phase=candidate.phase,
                tier=Tier.T0,
                commit_sha=pre_sha,
                config_path="",
                impl_summary=impl_summary,
                scenario_set=[],
                status=ExperimentStatus.FAILED,
                started_at=it.started_at,
                report_path="",
            )
            db.experiments[experiment_id] = experiment
            it.experiment_ids.append(experiment_id)
            _reject_candidate(
                db=db, root=root, it=it, candidate=candidate,
                experiment=experiment,
                decision=RejectionDecision(
                    stage=RejectionStage.REGISTRATION,
                    reason=reason,
                    evidence={
                        "unregistered": sorted(unregistered),
                        "expected": sorted(detectors),
                        "impl_summary": impl_summary[:300],
                    },
                ),
                body_md=(
                    f"**iter {iter_num}** · `{candidate.id}` — auto-rejected "
                    f"before eval: detector(s) `{sorted(unregistered)}` were "
                    f"not registered in `component_catalog.go`.\n\n"
                    f"**Implementer summary**: `{impl_summary[:300]}`\n\n"
                    f"Working tree reverted; moving on."
                ),
                coord_out_kind="iter_rejected",
                iter_num=iter_num,
            )
            return

    # 3b. Independent pre-eval gate: a separate reviewer inspects the
    # implementer's summary + diff before we spend on full scenario eval.
    # This catches predictable rejects that deterministic registration
    # checks cannot see: dead code, default-disabled additions, plan
    # abandonment, miswired correlator/filter paths, and obviously
    # implausible metric logic.
    if not dry_run:
        try:
            gate = sdk_mod.pre_eval_gate(
                candidate,
                impl_summary=impl_summary,
                root=root,
                iter_num=iter_num,
            )
        except Exception as e:
            gate = {
                "verdict": "run_eval",
                "confidence": 0.0,
                "primary_reason": f"pre_eval_gate SDK call failed; proceeding to eval rather than rejecting candidate on infrastructure failure: {type(e).__name__}: {str(e)[:300]}",
                "checks": {},
                "required_before_eval": [],
                "risks": ["pre_eval_gate unavailable; eval budget not protected for this iteration"],
                "gate_error": True,
            }
            stderr_tail = sdk_mod.tail_sdk_error_for_pr(str(e))
            if stderr_tail:
                gate["primary_reason"] += f"\n\nCaptured CLI stderr tail:\n{stderr_tail}"

        verdict = str(gate.get("verdict", "request_fix"))
        journal.append(
            "pre_eval_gate_done",
            {
                "iter": iter_num,
                "candidate": candidate.id,
                "verdict": verdict,
                "confidence": gate.get("confidence"),
                "reason": str(gate.get("primary_reason", ""))[:500],
                "checks": gate.get("checks", {}),
                "required_before_eval": gate.get("required_before_eval", []),
                "risks": gate.get("risks", []),
                "failing_checks": gate.get("failing_checks", []),
                "unknown_critical_checks": gate.get("unknown_critical_checks", []),
                "gate_error": gate.get("gate_error", False),
                "raw_text": str(gate.get("raw_text", ""))[:1000],
            },
            root,
        )
        if verdict not in ("run_eval", "run_smoke_eval"):
            reason = (
                f"pre_eval_gate:{verdict}: "
                f"{str(gate.get('primary_reason', '') or '(no reason)')[:600]}"
            )
            experiment = Experiment(
                id=experiment_id,
                candidate_id=candidate.id,
                phase=candidate.phase,
                tier=Tier.T0,
                commit_sha=pre_sha,
                config_path="",
                impl_summary=impl_summary,
                scenario_set=[],
                status=ExperimentStatus.FAILED,
                started_at=it.started_at,
                report_path="",
            )
            db.experiments[experiment_id] = experiment
            it.experiment_ids.append(experiment_id)

            checks = gate.get("checks", {})
            check_lines: list[str] = []
            if isinstance(checks, dict):
                for name, body in list(checks.items())[:8]:
                    if isinstance(body, dict):
                        status = body.get("status", "unknown")
                        evidence = str(body.get("evidence", "") or "").replace("\n", " ")[:240]
                    else:
                        status = "unknown"
                        evidence = str(body).replace("\n", " ")[:240]
                    check_lines.append(f"- `{name}`: **{status}** — {evidence}")
            required = gate.get("required_before_eval") or []
            required_lines = "\n".join(f"- {str(x)[:240]}" for x in required) or "- (none)"
            checks_md = "\n".join(check_lines) or "- (no structured checks returned)"

            _reject_candidate(
                db=db, root=root, it=it, candidate=candidate,
                experiment=experiment,
                decision=RejectionDecision(
                    stage=RejectionStage.PRE_EVAL_GATE,
                    reason=reason,
                    evidence={
                        "verdict": verdict,
                        "confidence": gate.get("confidence"),
                        "checks": checks,
                        "required_before_eval": required,
                        "risks": gate.get("risks", []),
                        "failing_checks": gate.get("failing_checks", []),
                        "unknown_critical_checks": gate.get("unknown_critical_checks", []),
                        "raw_text": str(gate.get("raw_text", ""))[:1000],
                        "impl_summary": impl_summary[:300],
                    },
                ),
                body_md=(
                    f"**iter {iter_num}** · `{candidate.id}` — pre-eval "
                    f"gate returned **{verdict}**; skipping expensive eval.\n\n"
                    f"**Reason**: {str(gate.get('primary_reason', '') or '(none)')[:900]}\n\n"
                    f"**Checks**:\n{checks_md}\n\n"
                    f"**Required before eval**:\n{required_lines}\n\n"
                    f"Working tree reverted; moving on."
                ),
                coord_out_kind="pre_eval_gate_rejected",
                iter_num=iter_num,
            )
            return

    # 4. Eval — single system-level run per iter. The catalog's
    # `defaultEnabled` set defines the prod-realistic pipeline; the
    # candidate's modification (or new component with defaultEnabled:
    # true) is picked up automatically. No per-detector loop, no --only
    # juggling — one report describes the whole system's F1 with the
    # change in place.
    eval_ok = True
    eval_msg = "ok"
    report_path = state_dir(root) / "reports" / f"{experiment_id}-system.json"
    scenario_out = state_dir(root) / "reports" / f"{experiment_id}-system"

    if dry_run:
        eval_msg = "[dry-run]"
    else:
        run = evaluator.run_scenarios(
            report_path=report_path,
            scenario_output_dir=scenario_out,
            root=root,
        )
        journal.append(
            "eval_done",
            {
                "iter": iter_num,
                "candidate": candidate.id,
                "ok": run.ok,
                "report_path": str(report_path),
                "rc": run.returncode,
            },
            root,
        )
        if not run.ok:
            eval_ok = False
            eval_msg = (run.stderr or 'failed')[-500:]

    experiment = Experiment(
        id=experiment_id,
        candidate_id=candidate.id,
        phase=candidate.phase,
        tier=Tier.T0,
        commit_sha=pre_sha,
        config_path="",
        impl_summary=impl_summary,
        scenario_set=[],  # filled from report below
        status=ExperimentStatus.DONE if eval_ok else ExperimentStatus.FAILED,
        started_at=it.started_at,
        report_path=str(report_path),
    )
    db.experiments[experiment_id] = experiment
    it.experiment_ids.append(experiment_id)

    if not eval_ok:
        print(f"[iter {iter_num}] eval failed: {eval_msg}", file=sys.stderr)
        # Eval failure is not a judgement on the change itself — reject and move on.
        candidate.status = CandidateStatus.REJECTED
        if not dry_run:
            git_ops.revert_working_tree(root)
            coord_out.emit(
                "iter_eval_failed",
                (
                    f"**iter {iter_num}** · `{candidate.id}` — eval failed.\n\n"
                    f"Stderr tail: `{eval_msg[:400]}`\n\n"
                    f"Working tree reverted; moving on."
                    + _budget_footer(root, iter_num, ceiling=db.budget.api_token_ceiling)
                ),
                requires_ack=False,
                root=root,
            )
        it.ended_at = now_iso()
        db.iterations.append(it)
        if not dry_run:
            save_db(db, root)
            metrics.regenerate(db, root)
        return

    # 5. Score
    if dry_run or db.baseline is None:
        # Skip review in dry-run or when no baseline is loaded.
        # Leave candidate as PROPOSED so it can be retried on a real run.
        print(f"[iter {iter_num}] skipping review (dry-run or no baseline)")
        it.ended_at = now_iso()
        db.iterations.append(it)
        if not dry_run:
            save_db(db, root)
            metrics.regenerate(db, root)
        return

    train_set = db.split.as_train_set() if db.split else None
    # Gate vs FROZEN baseline, across ALL relevant detectors. Aggregate
    # policy: gate-on-worst (any detector with a catastrophe regression
    # System-level eval: one report → one ScoringResult.
    # Use effective (best-historical) baseline if present so a candidate
    # can't ratchet backwards through tolerance-bound drift.
    score_ref = db.effective_baseline or db.baseline
    scoring = score_against_baseline(
        report_path,
        score_ref,
        train_scenarios=train_set,
    )
    experiment.score = scoring.mean_f1
    experiment.num_baseline_fps_sum = scoring.total_fps
    experiment.per_scenario = scoring.per_scenario
    experiment.scenario_set = list(scoring.per_scenario.keys())

    # 5a. Update phase state based on SCORE alone (approval-independent).
    # Effect-size aware plateau: a score only counts as "improvement" if
    # it clears best_score by > ε. A raw strict-greater comparison let
    # noisy +0.001 bumps reset the plateau counter and keep dead-end
    # phases alive forever. ε matches CONFIG.plateau_epsilon used by
    # scheduler._family_consecutive_nonimproving.
    if scoring.mean_f1 > db.phase_state.best_score + CONFIG.plateau_epsilon:
        db.phase_state.best_score = scoring.mean_f1
        db.phase_state.plateau_counter = 0
    else:
        db.phase_state.plateau_counter += 1

    # 5b. Hard auto-reject gates — reject without review if:
    #   (a) any train scenario catastrophe-regressed (absolute or relative)
    #   (b) any defended recall floor was violated
    #   (c) total FPs exploded past fp_ceiling_ratio × baseline FPs
    # Keeps the gate deterministic instead of relying on LLM personas.
    # (c) catches the "emit-everything" attack: rewriting a detector to
    # fire aggressively boosts recall while F1 per-scenario can stay
    # within catastrophe bounds — total FPs is the honest signal.
    # FP ceiling split into two tiers:
    #   - tier 1 (auto-reject):  total FPs > fp_egregious_ratio × baseline (default 3×)
    #   - tier 2 (advisory):     1.5× < FPs ≤ 3× — flag to reviewer, don't auto-reject
    fp_ceiling = int(scoring.baseline_total_fps * CONFIG.fp_ceiling_ratio)
    fp_egregious_ceiling = int(scoring.baseline_total_fps * 3.0)
    fp_ceiling_breached = (
        scoring.baseline_total_fps > 0
        and scoring.total_fps > fp_ceiling
    )
    # Tier-1 FP egregious: 3× baseline AND absolute floor of 20 FPs. The
    # absolute floor matters because at small baselines (e.g. 2 FPs)
    # 3× = 6, which a single noisy run can hit on a real signal — that's
    # judgment-call territory, not catastrophe. Catastrophe-grade FP
    # explosions are large in absolute terms.
    fp_egregious_breached = (
        scoring.baseline_total_fps > 0
        and scoring.total_fps > fp_egregious_ceiling
        and scoring.total_fps > 20
    )

    # Post-eval sanity: did the eval produce all-zero F1/precision/recall
    # across every scenario? Default behavior changed 2026-04-29: instead
    # of auto-pausing, REJECT-AND-CONTINUE. A single silent_failure is
    # almost always a structural mismatch (e.g. correlator candidate run
    # standalone → no upstream input → zero detections), not a sick eval
    # pipeline. Pausing on every one blocks the run for hours waiting for
    # human ack on what the harness should just learn from.
    #
    # Streak-3 protection: if THREE consecutive silent_failures stack
    # without any iter producing real metrics, then we assume the env
    # is genuinely broken and auto-pause with rich diagnostics. Streak
    # resets to 0 on the first iter with non-zero metrics (below).
    silent_failure = _sanity_zero_detections(scoring)
    if silent_failure:
        db.budget.consecutive_silent_failures += 1
        streak = db.budget.consecutive_silent_failures
        pause_threshold = CONFIG.silent_failure_pause_streak
        should_pause = streak >= pause_threshold

        print(
            f"[iter {iter_num}] SILENT EVAL FAILURE: {silent_failure} "
            f"(streak {streak}/{pause_threshold})",
            file=sys.stderr,
        )

        # Rich diagnostics — populated regardless of pause vs continue,
        # journalled and (if pausing) written to the pause file too.
        diag = _silent_failure_diagnostics(
            root=root,
            iter_num=iter_num,
            candidate=candidate,
            scoring=scoring,
            experiment=experiment,
        )

        if should_pause:
            # Streak hit threshold — consult the oracle before pausing.
            # Opus reviews recent context and decides continue / pivot / stop.
            decision, rationale = _consult_oracle(
                db=db, root=root,
                trigger=f"silent_failure streak {streak}/{pause_threshold}",
                detail=f"{silent_failure}\n\nDIAGNOSTICS:\n{diag}",
            )
            actually_pause = _apply_oracle_decision(
                db=db, root=root,
                decision=decision, rationale=rationale,
                streak_field="consecutive_silent_failures",
            )
            if actually_pause:
                body = (
                    f"**iter {iter_num}** · `{candidate.id}` — eval reported all-zero "
                    f"F1/precision/recall, **{streak}th consecutive** silent failure. "
                    f"Oracle reviewed and decided **stop**.\n\n"
                    f"**Detail**: `{silent_failure}`\n\n"
                    f"**Oracle rationale**: {rationale}\n\n"
                    f"**Diagnostics**:\n```\n{diag}\n```"
                )
                pause_after = (
                    f"auto-paused: eval_silent_failure streak={streak} at iter {iter_num}\n"
                    f"oracle: {rationale}\n{silent_failure}\n\n=== DIAGNOSTICS ===\n{diag}\n"
                )
                requires_ack = True
            else:
                # Oracle said continue or pivot — streak already reset by helper.
                body = (
                    f"**iter {iter_num}** · `{candidate.id}` — silent_failure streak hit "
                    f"{streak}, oracle said **{decision}**. Rejecting candidate, NOT pausing.\n\n"
                    f"**Detail**: `{silent_failure}`\n\n"
                    f"**Oracle rationale**: {rationale}"
                )
                pause_after = None
                requires_ack = False
        else:
            body = (
                f"**iter {iter_num}** · `{candidate.id}` — eval reported all-zero "
                f"F1/precision/recall across every scenario. Treating as an "
                f"unevaluable candidate (silent_failure streak {streak}/"
                f"{pause_threshold}); rejecting and continuing.\n\n"
                f"**Detail**: `{silent_failure}`\n\n"
                f"**Diagnostics**:\n```\n{diag}\n```\n\n"
                f"Auto-pause kicks in at streak {pause_threshold}."
            )
            pause_after = None
            requires_ack = False

        _reject_candidate(
            db=db, root=root, it=it, candidate=candidate,
            experiment=experiment,
            decision=RejectionDecision(
                stage=RejectionStage.EVAL_SILENT_FAILURE,
                reason=f"eval_silent_failure: {silent_failure}",
                evidence={
                    "detail": silent_failure,
                    "streak": streak,
                    "pause_threshold": pause_threshold,
                },
            ),
            body_md=body,
            coord_out_kind="eval_silent_failure",
            requires_ack=requires_ack,
            pause_after=pause_after,
            iter_num=iter_num,
        )
        return

    # First iter past silent_failure → streak resets. Real metrics flowed.
    if db.budget.consecutive_silent_failures > 0:
        db.budget.consecutive_silent_failures = 0

    # Blank-mode quality floor for the FIRST-ever ship. Catastrophe filters
    # can't fire when baseline F1 ≈ 0 across the board (blank-mode runs),
    # so the first candidate that compiles would otherwise ship regardless
    # of detection quality. ONLY enforce the floor when the baseline is
    # essentially absent — for full-mode runs with a real baseline, rely
    # on catastrophe filters + reviewer; an absolute 0.25 floor would
    # block legitimate small improvements (e.g. baseline=0.116 + Δ=0.10
    # is a real win but doesn't clear 0.25).
    any_prior_ship = any(
        c.status == CandidateStatus.SHIPPED for c in db.candidates.values()
    )
    baseline_essentially_blank = (
        scoring.baseline_mean_f1 < 0.05
    )
    first_ship_floor_breached = (
        not any_prior_ship
        and baseline_essentially_blank
        and scoring.mean_f1 < CONFIG.first_ship_min_mean_f1
    )

    # TIER 1 — egregious gates that auto-reject without review. These are
    # cases where there's nothing for the panel to weigh: pipeline broken,
    # FPs exploded so badly any positive trade is implausible, or first
    # ship of a blank run failing the quality floor.
    tier1_reject = (
        fp_egregious_breached  # FPs > 3× baseline → no plausible trade
        or first_ship_floor_breached  # blank-mode quality floor
    )

    # TIER 2 — concerning per-scenario regressions or moderate FP increase.
    # The aggregate trade may still be net-positive (e.g. PR 50011 iter 4:
    # mean F1 +0.039, FPs cut in half, but 3 scenarios lost recall > 0.10).
    # Don't auto-reject; route to the 3-persona review with these signals
    # as STRUCTURED advisory context. Reviewer prompt has an explicit rule
    # for handling them (see sdk.review_experiment).
    tier2_signals: list[dict[str, Any]] = []
    if scoring.strict_regressions:
        tier2_signals.append({
            "kind": "strict_regressions",
            "detail": list(scoring.strict_regressions),
        })
    if scoring.recall_floor_violations:
        tier2_signals.append({
            "kind": "recall_floor_violations",
            "detail": list(scoring.recall_floor_violations),
        })
    if fp_ceiling_breached and not fp_egregious_breached:
        tier2_signals.append({
            "kind": "fp_ceiling_breached_moderate",
            "observed": scoring.total_fps,
            "ceiling": fp_ceiling,
            "baseline": scoring.baseline_total_fps,
        })

    if tier1_reject:
        reasons: list[str] = []
        evidence: dict[str, Any] = {
            "mean_f1": scoring.mean_f1,
            "baseline_mean_f1": scoring.baseline_mean_f1,
            "mean_df1": scoring.mean_df1,
            "total_fps": scoring.total_fps,
            "baseline_total_fps": scoring.baseline_total_fps,
        }
        if fp_egregious_breached:
            reasons.append(
                f"fp_egregious={scoring.total_fps} > "
                f"{fp_egregious_ceiling} (3× baseline {scoring.baseline_total_fps}) "
                f"and absolute > 20"
            )
            evidence["fp_egregious_ceiling"] = fp_egregious_ceiling
            evidence["fp_egregious"] = True
        if first_ship_floor_breached:
            reasons.append(
                f"first_ship_floor=mean_f1 {scoring.mean_f1:.4f} < "
                f"{CONFIG.first_ship_min_mean_f1:.2f} (no prior ship — "
                f"quality bar for first commit on a blank-baseline run)"
            )
            evidence["first_ship_floor"] = True
            evidence["first_ship_min_mean_f1"] = CONFIG.first_ship_min_mean_f1
        reason = " ; ".join(reasons) if reasons else "tier1 (unspecified)"
        print(f"[iter {iter_num}] AUTO-REJECTED ({reason})")

        # Compact per-scenario delta summary — biggest 5 |ΔF1| swings.
        top = sorted(
            scoring.per_scenario_delta.values(),
            key=lambda d: abs(d.df1),
            reverse=True,
        )[:5]
        delta_lines = "\n".join(
            f"- `{d.scenario}`: F1 {d.baseline.f1:.3f} → {d.observed.f1:.3f} "
            f"(Δ{d.df1:+.3f}), recall Δ{d.drecall:+.3f}"
            for d in top
        )
        archive_prefilter_ok, archive_prefilter = _archive_numeric_prefilter(db, scoring)
        archive_decision: dict[str, Any] | None = None
        if archive_prefilter_ok and db.baseline is not None:
            try:
                archive_decision = sdk_mod.assess_archive_merit(
                    candidate=candidate,
                    experiment=experiment,
                    scoring=scoring,
                    original_baseline_mean_f1=db.baseline.system.mean_f1,
                    original_baseline_total_fps=db.baseline.system.total_fps,
                    tier2_signals=tier2_signals,
                    root=root,
                    iter_num=iter_num,
                )
            except Exception as e:
                archive_decision = {
                    "archive": False,
                    "confidence": 0.0,
                    "reason": f"archive_merit SDK call failed: {type(e).__name__}: {str(e)[:300]}",
                    "suggested_followup": "",
                }
                stderr_tail = sdk_mod.tail_sdk_error_for_pr(str(e))
                if stderr_tail:
                    archive_decision["reason"] += f"\n\nCaptured CLI stderr tail:\n{stderr_tail}"
            journal.append(
                "archive_merit_done",
                {
                    "iter": iter_num,
                    "candidate": candidate.id,
                    "prefilter": archive_prefilter,
                    "decision": archive_decision,
                    "prior_rejection_stage": "tier1_gate",
                },
                root,
            )
        if archive_decision and archive_decision.get("archive"):
            _archive_candidate_commit(
                db=db,
                root=root,
                it=it,
                candidate=candidate,
                experiment=experiment,
                scoring=scoring,
                archive_prefilter=archive_prefilter,
                archive_decision=archive_decision,
                rationale_lines=(
                    f"Deterministic tier-1 gate rejected this candidate before "
                    f"shipping review.\n\n**Gate failures**: {reason}\n\n"
                    f"**Top 5 |ΔF1| scenarios**:\n{delta_lines}"
                ),
                iter_num=iter_num,
            )
            return
        _reject_candidate(
            db=db, root=root, it=it, candidate=candidate,
            experiment=experiment,
            decision=RejectionDecision(
                stage=RejectionStage.TIER1_GATE,
                reason=reason,
                evidence=evidence,
                advisory_signals=[s["kind"] for s in tier2_signals],
            ),
            body_md=(
                f"**iter {iter_num}** · `{candidate.id}` — "
                f"auto-rejected on catastrophe filter.\n\n"
                f"**Gate failures**: {reason}\n\n"
                f"**Top 5 |ΔF1| scenarios**:\n{delta_lines}\n\n"
                f"Observed mean F1 {scoring.mean_f1:.4f} vs baseline "
                f"{scoring.baseline_mean_f1:.4f} (Δ{scoring.mean_df1:+.4f}). "
                f"Total FPs {scoring.baseline_total_fps} → {scoring.total_fps} "
                f"(Δ{scoring.total_dfps:+d}).\n\n"
                f"Working tree reverted; no commit."
            ),
            coord_out_kind="strict_regression",
            iter_num=iter_num,
        )
        return

    # 6. Review
    from . import sdk  # lazy

    try:
        verdict = sdk.review_experiment(
            experiment, scoring, candidate.phase,
            all_scenarios=sorted(
                (list(train_set) if train_set else [])
                + (list(db.split.lockbox) if db.split else [])
            ),
            root=root,
            candidate=candidate,
            iter_num=iter_num,
            tier2_signals=tier2_signals,
        )
    except Exception as e:
        print(f"[iter {iter_num}] review failed: {e}", file=sys.stderr)
        journal.append("review_failed", {"iter": iter_num, "error": str(e)}, root)
        stderr_tail = sdk_mod.tail_sdk_error_for_pr(str(e))
        tail_block = f"\n\n**Captured CLI stderr (tail)**:\n{stderr_tail}" if stderr_tail else ""
        coord_out.emit(
            "iter_review_failed",
            (
                f"**iter {iter_num}** · `{candidate.id}` — "
                f"review crashed. Candidate marked REJECTED to prevent loop.\n\n"
                f"Error: `{str(e)[:400]}`"
                + tail_block
                + f"\n\nWorking tree reverted; moving on."
                + _budget_footer(root, iter_num, ceiling=db.budget.api_token_ceiling)
            ),
            requires_ack=False,
            root=root,
        )
        git_ops.revert_working_tree(root)
        # Review crash is not a judgement — revert, reject, move on.
        candidate.status = CandidateStatus.REJECTED
        it.ended_at = now_iso()
        db.iterations.append(it)
        save_db(db, root)
        metrics.regenerate(db, root)
        return

    experiment.review = verdict
    journal.append(
        "review_done",
        {
            "iter": iter_num,
            "candidate": candidate.id,
            "unanimous_approve": verdict.unanimous_approve,
            "decisions": [
                {"persona": d.persona, "approve": d.approve} for d in verdict.decisions
            ],
        },
        root,
    )

    # 7. Commit or revert
    if verdict.unanimous_approve:
        # Crash-safe ordering: mark SHIPPED in db.yaml with a sentinel sha
        # BEFORE creating the commit. If we crash between save_db and
        # commit_candidate, startup_cleanup sees a PROPOSED→SHIPPED record
        # with no matching commit and reconciles it (or we simply re-commit
        # since status is idempotent). Inverted order would leave git ahead
        # of db (commit exists, db says PROPOSED) which re-selects the
        # candidate and double-commits.
        candidate.status = CandidateStatus.SHIPPED
        experiment.commit_sha = "pending"
        save_db(db, root)

        commit_sha = git_ops.commit_candidate(candidate.id, experiment_id, root)
        experiment.commit_sha = commit_sha
        # Update the effective (best-historical) baseline element-wise.
        # Single system entry; scoring.per_scenario is keyed by bare
        # scenario name. Best-historical can only ratchet up or sideways.
        from .scoring import merge_best_historical
        eff = db.effective_baseline or db.baseline
        if eff is not None and scoring.per_scenario:
            db.effective_baseline = merge_best_historical(eff, scoring.per_scenario)
        # Persist the real sha BEFORE pushing. If push fails, db.yaml already
        # reflects the commit; startup_cleanup will push the orphan on restart.
        save_db(db, root)
        pushed_ok, push_msg = git_ops.push_scratch_branch(root)
        journal.append(
            "push_attempted",
            {"iter": iter_num, "ok": pushed_ok, "msg": push_msg, "sha": commit_sha},
            root,
        )
        push_tag = "pushed" if pushed_ok else f"push-failed ({push_msg[:60]})"
        print(
            f"[iter {iter_num}] APPROVED; committed {commit_sha[:10]} "
            f"({push_tag}) score {scoring.mean_f1:.4f}"
        )

        # Ship announcement with the key numbers + top scenario wins.
        top_wins = sorted(
            scoring.per_scenario_delta.values(),
            key=lambda d: d.df1,
            reverse=True,
        )[:5]
        win_lines = "\n".join(
            f"- `{d.scenario}`: F1 {d.baseline.f1:.3f} → {d.observed.f1:.3f} "
            f"(Δ{d.df1:+.3f})"
            for d in top_wins
        )
        rationale_lines = "\n".join(
            f"- **{dec.persona}**: {dec.rationale[:300]}"
            for dec in verdict.decisions
        )
        matrix_compact = "\n".join(metrics._f1_matrix_compact(db))
        matrix_block = (
            f"\n\n**F1 matrix (cumulative vs baseline)**:\n{matrix_compact}"
            if matrix_compact
            else ""
        )
        coord_out.emit(
            "iter_shipped",
            (
                f"**iter {iter_num}** · `{candidate.id}` — **SHIPPED** "
                f"(commit `{commit_sha[:10]}`, {push_tag}).\n\n"
                f"Mean F1 {scoring.baseline_mean_f1:.4f} → {scoring.mean_f1:.4f} "
                f"(Δ{scoring.mean_df1:+.4f}). FPs "
                f"{scoring.baseline_total_fps} → {scoring.total_fps} "
                f"(Δ{scoring.total_dfps:+d}).\n\n"
                f"**Top 5 scenario wins**:\n{win_lines}\n\n"
                f"**Reviewer verdicts**:\n{rationale_lines}"
                + matrix_block
                + _budget_footer(root, iter_num, ceiling=db.budget.api_token_ceiling)
            ),
            requires_ack=False,
            root=root,
        )

        # Plateau-gated eval-component dispatch: certify a NEW component
        # ONCE, when the family iterating on it has plateaued and at least
        # this candidate shipped. Pre-existing components (bocpd / scanmw /
        # scanwelch) are already in db.components_eval_dispatched.
        fam = candidate.approach_family
        fam_streak = scheduler._family_consecutive_nonimproving(db, fam)
        # fam_streak is computed AFTER this iteration's experiment is
        # appended, so a plateau means fam_streak >= STUCK_THRESHOLD.
        fam_plateaued = fam_streak >= CONFIG.stuck_threshold
        if fam_plateaued:
            for comp in candidate.target_components:
                if comp in db.components_eval_dispatched:
                    continue
                pv = workspace_validate.dispatch_validation(
                    experiment_id=experiment_id,
                    candidate_id=candidate.id,
                    detector=comp,
                    db=db,
                    root=root,
                )
                # Mark dispatched even on ssh failure (no workspace). Next
                # plateau won't retry; the user creates the workspace if
                # they want the validation.
                db.components_eval_dispatched.append(comp)
                save_db(db, root)
                if pv:
                    print(
                        f"[iter {iter_num}] eval-component dispatched for new "
                        f"component '{comp}' (family {fam} plateaued) on {pv.workspace}"
                    )
                else:
                    print(
                        f"[iter {iter_num}] eval-component dispatch skipped for "
                        f"'{comp}' (no workspace / ssh failed); journalled"
                    )

        # Overfit telltale (periodic, best-effort): after every Nth ship,
        # evaluate all shipped candidates on the lockbox and Spearman-check
        # train-rank vs lockbox-rank. Warns via coord-out if drift; does
        # NOT gate. Lockbox scores never flow into agent prompts.
        try:
            overfit_check.maybe_run_overfit_check(db, root)
        except Exception as e:
            journal.append("overfit_check_error", {"error": str(e)}, root)
    else:
        # Review-level reject — passed deterministic gates, failed unanimity.
        # Show each persona's verdict for transparency.
        rationale_lines = "\n".join(
            f"- **{dec.persona}** ({'approve' if dec.approve else 'reject'}): "
            f"{dec.rationale[:400]}"
            for dec in verdict.decisions
        )
        archive_prefilter_ok, archive_prefilter = _archive_numeric_prefilter(db, scoring)
        archive_decision: dict[str, Any] | None = None
        if archive_prefilter_ok and db.baseline is not None:
            try:
                archive_decision = sdk.assess_archive_merit(
                    candidate=candidate,
                    experiment=experiment,
                    scoring=scoring,
                    original_baseline_mean_f1=db.baseline.system.mean_f1,
                    original_baseline_total_fps=db.baseline.system.total_fps,
                    tier2_signals=tier2_signals,
                    root=root,
                    iter_num=iter_num,
                )
            except Exception as e:
                archive_decision = {
                    "archive": False,
                    "confidence": 0.0,
                    "reason": f"archive_merit SDK call failed: {type(e).__name__}: {str(e)[:300]}",
                    "suggested_followup": "",
                }
                stderr_tail = sdk_mod.tail_sdk_error_for_pr(str(e))
                if stderr_tail:
                    archive_decision["reason"] += f"\n\nCaptured CLI stderr tail:\n{stderr_tail}"
            journal.append(
                "archive_merit_done",
                {
                    "iter": iter_num,
                    "candidate": candidate.id,
                    "prefilter": archive_prefilter,
                    "decision": archive_decision,
                },
                root,
            )

        if archive_decision and archive_decision.get("archive"):
            _archive_candidate_commit(
                db=db,
                root=root,
                it=it,
                candidate=candidate,
                experiment=experiment,
                scoring=scoring,
                archive_prefilter=archive_prefilter,
                archive_decision=archive_decision,
                rationale_lines=f"**Shipping reviewer verdicts**:\n{rationale_lines}",
                iter_num=iter_num,
            )
            return

        print(f"[iter {iter_num}] REJECTED; reverted (score {scoring.mean_f1:.4f})")
        _reject_candidate(
            db=db, root=root, it=it, candidate=candidate,
            experiment=experiment,
            decision=RejectionDecision(
                stage=RejectionStage.REVIEW,
                reason=(
                    f"review_rejected: panel non-unanimous "
                    f"(approves={sum(1 for d in verdict.decisions if d.approve)}/"
                    f"{len(verdict.decisions)})"
                ),
                evidence={
                    "mean_f1": scoring.mean_f1,
                    "baseline_mean_f1": scoring.baseline_mean_f1,
                    "mean_df1": scoring.mean_df1,
                    "decisions": [
                        {"persona": d.persona, "approve": d.approve}
                        for d in verdict.decisions
                    ],
                    "archive_prefilter": archive_prefilter,
                    "archive_merit": archive_decision,
                },
                advisory_signals=[s["kind"] for s in tier2_signals],
            ),
            body_md=(
                f"**iter {iter_num}** · `{candidate.id}` — rejected by review "
                f"(passed deterministic gates, failed unanimity).\n\n"
                f"Mean F1 {scoring.baseline_mean_f1:.4f} → {scoring.mean_f1:.4f} "
                f"(Δ{scoring.mean_df1:+.4f}).\n\n"
                f"**Reviewer verdicts**:\n{rationale_lines}\n\n"
                f"**Archive prefilter**: "
                f"{'passed' if archive_prefilter_ok else 'did not pass'} "
                f"`{archive_prefilter}`\n\n"
                f"Working tree reverted; no commit."
            ),
            coord_out_kind="iter_rejected",
            iter_num=iter_num,
        )
        return

    it.ended_at = now_iso()
    db.iterations.append(it)
    save_db(db, root)
    metrics.regenerate(db, root)
    return


# ---------------------------------------------------------------------------
# Entry
# ---------------------------------------------------------------------------

def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="coordinator")
    parser.add_argument("--root", default=".")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--once", action="store_true")
    parser.add_argument("--forever", action="store_true")
    parser.add_argument(
        "--token-ceiling", type=int, default=None,
        # `--token-ceiling 0` disables the panic-brake entirely; only
        # wall-hours halts the run.
        help=(
            "Override CONFIG.api_token_ceiling for THIS run only. "
            "Useful after a budget_halt to bump the cap without a code "
            "edit. Accepts plain integer (e.g. --token-ceiling 30000000)."
        ),
    )
    parser.add_argument(
        "--wall-hours-ceiling", type=float, default=None,
        help="Override CONFIG.default_wall_hours_ceiling for THIS run.",
    )
    args = parser.parse_args(argv)

    root = Path(args.root)
    state_dir(root).mkdir(parents=True, exist_ok=True)

    # Single-instance lock: two drivers reading/writing the same db.yaml +
    # racing commits to the scratch branch is catastrophic. flock is held
    # for the process lifetime; released implicitly on exit.
    lock_path = state_dir(root) / "driver.lock"
    lock_fp = open(lock_path, "w")
    try:
        fcntl.flock(lock_fp.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        print(
            f"another coordinator is running (lock held on {lock_path}); exiting.",
            file=sys.stderr,
        )
        return 2
    lock_fp.write(f"{_dt.datetime.now().isoformat(timespec='seconds')}\n")
    lock_fp.flush()

    # Refuse to enter --forever with unlimited budget. Smoke tests (--once,
    # --dry-run) bypass this: a single iteration can't run away.
    if args.forever and not args.dry_run:
        if CONFIG.api_token_ceiling is None or CONFIG.default_wall_hours_ceiling is None:
            print(
                "refusing --forever: CONFIG.api_token_ceiling and "
                "default_wall_hours_ceiling must both be set. Edit "
                "tasks/coordinator/config.py.",
                file=sys.stderr,
            )
            return 2

    db = load_db(root)
    if db.baseline is None:
        print(
            "warning: baseline not loaded. Run `coordinator.import_baseline` first.",
            file=sys.stderr,
        )

    # CLI overrides for budget ceilings — apply BEFORE the iteration loop
    # so this run uses the new cap. Persisted to db.budget so subsequent
    # restarts (without the flag) keep the same value; pass the flag again
    # to bump further.
    if args.token_ceiling is not None:
        # 0 = disable (only wall-hours halts the run). Useful when the
        # operator only cares about a time-bounded run, not a cost-bounded
        # one. Sets api_token_ceiling=None which the halt-check skips.
        new_ceiling = None if args.token_ceiling == 0 else args.token_ceiling
        print(
            f"[startup] --token-ceiling override: {db.budget.api_token_ceiling} "
            f"→ {new_ceiling if new_ceiling is not None else 'disabled (wall-hours only)'}",
            file=sys.stderr,
        )
        db.budget.api_token_ceiling = new_ceiling
    if args.wall_hours_ceiling is not None:
        print(
            f"[startup] --wall-hours-ceiling override: {db.budget.wall_hours_ceiling} "
            f"→ {args.wall_hours_ceiling}",
            file=sys.stderr,
        )
        db.budget.wall_hours_ceiling = args.wall_hours_ceiling

    # Crash-recovery: revert any orphaned working-tree diffs, push any
    # unpushed commits on claude/observer-improvements, and recover any
    # inbox.md.reading orphan from a prior mid-drain crash. Safe no-op on
    # a clean restart.
    if not args.dry_run:
        cleanup = git_ops.startup_cleanup(root)
        if cleanup.get("reverted_dirty_tree") or cleanup.get("pushed_orphan_commits"):
            journal.append("startup_cleanup", cleanup, root)
            print(f"[startup] cleanup: {cleanup}")
        from .inbox import recover_orphan_reading

        if recover_orphan_reading(root):
            journal.append("inbox_orphan_recovered", {}, root)
            print("[startup] recovered orphan inbox.md.reading")
        # Reconcile any validations stuck in 'dispatching' from a crash
        # between ssh dispatch and db save. Safe default: mark failed; the
        # next iteration won't dispatch a duplicate, and poll_pending
        # ignores non-pending entries.
        for pv in list(db.validations.values()):
            if pv.status == "dispatching":
                pv.status = "failed"
                pv.completed_at = now_iso()
                journal.append(
                    "validation_dispatching_reaped",
                    {"validation_id": pv.id, "workspace": pv.workspace},
                    root,
                )
        # Reconcile experiments with commit_sha == "pending": the driver
        # marked SHIPPED in db.yaml but crashed before git_ops.commit_candidate
        # ran (or before the post-commit save_db). Resolve against current
        # branch HEAD — startup_cleanup already pushed any orphan commit.
        # If HEAD is ahead of the last known sha, adopt it; otherwise mark
        # the experiment failed so the scheduler can re-select.
        try:
            current_sha = git_ops.head_sha(root)
        except Exception:
            current_sha = ""
        for exp in db.experiments.values():
            if exp.commit_sha != "pending":
                continue
            if current_sha and current_sha != "":
                exp.commit_sha = current_sha
                journal.append(
                    "pending_commit_adopted_head",
                    {"experiment_id": exp.id, "adopted_sha": current_sha},
                    root,
                )
            else:
                exp.status = ExperimentStatus.FAILED
                exp.commit_sha = ""
                cand = db.candidates.get(exp.candidate_id)
                if cand is not None and cand.status in (
                    CandidateStatus.SHIPPED,
                    CandidateStatus.ARCHIVED,
                ):
                    cand.status = CandidateStatus.REJECTED
                journal.append(
                    "pending_commit_reverted",
                    {"experiment_id": exp.id, "candidate_id": exp.candidate_id},
                    root,
                )
        save_db(db, root)

    if args.once or not args.forever:
        try:
            run_iteration(db, root, dry_run=args.dry_run)
        except UpstreamConflictHalt:
            print("upstream conflict: halting. Resolve manually and re-run.", file=sys.stderr)
        except GhChannelDeadHalt as e:
            print(f"gh channel halt: {e}", file=sys.stderr)
        except ProtectedPathTamperHalt as e:
            print(f"protected-path tamper: {e}", file=sys.stderr)
        return 0

    while True:
        # Cooperative pause check at iteration boundary (no work in
        # flight). Sleeps until `.coordinator/pause` is removed. Either
        # the user touched it or auto-pause from cost_anomaly streak did.
        _wait_while_paused(root)

        before = len(db.iterations)
        try:
            db = run_iteration(db, root, dry_run=args.dry_run)
        except UpstreamConflictHalt:
            print("upstream conflict: halting coordinator. Resolve manually and re-run.", file=sys.stderr)
            break
        except BudgetCeilingHalt as e:
            print(f"budget halt: {e}", file=sys.stderr)
            break
        except GhChannelDeadHalt as e:
            print(f"gh channel halt: {e}. Fix gh auth and re-run.", file=sys.stderr)
            break
        except ProtectedPathTamperHalt as e:
            print(f"protected-path tamper: {e}", file=sys.stderr)
            break
        except KeyboardInterrupt:
            # Operator hit Ctrl-C; let it propagate.
            raise
        except Exception as e:  # noqa: BLE001
            # Catch-all so a single iter's crash doesn't kill the driver.
            # Without this, transient errors (git lock, subprocess hiccup,
            # JSON parse fail mid-iter, etc.) tear down the whole run and
            # the operator wakes up to a dead workspace 15 hours later.
            # Log + journal + post a PR comment + continue to next iter.
            import traceback
            tb = traceback.format_exc()
            print(f"[iter {before}] DRIVER EXCEPTION: {type(e).__name__}: {e}", file=sys.stderr)
            print(tb, file=sys.stderr)
            # Capture stderr from CalledProcessError if present.
            extra = ""
            if hasattr(e, "stderr") and getattr(e, "stderr", None):
                extra = f"\n\n**stderr**:\n```\n{str(e.stderr)[-2000:]}\n```"
            try:
                journal.append(
                    "iter_driver_exception",
                    {"iter": before, "exc_type": type(e).__name__,
                     "exc": str(e)[:500], "traceback": tb[-2000:]},
                    root,
                )
                coord_out.emit(
                    "iter_driver_exception",
                    (
                        f"**iter {before}** — driver hit `{type(e).__name__}` "
                        f"and would normally crash; harness recovered, "
                        f"continuing to next iter.\n\n"
                        f"**Error**: `{str(e)[:400]}`{extra}"
                    ),
                    requires_ack=False,
                    root=root,
                )
                # Best-effort: revert any half-applied working tree from this iter.
                try:
                    git_ops.revert_working_tree(root)
                except Exception:
                    pass
                # Reload db from disk in case the in-memory copy is stale
                # (the iter may have been mid-write when it crashed).
                db = load_db(root)
            except Exception:
                # If even the recovery path fails, just keep going — better
                # to lose one iter's bookkeeping than halt the driver.
                pass
            continue
        if len(db.iterations) == before:
            break
        if db.phase_state.plateau_counter >= CONFIG.plateau_patience:
            # Autopilot pivot: ban every family seen in the last N iters,
            # reset the plateau counter, let the proposer generate fresh
            # structurally-different candidates on the next iteration.
            # No user input required. If this is the Nth consecutive
            # plateau-without-a-ship, hard-halt (something's wrong with
            # the problem framing, not just the family pool).
            if not args.dry_run:
                halted = _pivot_on_plateau(db, root)
                save_db(db, root)
                if halted:
                    break
            else:
                # Dry-run: still break, no SDK calls.
                break
    return 0


if __name__ == "__main__":
    sys.exit(main())
