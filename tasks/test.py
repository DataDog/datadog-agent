"""
High level testing tasks
"""
# TODO: check if we really need the typing import.
# Recent versions of Python should be able to use dict and list directly in type hints,
# so we only need to check that we don't run this code with old Python versions.

import operator
import os
import platform
import re
import sys
from contextlib import contextmanager
from typing import Dict, List

from invoke import task
from invoke.exceptions import Exit

from .agent import integration_tests as agent_integration_tests
from .build_tags import compute_build_tags_for_flavor
from .cluster_agent import integration_tests as dca_integration_tests
from .dogstatsd import integration_tests as dsd_integration_tests
from .flavor import AgentFlavor
from .go import golangci_lint
from .libs.copyright import CopyrightLinter
from .libs.junit_upload import add_flavor_to_junitxml, junit_upload_from_tgz, produce_junit_tar
from .modules import DEFAULT_MODULES, GoModule
from .trace_agent import integration_tests as trace_integration_tests
from .utils import DEFAULT_BRANCH, get_build_flags

PROFILE_COV = "profile.cov"
GO_TEST_RESULT_TMP_JSON = 'tmp.json'


class TestProfiler:
    times = []
    parser = re.compile(r"^ok\s+github.com\/DataDog\/datadog-agent\/(\S+)\s+([0-9\.]+)s", re.MULTILINE)

    def write(self, txt):
        # Output to stdout
        # NOTE: write to underlying stream on Python 3 to avoid unicode issues when default encoding is not UTF-8
        getattr(sys.stdout, 'buffer', sys.stdout).write(ensure_bytes(txt))
        # Extract the run time
        for result in self.parser.finditer(txt):
            self.times.append((result.group(1), float(result.group(2))))

    def flush(self):
        sys.stdout.flush()

    def print_sorted(self, limit=0):
        if self.times:
            sorted_times = sorted(self.times, key=operator.itemgetter(1), reverse=True)

            if limit:
                sorted_times = sorted_times[:limit]
            for pkg, time in sorted_times:
                print(f"{time}s\t{pkg}")


def ensure_bytes(s):
    if not isinstance(s, bytes):
        return s.encode('utf-8')

    return s


@contextmanager
def environ(env):
    original_environ = os.environ.copy()
    os.environ.update(env)
    yield
    for var in env.keys():
        if var in original_environ:
            os.environ[var] = original_environ[var]
        else:
            os.environ.pop(var)


TOOL_LIST = [
    'github.com/frapposelli/wwhrd',
    'github.com/go-enry/go-license-detector/v4/cmd/license-detector',
    'github.com/golangci/golangci-lint/cmd/golangci-lint',
    'github.com/goware/modvendor',
    'github.com/mgechev/revive',
    'github.com/stormcat24/protodep',
    'gotest.tools/gotestsum',
    'github.com/vektra/mockery/v2',
]

TOOL_LIST_PROTO = [
    'github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway',
    'github.com/golang/protobuf/protoc-gen-go',
    'github.com/golang/mock/mockgen',
    'github.com/tinylib/msgp',
]

TOOLS = {
    'internal/tools': TOOL_LIST,
    'internal/tools/proto': TOOL_LIST_PROTO,
}


@task
def download_tools(ctx):
    """Download all Go tools for testing."""
    with environ({'GO111MODULE': 'on'}):
        for path, _ in TOOLS.items():
            with ctx.cd(path):
                ctx.run("go mod download")


@task
def install_tools(ctx):
    """Install all Go tools for testing."""
    with environ({'GO111MODULE': 'on'}):
        for path, tools in TOOLS.items():
            with ctx.cd(path):
                for tool in tools:
                    ctx.run(f"go install {tool}")


# TODO(AP-1879): The following four functions all do something similar: they run a given command on a list of modules
# This could be refactored in a core function that does the loop on modules and returns failures, and
# wrapper functions that craft the command to run and process the results and errors.


