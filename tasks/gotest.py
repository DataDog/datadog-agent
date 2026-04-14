"""
High level testing tasks
"""

from __future__ import annotations

import dataclasses
import fnmatch
import glob
import operator
import os
import re
import sys
import time
from collections import defaultdict
from collections.abc import Iterable
from datetime import datetime
from pathlib import Path

import requests
from invoke import task
from invoke.context import Context
from invoke.exceptions import Exit

from tasks.build_tags import compute_build_tags_for_flavor
from tasks.collector import OTEL_CONTRIB_VERSION
from tasks.coverage import PROFILE_COV, CodecovWorkaround
from tasks.devcontainer import run_on_devcontainer
from tasks.flavor import AgentFlavor
from tasks.libs.build.bazel import bazel
from tasks.libs.common.color import color_message
from tasks.libs.common.datadog_api import create_count, send_metrics
from tasks.libs.common.git import get_modified_files
from tasks.libs.common.gomodules import get_default_modules
from tasks.libs.common.junit_upload_core import enrich_junitxml, produce_junit_tar
from tasks.libs.common.utils import (
    clean_nested_paths,
    get_build_flags,
    gitlab_section,
    running_in_ci,
)
from tasks.libs.releasing.json import _get_release_json_value
from tasks.libs.testing.result_json import ActionType, ResultJson
from tasks.modules import GoModule, get_module_by_path
from tasks.test_core import DEFAULT_TEST_OUTPUT_JSON, TestResult, process_input_args, process_result
from tasks.testwasher import TestWasher
from tasks.update_go import PATTERN_MAJOR_MINOR, update_file


@dataclasses.dataclass
class TestStats:
    """Counts from a single test runner (go or bazel)."""

    total: int = 0
    passed: int = 0
    failed: int = 0
    skipped: int = 0
    duration_s: float = 0.0

    def __add__(self, other: TestStats) -> TestStats:
        return TestStats(
            total=self.total + other.total,
            passed=self.passed + other.passed,
            failed=self.failed + other.failed,
            skipped=self.skipped + other.skipped,
            duration_s=self.duration_s + other.duration_s,
        )


