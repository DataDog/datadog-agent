"""
High level testing tasks
"""

from __future__ import annotations

import fnmatch
import glob
import json
import operator
import os
import re
import sys
from collections import defaultdict
from collections.abc import Iterable
from datetime import datetime
from pathlib import Path

import requests
from invoke import task
from invoke.context import Context
from invoke.exceptions import Exit

from tasks.agent import integration_tests as agent_integration_tests
from tasks.build_tags import compute_build_tags_for_flavor
from tasks.cluster_agent import integration_tests as dca_integration_tests
from tasks.coverage import PROFILE_COV, CodecovWorkaround
from tasks.devcontainer import run_on_devcontainer
from tasks.dogstatsd import integration_tests as dsd_integration_tests
from tasks.flavor import AgentFlavor
from tasks.libs.common.color import color_message
from tasks.libs.common.datadog_api import create_count, send_metrics
from tasks.libs.common.git import get_modified_files
from tasks.libs.common.junit_upload_core import enrich_junitxml, produce_junit_tar
from tasks.libs.common.utils import (
    clean_nested_paths,
    get_build_flags,
    gitlab_section,
    running_in_ci,
)
from tasks.libs.releasing.json import _get_release_json_value
from tasks.modules import DEFAULT_MODULES, GoModule, get_module_by_path
from tasks.test_core import ModuleTestResult, process_input_args, process_module_results, test_core
from tasks.testwasher import TestWasher
from tasks.trace_agent import integration_tests as trace_integration_tests
from tasks.update_go import PATTERN_MAJOR_MINOR_BUGFIX

GO_TEST_RESULT_TMP_JSON = 'module_test_output.json'
WINDOWS_MAX_PACKAGES_NUMBER = 150
TRIGGER_ALL_TESTS_PATHS = ["tasks/gotest.py", "tasks/build_tags.py", ".gitlab/source_test/*"]
OTEL_UPSTREAM_GO_MOD_PATH = (
    "https://raw.githubusercontent.com/open-telemetry/opentelemetry-collector-contrib/main/go.mod"
)


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


def build_standard_lib(
    ctx,
    build_tags: list[str],
    cmd: str,
    env: dict[str, str],
    args: dict[str, str],
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
    build_tags: list[str],
    modules: Iterable[GoModule],
    cmd: str,
    env: dict[str, str],
    args: dict[str, str],
    junit_tar: str,
    save_result_json: str,
    test_profiler: TestProfiler,
    coverage: bool = False,
):
    """
    Runs unit tests for given flavor, build tags, and modules.
    """
    args["go_build_tags"] = " ".join(build_tags)

    args["json_flag"] = "--jsonfile " + GO_TEST_RESULT_TMP_JSON
    junit_file = f"junit-out-{flavor.name}.xml"
    junit_file_flag = "--junitfile " + junit_file if junit_tar else ""
    args["junit_file_flag"] = junit_file_flag

    def command(test_results, module, module_result):
        module_path = module.full_path()
        with ctx.cd(module_path):
            packages = ' '.join(f"{t}/..." if not t.endswith("/...") else t for t in module.targets)
            with CodecovWorkaround(ctx, module_path, coverage, packages, args) as cov_test_path:
                res = ctx.run(
                    command=cmd.format(
                        packages=packages,
                        cov_test_path=cov_test_path,
                        **args,
                    ),
                    env=env,
                    out_stream=test_profiler,
                    warn=True,
                )

        module_result.result_json_path = os.path.join(module_path, GO_TEST_RESULT_TMP_JSON)

        if res.exited is None or res.exited > 0:
            module_result.failed = True
        else:
            lines = res.stdout.splitlines()
            if lines is not None and 'DONE 0 tests' in lines[-1]:
                cov_path = os.path.join(module_path, PROFILE_COV)
                print(color_message(f"No tests were run, skipping coverage report. Removing {cov_path}.", "orange"))
                try:
                    os.remove(cov_path)
                except FileNotFoundError as e:
                    print(f"Couldn't remove coverage file {cov_path}\n{e}")
                return

        if save_result_json:
            with open(save_result_json, 'ab') as json_file, open(module_result.result_json_path, 'rb') as module_file:
                json_file.write(module_file.read())

        if junit_tar:
            module_result.junit_file_path = os.path.join(module_path, junit_file)
            enrich_junitxml(module_result.junit_file_path, flavor)

        test_results.append(module_result)

    return test_core(modules, flavor, ModuleTestResult, "unit tests", command)


