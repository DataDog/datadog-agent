from invoke.context import Context
from invoke.tasks import task

from tasks.libs.build.bazel import bazel


@task
def generate(_: Context):
    """
    Generate the code for the template package.
    Takes the code from the Go standard library and applies the patches.
    """
    bazel("run", "//pkg/template:generate")