WINDOWS_MAX_PACKAGES_NUMBER = 150
WINDOWS_MAX_CLI_LENGTH = 8000  # Windows has a max command line length of 8192 characters
TRIGGER_ALL_TESTS_PATHS = ["tasks/gotest.py", "tasks/build_tags.py", ".gitlab/build/source_test/*", ".gitlab-ci.yml"]
MODULE_PREFIX = "github.com/DataDog/datadog-agent"
OTEL_UPSTREAM_GO_MOD_PATH = (
    f"https://raw.githubusercontent.com/open-telemetry/opentelemetry-collector-contrib/v{OTEL_CONTRIB_VERSION}/go.mod"
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

    ctx.run(cmd.format(**args), env=env, out_stream=test_profiler)  # with `warn=True`, errors went unnoticed


def _target_to_bazel_pattern(target: str) -> str:
    """Convert a Go test target path to a Bazel target pattern.

    Examples:
        '.'           -> '//...'
        './'          -> '//...'
        './pkg/util'  -> '//pkg/util/...'
        './pkg/util/' -> '//pkg/util/...'
        './pkg/...'   -> '//pkg/...'
    """
    if target in ('.', './'):
        return '//...'
    # Strip leading './' then any trailing '/' to avoid double-slash before '/...'
    rel = target.removeprefix('./').rstrip('/')
    if rel.endswith('/...'):
        return f'//{rel}'
    return f'//{rel}/...'


def _minimize_bazel_patterns(patterns: list[str]) -> list[str]:
    """Remove patterns that are already covered by a broader pattern in the list.

    Patterns are of the form '//some/path/...' .  Pattern B is subsumed by
    pattern A when B's directory starts with A's directory, e.g.:
        //comp/core/... is subsumed by //comp/...
        //pkg/util/log/... is subsumed by //pkg/...
        //...  subsumes everything.

    The input list may contain duplicates; the output will not.
    """
    # Sort lexicographically so shorter (broader) patterns come before the
    # longer (narrower) ones they subsume.
    sorted_patterns = sorted(set(patterns))
    result: list[str] = []
    for pattern in sorted_patterns:
        covered = any(kept == '//...' or pattern.startswith(kept[: -len('...')]) for kept in result)
        if not covered:
            result.append(pattern)
    return result


def get_bazel_test_targets(ctx, modules: list[GoModule]) -> dict[str, str]:
    """Query Bazel for go_test targets within the scope of the given modules.

    Returns a dict mapping Bazel label (without config hash) to Go import path.

    Example:
        {"//pkg/util/log:log_test": "github.com/DataDog/datadog-agent/pkg/util/log"}

    The query is scoped to the same targets passed to go test, so
    'dda inv test --targets=./pkg/util' only queries //pkg/util/...
    instead of all of //...
    """
    bazel_patterns = []
    for module in modules:
        if not module.should_test():
            continue
        prefix = '' if module.path == '.' else module.path + '/'
        for target in module.test_targets:
            # Prepend module path for non-root modules
            full_target = f'./{prefix}{target.lstrip("./")}' if module.path != '.' else target
            bazel_patterns.append(_target_to_bazel_pattern(full_target))

    if not bazel_patterns:
        return {}

    bazel_patterns = _minimize_bazel_patterns(bazel_patterns)

    # Temporary: restrict queries to the top-level trees that have been migrated
    # to Bazel.  Remove this filter when other trees (e.g. test/, tasks/) follow.
    _BAZEL_QUERYABLE_ROOTS = ('//cmd/', '//comp/', '//pkg/')
    bazel_patterns = [p for p in bazel_patterns if any(p.startswith(r) for r in _BAZEL_QUERYABLE_ROOTS)]
    if not bazel_patterns:
        return {}

    scope = ' + '.join(bazel_patterns)
    output = bazel(ctx, "cquery", f"kind(go_test, {scope})", capture_output=True, hide_stderr=True)
    if not output:
        return {}

    result = {}
    for line in output.splitlines():
        line = line.strip()
        if not line:
            continue
        # Strip config hash: //pkg/util/log:log_test (abc1234) -> //pkg/util/log:log_test
        label = line.split(' ')[0]
        if not label.startswith('//'):
            continue
        # Get directory from label: //pkg/util/log:log_test -> pkg/util/log
        dir_path = label.split(':')[0][2:]  # strip //
        result[label] = f'{MODULE_PREFIX}/{dir_path}'
    return result


def _parse_bazel_test_line(line: str) -> tuple[str, str, str | None, bool] | None:
    """Parse a Bazel test result summary line.

    Returns (label, status, timing, cached) or None if not a result line.

    Expected input formats:
        //pkg/util/log:log_test                   PASSED in 0.521s
        //pkg/aggregator/ckey:ckey_test           (cached) PASSED in 1.234s
        //pkg/api/security:security_test          SKIPPED
        //pkg/process/util:util_test              FAILED in 0.345s
    """
    line = line.strip()
    if not line.startswith('//'):
        return None
    parts = line.split()
    label = parts[0]
    cached = '(cached)' in parts
    for status in ('PASSED', 'FAILED', 'SKIPPED'):
        if status in parts:
            m = re.search(r'in ([\d.]+s)', line)
            return (label, status, m.group(1) if m else None, cached)
    return None


def _run_bazel_tests(ctx, bazel_targets: list[str], verbose: bool = False) -> TestStats:
    """Run Bazel test targets and print results formatted like go test output.

    Targets are batched so the total command length stays under 32000 chars,
    which is a safe heuristic for Windows command-line limits.
    TODO: relax this limit on Linux where the effective limit is ~2 MB.

    Prints one ✓/∅/FAIL line per target then a DONE summary.
    Returns a TestStats with counts from this run.
    """
    from invoke.exceptions import UnexpectedExit

    if not bazel_targets:
        return TestStats()

    # Windows-safe command-length limit.
    # TODO: on Linux runners, the limit is much higher; consider platform-specific batching.
    MAX_CMD_LENGTH = 32000
    FIXED_ARGS = ["test", "--keep_going"]
    fixed_len = sum(len(a) + 1 for a in FIXED_ARGS)

    # Batch targets so no single invocation exceeds the limit.
    batches: list[list[str]] = []
    current_batch: list[str] = []
    current_len = fixed_len
    for target in bazel_targets:
        target_len = len(target) + 1  # +1 for the separating space
        if current_batch and current_len + target_len > MAX_CMD_LENGTH:
            batches.append(current_batch)
            current_batch = [target]
            current_len = fixed_len + target_len
        else:
            current_batch.append(target)
            current_len += target_len
    if current_batch:
        batches.append(current_batch)

    # Lines written by Bazel that are not test results and should be hidden.
    _NOISY_PREFIXES = ('Loading:', 'Analyzing:', 'INFO:', 'WARNING:', 'Computing main repo mapping:')

    parsed_results: list[tuple[str, str, str | None, bool]] = []
    run_failed = False
    t_start = time.monotonic()

    for batch in batches:
        try:
            # capture_stderr=True because Bazel writes test result lines to stderr.
            output = bazel(ctx, *FIXED_ARGS, *batch, capture_output=True, capture_stderr=True)
        except UnexpectedExit as e:
            output = (e.result.stdout or "") + (e.result.stderr or "")
            run_failed = True

        if output:
            for line in output.splitlines():
                # In non-verbose mode filter Bazel's progress/info noise.
                if not verbose and line.strip().startswith(_NOISY_PREFIXES):
                    continue
                parsed = _parse_bazel_test_line(line)
                if parsed:
                    parsed_results.append(parsed)

    duration_s = time.monotonic() - t_start

    # Print one formatted line per result.
    n_passed = n_failed = n_skipped = 0
    for label, status, timing, cached in parsed_results:
        if status == 'PASSED':
            symbol = color_message('✓', 'green')
            n_passed += 1
        elif status == 'SKIPPED':
            symbol = color_message('∅', 'yellow')
            n_skipped += 1
        else:
            symbol = color_message('FAIL', 'red')
            n_failed += 1

        timing_str = f' ({timing})' if timing else ''
        cached_str = ' (cached)' if cached else ''
        print(f'{symbol}  {label}{timing_str}{cached_str}')

    total = n_passed + n_failed + n_skipped
    # Treat a non-zero Bazel exit as a failure even if some result lines were parsed.
    # This avoids reporting success when a later batch fails due to build/infra issues.
    if run_failed:
        if total == 0:
            n_failed = len(bazel_targets)
            total = n_failed
        elif n_failed == 0:
            n_failed += 1
            total += 1

    return TestStats(total=total, passed=n_passed, failed=n_failed, skipped=n_skipped, duration_s=duration_s)


def expand_packages_excluding(
    ctx,
    modules: list[GoModule],
    build_tags: list[str],
    exclude_packages: set[str],
    recursive: bool = True,
) -> str:
    """
    Enumerate Go packages via `go list`, filter out exclude_packages, and return
    the explicit space-separated list for gotestsum --packages.

    This is needed because go test / gotestsum have no native exclusion syntax.
    """
    result = []
    tag_str = ' '.join(build_tags)
    for module in modules:
        if not module.should_test():
            continue
        for target in module.test_targets:
            # Build the pattern relative to module.path — ctx.cd() below makes
            # that the working directory, so do NOT prepend module.path again.
            t = target if target.startswith('./') else f'./{target}'
            if recursive and not t.endswith('/...'):
                pattern = f'{t}/...'
            else:
                pattern = t
            with ctx.cd(module.path):
                res = ctx.run(
                    f'go list -buildvcs=false -tags "{tag_str}" {pattern}',
                    hide=True,
                    warn=True,
                )
            if res and res.stdout:
                for pkg in res.stdout.splitlines():
                    if pkg not in exclude_packages:
                        result.append(pkg)
    return ' '.join(result)


def test_flavor(
    ctx,
    flavor: AgentFlavor,
    build_tags: list[str],
    modules: Iterable[GoModule],
    cmd: str,
    env: dict[str, str],
    args: dict[str, str],
    result_junit: str,
    test_profiler: TestProfiler,
    coverage: bool = False,
    result_json: str = DEFAULT_TEST_OUTPUT_JSON,
    recursive: bool = True,
    exclude_packages: set[str] | None = None,
):
    """
    Runs unit tests for given flavor, build tags, and modules.
    """

    # Early return if no modules are given
    # Can happen when --only-impacted-packages or --only-modified-packages is used
    if not modules:
        return

    result = TestResult('.')

    # Set default values for args
    args["go_build_tags"] = " ".join(build_tags)
    args["json_flag"] = ""
    args["junit_file_flag"] = ""

    # Produce the result json file, which is used to show the failures at the end of the test run
    if result_json:
        result.result_json_path = os.path.join(result.path, result_json)
        args["json_flag"] = "--jsonfile " + result.result_json_path

    # Produce the junit file if needed
    if result_junit:
        result_junit_path = os.path.join(result.path, result_junit)
        args["junit_file_flag"] = "--junitfile " + result_junit_path

    # Compute full list of targets to run tests against
    module_list = list(modules)
    if exclude_packages:
        packages = expand_packages_excluding(ctx, module_list, build_tags, exclude_packages, recursive)
    else:
        packages = compute_gotestsum_cli_args(module_list, recursive)

    with CodecovWorkaround(ctx, result.path, coverage, packages, args) as cov_test_path:
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
        # early stop on SIGINT: exit code is 128 + signal number, SIGINT is 2, so 130
        if res is not None and res.exited == 130:
            raise KeyboardInterrupt()

    if res.exited is None or res.exited > 0:
        result.failed = True
    else:
        lines = res.stdout.splitlines()
        if lines is not None and 'DONE 0 tests' in lines[-1]:
            cov_path = os.path.join(result.path, PROFILE_COV)
            print(color_message(f"No tests were run, skipping coverage report. Removing {cov_path}.", "orange"))
            try:
                os.remove(cov_path)
            except FileNotFoundError as e:
                print(f"Could not remove coverage file {cov_path}\n{e}")
            return

    if result_junit:
        enrich_junitxml(result_junit, flavor)  # type: ignore

    return result


def coverage_flavor(ctx):
    """
    Prints the code coverage for the given flavor.
    This expects that the coverage file has already been generated by
    dda inv test --coverage.
    """

    ctx.run(f"go tool cover -func {PROFILE_COV}", warn=True)


def sanitize_env_vars():
    """
    Sanitizes environment variables
    We want to ignore all `DD_` variables, as they will interfere with the behavior of some unit tests
    """
    for env in os.environ:
        # Allow the env var that enables NodeTreeModel for testing purposes
        if env == "DD_CONF_NODETREEMODEL":
            continue
        if env.startswith("DD_"):
            del os.environ[env]


def _generate_unified_output(
    ctx, test_result: TestResult, flavor: AgentFlavor, tw: TestWasher | None = None, test_system: str = "unit"
) -> TestStats | None:
    """Generate a UTOF JSON file alongside the test output JSON.

    Returns a TestStats with go test counts extracted from the UTOF summary,
    or None if the unified output could not be generated.
    """
    if not test_result.result_json_path or not os.path.exists(test_result.result_json_path):
        return None

    try:
        from tasks.libs.testing.utof import format_report
        from tasks.libs.testing.utof.go_unit import convert_unit_test_results, generate_metadata

        result_json = ResultJson.from_file(test_result.result_json_path)
        metadata = generate_metadata(ctx, test_system=test_system, flavor=flavor.name)
        utof = convert_unit_test_results(ctx, result_json, test_washer=tw, metadata=metadata)
        utof_path = test_result.result_json_path.replace('.json', '_unified.json')
        utof.write_json(utof_path)
        print(f"Unified test output written to {utof_path}")
        with gitlab_section("Unified test report", collapsed=False):
            print(format_report(utof))
        s = utof.summary
        return TestStats(
            total=s.total,
            passed=s.passed,
            failed=s.failed,
            skipped=getattr(s, 'skipped', 0),
            duration_s=getattr(utof.metadata, 'duration_seconds', 0.0) or 0.0,
        )
    except Exception:
        import traceback

        print(f"Warning: Failed to generate unified test output:\n{traceback.format_exc()}")
        return None


def process_test_result(
    ctx,
    test_result: TestResult,
    junit_tar: str,
    junit_files: list[str],
    flavor: AgentFlavor,
    test_washer: bool,
    test_system: str = "unit",
    skip_unified_output: bool = False,
) -> tuple[bool, TestStats | None]:
    """Process go test results.

    Returns (success, go_stats). go_stats is None when skip_unified_output=True
    or when the unified output could not be generated. Callers that only care
    about success can unpack with: success, _ = process_test_result(...)
    """
    if junit_tar:
        produce_junit_tar(junit_files, junit_tar)

    success = process_result(flavor=flavor, result=test_result)
    tw = None

    if success:
        print(color_message("All tests passed", "green"))
        go_stats = (
            None if skip_unified_output else _generate_unified_output(ctx, test_result, flavor, test_system=test_system)
        )
        return True, go_stats

    if test_washer or running_in_ci():
        if not test_washer:
            print("Test washer is always enabled in the CI, enforcing it")

        tw = TestWasher(test_output_json_file=test_result.result_json_path)
        print(
            "Processing test results for known flakes. Learn more about flake marker and test washer at https://datadoghq.atlassian.net/wiki/spaces/ADX/pages/3405611398/Flaky+tests+in+go+introducing+flake.Mark"
        )
        should_succeed = tw.process_result(test_result)
        if should_succeed:
            print(
                color_message("All failing tests are known to be flaky, marking the test job as successful", "orange")
            )
            go_stats = (
                None
                if skip_unified_output
                else _generate_unified_output(ctx, test_result, flavor, tw=tw, test_system=test_system)
            )
            return True, go_stats

    go_stats = (
        None
        if skip_unified_output
        else _generate_unified_output(ctx, test_result, flavor, tw=tw, test_system=test_system)
    )
    return False, go_stats


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
    python_home_3=None,
    cpus=None,
    timeout=180,
    cache=True,
    test_run_name="",
    result_json=DEFAULT_TEST_OUTPUT_JSON,
    rerun_fails=None,
    go_mod="readonly",
    junit_tar="",
    only_modified_packages=False,
    only_impacted_packages=False,
    skip_flakes=False,
    build_stdlib=False,
    test_washer=False,
    extra_args=None,
    skip_bazel_tests=False,
    save_bazel_tests=None,
    run_bazel_tests=False,
    run_on=None,  # noqa: U100, F841. Used by the run_on_devcontainer decorator
):
    """
    Run go tests on the given module and targets.

    A module should be provided as the path to one of the go modules in the repository.

    Targets should be provided as a comma-separated list of relative paths within the given module.
    If targets are provided but no module is set, the main module (".") is used.

    If no module or target is set the tests are run against all modules and targets.

    Example invokation:
        dda inv test --targets=./pkg/collector/check,./pkg/aggregator --race
        dda inv test --module=. --race
    """
    sanitize_env_vars()

    # Allow opting in to bazel integration via environment variable.
    _bazel_env = os.environ.get("EXPERIMENTAL_USE_BAZEL_TESTS", "")
    if _bazel_env not in ("", "0"):
        skip_bazel_tests = True
        run_bazel_tests = True

    modules, flavor = process_input_args(ctx, module, targets, flavor)

    unit_tests_tags = compute_build_tags_for_flavor(
        flavor=flavor,
        build="unit-tests",
        build_include=build_include,
        build_exclude=build_exclude,
    )

    ldflags, gcflags, env = get_build_flags(
        ctx,
        rtloader_root=rtloader_root,
        python_home_3=python_home_3,
    )

    # Use stdout if no profile is set
    test_profiler = TestProfiler() if profile else None

    race_opt = "-race" if race else ""
    # atomic is quite expensive but it's the only way to run both the coverage and the race detector at the same time without getting false positives from the cover counter
    covermode_opt = "-covermode=" + ("atomic" if race else "count") if coverage else ""
    build_cpus_opt = f"-p {cpus}" if cpus else ""
    test_cpus_opt = f"-parallel {cpus}" if cpus else ""
    trimpath_opt = "-trimpath" if 'DELVE' not in os.environ else ""

    nocache = '-count=1' if not cache else ''

    # Create temporary file for flaky patterns config
    if os.environ.get("FLAKY_PATTERNS_CONFIG"):
        with open(os.environ.get("FLAKY_PATTERNS_CONFIG"), 'w') as f:
            f.write("{}")

    if race:
        gorace = os.getenv("GORACE", "")
        if "atexit_sleep_ms" not in gorace:
            # https://go.dev/doc/articles/race_detector#Options
            # The default is 1000ms, which adds minutes to the full test run
            gorace += " atexit_sleep_ms=50"
            env["GORACE"] = gorace.strip()

    if result_json and os.path.isfile(result_json):
        # Remove existing file since we append to it.
        print(f"Removing existing '{result_json}' file")
        os.remove(result_json)

    test_run_arg = f"-run {test_run_name}" if test_run_name else ""

    # build flags are used both for building the stdlib and to run the tests
    gobuild_flags = '-mod={go_mod} -tags "{go_build_tags}" -gcflags="{gcflags}" -ldflags="{ldflags}" {build_cpus} {race_opt} {trimpath_opt}'

    stdlib_build_cmd = f'go build {{verbose}} {gobuild_flags} std cmd'
    rerun_coverage_fix = '--raw-command {cov_test_path}' if coverage else ""
    gotestsum_flags = (
        '{junit_file_flag} {json_flag} --format {gotestsum_format} {rerun_fails} --packages="{packages}" '
        + rerun_coverage_fix
    )
    govet_flags = '-vet=off'
    gotest_flags = (
        '{verbose} {test_cpus} -timeout {timeout}s -short {covermode_opt} {test_run_arg} {nocache} {extra_args}'
    )
    cmd = f'gotestsum {gotestsum_flags} -- {gobuild_flags} {govet_flags} {gotest_flags}'
    args = {
        "go_mod": go_mod,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "race_opt": race_opt,
        "build_cpus": build_cpus_opt,
        "test_cpus": test_cpus_opt,
        "covermode_opt": covermode_opt,
        "test_run_arg": test_run_arg,
        "timeout": int(timeout),
        "verbose": '-v' if verbose else '',
        "nocache": nocache,
        # Used to print failed tests at the end of the go test command
        "rerun_fails": f"--rerun-fails={rerun_fails}" if rerun_fails else "",
        "skip_flakes": "--skip-flake" if skip_flakes else "",
        "gotestsum_format": "standard-verbose" if verbose else "pkgname",
        "extra_args": extra_args or "",
        "trimpath_opt": trimpath_opt,
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

    exclude_packages: set[str] = set()
    bazel_targets: dict[str, str] = {}
    if skip_bazel_tests or save_bazel_tests or run_bazel_tests:
        bazel_targets = get_bazel_test_targets(ctx, list(modules))
        print(f"Found {len(bazel_targets)} Bazel-covered go_test targets")

        if save_bazel_tests:
            with open(save_bazel_tests, 'w') as f:
                f.write('\n'.join(sorted(bazel_targets)) + '\n')
            print(f"Bazel test targets written to {save_bazel_tests}")

        if skip_bazel_tests:
            exclude_packages = set(bazel_targets.values())
            print(f"Skipping {len(exclude_packages)} Bazel-covered packages from go test")

    with gitlab_section("Running unit tests", collapsed=True):
        result_junit = f"junit-out-{flavor}.xml" if junit_tar else ""
        test_result = test_flavor(
            ctx,
            flavor=flavor,
            build_tags=unit_tests_tags,
            modules=modules,
            cmd=cmd,
            env=env,
            args=args,
            result_junit=result_junit,
            result_json=result_json,
            test_profiler=test_profiler,
            coverage=coverage,
            recursive=not only_modified_packages,  # Disable recursive tests when only modified packages is enabled, to avoid testing a package and all its subpackages
            exclude_packages=exclude_packages or None,
        )

    # Go test output (only if tests ran)
    go_success = True
    go_stats: TestStats | None = None
    if test_result:
        if coverage and print_coverage:
            coverage_flavor(ctx)

        # FIXME(AP-1958): this prints nothing in CI. Commenting out the print line
        # in the meantime to avoid confusion
        if profile:
            # print("\n--- Top 15 packages sorted by run time:")
            test_profiler.print_sorted(15)

        go_success, go_stats = process_test_result(
            ctx,
            test_result,
            junit_tar,
            [result_junit],
            flavor,
            test_washer,
        )

    # Bazel test output — displayed after go test results.
    bazel_success = True
    bazel_stats: TestStats | None = None
    if run_bazel_tests and bazel_targets:
        print(f"\n{'=' * 12} Bazel tests {'=' * 12}")
        with gitlab_section("Bazel test results", collapsed=True):
            bazel_stats = _run_bazel_tests(ctx, list(bazel_targets), verbose=verbose)
        bazel_success = bazel_stats.failed == 0
        bazel_status = (
            color_message('All tests passed', 'green') if bazel_success else color_message('Tests FAILED', 'red')
        )
        print(f"DONE {bazel_stats.total} tests in {bazel_stats.duration_s:.3f}s")
        print(bazel_status)

    # Combined summary in the same style as the go Test Report block.
    if run_bazel_tests and (go_stats is not None or bazel_stats is not None):
        combined = (go_stats or TestStats()) + (bazel_stats or TestStats())
        all_ok = go_success and bazel_success
        sep = "=" * 60
        result_str = color_message("PASSED", "green") if all_ok else color_message("FAILED", "red")
        parts = [f"{combined.total} total", f"{combined.passed} passed"]
        if combined.failed:
            parts.append(f"{combined.failed} failed")
        if combined.skipped:
            parts.append(f"{combined.skipped} skipped")
        parts.append(f"in {combined.duration_s:.1f}s")
        print(f"\n{sep}")
        print(f"  Test Report (unit + bazel) \u2014 {result_str}")
        print(sep)
        print()
        print("  ".join(parts))

    if not go_success or not bazel_success:
        raise Exit(code=1)

    if test_result and not run_bazel_tests:
        print(f"Tests final status (including re-runs): {color_message('ALL TESTS PASSED', 'green')}")


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
        if modified_file.endswith(".mod") or modified_file.endswith(".sum"):
            continue

        best_module_path = Path(get_go_module(modified_file))

        # Check if the package is in the target list of the module we want to test
        targeted = False

        assert best_module_path, f"No module found for {modified_file}"
        module = get_module_by_path(best_module_path)

        if not module.should_test():
            continue

        targets = module.lint_targets if lint else module.test_targets

        for target in targets:
            if os.path.normpath(os.path.join(best_module_path, target)) in modified_file:
                targeted = True
                break
        if not targeted:
            continue

        # If go mod was modified in the module we run the test for the whole module so we do not need to add modified packages to targets
        if best_module_path in go_mod_modified_modules:
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
                modules_to_test[best_module_path].test_targets is not None
                and os.path.dirname(modified_file) not in modules_to_test[best_module_path].test_targets
            ):
                modules_to_test[best_module_path].test_targets.append(relative_target)
        else:
            modules_to_test[best_module_path] = GoModule(best_module_path, test_targets=[relative_target])

    # Clean up duplicated paths to reduce Go test cmd length
    default_modules = get_default_modules()
    for module in modules_to_test:
        modules_to_test[module].test_targets = clean_nested_paths(modules_to_test[module].test_targets)
        if (
            len(modules_to_test[module].test_targets) >= WINDOWS_MAX_PACKAGES_NUMBER
        ):  # With more packages we can reach the limit of the command line length on Windows
            modules_to_test[module].test_targets = default_modules[module].test_targets

    if not modules_to_test:
        print("No modules to test")
    else:
        print("Running tests for the following modules:")
        for module in modules_to_test:
            print(f"- {module}: {modules_to_test[module].test_targets}")

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
    obj: ResultJson = ResultJson.from_file(log_file)
    failed_tests = [
        f"{package}/{test_name}"
        for package, tests in obj.failing_tests.items()
        for test_name in tests
        if test_name != "_"  # Exclude package-level failures
    ]

    n_test_executed = len([line for line in obj.lines if line.action in (ActionType.PASS, ActionType.FAIL)])
    return failed_tests, n_test_executed