def coverage_flavor(
    ctx,
    flavor: AgentFlavor,
    modules: list[GoModule],
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


def sanitize_env_vars():
    """
    Sanitizes environment variables
    We want to ignore all `DD_` variables, as they will interfere with the behavior of some unit tests
    """
    for env in os.environ:
        if env.startswith("DD_"):
            del os.environ[env]


def process_test_result(test_results, junit_tar: str, flavor: AgentFlavor, test_washer: bool) -> bool:
    if junit_tar:
        junit_files = [
            module_test_result.junit_file_path
            for module_test_result in test_results
            if module_test_result.junit_file_path
        ]

        produce_junit_tar(junit_files, junit_tar)

    success = process_module_results(flavor=flavor, module_results=test_results)

    if success:
        print(color_message("All tests passed", "green"))
        return True

    if test_washer or running_in_ci():
        if not test_washer:
            print("Test washer is always enabled in the CI, enforcing it")

        tw = TestWasher()
        should_succeed = tw.process_module_results(test_results)
        if should_succeed:
            print(
                color_message("All failing tests are known to be flaky, marking the test job as successful", "orange")
            )
            return True

    return False


@task
@run_on_devcontainer
def test(
    ctx,
    module=None,
    targets=None,
    flavor=None,
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
    timeout=180,
    cache=True,
    test_run_name="",
    save_result_json=None,
    rerun_fails=None,
    go_mod="mod",
    junit_tar="",
    only_modified_packages=False,
    only_impacted_packages=False,
    include_sds=False,
    skip_flakes=False,
    build_stdlib=False,
    test_washer=False,
    run_on=None,  # noqa: U100, F841. Used by the run_on_devcontainer decorator
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
    sanitize_env_vars()

    modules, flavor = process_input_args(ctx, module, targets, flavor)

    unit_tests_tags = compute_build_tags_for_flavor(
        flavor=flavor,
        build="unit-tests",
        build_include=build_include,
        build_exclude=build_exclude,
        include_sds=include_sds,
    )

    ldflags, gcflags, env = get_build_flags(
        ctx,
        rtloader_root=rtloader_root,
        python_home_2=python_home_2,
        python_home_3=python_home_3,
        major_version=major_version,
    )

    # Use stdout if no profile is set
    test_profiler = TestProfiler() if profile else None

    race_opt = "-race" if race else ""
    # atomic is quite expensive but it's the only way to run both the coverage and the race detector at the same time without getting false positives from the cover counter
    covermode_opt = "-covermode=" + ("atomic" if race else "count") if coverage else ""
    build_cpus_opt = f"-p {cpus}" if cpus else ""

    nocache = '-count=1' if not cache else ''

    if save_result_json and os.path.isfile(save_result_json):
        # Remove existing file since we append to it.
        # We don't need to do that for GO_TEST_RESULT_TMP_JSON since gotestsum overwrites the output.
        print(f"Removing existing '{save_result_json}' file")
        os.remove(save_result_json)

    test_run_arg = f"-run {test_run_name}" if test_run_name else ""

    stdlib_build_cmd = 'go build {verbose} -mod={go_mod} -tags "{go_build_tags}" -gcflags="{gcflags}" '
    stdlib_build_cmd += '-ldflags="{ldflags}" {build_cpus} {race_opt} std cmd'
    rerun_coverage_fix = '--raw-command {cov_test_path}' if coverage else ""
    gotestsum_flags = (
        '{junit_file_flag} {json_flag} --format {gotestsum_format} {rerun_fails} --packages="{packages}" '
        + rerun_coverage_fix
    )
    gobuild_flags = (
        '-mod={go_mod} -tags "{go_build_tags}" -gcflags="{gcflags}" -ldflags="{ldflags}" {build_cpus} {race_opt}'
    )
    govet_flags = '-vet=off'
    gotest_flags = '{verbose} -timeout {timeout}s -short {covermode_opt} {test_run_arg} {nocache}'
    cmd = f'gotestsum {gotestsum_flags} -- {gobuild_flags} {govet_flags} {gotest_flags}'
    args = {
        "go_mod": go_mod,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "race_opt": race_opt,
        "build_cpus": build_cpus_opt,
        "covermode_opt": covermode_opt,
        "test_run_arg": test_run_arg,
        "timeout": int(timeout),
        "verbose": '-v' if verbose else '',
        "nocache": nocache,
        # Used to print failed tests at the end of the go test command
        "rerun_fails": f"--rerun-fails={rerun_fails}" if rerun_fails else "",
        "skip_flakes": "--skip-flake" if skip_flakes else "",
        "gotestsum_format": "standard-verbose" if verbose else "pkgname",
    }

    # Test
    if build_stdlib:
        build_standard_lib(
            ctx,
            build_tags=unit_tests_tags,
            cmd=stdlib_build_cmd,
            env=env,
            args=args,
            test_profiler=test_profiler,
        )

    if only_modified_packages:
        modules = get_modified_packages(ctx, build_tags=unit_tests_tags)
    if only_impacted_packages:
        modules = get_impacted_packages(ctx, build_tags=unit_tests_tags)

    with gitlab_section("Running unit tests", collapsed=True):
        test_results = test_flavor(
            ctx,
            flavor=flavor,
            build_tags=unit_tests_tags,
            modules=modules,
            cmd=cmd,
            env=env,
            args=args,
            junit_tar=junit_tar,
            save_result_json=save_result_json,
            test_profiler=test_profiler,
            coverage=coverage,
        )

    # Output

    if coverage and print_coverage:
        coverage_flavor(ctx, flavor, modules)

    # FIXME(AP-1958): this prints nothing in CI. Commenting out the print line
    # in the meantime to avoid confusion
    if profile:
        # print("\n--- Top 15 packages sorted by run time:")
        test_profiler.print_sorted(15)

    success = process_test_result(test_results, junit_tar, flavor, test_washer)
    if not success:
        raise Exit(code=1)

    print(f"Tests final status (including re-runs): {color_message('ALL TESTS PASSED', 'green')}")


@task
def integration_tests(ctx, race=False, remote_docker=False, debug=False, timeout=""):
    """
    Run all the available integration tests
    """
    tests = [
        lambda: agent_integration_tests(ctx, race=race, remote_docker=remote_docker, timeout=timeout),
        lambda: dsd_integration_tests(ctx, race=race, remote_docker=remote_docker, timeout=timeout),
        lambda: dca_integration_tests(ctx, race=race, remote_docker=remote_docker, timeout=timeout),
        lambda: trace_integration_tests(ctx, race=race, timeout=timeout),
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
        raise Exit(code=1)
    if not os.getenv("DATADOG_AGENT_IMAGE"):
        if not agent_image:
            print("define DATADOG_AGENT_IMAGE envvar or image flag")
            raise Exit(code=1)
        os.environ["DATADOG_AGENT_IMAGE"] = agent_image
    if not os.getenv("DATADOG_CLUSTER_AGENT_IMAGE"):
        if not dca_image:
            print("define DATADOG_CLUSTER_AGENT_IMAGE envvar or image flag")
            raise Exit(code=1)
        os.environ["DATADOG_CLUSTER_AGENT_IMAGE"] = dca_image
    if not os.getenv("ARGO_WORKFLOW"):
        if argo_workflow:
            os.environ["ARGO_WORKFLOW"] = argo_workflow

    ctx.run(f"./test/e2e/scripts/setup-instance/00-entrypoint-{target}.sh")


@task
def get_modified_packages(ctx, build_tags=None, lint=False) -> list[GoModule]:
    modified_files = get_go_modified_files(ctx)

    modified_go_files = [f"./{file}" for file in modified_files]

    if build_tags is None:
        build_tags = []

    modules_to_test = {}
    go_mod_modified_modules = set()

    for modified_file in modified_go_files:
        best_module_path = Path(get_go_module(modified_file))

        # Check if the package is in the target list of the module we want to test
        targeted = False

        assert best_module_path, f"No module found for {modified_file}"
        module = get_module_by_path(best_module_path)
        targets = module.lint_targets if lint else module.targets

        for target in targets:
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
            modules_to_test[best_module_path] = get_module_by_path(best_module_path)
            go_mod_modified_modules.add(best_module_path)
            continue

        # If the package has been deleted we do not try to run tests
        if not os.path.exists(os.path.dirname(modified_file)):
            continue

        # If there are go file matching the build tags in the folder we do not try to run tests
        res = ctx.run(
            f'go list -tags "{" ".join(build_tags)}" ./{os.path.dirname(modified_file)}/...', hide=True, warn=True
        )
        if res.stderr is not None and "matched no packages" in res.stderr:
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
            len(modules_to_test[module].targets) >= WINDOWS_MAX_PACKAGES_NUMBER
        ):  # With more packages we can reach the limit of the command line length on Windows
            modules_to_test[module].targets = DEFAULT_MODULES[module].targets

    print("Running tests for the following modules:")
    for module in modules_to_test:
        print(f"- {module}: {modules_to_test[module].targets}")

    return list(modules_to_test.values())


@task(iterable=["extra_tag"])
def send_unit_tests_stats(_, job_name, extra_tag=None):
    if extra_tag is None:
        extra_tag = []

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
            ]
            + extra_tag,
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
            ]
            + extra_tag,
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
            ]
            + extra_tag,
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
            ]
            + extra_tag,
        )
    )

    send_metrics(series)


