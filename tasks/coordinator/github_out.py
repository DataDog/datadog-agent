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

# Process-lifetime counters for gh health. If auth expires on day 2 the
# channel goes dark silently; driver reads these and escalates. Counters
# reset on any successful call; a driver restart re-checks from zero.
_CONSECUTIVE_ERRORS = {"post": 0, "poll": 0}
GH_WARN_THRESHOLD = 3   # emit coord-out warning, keep going
GH_HALT_THRESHOLD = 5   # user channel is dead, halt the loop


def record_gh_result(channel: str, ok: bool) -> int:
    """Update consecutive-error counter for channel ("post" or "poll")."""
    if ok:
        _CONSECUTIVE_ERRORS[channel] = 0
    else:
        _CONSECUTIVE_ERRORS[channel] = _CONSECUTIVE_ERRORS.get(channel, 0) + 1
    return _CONSECUTIVE_ERRORS[channel]


def gh_consecutive_errors() -> dict[str, int]:
    return dict(_CONSECUTIVE_ERRORS)


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
    "iter_start": "▶️",
    "iter_shipped": "✅",
    "iter_rejected": "❌",
    "iter_eval_failed": "💥",
    "iter_impl_failed": "💥",
}


def pr_number() -> Optional[str]:
    """Return the configured run-log PR number, or None if unset."""
    v = os.environ.get(PR_NUMBER_ENV, "").strip()
    return v or None


def is_configured() -> bool:
    return pr_number() is not None


def _signature() -> str:
    """Build a compact signature line appended to every coord-out comment.

    Makes clear the comment is from the coordinator (not a human) and
    gives the reader enough breadcrumbs to find the context: git sha of
    the harness code that posted it, and timestamp.
    """
    import datetime as _dt
    import subprocess
    sha = ""
    try:
        r = subprocess.run(
            ["git", "rev-parse", "--short", "HEAD"],
            capture_output=True, text=True, timeout=5,
        )
        if r.returncode == 0:
            sha = r.stdout.strip()
    except (FileNotFoundError, subprocess.TimeoutExpired, Exception):
        pass
    ts = _dt.datetime.now().isoformat(timespec="seconds")
    parts = ["— Claude (coordinator harness)"]
    if sha:
        parts.append(f"`{sha}`")
    parts.append(ts)
    return " · ".join(parts)


def format_message(msg_type: str, content: str, requires_ack: bool) -> str:
    emoji = _EMOJI.get(msg_type, "🔔")
    ack_tag = "  **[requires ack]**" if requires_ack else ""
    return (
        f"{OWN_MESSAGE_MARKER}{emoji}  `{msg_type}`{ack_tag}\n\n"
        f"{content.rstrip()}\n\n"
        f"<sub>{_signature()}</sub>"
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
        record_gh_result("post", False)
        return False, "gh_cli_missing"
    except subprocess.TimeoutExpired:
        record_gh_result("post", False)
        return False, "timeout"
    except Exception as e:
        record_gh_result("post", False)
        return False, f"exception: {e.__class__.__name__}"

    if r.returncode != 0:
        record_gh_result("post", False)
        return False, f"gh_error: {(r.stderr or r.stdout).strip()[:300]}"
    record_gh_result("post", True)
    return True, "ok"
