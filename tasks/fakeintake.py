"""
Build or use the fake intake client CLI
"""

from invoke import task

from tasks.libs.common.go import go_build


@task
def build(ctx):
    """
    Build the fake intake
    """
    with ctx.cd("test/fakeintake"):
        go_build(ctx, "cmd/server/main.go", bin_path="build/fakeintake")
        go_build(ctx, "cmd/client/main.go", bin_path="build/fakeintakectl")


@task
def test(ctx):
    """
    Run the fake intake tests
    """
    with ctx.cd("test/fakeintake"):
        ctx.run("go test ./...")