def lint_flavor(
    ctx, modules: List[GoModule], flavor: AgentFlavor, build_tags: List[str], arch: str, rtloader_root: bool
):
    """
    Runs linters for given flavor, build tags, and modules.
    """
    print(f"--- Flavor {flavor.name}: golangci_lint")
    for module in modules:
        print(f"----- Module '{module.full_path()}'")
        if not module.condition():
            print("----- Skipped")
            continue

        with ctx.cd(module.full_path()):
            golangci_lint(
                ctx, targets=module.targets, rtloader_root=rtloader_root, build_tags=build_tags, arch=arch
            )


def test_flavor(
    ctx,
    flavor: AgentFlavor,
    build_tags: List[str],
    modules: List[GoModule],
    cmd: str,
    env: Dict[str, str],
    args: Dict[str, str],
    junit_tar: str,
    save_result_json: str,
    test_profiler: TestProfiler,
):
    """
    Runs unit tests for given flavor, build tags, and modules.
    """
    print(f"--- Flavor {flavor.name}: unit tests")

    failed_modules = []
    junit_files = []

    args["go_build_tags"] = " ".join(build_tags + ["test"])

    junit_file_flag = ""
    junit_file = f"junit-out-{flavor.name}.xml"
    if junit_tar:
        junit_file_flag = "--junitfile " + junit_file
    args["junit_file_flag"] = junit_file_flag

    for module in modules:
        print(f"----- Module '{module.full_path()}'")
        if not module.condition():
            print("----- Skipped")
            continue

        with ctx.cd(module.full_path()):
            res = ctx.run(
                cmd.format(
                    packages=' '.join(f"{t}/..." if not t.endswith("/...") else t for t in module.targets), **args
                ),
                env=env,
                out_stream=test_profiler,
                warn=True,
            )

        if res.exited is None or res.exited > 0:
            failed_modules.append(module.full_path())

        if save_result_json:
            with open(save_result_json, 'ab') as json_file, open(
                os.path.join(module.full_path(), GO_TEST_RESULT_TMP_JSON), 'rb'
            ) as module_file:
                json_file.write(module_file.read())

        if junit_tar:
            junit_file_path = os.path.join(module.full_path(), junit_file)
            add_flavor_to_junitxml(junit_file_path, flavor)
            junit_files.append(junit_file_path)

    return junit_files, failed_modules


def coverage_flavor(
    ctx,
    flavor: AgentFlavor,
    modules: List[GoModule],
):
    """
    Prints the code coverage of all modules for the given flavor.
    This expects that the coverage files have already been generated by
    inv test --coverage.
    """
    print(f"--- Flavor {flavor.name}: code coverage")

    for module in modules:
        print(f"----- Module '{module.full_path()}'")
        if not module.condition():
            print("----- Skipped")
            continue

        with ctx.cd(module.full_path()):
            ctx.run(f"go tool cover -func {PROFILE_COV}", warn=True)


def codecov_flavor(
    ctx,
    flavor: AgentFlavor,
    modules: List[GoModule],
):
    """
    Uploads coverage data of all modules for the given flavor.
    This expects that the coverage files have already been generated by
    inv test --coverage.
    """
    print(f"--- Flavor {flavor.name}: codecov upload")

    for module in modules:
        print(f"----- Module '{module.full_path()}'")
        if not module.condition():
            print("----- Skipped")
            continue

        # Codecov flags are limited to 45 characters
        tag = f"{platform.system()}-{flavor.name}-{module.codecov_path()}"
        if len(tag) > 45:
            # Best-effort attempt to get a unique and legible tag name
            tag = f"{platform.system()[:1]}-{flavor.name}-{module.codecov_path()}"[:45]

        # The codecov command has to be run from the root of the repository, otherwise
        # codecov gets confused and merges the roots of all modules, resulting in a
        # nonsensical directory tree in the codecov app
        path = os.path.normpath(os.path.join(module.path, PROFILE_COV))
        ctx.run(f"codecov -f {path} -F {tag}", warn=True)


