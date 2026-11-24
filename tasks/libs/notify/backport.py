"""
Slack-driven backport decision for a merged PR, used by `dda inv github.ask-backport-owner`.

The PR owner gets a DM with one button per candidate release branch, an "Apply selection"
button and a "No backport needed" button. Clicks are received over Socket Mode, so no public
endpoint is needed. This module never applies labels: it returns the owner's decision so the
caller (the "Assess backport" workflow) can apply them with its own scoped token.
"""

from __future__ import annotations

import json
from collections.abc import Callable
from dataclasses import dataclass, field
from threading import Event

from tasks.libs.ciproviders.github_api import RELEASE_BRANCH_PATTERN
from tasks.libs.pipeline.notifications import DEFAULT_SLACK_CHANNEL

# Outcomes reported to the caller
ADD = "add"
DISMISS = "dismiss"
TIMEOUT = "timeout"
NO_RECIPIENT = "no_recipient"

# Button action ids
ADD_LABEL_PREFIX = "add_label:"
APPLY_ACTION = "apply"
DISMISS_ACTION = "dismiss"

CONFIDENCE_LEVELS = ("HIGH", "MEDIUM", "LOW")


def slack_escape(text: str) -> str:
    """Escape Slack control characters in free text (e.g. a PR title)."""
    return text.replace('&', '&amp;').replace('<', '&lt;').replace('>', '&gt;')


def valid_branches(branches: str) -> list[str]:
    """Keep only well-formed release branches (A.B.x) from a comma-separated list, without duplicates."""
    result = []
    for branch in (b.strip() for b in branches.split(',')):
        if RELEASE_BRANCH_PATTERN.match(branch) and branch not in result:
            result.append(branch)
    return result


def resolve_owner_email(pr) -> str | None:
    """
    Resolve the PR owner's email from their own commits on the PR.

    The last commit may have been pushed by someone else, so prefer the last commit authored by
    the PR owner, and fall back to the last commit's author.
    """
    commits = list(pr.get_commits())
    if not commits:
        return None
    owner = pr.user.login
    for commit in reversed(commits):
        if commit.author and commit.author.login == owner:
            return commit.commit.author.email
    return commits[-1].commit.author.email


@dataclass
class BackportRequest:
    """What the PR owner is asked about."""

    pr_number: int
    pr_title: str
    pr_url: str
    branches: list[str]
    explanation: str = ""
    confidence: str = "LOW"
    ttl: int = 7200

    @property
    def hours(self) -> int:
        return max(1, round(self.ttl / 3600))

    @property
    def pr_link(self) -> str:
        return f"<{self.pr_url}|#{self.pr_number}>"

    @property
    def fallback_text(self) -> str:
        return f":rewind: Backport assessment for #{self.pr_number}"

    def section_text(self) -> str:
        rationale = f"\n> {slack_escape(self.explanation)}\n" if self.explanation else "\n"
        return (
            f":rewind: :claude: thinks <{self.pr_url}|#{self.pr_number} ({slack_escape(self.pr_title)})> "
            f"may need a backport (confidence: {self.confidence}).\n{rationale}"
            "Pick one or more branches, then hit *Apply selection* — the labels are only applied once you do. "
            f"Buttons expire in about {self.hours} hour(s) — after that, add the backport/<branch> label manually."
        )

    def blocks(self, remaining: list[str] | None) -> list[dict]:
        """Message blocks; `remaining=None` drops the buttons (the exchange is over)."""
        blocks: list[dict] = [{"type": "section", "text": {"type": "mrkdwn", "text": self.section_text()}}]
        if remaining is None:
            return blocks
        elements: list[dict] = [
            {
                "type": "button",
                "style": "primary",
                # action_id must be unique per element, so append the branch
                "action_id": f"{ADD_LABEL_PREFIX}{branch}",
                "text": {"type": "plain_text", "text": branch},
                "value": branch,
            }
            for branch in remaining
        ]
        elements.append(
            {
                "type": "button",
                "action_id": APPLY_ACTION,
                "text": {"type": "plain_text", "text": "Apply selection"},
                "value": APPLY_ACTION,
            }
        )
        elements.append(
            {
                "type": "button",
                "style": "danger",
                "action_id": DISMISS_ACTION,
                "text": {"type": "plain_text", "text": "No backport needed"},
                "value": DISMISS_ACTION,
            }
        )
        blocks.append({"type": "actions", "elements": elements})
        return blocks


