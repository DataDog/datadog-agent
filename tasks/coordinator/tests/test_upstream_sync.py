"""Tests for git_ops.sync_from_upstream against a real local repo pair.

Creates a bare 'origin' with a q-branch-observer and a local clone with a
claude/observer-improvements branch, then exercises the three paths:
  - no upstream commits → no-op
  - upstream has new commits → fast-forward-ish merge
  - conflicting change on claude/observer-improvements → merge aborted, tree restored
"""

import subprocess
from pathlib import Path

import pytest

from coordinator import git_ops


def _shell(args, cwd, check=True):
    return subprocess.run(args, cwd=cwd, check=check, capture_output=True, text=True)


@pytest.fixture
def repo_pair(tmp_path: Path):
    """Build origin (bare) + clone (working copy) with q-branch-observer and claude/observer-improvements."""
    origin = tmp_path / "origin.git"
    clone = tmp_path / "clone"

    _shell(["git", "init", "--bare", "-b", "q-branch-observer", str(origin)], cwd=tmp_path)

    # Seed clone with an initial commit on q-branch-observer.
    _shell(["git", "init", "-q", "-b", "q-branch-observer", str(clone)], cwd=tmp_path)
    _shell(["git", "config", "user.email", "t@t"], cwd=clone)
    _shell(["git", "config", "user.name", "t"], cwd=clone)
    (clone / "comp").mkdir()
    (clone / "comp" / "observer").mkdir()
    (clone / "comp" / "observer" / "x.go").write_text("// initial\n")
    _shell(["git", "add", "comp"], cwd=clone)
    _shell(["git", "commit", "-q", "-m", "init"], cwd=clone)
    _shell(["git", "remote", "add", "origin", str(origin)], cwd=clone)
    _shell(["git", "push", "-q", "-u", "origin", "q-branch-observer"], cwd=clone)

    # Create claude/observer-improvements off q-branch-observer and check it out.
    _shell(["git", "checkout", "-q", "-b", "claude/observer-improvements"], cwd=clone)

    return {"origin": origin, "clone": clone}


def _push_upstream_commit(origin: Path, tmp_path: Path, filename: str, content: str, msg: str):
    """Push a fresh commit to origin/q-branch-observer via a temp working clone."""
    tmp_clone = tmp_path / f"push-helper-{filename.replace('/', '_')}"
    _shell(["git", "clone", "-q", str(origin), str(tmp_clone)], cwd=tmp_path)
    _shell(["git", "config", "user.email", "u@u"], cwd=tmp_clone)
    _shell(["git", "config", "user.name", "u"], cwd=tmp_clone)
    _shell(["git", "checkout", "-q", "q-branch-observer"], cwd=tmp_clone)
    f = tmp_clone / filename
    f.parent.mkdir(parents=True, exist_ok=True)
    f.write_text(content)
    _shell(["git", "add", str(f)], cwd=tmp_clone)
    _shell(["git", "commit", "-q", "-m", msg], cwd=tmp_clone)
    _shell(["git", "push", "-q"], cwd=tmp_clone)


# --- tests --- --------------------------------------------------------------

def test_sync_noop_when_no_new_upstream_commits(repo_pair):
    clone = repo_pair["clone"]
    summary = git_ops.sync_from_upstream(clone)
    assert summary["fetched"] is True
    assert summary["ahead_count"] == 0
    assert summary["merged"] is False
    assert summary["conflict"] is False
    assert summary["error"] is None


def test_sync_pulls_new_upstream_commits(repo_pair, tmp_path):
    _push_upstream_commit(
        repo_pair["origin"], tmp_path,
        "comp/observer/y.go", "// new upstream\n", "add y.go upstream",
    )

    clone = repo_pair["clone"]
    pre_head = git_ops.head_sha(clone)

    summary = git_ops.sync_from_upstream(clone)
    assert summary["fetched"] is True
    assert summary["ahead_count"] == 1
    assert summary["merged"] is True
    assert summary["conflict"] is False
    assert git_ops.head_sha(clone) != pre_head
    assert (clone / "comp" / "observer" / "y.go").exists()


def test_sync_conflict_aborts_merge_and_restores_tree(repo_pair, tmp_path):
    clone = repo_pair["clone"]

    # Diverge: commit on claude/observer-improvements that changes x.go locally...
    (clone / "comp" / "observer" / "x.go").write_text("// coord side\n")
    _shell(["git", "add", "comp/observer/x.go"], cwd=clone)
    _shell(["git", "commit", "-q", "-m", "coord commit"], cwd=clone)

    # ...and a different change to the same line upstream.
    _push_upstream_commit(
        repo_pair["origin"], tmp_path,
        "comp/observer/x.go", "// upstream side\n", "upstream edit",
    )

    pre_head = git_ops.head_sha(clone)
    summary = git_ops.sync_from_upstream(clone)
    assert summary["fetched"] is True
    assert summary["ahead_count"] == 1
    assert summary["merged"] is False
    assert summary["conflict"] is True
    # Merge aborted → HEAD unchanged, no MERGE_HEAD lingering.
    assert git_ops.head_sha(clone) == pre_head
    assert not (clone / ".git" / "MERGE_HEAD").exists()
    # Local content preserved.
    assert (clone / "comp" / "observer" / "x.go").read_text() == "// coord side\n"


def test_sync_refuses_off_scratch_branch(repo_pair):
    clone = repo_pair["clone"]
    _shell(["git", "checkout", "-q", "q-branch-observer"], cwd=clone)
    with pytest.raises(git_ops.WrongBranchError):
        git_ops.sync_from_upstream(clone)
