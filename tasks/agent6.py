from invoke import task

from tasks.libs.common.agent6 import _agent6_context, enter_agent6_context, prepare


@task
def init(ctx):
    """Will prepare the agent 6 context (git clone / pull of the agent 6 branch)."""

    prepare(ctx)


@task
def run(ctx, command):
    """Runs a command in the agent 6 environment."""

    with _agent6_context(ctx):
        ctx.run(command)


@task
def invoke(ctx):
    """Enters the agent 6 environment in order to invoke tasks in this context.

    Usage:
        > inv agent6.invoke modules.show-all  # Will show agent 6 modules
    """

    # The tasks run after this one will be using the agent 6 environment
    enter_agent6_context(ctx)
