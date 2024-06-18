"""
Golang related tasks go here
"""

from __future__ import annotations

import glob
import os
import posixpath
import re
import shutil
import sys
import textwrap
import traceback
from collections.abc import Iterable
from pathlib import Path

from invoke import task
from invoke.exceptions import Exit

import tasks.modules
from tasks.build_tags import ALL_TAGS, UNIT_TEST_TAGS, get_default_build_tags
from tasks.libs.common.color import color_message
from tasks.libs.common.git import check_uncommitted_changes
from tasks.libs.common.utils import TimedOperationResult, get_build_flags, timed
from tasks.licenses import get_licenses_list
from tasks.modules import DEFAULT_MODULES, generate_dummy_package

GOOS_MAPPING = {
    "win32": "windows",
    "linux": "linux",
    "darwin": "darwin",
}
GOARCH_MAPPING = {
    "x64": "amd64",
    "arm64": "arm64",
}


def run_golangci_lint(
    ctx,
    module_path,
    targets,
    rtloader_root=None,
    build_tags=None,
    build="test",
    concurrency=None,
    timeout=None,
    verbose=False,
    golangci_lint_kwargs="",
    headless_mode: bool = False,
):
    if isinstance(targets, str):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    tags = build_tags or get_default_build_tags(build=build)
    if not isinstance(tags, list):
        tags = [tags]

    # Always add `test` tags while linting as test files are also linted
    tags.extend(UNIT_TEST_TAGS)

    _, _, env = get_build_flags(ctx, rtloader_root=rtloader_root, headless_mode=headless_mode)
    verbosity = "-v" if verbose else ""
    # we split targets to avoid going over the memory limit from circleCI
    results = []
    time_results = []
    for target in targets:

        def lint_module(target):
            if not headless_mode:
                print(f"running golangci on {target}")
            concurrency_arg = "" if concurrency is None else f"--concurrency {concurrency}"
            tags_arg = " ".join(sorted(set(tags)))
            timeout_arg_value = "25m0s" if not timeout else f"{timeout}m0s"
            return ctx.run(
                f'golangci-lint run {verbosity} --timeout {timeout_arg_value} {concurrency_arg} --build-tags "{tags_arg}" --path-prefix "{module_path}" {golangci_lint_kwargs} {target}/...',
                env=env,
                warn=True,
            )

        target_path = Path(module_path) / target
        result, time_result = TimedOperationResult.run(
            lint_module, target_path, 'Lint ' + target_path.as_posix(), target=target
        )

        results.append(result)
        time_results.append(time_result)

    return results, time_results


@task
def internal_deps_checker(ctx, formatFile=False):
    """
    Check that every required internal dependencies are correctly replaced
    """
    extra_params = "--formatFile true" if formatFile else ""
    for mod in DEFAULT_MODULES.values():
        ctx.run(f"go run ./internal/tools/modformatter/modformatter.go --path={mod.full_path()} {extra_params}")


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
        ctx.run(f"go mod tidy{verbosity}")

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
    with open(file, encoding='utf-8') as f:
        next(f)
        for line in f:
            licenses.append(line.rstrip())

    new_licenses = get_licenses_list(ctx, file)

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
    new_licenses = get_licenses_list(ctx, filename)

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
def check_go_mod_replaces(_):
    errors_found = set()
    for mod in DEFAULT_MODULES.values():
        go_sum = os.path.join(mod.full_path(), "go.sum")
        if not os.path.exists(go_sum):
            continue
        with open(go_sum) as f:
            for line in f:
                if "github.com/datadog/datadog-agent" in line.lower():
                    err_mod = line.split()[0]
                    errors_found.add(f"{mod.import_path}/go.mod is missing a replace for {err_mod}")

    if errors_found:
        message = "\nErrors found:\n"
        message += "\n".join("  - " + error for error in sorted(errors_found))
        message += (
            "\n\nThis task operates on go.sum files, so make sure to run `inv -e tidy` before re-running this task."
        )
        raise Exit(message=message)


