"""
Build or use the fake intake client CLI
"""

from invoke import task


@task
def build(ctx):
    """
    Build the fake intake
    """
    with ctx.cd("test/fakeintake"):
        ctx.run("go build -o build/fakeintake    cmd/server/main.go")
        ctx.run("go build -o build/fakeintakectl cmd/client/main.go")


@task
def test(ctx):
    """
    Run the fake intake tests
    """
    with ctx.cd("test/fakeintake"):
        ctx.run("go test ./...")
