"""
High level testing tasks
"""
# TODO: check if we really need the typing import.
# Recent versions of Python should be able to use dict and list directly in type hints,
# so we only need to check that we don't run this code with old Python versions.

import abc
import glob
import json
import operator
import os
import platform
import re
import sys
from collections import defaultdict
from contextlib import contextmanager
from datetime import datetime
from typing import Dict, List

from invoke import task
from invoke.exceptions import Exit

from .agent import integration_tests as agent_integration_tests
from .build_tags import compute_build_tags_for_flavor
from .cluster_agent import integration_tests as dca_integration_tests
from .dogstatsd import integration_tests as dsd_integration_tests
from .flavor import AgentFlavor
from .go import run_golangci_lint
from .libs.common.color import color_message
from .libs.copyright import CopyrightLinter
from .libs.datadog_api import create_count, send_metrics
from .libs.junit_upload import add_flavor_to_junitxml, junit_upload_from_tgz, produce_junit_tar, repack_macos_junit_tar
from .modules import DEFAULT_MODULES, GoModule
from .trace_agent import integration_tests as trace_integration_tests
from .utils import DEFAULT_BRANCH, clean_nested_paths, get_build_flags

PROFILE_COV = "coverage.out"
GO_TEST_RESULT_TMP_JSON = 'module_test_output.json'
UNIT_TEST_FILE_FORMAT = re.compile(r'[^a-zA-Z0-9_\-]')


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
    for var in env:
        if var in original_environ:
            os.environ[var] = original_environ[var]
        else:
            os.environ.pop(var)


TOOL_LIST = [
    'github.com/frapposelli/wwhrd',
    'github.com/go-enry/go-license-detector/v4/cmd/license-detector',
    'github.com/golangci/golangci-lint/cmd/golangci-lint',
    'github.com/goware/modvendor',
    'github.com/stormcat24/protodep',
    'gotest.tools/gotestsum',
    'github.com/vektra/mockery/v2',
]

