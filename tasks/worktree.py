from invoke import task

from tasks.libs.common.worktree import agent_context, enter_env, init_env, remove_env


@task
def init(ctx, branch: str | None = None):
    """Will prepare the worktree context (git clone / pull of the agent branch)."""

    init_env(ctx, branch)


@task
def remove(ctx):
    """Will the git worktree context."""

    remove_env(ctx)


@task
def run(ctx, branch: str, command: str):
    """Runs a command in the target worktree environment.

    Usage:
        $ inv worktree.run 6.53.x "head CHANGELOG.rst"  # Displays the changelog of the target branch
    """

    with agent_context(ctx, branch):
        ctx.run(command)


@task
def invoke(ctx, branch: str):
    """Enters the worktree environment in order to invoke tasks in this context.

    Note:
        This task should be avoided when a --version, --major-version or --agent-version argument is available in the task.

    Usage:
        > inv worktree.invoke 6.53.x modules.show-all  # Will show agent 6 modules
    """

    # The tasks running after this one will be using the agent 6 environment
    enter_env(ctx, branch)
