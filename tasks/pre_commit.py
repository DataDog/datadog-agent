import re
import sys

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import color_message
from tasks.libs.common.git import get_staged_files

DEFAULT_PRE_COMMIT_CONFIG = ".pre-commit-config.yaml"
DEVA_PRE_COMMIT_CONFIG = ".pre-commit-config-deva.yaml"


def update_pyapp_file() -> str:
    with open(DEFAULT_PRE_COMMIT_CONFIG) as file:
        data = file.read()
        for cmd in ('invoke', 'inv'):
            data = data.replace(f"entry: '{cmd}", "entry: 'deva")
    with open(DEVA_PRE_COMMIT_CONFIG, 'w') as file:
        file.write(data)
    return DEVA_PRE_COMMIT_CONFIG


@task
def check_set_x(ctx):
    # Select only relevant files
    files = [
        path
        for path in get_staged_files(ctx)
        if path.endswith(".sh")
        or path.endswith("Dockerfile")
        or path.endswith(".yml")
        or (path.endswith(".yaml") and not path.startswith(".pre-commit-config"))
    ]

    errors = []
    for file in files:
        with open(file) as f:
            for nb, line in enumerate(f):
                if re.search(r"set( +-[^ ])* +-[^ ]*(x|( +xtrace))", line):
                    errors.append(
                        f"{color_message(file, 'magenta')}:{color_message(nb + 1, 'green')}: {color_message(line.strip(), 'red')}"
                    )

    if errors:
        for error in errors:
            print(error, file=sys.stderr)
        print(color_message('error:', 'red'), 'No shell script should use "set -x"', file=sys.stderr)
        raise Exit(code=1)
