#!/usr/bin/env python3
"""
Used by the "Assess backport" workflow (.github/workflows/assess-backport.yml).

Sends the PR owner a Slack DM with one "backport" button per candidate release
branch (as assessed by Claude), then listens over Socket Mode for up to
SLACK_BUTTON_TTL_SECONDS for a click — no public endpoint needed. Button clicks
are applied back in the workflow with a dd-octo-sts token, never with a token
held by this script.

Requires a Slack app with Socket Mode interactivity enabled and the bot scopes
`chat:write`, `im:write`, `users:read.email`, plus an app-level token with
`connections:write`. See the workflow file for the expected secrets.

Inputs (environment):
  SLACK_BOT_TOKEN        xoxb-... bot token of the Slack app above
  SLACK_APP_TOKEN        xapp-... app-level token (connections:write)
  GITHUB_TOKEN           read-only, to resolve the PR owner's email from their commits
  PR_NUMBER, PR_TITLE, PR_URL
  BRANCHES               comma-separated candidate branches, e.g. "7.85.x,7.86.x"
  EXPLANATION            Claude's one-line rationale (may be empty)
  CONFIDENCE             HIGH|MEDIUM|LOW
  SLACK_BUTTON_TTL_SECONDS  how long to wait for a click (default 7200)

Outputs (GITHUB_OUTPUT):
  outcome                add | dismiss | timeout | no_recipient
  branch                 the branch chosen when outcome is "add"
"""

import json
import logging
import os
import sys
from threading import Event

import requests
from slack_sdk import WebClient
from slack_sdk.socket_mode import SocketModeClient
from slack_sdk.socket_mode.request import SocketModeRequest
from slack_sdk.socket_mode.response import SocketModeResponse


def github_get(token: str, url: str) -> dict:
    resp = requests.get(
        url,
        headers={"Authorization": f"Bearer {token}", "Accept": "application/vnd.github+json"},
        timeout=30,
    )
    resp.raise_for_status()
    return resp.json()


def main() -> int:
    def env(name: str, default: str = "") -> str:
        return os.environ.get(name, default)

    pr_number = env("PR_NUMBER", "").strip()
    branches = [b.strip() for b in env("BRANCHES").split(",") if b.strip()]
    explanation = env("EXPLANATION").strip()
    confidence = env("CONFIDENCE", "LOW").strip()
    ttl = int(env("SLACK_BUTTON_TTL_SECONDS", "7200"))
    if not pr_number or not branches:
        print("Missing PR number or branches — nothing to ask about")
        return 0

    def output(key: str, value: str) -> None:
        with open(os.environ["GITHUB_OUTPUT"], "a") as f:
            f.write(f"{key}={value}\n")

    web = WebClient(token=env("SLACK_BOT_TOKEN"))

    # Resolve the PR owner's email from the original PR's commits (the last commit's
    # author is the PR owner, even when the commit results from a suggested change).
    try:
        commits = github_get(
            env("GITHUB_TOKEN"),
            f"https://api.github.com/repos/{env('GITHUB_REPOSITORY')}/pulls/{pr_number}/commits",
        )
        owner_email = commits[-1]["commit"]["author"]["email"]
    except Exception as e:  # noqa: BLE001
        print(f"Could not resolve the PR owner's email: {e}")
        output("outcome", "no_recipient")
        return 0

    try:
        user_id = web.users_lookupByEmail(email=owner_email)["user"]["id"]
    except Exception as e:  # noqa: BLE001
        # External contributor or no Slack account: nobody to ask, skip silently.
        print(f"No Slack user for {owner_email}: {e}")
        output("outcome", "no_recipient")
        return 0

    hours = max(1, round(ttl / 3600))
    rationale = f"\n> {explanation}\n" if explanation else "\n"
    buttons = [
        {
            "type": "button",
            "style": "primary",
            # action_id must be unique per element, so append the branch
            "action_id": f"add_label:{b}",
            "text": {"type": "plain_text", "text": b},
            "value": b,
        }
        for b in branches
    ]
    buttons.append(
        {
            "type": "button",
            "style": "danger",
            "action_id": "dismiss",
            "text": {"type": "plain_text", "text": "No backport needed"},
            "value": "dismiss",
        }
    )
    msg = web.chat_postMessage(
        channel=web.conversations_open(users=[user_id])["channel"]["id"],
        text=f":rewind: Backport assessment for #{pr_number}",
        blocks=[
            {
                "type": "section",
                "text": {
                    "type": "mrkdwn",
                    "text": (
                        f":rewind: Claude thinks <{env('PR_URL')}|#{pr_number} "
                        f"({env('PR_TITLE')})> may need a backport "
                        f"(confidence: {confidence}).\n{rationale}"
                        f"Buttons expire in about {hours} hour(s) — after that, add the "
                        "backport/<branch> label manually."
                    ),
                },
            },
            {"type": "actions", "elements": buttons},
        ],
    )
    print(f"Sent: {msg['permalink']}", flush=True)

    # Listen for the click over Socket Mode (no public endpoint required).
    logging.basicConfig(level=logging.INFO, stream=sys.stdout)
    sm = SocketModeClient(app_token=env("SLACK_APP_TOKEN"), web_client=web)
    clicked = Event()
    result = {"outcome": "timeout", "branch": ""}
    ts = msg["ts"]

    def process(client: SocketModeClient, req: SocketModeRequest) -> None:
        print(f"[debug] socket event: type={req.type} payload={req.payload}", flush=True)
        if req.type != "interactive":
            return
        # Newer slack_sdk versions parse the payload into a dict for us
        payload = req.payload if isinstance(req.payload, dict) else json.loads(req.payload)
        if payload.get("type") != "block_actions":
            return
        client.send_socket_mode_response(SocketModeResponse(envelope_id=req.envelope_id))
        # Another concurrent listener may receive this click — only act on ours.
        if payload.get("container", {}).get("message_ts") != ts:
            return
        for action in payload.get("actions", []):
            if action["action_id"] == "dismiss":
                result["outcome"] = "dismiss"
            elif action["action_id"].startswith("add_label:"):
                result["outcome"] = "add"
                result["branch"] = action["value"]
            clicked.set()  # stop waiting as soon as a button on our message is clicked

    sm.socket_mode_request_listeners.append(process)
    sm.connect()
    print(f"Socket Mode connected: {sm.is_connected()}", flush=True)
    clicked.wait(timeout=ttl)

    # Always remove the buttons so stale clicks can't pile up after expiry/choice.
    try:
        web.chat_update(
            channel=msg["channel"],
            ts=ts,
            text=f":rewind: Backport assessment for #{pr_number}",
            blocks=[
                {
                    "type": "section",
                    "text": {
                        "type": "mrkdwn",
                        "text": (
                            f":white_check_mark: Backport to `{result['branch']}` requested — "
                            "the label is being added, the backport PR will follow."
                            if result["outcome"] == "add"
                            else f":no_entry_sign: No backport for <{env('PR_URL')}|#{pr_number}>."
                            if result["outcome"] == "dismiss"
                            else f":alarm_clock: No answer within {hours} hour(s) — to backport "
                            f"<{env('PR_URL')}|#{pr_number}> manually, add a backport/<branch> label."
                        ),
                    },
                }
            ],
        )
    except Exception as e:  # noqa: BLE001
        print(f"Could not update the message (non-fatal): {e}")

    output("outcome", result["outcome"])
    output("branch", result["branch"])
    print(f"Outcome: {result['outcome']} {result['branch']}", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
