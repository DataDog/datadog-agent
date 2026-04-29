"""ensure_scratch_branch roots the new branch at origin/q-branch-observer,
not at whatever HEAD the operator happened to be on.
"""

import subprocess
from pathlib import Path

import pytest

from coordinator import git_ops


def _shell(args, cwd, check=True):
    return subprocess.run(args, cwd=cwd, check=check, capture_output=True, text=True)


@pytest.fixture
def repo_with_upstream(tmp_path: Path):
    origin = tmp_path / "origin.git"
    clone = tmp_path / "clone"
    _shell(["git", "init", "--bare", "-b", "q-branch-observer", str(origin)], cwd=tmp_path)

    _shell(["git", "init", "-q", "-b", "q-branch-observer", str(clone)], cwd=tmp_path)
    _shell(["git", "config", "user.email", "t@t"], cwd=clone)
    _shell(["git", "config", "user.name", "t"], cwd=clone)
    (clone / "upstream_file.txt").write_text("upstream\n")
    _shell(["git", "add", "upstream_file.txt"], cwd=clone)
    _shell(["git", "commit", "-q", "-m", "upstream init"], cwd=clone)
    _shell(["git", "remote", "add", "origin", str(origin)], cwd=clone)
    _shell(["git", "push", "-q", "-u", "origin", "q-branch-observer"], cwd=clone)
    return clone


def test_creates_branch_at_upstream_even_when_on_a_different_branch(repo_with_upstream):
    clone = repo_with_upstream
    # Simulate "user starts the coordinator from an unrelated branch"
    _shell(["git", "checkout", "-q", "-b", "some-random-feature"], cwd=clone)
    (clone / "random_feature.txt").write_text("unrelated\n")
    _shell(["git", "add", "random_feature.txt"], cwd=clone)
    _shell(["git", "commit", "-q", "-m", "random work"], cwd=clone)

    git_ops.ensure_scratch_branch(clone)

    # We should be on claude/observer-improvements, rooted at upstream tip,
    # with NO trace of the random feature commit.
    assert git_ops.current_branch(clone) == git_ops.SCRATCH_BRANCH
    assert (clone / "upstream_file.txt").exists()
    assert not (clone / "random_feature.txt").exists()


def test_checkout_existing_scratch_branch_unchanged(repo_with_upstream):
    clone = repo_with_upstream
    # First call creates the branch.
    git_ops.ensure_scratch_branch(clone)
    # Commit something on it so we can verify nothing gets wiped.
    (clone / "scratch_work.txt").write_text("scratch\n")
    _shell(["git", "add", "scratch_work.txt"], cwd=clone)
    _shell(["git", "commit", "-q", "-m", "scratch commit"], cwd=clone)
    scratch_sha = git_ops.head_sha(clone)

    # Switch off, then back — existing scratch branch must be preserved.
    _shell(["git", "checkout", "-q", "q-branch-observer"], cwd=clone)
    git_ops.ensure_scratch_branch(clone)
    assert git_ops.current_branch(clone) == git_ops.SCRATCH_BRANCH
    assert git_ops.head_sha(clone) == scratch_sha
    assert (clone / "scratch_work.txt").exists()
