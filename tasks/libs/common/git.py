from __future__ import annotations

import os
import tempfile
from contextlib import contextmanager
from typing import TYPE_CHECKING

from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.constants import DEFAULT_AGENT6_BRANCH, DEFAULT_BRANCH
from tasks.libs.common.user_interactions import yes_no_question

if TYPE_CHECKING:
    from collections.abc import Iterable


@contextmanager
def clone(ctx, repo, branch, options=""):
    """
    Context manager to clone a git repository and checkout a specific branch.
    """
    current_dir = os.getcwd()
    try:
        with tempfile.TemporaryDirectory() as clone_dir:
            ctx.run(f"git clone -b {branch} {options} https://github.com/DataDog/{repo} {clone_dir}")
            os.chdir(clone_dir)
            yield
    finally:
        os.chdir(current_dir)


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


def get_file_modifications(
    ctx, base_branch=None, added=False, modified=False, removed=False, only_names=False, no_renames=False
) -> list[tuple[str, str]]:
    """Gets file status changes for the current branch compared to the base branch.

    If no filter is provided, will return all the files.

    Args:
        added: Include added files
        modified: Include modified files
        removed: Include removed files
        only_names: Return only the file names without the status
        no_renames: Do not include renamed files

    Returns:
        A list of (status, filename)
    """

    from tasks.libs.releasing.json import _get_release_json_value

    base_branch = base_branch or _get_release_json_value('base_branch')

    last_main_commit = ctx.run(f"git merge-base HEAD origin/{base_branch}", hide=True).stdout.strip()

    flags = '--no-renames' if no_renames else ''

    modifications = [
        line.split()
        for line in ctx.run(f"git diff --name-status {flags} {last_main_commit}", hide=True).stdout.splitlines()
    ]

    if added or modified or removed:
        modifications = [
            (status, file)
            for status, file in modifications
            if (added and status == "A") or (modified and status in "MCRT") or (removed and status == "D")
        ]

    if only_names:
        modifications = [file for _, file in modifications]

    return modifications


def get_modified_files(ctx, base_branch="main") -> list[str]:
    return get_file_modifications(
        ctx, base_branch=base_branch, added=True, modified=True, only_names=True, no_renames=True
    )


def get_current_branch(ctx) -> str:
    return ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()


def get_common_ancestor(ctx, branch, base=DEFAULT_BRANCH) -> str:
    return ctx.run(f"git merge-base {branch} {base}", hide=True).stdout.strip()


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


def check_base_branch(branch, release_version, agent6=False):
    """
    Checks if the given branch is either the default branch or the release branch associated
    with the given release version.
    """
    return branch == (DEFAULT_AGENT6_BRANCH if agent6 else DEFAULT_BRANCH) or branch == release_version.branch()


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


def get_last_commit(ctx, repo, branch):
    # Repo is only the repo name, e.g. "datadog-agent"
    return (
        ctx.run(
            rf'git ls-remote -h https://github.com/DataDog/{repo} "refs/heads/{branch}"',
            hide=True,
        )
        .stdout.strip()
        .split()[0]
    )


def get_last_release_tag(ctx, repo, pattern):
    import re
    from functools import cmp_to_key

    import semver

    tags = ctx.run(
        rf'git ls-remote -t https://github.com/DataDog/{repo} "{pattern}"',
        hide=True,
    ).stdout.strip()
    if not tags:
        raise Exit(
            color_message(
                f"No tag found for pattern {pattern} in {repo}",
                Color.RED,
            ),
            code=1,
        )

    release_pattern = re.compile(r'.*7\.[0-9]+\.[0-9]+(-rc.*|-devel.*)?$')
    tags_without_suffix = [
        line for line in tags.splitlines() if not line.endswith("^{}") and release_pattern.match(line)
    ]
    last_tag = max(tags_without_suffix, key=lambda x: cmp_to_key(semver.compare)(x.split('/')[-1]))
    last_tag_commit, last_tag_name = last_tag.split()
    tags_with_suffix = [line for line in tags.splitlines() if line.endswith("^{}") and release_pattern.match(line)]
    if tags_with_suffix:
        last_tag_with_suffix = max(
            tags_with_suffix, key=lambda x: cmp_to_key(semver.compare)(x.split('/')[-1].removesuffix("^{}"))
        )
        last_tag_commit_with_suffix, last_tag_name_with_suffix = last_tag_with_suffix.split()
        if (
            semver.compare(last_tag_name_with_suffix.split('/')[-1].removesuffix("^{}"), last_tag_name.split("/")[-1])
            >= 0
        ):
            last_tag_commit = last_tag_commit_with_suffix
            last_tag_name = last_tag_name_with_suffix.removesuffix("^{}")
    last_tag_name = last_tag_name.removeprefix("refs/tags/")
    return last_tag_commit, last_tag_name