@dataclass
class BackportSelection:
    """
    State of the owner's choice. Labels are only reported for application once the owner
    explicitly clicks *Apply selection*: a timeout or *No backport needed* never applies anything.
    """

    request: BackportRequest
    chosen: list[str] = field(default_factory=list)
    outcome: str = TIMEOUT
    done: Event = field(default_factory=Event)

    def handle(
        self,
        action_id: str,
        value: str,
        set_buttons: Callable[[list[str] | None], None],
        reply: Callable[[str], None],
    ) -> None:
        if self.done.is_set():
            return
        if action_id == APPLY_ACTION:
            if not self.chosen:
                reply("No branch selected yet. Pick at least one, or choose *No backport needed*.")
                return
            self.outcome = ADD
            self.done.set()
        elif action_id == DISMISS_ACTION:
            self.chosen.clear()
            self.outcome = DISMISS
            self.done.set()
            set_buttons(None)
            reply(f":no_entry_sign: No backport for {self.request.pr_link}.")
        elif action_id.startswith(ADD_LABEL_PREFIX):
            if value not in self.request.branches or value in self.chosen:
                return
            self.chosen.append(value)
            set_buttons([b for b in self.request.branches if b not in self.chosen])
            reply(
                f":white_check_mark: Selected `{value}`. Keep picking branches, or hit *Apply selection*: "
                "nothing is applied until you do."
            )

    def finish(self, set_buttons: Callable[[list[str] | None], None], reply: Callable[[str], None]) -> list[str]:
        """Close the exchange and return the branches to label (empty unless the owner applied)."""
        self.done.set()
        if self.outcome == ADD:
            set_buttons(None)
            reply(f":ok: Choice recorded, applying the backport labels now (`{', '.join(self.chosen)}`).")
            return list(self.chosen)
        if self.outcome == TIMEOUT:
            set_buttons(None)
            unconfirmed = f" The selection (`{', '.join(self.chosen)}`) was not applied." if self.chosen else ""
            reply(
                f":alarm_clock: No confirmation within {self.request.hours} hour(s).{unconfirmed} "
                f"To backport {self.request.pr_link} manually, add a backport/<branch> label."
            )
        return []


def notify_devx(web, request: BackportRequest, problem: str) -> None:
    """Tell agent-devx-ops when the PR owner cannot be reached, so the decision is not silently dropped."""
    candidates = ', '.join(f'`{b}`' for b in request.branches)
    message = (
        f":warning: Backport assessment could not reach the owner of {request.pr_link} "
        f"(candidates: {candidates}). {problem}"
    )
    try:
        web.chat_postMessage(channel=DEFAULT_SLACK_CHANNEL, text=message)
    except Exception as e:
        print(f"Could not notify {DEFAULT_SLACK_CHANNEL} (non-fatal): {e}", flush=True)


def ask_backport_owner(pr, request: BackportRequest, bot_token: str, app_token: str) -> tuple[str, list[str]]:
    """
    DM the PR owner and wait up to `request.ttl` seconds for their choice.

    Returns the outcome (add | dismiss | timeout | no_recipient) and the branches to label.
    """
    from slack_sdk import WebClient
    from slack_sdk.errors import SlackApiError
    from slack_sdk.socket_mode import SocketModeClient
    from slack_sdk.socket_mode.response import SocketModeResponse

    web = WebClient(token=bot_token)

    try:
        email = resolve_owner_email(pr)
    except Exception as e:
        print(f"Could not resolve the PR owner's email: {e}", flush=True)
        notify_devx(web, request, f"Could not resolve the PR owner's email from their commits ({e}).")
        return NO_RECIPIENT, []
    try:
        if not email:
            raise ValueError("the PR has no commits")
        user_id = web.users_lookupByEmail(email=email)["user"]["id"]
    except (SlackApiError, ValueError) as e:
        # External contributor or no Slack account: nobody to DM
        print(f"No Slack user for {email}: {e}", flush=True)
        notify_devx(
            web,
            request,
            f"Could not DM the PR owner (no Slack user for {email or 'unknown email'}). "
            "Please assess the backport yourself and add the label if warranted.",
        )
        return NO_RECIPIENT, []

    msg = web.chat_postMessage(
        channel=web.conversations_open(users=[user_id])["channel"]["id"],
        text=request.fallback_text,
        blocks=request.blocks(request.branches),
    )
    print(f"DM sent to the PR owner (channel {msg['channel']}, ts {msg['ts']})", flush=True)

    def reply(text: str) -> None:
        try:
            web.chat_postMessage(channel=msg["channel"], thread_ts=msg["ts"], text=text)
        except SlackApiError as e:
            print(f"Could not post a thread reply (non-fatal): {e}", flush=True)

    def set_buttons(remaining: list[str] | None) -> None:
        try:
            web.chat_update(
                channel=msg["channel"], ts=msg["ts"], text=request.fallback_text, blocks=request.blocks(remaining)
            )
        except SlackApiError as e:
            print(f"Could not update the message (non-fatal): {e}", flush=True)

    selection = BackportSelection(request)

    def process(client, req) -> None:
        if req.type != "interactive":
            return
        # Recent slack_sdk versions already parse the payload into a dict
        payload = req.payload if isinstance(req.payload, dict) else json.loads(req.payload)
        if payload.get("type") != "block_actions":
            return
        client.send_socket_mode_response(SocketModeResponse(envelope_id=req.envelope_id))
        # Another concurrent listener of the same Slack app may receive this click: only act on ours.
        if payload.get("container", {}).get("message_ts") != msg["ts"]:
            return
        for action in payload.get("actions", []):
            # Never log the raw payload: it holds Slack user details and ends up in public CI logs
            print(f"Received action {action['action_id']}", flush=True)
            selection.handle(action["action_id"], action.get("value", ""), set_buttons, reply)

    socket = SocketModeClient(app_token=app_token, web_client=web)
    socket.socket_mode_request_listeners.append(process)
    try:
        socket.connect()
        print(f"Socket Mode connected: {socket.is_connected()}, waiting up to {request.ttl}s", flush=True)
        selection.done.wait(timeout=request.ttl)
    finally:
        socket.close()

    branches = selection.finish(set_buttons, reply)
    return selection.outcome, branches
