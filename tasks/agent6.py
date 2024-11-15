from invoke import task

from tasks.libs.common.agent6 import prepare


@task
def init(ctx):
    prepare(ctx)


@task
def run(ctx, command):
    """Run a command in the agent 6 environment."""
    prepare(ctx)
    ctx.run(f"cd {ctx.agent6_worktree} && {command}")
