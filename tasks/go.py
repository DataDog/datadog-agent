"""
Golang related tasks go here
"""


import datetime
import os
import sys

import yaml
from invoke import task
from invoke.exceptions import Exit

from .build_tags import get_default_build_tags
from .utils import get_build_flags

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
    "routes.go",  # pkg/util/winutil/iphelper
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
GO_GENERATE_TARGETS = ["./pkg/status", "./cmd/agent/gui"]


@task
def fmt(ctx, targets, fail_on_fmt=False):
    """
    Run go fmt on targets.

    Example invokation:
        inv fmt --targets=./pkg/collector/check,./pkg/aggregator
    """
    if isinstance(targets, str):
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
    if isinstance(targets, str):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    # add the /... suffix to the targets
    targets_list = ["{}/...".format(t) for t in targets]
    result = ctx.run("go run golang.org/x/lint/golint {}".format(' '.join(targets_list)))
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
def vet(ctx, targets, rtloader_root=None, build_tags=None, arch="x64"):
    """
    Run go vet on targets.

    Example invokation:
        inv vet --targets=./pkg/collector/check,./pkg/aggregator
    """
    if isinstance(targets, str):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    # add the /... suffix to the targets
    args = ["{}/...".format(t) for t in targets]
    tags = build_tags or get_default_build_tags(build="test", arch=arch)
    tags.append("dovet")

    _, _, env = get_build_flags(ctx, rtloader_root=rtloader_root)
    env["CGO_ENABLED"] = "1"

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
    if isinstance(targets, str):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    ctx.run("go run github.com/fzipp/gocyclo -over {} ".format(limit) + " ".join(targets))
    # gocyclo exits with status 1 when it finds an issue, if we're here
    # everything went smooth
    print("gocyclo found no issues")


@task
def golangci_lint(ctx, targets, rtloader_root=None, build_tags=None, arch="x64"):
    """
    Run golangci-lint on targets using .golangci.yml configuration.

    Example invocation:
        inv golangci_lint --targets=./pkg/collector/check,./pkg/aggregator
    """
    if isinstance(targets, str):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    tags = build_tags or get_default_build_tags(build="test", arch=arch)
    _, _, env = get_build_flags(ctx, rtloader_root=rtloader_root)
    # we split targets to avoid going over the memory limit from circleCI
    for target in targets:
        print("running golangci on {}".format(target))
        ctx.run(
            "go run github.com/golangci/golangci-lint/cmd/golangci-lint run --timeout 10m0s -c .golangci.yml --build-tags '{}' {}".format(
                " ".join(tags), "{}/...".format(target)
            ),
            env=env,
        )

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
    if isinstance(targets, str):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    ctx.run("go run github.com/gordonklaus/ineffassign " + " ".join(target + "/..." for target in targets))
    # ineffassign exits with status 1 when it finds an issue, if we're here
    # everything went smooth
    print("ineffassign found no issues")


@task
def staticcheck(ctx, targets, build_tags=None, arch="x64"):
    """
    Run staticcheck on targets.

    Example invokation:
        inv statickcheck --targets=./pkg/collector/check,./pkg/aggregator
    """
    if isinstance(targets, str):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    # staticcheck checks recursively only if path is in "path/..." format
    pkgs = [sub + "/..." for sub in targets]

    tags = build_tags or get_default_build_tags(build="test", arch=arch)
    # these two don't play well with static checking
    tags.remove("python")
    tags.remove("jmx")

    ctx.run("go run honnef.co/go/tools/cmd/staticcheck -checks=SA1027 -tags=" + ",".join(tags) + " " + " ".join(pkgs))
    # staticcheck exits with status 1 when it finds an issue, if we're here
    # everything went smooth
    print("staticcheck found no issues")


