"""Outbound Slack notifications via Incoming Webhook.

Simplest auth path: user creates an Incoming Webhook in Slack once and
exports `COORD_SLACK_WEBHOOK_URL` in the coordinator's environment. Each
`coord_out.emit` call also POSTs a short message to the webhook.

Design choices:
  - Webhook (not bot token) — no OAuth, no token refresh, no MCP session.
  - Fail-soft — any error (no URL, HTTP failure, timeout) is journaled and
    swallowed so coord-out.md remains the source of truth.
  - Plain text, no Block Kit — each message is a ~3 line post with a
    type-tagged title and the raw content body. Keeps the Python side
    dependency-light (just `requests`).
  - Per-type emoji prefix helps Slack glance-parse event streams.

Upgrade path (if bidirectional wanted later):
  - Add `slack_inbox.py` that polls a bot DM via `slack_sdk`.
  - Appends messages into `.coordinator/inbox.md` on arrival.
  - Bot token stored in `COORD_SLACK_BOT_TOKEN` env var.
"""

from __future__ import annotations

import json
import os
from typing import Optional


WEBHOOK_ENV = "COORD_SLACK_WEBHOOK_URL"

_EMOJI = {
    "budget_milestone": ":money_with_wings:",
    "phase_exit": ":checkered_flag:",
    "strict_regression": ":rotating_light:",
    "tripwire": ":warning:",
    "validation_completed": ":white_check_mark:",
    "validation_dispatched": ":rocket:",
    "coordinator_startup": ":wave:",
}


def _webhook_url() -> Optional[str]:
    url = os.environ.get(WEBHOOK_ENV, "").strip()
    return url or None


def is_configured() -> bool:
    return _webhook_url() is not None


def format_message(msg_type: str, content: str, requires_ack: bool) -> str:
    emoji = _EMOJI.get(msg_type, ":bell:")
    ack_tag = "  *[requires ack]*" if requires_ack else ""
    return f"{emoji}  `{msg_type}`{ack_tag}\n{content.rstrip()}"


def post(msg_type: str, content: str, requires_ack: bool = False) -> tuple[bool, str]:
    """POST the message to the configured webhook.

    Returns (ok, detail). (False, "not_configured") if no webhook URL.
    Never raises.
    """
    url = _webhook_url()
    if not url:
        return False, "not_configured"

    try:
        import requests  # lazy; optional dep
    except ImportError:
        return False, "requests_not_installed"

    payload = {"text": format_message(msg_type, content, requires_ack)}
    try:
        resp = requests.post(
            url,
            data=json.dumps(payload),
            headers={"Content-Type": "application/json"},
            timeout=5,
        )
    except Exception as e:
        return False, f"exception: {e.__class__.__name__}"

    if 200 <= resp.status_code < 300:
        return True, "ok"
    return False, f"http_{resp.status_code}: {resp.text[:200]}"