def parse_test_log(log_file):
    failed_tests = []
    n_test_executed = 0
    with open(log_file) as f:
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


@task
def get_impacted_packages(ctx, build_tags=None):
    if build_tags is None:
        build_tags = []
    dependencies = create_dependencies(ctx, build_tags)
    files = get_go_modified_files(ctx)

    # Safeguard to be sure that the files that should trigger all test are not renamed without being updated
    for file in TRIGGER_ALL_TESTS_PATHS:
        if len(glob.glob(file)) == 0:
            raise Exit(
                code=1,
                message=f"No file matched {file} make sure you modified TRIGGER_ALL_TEST_FILES if you renamed one of them",
            )

    # Some files like tasks/gotest.py should trigger all tests
    if should_run_all_tests(ctx, TRIGGER_ALL_TESTS_PATHS):
        print(f"Triggering all tests because a file matching one of the {TRIGGER_ALL_TESTS_PATHS} was modified")
        return DEFAULT_MODULES.values()

    modified_packages = {f"github.com/DataDog/datadog-agent/{os.path.dirname(file)}" for file in files}

    # Modification to go.mod and go.sum should force the tests of the whole module to run
    for file in files:
        if file.endswith("go.mod") or file.endswith("go.sum"):
            with ctx.cd(os.path.dirname(file)):
                all_packages = ctx.run(
                    f'go list -tags "{" ".join(build_tags)}" ./...', hide=True, warn=True
                ).stdout.splitlines()
                modified_packages.update(set(all_packages))

    # Modification to fixture folders count as modification to their parent package
    for file in files:
        if not file.endswith(".go"):
            formatted_path = Path(os.path.dirname(file)).as_posix()
            while len(formatted_path) > 0:
                if glob.glob(f"{formatted_path}/*.go"):
                    print(f"Found {file} belonging to package {formatted_path}")
                    modified_packages.add(f"github.com/DataDog/datadog-agent/{formatted_path}")
                    break
                formatted_path = "/".join(formatted_path.split("/")[:-1])

    imp = find_impacted_packages(dependencies, modified_packages)
    return format_packages(ctx, impacted_packages=imp, build_tags=build_tags)


