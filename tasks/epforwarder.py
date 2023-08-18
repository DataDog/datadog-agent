from invoke import task


@task
def gen_mocks(ctx):
    """
    Generate mocks
    """

    ctx.run("go generate pkg/epforwarder/epforwarder.go")
