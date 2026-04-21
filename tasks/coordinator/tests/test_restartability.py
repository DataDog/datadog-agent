"""Tests for restart-safety mechanisms.

Covers:
  - sdk._is_transient classifier
  - sdk._with_retries behaviour (transient retries, non-transient propagates,
    attempt cap enforced)
  - git_ops.startup_cleanup orphan-revert behaviour
  - workspace_validate abandons stale pending validations
"""

from __future__ import annotations

import datetime as _dt
import subprocess
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import pytest

from coordinator import git_ops, sdk, workspace_validate as wv
from coordinator.config import CONFIG
from coordinator.db import empty_db, state_dir
from coordinator.schema import PendingValidation


# --- sdk retries --- -------------------------------------------------------

def test_is_transient_recognises_rate_limit():
    e = Exception("rate_limit_error from API")
    assert sdk._is_transient(e) is True


def test_is_transient_recognises_5xx():
    e = Exception("503 service unavailable")
    assert sdk._is_transient(e) is True


def test_is_transient_rejects_programmer_errors():
    e = ValueError("bad prompt key")
    assert sdk._is_transient(e) is False
    e2 = KeyError("missing_field")
    assert sdk._is_transient(e2) is False


def test_with_retries_passes_through_success():
    calls = []

    def f():
        calls.append(1)
        return "ok"

    assert sdk._with_retries(f) == "ok"
    assert len(calls) == 1


def test_with_retries_succeeds_after_transient():
    attempts = []

    def f():
        attempts.append(1)
        if len(attempts) < 2:
            raise RuntimeError("503 service unavailable")
        return "eventual"

    with patch("coordinator.sdk.time.sleep"):
        assert sdk._with_retries(f) == "eventual"
    assert len(attempts) == 2


def test_with_retries_respects_attempt_cap():
    attempts = []

    def f():
        attempts.append(1)
        raise RuntimeError("rate limit hit")

    with patch("coordinator.sdk.time.sleep"):
        with pytest.raises(RuntimeError):
            sdk._with_retries(f)
    # Exactly CONFIG.sdk_retry_max_attempts tries.
    assert len(attempts) == CONFIG.sdk_retry_max_attempts


def test_with_retries_propagates_non_transient():
    attempts = []

    def f():
        attempts.append(1)
        raise ValueError("malformed prompt")

    with pytest.raises(ValueError):
        sdk._with_retries(f)
    assert len(attempts) == 1  # no retries


# --- git_ops startup_cleanup --- -------------------------------------------

def _init_repo(tmp_path: Path) -> Path:
    subprocess.run(["git", "init", "-q", "-b", "main"], cwd=tmp_path, check=True)
    subprocess.run(["git", "config", "user.email", "t@t"], cwd=tmp_path, check=True)
    subprocess.run(["git", "config", "user.name", "t"], cwd=tmp_path, check=True)
    (tmp_path / "seed").write_text("x\n")
    subprocess.run(["git", "add", "seed"], cwd=tmp_path, check=True)
    subprocess.run(["git", "commit", "-q", "-m", "init"], cwd=tmp_path, check=True)
    return tmp_path


def test_startup_cleanup_reverts_orphan_working_tree(tmp_path: Path):
    repo = _init_repo(tmp_path)
    (repo / "comp" / "observer").mkdir(parents=True)
    (repo / "comp" / "observer" / "x.go").write_text("// committed\n")
    subprocess.run(["git", "add", "comp"], cwd=repo, check=True)
    subprocess.run(["git", "commit", "-q", "-m", "seed observer"], cwd=repo, check=True)
    git_ops.ensure_scratch_branch(repo)

    # Simulate a mid-iteration crash: an uncommitted edit is on disk.
    (repo / "comp" / "observer" / "x.go").write_text("// CHANGED mid-iter\n")
    assert not git_ops.is_clean(repo)

    summary = git_ops.startup_cleanup(repo)
    assert summary["reverted_dirty_tree"] is True
    assert git_ops.is_clean(repo)
    # File content restored to the committed version.
    assert (repo / "comp" / "observer" / "x.go").read_text() == "// committed\n"


def test_startup_cleanup_clean_tree_is_noop(tmp_path: Path):
    repo = _init_repo(tmp_path)
    git_ops.ensure_scratch_branch(repo)
    summary = git_ops.startup_cleanup(repo)
    assert summary["reverted_dirty_tree"] is False
    assert summary["pushed_orphan_commits"] == 0


def test_unpushed_commit_count_zero_when_no_remote(tmp_path: Path):
    repo = _init_repo(tmp_path)
    git_ops.ensure_scratch_branch(repo)
    # No remote configured.
    assert git_ops.unpushed_commit_count(repo) == 0


# --- workspace_validate abandons stale --- --------------------------------

def test_poll_abandons_stale_pending_validations(tmp_path: Path, monkeypatch):
    state_dir(tmp_path).mkdir(parents=True)
    db = empty_db()
    stale_ts = (_dt.datetime.now() - _dt.timedelta(hours=CONFIG.validation_max_age_hours + 5)).isoformat(timespec="seconds")
    db.validations["v-stale"] = PendingValidation(
        id="v-stale",
        experiment_id="exp-1",
        candidate_id="c",
        detector="scanmw",
        workspace="workspace-evals-scanmw",
        remote_output_dir="/tmp/x",
        dispatched_at=stale_ts,
        status="pending",
    )

    # Should abandon WITHOUT ever calling ssh.
    def must_not_be_called(*a, **kw):
        raise AssertionError("ssh should not be invoked on a stale validation")

    monkeypatch.setattr(wv, "_check_remote_done", must_not_be_called)
    wv.poll_pending_validations(db, tmp_path)
    assert db.validations["v-stale"].status == "abandoned"
    assert db.validations["v-stale"].completed_at is not None


def test_poll_leaves_fresh_pending_alone(tmp_path: Path, monkeypatch):
    state_dir(tmp_path).mkdir(parents=True)
    db = empty_db()
    db.validations["v-fresh"] = PendingValidation(
        id="v-fresh",
        experiment_id="exp-1",
        candidate_id="c",
        detector="scanmw",
        workspace="workspace-evals-scanmw",
        remote_output_dir="/tmp/x",
        dispatched_at=_dt.datetime.now().isoformat(timespec="seconds"),
        status="pending",
    )
    # Pretend remote isn't done yet.
    monkeypatch.setattr(wv, "_check_remote_done", lambda pv: False)
    wv.poll_pending_validations(db, tmp_path)
    assert db.validations["v-fresh"].status == "pending"