def create_dependencies(ctx, build_tags=None):
    if build_tags is None:
        build_tags = []
    modules_deps = defaultdict(set)
    for modules in DEFAULT_MODULES:
        with ctx.cd(modules):
            res = ctx.run(
                'go list '
                + f'-tags "{" ".join(build_tags)}" '
                + '-f "{{.ImportPath}} {{.Imports}} {{.TestImports}}" ./...',
                hide=True,
                warn=True,
            )
            imports = res.stdout.splitlines()
            for imp in imports:
                imp = imp.split(" ", 1)
                package, imported_packages = imp[0], imp[1].replace("[", "").replace("]", "").split(" ")
                for imported_package in imported_packages:
                    if imported_package.startswith("github.com/DataDog/datadog-agent"):
                        modules_deps[imported_package].add(package)

    return modules_deps


def find_impacted_packages(dependencies, modified_modules, cache=None):
    if cache is None:
        cache = {}
    impacted_modules = set()
    for modified_module in modified_modules:
        if modified_module in cache:
            impacted_modules.update(cache[modified_module])
        else:
            stack = [modified_module]
            while stack:
                module = stack.pop()
                if module in impacted_modules:
                    continue
                impacted_modules.add(module)
                stack.extend(dependencies[module])
            cache[modified_module] = impacted_modules
    return impacted_modules


