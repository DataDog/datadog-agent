import os

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.git import get_staged_files


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
    from tasks.hooks import check_files, selected_files

    files = selected_files("shell-check-no-set-x", list(get_staged_files(ctx, relative_path=True)))
    if check_files("shell-check-no-set-x", files):
        raise Exit(code=1)
