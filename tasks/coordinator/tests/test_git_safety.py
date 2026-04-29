"""Git-safety invariants: no pushes, no off-branch commits, no stray git calls."""

import subprocess
from pathlib import Path

import pytest

from coordinator.git_ops import (
    SCRATCH_BRANCH,
    WrongBranchError,
    assert_on_scratch_branch,
    commit_candidate,
    ensure_scratch_branch,
    is_clean,
)
from coordinator.sdk import is_git_command


def _init_repo(tmp_path: Path) -> Path:
    subprocess.run(["git", "init", "-q"], cwd=tmp_path, check=True)
    subprocess.run(["git", "config", "user.email", "t@t"], cwd=tmp_path, check=True)
    subprocess.run(["git", "config", "user.name", "t"], cwd=tmp_path, check=True)
    (tmp_path / "seed").write_text("x\n")
    subprocess.run(["git", "add", "seed"], cwd=tmp_path, check=True)
    subprocess.run(["git", "commit", "-q", "-m", "init"], cwd=tmp_path, check=True)
    return tmp_path


# --- branch-guard --- -------------------------------------------------------

def test_commit_refused_off_scratch_branch(tmp_path: Path):
    repo = _init_repo(tmp_path)
    # Stay on default branch (main/master), NOT claude/observer-improvements
    (repo / "comp").mkdir()
    (repo / "comp" / "observer").mkdir()
    (repo / "comp" / "observer" / "x.go").write_text("// test\n")

    with pytest.raises(WrongBranchError):
        commit_candidate("cand-x", "exp-0", root=repo, paths=["comp/observer"])


def test_assert_on_scratch_branch_happy_path(tmp_path: Path):
    repo = _init_repo(tmp_path)
    ensure_scratch_branch(repo)
    # No exception
    assert_on_scratch_branch(repo)


def test_commit_on_scratch_branch_works(tmp_path: Path):
    repo = _init_repo(tmp_path)
    ensure_scratch_branch(repo)
    (repo / "comp").mkdir()
    (repo / "comp" / "observer").mkdir()
    (repo / "comp" / "observer" / "x.go").write_text("// test\n")
    sha = commit_candidate("cand-x", "exp-0", root=repo, paths=["comp/observer"])
    assert len(sha) == 40


# --- is_git_command --- -----------------------------------------------------

@pytest.mark.parametrize(
    "cmd, expected",
    [
        ("git status", True),
        ("  git push origin main", True),
        ("git log | head -5", True),
        ("git checkout -- .", True),
        ("echo foo", False),
        ("ls -la git", False),  # "git" as a filename arg, not command
        ("gitk", False),  # different binary
        ("git-foo", False),  # different binary
        ("true && git push", True),
        ("cd foo; git push", True),
        ("", False),
    ],
)
def test_is_git_command(cmd: str, expected: bool):
    assert is_git_command(cmd) is expected


# --- is_clean --- -----------------------------------------------------------

def test_is_clean_after_init(tmp_path: Path):
    repo = _init_repo(tmp_path)
    (repo / "comp").mkdir()
    (repo / "comp" / "observer").mkdir()
    # No changes under WATCH_PATHS yet
    assert is_clean(repo) is True


def test_is_clean_detects_dirty(tmp_path: Path):
    repo = _init_repo(tmp_path)
    (repo / "comp").mkdir()
    (repo / "comp" / "observer").mkdir()
    (repo / "comp" / "observer" / "x.go").write_text("// test\n")
    assert is_clean(repo) is False
