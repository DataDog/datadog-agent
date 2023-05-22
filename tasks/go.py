"""
Golang related tasks go here
"""

import datetime
import glob
import os
import shutil
import textwrap
from pathlib import Path

from invoke import task
from invoke.exceptions import Exit

from .build_tags import ALL_TAGS, UNIT_TEST_TAGS, get_default_build_tags
from .licenses import get_licenses_list
from .modules import DEFAULT_MODULES, generate_dummy_package
from .utils import get_build_flags


def run_golangci_lint(ctx, targets, rtloader_root=None, build_tags=None, build="test", arch="x64", concurrency=None):
    if isinstance(targets, str):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    tags = build_tags or get_default_build_tags(build=build, arch=arch)
    if not isinstance(tags, list):
        tags = [tags]

    # Always add `test` tags while linting as test files are also linted
    tags.extend(UNIT_TEST_TAGS)

    _, _, env = get_build_flags(ctx, rtloader_root=rtloader_root)
    # we split targets to avoid going over the memory limit from circleCI
    results = []
    for target in targets:
        print(f"running golangci on {target}")
        concurrency_arg = "" if concurrency is None else f"--concurrency {concurrency}"
        tags_arg = " ".join(tags)
        result = ctx.run(
            f'golangci-lint run --timeout 20m0s {concurrency_arg} --build-tags "{tags_arg}" {target}/...',
            env=env,
            warn=True,
        )
        results.append(result)

    return results


@task
def golangci_lint(ctx, targets, rtloader_root=None, build_tags=None, build="test", arch="x64", concurrency=None):
    """
    Run golangci-lint on targets using .golangci.yml configuration.

    Example invocation:
        inv golangci-lint --targets=./pkg/collector/check,./pkg/aggregator
    """
    results = run_golangci_lint(ctx, targets, rtloader_root, build_tags, build, arch, concurrency)

    should_fail = False
    for result in results:
        # golangci exits with status 1 when it finds an issue
        if result.exited != 0:
            should_fail = True

    if should_fail:
        raise Exit(code=1)
    else:
        print("golangci-lint found no issues")


@task
def deps(ctx, verbose=False):
    """
    Setup Go dependencies
    """

    print("downloading dependencies")
    start = datetime.datetime.now()
    verbosity = ' -x' if verbose else ''
    for mod in DEFAULT_MODULES.values():
        with ctx.cd(mod.full_path()):
            ctx.run(f"go mod download{verbosity}")
    dep_done = datetime.datetime.now()
    print(f"go mod download, elapsed: {dep_done - start}")


@task
def deps_vendored(ctx, verbose=False):
    """
    Vendor Go dependencies
    """

    print("vendoring dependencies")
    start = datetime.datetime.now()
    verbosity = ' -v' if verbose else ''

    ctx.run(f"go mod vendor{verbosity}")
    ctx.run(f"go mod tidy{verbosity} -compat=1.17")

    # "go mod vendor" doesn't copy files that aren't in a package: https://github.com/golang/go/issues/26366
    # This breaks when deps include other files that are needed (eg: .java files from gomobile): https://github.com/golang/go/issues/43736
    # For this reason, we need to use a 3rd party tool to copy these files.
    # We won't need this if/when we change to non-vendored modules
    ctx.run(f'modvendor -copy="**/*.c **/*.h **/*.proto **/*.java"{verbosity}')

    # If github.com/DataDog/datadog-agent gets vendored too - nuke it
    # This may happen because of the introduction of nested modules
    if os.path.exists('vendor/github.com/DataDog/datadog-agent'):
        print("Removing vendored github.com/DataDog/datadog-agent")
        shutil.rmtree('vendor/github.com/DataDog/datadog-agent')

    dep_done = datetime.datetime.now()
    print(f"go mod vendor, elapsed: {dep_done - start}")


@task
def lint_licenses(ctx):
    """
    Checks that the LICENSE-3rdparty.csv file is up-to-date with contents of go.sum
    """
    print("Verify licenses")

    licenses = []
    file = 'LICENSE-3rdparty.csv'
    with open(file, 'r', encoding='utf-8') as f:
        next(f)
        for line in f:
            licenses.append(line.rstrip())

    new_licenses = get_licenses_list(ctx)

    removed_licenses = [ele for ele in new_licenses if ele not in licenses]
    for license in removed_licenses:
        print(f"+ {license}")

    added_licenses = [ele for ele in licenses if ele not in new_licenses]
    for license in added_licenses:
        print(f"- {license}")

    if len(removed_licenses) + len(added_licenses) > 0:
        raise Exit(
            message=textwrap.dedent(
                """\
                Licenses are not up-to-date.

                Please run 'inv generate-licenses' to update {}."""
            ).format(file),
            code=1,
        )

    print("Licenses are ok.")


