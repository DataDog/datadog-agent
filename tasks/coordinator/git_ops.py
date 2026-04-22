"""Git plumbing for candidate implementation.

All candidate work happens on a dedicated branch (default `claude/observer-improvements`)
so the user's main branch is never touched destructively. Approved
candidates are committed; rejected ones have their working-tree changes
discarded before the next iteration.

Safety:
  - precheck_clean() refuses to run if observer files are dirty outside
    the scratch branch (would risk silently reverting user work).
  - revert_working_tree() uses `git checkout -- .` which only discards
    staged/unstaged tracked changes — it never nukes the branch itself.
"""

from __future__ import annotations

import subprocess
from pathlib import Path

SCRATCH_BRANCH = "claude/observer-improvements"
UPSTREAM_BRANCH = "q-branch-observer"
# Only comp/observer/ is committable by the coordinator, with the
# critical EXCEPTION of comp/observer/scenarios/ — that directory holds
# ground-truth labels (ground_truth.json) and episode windows
# (episode.json), which define what "improvement" even means. If the
# agent could edit those, it would trivially reward-hack by relabeling
# FPs as TPs or widening disruption windows. The scenarios dir is
# snapshot-hashed at coordinator startup and verified unchanged at
# every iteration boundary.
WATCH_PATHS = ["comp/observer"]
EXCLUDE_PATHS = ["comp/observer/scenarios"]
PROTECTED_PATHS = ["comp/observer/scenarios"]  # integrity-checked at iteration start


def _run(args: list[str], root: Path, check: bool = True) -> subprocess.CompletedProcess:
    return subprocess.run(
        ["git", *args],
        cwd=root,
        capture_output=True,
        text=True,
        check=check,
    )


def current_branch(root: Path = Path(".")) -> str:
    return _run(["rev-parse", "--abbrev-ref", "HEAD"], root).stdout.strip()


def head_sha(root: Path = Path(".")) -> str:
    return _run(["rev-parse", "HEAD"], root).stdout.strip()


def is_clean(root: Path = Path("."), paths: list[str] | None = None) -> bool:
    """Return True if the working tree has no uncommitted changes under paths."""
    paths = paths or WATCH_PATHS
    # --porcelain is stable; each modified file = 1 line.
    res = _run(["status", "--porcelain", "--", *paths], root)
    return res.stdout.strip() == ""


def ensure_scratch_branch(
    root: Path = Path("."),
    branch: str = SCRATCH_BRANCH,
    upstream: str = UPSTREAM_BRANCH,
    remote: str = "origin",
) -> None:
    """Create the scratch branch at `origin/<upstream>` if it doesn't exist,
    and check it out.

    Rooting the scratch branch at `origin/<upstream>` (not current HEAD) makes
    the coordinator behave consistently regardless of which branch the user
    was on when they started it. If the scratch branch already exists, it is
    checked out as-is and `sync_from_upstream` will fast-forward it on the
    next iteration.
    """
    branches = _run(["branch", "--list", branch], root).stdout
    if branch not in branches:
        # Fetch BOTH the upstream feature branch AND the remote copy of
        # the scratch branch if it already exists on origin (created e.g.
        # by a prior coordinator run or by a human opening the run-log
        # PR). Preference order:
        #   1. origin/<scratch>  — someone already pushed; track it so
        #      `git push` later is a fast-forward, not a non-FF rejection.
        #   2. origin/<upstream> — first-ever run, no remote scratch yet.
        #   3. local <upstream>  — no remote at all (test repos).
        #   4. current HEAD      — nothing else resolvable.
        fetch_upstream(root, remote=remote, branch=upstream)
        _run(["fetch", remote, branch], root, check=False)
        remote_scratch = f"{remote}/{branch}"
        probe_remote_scratch = _run(
            ["rev-parse", "--verify", remote_scratch], root, check=False
        )
        if probe_remote_scratch.returncode == 0:
            # `-b branch remote_scratch` creates local tracking remote.
            _run(["checkout", "-b", branch, remote_scratch], root)
            # Ensure upstream tracking so `git push` with no args works too.
            _run(["branch", "--set-upstream-to", remote_scratch, branch], root, check=False)
            return
        upstream_ref = f"{remote}/{upstream}"
        probe_remote = _run(["rev-parse", "--verify", upstream_ref], root, check=False)
        if probe_remote.returncode == 0:
            _run(["checkout", "-b", branch, upstream_ref], root)
        else:
            probe_local = _run(["rev-parse", "--verify", upstream], root, check=False)
            if probe_local.returncode == 0:
                _run(["checkout", "-b", branch, upstream], root)
            else:
                _run(["checkout", "-b", branch], root)
    else:
        _run(["checkout", branch], root)