def process_input_args(input_module, input_targets, input_flavors):
    """
    Takes the input module, targets and flavors arguments from inv test and inv codecov,
    sets default values for them & casts them to the expected types.
    """
    if isinstance(input_module, str):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        if isinstance(input_targets, str):
            modules = [GoModule(input_module, targets=input_targets.split(','))]
        else:
            modules = [m for m in DEFAULT_MODULES.values() if m.path == input_module]
    elif isinstance(input_targets, str):
        modules = [GoModule(".", targets=input_targets.split(','))]
    else:
        print("Using default modules and targets")
        modules = DEFAULT_MODULES.values()

    if not input_flavors:
        flavors = [AgentFlavor.base]
    else:
        flavors = [AgentFlavor[f] for f in input_flavors]

    return modules, flavors


@task(iterable=['flavors'])
def test(
    ctx,
    module=None,
    targets=None,
    flavors=None,
    coverage=False,
    build_include=None,
    build_exclude=None,
    verbose=False,
    race=False,
    profile=False,
    rtloader_root=None,
    python_home_2=None,
    python_home_3=None,
    cpus=0,
    major_version='7',
    python_runtimes='3',
    timeout=180,
    arch="x64",
    cache=True,
    skip_linters=False,
    save_result_json=None,
    rerun_fails=None,
    go_mod="mod",
    junit_tar="",
):
    """
    Run all the tools and tests on the given module and targets.

    A module should be provided as the path to one of the go modules in the repository.

    Targets should be provided as a comma-separated list of relative paths within the given module.
    If targets are provided but no module is set, the main module (".") is used.

    If no module or target is set the tests are run against all modules and targets.

    Example invokation:
        inv test --targets=./pkg/collector/check,./pkg/aggregator --race
        inv test --module=. --race
    """
    # Process input arguments

    modules, flavors = process_input_args(module, targets, flavors)

    flavors_build_tags = {
        f: compute_build_tags_for_flavor(
            flavor=f, build="unit-tests", arch=arch, build_include=build_include, build_exclude=build_exclude
        )
        for f in flavors
    }

    timeout = int(timeout)

    # Lint

    if skip_linters:
        print("--- [skipping Go linters]")
    else:
        for flavor, build_tags in flavors_build_tags.items():
            lint_flavor(
                ctx, modules=modules, flavor=flavor, build_tags=build_tags, arch=arch, rtloader_root=rtloader_root
            )

    ldflags, gcflags, env = get_build_flags(
        ctx,
        rtloader_root=rtloader_root,
        python_home_2=python_home_2,
        python_home_3=python_home_3,
        major_version=major_version,
        python_runtimes=python_runtimes,
    )

    if sys.platform == 'win32':
        env['CGO_LDFLAGS'] += ' -Wl,--allow-multiple-definition'

    if profile:
        test_profiler = TestProfiler()
    else:
        test_profiler = None  # Use stdout

    race_opt = ""
    covermode_opt = ""
    build_cpus_opt = ""
    if cpus:
        build_cpus_opt = f"-p {cpus}"
    if race:
        # race doesn't appear to be supported on non-x64 platforms
        if arch == "x86":
            print("\n -- Warning... disabling race test, not supported on this platform --\n")
        else:
            race_opt = "-race"

        # Needed to fix an issue when using -race + gcc 10.x on Windows
        # https://github.com/bazelbuild/rules_go/issues/2614
        if sys.platform == 'win32':
            ldflags += " -linkmode=external"

    if coverage:
        if race:
            # atomic is quite expensive but it's the only way to run
            # both the coverage and the race detector at the same time
            # without getting false positives from the cover counter
            covermode_opt = "-covermode=atomic"
        else:
            covermode_opt = "-covermode=count"

    coverprofile = ""
    if coverage:
        coverprofile = f"-coverprofile={PROFILE_COV}"

    nocache = '-count=1' if not cache else ''

    if save_result_json and os.path.isfile(save_result_json):
        # Remove existing file since we append to it.
        # We don't need to do that for GO_TEST_RESULT_TMP_JSON since gotestsum overwrites the output.
        print(f"Removing existing '{save_result_json}' file")
        os.remove(save_result_json)

    cmd = 'gotestsum {junit_file_flag} {json_flag} --format pkgname {rerun_fails} --packages="{packages}" -- {verbose} -mod={go_mod} -vet=off -timeout {timeout}s -tags "{go_build_tags}" -gcflags="{gcflags}" '
    cmd += '-ldflags="{ldflags}" {build_cpus} {race_opt} -short {covermode_opt} {coverprofile} {nocache}'
    args = {
        "go_mod": go_mod,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "race_opt": race_opt,
        "build_cpus": build_cpus_opt,
        "covermode_opt": covermode_opt,
        "coverprofile": coverprofile,
        "timeout": timeout,
        "verbose": '-v' if verbose else '',
        "nocache": nocache,
        "json_flag": f'--jsonfile "{GO_TEST_RESULT_TMP_JSON}" ' if save_result_json else "",
        "rerun_fails": f"--rerun-fails={rerun_fails}" if rerun_fails else "",
    }

    # Test

    failed_modules = {}
    junit_files = []
    for flavor, build_tags in flavors_build_tags.items():
        junit_files_for_flavor, failed_modules_for_flavor = test_flavor(
            ctx,
            flavor=flavor,
            build_tags=build_tags,
            modules=modules,
            cmd=cmd,
            env=env,
            args=args,
            junit_tar=junit_tar,
            save_result_json=save_result_json,
            test_profiler=test_profiler,
        )

        if failed_modules_for_flavor:
            failed_modules[flavor] = failed_modules_for_flavor
        if junit_files_for_flavor:
            junit_files.extend(junit_files_for_flavor)

    # Output

    if junit_tar:
        produce_junit_tar(junit_files, junit_tar)

    if coverage:
        for flavor in flavors:
            coverage_flavor(ctx, flavor, modules)

    if profile:
        print("\n--- Top 15 packages sorted by run time:")
        test_profiler.print_sorted(15)

    if failed_modules:
        failure_string = '\n'.join(
            [
                f"{', '.join(failed_modules_for_flavor)} ({flavor.name} flavor)"
                for flavor, failed_modules_for_flavor in failed_modules.items()
            ]
        )
        # Exit if any of the modules failed
        raise Exit(code=1, message=f"Unit tests failed in the following modules:\n{failure_string}")


