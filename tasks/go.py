"""
Golang related tasks go here
"""
from __future__ import print_function
import datetime
import os
import shutil
import sys

from invoke import task
from invoke.exceptions import Exit
from .build_tags import get_default_build_tags
from .utils import get_build_flags
from .bootstrap import get_deps, process_deps

#We use `basestring` in the code for compat with python2 unicode strings.
#This makes the same code work in python3 as well.
try:
    basestring
except NameError:
    basestring = str

# List of modules to ignore when running lint
MODULE_WHITELIST = [
    # Windows
    "doflare.go",
    "iostats_pdh_windows.go",
    "iostats_wmi_windows.go",
    "pdh.go",
    "pdh_amd64.go",
    "pdh_386.go",
    "pdhhelper.go",
    "shutil.go",
    "tailer_windows.go",
    "winsec.go",
    "allprocesses_windows.go",
    "allprocesses_windows_test.go",
    "adapters.go",  # pkg/util/winutil/iphelper
    "routes.go",    # pkg/util/winutil/iphelper
    # All
    "agent.pb.go",
    "bbscache_test.go",
]

# List of paths to ignore in misspell's output
MISSPELL_IGNORED_TARGETS = [
    os.path.join("cmd", "agent", "dist", "checks", "prometheus_check"),
    os.path.join("cmd", "agent", "gui", "views", "private"),
    os.path.join("pkg", "collector", "corechecks", "system", "testfiles"),
    os.path.join("pkg", "ebpf", "testdata"),
]

# Packages that need go:generate
GO_GENERATE_TARGETS = [
    "./pkg/status"
]

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
            if fname in MODULE_WHITELIST:
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
def vet(ctx, targets, rtloader_root=None, build_tags=None):
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
    tags = build_tags or get_default_build_tags()
    tags.append("dovet")

    _, _, env = get_build_flags(ctx, rtloader_root=rtloader_root)

    ctx.run("go vet -tags \"{}\" ".format(" ".join(tags)) + " ".join(args), env=env)
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
def golangci_lint(ctx, targets, rtloader_root=None, build_tags=None):
    """
    Run golangci-lint on targets using .golangci.yml configuration.

    Example invocation:
        inv golangci_lint --targets=./pkg/collector/check,./pkg/aggregator
    """
    if isinstance(targets, basestring):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    tags = build_tags or get_default_build_tags()
    _, _, env = get_build_flags(ctx, rtloader_root=rtloader_root)
    # we split targets to avoid going over the memory limit from circleCI
    for target in targets:
        print("running golangci on {}".format(target))
        ctx.run("golangci-lint run -c .golangci.yml --build-tags '{}' {}".format(" ".join(tags), "{}/...".format(target)), env=env)

    # golangci exits with status 1 when it finds an issue, if we're here
    # everything went smooth
    print("golangci-lint found no issues")

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
def deps(ctx, no_checks=False, core_dir=None, verbose=False, android=False, dep_vendor_only=False, no_dep_ensure=False):
    """
    Setup Go dependencies
    """
    deps = get_deps('deps')
    order = deps.get("order", deps.keys())
    for dependency in order:
        tool = deps.get(dependency)
        if not tool:
            print("Malformed bootstrap JSON, dependency {} not found". format(dependency))
            raise Exit(code=1)
        print("processing checkout tool {}".format(dependency))
        process_deps(ctx, dependency, tool.get('version'), tool.get('type'), 'checkout', verbose=verbose)

    order = deps.get("order", deps.keys())
    for dependency in order:
        tool = deps.get(dependency)
        if tool.get('install', True):
            print("processing get tool {}".format(dependency))
            process_deps(ctx, dependency, tool.get('version'), tool.get('type'), 'install', cmd=tool.get('cmd'), verbose=verbose)

    if android:
        ndkhome = os.environ.get('ANDROID_NDK_HOME')
        if not ndkhome:
            print("set ANDROID_NDK_HOME to build android")
            raise Exit(code=1)

        cmd = "gomobile init -ndk {}". format(ndkhome)
        print("gomobile command {}". format(cmd))
        ctx.run(cmd)

    if not no_dep_ensure:
        # source level deps
        print("calling dep ensure")
        start = datetime.datetime.now()
        verbosity = ' -v' if verbose else ''
        vendor_only = ' --vendor-only' if dep_vendor_only else ''
        ctx.run("dep ensure{}{}".format(verbosity, vendor_only))
        dep_done = datetime.datetime.now()

        # If github.com/DataDog/datadog-agent gets vendored too - nuke it
        #
        # This may happen as a result of having to introduce DEPPROJECTROOT
        # in our builders to get around a known-issue with go dep, and the
        # strange GOPATH situation in our builders.
        #
        # This is only a workaround, we should eliminate the need to resort
        # to DEPPROJECTROOT.
        if os.path.exists('vendor/github.com/DataDog/datadog-agent'):
            print("Removing vendored github.com/DataDog/datadog-agent")
            shutil.rmtree('vendor/github.com/DataDog/datadog-agent')

        # make sure PSUTIL is gone on windows; the dep ensure above will vendor it
        # in because it's necessary on other platforms
        if not android and sys.platform == 'win32':
            print("Removing PSUTIL on Windows")
            ctx.run("rd /s/q vendor\\github.com\\shirou\\gopsutil")

        # Make sure that golang.org/x/mobile is deleted.  It will get vendored in
        # because we use it, and there's no way to exclude; however, we must use
        # the version from $GOPATH
        if os.path.exists('vendor/golang.org/x/mobile'):
            print("Removing vendored golang.org/x/mobile")
            shutil.rmtree('vendor/golang.org/x/mobile')

    checks_start = datetime.datetime.now()
    if not no_checks:
        verbosity = 'v' if verbose else 'q'
        core_dir = core_dir or os.getenv('DD_CORE_DIR')

        if core_dir:
            checks_base = os.path.join(os.path.abspath(core_dir), 'datadog_checks_base')
            ctx.run('pip install -{} -e "{}[deps]"'.format(verbosity, checks_base))
        else:
            core_dir = os.path.join(os.getcwd(), 'vendor', 'integrations-core')
            checks_base = os.path.join(core_dir, 'datadog_checks_base')
            if not os.path.isdir(core_dir):
                ctx.run('git clone -{} https://github.com/DataDog/integrations-core {}'.format(verbosity, core_dir))
            ctx.run('pip install -{} "{}[deps]"'.format(verbosity, checks_base))
    checks_done = datetime.datetime.now()

    if not no_dep_ensure:
        print("dep ensure, elapsed:    {}".format(dep_done - start))
    print("checks install elapsed: {}".format(checks_done - checks_start))

