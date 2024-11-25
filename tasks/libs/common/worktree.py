"""Worktree utilities, used to execute tasks from this local repository (main) to a worktree with a different HEAD (e.g. 6.53.x).

Common environment variables that can be used:
- WORKTREE_NO_PULL: If set to any value, the worktree will not be pulled before running the command.
"""

import os
from contextlib import contextmanager
from pathlib import Path

from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message

WORKTREE_DIRECTORY = Path.cwd().parent / "datadog-agent-worktree"
LOCAL_DIRECTORY = Path.cwd().resolve()


def init_env(ctx, branch: str | None = None):
    """Will prepare the environment for commands applying to a worktree.

    To be used before each worktree section.
    Will:
    1. Add the agent worktree if not present.
    2. Fetch the latest changes from the agent worktree.
    """

    if not WORKTREE_DIRECTORY.is_dir():
        if not ctx.run(f"git worktree add '{WORKTREE_DIRECTORY}' origin/{branch or 'main'}", warn=True):
            raise Exit(
                f'{color_message("Error", Color.RED)}: Cannot initialize worktree environment. You might want to reset the worktree directory with `inv worktree.remove`',
                code=1,
            )

    if branch:
        worktree_branch = ctx.run(
            f"git -C '{WORKTREE_DIRECTORY}' rev-parse --abbrev-ref HEAD", hide=True
        ).stdout.strip()
        if worktree_branch != branch:
            ctx.run(f"git -C '{WORKTREE_DIRECTORY}' checkout '{branch}'", hide=True)

        if not os.environ.get("AGENT_WORKTREE_NO_PULL"):
            ctx.run(f"git -C '{WORKTREE_DIRECTORY}' pull", hide=True)


def remove_env(ctx):
    """Will remove the environment for commands applying to a worktree."""

    ctx.run(f"git worktree remove -f '{WORKTREE_DIRECTORY}'", warn=True)


def is_worktree():
    """Will return True if the current environment is a worktree environment."""

    return Path.cwd() != LOCAL_DIRECTORY


def enter_env(ctx, branch: str, no_checkout=False):
    """Enters the worktree environment."""

    if not no_checkout:
        init_env(ctx, branch)
    else:
        assert WORKTREE_DIRECTORY.is_dir(), "Worktree directory is not present and no_checkout is set to True"

    os.chdir(WORKTREE_DIRECTORY)


def exit_env():
    """Exits the worktree environment."""

    os.chdir(LOCAL_DIRECTORY)


@contextmanager
def agent_context(ctx, branch: str | None, no_checkout=False):
    """Applies code to the worktree environment if the branch is not None.

    Args:
        branch: The branch to switch to.
        no_checkout: If True, the branch will not be switched (no pull will be performed too).

    Usage:
        > with agent_context(ctx, branch):
        >    ctx.run("head CHANGELOG.rst")  # Displays the changelog of the target branch
    """

    if branch is not None:
        # Do not stack two environments
        if is_worktree():
            yield
            return

        try:
            # Enter
            enter_env(ctx, branch, no_checkout=no_checkout)

            yield
        finally:
            # Exit
            exit_env()
    else:
        # NOTE: This ensures that we don't push local context from a worktree context (context might be switched within inner functions)
        assert not is_worktree(), 'Local context cannot be used within a worktree context'

        yield
