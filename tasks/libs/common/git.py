from __future__ import annotations

import os.path
from typing import TYPE_CHECKING

from invoke.exceptions import Exit

from tasks.libs.common.color import color_message
from tasks.libs.common.constants import DEFAULT_BRANCH
from tasks.libs.common.user_interactions import yes_no_question

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


def get_main_parent_commit(ctx) -> str:
    """
    Get the commit sha your current branch originated from
    """
    return ctx.run("git merge-base HEAD origin/main", hide=True).stdout.strip()


def check_base_branch(branch, release_version):
    """
    Checks if the given branch is either the default branch or the release branch associated
    with the given release version.
    """
    return branch == DEFAULT_BRANCH or branch == release_version.branch()


def try_git_command(ctx, git_command):
    """
    Try a git command that should be retried (after user confirmation) if it fails.
    Primarily useful for commands which can fail if commit signing fails: we don't want the
    whole workflow to fail if that happens, we want to retry.
    """

    do_retry = True

    while do_retry:
        res = ctx.run(git_command, warn=True)
        if res.exited is None or res.exited > 0:
            print(
                color_message(
                    f"Failed to run \"{git_command}\" (did the commit/tag signing operation fail?)",
                    "orange",
                )
            )
            do_retry = yes_no_question("Do you want to retry this operation?", color="orange", default=True)
            continue

        return True

    return False


def check_clean_branch_state(ctx, github, branch):
    """
    Check we are in a clean situation to create a new branch:
    No uncommitted change, and branch doesn't exist locally or upstream
    """
    if check_uncommitted_changes(ctx):
        raise Exit(
            color_message(
                "There are uncomitted changes in your repository. Please commit or stash them before trying again.",
                "red",
            ),
            code=1,
        )
    if check_local_branch(ctx, branch):
        raise Exit(
            color_message(
                f"The branch {branch} already exists locally. Please remove it before trying again.",
                "red",
            ),
            code=1,
        )

    if github.get_branch(branch) is not None:
        raise Exit(
            color_message(
                f"The branch {branch} already exists upstream. Please remove it before trying again.",
                "red",
            ),
            code=1,
        )