@task
def lint_licenses(ctx):
    """
    Checks that the LICENSE-3rdparty.csv file is up-to-date with contents of Gopkg.lock
    """
    import csv
    import toml

    # non-go deps that should be listed in the license file, but not in Gopkg.lock
    NON_GO_DEPS = set([
        'github.com/codemirror/CodeMirror',
        'github.com/FortAwesome/Font-Awesome',
        'github.com/jquery/jquery',
    ])

    # Read all dep names from Gopkg.lock
    go_deps = set()
    gopkg_lock = toml.load('Gopkg.lock')
    for project in gopkg_lock['projects']:
        # FIXME: this conditional is necessary because of the issue introduced by DEPPROJECTROOT
        # (for some reason `datadog-agent` gets added to Gopkg.lock and vendored), see comment in `deps`
        # task for details
        if project['name'] != 'github.com/DataDog/datadog-agent':
            go_deps.add(project['name'])

    deps = go_deps | NON_GO_DEPS

    # Read all dep names listed in LICENSE-3rdparty
    licenses = csv.DictReader(open('LICENSE-3rdparty.csv'))
    license_deps = set()
    for entry in licenses:
        if len(entry['License']) == 0:
            raise Exit(message="LICENSE-3rdparty entry '{}' has an empty license".format(entry['Origin']), code=1)
        license_deps.add(entry['Origin'])

    if deps != license_deps:
        raise Exit(message="LICENSE-3rdparty.csv is outdated compared to deps listed in Gopkg.lock:\n" +
                           "missing from LICENSE-3rdparty.csv: {}\n".format(deps - license_deps) +
                           "listed in LICENSE-3rdparty.csv but not in Gopkg.lock: {}".format(license_deps - deps),
                   code=1)

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

@task
def generate(ctx):
    """
    Run go generate required package
    """
    ctx.run("go generate " + " ".join(GO_GENERATE_TARGETS))
    print("go generate ran successfully")