@task(iterable=['flavors'])
def codecov(
    ctx,
    module=None,
    targets=None,
    flavors=None,
):
    modules, flavors = process_input_args(module, targets, flavors)

    for flavor in flavors:
        codecov_flavor(ctx, flavor, modules)


@task
def lint_teamassignment(_):
    """
    Make sure PRs are assigned a team label
    """
    branch = os.environ.get("CIRCLE_BRANCH")
    pr_url = os.environ.get("CIRCLE_PULL_REQUEST")

    if branch == DEFAULT_BRANCH:
        print(f"Running on {DEFAULT_BRANCH}, skipping check for team assignment.")
    elif pr_url:
        import requests

        pr_id = pr_url.rsplit('/')[-1]

        res = requests.get(f"https://api.github.com/repos/DataDog/datadog-agent/issues/{pr_id}")
        issue = res.json()

        labels = {l['name'] for l in issue.get('labels', [])}
        if "qa/skip-qa" in labels:
            print("qa/skip-qa label set -- no need for team assignment")
            return

        for label in labels:
            if label.startswith('team/'):
                print(f"Team Assignment: {label}")
                return

        print(f"PR {pr_url} requires team assignment label (team/...); got labels:")
        for label in labels:
            print(f" {label}")
        raise Exit(code=1)

    # No PR is associated with this build: given that we have the "run only on PRs" setting activated,
    # this can only happen when we're building on a tag. We don't need to check for a team assignment.
    else:
        print("PR not found, skipping check for team assignment.")