@task
def check_mod_tidy(ctx, test_folder="testmodule"):
    with generate_dummy_package(ctx, test_folder) as dummy_folder:
        errors_found = []
        for mod in DEFAULT_MODULES.values():
            with ctx.cd(mod.full_path()):
                ctx.run("go mod tidy")

                files = "go.mod"
                if os.path.exists(os.path.join(mod.full_path(), "go.sum")):
                    # if the module has no dependency, no go.sum file will be created
                    files += " go.sum"

                res = ctx.run(f"git diff --exit-code {files}", warn=True)
                if res.exited is None or res.exited > 0:
                    errors_found.append(f"go.mod or go.sum for {mod.import_path} module is out of sync")

        for mod in DEFAULT_MODULES.values():
            # Ensure that none of these modules import the datadog-agent main module.
            if mod.independent:
                ctx.run(f"go run ./internal/tools/independent-lint/independent.go --path={mod.full_path()}")

        with ctx.cd(dummy_folder):
            ctx.run("go mod tidy")
            res = ctx.run("go build main.go", warn=True)
            if res.exited is None or res.exited > 0:
                errors_found.append("could not build test module importing external modules")
            if os.path.isfile(os.path.join(ctx.cwd, "main")):
                os.remove(os.path.join(ctx.cwd, "main"))

        if errors_found:
            message = "\nErrors found:\n" + "\n".join("  - " + error for error in errors_found)
            message += "\n\nRun 'inv tidy' to fix 'out of sync' errors."
            raise Exit(message=message)


@task
def tidy_all(ctx):
    sys.stderr.write(color_message('This command is deprecated, please use `tidy` instead\n', "orange"))
    sys.stderr.write("Running `tidy`...\n")
    tidy(ctx)


@task
def tidy(ctx):
    if os.name != 'nt':  # not windows
        import resource

        # Some people might face ulimit issues, so we bump it up if needed.
        # It won't change it globally, only for this process and child processes.
        # TODO: if this is working fine, let's do it during the init so all tasks can benefit from it if needed.
        current_ulimit = resource.getrlimit(resource.RLIMIT_NOFILE)
        if current_ulimit[0] < 1024:
            resource.setrlimit(resource.RLIMIT_NOFILE, (1024, current_ulimit[1]))

    # Note: It's currently faster to tidy everything than looking for exactly what we should tidy
    promises = []
    for mod in DEFAULT_MODULES.values():
        with ctx.cd(mod.full_path()):
            # https://docs.pyinvoke.org/en/stable/api/runners.html#invoke.runners.Runner.run
            promises.append(ctx.run("go mod tidy", asynchronous=True))

    for promise in promises:
        promise.join()


@task
def check_go_version(ctx):
    go_version_output = ctx.run('go version')
    # result is like "go version go1.21.11 linux/amd64"
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


def get_deps(ctx, path):
    with ctx.cd(path):
        # Might fail if no mod tidy
        deps: list[str] = ctx.run("go list -deps ./...", hide=True, warn=True).stdout.strip().splitlines()
        prefix = 'github.com/DataDog/datadog-agent/'
        deps = [
            dep.removeprefix(prefix)
            for dep in deps
            if dep.startswith(prefix) and dep != f'github.com/DataDog/datadog-agent/{path}'
        ]

        return deps


def add_replaces(ctx, path, replaces: Iterable[str]):
    repo_path = posixpath.abspath('.')
    with ctx.cd(path):
        for repo_local_path in replaces:
            if repo_local_path != path:
                # Example for pkg/util/log with path=pkg/util/cachedfetch
                # - repo_local_path: pkg/util/log
                # - online_path: github.com/DataDog/datadog-agent/pkg/util/log
                # - module_local_path: ../../../pkg/util/log
                level = os.path.abspath(path).count('/') - repo_path.count('/')
                module_local_path = ('./' if level == 0 else '../' * level) + repo_local_path
                online_path = f'github.com/DataDog/datadog-agent/{repo_local_path}'

                ctx.run(f"go mod edit -replace={online_path}={module_local_path}")


