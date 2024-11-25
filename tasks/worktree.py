from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.worktree import WORKTREE_DIRECTORY, agent_context, enter_env, init_env, remove_env


@task
def init(ctx, branch: str | None = None):
    """Will prepare the worktree context (git clone / pull of the agent branch)."""

    init_env(ctx, branch)


@task
def remove(ctx):
    """Will remove the git worktree context."""

    remove_env(ctx)


@task
def status(ctx):
    """Displays the status of the worktree environment."""

    if not WORKTREE_DIRECTORY.is_dir():
        raise Exit('No worktree environment found.')

    ctx.run(f"git -C '{WORKTREE_DIRECTORY}' status", pty=True)


@task
def switch(ctx, ref):
    """Changes the worktree environment to the specified ref.

    Note:
        This won't pull.
    """

    if not WORKTREE_DIRECTORY.is_dir():
        raise Exit('No worktree environment found.')

    ctx.run(f"git -C '{WORKTREE_DIRECTORY}' checkout '{ref}'", pty=True)


@task
def pull(ctx):
    """Pulls the worktree environment."""

    if not WORKTREE_DIRECTORY.is_dir():
        raise Exit('No worktree environment found.')

    ctx.run(f"git -C '{WORKTREE_DIRECTORY}' pull", pty=True)


@task
def run(ctx, branch: str, command: str, skip_checkout: bool = False):
    """Runs a command in the target worktree environment.

    Usage:
        $ inv worktree.run 6.53.x "head CHANGELOG.rst"  # Displays the changelog of the target branch
    """

    with agent_context(ctx, branch, skip_checkout=skip_checkout):
        ctx.run(command)


@task
def invoke(ctx, branch: str, skip_checkout: bool = False):
    """Enters the worktree environment in order to invoke tasks in this context.

    Note:
        This task should be avoided when a --version, --major-version or --agent-version argument is available in the task.

    Usage:
        > inv worktree.invoke 6.53.x modules.show-all  # Will show agent 6 modules
    """

    # The tasks running after this one will be using the agent 6 environment
    enter_env(ctx, branch, skip_checkout=skip_checkout)
