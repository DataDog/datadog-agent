"""Outbound GitHub PR comments — the coordinator's user-facing channel.

Posts each coord-out event as a comment on the long-lived run-log PR
(see ~/.claude/plans/ad-harness-plan.md §10 and PR #49678). GitHub
mobile/desktop/web render the feed; user replies with PR comments;
`github_in.poll` polls and routes replies into `inbox.md`.

Auth: relies on the `gh` CLI being authenticated on the driver workspace
(typically already true via DD's default workspace provisioning). No
tokens managed here.

Env vars:
  COORD_GITHUB_PR_NUMBER   PR number for the run-log PR (e.g. 49678).
                           Unset → no-op, `post()` returns (False, "not_configured").

Fail-soft: every failure (missing CLI, timeout, API error) is swallowed
and returned as `(False, detail)`. `coord-out.md` remains the canonical
channel; GitHub is additive.
"""

from __future__ import annotations

import os
import subprocess
from typing import Optional

PR_NUMBER_ENV = "COORD_GITHUB_PR_NUMBER"

# Zero-width space prepended to every coordinator-posted comment so
# `github_in.poll` can cheaply tell "this is us" from "this is a user
# reply" — the `gh` CLI posts as the identity running the command, which
# on the driver workspace is the same human account as the operator.
OWN_MESSAGE_MARKER = "\u200b"

_EMOJI = {
    "budget_milestone": "💸",
    "phase_exit": "🏁",
    "strict_regression": "🚨",
    "tripwire": "⚠️",
    "validation_completed": "✅",
    "validation_dispatched": "🚀",
    "validation_abandoned": "⚠️",
    "coordinator_startup": "👋",
    "upstream_conflict": "🚨",
    "inbox_ack": "📥",
}


def pr_number() -> Optional[str]:
    """Return the configured run-log PR number, or None if unset."""
    v = os.environ.get(PR_NUMBER_ENV, "").strip()
    return v or None


def is_configured() -> bool:
    return pr_number() is not None


def format_message(msg_type: str, content: str, requires_ack: bool) -> str:
    emoji = _EMOJI.get(msg_type, "🔔")
    ack_tag = "  **[requires ack]**" if requires_ack else ""
    return (
        f"{OWN_MESSAGE_MARKER}{emoji}  `{msg_type}`{ack_tag}\n\n"
        f"{content.rstrip()}"
    )


def post(msg_type: str, content: str, requires_ack: bool = False) -> tuple[bool, str]:
    """Post a comment on the run-log PR. Returns (ok, detail). Never raises."""
    pr = pr_number()
    if not pr:
        return False, "not_configured"

    body = format_message(msg_type, content, requires_ack)
    try:
        r = subprocess.run(
            ["gh", "pr", "comment", pr, "--body", body],
            capture_output=True,
            text=True,
            timeout=15,
        )
    except FileNotFoundError:
        return False, "gh_cli_missing"
    except subprocess.TimeoutExpired:
        return False, "timeout"
    except Exception as e:
        return False, f"exception: {e.__class__.__name__}"

    if r.returncode != 0:
        return False, f"gh_error: {(r.stderr or r.stdout).strip()[:300]}"
    return True, "ok"
