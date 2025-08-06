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
from collections import defaultdict
from collections.abc import Iterable
from pathlib import Path

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import ALL_TAGS, UNIT_TEST_TAGS, get_default_build_tags
from tasks.libs.common.color import color_message
from tasks.libs.common.git import check_uncommitted_changes
from tasks.libs.common.go import download_go_dependencies
from tasks.libs.common.gomodules import Configuration, GoModule, get_default_modules
from tasks.libs.common.user_interactions import yes_no_question
from tasks.libs.common.utils import TimedOperationResult, get_build_flags, timed
from tasks.licenses import get_licenses_list
from tasks.modules import generate_dummy_package

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
    base_path,
    targets,
    rtloader_root=None,
    build_tags=None,
    build="test",
    concurrency=None,
    timeout=None,
    verbose=False,
    golangci_lint_kwargs="",
    headless_mode: bool = False,
    recursive: bool = True,
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
    concurrency_arg = "" if concurrency is None else f"--concurrency {concurrency}"
    tags_arg = " ".join(sorted(set(tags)))
    timeout_arg_value = "25m0s" if not timeout else f"{timeout}m0s"
    # Compose the targets string for the command
    targets_str = " ".join(f"{target}{'/...' if recursive else ''}" for target in targets)
    cmd = (
        f'golangci-lint run {verbosity} --timeout {timeout_arg_value} {concurrency_arg} '
        f'--build-tags "{tags_arg}" --path-prefix "{base_path}" {golangci_lint_kwargs} {targets_str}'
    )
    if not headless_mode:
        print(f"running golangci-lint on: {targets_str}")
    result, time_result = TimedOperationResult.run(
        lambda: ctx.run(cmd, env=env, warn=True), "golangci-lint", f"Lint {targets_str}"
    )
    return [result], [time_result]


@task
def internal_deps_checker(ctx, formatFile=False):
    """
    Check that every required internal dependencies are correctly replaced
    """
    repo_path = os.getcwd()
    extra_params = "--formatFile true" if formatFile else ""
    for mod in get_default_modules().values():
        ctx.run(
            f"go run ./internal/tools/modformatter/modformatter.go --path={mod.full_path()} --repoPath={repo_path} {extra_params}"
        )


@task
def deps(ctx, verbose=False):
    """
    Setup Go dependencies
    """
    paths = [mod.full_path() for mod in get_default_modules().values()]
    download_go_dependencies(ctx, paths, verbose=verbose)


@task
def deps_vendored(ctx, verbose=False):
    """
    Vendor Go dependencies
    """

    print("vendoring dependencies")
    with timed("go mod vendor"):
        verbosity = ' -v' if verbose else ''

        # We need to set GOWORK=off to avoid the go command to use the go.work directory
        # It is needed because it does not work very well with vendoring, we should no longer need it when we get rid of vendoring. ADXR-766
        ctx.run(f"go mod vendor{verbosity}", env={"GOWORK": "off"})
        ctx.run(f"go mod tidy{verbosity}", env={"GOWORK": "off"})

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

                Please run 'dda inv generate-licenses' to update {}."""
            ).format(file),
            code=1,
        )

    print("Licenses are ok.")


@task
def generate_licenses(ctx, filename='LICENSE-3rdparty.csv', verbose=False):
    """
    Generates the LICENSE-3rdparty.csv file. Run this if `dda inv lint-licenses` fails.
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
def check_go_mod_replaces(ctx, fix=False):
    errors_found = set()
    for mod in get_default_modules().values():
        with ctx.cd(mod.path):
            go_sum = os.path.join(mod.full_path(), "go.sum")
            if not os.path.exists(go_sum):
                continue
            with open(go_sum) as f:
                for line in f:
                    if "github.com/datadog/datadog-agent" in line.lower():
                        err_mod = line.split()[0]
                        if (Path(err_mod.removeprefix("github.com/DataDog/datadog-agent/")) / "go.mod").exists():
                            if fix:
                                relative_path = os.path.relpath(err_mod, mod.import_path)
                                ctx.run(f"go mod edit -replace {err_mod}={relative_path}")
                            else:
                                errors_found.add(f"{mod.import_path}/go.mod is missing a replace for {err_mod}")

    if errors_found:
        message = "\nErrors found:\n"
        message += "\n".join("  - " + error for error in sorted(errors_found))
        message += "\n\n Run `dda inv check-go-mod-replaces --fix` to fix the errors.\n"
        message += "This task operates on go.sum files, so make sure to run `dda inv -e tidy` after fixing the errors."
        raise Exit(message=message)


def raise_if_errors(errors_found, suggestion_msg=None):
    if errors_found:
        message = "\nErrors found:\n" + "\n".join("  - " + error for error in errors_found)
        if suggestion_msg:
            message += f"\n\n{suggestion_msg}"
        raise Exit(message=message)


def check_valid_mods(ctx):
    errors_found = []
    for mod in get_default_modules().values():
        pattern = os.path.join(mod.full_path(), '*.go')
        if not glob.glob(pattern):
            errors_found.append(f"module {mod.import_path} does not contain *.go source files, so it is not a package")
    raise_if_errors(errors_found)
    return bool(errors_found)


