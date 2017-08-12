"""
Golang related tasks go here
"""
from __future__ import print_function

from invoke import task
from invoke.exceptions import Exit


@task
def fmt(ctx, targets=None, fail_on_mod=False):
    """
    Run go fmt on targets.
    """
    if targets is None:
        targets = " ".join("./{}/...".format(x) for x in ctx.targets)

    result = ctx.run("go fmt {}".format(targets))
    if result.stdout:
        files = {x for x in result.stdout.split('\n') if x}
        print("Reformatted the following files: {}".format(','.join(files)))
        if fail_on_mod:
            print("Code was not properly formatted, exiting...")
            raise Exit(1)
    print("go fmt found no issues")


@task
def lint(ctx, targets=None):
    """
    Run golint on targets.
    """
    if targets is None:
        targets = " ".join("./{}/...".format(x) for x in ctx.targets)

    result = ctx.run("golint {}".format(targets))
    if result.stdout:
        files = {x for x in result.stdout.split('\n') if x}
        print("Linting issues found in files: {}".format(','.join(files)))
        raise Exit(1)
    print("golint found no issues")


@task
def vet(ctx, targets=None):
    """
    Run go vet on targets.
    """
    if targets is None:
        targets = " ".join("./{}/...".format(x) for x in ctx.targets)

    ctx.run("go vet {}".format(targets, hide=True))
    # go vet exits with status 1 when it finds an issue, if we're here
    # everything went smooth
    print("go vet found no issues")
