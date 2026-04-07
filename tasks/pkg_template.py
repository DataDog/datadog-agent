from invoke.context import Context
from invoke.tasks import task

from tasks.libs.build.bazel import bazel


@task
def generate(ctx: Context):
    """
    Generate the code for the template package.
    Takes the code from the Go standard library and applies the patches.
    """
    bazel(ctx, "run", "//pkg/template:generate")