@task
def misspell(ctx, targets):
    """
    Run misspell on targets.

    Example invokation:
        inv misspell --targets=./pkg/collector/check,./pkg/aggregator
    """
    if isinstance(targets, str):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    result = ctx.run("go run github.com/client9/misspell/cmd/misspell " + " ".join(targets), hide=True)
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
def deps(ctx, verbose=False):
    """
    Setup Go dependencies
    """

    print("vendoring dependencies")
    start = datetime.datetime.now()
    verbosity = ' -v' if verbose else ''

    ctx.run("go mod vendor{}".format(verbosity))
    ctx.run("go mod tidy{}".format(verbosity))

    # "go mod vendor" doesn't copy files that aren't in a package: https://github.com/golang/go/issues/26366
    # This breaks when deps include other files that are needed (eg: .java files from gomobile): https://github.com/golang/go/issues/43736
    # For this reason, we need to use a 3rd party tool to copy these files.
    # We won't need this if/when we change to non-vendored modules
    ctx.run('go run github.com/goware/modvendor -copy="**/*.c **/*.h **/*.proto **/*.java"{}'.format(verbosity))

    # delete gopsutil on windows, it get vendored because it's necessary on other platforms
    if sys.platform == 'win32':
        print("Removing PSUTIL on Windows")
        ctx.run("rd /s/q vendor\\github.com\\shirou\\gopsutil")

    dep_done = datetime.datetime.now()
    print("go mod vendor, elapsed: {}".format(dep_done - start))


@task
def lint_licenses(ctx, verbose=False):
    """
    Checks that the LICENSE-3rdparty.csv file is up-to-date with contents of go.sum
    """
    print("Verify licenses")

    licenses = []
    file = 'LICENSE-3rdparty.csv'
    with open(file, 'r') as f:
        next(f)
        for line in f:
            licenses.append(line.rstrip())

    new_licenses = get_licenses_list(ctx)

    if sys.platform == 'win32':
        # ignore some licenses because we remove
        # the deps in a hack for windows
        ignore_licenses = ['github.com/shirou/gopsutil']
        to_removed = []
        for ignore in ignore_licenses:
            for license in licenses:
                if ignore in license:
                    if verbose:
                        print("[hack-windows] ignore: {}".format(license))
                    to_removed.append(license)
        licenses = [x for x in licenses if x not in to_removed]

    removed_licenses = [ele for ele in new_licenses if ele not in licenses]
    for license in removed_licenses:
        print("+ {}".format(license))

    added_licenses = [ele for ele in licenses if ele not in new_licenses]
    for license in added_licenses:
        print("- {}".format(license))

    if len(removed_licenses) + len(added_licenses) > 0:
        print("licenses are not up-to-date")
        raise Exit(code=1)

    print("licenses ok")


@task
def generate_licenses(ctx, filename='LICENSE-3rdparty.csv', verbose=False):
    """
    Generates that the LICENSE-3rdparty.csv file is up-to-date with contents of go.sum
    """
    with open(filename, 'w') as f:
        f.write("Component,Origin,License\n")
        for license in get_licenses_list(ctx):
            if verbose:
                print(license)
            f.write('{}\n'.format(license))
    print("licenses files generated")


# FIXME: This doesn't include licenses for non-go dependencies, like the javascript libs we use for the web gui
def get_licenses_list(ctx):
    # Read the list of packages to exclude from the list from wwhrd's
    exceptions_wildcard = []
    exceptions = []
    with open('.wwhrd.yml') as wwhrd_conf_yml:
        wwhrd_conf = yaml.safe_load(wwhrd_conf_yml)
        for pkg in wwhrd_conf['exceptions']:
            if pkg.endswith("/..."):
                # TODO(python3.9): use removesuffix
                exceptions_wildcard.append(pkg[: -len("/...")])
            else:
                exceptions.append(pkg)

    def is_excluded(pkg):
        if package in exceptions:
            return True
        for exception in exceptions_wildcard:
            if package.startswith(exception):
                return True
        return False

    # Parse the output of wwhrd to generate the list
    result = ctx.run('go run github.com/frapposelli/wwhrd list --no-color', hide='err')
    licenses = []
    if result.stderr:
        for line in result.stderr.split("\n"):
            index = line.find('msg="Found License"')
            if index == -1:
                continue
            license = ""
            package = ""
            for val in line[index + len('msg="Found License"') :].split(" "):
                if val.startswith('license='):
                    license = val[len('license=') :]
                elif val.startswith('package='):
                    package = val[len('package=') :]
                    if is_excluded(package):
                        print("Skipping {} ({}) excluded in .wwhrd.yml".format(package, license))
                    else:
                        licenses.append("core,\"{}\",{}".format(package, license))
    licenses.sort()
    return licenses


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
    ctx.run("go generate -mod=vendor " + " ".join(GO_GENERATE_TARGETS))
    print("go generate ran successfully")