@task
def check_mod_tidy(ctx, test_folder="testmodule"):
    check_valid_mods(ctx)
    with generate_dummy_package(ctx, test_folder) as dummy_folder:
        errors_found = []
        ctx.run("go work sync")
        res = ctx.run("git diff --exit-code **/go.mod **/go.sum", warn=True)
        if res.exited is None or res.exited > 0:
            errors_found.append("modules dependencies are out of sync, please run go work sync")

        for mod in get_default_modules().values():
            with ctx.cd(mod.full_path()):
                ctx.run("go mod tidy")

                files = "go.mod"
                if os.path.exists(os.path.join(mod.full_path(), "go.sum")):
                    # if the module has no dependency, no go.sum file will be created
                    files += " go.sum"

                res = ctx.run(f"git diff --exit-code {files}", warn=True)
                if res.exited is None or res.exited > 0:
                    errors_found.append(f"go.mod or go.sum for {mod.import_path} module is out of sync")

        for mod in get_default_modules().values():
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

        raise_if_errors(errors_found, "Run 'dda inv tidy' to fix 'out of sync' errors.")


@task
def tidy_all(ctx):
    sys.stderr.write(color_message('This command is deprecated, please use `tidy` instead\n', "orange"))
    sys.stderr.write("Running `tidy`...\n")
    tidy(ctx)


@task
def tidy(ctx, verbose: bool = False):
    check_valid_mods(ctx)

    ctx.run("go work sync")

    if os.name != 'nt':  # not windows
        import resource

        # Some people might face ulimit issues, so we bump it up if needed.
        # It won't change it globally, only for this process and child processes.
        # TODO: if this is working fine, let's do it during the init so all tasks can benefit from it if needed.
        current_ulimit = resource.getrlimit(resource.RLIMIT_NOFILE)
        if current_ulimit[0] < 1024:
            resource.setrlimit(resource.RLIMIT_NOFILE, (1024, current_ulimit[1]))

    # Note: It's currently faster to tidy everything than looking for exactly what we should tidy
    verbosity = "-x" if verbose else ""
    promises = []
    for mod in get_default_modules().values():
        with ctx.cd(mod.full_path()):
            # https://docs.pyinvoke.org/en/stable/api/runners.html#invoke.runners.Runner.run
            promises.append(ctx.run(f"go mod tidy {verbosity}", asynchronous=True))

    for promise in promises:
        promise.join()


@task(autoprint=True)
def version(_):
    return Path(".go-version").read_text(encoding="utf-8").strip()


@task
def check_go_version(ctx):
    go_version_output = ctx.run('go version')
    # result is like "go version go1.24.4 linux/amd64"
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

    for mod in get_default_modules().values():
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
        modules = Configuration.from_file()
        assert path not in modules.modules, f'Module {path} already exists'

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

        # Add this module as independent in the module configuration
        modules.modules[path] = GoModule(path, independent=True)
        modules.to_file()
        print(
            f'{color_message("NOTE", "blue")}: The modules.yml file has been updated to mark the module as independent, you can modify this file to change the module configuration.'
        )

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
    except Exception as e:
        traceback.print_exc()

        # Restore files if user wants to
        if sys.stdin.isatty():
            print(color_message("Failed to create module", "red"))
            if yes_no_question('Do you want to restore all files ?', default=False):
                print(color_message("Restoring files", "blue"))

                ctx.run('git clean -f')
                ctx.run('git checkout HEAD -- .')

                raise Exit(code=1) from e

        print(color_message("Not removing changed files", "red"))

        raise Exit(code=1) from e


@task(iterable=['targets'])
def mod_diffs(_, targets):
    """
    Lists differences in versions of libraries in the repo,
    optionally compared to a list of target go.mod files.

    Parameters:
    - targets: list of paths to target go.mod files.
    """
    # Find all go.mod files in the repo
    all_go_mod_files = []
    for module in get_default_modules():
        all_go_mod_files.append(os.path.join(module, 'go.mod'))

    # Validate the provided targets
    for target in targets:
        if target not in all_go_mod_files:
            raise Exit(f"Error: Target go.mod file '{target}' not found.")

    for target in targets:
        all_go_mod_files.remove(target)

    # Dictionary to store library versions
    library_versions = defaultdict(lambda: defaultdict(set))

    # Regular expression to match require statements in go.mod
    require_pattern = re.compile(r'^\s*([a-zA-Z0-9.\-/]+)\s+([a-zA-Z0-9.\-+]+)')

    # Process each go.mod file
    for go_mod_file in all_go_mod_files + targets:
        with open(go_mod_file) as f:
            inside_require_block = False
            for line in f:
                line = line.strip()
                if line == "require (":
                    inside_require_block = True
                    continue
                elif inside_require_block and line == ")":
                    inside_require_block = False
                    continue
                if inside_require_block or line.startswith("require "):
                    match = require_pattern.match(line)
                    if match:
                        library, version = match.groups()
                        if not library.startswith("github.com/DataDog/datadog-agent/"):
                            library_versions[library][version].add(go_mod_file)

    # List libraries with multiple versions
    for library, versions in library_versions.items():
        if targets:
            relevant_paths = {path for version_paths in versions.values() for path in version_paths if path in targets}
            if relevant_paths:
                print(f"Library {library} differs in:")
                for version, paths in versions.items():
                    intersecting_paths = set(paths).intersection(targets)
                    if intersecting_paths:
                        print(f"  - Version {version} in:")
                        for path in intersecting_paths:
                            print(f"    * {path}")
        elif len(versions) > 1:
            print(f"Library {library} has different versions:")
            for version, paths in versions.items():
                print(f"  - Version {version} in:")
                for path in paths:
                    print(f"    * {path}")
