"""
Golang related tasks go here
"""

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
from .utils import get_build_flags, timed

GOOS_MAPPING = {
    "win32": "windows",
    "linux": "linux",
    "darwin": "darwin",
}
GOARCH_MAPPING = {
    "x64": "amd64",
    "x86": "386",
    "arm64": "arm64",
}


def run_golangci_lint(
    ctx,
    targets,
    rtloader_root=None,
    build_tags=None,
    build="test",
    arch="x64",
    concurrency=None,
    timeout=None,
    verbose=False,
    golangci_lint_kwargs="",
):
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
    verbosity = "-v" if verbose else ""
    # we split targets to avoid going over the memory limit from circleCI
    results = []
    for target in targets:
        print(f"running golangci on {target}")
        concurrency_arg = "" if concurrency is None else f"--concurrency {concurrency}"
        tags_arg = " ".join(sorted(set(tags)))
        timeout_arg_value = "25m0s" if not timeout else f"{timeout}m0s"
        result = ctx.run(
            f'golangci-lint run {verbosity} --timeout {timeout_arg_value} {concurrency_arg} --build-tags "{tags_arg}" {golangci_lint_kwargs} {target}/...',
            env=env,
            warn=True,
        )
        results.append(result)

    return results


@task
def golangci_lint(
    ctx, targets, rtloader_root=None, build_tags=None, build="test", arch="x64", concurrency=None  # noqa: U100
):
    """
    Run golangci-lint on targets using .golangci.yml configuration.

    Example invocation:
        inv golangci-lint --targets=./pkg/collector/check,./pkg/aggregator
    DEPRECATED
    Please use inv lint-go instead
    """
    print("WARNING: golangci-lint task is deprecated, please migrate to lint-go task")
    raise Exit(code=1)


@task
def deps(ctx, verbose=False):
    """
    Setup Go dependencies
    """

    print("downloading dependencies")
    with timed("go mod download"):
        verbosity = ' -x' if verbose else ''
        for mod in DEFAULT_MODULES.values():
            with ctx.cd(mod.full_path()):
                ctx.run(f"go mod download{verbosity}")


@task
def deps_vendored(ctx, verbose=False):
    """
    Vendor Go dependencies
    """

    print("vendoring dependencies")
    with timed("go mod vendor"):
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

    We must build the packages one at a time due to protoc-gen-go limitations
    """

    # Key: path, Value: grpc_gateway, inject_tags
    PROTO_PKGS = {
        'model/v1': (False, False),
        'remoteconfig': (False, False),
        'api/v1': (True, False),
        'trace': (False, True),
        'process': (False, False),
        'workloadmeta': (False, False),
        'languagedetection': (False, False),
    }

    # maybe put this in a separate function
    PKG_PLUGINS = {
        'trace': '--go-vtproto_out=',
    }

    PKG_CLI_EXTRAS = {
        'trace': '--go-vtproto_opt=features=marshal+unmarshal+size',
    }

    # protoc-go-inject-tag targets
    inject_tag_targets = {
        'trace': ['span.pb.go', 'stats.pb.go', 'tracer_payload.pb.go', 'agent_payload.pb.go'],
    }

    # msgp targets (file, io)
    msgp_targets = {
        'trace': [
            ('trace.go', False),
            ('span.pb.go', False),
            ('stats.pb.go', True),
            ('tracer_payload.pb.go', False),
            ('agent_payload.pb.go', False),
        ],
        'core': [('remoteconfig.pb.go', False)],
    }

    # msgp patches key is `pkg` : (patch, destination)
    #     if `destination` is `None` diff will target inherent patch files
    msgp_patches = {
        'trace': [
            ('0001-Customize-msgpack-parsing.patch', '-p4'),
            ('0002-Make-nil-map-deserialization-retrocompatible.patch', '-p4'),
            ('0003-pkg-trace-traceutil-credit-card-obfuscation-9213.patch', '-p4'),
        ],
    }

    base = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.abspath(os.path.join(base, ".."))
    proto_root = os.path.join(repo_root, "pkg", "proto")
    protodep_root = os.path.join(proto_root, "protodep")

    print(f"nuking old definitions at: {proto_root}")
    file_list = glob.glob(os.path.join(proto_root, "pbgo", "*.pb.go"))
    for file_path in file_list:
        try:
            os.remove(file_path)
        except OSError:
            print("Error while deleting file : ", file_path)

    # also cleanup gateway generated files
    file_list = glob.glob(os.path.join(proto_root, "pbgo", "*.pb.gw.go"))
    for file_path in file_list:
        try:
            os.remove(file_path)
        except OSError:
            print("Error while deleting file : ", file_path)

    with ctx.cd(repo_root):
        # protobuf defs
        print(f"generating protobuf code from: {proto_root}")

        for pkg, (grpc_gateway, inject_tags) in PROTO_PKGS.items():
            files = []
            pkg_root = os.path.join(proto_root, "datadog", pkg).rstrip(os.sep)
            pkg_root_level = pkg_root.count(os.sep)
            for path in Path(pkg_root).rglob('*.proto'):
                if path.as_posix().count(os.sep) == pkg_root_level + 1:
                    files.append(path.as_posix())

            targets = ' '.join(files)

            # output_generator could potentially change for some packages
            # so keep it in a variable for sanity.
            output_generator = "--go_out=plugins=grpc:"
            cli_extras = ''
            ctx.run(f"protoc -I{proto_root} -I{protodep_root} {output_generator}{repo_root} {cli_extras} {targets}")

            if pkg in PKG_PLUGINS:
                output_generator = PKG_PLUGINS[pkg]

                if pkg in PKG_CLI_EXTRAS:
                    cli_extras = PKG_CLI_EXTRAS[pkg]

                ctx.run(f"protoc -I{proto_root} -I{protodep_root} {output_generator}{repo_root} {cli_extras} {targets}")

            if inject_tags:
                inject_path = os.path.join(proto_root, "pbgo", pkg)
                # inject_tags logic
                for target in inject_tag_targets[pkg]:
                    ctx.run(f"protoc-go-inject-tag -input={os.path.join(inject_path, target)}")

            if grpc_gateway:
                # grpc-gateway logic
                ctx.run(
                    f"protoc -I{proto_root} -I{protodep_root} --grpc-gateway_out=logtostderr=true:{repo_root} {targets}"
                )

        # mockgen
        pbgo_dir = os.path.join(proto_root, "pbgo")
        mockgen_out = os.path.join(proto_root, "pbgo", "mocks")
        try:
            os.mkdir(mockgen_out)
        except FileExistsError:
            print(f"{mockgen_out} folder already exists")

        # TODO: this should be parametrized
        ctx.run(f"mockgen -source={pbgo_dir}/core/api.pb.go -destination={mockgen_out}/core/api_mockgen.pb.go")

    # generate messagepack marshallers
    for pkg, files in msgp_targets.items():
        for src, io_gen in files:
            dst = os.path.splitext(os.path.basename(src))[0]  # .go
            dst = os.path.splitext(dst)[0]  # .pb
            ctx.run(f"msgp -file {pbgo_dir}/{pkg}/{src} -o={pbgo_dir}/{pkg}/{dst}_gen.go -io={io_gen}")

    # apply msgp patches
    for pkg, patches in msgp_patches.items():
        for patch in patches:
            patch_file = os.path.join(proto_root, "patches", patch[0])
            switches = patch[1] if patch[1] else ''
            ctx.run(f"git apply {switches} --unsafe-paths --directory='{pbgo_dir}/{pkg}' {patch_file}")


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
    # result is like "go version go1.20.8 linux/amd64"
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