@task
def get_impacted_packages(ctx, build_tags=None):
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
        return get_default_modules().values()

    if build_tags is None:
        build_tags = []
    dependencies = create_dependencies(ctx, build_tags)
    base_branch = _get_release_json_value("base_branch")
    files = get_modified_files(ctx, base_branch=base_branch)
    print(f"Detected the following modified files: {files}")
    # Only .go files directly identify their containing directory as a Go package.
    # Non-Go files (fixtures, Cargo.toml, etc.) are resolved to the nearest
    # ancestor Go package by the walk-up loop below.
    modified_packages = {
        f"github.com/DataDog/datadog-agent/{os.path.dirname(file)}" for file in files if file.endswith(".go")
    }

    # Modification to go.mod and go.sum should force the tests of the whole module to run
    for file in files:
        if file.endswith("go.mod") or file.endswith("go.sum"):
            with ctx.cd(os.path.dirname(file)):
                all_packages = ctx.run(
                    f'go list -tags "{" ".join(build_tags)}" ./...', hide=True, warn=True
                ).stdout.splitlines()
                modified_packages.update(set(all_packages))

    # Modification to non-Go files (fixtures, testdata, etc.) count as
    # modification to the nearest ancestor package that contains Go sources.
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
    """Parallel version of create_dependencies using async ctx.run with batched execution"""
    if build_tags is None:
        build_tags = []

    modules_deps = defaultdict(set)
    modules = list(get_default_modules())

    # Process modules in batches of 16 to avoid too many open files errors
    batch_size = 8
    for i in range(0, len(modules), batch_size):
        batch_modules = modules[i : i + batch_size]
        running_commands = []
        results = {}

        # Start commands for current batch asynchronously
        for module in batch_modules:
            with ctx.cd(module):
                cmd = (
                    'go list -buildvcs=false '
                    + f'-tags "{" ".join(build_tags)}" '
                    + '-f "{{.ImportPath}} {{.Imports}} {{.TestImports}}" ./...'
                )
                running_commands.append((module, ctx.run(cmd, hide=True, warn=True, asynchronous=True)))

        # Wait for all commands in current batch to complete
        for module, cmd in running_commands:
            try:
                result = cmd.join()
                if result.stdout:
                    results[module] = result.stdout
            except Exception as e:
                print(f"Error processing module {module}: {e}")
                continue

        # Process results from current batch
        for module, stdout in results.items():
            imports = stdout.splitlines()
            for imp in imports:
                if not imp:
                    continue
                try:
                    imp = imp.split(" ", 1)
                    if len(imp) != 2:
                        continue
                    package, imported_packages = imp[0], imp[1].replace("[", "").replace("]", "").split(" ")
                    for imported_package in imported_packages:
                        if imported_package and imported_package.startswith("github.com/DataDog/datadog-agent"):
                            modules_deps[imported_package].add(package)
                except Exception as e:
                    print(f"Error processing import {imp} in module {module}: {e}")
                    continue

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

    default_modules = get_default_modules()
    for package in packages:
        module_path = get_go_module(package)

        # Check if the module is in the target list of the modules we want to test
        if module_path not in default_modules or not default_modules[module_path].should_test():
            continue

        # Check if the package is in the target list of the module we want to test
        targeted = False
        for target in default_modules[module_path].test_targets:
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
            if (
                modules_to_test[module_path].test_targets is not None
                and package not in modules_to_test[module_path].test_targets
            ):
                modules_to_test[module_path].test_targets.append(relative_target)
        else:
            modules_to_test[module_path] = GoModule(module_path, test_targets=[relative_target])

    # Clean up duplicated paths to reduce Go test cmd length
    default_modules = get_default_modules()
    for module in modules_to_test:
        modules_to_test[module].test_targets = clean_nested_paths(modules_to_test[module].test_targets)
        if (
            len(modules_to_test[module].test_targets) >= WINDOWS_MAX_PACKAGES_NUMBER
        ):  # With more packages we can reach the limit of the command line length on Windows
            modules_to_test[module].test_targets = default_modules[module].test_targets

    module_to_remove = []
    # Clean up to avoid running tests on package with no Go files matching build tags
    for module in modules_to_test:
        with ctx.cd(module):
            res = ctx.run(
                f'go list -buildvcs=false -tags "{" ".join(build_tags)}" {" ".join([normpath(os.path.join("github.com/DataDog/datadog-agent", module, target)) for target in modules_to_test[module].test_targets])}',
                hide=True,
                warn=True,
            )
            if res is not None and res.stderr is not None:
                for package in res.stderr.splitlines():
                    package_to_remove = os.path.relpath(
                        package.split(" ")[1].strip(":").replace("github.com/DataDog/datadog-agent/", ""), module
                    ).replace("\\", "/")
                    try:
                        modules_to_test[module].test_targets.remove(f"./{package_to_remove}")
                        if len(modules_to_test[module].test_targets) == 0:
                            module_to_remove.append(module)
                    except Exception:
                        print("Could not remove ", package_to_remove, ", ignoring...")
    for module in module_to_remove:
        del modules_to_test[module]

    if not modules_to_test:
        print("No modules to test")
    else:
        print("Running tests for the following modules:")
        for module in modules_to_test:
            print(f"- {module}: {modules_to_test[module].test_targets}")

    # We need to make sure the CLI length is not too long
    packages = compute_gotestsum_cli_args(modules_to_test.values())
    # -1000 because there are ~1000 extra characters in the gotestsum command
    if sys.platform == "win32" and len(packages) > WINDOWS_MAX_CLI_LENGTH - 1000:
        print("CLI length is too long, skipping fast tests")
        return get_default_modules().values()

    return modules_to_test.values()


