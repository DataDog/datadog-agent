"""Worktree utilities, used to execute tasks from this local repository (main) to a worktree with a different HEAD (e.g. 6.53.x).

Common environment variables that can be used:
- WORKTREE_NO_PULL: If set to any value, the worktree will not be pulled before running the command.
"""

import os
import sys
from contextlib import contextmanager
from pathlib import Path

from invoke.exceptions import Exit, UnexpectedExit

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.git import get_current_branch

WORKTREE_DIRECTORY = Path.cwd().parent / "datadog-agent-worktree"
LOCAL_DIRECTORY = Path.cwd().resolve()


def init_env(ctx, branch: str | None = None, commit: str | None = None):
    """Will prepare the environment for commands applying to a worktree.

    To be used before each worktree section.
    Will:
    1. Add the agent worktree if not present.
    2. Fetch the latest changes from the agent worktree.
    """

    if not WORKTREE_DIRECTORY.is_dir():
        print(f'{color_message("Info", Color.BLUE)}: Cloning datadog agent into {WORKTREE_DIRECTORY}', file=sys.stderr)
        remote = ctx.run("git remote get-url origin", hide=True).stdout.strip()
        # Try to use this option to reduce cloning time
        if not ctx.run(
            f"git clone '{remote}' '{WORKTREE_DIRECTORY}' -b {branch or 'main'}",
            warn=True,
            hide=True,
        ):
            raise Exit(
                f'{color_message("Error", Color.RED)}: Cannot initialize worktree environment. You might want to reset the worktree directory with `dda inv worktree.remove`',
                code=1,
            )

    # Copy the configuration file
    ctx.run(f"cp {LOCAL_DIRECTORY}/.git/config {WORKTREE_DIRECTORY}/.git/config", hide=True)
    # Be sure the target branch is present locally and set up to track the remote branch
    ctx.run(f"git -C '{WORKTREE_DIRECTORY}' branch {branch or 'main'} origin/{branch or 'main'} || true", hide=True)
    # If the state is not clean, clean it
    if ctx.run(f"git -C '{WORKTREE_DIRECTORY}' status --porcelain", hide=True).stdout.strip():
        print(f'{color_message("Info", Color.BLUE)}: Cleaning worktree directory', file=sys.stderr)
        ctx.run(f"git -C '{WORKTREE_DIRECTORY}' reset --hard", hide=True)
        ctx.run(f"git -C '{WORKTREE_DIRECTORY}' clean -f", hide=True)

    if branch:
        worktree_branch = ctx.run(
            f"git -C '{WORKTREE_DIRECTORY}' rev-parse --abbrev-ref HEAD", hide=True
        ).stdout.strip()
        if worktree_branch != branch:
            for retry in range(2):
                try:
                    ctx.run(f"git -C '{WORKTREE_DIRECTORY}' checkout '{branch}'", hide=True)
                except UnexpectedExit as e:
                    if retry == 1:
                        raise e
                    else:
                        print(
                            f'{color_message("Warning", Color.ORANGE)}: Git branch not found in the local worktree folder, fetching repository',
                            file=sys.stderr,
                        )
                        ctx.run(f"git -C '{WORKTREE_DIRECTORY}' fetch", hide=True)

        if not os.environ.get("AGENT_WORKTREE_NO_PULL"):
            ctx.run(f"git -C '{WORKTREE_DIRECTORY}' pull", hide=True)

    if commit:
        if not os.environ.get("AGENT_WORKTREE_NO_PULL"):
            ctx.run(f"git -C '{WORKTREE_DIRECTORY}' fetch", hide=True)

        ctx.run(f"git -C '{WORKTREE_DIRECTORY}' checkout '{commit}'", hide=True)


def remove_env(ctx):
    """Will remove the environment for commands applying to a worktree."""

    ctx.run(f"rm -rf '{WORKTREE_DIRECTORY}'", warn=True)


def is_worktree():
    """Will return True if the current environment is a worktree environment."""

    return Path.cwd().resolve() == WORKTREE_DIRECTORY.resolve()


def enter_env(ctx, branch: str | None, skip_checkout=False, commit: str | None = None):
    """Enters the worktree environment."""

    if not (branch or commit):
        assert skip_checkout, 'skip_checkout must be set to True if branch and commit are None'

    if not skip_checkout:
        init_env(ctx, branch, commit=commit)
    else:
        assert WORKTREE_DIRECTORY.is_dir(), "Worktree directory is not present and skip_checkout is set to True"

    os.chdir(WORKTREE_DIRECTORY)
    if skip_checkout and branch:
        current_branch = get_current_branch(ctx)
        assert (
            current_branch == branch
        ), f"skip_checkout is True but the current branch ({current_branch}) is not {branch}. You should check out the branch before using this command, this can be safely done with `dda inv worktree.checkout {branch}`."


def exit_env():
    """Exits the worktree environment."""

    os.chdir(LOCAL_DIRECTORY)


@contextmanager
def agent_context(ctx, branch: str | None = None, skip_checkout=False, commit: str | None = None):
    """Applies code to the worktree environment if the branch is not None.

    Args:
        branch: The branch to switch to. If None, will enter the worktree environment without switching branch (ensures that skip_checkout is True).
        skip_checkout: If True, the branch will not be checked out (no pull will be performed too).
        commit: The commit to checkout. Is used instead of branch if provided.

    Usage:
        > with agent_context(ctx, branch):
        >    ctx.run("head CHANGELOG.rst")  # Displays the changelog of the target branch
    """

    # Do not stack two environments
    if is_worktree():
        if not skip_checkout and (
            branch
            and branch != get_current_branch(ctx)
            or commit
            and commit != ctx.run("git rev-parse HEAD", hide=True).stdout.strip()
        ):
            raise RuntimeError('Cannot stack two different worktree environments (two different branches requested)')
        else:
            # Some tasks need to stack two different worktree environments but
            # on the same branch
            # Simulate worktree environment
            yield
            return

    try:
        # Enter
        enter_env(ctx, branch, skip_checkout=skip_checkout, commit=commit)

        yield
    except Exception as e:
        location = get_current_branch(ctx)
        message = f'{color_message("WARNING", Color.ORANGE)}: This error takes place in a worktree environment on branch {location}'

        e.add_note(message)
        # Also print the warning since it might be an invoke error which exits without displaying the message
        print(message, file=sys.stderr)

        raise e
    finally:
        # Exit
        exit_env()