def format_packages(ctx: Context, impacted_packages: set[str], build_tags: list[str] | None = None):
    """
    Format the packages list to be used in our test function. Will take each path and create a list of modules with its targets
    """
    if build_tags is None:
        build_tags = []

    packages = [f'{package.replace("github.com/DataDog/datadog-agent/", "./")}' for package in impacted_packages]
    modules_to_test = {}

    for package in packages:
        module_path = get_go_module(package)

        # Check if the module is in the target list of the modules we want to test
        if module_path not in DEFAULT_MODULES or not DEFAULT_MODULES[module_path].condition():
            continue

        # Check if the package is in the target list of the module we want to test
        targeted = False
        for target in DEFAULT_MODULES[module_path].targets:
            if normpath(os.path.join(module_path, target)) in package:
                targeted = True
                break
        if not targeted:
            continue

        # If the package has been deleted we do not try to run tests
        if not os.path.exists(package):
            continue

        relative_target = "./" + os.path.relpath(package, module_path).replace("\\", "/")

        if module_path in modules_to_test:
            if modules_to_test[module_path].targets is not None and package not in modules_to_test[module_path].targets:
                modules_to_test[module_path].targets.append(relative_target)
        else:
            modules_to_test[module_path] = GoModule(module_path, targets=[relative_target])

    # Clean up duplicated paths to reduce Go test cmd length
    for module in modules_to_test:
        modules_to_test[module].targets = clean_nested_paths(modules_to_test[module].targets)
        if (
            len(modules_to_test[module].targets) >= WINDOWS_MAX_PACKAGES_NUMBER
        ):  # With more packages we can reach the limit of the command line length on Windows
            modules_to_test[module].targets = DEFAULT_MODULES[module].targets

    module_to_remove = []
    # Clean up to avoid running tests on package with no Go files matching build tags
    for module in modules_to_test:
        with ctx.cd(module):
            res = ctx.run(
                f'go list -tags "{" ".join(build_tags)}" {" ".join([normpath(os.path.join("github.com/DataDog/datadog-agent", module, target)) for target in modules_to_test[module].targets])}',
                hide=True,
                warn=True,
            )
            if res is not None and res.stderr is not None:
                for package in res.stderr.splitlines():
                    package_to_remove = os.path.relpath(
                        package.split(" ")[1].strip(":").replace("github.com/DataDog/datadog-agent/", ""), module
                    ).replace("\\", "/")
                    try:
                        modules_to_test[module].targets.remove(f"./{package_to_remove}")
                        if len(modules_to_test[module].targets) == 0:
                            module_to_remove.append(module)
                    except Exception:
                        print("Could not remove ", package_to_remove, ", ignoring...")
    for module in module_to_remove:
        del modules_to_test[module]

    print("Running tests for the following modules:")
    for module in modules_to_test:
        print(f"- {module}: {modules_to_test[module].targets}")

    return modules_to_test.values()


