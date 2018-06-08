"""
Golang related tasks go here
"""
from __future__ import print_function
import os
import sys
import json

from invoke import task
from invoke.exceptions import Exit
from .build_tags import get_default_build_tags
from .utils import pkg_config_path, get_build_flags


# List of modules to ignore when running lint on Windows platform
WIN_MODULE_WHITELIST = [
    "doflare.go",
    "iostats_pdh_windows.go",
    "iostats_wmi_windows.go",
    "pdh.go",
    "pdhhelper.go",
    "tailer_windows.go",
]

# List of paths to ignore in misspell's output
MISSPELL_IGNORED_TARGETS = [
    os.path.join("cmd", "agent", "dist", "checks", "prometheus_check"),
    os.path.join("cmd", "agent", "gui", "views", "private"),
]

# Bootstrap dependencies description
BOOTSTRAP_DEPS = "bootstrap.json"


def get_deps():
    here = os.path.abspath(os.path.dirname(__file__))
    with open(os.path.join(here, '..', BOOTSTRAP_DEPS)) as f:
        deps = json.load(f)

    return deps.get('deps', {})


@task
def fmt(ctx, targets, fail_on_fmt=False):
    """
    Run go fmt on targets.

    Example invokation:
        inv fmt --targets=./pkg/collector/check,./pkg/aggregator
    """
    if isinstance(targets, basestring):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    result = ctx.run("gofmt -l -w -s " + " ".join(targets))
    if result.stdout:
        files = {x for x in result.stdout.split("\n") if x}
        print("Reformatted the following files: {}".format(','.join(files)))
        if fail_on_fmt:
            print("Code was not properly formatted, exiting...")
            raise Exit(code=1)
    print("gofmt found no issues")


@task
def lint(ctx, targets):
    """
    Run golint on targets. If targets are not specified,
    the value from `invoke.yaml` will be used.

    Example invokation:
        inv lint --targets=./pkg/collector/check,./pkg/aggregator
    """
    if isinstance(targets, basestring):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    # add the /... suffix to the targets
    targets_list = ["{}/...".format(t) for t in targets]
    result = ctx.run("golint {}".format(' '.join(targets_list)))
    if result.stdout:
        files = []
        skipped_files = set()
        for line in (out for out in result.stdout.split('\n') if out):
            fname = os.path.basename(line.split(":")[0])
            if fname in WIN_MODULE_WHITELIST:
                skipped_files.add(fname)
                continue
            files.append(fname)

        if files:
            print("Linting issues found in {} files.".format(len(files)))
            raise Exit(code=1)

        if skipped_files:
            for skipped in skipped_files:
                print("Allowed errors in whitelisted file {}".format(skipped))

    print("golint found no issues")


@task
def vet(ctx, targets, use_embedded_libs=False):
    """
    Run go vet on targets.

    Example invokation:
        inv vet --targets=./pkg/collector/check,./pkg/aggregator
    """
    if isinstance(targets, basestring):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    # add the /... suffix to the targets
    args = ["{}/...".format(t) for t in targets]
    build_tags = get_default_build_tags()

    _, _, env = get_build_flags(ctx, use_embedded_libs=use_embedded_libs)

    ctx.run("go vet -tags \"{}\" ".format(" ".join(build_tags)) + " ".join(args), env=env)
    # go vet exits with status 1 when it finds an issue, if we're here
    # everything went smooth
    print("go vet found no issues")


@task
def cyclo(ctx, targets, limit=15):
    """
    Run gocyclo on targets.
    Use the 'limit' parameter to change the maximum cyclic complexity.

    Example invokation:
        inv cyclo --targets=./pkg/collector/check,./pkg/aggregator
    """
    if isinstance(targets, basestring):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    ctx.run("gocyclo -over {} ".format(limit) + " ".join(targets))
    # gocyclo exits with status 1 when it finds an issue, if we're here
    # everything went smooth
    print("gocyclo found no issues")


@task
def ineffassign(ctx, targets):
    """
    Run ineffassign on targets.

    Example invokation:
        inv ineffassign --targets=./pkg/collector/check,./pkg/aggregator
    """
    if isinstance(targets, basestring):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    ctx.run("ineffassign " + " ".join(targets))
    # ineffassign exits with status 1 when it finds an issue, if we're here
    # everything went smooth
    print("ineffassign found no issues")


@task
def misspell(ctx, targets):
    """
    Run misspell on targets.

    Example invokation:
        inv misspell --targets=./pkg/collector/check,./pkg/aggregator
    """
    if isinstance(targets, basestring):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    result = ctx.run("misspell " + " ".join(targets), hide=True)
    legit_misspells = []
    for found_misspell in result.stdout.split("\n"):
        if len(found_misspell.strip()) > 0:
            if not any([ignored_target in found_misspell for ignored_target in MISSPELL_IGNORED_TARGETS]):
                legit_misspells.append(found_misspell)

    if len(legit_misspells) > 0:
        print("Misspell issues found:\n" + "\n".join(legit_misspells))
        raise Exit(code=2)
    else:
        print("misspell found no issues")

@task
def deps(ctx, no_checks=False, core_dir=None, verbose=False):
    """
    Setup Go dependencies
    """
    verbosity = ' -v' if verbose else ''
    deps = get_deps()
    for tool, version in deps.iteritems():
        # download tools
        path = os.path.join(os.environ.get('GOPATH'), 'src', tool)
        if not os.path.exists(path):
            ctx.run("go get{} -d -u {}".format(verbosity, tool))

        with ctx.cd(path):
            # checkout versions
            ctx.run("git fetch")
            ctx.run("git checkout {}".format(version))

        # install tools
        ctx.run("go install{} {}".format(verbosity, tool))

    # source level deps
    ctx.run("dep ensure{}".format(verbosity))
    # make sure PSUTIL is gone on windows; the dep ensure above will vendor it
    # in because it's necessary on other platforms
    if sys.platform == 'win32':
        print("Removing PSUTIL on Windows")
        ctx.run("rd /s/q vendor\\github.com\\shirou\\gopsutil")


    if not no_checks:
        verbosity = 'v' if verbose else 'q'
        core_dir = core_dir or os.getenv('DD_CORE_DIR')

        if core_dir:
            checks_base = os.path.join(os.path.abspath(core_dir), 'datadog_checks_base')
            ctx.run('pip install -{} -e {}'.format(verbosity, checks_base))
        else:
            core_dir = os.path.join(os.getcwd(), 'vendor', 'integrations-core')
            checks_base = os.path.join(core_dir, 'datadog_checks_base')
            if not os.path.isdir(core_dir):
                ctx.run('git clone -{} https://github.com/DataDog/integrations-core {}'.format(verbosity, core_dir))
            ctx.run('pip install -{} {}'.format(verbosity, checks_base))


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
