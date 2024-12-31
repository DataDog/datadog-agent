import shutil

from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.libs.common.color import color_message


@task
def run(ctx):
    """
    Run type checking for files defined in pyright.json
    """

    pyright = "pyright"
    if shutil.which(pyright) is None:
        raise Exit(
            message=color_message(f"'{pyright}' not found in PATH. Please install it with `inv setup`", "red"),
        )
    ctx.run("pyright -p ./tasks/pyright.json")
