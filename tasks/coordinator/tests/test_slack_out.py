"""Tests for slack_out (no real HTTP calls)."""

import os
from unittest.mock import patch, MagicMock

import pytest

from coordinator import slack_out


def test_not_configured_when_env_missing(monkeypatch):
    monkeypatch.delenv(slack_out.WEBHOOK_ENV, raising=False)
    assert slack_out.is_configured() is False
    ok, detail = slack_out.post("budget_milestone", "hi")
    assert ok is False
    assert detail == "not_configured"


def test_not_configured_when_env_empty(monkeypatch):
    monkeypatch.setenv(slack_out.WEBHOOK_ENV, "   ")
    assert slack_out.is_configured() is False


def test_format_message_includes_type_and_emoji():
    s = slack_out.format_message("phase_exit", "done", requires_ack=True)
    assert ":checkered_flag:" in s
    assert "phase_exit" in s
    assert "[requires ack]" in s
    assert "done" in s


def test_format_message_unknown_type_uses_default_emoji():
    s = slack_out.format_message("unknown_custom_type", "body", requires_ack=False)
    assert ":bell:" in s
    assert "[requires ack]" not in s


def test_post_success(monkeypatch):
    monkeypatch.setenv(slack_out.WEBHOOK_ENV, "https://hooks.slack.com/fake")
    fake_requests = MagicMock()
    fake_resp = MagicMock(status_code=200, text="ok")
    fake_requests.post.return_value = fake_resp
    with patch.dict("sys.modules", {"requests": fake_requests}):
        ok, detail = slack_out.post("validation_completed", "val-1 done", requires_ack=False)
    assert ok is True
    assert detail == "ok"
    # Verify we sent JSON to the configured URL
    fake_requests.post.assert_called_once()
    args, kwargs = fake_requests.post.call_args
    assert args[0] == "https://hooks.slack.com/fake"
    assert "val-1 done" in kwargs["data"]


def test_post_http_error(monkeypatch):
    monkeypatch.setenv(slack_out.WEBHOOK_ENV, "https://hooks.slack.com/fake")
    fake_requests = MagicMock()
    fake_resp = MagicMock(status_code=500, text="server error")
    fake_requests.post.return_value = fake_resp
    with patch.dict("sys.modules", {"requests": fake_requests}):
        ok, detail = slack_out.post("phase_exit", "oops")
    assert ok is False
    assert "http_500" in detail


def test_post_exception_caught(monkeypatch):
    monkeypatch.setenv(slack_out.WEBHOOK_ENV, "https://hooks.slack.com/fake")
    fake_requests = MagicMock()
    fake_requests.post.side_effect = ConnectionError("network down")
    with patch.dict("sys.modules", {"requests": fake_requests}):
        ok, detail = slack_out.post("phase_exit", "oops")
    assert ok is False
    assert "ConnectionError" in detail