def add_go_module(path):
    """
    Add go module to modules.py
    """
    print(color_message("Updating DEFAULT_MODULES within modules.py", "blue"))
    modules_path = tasks.modules.__file__
    with open(modules_path) as f:
        modulespy = f.read()

    modulespy_regex = re.compile(r"DEFAULT_MODULES = {\n(.+?)\n}", re.DOTALL | re.MULTILINE)

    all_modules_match = modulespy_regex.search(modulespy)
    assert all_modules_match, "Could not find DEFAULT_MODULES in modules.py"
    all_modules = all_modules_match.group(1)
    all_modules = all_modules.split('\n')
    indent = ' ' * 4

    new_module = f'{indent}"{path}": GoModule("{path}", independent=True),'

    # Insert in order
    insert_line = 0
    for i, line in enumerate(all_modules):
        # This line is the start of a module (not a comment / middle of a module declaration)
        if line.startswith(f'{indent}"'):
            results = re.search(rf'{indent}"([^"]*)"', line)
            assert results, f"Could not find module name in line '{line}'"
            module = results.group(1)
            if module < path:
                insert_line = i
            else:
                assert module != path, f"Module {path} already exists within {modules_path}"

    all_modules.insert(insert_line, new_module)
    all_modules = '\n'.join(all_modules)
    with open(modules_path, 'w') as f:
        f.write(modulespy.replace(all_modules_match.group(1), all_modules))


@task
def create_module(ctx, path: str, no_verify: bool = False):
    """
    Create new go module following steps within <docs/dev/modules.md>
    - packages: Comma separated list of packages the will use the new module
    """

    path = path.rstrip('/').rstrip('\\')

    if check_uncommitted_changes(ctx):
        raise RuntimeError("There are uncomitted changes, all changes must be committed to run this command.")

    # Perform checks + save current state to restore it in case of failure
    assert not posixpath.exists(path + '/go.mod'), f"Path {path + '/go.mod'} already exists"
    is_empty = not posixpath.exists(path)

    # Get info
    with open('go.mod') as f:
        mainmod = f.read()

    goversion_regex = re.compile(r'^go +([.0-9]+)$', re.MULTILINE)
    goversion = next(goversion_regex.finditer(mainmod)).group(1)

    # Module content
    gomod = f"""
    module github.com/DataDog/datadog-agent/{path}

    go {goversion}
    """.replace('    ', '')

    try:
        # Create package
        print(color_message(f"Creating package {path}", "blue"))

        ctx.run(f"mkdir -p {path}")
        with open(f"{path}/go.mod", 'w') as f:
            f.write(gomod)

        if not is_empty:
            # 1. Update current module
            deps = get_deps(ctx, path)
            add_replaces(ctx, path, deps)
            with ctx.cd(path):
                ctx.run('go mod tidy')

            # Find and update indirect replaces within go.mod
            with open(path + '/go.mod') as f:
                mod_content = f.read()
                replaces = {
                    replace
                    for replace in re.findall(r'github.com/DataDog/datadog-agent/([^\n ]*)', mod_content)
                    if replace != path
                }
            with open(f"{path}/go.mod", 'w') as f:
                # Cancel mod tidy since it can update the go version
                f.write(gomod)
            add_replaces(ctx, path, replaces)
            with ctx.cd(path):
                ctx.run('go mod tidy')

            # 2. Update dependencies
            # Find module that must include the new module
            dependent_modules = []
            for gomod in glob.glob('./**/go.mod', recursive=True):
                gomod = Path(gomod).as_posix()
                mod_path = posixpath.dirname(gomod)
                if posixpath.abspath(mod_path) != posixpath.abspath(path):
                    deps = get_deps(ctx, mod_path)
                    if path in deps:
                        dependent_modules.append(mod_path)

            for mod in dependent_modules:
                add_replaces(ctx, mod, [path])

        # Update modules.py
        add_go_module(path)

        if not is_empty:
            # Tidy all
            print(color_message("Running tidy-all task", "bold"))
            tidy(ctx)

        if not no_verify:
            # Stage updated files since some linting tasks will require it
            print(color_message("Staging new module files", "bold"))
            ctx.run("git add --all")

            print(color_message("Linting repo", "blue"))
            print(color_message("Running internal-deps-checker task", "bold"))
            internal_deps_checker(ctx)
            print(color_message("Running check-mod-tidy task", "bold"))
            check_mod_tidy(ctx)
            print(color_message("Running check-go-mod-replaces task", "bold"))
            check_go_mod_replaces(ctx)

        print(color_message(f"Created package {path}", "green"))
    except Exception:
        traceback.print_exc()

        # Restore files if user wants to
        if sys.stdin.isatty():
            print(color_message("Failed to create module", "red"))
            if input('Do you want to restore all files ? [N/y]').strip() in 'yY':
                print(color_message("Restoring files", "blue"))

                ctx.run('git clean -f')
                ctx.run('git checkout HEAD -- .')

                raise Exit(code=1)

        print(color_message("Not removing changed files", "red"))

        raise Exit(code=1)
