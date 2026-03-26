"""
Invoke tasks for workloadmeta code generation.
"""

import sys
from subprocess import check_output

from invoke import task
from invoke.exceptions import Exit


def get_git_dirty_files():
    dirty_stats = check_output(["git", "status", "--porcelain=v1", "--untracked-files=no"]).decode('utf-8')
    paths = []
    for line in dirty_stats.splitlines():
        if len(line) < 2:
            continue
        path = line[2:].split()[0]
        paths.append(path)
    return paths


@task
def go_generate(ctx):
    """Run go generate for workloadmeta entity types (regenerates intern_generated.go)."""
    ctx.run("go generate ./comp/core/workloadmeta/def/...")


@task
def go_generate_check(ctx):
    """Check that go generate has been run for workloadmeta (CI gate)."""
    previous_dirty = set(get_git_dirty_files())

    go_generate(ctx)

    sys.stdout.flush()
    sys.stderr.flush()

    dirty_files = [f for f in get_git_dirty_files() if f not in previous_dirty]
    if dirty_files:
        print("Task `dda inv workloadmeta.go-generate` resulted in dirty files, please re-run it:")
        for f in dirty_files:
            print(f"* {f}")
        raise Exit(code=1)