@task
def generate_licenses(ctx, filename='LICENSE-3rdparty.csv', verbose=False):
    """
    Generates the LICENSE-3rdparty.csv file. Run this if `inv lint-licenses` fails.
    """
    new_licenses = get_licenses_list(ctx)

    # check that all deps have a non-"UNKNOWN" copyright and license
    unknown_licenses = False
    for line in new_licenses:
        if ',UNKNOWN' in line:
            unknown_licenses = True
            print(f"! {line}")

    if unknown_licenses:
        raise Exit(
            message=textwrap.dedent(
                """\
                At least one dependency's license or copyright could not be determined.

                Consult the dependency's source, update
                `.copyright-overrides.yml` or `.wwhrd.yml` accordingly, and run
                `inv generate-licenses` to update {}."""
            ).format(filename),
            code=1,
        )

    with open(filename, 'w') as f:
        f.write("Component,Origin,License,Copyright\n")
        for license in new_licenses:
            if verbose:
                print(license)
            f.write(f'{license}\n')
    print("licenses files generated")


@task
def generate_protobuf(ctx):
    """
    Generates protobuf definitions in pkg/proto
    """
    base = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.abspath(os.path.join(base, ".."))
    proto_root = os.path.join(repo_root, "pkg", "proto")

    print(f"nuking old definitions at: {proto_root}")
    file_list = glob.glob(os.path.join(proto_root, "pbgo", "*.go"))
    for file_path in file_list:
        try:
            os.remove(file_path)
        except OSError:
            print("Error while deleting file : ", file_path)

    with ctx.cd(repo_root):
        # protobuf defs
        print(f"generating protobuf code from: {proto_root}")

        files = []
        for path in Path(os.path.join(proto_root, "datadog")).rglob('*.proto'):
            files.append(path.as_posix())

        ctx.run(f"protoc -I{proto_root} --go_out=plugins=grpc:{repo_root} {' '.join(files)}")
        # grpc-gateway logic
        ctx.run(f"protoc -I{proto_root} --grpc-gateway_out=logtostderr=true:{repo_root} {' '.join(files)}")
        # mockgen
        pbgo_dir = os.path.join(proto_root, "pbgo")
        mockgen_out = os.path.join(proto_root, "pbgo", "mocks")
        try:
            os.mkdir(mockgen_out)
        except FileExistsError:
            print(f"{mockgen_out} folder already exists")

        ctx.run(f"mockgen -source={pbgo_dir}/api.pb.go -destination={mockgen_out}/api_mockgen.pb.go")

    # generate messagepack marshallers
    ctx.run("msgp -file pkg/proto/msgpgo/key.go -o=pkg/proto/msgpgo/key_gen.go")


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
def check_mod_tidy(ctx, test_folder="testmodule"):
    with generate_dummy_package(ctx, test_folder) as dummy_folder:
        errors_found = []
        for mod in DEFAULT_MODULES.values():
            with ctx.cd(mod.full_path()):
                ctx.run("go mod tidy -compat=1.17")
                res = ctx.run("git diff-files --exit-code go.mod go.sum", warn=True)
                if res.exited is None or res.exited > 0:
                    errors_found.append(f"go.mod or go.sum for {mod.import_path} module is out of sync")

        for mod in DEFAULT_MODULES.values():
            # Ensure that none of these modules import the datadog-agent main module.
            if mod.independent:
                ctx.run(f"go run ./internal/tools/independent-lint/independent.go --path={mod.full_path()}")

        with ctx.cd(dummy_folder):
            ctx.run("go mod tidy -compat=1.17")
            res = ctx.run("go build main.go", warn=True)
            if res.exited is None or res.exited > 0:
                errors_found.append("could not build test module importing external modules")
            if os.path.isfile(os.path.join(ctx.cwd, "main")):
                os.remove(os.path.join(ctx.cwd, "main"))

        if errors_found:
            message = "\nErrors found:\n" + "\n".join("  - " + error for error in errors_found)
            message += "\n\nRun 'inv tidy-all' to fix 'out of sync' errors."
            raise Exit(message=message)


@task
def tidy_all(ctx):
    for mod in DEFAULT_MODULES.values():
        with ctx.cd(mod.full_path()):
            ctx.run("go mod tidy -compat=1.17")


@task
def check_go_version(ctx):
    go_version_output = ctx.run('go version')
    # result is like "go version go1.19.7 linux/amd64"
    running_go_version = go_version_output.stdout.split(' ')[2]

    with open(".go-version") as f:
        dot_go_version = f.read()
        dot_go_version = dot_go_version.strip()
        if not dot_go_version.startswith("go"):
            dot_go_version = f"go{dot_go_version}"

    if dot_go_version != running_go_version:
        raise Exit(message=f"Expected {dot_go_version} (from `.go-version`), but running {running_go_version}")


@task
def go_fix(ctx, fix=None):
    if fix:
        fixarg = f" -fix {fix}"
    oslist = ["linux", "windows", "darwin"]

    for mod in DEFAULT_MODULES.values():
        with ctx.cd(mod.full_path()):
            for osname in oslist:
                tags = set(ALL_TAGS).union({osname, "ebpf_bindata"})
                ctx.run(f"go fix{fixarg} -tags {','.join(tags)} ./...")