class WrongBranchError(RuntimeError):
    """Raised when coordinator would commit on a branch other than SCRATCH_BRANCH."""


def assert_on_scratch_branch(root: Path = Path(".")) -> None:
    b = current_branch(root)
    if b != SCRATCH_BRANCH:
        raise WrongBranchError(
            f"refusing to operate on branch '{b}'; coordinator only commits on '{SCRATCH_BRANCH}'"
        )


def commit_candidate(
    candidate_id: str,
    experiment_id: str,
    root: Path = Path("."),
    paths: list[str] | None = None,
) -> str:
    """Commit working-tree changes on the scratch branch. Returns new HEAD SHA.

    Refuses to run if HEAD is not on SCRATCH_BRANCH — protects the user's main
    branch from accidental coordinator commits. Ground-truth fixtures under
    EXCLUDE_PATHS are never staged: the agent is not permitted to modify
    scoring labels, so any drift there is ignored rather than frozen into a
    commit. (Integrity-hash verify runs separately at iteration start.)
    """
    assert_on_scratch_branch(root)
    paths = paths or WATCH_PATHS
    # Pathspec `:(exclude)<p>` tells git to skip files under <p> even when
    # their parent is in the include list. Without this, `git add comp/observer`
    # would stage any edits to comp/observer/scenarios/ ground-truth files.
    pathspec = list(paths) + [f":(exclude){p}" for p in EXCLUDE_PATHS]
    _run(["add", "--", *pathspec], root)
    msg = f"coord: {candidate_id} ({experiment_id})"
    _run(["commit", "-m", msg, "--allow-empty"], root)
    return head_sha(root)


def tree_hash(root: Path, path: str) -> str:
    """Recursive content hash of `<root>/<path>`. Returns empty string if path
    doesn't exist. Used to detect agent tampering with ground-truth fixtures
    between iterations.
    """
    p = root / path
    if not p.exists():
        return ""
    # git ls-tree -r HEAD <path> gives (mode, type, sha, name) lines for
    # committed state — but we want CURRENT disk state (agent edits to
    # ground-truth are uncommitted). Use hash-object on each file for a
    # stable content-addressed snapshot.
    import hashlib
    h = hashlib.sha256()
    for f in sorted(p.rglob("*")):
        if not f.is_file():
            continue
        rel = str(f.relative_to(root))
        h.update(rel.encode())
        h.update(b"\0")
        h.update(f.read_bytes())
        h.update(b"\0")
    return h.hexdigest()


class ProtectedPathTamperError(RuntimeError):
    """Raised when a PROTECTED_PATH tree hash changes between snapshots.

    Ground-truth files (scenarios/) are the scoring labels. Any change there
    is either a benign upstream merge OR an agent reward-hack. The coordinator
    halts on detection and a human decides which.
    """


def revert_working_tree(root: Path = Path("."), paths: list[str] | None = None) -> None:
    """Discard uncommitted changes under `paths`. Never touches branches.

    Silently skips paths that don't exist on disk — recent git versions
    error out when a pathspec matches nothing, which would leave orphan
    changes in sibling paths untouched.
    """
    paths = paths or WATCH_PATHS
    existing = [p for p in paths if (root / p).exists()]
    if not existing:
        return
    _run(["checkout", "--", *existing], root, check=False)
    # Also clean untracked files under these paths so the next run starts fresh.
    _run(["clean", "-fd", "--", *existing], root, check=False)


