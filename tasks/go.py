"""
Golang related tasks go here
"""
from __future__ import print_function

from invoke import task
from invoke.exceptions import Exit


@task
def fmt(ctx, targets=None, fail_on_fmt=False):
    """
    Run go fmt on targets. If targets are not specified,
    the value from `invoke.yaml` will be used.

    Example invokation:
        inv fmt --targets=./pkg/collector/check,./pkg/aggregator
    """
    targets_list = ctx.targets if targets is None else targets.split(',')

    # add the /... suffix to the targets
    args = ["{}/...".format(t) for t in targets_list]
    result = ctx.run("go fmt " + " ".join(args))
    if result.stdout:
        files = {x for x in result.stdout.split("\n") if x}
        print("Reformatted the following files: {}".format(','.join(files)))
        if fail_on_fmt:
            print("Code was not properly formatted, exiting...")
            raise Exit(1)
    print("go fmt found no issues")


@task
def lint(ctx, targets=None):
    """
    Run golint on targets. If targets are not specified,
    the value from `invoke.yaml` will be used.

    Example invokation:
        inv lint --targets=./pkg/collector/check,./pkg/aggregator
    """
    targets_list = ctx.targets if targets is None else targets.split(',')
    # add the /... suffix to the targets
    targets_list = ["{}/...".format(t) for t in targets_list]
    result = ctx.run("golint {}".format(' '.join(targets_list)))
    if result.stdout:
        files = {x for x in result.stdout.split('\n') if x}
        print("Linting issues found in files: {}".format(','.join(files)))
        raise Exit(1)
    print("golint found no issues")


@task
def vet(ctx, targets=None):
    """
    Run go vet on targets. If targets are not specified,
    the value from `invoke.yaml` will be used.

    Example invokation:
        inv vet --targets=./pkg/collector/check,./pkg/aggregator
    """
    targets_list = ctx.targets if targets is None else targets.split(',')

    # add the /... suffix to the targets
    args = ["{}/...".format(t) for t in targets_list]
    ctx.run("go vet " + " ".join(args))
    # go vet exits with status 1 when it finds an issue, if we're here
    # everything went smooth
    print("go vet found no issues")


@task
def deps(ctx):
    """
    Setup Go dependencies
    """
    ctx.run("go get -u github.com/golang/dep/cmd/dep")
    ctx.run("go get -u github.com/golang/lint/golint")
    ctx.run("dep ensure")
    # prune packages from /vendor, remove this hack
    # as soon as `dep prune` is merged within `dep ensure`,
    # see https://github.com/golang/dep/issues/944
    ctx.run("mv vendor/github.com/shirou/gopsutil/host/include .")
    ctx.run("dep prune")
    ctx.run("mv include vendor/github.com/shirou/gopsutil/host/")


@task
def reset(ctx):
    """
    Clean everything and remove vendoring
    """
    # go clean
    print("Executing go clean")
    ctx.run("go clean")

    # remove the bin/ folder
    print("Remove agent binary folder")
    ctx.run("rm -rf ./bin/")

    # remove vendor folder
    print("Remove vendor folder")
    ctx.run("rm -rf ./vendor")
