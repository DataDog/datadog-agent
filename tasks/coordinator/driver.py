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
import sys
from pathlib import Path

from . import budget as budget_mod
from . import coord_out, evaluator, git_ops, journal, metrics, scheduler, workspace_validate
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

            interpretation, planned_change = sdk.interpret_inbox_message(msg.content)
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

def primary_detector(candidate: Candidate) -> str | None:
    for c in candidate.target_components:
        if c in {"bocpd", "scanmw", "scanwelch"}:
            return c
    return None


# ---------------------------------------------------------------------------
# Full iteration (TODOs 2–5)
# ---------------------------------------------------------------------------

def run_iteration(db: Db, root: Path, dry_run: bool = False) -> Db:
    iter_num = len(db.iterations)
    it = Iteration(number=iter_num, started_at=now_iso())

    with budget_mod.WallTimer(db.budget):
        _run_iteration_body(db, root, dry_run, iter_num, it)

    if not dry_run:
        new_msgs = budget_mod.check_milestones(db.budget, root)
        for m in new_msgs:
            journal.append(
                "budget_milestone_emitted",
                {"type": m.type, "requires_ack": m.requires_ack},
                root,
            )
        # Persist updated wall-hours.
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

    # 1. Process inbox
    it.inbox_acks = process_inbox(db, root, dry_run)

    # 1a. Sync from upstream feature branch (q-branch-observer) so the
    # coordinator picks up any newly-merged work before iterating. We're
    # running on claude/observer-improvements, not merging into upstream — this is one-way.
    # On conflict the iteration halts with a coord-out message; the human
    # resolves by hand (rebase or abandon the diverging claude/observer-improvements commit).
    if not dry_run:
        git_ops.ensure_scratch_branch(root)
        sync = git_ops.sync_from_upstream(root)
        if sync.get("merged") or sync.get("ahead_count"):
            journal.append("upstream_sync", sync, root)
        if sync.get("conflict"):
            print(
                f"[iter {iter_num}] upstream sync CONFLICT; aborting iteration",
                file=sys.stderr,
            )
            journal.append("upstream_sync_conflict", sync, root)
            coord_out.emit(
                "upstream_conflict",
                f"Sync from `origin/{git_ops.UPSTREAM_BRANCH}` conflicted with "
                f"claude/observer-improvements. Merge was aborted; working tree restored. "
                f"Manual resolution required — rebase claude/observer-improvements onto the new "
                f"upstream tip, or reset and replay candidates. "
                f"Coordinator halted.\n\n```\n{sync.get('error') or ''}\n```",
                requires_ack=True,
                root=root,
            )
            it.ended_at = now_iso()
            db.iterations.append(it)
            save_db(db, root)
            metrics.regenerate(db, root)
            return
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
    detector = primary_detector(candidate)
    if detector is None:
        print(f"[iter {iter_num}] candidate {candidate.id} has no target detector; skipping")
        candidate.status = CandidateStatus.REJECTED
        it.ended_at = now_iso()
        db.iterations.append(it)
        if not dry_run:
            save_db(db, root)
            metrics.regenerate(db, root)
        return

    experiment_id = f"exp-{iter_num:04d}-{candidate.id}"
    print(f"[iter {iter_num}] candidate={candidate.id} detector={detector}")

    # 2a. Refuse to run on a dirty tree
    if not dry_run and not git_ops.is_clean(root):
        print(f"[iter {iter_num}] working tree dirty; aborting iteration", file=sys.stderr)
        journal.append("iteration_aborted_dirty_tree", {"iter": iter_num}, root)
        it.ended_at = now_iso()
        db.iterations.append(it)
        save_db(db, root)
        return

    # 2b. Capture pre-SHA (scratch branch already ensured + sync'd at step 1a).
    # Post-sync SHA is the correct baseline for the candidate's commit.
    pre_sha = git_ops.head_sha(root) if not dry_run else "dry-run"

    # 3. Implement (SDK)
    impl_summary = "[dry-run] SDK not called"
    if not dry_run:
        from . import sdk

        try:
            impl_summary = sdk.implement_candidate(candidate, root)
        except Exception as e:
            print(f"[iter {iter_num}] implementation failed: {e}", file=sys.stderr)
            journal.append(
                "implementation_failed", {"iter": iter_num, "error": str(e)}, root
            )
            git_ops.revert_working_tree(root)
            it.ended_at = now_iso()
            db.iterations.append(it)
            save_db(db, root)
            return

    journal.append(
        "implementation_done",
        {"iter": iter_num, "candidate": candidate.id, "summary": impl_summary},
        root,
    )

    # 4. Eval
    report_path = state_dir(root) / "reports" / f"{experiment_id}.json"
    scenario_out = state_dir(root) / "reports" / experiment_id

    if dry_run:
        eval_ok = True
        eval_msg = "[dry-run]"
    else:
        run = evaluator.run_scenarios(
            detector=detector,
            report_path=report_path,
            scenario_output_dir=scenario_out,
            root=root,
        )
        eval_ok = run.ok
        eval_msg = run.stderr[-500:] if run.stderr else "ok"
        journal.append(
            "eval_done",
            {
                "iter": iter_num,
                "candidate": candidate.id,
                "detector": detector,
                "ok": run.ok,
                "report_path": str(report_path),
                "rc": run.returncode,
            },
            root,
        )

    experiment = Experiment(
        id=experiment_id,
        candidate_id=candidate.id,
        phase=candidate.phase,
        tier=Tier.T0,
        commit_sha=pre_sha,
        config_path="",
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
    scoring = score_against_baseline(
        report_path,
        db.baseline,
        detector,
        tau=CONFIG.tau_default,
        train_scenarios=train_set,
    )
    experiment.score = scoring.mean_f1
    experiment.num_baseline_fps_sum = scoring.total_fps
    experiment.per_scenario = scoring.per_scenario
    experiment.scenario_set = list(scoring.per_scenario.keys())

    # 5a. Update phase state based on SCORE alone (approval-independent).
    # This matches the allium spec HandleT0Completion rule and removes the
    # plateau-on-rejection bug.
    if scoring.mean_f1 > db.phase_state.best_score:
        db.phase_state.best_score = scoring.mean_f1
        db.phase_state.plateau_counter = 0
    else:
        db.phase_state.plateau_counter += 1

    # 5b. Hard strict-regression gate — reject without review if any train
    # scenario regressed > tau or any defended recall floor was violated.
    # Keeps the gate deterministic instead of relying on LLM personas to catch it.
    if scoring.strict_regressions or scoring.recall_floor_violations:
        reason = (
            f"strict_regressions={scoring.strict_regressions} "
            f"recall_violations={scoring.recall_floor_violations}"
        )
        print(f"[iter {iter_num}] AUTO-REJECTED ({reason})")
        journal.append(
            "auto_rejected_strict_regression",
            {"iter": iter_num, "candidate": candidate.id, "reason": reason},
            root,
        )
        coord_out.emit(
            "strict_regression",
            f"Candidate `{candidate.id}` auto-rejected at iter {iter_num}: {reason}. "
            f"Working tree reverted; no commit.",
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
        verdict = sdk.review_experiment(experiment, scoring, candidate.phase)
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
        commit_sha = git_ops.commit_candidate(candidate.id, experiment_id, root)
        experiment.commit_sha = commit_sha
        candidate.status = CandidateStatus.SHIPPED
        # Persist the ship BEFORE pushing. If push (or a later step) crashes,
        # db.yaml already reflects the commit; startup_cleanup will push the
        # orphan commit on restart.
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
        # Fire-and-forget post-ship eval-component validation on the matching
        # workspace. Non-blocking — result ports back at a future iteration via
        # poll_pending_validations.
        pv = workspace_validate.dispatch_validation(
            experiment_id=experiment_id,
            candidate_id=candidate.id,
            detector=detector,
            db=db,
            root=root,
        )
        if pv:
            # Persist immediately so a crash between dispatch and iteration-end
            # doesn't lose track of the in-flight workspace run.
            save_db(db, root)
            print(f"[iter {iter_num}] validation dispatched: {pv.id} on {pv.workspace}")
    else:
        git_ops.revert_working_tree(root)
        candidate.status = CandidateStatus.REJECTED
        print(f"[iter {iter_num}] REJECTED; reverted (score {scoring.mean_f1:.4f})")

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
    args = parser.parse_args(argv)

    root = Path(args.root)
    state_dir(root).mkdir(parents=True, exist_ok=True)

    db = load_db(root)
    if db.baseline is None:
        print(
            "warning: baseline not loaded. Run `coordinator.import_baseline` first.",
            file=sys.stderr,
        )

    # Crash-recovery: revert any orphaned working-tree diffs, push any
    # unpushed commits on claude/observer-improvements. Safe no-op on a clean restart.
    if not args.dry_run:
        cleanup = git_ops.startup_cleanup(root)
        if cleanup.get("reverted_dirty_tree") or cleanup.get("pushed_orphan_commits"):
            journal.append("startup_cleanup", cleanup, root)
            print(f"[startup] cleanup: {cleanup}")

    if args.once or not args.forever:
        run_iteration(db, root, dry_run=args.dry_run)
        return 0

    while True:
        before = len(db.iterations)
        db = run_iteration(db, root, dry_run=args.dry_run)
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