@task
def lint_milestone(_):
    """
    Make sure PRs are assigned a milestone
    """
    branch = os.environ.get("CIRCLE_BRANCH")
    pr_url = os.environ.get("CIRCLE_PULL_REQUEST")

    if branch == DEFAULT_BRANCH:
        print(f"Running on {DEFAULT_BRANCH}, skipping check for milestone.")
    elif pr_url:
        import requests

        pr_id = pr_url.rsplit('/')[-1]

        res = requests.get(f"https://api.github.com/repos/DataDog/datadog-agent/issues/{pr_id}")
        pr = res.json()
        if pr.get("milestone"):
            print(f"Milestone: {pr['milestone'].get('title', 'NO_TITLE')}")
            return

        print(f"PR {pr_url} requires a milestone.")
        raise Exit(code=1)

    # No PR is associated with this build: given that we have the "run only on PRs" setting activated,
    # this can only happen when we're building on a tag. We don't need to check for a milestone.
    else:
        print("PR not found, skipping check for milestone.")


@task
def lint_releasenote(ctx):
    """
    Lint release notes with Reno
    """

    branch = os.environ.get("CIRCLE_BRANCH")
    pr_url = os.environ.get("CIRCLE_PULL_REQUEST")

    if branch == DEFAULT_BRANCH:
        print(f"Running on {DEFAULT_BRANCH}, skipping release note check.")
    # Check if a releasenote has been added/changed
    elif pr_url:
        import requests

        pr_id = pr_url.rsplit('/')[-1]

        # first check 'changelog/no-changelog' label
        res = requests.get(f"https://api.github.com/repos/DataDog/datadog-agent/issues/{pr_id}")
        issue = res.json()
        if any([l['name'] == 'changelog/no-changelog' for l in issue.get('labels', {})]):
            print("'changelog/no-changelog' label found on the PR: skipping linting")
            return

        # Then check that at least one note was touched by the PR
        url = f"https://api.github.com/repos/DataDog/datadog-agent/pulls/{pr_id}/files"
        # traverse paginated github response
        while True:
            res = requests.get(url)
            files = res.json()
            if any(
                [
                    f['filename'].startswith("releasenotes/notes/")
                    or f['filename'].startswith("releasenotes-dca/notes/")
                    or f['filename'].startswith("releasenotes-installscript/notes/")
                    for f in files
                ]
            ):
                break

            if 'next' in res.links:
                url = res.links['next']['url']
            else:
                print(
                    "Error: No releasenote was found for this PR. Please add one using 'reno'"
                    ", or apply the label 'changelog/no-changelog' to the PR."
                )
                raise Exit(code=1)
    # No PR is associated with this build: given that we have the "run only on PRs" setting activated,
    # this can only happen when we're building on a tag. We don't need to check for release notes.
    else:
        print("PR not found, skipping release note check.")

    ctx.run("reno lint")


@task
def lint_filenames(ctx):
    """
    Scan files to ensure there are no filenames too long or containing illegal characters
    """
    files = ctx.run("git ls-files -z", hide=True).stdout.split("\0")
    failure = False

    if sys.platform == 'win32':
        print("Running on windows, no need to check filenames for illegal characters")
    else:
        print("Checking filenames for illegal characters")
        forbidden_chars = '<>:"\\|?*'
        for file in files:
            if any(char in file for char in forbidden_chars):
                print(f"Error: Found illegal character in path {file}")
                failure = True

    print("Checking filename length")
    # Approximated length of the prefix of the repo during the windows release build
    prefix_length = 160
    # Maximum length supported by the win32 API
    max_length = 255
    for file in files:
        if not file.startswith('test/kitchen/') and prefix_length + len(file) > max_length:
            print(f"Error: path {file} is too long ({prefix_length + len(file) - max_length} characters too many)")
            failure = True

    if failure:
        raise Exit(code=1)