def push_scratch_branch(
    root: Path = Path("."),
    remote: str = "origin",
    branch: str = SCRATCH_BRANCH,
) -> tuple[bool, str]:
    """Push the scratch branch to origin. Refuses to push any other branch.

    Returns (ok, message). Caller should log failures but not abort — the
    local commit is still the source of truth.
    """
    assert_on_scratch_branch(root)
    # `-u` is idempotent: sets upstream on first push, no-op after.
    # `--no-verify`: claude/observer-improvements is a draft scratch branch
    # (run-log PR), never merged to mainline. Repo-level pre-push hooks
    # like go-test / invoke-based checks are designed for real PRs and
    # can take tens of minutes or fail on unrelated env drift (missing
    # `invoke` module, etc.). Bypassing them here is correct for this
    # branch's purpose and unblocks the coordinator loop; real merge
    # review still happens via the offline reeval_ships → cherry-pick
    # onto a clean branch workflow.
    res = _run(
        ["push", "-u", "--no-verify", remote, branch],
        root,
        check=False,
    )
    if res.returncode == 0:
        return True, res.stderr.strip() or res.stdout.strip() or "ok"
    return False, (res.stderr or res.stdout).strip()


def unpushed_commit_count(
    root: Path = Path("."),
    remote: str = "origin",
    branch: str = SCRATCH_BRANCH,
) -> int:
    """How many local commits on `branch` are ahead of `remote/branch`?

    Returns 0 if remote branch doesn't exist yet OR if the branches are in
    sync. Used at startup to reconcile crashes between commit and push.
    """
    # `remote/branch` may not exist on first run. Probe it first.
    res = _run(
        ["rev-parse", "--verify", f"{remote}/{branch}"],
        root,
        check=False,
    )
    if res.returncode != 0:
        # No remote branch yet — treat as "nothing to reconcile."
        return 0
    res = _run(
        ["rev-list", "--count", f"{remote}/{branch}..{branch}"],
        root,
        check=False,
    )
    if res.returncode != 0:
        return 0
    try:
        return int(res.stdout.strip())
    except ValueError:
        return 0


def fetch_upstream(
    root: Path = Path("."),
    remote: str = "origin",
    branch: str = UPSTREAM_BRANCH,
) -> tuple[bool, str]:
    """Fetch the feature branch from origin. Returns (ok, msg)."""
    res = _run(["fetch", remote, branch], root, check=False)
    if res.returncode == 0:
        return True, res.stderr.strip() or "ok"
    return False, (res.stderr or res.stdout).strip()


def upstream_ahead_count(
    root: Path = Path("."),
    remote: str = "origin",
    branch: str = UPSTREAM_BRANCH,
) -> int:
    """How many commits on origin/<branch> are not yet on the current branch?"""
    res = _run(
        ["rev-parse", "--verify", f"{remote}/{branch}"],
        root,
        check=False,
    )
    if res.returncode != 0:
        return 0
    res = _run(
        ["rev-list", "--count", f"HEAD..{remote}/{branch}"],
        root,
        check=False,
    )
    if res.returncode != 0:
        return 0
    try:
        return int(res.stdout.strip())
    except ValueError:
        return 0


