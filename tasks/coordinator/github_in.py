"""Inbound GitHub PR comment polling.

Called by the driver at the top of each iteration. Pulls new PR comments
from the run-log PR (`COORD_GITHUB_PR_NUMBER`), filters out comments
posted by the coordinator itself (via the `OWN_MESSAGE_MARKER` prefix
set by `github_out.format_message`), and appends the remainder to
`.coordinator/inbox.md` for the normal inbox drain → SDK-interpret → ACK
flow.

State is persisted to `.coordinator/github_state.json`:
  last_seen_id: int   Highest GitHub comment id already ingested.
                      Ensures idempotent re-polls.

Fails soft on any subprocess / gh / JSON error — `inbox.md` remains
the canonical input channel.
"""

from __future__ import annotations

import json
import subprocess
from pathlib import Path

from . import github_out
from .db import state_dir
from .inbox import INBOX_NAME, inbox_lock

STATE_FILENAME = "github_state.json"


def _state_path(root: Path) -> Path:
    return state_dir(root) / STATE_FILENAME


def _load_state(root: Path) -> dict:
    p = _state_path(root)
    if not p.exists():
        return {"last_seen_id": 0}
    try:
        return json.loads(p.read_text())
    except (OSError, json.JSONDecodeError):
        return {"last_seen_id": 0}


def _save_state(root: Path, state: dict) -> None:
    p = _state_path(root)
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(json.dumps(state, indent=2))


def is_configured() -> bool:
    return github_out.is_configured()


def _append_to_inbox(root: Path, text: str) -> None:
    # Serialise with claim_inbox / ack_and_archive: without the lock, an
    # append racing a rename writes into the renamed inode (inbox.md.reading)
    # and the bytes are silently archived as orphan content.
    p = state_dir(root) / INBOX_NAME
    p.parent.mkdir(parents=True, exist_ok=True)
    with inbox_lock(root):
        with p.open("a") as f:
            f.write(text.rstrip() + "\n\n")


def _fetch_comments(pr: str) -> tuple[list[dict] | None, str]:
    """Call `gh api` to pull all issue comments for the PR (PRs are issues).

    Returns (comments, detail). comments is None on error. Each comment
    includes `user.login` so callers can filter own comments by author,
    not by the fragile zero-width-space marker.
    """
    try:
        r = subprocess.run(
            [
                "gh",
                "api",
                "--paginate",
                f"repos/{{owner}}/{{repo}}/issues/{pr}/comments",
                "--jq",
                ".[] | {id, body, user_login: .user.login}",
            ],
            capture_output=True,
            text=True,
            timeout=30,
        )
    except FileNotFoundError:
        github_out.record_gh_result("poll", False)
        return None, "gh_cli_missing"
    except subprocess.TimeoutExpired:
        github_out.record_gh_result("poll", False)
        return None, "timeout"
    except Exception as e:
        github_out.record_gh_result("poll", False)
        return None, f"exception: {e.__class__.__name__}"

    if r.returncode != 0:
        github_out.record_gh_result("poll", False)
        return None, f"gh_error: {(r.stderr or r.stdout).strip()[:300]}"
    github_out.record_gh_result("poll", True)

    # --jq with --paginate emits one JSON object per line.
    out: list[dict] = []
    for line in r.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            out.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    return out, "ok"


def poll(root: Path = Path(".")) -> tuple[int, str]:
    """Pull new PR comments, append user replies to inbox.md.

    Returns (count_appended, detail). Never raises. No-op if unconfigured.
    """
    if not is_configured():
        return 0, "not_configured"

    pr = github_out.pr_number()
    assert pr is not None  # guarded by is_configured
    comments, detail = _fetch_comments(pr)
    if comments is None:
        return 0, detail

    state = _load_state(root)
    last_seen_id = int(state.get("last_seen_id", 0) or 0)

    # Process oldest-first so inbox ordering matches chronology.
    comments.sort(key=lambda c: int(c.get("id", 0)))

    # Own-filter: body-marker ONLY. Every coordinator-emitted comment
    # begins with OWN_MESSAGE_MARKER (U+200B, zero-width space) prepended
    # by github_out.format_message. A human posting from the same GitHub
    # login as the coordinator's gh auth cannot type that marker, so the
    # marker cleanly distinguishes the two — author login cannot, because
    # the coordinator posts under the same account the operator uses to
    # type steering comments, and an author-login filter silently swallows
    # every human message. (Pre-fix: this exact bug dropped 2+ days of
    # steering input without a trace.)
    appended = 0
    max_id = last_seen_id
    for c in comments:
        try:
            cid = int(c.get("id", 0))
        except (TypeError, ValueError):
            continue
        if cid <= last_seen_id:
            continue
        body = c.get("body") or ""
        is_own = body.startswith(github_out.OWN_MESSAGE_MARKER)
        if is_own:
            # Own comment; advance the cursor so we don't reconsider next poll.
            if cid > max_id:
                max_id = cid
            continue
        _append_to_inbox(root, body)
        appended += 1
        if cid > max_id:
            max_id = cid

    if max_id != last_seen_id:
        state["last_seen_id"] = max_id
        _save_state(root, state)

    return appended, f"appended={appended}"
