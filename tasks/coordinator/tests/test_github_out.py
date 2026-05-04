"""Tests for github_out (subprocess stubbed)."""

import subprocess
from types import SimpleNamespace
from unittest.mock import patch

from coordinator import github_out


def _fake(returncode=0, stdout="", stderr=""):
    return SimpleNamespace(returncode=returncode, stdout=stdout, stderr=stderr)


# --- configuration --- -----------------------------------------------------

def test_not_configured_when_env_missing(monkeypatch):
    monkeypatch.delenv(github_out.PR_NUMBER_ENV, raising=False)
    assert github_out.is_configured() is False
    ok, detail = github_out.post("phase_exit", "hi")
    assert ok is False
    assert detail == "not_configured"


def test_not_configured_when_env_empty(monkeypatch):
    monkeypatch.setenv(github_out.PR_NUMBER_ENV, "   ")
    assert github_out.is_configured() is False


def test_configured_when_env_set(monkeypatch):
    monkeypatch.setenv(github_out.PR_NUMBER_ENV, "49678")
    assert github_out.is_configured() is True


# --- formatting --- --------------------------------------------------------

def test_format_message_has_marker_and_emoji():
    s = github_out.format_message("phase_exit", "done", requires_ack=True)
    assert s.startswith(github_out.OWN_MESSAGE_MARKER)
    assert "🏁" in s
    assert "phase_exit" in s
    assert "[requires ack]" in s
    assert "done" in s


def test_format_message_unknown_type_uses_default_emoji():
    s = github_out.format_message("weird_type", "body", requires_ack=False)
    assert "🔔" in s
    assert "[requires ack]" not in s


# --- post via gh CLI --- ----------------------------------------------------

def test_post_happy_path(monkeypatch):
    monkeypatch.setenv(github_out.PR_NUMBER_ENV, "49678")
    captured = {}

    def fake_run(args, **kwargs):
        captured["args"] = args
        return _fake(returncode=0, stdout="")

    with patch("subprocess.run", fake_run):
        ok, detail = github_out.post("validation_completed", "hi")
    assert ok is True
    assert detail == "ok"
    assert captured["args"][0] == "gh"
    assert captured["args"][1:4] == ["pr", "comment", "49678"]
    # body is the last arg (--body <text>)
    body = captured["args"][-1]
    assert body.startswith(github_out.OWN_MESSAGE_MARKER)
    assert "hi" in body


def test_post_gh_error(monkeypatch):
    monkeypatch.setenv(github_out.PR_NUMBER_ENV, "49678")
    with patch("subprocess.run", lambda *a, **kw: _fake(returncode=1, stderr="rate limited")):
        ok, detail = github_out.post("phase_exit", "x")
    assert ok is False
    assert "gh_error" in detail
    assert "rate limited" in detail


def test_post_gh_missing(monkeypatch):
    monkeypatch.setenv(github_out.PR_NUMBER_ENV, "49678")

    def raise_fnf(*a, **kw):
        raise FileNotFoundError("gh not found")

    with patch("subprocess.run", raise_fnf):
        ok, detail = github_out.post("phase_exit", "x")
    assert ok is False
    assert detail == "gh_cli_missing"


def test_post_timeout(monkeypatch):
    monkeypatch.setenv(github_out.PR_NUMBER_ENV, "49678")

    def raise_to(*a, **kw):
        raise subprocess.TimeoutExpired(cmd="gh", timeout=15)

    with patch("subprocess.run", raise_to):
        ok, detail = github_out.post("phase_exit", "x")
    assert ok is False
    assert detail == "timeout"
