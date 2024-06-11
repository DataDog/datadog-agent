from __future__ import annotations

import os.path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Iterable


def get_staged_files(ctx, commit="HEAD", include_deleted_files=False) -> Iterable[str]:
    """
    Get the list of staged (to be committed) files in the repository compared to the `commit` commit.
    """

    files = ctx.run(f"git diff --name-only --staged {commit}", hide=True).stdout.strip().splitlines()

    if include_deleted_files:
        yield from files
    else:
        for file in files:
            if os.path.isfile(file):
                yield file


def get_modified_files(ctx) -> list[str]:
    last_main_commit = ctx.run("git merge-base HEAD origin/main", hide=True).stdout
    return ctx.run(f"git diff --name-only --no-renames {last_main_commit}", hide=True).stdout.splitlines()


def get_current_branch(ctx) -> str:
    return ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()


def check_uncommitted_changes(ctx):
    """
    Checks if there are uncommitted changes in the local git repository.
    """
    modified_files = ctx.run("git --no-pager diff --name-only HEAD | wc -l", hide=True).stdout.strip()

    # Return True if at least one file has uncommitted changes.
    return modified_files != "0"


def check_local_branch(ctx, branch):
    """
    Checks if the given branch exists locally
    """
    matching_branch = ctx.run(f"git --no-pager branch --list {branch} | wc -l", hide=True).stdout.strip()

    # Return True if a branch is returned by git branch --list
    return matching_branch != "0"


def get_commit_sha(ctx, commit="HEAD", short=False) -> str:
    return ctx.run(f"git rev-parse {'--short ' if short else ''}{commit}", hide=True).stdout.strip()
