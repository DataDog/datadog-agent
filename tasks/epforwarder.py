from invoke import task

@task
def gen_mocks(ctx):
    """
    Generate mocks
    """

    ctx.run(f"go generate pkg/epforwarder/epforwarder.go")