def normpath(path):  # Normpath with forward slashes to avoid issues on Windows
    return os.path.normpath(path).replace("\\", "/")


def get_go_module(path):
    while path != '/':
        go_mod_path = os.path.join(path, 'go.mod')
        if os.path.isfile(go_mod_path):
            return os.path.relpath(path)
        path = os.path.dirname(path)
    raise Exception(f"No go.mod file found for package at {path}")


def should_run_all_tests(ctx, trigger_files):
    base_branch = _get_release_json_value("base_branch")
    files = get_modified_files(ctx, base_branch=base_branch)
    return any(len(fnmatch.filter(files, trigger_file)) for trigger_file in trigger_files)


def get_go_modified_files(ctx):
    base_branch = _get_release_json_value("base_branch")
    files = get_modified_files(ctx, base_branch=base_branch)
    return [
        file
        for file in files
        if file.find("unit_tests/testdata/components_src") == -1
        and (file.endswith(".go") or file.endswith(".mod") or file.endswith(".sum"))
    ]


@task
def lint_go(
    ctx,
    module=None,
    targets=None,
    flavor=None,
    build="lint",
    build_tags=None,
    build_include=None,
    build_exclude=None,
    rtloader_root=None,
    cpus=None,
    timeout: int | None = None,
    golangci_lint_kwargs="",
    headless_mode=False,
    include_sds=False,
    only_modified_packages=False,
):
    raise Exit("This task is deprecated, please use `inv linter.go`", 1)


def rename_package(file_path, old_name, new_name):
    with open(file_path) as f:
        content = f.read()
    # Rename package
    content = content.replace(old_name, new_name)
    with open(file_path, "w") as f:
        f.write(content)


@task
def check_otel_build(ctx):
    file_path = "test/otel/dependencies.go"
    package_otel = "package otel"
    package_main = "package main"
    rename_package(file_path, package_otel, package_main)

    with ctx.cd("test/otel"):
        # Update dependencies to latest local version
        res = ctx.run("go mod tidy")
        if not res.ok:
            raise Exit(f"Error running `go mod tidy`: {res.stderr}")

        # Build test/otel/dependencies.go with same settings as `make otelcontribcol`
        res = ctx.run("GO111MODULE=on CGO_ENABLED=0 go build -trimpath -o . .", warn=True)
        if res is None or not res.ok:
            raise Exit(f"Error building otel components with datadog-agent dependencies: {res.stderr}")

    rename_package(file_path, package_main, package_otel)


@task
def check_otel_module_versions(ctx):
    pattern = f"^go {PATTERN_MAJOR_MINOR_BUGFIX}\r?$"
    r = requests.get(OTEL_UPSTREAM_GO_MOD_PATH)
    matches = re.findall(pattern, r.text, flags=re.MULTILINE)
    if len(matches) != 1:
        raise Exit(f"Error parsing upstream go.mod version: {OTEL_UPSTREAM_GO_MOD_PATH}")
    upstream_version = matches[0]

    for path, module in DEFAULT_MODULES.items():
        if module.used_by_otel:
            mod_file = f"./{path}/go.mod"
            with open(mod_file, newline='', encoding='utf-8') as reader:
                content = reader.read()
                matches = re.findall(pattern, content, flags=re.MULTILINE)
                if len(matches) != 1:
                    raise Exit(f"{mod_file} does not match expected go directive format")
                if matches[0] != upstream_version:
                    raise Exit(f"{mod_file} version {matches[0]} does not match upstream version: {upstream_version}")
