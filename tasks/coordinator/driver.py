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
    Tier,
)
from .scoring import score_against_baseline


def now_iso() -> str:
    return _dt.datetime.now().isoformat(timespec="seconds")


class ProtectedPathTamperHalt(Exception):
    """Raised when ground-truth fixtures changed between iterations."""


PAUSE_FILE = "pause"


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
      B. iter_cost > cost_anomaly_absolute_usd
      C. iter_tokens > cost_anomaly_absolute_tokens
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
    if iter_cost > CONFIG.cost_anomaly_absolute_usd:
        triggers.append(
            f"iter cost ${iter_cost:.2f} > absolute ${CONFIG.cost_anomaly_absolute_usd:.0f}"
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
    return ack_ids


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
        base_mean = (
            db.baseline.detectors.get(
                next(iter(candidate.target_components), ""), None
            ).mean_f1
            if db.baseline
            and next(iter(candidate.target_components), "") in db.baseline.detectors
            else None
        )
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


KNOWN_DETECTORS = ("bocpd", "scanmw", "scanwelch")


def relevant_detectors(candidate: Candidate) -> list[str]:
    """Which detectors' F1 do we measure to decide if this candidate shipped?

    A candidate can modify any file under comp/observer/. But scoring runs
    per-detector, and the panel review caught this: silently defaulting to
    ONE detector (previously scanmw) meant candidates modifying e.g. bocpd
    internals got scored against scanmw's unaffected output — ΔF1≈0 by
    construction, "improvement" or "regression" both invisible.

    Policy:
      - Intersect target_components with the 3 known detectors. If the
        intersection is non-empty, eval each one in it and gate on the
        WORST ΔF1 across them.
      - If the intersection is empty (correlator changes, new features,
        pipeline-level work), eval ALL 3 detectors — we can't tell in
        advance which one the change affects, so measure them all.

    Always returns a non-empty list.
    """
    named = [c for c in candidate.target_components if c in KNOWN_DETECTORS]
    if named:
        return named
    return list(KNOWN_DETECTORS)


def primary_detector(candidate: Candidate) -> str:
    """Deprecated single-detector view. Kept for callers that print a
    single string (metrics, log lines). Returns the first relevant detector.
    """
    return relevant_detectors(candidate)[0]


def _merge_scorings(scorings: dict, detectors: list[str]):
    """Combine per-detector ScoringResults into one gate-on-worst view.

    - `mean_f1` = simple average of detector means.
    - `strict_regressions` / `recall_floor_violations` = union across
      detectors, prefixed with `<detector>/` so the reviewer can see which
      detector is suffering.
    - `per_scenario` / `per_scenario_delta` keyed by `<detector>/<scenario>`
      so downstream (review prompts, reeval_ships) sees the full surface.
    - `total_fps` / `baseline_total_fps` = sum across detectors.
    """
    from .scoring import ScoringResult

    if not scorings:
        # No usable detector scores — construct an empty no-op result so the
        # caller treats this as eval failure rather than crashing.
        return ScoringResult(
            detector="|".join(detectors) or "unknown",
            mean_f1=0.0, total_fps=0, per_scenario={},
            baseline_mean_f1=0.0, baseline_total_fps=0,
            mean_df1=0.0, total_dfps=0, per_scenario_delta={},
            strict_regressions=[], recall_floor_violations=[],
            fp_reduction_pct=0.0,
        )

    means = [s.mean_f1 for s in scorings.values()]
    base_means = [s.baseline_mean_f1 for s in scorings.values()]
    mean_f1 = sum(means) / len(means)
    base_mean_f1 = sum(base_means) / len(base_means)
    total_fps = sum(s.total_fps for s in scorings.values())
    base_total_fps = sum(s.baseline_total_fps for s in scorings.values())

    merged_per_scenario = {}
    merged_deltas = {}
    strict = []
    recall_v = []
    for det, s in scorings.items():
        for scen, sr in s.per_scenario.items():
            merged_per_scenario[f"{det}/{scen}"] = sr
        for scen, sd in s.per_scenario_delta.items():
            merged_deltas[f"{det}/{scen}"] = sd
        strict.extend(f"{det}/{n}" for n in s.strict_regressions)
        recall_v.extend(f"{det}/{n}" for n in s.recall_floor_violations)

    fp_reduction_pct = 0.0
    if base_total_fps > 0:
        fp_reduction_pct = (base_total_fps - total_fps) / base_total_fps

    return ScoringResult(
        detector="+".join(detectors),
        mean_f1=mean_f1,
        total_fps=total_fps,
        per_scenario=merged_per_scenario,
        baseline_mean_f1=base_mean_f1,
        baseline_total_fps=base_total_fps,
        mean_df1=mean_f1 - base_mean_f1,
        total_dfps=total_fps - base_total_fps,
        per_scenario_delta=merged_deltas,
        strict_regressions=strict,
        recall_floor_violations=recall_v,
        fp_reduction_pct=fp_reduction_pct,
    )


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
        iter_in, iter_out = token_log.sum_total(
            token_log.filter_by_iter(records, len(db.iterations))
        )
        total_in, total_out = token_log.sum_total(records)
        db.budget.api_tokens_used = total_in + total_out
        if iter_in or iter_out:
            journal.append(
                "tokens_used",
                {
                    "iter": len(db.iterations),
                    "iter_input": iter_in,
                    "iter_output": iter_out,
                    "iter_cost_usd": round(
                        token_log.cost_estimate(
                            token_log.filter_by_iter(records, len(db.iterations))
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
        _check_cost_anomaly(db, root, iter_num=len(db.iterations), records=records)

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
    detectors = relevant_detectors(candidate)
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
            coord_out.emit(
                "iter_impl_failed",
                (
                    f"**iter {iter_num}** · `{candidate.id}` — "
                    f"implementation agent crashed. Candidate marked "
                    f"REJECTED to prevent loop.\n\n"
                    f"Error: `{str(e)[:400]}`\n\n"
                    f"Working tree reverted; moving on."
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

    # 4. Eval — one run per relevant detector. Reports go to per-detector
    # subdirs so we keep full visibility across the set.
    reports_by_detector: dict[str, Path] = {}
    eval_ok = True
    eval_msg = "ok"

    if dry_run:
        eval_msg = "[dry-run]"
    else:
        for det in detectors:
            rp = state_dir(root) / "reports" / f"{experiment_id}-{det}.json"
            sd = state_dir(root) / "reports" / f"{experiment_id}-{det}"
            run = evaluator.run_scenarios(
                detector=det,
                report_path=rp,
                scenario_output_dir=sd,
                root=root,
            )
            journal.append(
                "eval_done",
                {
                    "iter": iter_num,
                    "candidate": candidate.id,
                    "detector": det,
                    "ok": run.ok,
                    "report_path": str(rp),
                    "rc": run.returncode,
                },
                root,
            )
            if not run.ok:
                eval_ok = False
                eval_msg = f"{det}: {(run.stderr or 'failed')[-500:]}"
                break
            reports_by_detector[det] = rp

    # The Experiment record keeps one canonical report_path for UI/metrics
    # continuity — use the first detector's report, but remember all are
    # available via reports_by_detector for scoring.
    report_path = next(iter(reports_by_detector.values()), None) or (
        state_dir(root) / "reports" / f"{experiment_id}-{detectors[0]}.json"
    )
    scenario_out = state_dir(root) / "reports" / f"{experiment_id}-{detectors[0]}"

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
    # or recall-floor violation rejects the candidate); combined
    # per_scenario uses keys "<detector>/<scenario>" to preserve full
    # visibility; combined mean_f1 is the simple mean of per-detector
    # means. Rolling "last shipped" reference was dropped earlier (it
    # introduced a noise ratchet).
    scorings: dict[str, "object"] = {}
    for det in detectors:
        rp = reports_by_detector.get(det)
        if rp is None:
            continue
        scorings[det] = score_against_baseline(
            rp,
            db.baseline,
            det,
            train_scenarios=train_set,
        )

    scoring = _merge_scorings(scorings, detectors)
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
    fp_ceiling = int(scoring.baseline_total_fps * CONFIG.fp_ceiling_ratio)
    fp_ceiling_breached = (
        scoring.baseline_total_fps > 0
        and scoring.total_fps > fp_ceiling
    )
    if (
        scoring.strict_regressions
        or scoring.recall_floor_violations
        or fp_ceiling_breached
    ):
        fp_reason = ""
        if fp_ceiling_breached:
            fp_reason = (
                f" fp_ceiling_breached={scoring.total_fps} > "
                f"{fp_ceiling} (ratio {CONFIG.fp_ceiling_ratio}× baseline "
                f"{scoring.baseline_total_fps})"
            )
        reason = (
            f"strict_regressions={scoring.strict_regressions} "
            f"recall_violations={scoring.recall_floor_violations}"
            f"{fp_reason}"
        )
        # Persist on the experiment so the proposer can see WHY this was
        # rejected on the next iteration's research-memory context.
        # Without this, the proposer only gets aggregate score_delta and
        # doesn't know which specific scenarios broke.
        experiment.auto_reject_reason = reason
        print(f"[iter {iter_num}] AUTO-REJECTED ({reason})")
        journal.append(
            "auto_rejected_strict_regression",
            {"iter": iter_num, "candidate": candidate.id, "reason": reason},
            root,
        )
        # Build a compact per-scenario delta summary — the biggest 5
        # |ΔF1| swings, both helpful and harmful, so the reader sees
        # WHY it was rejected at a glance.
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
        coord_out.emit(
            "strict_regression",
            (
                f"**iter {iter_num}** · `{candidate.id}` — "
                f"auto-rejected on catastrophe filter.\n\n"
                f"**Gate failures**: {reason}\n\n"
                f"**Top 5 |ΔF1| scenarios**:\n{delta_lines}\n\n"
                f"Observed mean F1 {scoring.mean_f1:.4f} vs baseline "
                f"{scoring.baseline_mean_f1:.4f} (Δ{scoring.mean_df1:+.4f}). "
                f"Total FPs {scoring.baseline_total_fps} → {scoring.total_fps} "
                f"(Δ{scoring.total_dfps:+d}).\n\n"
                f"Working tree reverted; no commit."
                + _budget_footer(root, iter_num, ceiling=db.budget.api_token_ceiling)
            ),
            requires_ack=False,
            root=root,
        )
        git_ops.revert_working_tree(root)
        candidate.status = CandidateStatus.REJECTED
        it.ended_at = now_iso()
        db.iterations.append(it)
        save_db(db, root)
        metrics.regenerate(db, root)
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
            iter_num=iter_num,
        )
    except Exception as e:
        print(f"[iter {iter_num}] review failed: {e}", file=sys.stderr)
        journal.append("review_failed", {"iter": iter_num, "error": str(e)}, root)
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
        git_ops.revert_working_tree(root)
        candidate.status = CandidateStatus.REJECTED
        print(f"[iter {iter_num}] REJECTED; reverted (score {scoring.mean_f1:.4f})")

        # Review-level reject (the catastrophe filter didn't fire, but the
        # panel said no). Show each persona's verdict for transparency.
        rationale_lines = "\n".join(
            f"- **{dec.persona}** ({'approve' if dec.approve else 'reject'}): "
            f"{dec.rationale[:400]}"
            for dec in verdict.decisions
        )
        coord_out.emit(
            "iter_rejected",
            (
                f"**iter {iter_num}** · `{candidate.id}` — rejected by review "
                f"(passed deterministic gates, failed unanimity).\n\n"
                f"Mean F1 {scoring.baseline_mean_f1:.4f} → {scoring.mean_f1:.4f} "
                f"(Δ{scoring.mean_df1:+.4f}).\n\n"
                f"**Reviewer verdicts**:\n{rationale_lines}\n\n"
                f"Working tree reverted; no commit."
                + _budget_footer(root, iter_num, ceiling=db.budget.api_token_ceiling)
            ),
            requires_ack=False,
            root=root,
        )

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
        print(
            f"[startup] --token-ceiling override: {db.budget.api_token_ceiling} "
            f"→ {args.token_ceiling}",
            file=sys.stderr,
        )
        db.budget.api_token_ceiling = args.token_ceiling
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
                if cand is not None and cand.status == CandidateStatus.SHIPPED:
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
        if len(db.iterations) == before:
            break
        if db.phase_state.plateau_counter >= CONFIG.plateau_patience:
            print(f"phase {db.phase_state.current_phase.value} plateaued; exiting")
            if not args.dry_run:
                coord_out.emit(
                    "phase_exit",
                    f"Phase {db.phase_state.current_phase.value} plateaued after "
                    f"{CONFIG.plateau_patience} consecutive non-improving iterations. "
                    f"Best score: {db.phase_state.best_score:.4f}. "
                    "Coordinator exiting; write `.coordinator/inbox.md` to redirect.",
                    requires_ack=True,
                    root=root,
                )
            break
    return 0


if __name__ == "__main__":
    sys.exit(main())
