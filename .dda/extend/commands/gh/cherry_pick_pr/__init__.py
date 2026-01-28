# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

import json
import os
from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app
from dda.utils.ci import running_in_ci

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(
    short_help="Cherry-pick a merged PR changes to another branch",
    context_settings={"help_option_names": [], "ignore_unknown_options": True},
    features=["github"],
)
@click.option("--pr-number", type=int)
@click.option("--target-branch", type=str, required=True)
@pass_app
def cmd(
    app: Application,
    target_branch: str,
    pr_number: int | None = None,
) -> None:
    """
    Cherry-pick a merged PR changes to another branch.
    """
    # Get the PR either from --pr-number or from the event
    if pr_number:
        original_pr = get_pr_by_number(pr_number)
    else:
        event = get_event()
        original_pr = event.get("pull_request")
        if not original_pr:
            app.display_warning("Expecting a pull request event or --pr-number argument.")
            return

    base = target_branch

    # Merge commit SHA (the commit created on base branch)
    merge_commit_sha = original_pr.get("merge_commit_sha")
    if not original_pr.get("merged", False) or not merge_commit_sha:
        app.display_info("For security reasons, this action should only run on merged PRs.")
        return

    original_pr_number = original_pr.get("number")

    # Authenticate to GitHub and get a token
    token = os.getenv("GITHUB_TOKEN")
    if not token:
        app.abort("GITHUB_TOKEN is not set")

    if running_in_ci():
        app.subprocess.run(["git", "config", "--global", "user.name", "github-actions[bot]"], check=True)
        app.subprocess.run(
            ["git", "config", "--global", "user.email", "github-actions[bot]@users.noreply.github.com"], check=True
        )
    app.subprocess.run(["git", "switch", base], check=True)

    if (
        app.subprocess.run(
            ["git", "cherry-pick", "-x", "--mainline", "1", merge_commit_sha],
        )
        != 0
    ):
        app.subprocess.run(["git", "cherry-pick", "--abort"], check=True)
        worktree_path = f".worktrees/backport-${base}"
        head = f"backport-{original_pr_number}-to-{base}"
        error_message = f"""Failed to cherry-pick {merge_commit_sha}
To backport manually, run these commands in your terminal:
```bash
# Fetch latest updates from GitHub
git fetch
# Create a new working tree
git worktree add {worktree_path} {base}
# Navigate to the new working tree
cd {worktree_path}
# Create a new branch
git switch --create {head}
# Cherry-pick the merged commit of this pull request and resolve the conflicts
git cherry-pick -x --mainline 1 {merge_commit_sha}
# Push it to GitHub
git push --set-upstream origin {head}
# Go back to the original working tree
cd ../..
# Delete the working tree
git worktree remove {worktree_path}"""
        app.abort(error_message)

    # extract message and author from the cherry-pick commit
    cherry_pick_message = app.subprocess.capture(["git", "log", "-1", "--pretty=format:%s%n%b"], check=True)
    cherry_pick_author = app.subprocess.capture(["git", "log", "-1", "--pretty=format:%an <%ae>"], check=True)

    # Create the backport PR
    original_body = original_pr.get("body", "")
    original_labels = get_non_backport_labels(original_pr.get("labels", []))
    original_title = original_pr.get("title")

    # Set outputs
    with open(os.environ["GITHUB_OUTPUT"], "a") as f:
        if original_pr_number:
            f.write(f"original_pr_number={original_pr_number}\n")
        if base:
            f.write(f"base={base}\n")
        if merge_commit_sha:
            f.write(f"merge_commit_sha={merge_commit_sha}\n")
        if original_labels:
            f.write(f"original_labels={','.join(original_labels)}\n")
        if original_title:
            f.write(f"original_title<<EOF\n{original_title}\nEOF\n")
        if original_body:
            f.write(f"original_body<<EOF\n{original_body}\nEOF\n")
        if cherry_pick_message:
            f.write(f"message<<EOF\n{cherry_pick_message}\nEOF\n")
        if cherry_pick_author:
            f.write(f"author<<EOF\n{cherry_pick_author}\nEOF\n")

    app.display(f"Cherry-picked PR #{original_pr_number} on branch {base}")


def get_event() -> dict:
    event_path = os.environ["GITHUB_EVENT_PATH"]
    with open(event_path, encoding="utf-8") as f:
        return json.load(f)


def get_non_backport_labels(labels: list[dict]) -> list[str]:
    """
    Get all labels that are not backport labels.
    """
    non_backport_labels = []
    for label in labels:
        name = label.get("name", "")
        if not name:
            continue
        if name.startswith("backport/"):
            continue
        non_backport_labels.append(name)
    return non_backport_labels


def get_pr_by_number(pr_number: int) -> dict:
    """
    Get a PR by its number.
    """
    from github import Github

    g = Github(os.getenv("GITHUB_TOKEN"))
    repo = g.get_repo("DataDog/datadog-agent")
    pr = repo.get_pull(pr_number)
    return pr.raw_data
