"""Tests for github_in (subprocess stubbed)."""

import json
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from coordinator import github_in, github_out
from coordinator.db import state_dir
from coordinator.inbox import INBOX_NAME


def _fake(returncode=0, stdout="", stderr=""):
    return SimpleNamespace(returncode=returncode, stdout=stdout, stderr=stderr)


def _configured(monkeypatch):
    monkeypatch.setenv(github_out.PR_NUMBER_ENV, "49678")


def _comments_ndjson(comments: list[dict]) -> str:
    return "\n".join(json.dumps(c) for c in comments)


def _inbox_text(root: Path) -> str:
    p = state_dir(root) / INBOX_NAME
    return p.read_text() if p.exists() else ""


# --- configuration --- -----------------------------------------------------

def test_not_configured_when_env_missing(tmp_path: Path, monkeypatch):
    monkeypatch.delenv(github_out.PR_NUMBER_ENV, raising=False)
    count, detail = github_in.poll(tmp_path)
    assert count == 0
    assert detail == "not_configured"


# --- happy path --- --------------------------------------------------------

def test_poll_appends_user_comments_filters_own(tmp_path: Path, monkeypatch):
    _configured(monkeypatch)
    state_dir(tmp_path).mkdir(parents=True)

    own_body = github_out.format_message("phase_exit", "auto-halt", requires_ack=True)
    comments = [
        {"id": 1, "body": "first user msg"},
        {"id": 2, "body": own_body},
        {"id": 3, "body": "second user msg"},
    ]
    with patch("subprocess.run", lambda *a, **kw: _fake(stdout=_comments_ndjson(comments))):
        count, detail = github_in.poll(tmp_path)
    assert count == 2
    body = _inbox_text(tmp_path)
    assert body.index("first user msg") < body.index("second user msg")
    state = json.loads((state_dir(tmp_path) / github_in.STATE_FILENAME).read_text())
    assert state["last_seen_id"] == 3


def test_repoll_does_not_duplicate(tmp_path: Path, monkeypatch):
    _configured(monkeypatch)
    state_dir(tmp_path).mkdir(parents=True)
    comments = [{"id": 5, "body": "hello"}]
    with patch("subprocess.run", lambda *a, **kw: _fake(stdout=_comments_ndjson(comments))):
        github_in.poll(tmp_path)
    # Second poll sees the same comment but it's below last_seen_id now.
    with patch("subprocess.run", lambda *a, **kw: _fake(stdout=_comments_ndjson(comments))):
        count, _ = github_in.poll(tmp_path)
    assert count == 0
    assert _inbox_text(tmp_path).count("hello") == 1


def test_own_comments_advance_state_without_appending(tmp_path: Path, monkeypatch):
    _configured(monkeypatch)
    state_dir(tmp_path).mkdir(parents=True)
    own_body = github_out.format_message("phase_exit", "x", requires_ack=False)
    comments = [{"id": 7, "body": own_body}]
    with patch("subprocess.run", lambda *a, **kw: _fake(stdout=_comments_ndjson(comments))):
        count, _ = github_in.poll(tmp_path)
    assert count == 0
    state = json.loads((state_dir(tmp_path) / github_in.STATE_FILENAME).read_text())
    assert state["last_seen_id"] == 7


def test_comments_sorted_chronologically(tmp_path: Path, monkeypatch):
    """gh api returns arbitrary order; poll must sort by id before appending."""
    _configured(monkeypatch)
    state_dir(tmp_path).mkdir(parents=True)
    comments = [
        {"id": 10, "body": "three"},
        {"id": 8, "body": "one"},
        {"id": 9, "body": "two"},
    ]
    with patch("subprocess.run", lambda *a, **kw: _fake(stdout=_comments_ndjson(comments))):
        count, _ = github_in.poll(tmp_path)
    assert count == 3
    body = _inbox_text(tmp_path)
    assert body.index("one") < body.index("two") < body.index("three")


# --- failure modes --- ----------------------------------------------------

def test_gh_missing_fails_soft(tmp_path: Path, monkeypatch):
    _configured(monkeypatch)
    state_dir(tmp_path).mkdir(parents=True)

    def raise_fnf(*a, **kw):
        raise FileNotFoundError("gh")

    with patch("subprocess.run", raise_fnf):
        count, detail = github_in.poll(tmp_path)
    assert count == 0
    assert detail == "gh_cli_missing"


def test_gh_error_fails_soft(tmp_path: Path, monkeypatch):
    _configured(monkeypatch)
    state_dir(tmp_path).mkdir(parents=True)
    with patch("subprocess.run", lambda *a, **kw: _fake(returncode=1, stderr="HTTP 403")):
        count, detail = github_in.poll(tmp_path)
    assert count == 0
    assert "gh_error" in detail
    assert _inbox_text(tmp_path) == ""


def test_malformed_json_line_ignored(tmp_path: Path, monkeypatch):
    _configured(monkeypatch)
    state_dir(tmp_path).mkdir(parents=True)
    # Third line is junk; the first two are valid.
    stdout = "\n".join(
        [
            json.dumps({"id": 1, "body": "ok"}),
            "",
            "not-json",
            json.dumps({"id": 2, "body": "also-ok"}),
        ]
    )
    with patch("subprocess.run", lambda *a, **kw: _fake(stdout=stdout)):
        count, _ = github_in.poll(tmp_path)
    assert count == 2
    assert "ok" in _inbox_text(tmp_path)
    assert "also-ok" in _inbox_text(tmp_path)
