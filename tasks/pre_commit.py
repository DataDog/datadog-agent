import sys

from invoke import task
from invoke.context import Context

from tasks.libs.common.color import color_message


@task
def shell_check_no_set_x(ctx: Context):
    """
    Check that shell scripts do not use 'set -x' or 'set -o xtrace'"
    """
    command = "git grep -rnE --color=always 'set( +-[^ ])* +-[^ ]*(x|( +xtrace))' -- ':*.sh' ':*/Dockerfile' ':*.yaml' ':*.yml' ':(exclude).pre-commit-config.yaml'"

    result = ctx.run(command, hide=True, warn=True)
    if result.return_code == 0:
        print(result.stdout.strip(), end="\n\n")
        print(color_message('error:', 'red'), 'No shell script should use "set -x"', file=sys.stderr)

        exit(1)
