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


@task
def t(ctx, version=None):
    from tasks.libs.common.agent6 import agent_context
    from tasks.libs.common.git import get_default_branch

    with agent_context(ctx, version):
        print(get_default_branch())