def sync_from_upstream(
    root: Path = Path("."),
    remote: str = "origin",
    branch: str = UPSTREAM_BRANCH,
) -> dict:
    """Fetch upstream + merge into the scratch branch if anything new lands.

    Must be called on SCRATCH_BRANCH. Uses `--no-edit` merge so history shows
    "Merge branch 'origin/<upstream>'" without pausing for an editor. On
    conflict, aborts the merge (working tree restored) and returns a dict
    with conflict=True.

    Returns:
        {
          "fetched": bool,              # fetch succeeded?
          "ahead_count": int,           # commits pulled (0 = no-op)
          "merged": bool,               # merge actually ran?
          "merge_sha": str | None,      # head after merge, if merged
          "conflict": bool,             # merge aborted on conflict?
          "error": str | None,          # any non-conflict failure message
        }
    """
    assert_on_scratch_branch(root)

    summary: dict = {
        "fetched": False,
        "ahead_count": 0,
        "merged": False,
        "merge_sha": None,
        "conflict": False,
        "error": None,
    }

    ok, msg = fetch_upstream(root, remote=remote, branch=branch)
    summary["fetched"] = ok
    if not ok:
        summary["error"] = f"fetch failed: {msg}"
        return summary

    ahead = upstream_ahead_count(root, remote=remote, branch=branch)
    summary["ahead_count"] = ahead
    if ahead == 0:
        return summary

    merge_target = f"{remote}/{branch}"
    res = _run(
        ["merge", "--no-edit", "--no-ff", merge_target],
        root,
        check=False,
    )
    if res.returncode == 0:
        summary["merged"] = True
        summary["merge_sha"] = head_sha(root)
        return summary

    # Conflict (or other error) — abort to restore working tree.
    _run(["merge", "--abort"], root, check=False)
    combined = (res.stdout + "\n" + res.stderr).lower()
    summary["conflict"] = "conflict" in combined or "would be overwritten" in combined
    summary["error"] = (res.stderr or res.stdout).strip()[-400:]
    return summary


def startup_cleanup(
    root: Path = Path("."),
    paths: list[str] | None = None,
) -> dict:
    """Reconcile scratch-branch state before entering the iteration loop.

    Handles three crash-recovery scenarios:
      1. Stale in-progress merge/rebase/cherry-pick state (.git/MERGE_HEAD
         etc). A prior sync_from_upstream SIGKILLed between `git merge`
         and `git merge --abort` leaves this residue; the next commit
         would silently create a merge commit carrying unintended content.
         Abort all in-progress operations before anything else.
      2. Orphaned working-tree diffs under WATCH_PATHS (a previous
         iteration crashed mid-implementation). Revert to HEAD.
      3. Unpushed commits on claude/observer-improvements from a prior crash between
         commit and push. Push them now.

    Returns a dict summary for journal logging.
    """
    paths = paths or WATCH_PATHS
    summary: dict = {
        "aborted_stale_merge": False,
        "reverted_dirty_tree": False,
        "pushed_orphan_commits": 0,
        "push_ok": None,
    }

    # 1. Stale merge/rebase/cherry-pick state → abort. These files live
    # under .git/ and persist across crashes; the next operation would
    # inherit them. Abort order: merge, cherry-pick, rebase — each is a
    # no-op if the corresponding state isn't present.
    git_dir = root / ".git"
    if (git_dir / "MERGE_HEAD").exists():
        _run(["merge", "--abort"], root, check=False)
        summary["aborted_stale_merge"] = True
    if (git_dir / "CHERRY_PICK_HEAD").exists():
        _run(["cherry-pick", "--abort"], root, check=False)
        summary["aborted_stale_merge"] = True
    if (git_dir / "rebase-merge").exists() or (git_dir / "rebase-apply").exists():
        _run(["rebase", "--abort"], root, check=False)
        summary["aborted_stale_merge"] = True

    # 2. Dirty working tree under watched paths → revert.
    if not is_clean(root, paths):
        revert_working_tree(root, paths)
        summary["reverted_dirty_tree"] = True

    # 2. Only attempt push reconciliation if we're on the scratch branch
    # (otherwise we don't know whether unpushed commits are ours).
    try:
        if current_branch(root) == SCRATCH_BRANCH:
            ahead = unpushed_commit_count(root)
            if ahead > 0:
                ok, msg = push_scratch_branch(root)
                summary["pushed_orphan_commits"] = ahead
                summary["push_ok"] = ok
                summary["push_msg"] = msg
    except subprocess.CalledProcessError:
        # Best-effort; the main loop will catch branch issues on first iter.
        pass

    return summary