TOOL_LIST_PROTO = [
    'github.com/favadi/protoc-go-inject-tag',
    'github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway',
    'github.com/golang/protobuf/protoc-gen-go',
    'github.com/golang/mock/mockgen',
    'github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto',
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
    if os.path.isfile("go.work") or os.path.isfile("go.work.sum"):
        # Someone reported issues with this command when using a go.work but other people
        # use a go.work and don't have any issue, so the root cause is unclear.
        # Printing a warning because it might help someone but not enforcing anything.

        # The issue which was reported was that `go install` would fail with the following error:
        ### no required module provides package <package>; to add it:
        ### go get <package>
        print(
            color_message(
                "WARNING: In case of issue, you might want to try disabling go workspaces by setting the environment variable GOWORK=off, or even deleting go.work and go.work.sum",
                "orange",
            )
        )

    with environ({'GO111MODULE': 'on'}):
        for path, tools in TOOLS.items():
            with ctx.cd(path):
                for tool in tools:
                    ctx.run(f"go install {tool}")


@task
def invoke_unit_tests(ctx):
    """
    Run the unit tests on the invoke tasks
    """
    for _, _, files in os.walk("tasks/unit-tests/"):
        for file in files:
            if file[-3:] == ".py" and file != "__init__.py" and not bool(UNIT_TEST_FILE_FORMAT.search(file[:-3])):
                ctx.run(f"{sys.executable} -m tasks.unit-tests.{file[:-3]}", env={"GITLAB_TOKEN": "fake_token"})


def test_core(
    modules: List[GoModule],
    flavor: AgentFlavor,
    module_class: GoModule,
    operation_name: str,
    command,
    skip_module_class: bool = False,
    headless_mode: bool = False,
):
    """
    Run the command function on each module of the modules list.
    """
    modules_results = []
    if not headless_mode:
        print(f"--- Flavor {flavor.name}: {operation_name}")
    for module in modules:
        module_result = None
        if not skip_module_class:
            module_result = module_class(path=module.full_path())
        if not headless_mode:
            skipped_header = "[Skipped]" if not module.condition() else ""
            print(f"----- {skipped_header} Module '{module.full_path()}'")
        if not module.condition():
            continue

        command(modules_results, module, module_result)
    return modules_results


class ModuleResult(abc.ABC):
    def __init__(self, path):
        # The full path of the module
        self.path = path
        # Whether the command failed for that module
        self.failed = False
        # String for representing the result type in printed output
        self.result_type = "generic"

    def failure_string(self, flavor):
        return color_message(f"{self.result_type} for module {self.path} failed ({flavor.name} flavor)\n", "red")

    @abc.abstractmethod
    def get_failure(self, flavor):  # noqa: U100
        """
        Return a tuple with two elements:
        * bool value - True if the result is failed, False otherwise
        * str value - human-readable failure representation (if failed), empty string otherwise
        """
        pass


class ModuleLintResult(ModuleResult):
    def __init__(self, path):
        super().__init__(path)
        self.result_type = "Linters"
        # Results of failed lint calls
        self.lint_outputs = []

    def get_failure(self, flavor):
        failure_string = ""

        if self.failed:
            failure_string = self.failure_string(flavor)
            failure_string += "Linter failures:\n"
            for lint_output in self.lint_outputs:
                if lint_output.exited != 0:
                    failure_string = f"{failure_string}{lint_output.stdout}\n" if lint_output.stdout else failure_string
                    failure_string = f"{failure_string}{lint_output.stderr}\n" if lint_output.stderr else failure_string

        return self.failed, failure_string


class ModuleTestResult(ModuleResult):
    def __init__(self, path):
        super().__init__(path)
        self.result_type = "Tests"
        # Path to the result.json file output by gotestsum (should always be present)
        self.result_json_path = None
        # Path to the junit file output by gotestsum (only present if specified in inv test)
        self.junit_file_path = None

    def get_failure(self, flavor):
        failure_string = ""

        if self.failed:
            failure_string = self.failure_string(flavor)
            failed_packages = set()
            failed_tests = defaultdict(set)

            # TODO(AP-1959): this logic is now repreated, with some variations, in three places:
            # here, in system-probe.py, and in libs/pipeline_notifications.py
            # We should have some common result.json parsing lib.
            if self.result_json_path is not None and os.path.exists(self.result_json_path):
                with open(self.result_json_path, encoding="utf-8") as tf:
                    for line in tf:
                        json_test = json.loads(line.strip())
                        # This logic assumes that the lines in result.json are "in order", i.e. that retries
                        # are logged after the initial test run.

                        # The line is a "Package" line, but not a "Test" line.
                        # We take these into account, because in some cases (panics, race conditions),
                        # individual test failures are not reported, only a package-level failure is.
                        if 'Package' in json_test and 'Test' not in json_test:
                            package = json_test['Package']
                            action = json_test["Action"]

                            if action == "fail":
                                failed_packages.add(package)
                            elif action == "pass" and package in failed_tests:
                                # The package was retried and fully succeeded, removing from the list of packages to report
                                failed_packages.remove(package)

                        # The line is a "Test" line.
                        elif 'Package' in json_test and 'Test' in json_test:
                            name = json_test['Test']
                            package = json_test['Package']
                            action = json_test["Action"]
                            if action == "fail":
                                failed_tests[package].add(name)
                            elif action == "pass" and name in failed_tests.get(package, set()):
                                # The test was retried and succeeded, removing from the list of tests to report
                                failed_tests[package].remove(name)

            if failed_packages:
                failure_string += "Test failures:\n"
                for package in sorted(failed_packages):
                    tests = failed_tests.get(package, set())
                    if not tests:
                        failure_string += f"- {package} package failed due to panic / race condition\n"
                    else:
                        for name in sorted(tests):
                            failure_string += f"- {package} {name}\n"
            else:
                failure_string += "The test command failed, but no test failures detected in the result json."

        return self.failed, failure_string


def lint_flavor(
    ctx,
    modules: List[GoModule],
    flavor: AgentFlavor,
    build_tags: List[str],
    arch: str,
    rtloader_root: bool,
    concurrency: int,
    timeout=None,
    golangci_lint_kwargs: str = "",
    headless_mode: bool = False,
):
    """
    Runs linters for given flavor, build tags, and modules.
    """

    def command(module_results, module: GoModule, module_result):
        with ctx.cd(module.full_path()):
            lint_results = run_golangci_lint(
                ctx,
                module_path=module.path,
                targets=module.lint_targets,
                rtloader_root=rtloader_root,
                build_tags=build_tags,
                arch=arch,
                concurrency=concurrency,
                timeout=timeout,
                golangci_lint_kwargs=golangci_lint_kwargs,
                headless_mode=headless_mode,
            )
            for lint_result in lint_results:
                module_result.lint_outputs.append(lint_result)
                if lint_result.exited != 0:
                    module_result.failed = True
        module_results.append(module_result)

    return test_core(modules, flavor, ModuleLintResult, "golangci_lint", command, headless_mode=headless_mode)


def build_stdlib(
    ctx,
    build_tags: List[str],
    cmd: str,
    env: Dict[str, str],
    args: Dict[str, str],
    test_profiler: TestProfiler,
):
    """
    Builds the stdlib with the same build flags as the tests.

    Since Go 1.20, standard library is not pre-compiled anymore but is built as needed and cached in the build cache.
    To avoid a perfomance overhead when running tests, we pre-compile the standard library and cache it.
    We must use the same build flags as the one we are using when compiling tests to not invalidate the cache.
    """
    args["go_build_tags"] = " ".join(build_tags)

    ctx.run(
        cmd.format(**args),
        env=env,
        out_stream=test_profiler,
        warn=True,
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
    args["go_build_tags"] = " ".join(build_tags)

    junit_file_flag = ""
    junit_file = f"junit-out-{flavor.name}.xml"
    if junit_tar:
        junit_file_flag = "--junitfile " + junit_file
    args["junit_file_flag"] = junit_file_flag

    def command(test_results, module, module_result):
        with ctx.cd(module.full_path()):
            res = ctx.run(
                cmd.format(
                    packages=' '.join(f"{t}/..." if not t.endswith("/...") else t for t in module.targets), **args
                ),
                env=env,
                out_stream=test_profiler,
                warn=True,
            )

        module_result.result_json_path = os.path.join(module.full_path(), GO_TEST_RESULT_TMP_JSON)

        if res.exited is None or res.exited > 0:
            module_result.failed = True
        else:
            lines = res.stdout.splitlines()
            if lines is not None and 'DONE 0 tests' in lines[-1]:
                print(color_message("No tests were run, skipping coverage report", "orange"))
                cov_path = os.path.join(module.full_path(), PROFILE_COV)
                if os.path.exists(cov_path):
                    os.remove(cov_path)
                return

        if save_result_json:
            with open(save_result_json, 'ab') as json_file, open(module_result.result_json_path, 'rb') as module_file:
                json_file.write(module_file.read())

        if junit_tar:
            module_result.junit_file_path = os.path.join(module.full_path(), junit_file)
            add_flavor_to_junitxml(module_result.junit_file_path, flavor)

        test_results.append(module_result)

    return test_core(modules, flavor, ModuleTestResult, "unit tests", command)


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

    def command(_empty_result, module, _module_result):
        with ctx.cd(module.full_path()):
            ctx.run(f"go tool cover -func {PROFILE_COV}", warn=True)

    return test_core(modules, flavor, None, "code coverage", command, skip_module_class=True)


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

    def command(_empty_result, module, _module_result):
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

    return test_core(modules, flavor, None, "codecov upload", command, skip_module_class=True)


def process_input_args(input_module, input_targets, input_flavors, headless_mode=False):
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
        if not headless_mode:
            print("Using default modules and targets")
        modules = DEFAULT_MODULES.values()

    if not input_flavors:
        flavors = [AgentFlavor.base]
    else:
        flavors = [AgentFlavor[f] for f in input_flavors]

    return modules, flavors


def process_module_results(module_results: Dict[str, Dict[str, List[ModuleResult]]]):
    """
    Expects results in the format:
    {
        "phase1": {
            "flavor1": [module_result1, module_result2],
            "flavor2": [module_result3, module_result4],
        }
    }

    Prints failures, and returns False if at least one module failed in one phase.
    """

    success = True
    for phase in module_results:
        for flavor in module_results[phase]:
            for module_result in module_results[phase][flavor]:
                if module_result is not None:
                    module_failed, failure_string = module_result.get_failure(flavor)
                    success = success and (not module_failed)
                    if module_failed:
                        print(failure_string)

    return success


def deprecating_skip_linters_flag(skip_linters):
    """
    We're deprecating the --skip-linters flag in the test invoke task

    Displays a warning when user is running inv -e test --skip-linters
    Also displays the command the user should run to
    """
    if skip_linters:
        deprecation_msg = """Warning: the --skip-linters is deprecated for the test invoke task.
Feel free to remove the flag when running inv -e test.
"""
    else:
        deprecation_msg = """Warning: the linters were removed from the test invoke task.
If you want to run the linters, please run inv -e lint-go instead.
"""
    print(deprecation_msg, file=sys.stderr)


def sanitize_env_vars():
    """
    Sanitizes environment variables
    We want to ignore all `DD_` variables, as they will interfere with the behavior of some unit tests
    """
    for env in os.environ:
        if env.startswith("DD_"):
            del os.environ[env]


@task(iterable=['flavors'])
def test(
    ctx,
    module=None,
    targets=None,
    flavors=None,
    coverage=False,
    print_coverage=False,
    build_include=None,
    build_exclude=None,
    verbose=False,
    race=False,
    profile=False,
    rtloader_root=None,
    python_home_2=None,
    python_home_3=None,
    cpus=None,
    major_version='7',
    python_runtimes='3',
    timeout=180,
    arch="x64",
    cache=True,
    test_run_name="",
    skip_linters=False,
    save_result_json=None,
    rerun_fails=None,
    go_mod="mod",
    junit_tar="",
    only_modified_packages=False,
):
    """
    Run go tests on the given module and targets.

    A module should be provided as the path to one of the go modules in the repository.

    Targets should be provided as a comma-separated list of relative paths within the given module.
    If targets are provided but no module is set, the main module (".") is used.

    If no module or target is set the tests are run against all modules and targets.

    Example invokation:
        inv test --targets=./pkg/collector/check,./pkg/aggregator --race
        inv test --module=. --race
    """

    modules_results_per_phase = defaultdict(dict)

    sanitize_env_vars()

    deprecating_skip_linters_flag(skip_linters)

    modules, flavors = process_input_args(module, targets, flavors)

    unit_tests_tags = {
        f: compute_build_tags_for_flavor(
            flavor=f, build="unit-tests", arch=arch, build_include=build_include, build_exclude=build_exclude
        )
        for f in flavors
    }

    ldflags, gcflags, env = get_build_flags(
        ctx,
        rtloader_root=rtloader_root,
        python_home_2=python_home_2,
        python_home_3=python_home_3,
        major_version=major_version,
        python_runtimes=python_runtimes,
    )

    # Use stdout if no profile is set
    test_profiler = TestProfiler() if profile else None

    race_opt = "-race" if race else ""
    # atomic is quite expensive but it's the only way to run both the coverage and the race detector at the same time without getting false positives from the cover counter
    covermode_opt = "-covermode=" + ("atomic" if race else "count") if coverage else ""
    build_cpus_opt = f"-p {cpus}" if cpus else ""

    coverprofile = f"-coverprofile={PROFILE_COV}" if coverage else ""

    nocache = '-count=1' if not cache else ''

    if save_result_json and os.path.isfile(save_result_json):
        # Remove existing file since we append to it.
        # We don't need to do that for GO_TEST_RESULT_TMP_JSON since gotestsum overwrites the output.
        print(f"Removing existing '{save_result_json}' file")
        os.remove(save_result_json)

    test_run_arg = f"-run {test_run_name}" if test_run_name else ""

    stdlib_build_cmd = 'go build {verbose} -mod={go_mod} -tags "{go_build_tags}" -gcflags="{gcflags}" '
    stdlib_build_cmd += '-ldflags="{ldflags}" {build_cpus} {race_opt} std cmd'
    cmd = 'gotestsum {junit_file_flag} {json_flag} --format pkgname {rerun_fails} --packages="{packages}" -- {verbose} -mod={go_mod} -vet=off -timeout {timeout}s -tags "{go_build_tags}" -gcflags="{gcflags}" '
    cmd += '-ldflags="{ldflags}" {build_cpus} {race_opt} -short {covermode_opt} {coverprofile} {nocache} {test_run_arg}'
    args = {
        "go_mod": go_mod,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "race_opt": race_opt,
        "build_cpus": build_cpus_opt,
        "covermode_opt": covermode_opt,
        "coverprofile": coverprofile,
        "test_run_arg": test_run_arg,
        "timeout": int(timeout),
        "verbose": '-v' if verbose else '',
        "nocache": nocache,
        # Used to print failed tests at the end of the go test command
        "json_flag": f'--jsonfile "{GO_TEST_RESULT_TMP_JSON}" ',
        "rerun_fails": f"--rerun-fails={rerun_fails}" if rerun_fails else "",
    }

    # Test
    for flavor, build_tags in unit_tests_tags.items():
        build_stdlib(
            ctx,
            build_tags=build_tags,
            cmd=stdlib_build_cmd,
            env=env,
            args=args,
            test_profiler=test_profiler,
        )
        if only_modified_packages:
            modules = get_modified_packages(ctx)

        modules_results_per_phase["test"][flavor] = test_flavor(
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

    # Output
    if junit_tar:
        junit_files = []
        for flavor in modules_results_per_phase["test"]:
            for module_test_result in modules_results_per_phase["test"][flavor]:
                if module_test_result.junit_file_path:
                    junit_files.append(module_test_result.junit_file_path)

        produce_junit_tar(junit_files, junit_tar)

    if coverage and print_coverage:
        for flavor in flavors:
            coverage_flavor(ctx, flavor, modules)

    # FIXME(AP-1958): this prints nothing in CI. Commenting out the print line
    # in the meantime to avoid confusion
    if profile:
        # print("\n--- Top 15 packages sorted by run time:")
        test_profiler.print_sorted(15)

    success = process_module_results(modules_results_per_phase)

    if success:
        print(color_message("All tests passed", "green"))
    else:
        # Exit if any of the modules failed on any phase
        raise Exit(code=1)


def run_lint_go(
    ctx,
    module=None,
    targets=None,
    flavors=None,
    build="lint",
    build_tags=None,
    build_include=None,
    build_exclude=None,
    rtloader_root=None,
    arch="x64",
    cpus=None,
    timeout=None,
    golangci_lint_kwargs="",
    headless_mode=False,
):
    modules, flavors = process_input_args(module, targets, flavors, headless_mode)

    linter_tags = {
        f: build_tags
        or compute_build_tags_for_flavor(
            flavor=f, build=build, arch=arch, build_include=build_include, build_exclude=build_exclude
        )
        for f in flavors
    }

    modules_lint_results_per_flavor = {flavor: [] for flavor in flavors}

    for flavor, build_tags in linter_tags.items():
        modules_lint_results_per_flavor[flavor] = lint_flavor(
            ctx,
            modules=modules,
            flavor=flavor,
            build_tags=build_tags,
            arch=arch,
            rtloader_root=rtloader_root,
            concurrency=cpus,
            timeout=timeout,
            golangci_lint_kwargs=golangci_lint_kwargs,
            headless_mode=headless_mode,
        )

    return modules_lint_results_per_flavor


@task(iterable=['flavors'])
def lint_go(
    ctx,
    module=None,
    targets=None,
    flavors=None,
    build="lint",
    build_tags=None,
    build_include=None,
    build_exclude=None,
    rtloader_root=None,
    arch="x64",
    cpus=None,
    timeout: int = None,
    golangci_lint_kwargs="",
    headless_mode=False,
):
    """
    Run go linters on the given module and targets.

    A module should be provided as the path to one of the go modules in the repository.

    Targets should be provided as a comma-separated list of relative paths within the given module.
    If targets are provided but no module is set, the main module (".") is used.

    If no module or target is set the tests are run against all modules and targets.

    --timeout is the number of minutes after which the linter should time out.
    --headless-mode allows you to output the result in a single json file.

    Example invokation:
        inv lint-go --targets=./pkg/collector/check,./pkg/aggregator
        inv lint-go --module=.
    """

    # Format:
    # {
    #     "phase1": {
    #         "flavor1": [module_result1, module_result2],
    #         "flavor2": [module_result3, module_result4],
    #     }
    # }
    modules_results_per_phase = defaultdict(dict)

    modules_results_per_phase["lint"] = run_lint_go(
        ctx=ctx,
        module=module,
        targets=targets,
        flavors=flavors,
        build=build,
        build_tags=build_tags,
        build_include=build_include,
        build_exclude=build_exclude,
        rtloader_root=rtloader_root,
        arch=arch,
        cpus=cpus,
        timeout=timeout,
        golangci_lint_kwargs=golangci_lint_kwargs,
        headless_mode=headless_mode,
    )

    success = process_module_results(modules_results_per_phase)

    if success:
        if not headless_mode:
            print(color_message("All linters passed", "green"))
    else:
        # Exit if any of the modules failed on any phase
        raise Exit(code=1)


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
    branch = os.environ.get("BRANCH_NAME")
    pr_url = os.environ.get("PR_ID")

    if branch == DEFAULT_BRANCH:
        print(f"Running on {DEFAULT_BRANCH}, skipping check for team assignment.")
    elif pr_url:
        import requests

        pr_id = pr_url.rsplit('/')[-1]

        res = requests.get(f"https://api.github.com/repos/DataDog/datadog-agent/issues/{pr_id}")
        issue = res.json()

        labels = {l['name'] for l in issue.get('labels', [])}
        skip_qa_labels = ["qa/skip-qa", "qa/done", "qa/no-code-change"]
        if any(skip_label in labels for skip_label in skip_qa_labels):
            print("A label to skip QA is set -- no need for team assignment")
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
def lint_skip_qa(_):
    """
    Ensure that when qa/skip-qa is used, we have one of [qa/done , qa/no-code-change]. Error if not valid.
    """
    branch = os.environ.get("BRANCH_NAME")
    pr_url = os.environ.get("PR_ID")

    if branch == DEFAULT_BRANCH:
        print(f"Running on {DEFAULT_BRANCH}, skipping check for skip-qa label.")
    elif pr_url:
        import requests

        pr_id = pr_url.rsplit('/')[-1]

        res = requests.get(f"https://api.github.com/repos/DataDog/datadog-agent/issues/{pr_id}")
        issue = res.json()

        labels = {l['name'] for l in issue.get('labels', [])}
        skip_qa = "qa/skip-qa"
        new_qa_labels = ["qa/done", "qa/no-code-change"]
        if skip_qa in labels and not any(skip_label in labels for skip_label in new_qa_labels):
            print(
                f"PR {pr_url} request to skip QA without justification. Requires an additional `qa/done` or `qa/no-code-change`."
            )
            raise Exit(code=1)
        return
    # No PR is associated with this build: given that we have the "run only on PRs" setting activated,
    # this can only happen when we're building on a tag. We don't need to check for skip-qa.
    else:
        print("PR not found, skipping check for skip-qa.")


@task
def lint_milestone(_):
    """
    Make sure PRs are assigned a milestone
    """
    branch = os.environ.get("BRANCH_NAME")
    pr_url = os.environ.get("PR_ID")

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
        if (
            not file.startswith(
                ('test/kitchen/', 'tools/windows/DatadogAgentInstaller', 'test/workload-checks', 'test/regression')
            )
            and prefix_length + len(file) > max_length
        ):
            print(f"Error: path {file} is too long ({prefix_length + len(file) - max_length} characters too many)")
            failure = True

    if failure:
        raise Exit(code=1)


@task
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False, debug=False):
    """
    Run all the available integration tests
    """
    tests = [
        lambda: agent_integration_tests(ctx, install_deps, race, remote_docker),
        lambda: dsd_integration_tests(ctx, install_deps, race, remote_docker),
        lambda: dca_integration_tests(ctx, install_deps, race, remote_docker),
        lambda: trace_integration_tests(ctx, install_deps, race),
    ]
    for t in tests:
        try:
            t()
        except Exit as e:
            if e.code != 0:
                raise
            elif debug:
                print(e.message)


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


@task
def junit_macos_repack(_, infile, outfile):
    """
    Repacks JUnit tgz file from macOS Github Action run, so it would
    contain correct job name and job URL.
    """
    repack_macos_junit_tar(infile, outfile)


@task
def get_modified_packages(ctx) -> List[GoModule]:
    modified_files = get_modified_files(ctx)
    modified_go_files = [
        f"./{file}" for file in modified_files if file.endswith(".go") or file.endswith(".mod") or file.endswith(".sum")
    ]

    modules_to_test = {}
    go_mod_modified_modules = set()

    for modified_file in modified_go_files:
        match_precision = 0
        best_module_path = None

        # Since several modules can match the path we take only the most precise one
        for module_path in DEFAULT_MODULES:
            if module_path in modified_file:
                if len(module_path) > match_precision:
                    match_precision = len(module_path)
                    best_module_path = module_path

        # Check if the package is in the target list of the module we want to test
        targeted = False
        for target in DEFAULT_MODULES[best_module_path].targets:
            if os.path.normpath(os.path.join(best_module_path, target)) in modified_file:
                targeted = True
                break
        if not targeted:
            continue

        # If go mod was modified in the module we run the test for the whole module so we do not need to add modified packages to targets
        if best_module_path in go_mod_modified_modules:
            continue

        # If we modify the go.mod or go.sum we run the tests for the whole module
        if modified_file.endswith(".mod") or modified_file.endswith(".sum"):
            modules_to_test[best_module_path] = DEFAULT_MODULES[best_module_path]
            go_mod_modified_modules.add(best_module_path)
            continue

        # If the package has been deleted we do not try to run tests
        if not os.path.exists(os.path.dirname(modified_file)):
            continue

        # If there are no _test.go file in the folder we do not try to run tests
        if not glob.glob(os.path.dirname(modified_file) + "/*_test.go"):
            continue

        relative_target = "./" + os.path.relpath(os.path.dirname(modified_file), best_module_path)

        if best_module_path in modules_to_test:
            if (
                modules_to_test[best_module_path].targets is not None
                and os.path.dirname(modified_file) not in modules_to_test[best_module_path].targets
            ):
                modules_to_test[best_module_path].targets.append(relative_target)
        else:
            modules_to_test[best_module_path] = GoModule(best_module_path, targets=[relative_target])

    # Clean up duplicated paths to reduce Go test cmd length
    for module in modules_to_test:
        modules_to_test[module].targets = clean_nested_paths(modules_to_test[module].targets)
        if (
            len(modules_to_test[module].targets) >= 150
        ):  # With more packages we can reach the limit of the command line length on Windows
            modules_to_test[module].targets = DEFAULT_MODULES[module].targets

    print("Running tests for the following modules:")
    for module in modules_to_test:
        print(f"- {module}: {modules_to_test[module].targets}")

    return modules_to_test.values()


def get_modified_files(ctx):
    last_main_commit = ctx.run("git merge-base HEAD origin/main", hide=True).stdout
    print(f"Checking diff from {last_main_commit} commit on main branch")

    modified_files = ctx.run(f"git diff --name-only --no-renames {last_main_commit}", hide=True).stdout.splitlines()
    return modified_files


@task
def send_unit_tests_stats(_, job_name):
    fast_success = True
    classic_success = True

    n_test_classic = 0
    n_test_fast = 0

    series = []

    failed_tests_classic, n_test_classic = parse_test_log("test_output.json")
    classic_success = len(failed_tests_classic) == 0

    # If the fast tests are not run, we don't have the output file and we consider the job successful since it did not run any test
    if os.path.isfile("test_output_fast.json"):
        failed_tests_fast, n_test_fast = parse_test_log("test_output_fast.json")
        fast_success = len(failed_tests_fast) == 0
    else:
        print("test_output_fast.json not found, assuming no tests were run")

    timestamp = int(datetime.now().timestamp())
    print("Sending unit tests stats to Datadog")

    print(f"Classic test executed: {n_test_classic}")
    series.append(
        create_count(
            "datadog.ci.unit_tests.executed",
            timestamp,
            n_test_classic,
            tags=[
                "experimentation:fast-tests",
                "test_type:classic",
                "repository:datadog-agent",
                f"pipeline_id:{os.getenv('CI_PIPELINE_ID')}",
                f"job_name:{job_name}",
            ],
        )
    )

    print(f"Fast test executed: {n_test_fast}")
    series.append(
        create_count(
            "datadog.ci.unit_tests.executed",
            timestamp,
            n_test_fast,
            tags=[
                "experimentation:fast-tests",
                "test_type:fast",
                "repository:datadog-agent",
                f"pipeline_id:{os.getenv('CI_PIPELINE_ID')}",
                f"job_name:{job_name}-fast",
            ],
        )
    )

    print(f"Classic test success: {classic_success}")
    print(f"Fast test success: {fast_success}")

    if fast_success == classic_success:
        false_positive = 0
        false_negative = 0
    elif fast_success:
        false_positive = 1
        false_negative = 0
    else:
        false_positive = 0
        false_negative = 1

    series.append(
        create_count(
            "datadog.ci.unit_tests.false_positive",
            timestamp,
            false_positive,
            tags=[
                "experimentation:fast-tests",
                "repository:datadog-agent",
                f"pipeline_id:{os.getenv('CI_PIPELINE_ID')}",
                f"job_name:{job_name}",
            ],
        )
    )
    series.append(
        create_count(
            "datadog.ci.unit_tests.false_negative",
            timestamp,
            false_negative,
            tags=[
                "experimentation:fast-tests",
                "repository:datadog-agent",
                f"pipeline_id:{os.getenv('CI_PIPELINE_ID')}",
                f"job_name:{job_name}",
            ],
        )
    )

    send_metrics(series)


def parse_test_log(log_file):
    failed_tests = []
    n_test_executed = 0
    with open(log_file, "r") as f:
        for line in f:
            json_line = json.loads(line)
            if (
                json_line["Action"] == "fail"
                and "Test" in json_line
                and f'{json_line["Package"]}/{json_line["Test"]}' not in failed_tests
            ):
                n_test_executed += 1
                failed_tests.append(f'{json_line["Package"]}/{json_line["Test"]}')
            if json_line["Action"] == "pass" and "Test" in json_line:
                n_test_executed += 1
                if f'{json_line["Package"]}/{json_line["Test"]}' in failed_tests:
                    failed_tests.remove(f'{json_line["Package"]}/{json_line["Test"]}')
    return failed_tests, n_test_executed
