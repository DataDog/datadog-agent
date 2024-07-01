import os
import re
import sys

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import color_message
from tasks.libs.common.git import get_staged_files
from tasks.tools import clang_format
from tasks.tools.clang_format import ExitStatus

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
def check_winclang_format(ctx):
    if os.name != 'nt':  # Don't run on Linux
        return

    def find_clang_format(search_dirs):
        for search_dir in search_dirs:
            for root, _, files in os.walk(search_dir):
                for basename in files:
                    if basename == 'clang-format.exe':
                        return os.path.join(root, basename)

    clang_format_path = os.environ.get('CLANG_FORMAT_PATH')
    if clang_format_path is None:
        search_dirs = ['C:/Program Files/Microsoft Visual Studio', 'C:/Program Files (x86)/Microsoft Visual Studio']
        clang_format_path = find_clang_format(search_dirs)

    print(clang_format_path)

    ctx.run(f'"{clang_format_path}" --dry-run --Werror {",".join(get_staged_files(ctx))}')


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
                        f"{color_message(file, 'magenta')}:{color_message(str(nb + 1), 'green')}: {color_message(line.strip(), 'red')}"
                    )

    if errors:
        for error in errors:
            print(error, file=sys.stderr)
        print(color_message('error:', 'red'), 'No shell script should use "set -x"', file=sys.stderr)
        raise Exit(code=1)


@task
def check_clang_format(ctx):
    files = [
        file
        for file in get_staged_files(ctx)
        if (
            re.match(r"^pkg/(ebpf|network|security)/.*\.(c|h)$", file)
            and not re.match(
                "^pkg/ebpf/(c/bpf_endian|c/bpf_helpers|compiler/clang-stdarg).h$",
                file,
            )
        )
    ]

    if files:
        res = clang_format.run(files)
        if res != ExitStatus.SUCCESS:
            raise Exit(code=res)
