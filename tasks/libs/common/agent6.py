"""Agent 6 / 7 compatibility utilities, used to execute tasks from Agent 7 (main) on Agent 6 (6.53.x).

Common environment variables that can be used:
- AGENT6_NO_PULL: If set to any value, the agent6 worktree will not be pulled before running the command.
"""

import os

from pathlib import Path

AGENT6_BRANCH = "6.53.x"
AGENT6_WORKTREE = Path.cwd().parent / "datadog-agent6"


def prepare(ctx):
    """Will prepare the environment for agent6 commands.

    To be used before each agent6 command.
    Will:
    1. Add the agent6 worktree if not present.
    2. Fetch the latest changes from the agent6 worktree.
    """

    if not AGENT6_WORKTREE.is_dir():
        ctx.run(f"git worktree add '{AGENT6_WORKTREE}' origin/{AGENT6_BRANCH}", warn=True)

    if not os.environ.get("AGENT6_NO_PULL"):
        ctx.run(f"git -C '{AGENT6_WORKTREE}' fetch origin {AGENT6_BRANCH}", warn=True)


def is_agent6():
    """Will return True if the current environment is an agent6 environment."""

    return Path.cwd() == AGENT6_WORKTREE
