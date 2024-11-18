from invoke import task

from tasks.libs.common.agent6 import _agent6_context, prepare


@task
def init(ctx):
    """Will prepare the agent 6 context (git clone / pull of the agent 6 branch)."""

    prepare(ctx)


@task
def run(ctx, command):
    """Runs a command in the agent 6 environment."""

    with _agent6_context(ctx):
        ctx.run(command)