@task
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False):
    """
    Run all the available integration tests
    """
    agent_integration_tests(ctx, install_deps, race, remote_docker)
    dsd_integration_tests(ctx, install_deps, race, remote_docker)
    dca_integration_tests(ctx, install_deps, race, remote_docker)
    trace_integration_tests(ctx, install_deps, race)


@task
def e2e_tests(ctx, target="gitlab", agent_image="", dca_image="", argo_workflow="default"):
    """
    Run e2e tests in several environments.
    """
    choices = ["gitlab", "dev", "local"]
    if target not in choices:
        print(f'target {target} not in {choices}')
        raise Exit(1)
    if not os.getenv("DATADOG_AGENT_IMAGE"):
        if not agent_image:
            print("define DATADOG_AGENT_IMAGE envvar or image flag")
            raise Exit(1)
        os.environ["DATADOG_AGENT_IMAGE"] = agent_image
    if not os.getenv("DATADOG_CLUSTER_AGENT_IMAGE"):
        if not dca_image:
            print("define DATADOG_CLUSTER_AGENT_IMAGE envvar or image flag")
            raise Exit(1)
        os.environ["DATADOG_CLUSTER_AGENT_IMAGE"] = dca_image
    if not os.getenv("ARGO_WORKFLOW"):
        if argo_workflow:
            os.environ["ARGO_WORKFLOW"] = argo_workflow

    ctx.run(f"./test/e2e/scripts/setup-instance/00-entrypoint-{target}.sh")


@task
def lint_python(ctx):
    """
    Lints Python files.
    See 'setup.cfg' and 'pyproject.toml' file for configuration.
    If running locally, you probably want to use the pre-commit instead.
    """

    print(
        f"""Remember to set up pre-commit to lint your files before committing:
    https://github.com/DataDog/datadog-agent/blob/{DEFAULT_BRANCH}/docs/dev/agent_dev_env.md#pre-commit-hooks"""
    )

    ctx.run("flake8 .")
    ctx.run("black --check --diff .")
    ctx.run("isort --check-only --diff .")
    ctx.run("vulture --ignore-decorators @task --ignore-names 'test_*,Test*' tasks")


@task
def lint_copyrights(_, fix=False, dry_run=False, debug=False):
    """
    Checks that all Go files contain the appropriate copyright header. If '--fix'
    is provided as an option, it will try to fix problems as it finds them. If
    '--dry_run' is provided when fixing, no changes to the files will be applied.
    """

    CopyrightLinter(debug=debug).assert_compliance(fix=fix, dry_run=dry_run)


@task
def install_shellcheck(ctx, version="0.8.0", destination="/usr/local/bin"):
    """
    Installs the requested version of shellcheck in the specified folder (by default /usr/local/bin).
    Required to run the shellcheck pre-commit hook.
    """

    if sys.platform == 'win32':
        print("shellcheck is not supported on Windows")
        raise Exit(code=1)
    if sys.platform.startswith('darwin'):
        platform = "darwin"
    if sys.platform.startswith('linux'):
        platform = "linux"

    ctx.run(
        f"wget -qO- \"https://github.com/koalaman/shellcheck/releases/download/v{version}/shellcheck-v{version}.{platform}.x86_64.tar.xz\" | tar -xJv -C /tmp"
    )
    ctx.run(f"cp \"/tmp/shellcheck-v{version}/shellcheck\" {destination}")
    ctx.run(f"rm -rf \"/tmp/shellcheck-v{version}\"")


@task()
def junit_upload(_, tgz_path):
    """
    Uploads JUnit XML files from an archive produced by the `test` task.
    """

    junit_upload_from_tgz(tgz_path)
