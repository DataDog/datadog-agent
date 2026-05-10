from __future__ import annotations

import re

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.color import color_message


def get_pr_for_branch(branch: str):
    """
    Get PR info for a branch. Returns the PR object or None.

    This function is used to cache PR lookup results for reuse across:
    - Adding PR number as a metric tag
    - Displaying PR comments

    Args:
        branch: The branch name to look up

    Returns:
        The PR object if found, None otherwise
    """
    try:
        github = GithubAPI()
        prs = list(github.get_pr_for_branch(branch))
        return prs[0] if prs else None
    except Exception as e:
        print(color_message(f"[WARN] Failed to get PR for branch {branch}: {type(e).__name__}: {e}", "orange"))
        return None


def get_pr_number_from_commit(ctx) -> str | None:
    """
    Extract PR number from the HEAD commit message.

    On main branch, merged commits typically end with (#XXXXX).
    Example: "Fix bug in quality gates (#44462)"

    Args:
        ctx: Invoke context for running git commands

    Returns:
        The PR number as a string, or None if not found.
    """
    try:
        result = ctx.run("git log -1 --pretty=%s HEAD", hide=True)
        commit_message = result.stdout.strip()

        # Match pattern like "(#12345)" at the end of the message
        match = re.search(r'\(#(\d+)\)\s*$', commit_message)
        if match:
            return match.group(1)
        return None
    except Exception as e:
        print(color_message(f"[WARN] Failed to extract PR number from commit: {e}", "orange"))
        return None


def get_pr_author(pr_number: str) -> str | None:
    """
    Get the author (login) of a PR by its number.

    Args:
        pr_number: The PR number as a string

    Returns:
        The PR author's GitHub login, or None if not found.
    """
    try:
        github = GithubAPI()
        pr = github.get_pr(int(pr_number))
        return pr.user.login if pr and pr.user else None
    except Exception as e:
        print(color_message(f"[WARN] Failed to get PR author for PR #{pr_number}: {e}", "orange"))
        return None