def normpath(path):  # Normpath with forward slashes to avoid issues on Windows
    return os.path.normpath(path).replace("\\", "/")


def get_go_module(path):
    while path != '/':
        go_mod_path = os.path.join(path, 'go.mod')
        if os.path.isfile(go_mod_path):
            return normpath(os.path.relpath(path))
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


def compute_gotestsum_cli_args(modules: list[GoModule], recursive: bool = True):
    targets = []
    for module in modules:
        if not module.should_test():
            continue
        for target in module.test_targets:
            target_path = os.path.join(module.path, target)
            if not target_path.startswith('./'):
                target_path = f"./{target_path}"
            targets.append(target_path)
    if recursive:
        packages = ' '.join(f"{t}/..." if not t.endswith("/...") else t for t in targets)
    else:
        packages = ' '.join(targets)
    return packages


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
    only_modified_packages=False,
):
    raise Exit("This task is deprecated, please use `dda inv linter.go`", 1)


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
def check_otel_module_versions(ctx, fix=False):
    # Get Go version from upstream (e.g., "1.24" or "1.24.0")
    upstream_pattern = r"^go (1(?:\.\d+){1,2})[\r]?$"
    r = requests.get(OTEL_UPSTREAM_GO_MOD_PATH)
    upstream_matches = re.findall(upstream_pattern, r.text, flags=re.MULTILINE)
    if len(upstream_matches) != 1:
        raise Exit(f"Error parsing upstream go.mod version: {OTEL_UPSTREAM_GO_MOD_PATH}")
    upstream_go_version = upstream_matches[0]

    expected_local_version = upstream_go_version
    if expected_local_version.count('.') == 1:
        expected_local_version += '.0'

    # Pattern to match major.minor.patch format in local modules
    local_pattern = f"^go ({PATTERN_MAJOR_MINOR}\\.\\d+)\r?$"

    # Collect all errors instead of failing at the first one
    format_errors = []
    version_errors = []

    for path, module in get_default_modules().items():
        if module.used_by_otel:
            mod_file = f"./{path}/go.mod"
            with open(mod_file, newline='', encoding='utf-8') as reader:
                content = reader.read()
                local_matches = re.findall(local_pattern, content, flags=re.MULTILINE)
                if len(local_matches) != 1:
                    format_errors.append(f"{mod_file} does not match expected go directive format")
                    continue

                actual_local_version = local_matches[0]
                if actual_local_version != expected_local_version:
                    if fix:
                        update_file(
                            True,
                            mod_file,
                            f"^go {PATTERN_MAJOR_MINOR}\\.\\d+\r?$",
                            f"go {expected_local_version}",
                        )
                    else:
                        version_errors.append(
                            f"{mod_file} version {actual_local_version} does not match expected version: {expected_local_version} (derived from upstream {upstream_go_version})"
                        )

    # Report all errors at once if any were found
    all_errors = format_errors + version_errors
    if all_errors:
        error_msg = "Found the following OTEL module version issues:\n" + "\n".join(
            f"  - {error}" for error in all_errors
        )
        raise Exit(error_msg)
