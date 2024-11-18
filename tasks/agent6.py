from invoke import task

from tasks.libs.common.agent6 import agent_context, enter_agent6_context, prepare


@task
def init(ctx):
    """Will prepare the agent 6 context (git clone / pull of the agent 6 branch)."""

    prepare(ctx)


@task
def run(ctx, command):
    """Runs a command in the agent 6 environment."""

    with agent_context(ctx, 6):
        ctx.run(command)


@task
def invoke(ctx):
    """Enters the agent 6 environment in order to invoke tasks in this context.

    Note:
        This task should be avoided when a --version, --major-version or --agent-version argument is available in the task.

    Usage:
        > inv agent6.invoke modules.show-all  # Will show agent 6 modules
    """

    # The tasks running after this one will be using the agent 6 environment
    enter_agent6_context(ctx)
