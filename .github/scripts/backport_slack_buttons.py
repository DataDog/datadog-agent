#!/usr/bin/env python3
"""
Used by the "Assess backport" workflow (.github/workflows/assess-backport.yml).

Sends the PR owner a Slack DM with one "backport" button per candidate release
branch (as assessed by Claude), then listens over Socket Mode for up to
SLACK_BUTTON_TTL_SECONDS for clicks — no public endpoint needed. Each branch
button is acknowledged in the thread and removed from the message, so several
branches can be chosen; "No backport needed" ends the exchange. Button clicks
are applied back in the workflow with a dd-octo-sts token, never with a token
held by this script.

Requires a Slack app with Socket Mode interactivity enabled and the bot scopes
`chat:write`, `im:write`, `users:read.email`, plus an app-level token with
`connections:write`. See the workflow file for the expected secrets.

Inputs (environment):
  SLACK_BOT_TOKEN        xoxb-... bot token of the Slack app above
  SLACK_APP_TOKEN        xapp-... app-level token (connections:write)
  GITHUB_TOKEN           read-only, to resolve the PR owner's email from their commits
  PR_NUMBER, PR_TITLE, PR_URL, PR_AUTHOR_LOGIN (the PR owner's GitHub login)
  BRANCHES               comma-separated candidate branches, e.g. "7.85.x,7.86.x"
  EXPLANATION            Claude's one-line rationale (may be empty)
  CONFIDENCE             HIGH|MEDIUM|LOW
  SLACK_BUTTON_TTL_SECONDS  how long to wait for clicks (default 7200)

Outputs (GITHUB_OUTPUT):
  outcome                add | dismiss | timeout | no_recipient
  branches               comma-separated clicked branches when outcome is "add"
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


DEVX_OPS_CHANNEL = "#agent-devx-ops"


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

    # Resolve the PR owner's email from their own commits on the PR: prefer a commit
    # authored by the PR owner (the last commit may have been pushed by someone else),
    # fall back to the last commit's author.
    try:
        commits = github_get(
            env("GITHUB_TOKEN"),
            f"https://api.github.com/repos/{env('GITHUB_REPOSITORY')}/pulls/{pr_number}/commits",
        )
        owner_email = next(
            (
                c["commit"]["author"]["email"]
                for c in reversed(commits)
                if c.get("author", {}).get("login") == env("PR_AUTHOR_LOGIN")
            ),
            commits[-1]["commit"]["author"]["email"],
        )
    except Exception as e:  # noqa: BLE001
        print(f"Could not resolve the PR owner's email: {e}")
        notify_devx(
            web, pr_number, env("PR_URL"), branches,
            f"Could not resolve the PR owner's email from their commits ({e}).",
        )
        output("outcome", "no_recipient")
        return 0

    try:
        user_id = web.users_lookupByEmail(email=owner_email)["user"]["id"]
    except Exception as e:  # noqa: BLE001
        # External contributor or no Slack account: nobody to DM, ask agent-devx-ops
        # to handle the backport decision (e.g. by adding the label manually).
        print(f"No Slack user for {owner_email}: {e}")
        notify_devx(
            web, pr_number, env("PR_URL"), branches,
            f"Could not DM the PR owner (no Slack user for {owner_email}). "
            "Please assess the backport yourself and add the label if warranted.",
        )
        output("outcome", "no_recipient")
        return 0

    hours = max(1, round(ttl / 3600))
    rationale = f"\n> {explanation}\n" if explanation else "\n"

    def section_text() -> str:
        return (
            f":rewind: Claude thinks <{env('PR_URL')}|#{pr_number} "
            f"({env('PR_TITLE')})> may need a backport "
            f"(confidence: {confidence}).\n{rationale}"
            "Pick one or more branches, then hit *Apply selection* — the labels are only "
            f"applied once you do. Buttons expire in about {hours} hour(s) — after that, "
            "add the backport/<branch> label manually."
        )

    def action_block(remaining: list[str]) -> dict:
        elements = [
            {
                "type": "button",
                "style": "primary",
                # action_id must be unique per element, so append the branch
                "action_id": f"add_label:{b}",
                "text": {"type": "plain_text", "text": b},
                "value": b,
            }
            for b in remaining
        ]
        elements.append(
            {
                "type": "button",
                "action_id": "apply",
                "text": {"type": "plain_text", "text": "Apply selection"},
                "value": "apply",
            }
        )
        elements.append(
            {
                "type": "button",
                "style": "danger",
                "action_id": "dismiss",
                "text": {"type": "plain_text", "text": "No backport needed"},
                "value": "dismiss",
            }
        )
        return {"type": "actions", "elements": elements}

    blocks = [
        {"type": "section", "text": {"type": "mrkdwn", "text": section_text()}},
        action_block(branches),
    ]
    msg = web.chat_postMessage(
        channel=web.conversations_open(users=[user_id])["channel"]["id"],
        text=f":rewind: Backport assessment for #{pr_number}",
        blocks=blocks,
    )
    print(f"Sent: {msg['permalink']}", flush=True)

    # Listen for clicks over Socket Mode (no public endpoint required).
    logging.basicConfig(level=logging.INFO, stream=sys.stdout)
    sm = SocketModeClient(app_token=env("SLACK_APP_TOKEN"), web_client=web)
    clicked = Event()
    result = {"outcome": "timeout", "chosen": []}

    def thread_reply(text: str) -> None:
        try:
            web.chat_postMessage(channel=msg["channel"], thread_ts=msg["ts"], text=text)
        except Exception as e:  # noqa: BLE001
            print(f"Could not post a thread reply (non-fatal): {e}", flush=True)

    def set_buttons(remaining: list[str] | None) -> None:
        # remaining=None removes the actions block entirely (exchange is over)
        blocks_now = [{"type": "section", "text": {"type": "mrkdwn", "text": section_text()}}]
        if remaining is not None:
            blocks_now.append(action_block(remaining))
        try:
            web.chat_update(
                channel=msg["channel"],
                ts=msg["ts"],
                text=f":rewind: Backport assessment for #{pr_number}",
                blocks=blocks_now,
            )
        except Exception as e:  # noqa: BLE001
            print(f"Could not update the message (non-fatal): {e}", flush=True)

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
        if payload.get("container", {}).get("message_ts") != msg["ts"]:
            return
        for action in payload.get("actions", []):
            if action["action_id"] == "apply":
                if result["chosen"]:
                    clicked.set()  # apply the selection and stop listening
                else:
                    thread_reply("No branch selected yet — pick at least one, or choose 'No backport needed'.")
            elif action["action_id"] == "dismiss":
                result["outcome"] = "dismiss"
                set_buttons(None)  # remove the buttons, keep the section
                thread_reply(f":no_entry_sign: No backport for <{env('PR_URL')}|#{pr_number}>.")
            elif action["action_id"].startswith("add_label:"):
                branch = action["value"]
                if branch not in result["chosen"]:
                    result["outcome"] = "add"
                    result["chosen"].append(branch)
                    remaining = [b for b in branches if b not in result["chosen"]]
                    set_buttons(remaining)
                    thread_reply(
                        f":white_check_mark: Selected `{branch}`. Keep picking branches, or hit "
                        "*Apply selection* — labels (and the backport PRs) follow from there."
                    )
                    if not remaining:
                        clicked.set()  # all candidate branches chosen, we're done

    sm.socket_mode_request_listeners.append(process)
    sm.connect()
    print(f"Socket Mode connected: {sm.is_connected()}", flush=True)
    clicked.wait(timeout=ttl)

    # End of exchange: drop any remaining buttons so stale clicks can't pile up.
    if result["outcome"] == "add":
        set_buttons(None)
        thread_reply(
            ":ok: Choice recorded — applying the backport labels now "
            f"(`{', '.join(result['chosen'])}`)."
        )
    elif result["outcome"] == "timeout" and not result["chosen"]:
        set_buttons(None)
        thread_reply(
            f":alarm_clock: No answer within {hours} hour(s) — to backport "
            f"<{env('PR_URL')}|#{pr_number}> manually, add a backport/<branch> label."
        )
    # On timeout with at least one click, the chosen buttons are already confirmed
    # in the thread and removed; proceed with them.

    outcome = "add" if result["chosen"] else result["outcome"]
    output("outcome", outcome)
    output("branches", ",".join(result["chosen"]))
    print(f"Outcome: {outcome} {','.join(result['chosen'])}", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())



def notify_devx(web, pr_number: str, pr_url: str, branches: list[str], problem: str) -> None:
    """Tell #agent-devx-ops when the PR owner cannot be reached, so the backport
    decision is not silently dropped."""
    message = (
        f":warning: Backport assessment could not reach the owner of "
        f"<{pr_url}|#{pr_number}> (candidates: {', '.join(f'`{b}`' for b in branches)}). "
        f"{problem}"
    )
    try:
        web.chat_postMessage(channel=DEVX_OPS_CHANNEL, text=message)
    except Exception as e:  # noqa: BLE001
        print(f"Could not notify {DEVX_OPS_CHANNEL} (non-fatal): {e}", flush=True)
