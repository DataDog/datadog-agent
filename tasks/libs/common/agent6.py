"""Agent 6 / 7 compatibility utilities, used to execute tasks from Agent 7 (main) on Agent 6 (6.53.x).

Common environment variables that can be used:
- AGENT6_NO_PULL: If set to any value, the agent6 worktree will not be pulled before running the command.
"""

import os
from contextlib import contextmanager
from pathlib import Path

AGENT6_BRANCH = "6.53.x"
AGENT6_WORKING_DIRECTORY = Path.cwd().parent / "datadog-agent6"
AGENT7_WORKING_DIRECTORY = Path.cwd()


def init_env(ctx):
    """Will prepare the environment for agent6 commands.

    To be used before each agent6 command.
    Will:
    1. Add the agent6 worktree if not present.
    2. Fetch the latest changes from the agent6 worktree.
    """

    if not AGENT6_WORKING_DIRECTORY.is_dir():
        ctx.run(f"git worktree add '{AGENT6_WORKING_DIRECTORY}' origin/{AGENT6_BRANCH}", warn=True)

    if not os.environ.get("AGENT6_NO_PULL"):
        ctx.run(f"git -C '{AGENT6_WORKING_DIRECTORY}' fetch origin {AGENT6_BRANCH}", warn=True, hide=True)


def remove_env(ctx):
    """Will remove the environment for agent6 commands."""

    if AGENT6_WORKING_DIRECTORY.is_dir():
        ctx.run(f"git worktree remove '{AGENT6_WORKING_DIRECTORY}'", warn=True)


def is_agent6():
    """Will return True if the current environment is an agent6 environment."""

    return Path.cwd() == AGENT6_WORKING_DIRECTORY


def enter_agent6_context(ctx):
    """Enters the agent 6 environment."""

    init_env(ctx)

    os.chdir(AGENT6_WORKING_DIRECTORY)


def exit_agent6_context():
    """Exits the agent 6 environment."""

    os.chdir(AGENT7_WORKING_DIRECTORY)


@contextmanager
def agent_context(ctx, version: str | int | None):
    """Runs code from the agent6 environment if the version is 6.

    Usage:
        > with agent_context(ctx, version):
        >    ctx.run("head CHANGELOG.rst")  # Displays the changelog of the target version
    """

    switch_agent6 = version == 6 or isinstance(version, str) and version.startswith("6")

    if switch_agent6:
        # Do not stack two agent 6 contexts
        if is_agent6():
            yield
            return

        try:
            # Enter
            enter_agent6_context(ctx)

            yield
        finally:
            # Exit
            exit_agent6_context()
    else:
        # NOTE: This ensures that we don't push agent 7 context from agent 6 context (context might be switched within inner functions)
        assert not is_agent6(), 'Agent 7 context cannot be used within an agent 6 context'

        yield
