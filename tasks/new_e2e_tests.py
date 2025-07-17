"""
Running E2E Tests with infra based on Pulumi
"""

from __future__ import annotations

import json
import multiprocessing
import os
import os.path
import re
import shutil
import tempfile
import threading
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

import yaml
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.flavor import AgentFlavor
from tasks.gotest import process_test_result, test_flavor
from tasks.libs.common.color import Color
from tasks.libs.common.git import get_commit_sha
from tasks.libs.common.go import download_go_dependencies
from tasks.libs.common.gomodules import get_default_modules
from tasks.libs.common.utils import (
    REPO_PATH,
    color_message,
    gitlab_section,
    running_in_ci,
)
from tasks.libs.testing.e2e import create_test_selection_gotest_regex, filter_only_leaf_tests
from tasks.libs.testing.result_json import ActionType, ResultJson
from tasks.test_core import DEFAULT_E2E_TEST_OUTPUT_JSON
from tasks.testwasher import TestWasher
from tasks.tools.e2e_stacks import destroy_remote_stack


class TestState:
    """Describes the state of a test, if it has failed and if it is flaky."""

    FAILED = True, False
    FLAKY_FAILED = True, True
    SUCCESS = False, False
    FLAKY_SUCCESS = False, True

    @staticmethod
    def get_human_readable_state(failing: bool, flaky: bool) -> str:
        return f'{"Failing" if failing else "Successful"} / {"Flaky" if flaky else "Non-flaky"}'


def _build_single_binary(ctx, pkg, build_tags, output_path, print_lock):
    """
    Build a single test binary for the given package.
    Returns (pkg, success, message) tuple.
    """
    try:
        # Create binary name from package path
        binary_name = pkg.replace("/", "-").replace("\\", "-") + ".test"
        binary_path = output_path / binary_name

        # Build test binary
        cmd = f"go test -c -tags '{build_tags}' -ldflags='-w -s -X {REPO_PATH}/test/new-e2e/tests/containers.GitCommit={get_commit_sha(ctx, short=True)}' -o {binary_path} ./{pkg}"

        result = ctx.run(cmd, hide=True)
        if result.ok:
            with print_lock:
                print(f"  ✓ Built {binary_name}")
            return (pkg, True, f"Built {binary_name}")
        else:
            with print_lock:
                print(f"  ✗ Failed to build {binary_name}: {result.stderr}")
            return (pkg, False, f"Failed to build {binary_name}: {result.stderr}")

    except Exception as e:
        with print_lock:
            print(f"  ✗ Error building {binary_name}: {e}")
        return (pkg, False, f"Error building {binary_name}: {e}")


@task(
    help={
        "output_dir": "Directory to store compiled test binaries",
        "tags": "Build tags to use",
        "parallel": "Number of parallel builds [default: number of CPUs]",
    },
)
def build_binaries(
    ctx,
    output_dir="test-binaries",
    manifest_file_path="manifest.json",
    tags=[],  # noqa: B006
    parallel=0,
):
    """
    Build E2E test binaries for all test packages to be reused across test jobs.
    This pre-builds all test binaries to optimize CI pipeline performance.
    """

    if parallel == 0:
        parallel = multiprocessing.cpu_count()

    print(f"Building test binaries using {parallel} parallel workers")

    e2e_test_dir = Path("test/new-e2e/tests")
    output_path = Path(output_dir).absolute()

    # Create output directory
    output_path.mkdir(exist_ok=True, parents=True)

    # Find all test packages
    test_packages = []
    for root, _, files in os.walk(e2e_test_dir):
        # Check if directory contains Go test files
        has_go_tests = any(f.endswith("_test.go") for f in files)
        if has_go_tests:
            # Convert to Go package path
            pkg_path = os.path.relpath(root, "./test/new-e2e")
            test_packages.append(pkg_path)

    if not test_packages:
        print("No test packages found")
        return

    print(f"Found {len(test_packages)} test packages to build")

    # Build tags
    build_tags = ",".join(tags) if tags else "test"

    # Build test binaries in parallel
    print_lock = threading.Lock()
    success_count = 0
    failure_count = 0
    built_packages = []  # Track successfully built packages with their info
    with ctx.cd("test/new-e2e"):
        with ThreadPoolExecutor(max_workers=parallel) as executor:
            # Submit all build jobs
            futures = {
                executor.submit(_build_single_binary, ctx, pkg, build_tags, output_path, print_lock): pkg
                for pkg in test_packages
            }

            # Process completed builds
            for i, future in enumerate(as_completed(futures), 1):
                pkg = futures[future]
                try:
                    pkg_result, success, message = future.result()
                    if success:
                        success_count += 1

                    else:
                        failure_count += 1

                    # Even if it failed to build, we still want to add it to the manifest, so that we know something is missing in the test execution
                    binary_name = pkg.replace("/", "-").replace("\\", "-") + ".test"
                    built_packages.append((pkg_result, binary_name))

                    # Print progress
                    with print_lock:
                        print(f"Progress: {i}/{len(test_packages)} completed")

                except Exception as e:
                    failure_count += 1
                    with print_lock:
                        print(f"  ✗ Unexpected error building {pkg}: {e}")

    print(f"\nBuild completed: {success_count} successful, {failure_count} failed")
    print(f"Test binaries built in: {output_path.absolute()}")

    # Create manifest file
    manifest = {
        "build_info": {
            "timestamp": ctx.run("date -u +%Y-%m-%dT%H:%M:%SZ", hide=True).stdout.strip(),
            "commit": get_commit_sha(ctx, short=True),
            "build_tags": build_tags,
            "parallel_workers": parallel,
            "success_count": success_count,
            "failure_count": failure_count,
        },
        "binaries": [],
    }

    # Use the original package paths from the build process
    for pkg_path, binary_name in built_packages:
        binary_file = output_path / binary_name
        if binary_file.exists():
            manifest["binaries"].append(
                {
                    "package": pkg_path,
                    "binary": binary_name,
                    "size": binary_file.stat().st_size,
                }
            )

    with open(manifest_file_path, "w") as f:
        json.dump(manifest, f, indent=2)

    print(f"Manifest created: {manifest_file_path}")

    if failure_count > 0:
        print(f"Error: {failure_count} packages failed to build")
        raise Exit(code=1)


@task(
    iterable=['tags', 'targets', 'configparams', 'run', 'skip'],
    help={
        "profile": "Override auto-detected runner profile (local or CI)",
        "tags": "Build tags to use",
        "targets": "Target packages (same as dda inv test)",
        "configparams": "Set overrides for ConfigMap parameters (same as -c option in test-infra-definitions)",
        "verbose": "Verbose output: log all tests as they are run (same as gotest -v) [default: True]",
        "run": "Only run tests matching the regular expression",
        "skip": "Only run tests not matching the regular expression",
        "agent_image": 'Full image path for the agent image (e.g. "repository:tag") to run the e2e tests with',
        "cluster_agent_image": 'Full image path for the cluster agent image (e.g. "repository:tag") to run the e2e tests with',
        "stack_name_suffix": "Suffix to add to the stack name, it can be useful when your stack is stuck in a weird state and you need to run the tests again",
        "use_prebuilt_binaries": "Use pre-built test binaries instead of building on the fly",
        "max_retries": "Maximum number of retries for failed tests, default 3",
    },
)
def run(
    ctx,
    profile="",
    tags=[],  # noqa: B006
    targets=[],  # noqa: B006
    configparams=[],  # noqa: B006
    verbose=True,
    run=[],  # noqa: B006
    skip=[],  # noqa: B006
    osversion="",
    platform="",
    arch="",
    flavor="",
    major_version="",
    cws_supported_osversion="",
    src_agent_version="",
    dest_agent_version="",
    keep_stacks=False,
    extra_flags="",
    cache=False,
    junit_tar="",
    test_run_name="",
    test_washer=False,
    agent_image="",
    cluster_agent_image="",
    logs_post_processing=False,
    logs_post_processing_test_depth=1,
    logs_folder="e2e_logs",
    local_package="",
    result_json=DEFAULT_E2E_TEST_OUTPUT_JSON,
    stack_name_suffix="",
    use_prebuilt_binaries=False,
    max_retries=0,
):
    """
    Run E2E Tests based on test-infra-definitions infrastructure provisioning.
    """

    if shutil.which("pulumi") is None:
        raise Exit(
            "pulumi CLI not found, Pulumi needs to be installed on the system (see https://github.com/DataDog/test-infra-definitions/blob/main/README.md)",
            1,
        )

    e2e_module = get_default_modules()["test/new-e2e"]
    e2e_module.should_test_condition = "always"
    if targets:
        e2e_module.test_targets = targets

    env_vars = {}
    if profile:
        env_vars["E2E_PROFILE"] = profile

    parsed_params = {}

    # Outside of CI try to automatically configure the secret to pull agent image
    if not running_in_ci():
        # Authentication against agent-qa is required for all kubernetes tests, to use the cache
        parsed_params["ddagent:imagePullPassword"] = ctx.run(
            "aws-vault exec sso-agent-qa-read-only -- aws ecr get-login-password", hide=True
        ).stdout.strip()
        parsed_params["ddagent:imagePullRegistry"] = "669783387624.dkr.ecr.us-east-1.amazonaws.com"
        parsed_params["ddagent:imagePullUsername"] = "AWS"
        # If we use an agent image from sandbox registry we need to authenticate against it
        if "376334461865" in agent_image or "376334461865" in cluster_agent_image:
            parsed_params["ddagent:imagePullPassword"] += (
                f",{ctx.run('aws-vault exec sso-agent-sandbox-account-admin -- aws ecr get-login-password', hide=True).stdout.strip()}"
            )
            parsed_params["ddagent:imagePullRegistry"] += ",376334461865.dkr.ecr.us-east-1.amazonaws.com"
            parsed_params["ddagent:imagePullUsername"] += ",AWS"

    for param in configparams:
        parts = param.split("=", 1)
        if len(parts) != 2:
            raise Exit(
                message=f"wrong format given for config parameter, expects key=value, actual: {param}",
                code=1,
            )
        parsed_params[parts[0]] = parts[1]

    if local_package:
        parsed_params["ddagent:localPackage"] = Path(local_package).absolute().as_posix()

    if agent_image:
        parsed_params["ddagent:fullImagePath"] = agent_image

    if cluster_agent_image:
        parsed_params["ddagent:clusterAgentFullImagePath"] = cluster_agent_image

    if parsed_params:
        env_vars["E2E_STACK_PARAMS"] = json.dumps(parsed_params)

    if stack_name_suffix:
        env_vars["E2E_STACK_NAME_SUFFIX"] = stack_name_suffix

    gotestsum_format = "standard-verbose" if verbose else "pkgname"

    test_run_arg = ""
    if test_run_name != "":
        test_run_arg = f"-run {test_run_name}"

    # Create temporary file for flaky patterns config
    if os.environ.get("FLAKY_PATTERNS_CONFIG"):
        if os.path.exists(os.environ.get("FLAKY_PATTERNS_CONFIG")):
            os.remove(os.environ.get("FLAKY_PATTERNS_CONFIG"))
        with open(os.environ.get("FLAKY_PATTERNS_CONFIG"), 'a') as f:
            f.write("{}")

    cmd = f"gotestsum --format {gotestsum_format} "
    raw_command = ""
    # Scrub the test output to avoid leaking API or APP keys when running in the CI

    if use_prebuilt_binaries:
        if not os.path.exists("test-binaries.tar.gz") or not os.path.exists("manifest.json"):
            print(
                "WARNING: required artifacts test-binaries.tar.gz and manifest.json not found, disabling use_prebuilt_binaries"
            )
            use_prebuilt_binaries = False

    if use_prebuilt_binaries:
        ctx.run("go build -o ./gotest-custom ./internal/tools/gotest-custom")
        raw_command = "--raw-command ./gotest-custom {packages}"
        env_vars["GOTEST_COMMAND"] = "./gotest-custom"

    if running_in_ci():
        raw_command = (
            # Using custom go command piped with scrubber sed instructions https://github.com/gotestyourself/gotestsum#custom-go-test-command
            f"--raw-command {os.path.join(os.path.dirname(__file__), 'tools', 'gotest-scrubbed.sh')} {{packages}}"
        )

    cmd += f'{{junit_file_flag}} {{json_flag}} --packages="{{packages}}" {raw_command} -- -ldflags="-X {{REPO_PATH}}/test/new-e2e/tests/containers.GitCommit={{commit}}" {{verbose}} -mod={{go_mod}} -vet=off -timeout {{timeout}} -tags "{{go_build_tags}}" {{nocache}} {{run}} {{skip}} {{test_run_arg}} -args {{osversion}} {{platform}} {{major_version}} {{arch}} {{flavor}} {{cws_supported_osversion}} {{src_agent_version}} {{dest_agent_version}} {{keep_stacks}} {{extra_flags}}'
    # Strinbuilt_binaries:gs can come with extra double-quotes which can break the command, remove them
    clean_run = []
    clean_skip = []
    for r in run:
        clean_run.append(r.replace('"', ''))
    for s in skip:
        clean_skip.append(s.replace('"', ''))

    args = {
        "go_mod": "readonly",
        "timeout": "4h",
        "verbose": "-test.v" if verbose else "",
        "nocache": "-test.count=1" if not cache else "",
        "REPO_PATH": REPO_PATH,
        "commit": get_commit_sha(ctx, short=True),
        "run": '-test.run ' + '"{}"'.format('|'.join(clean_run)) if run else '',
        "skip": '-test.skip ' + '"{}"'.format('|'.join(clean_skip)) if skip else '',
        "test_run_arg": test_run_arg,
        "osversion": f"-osversion {osversion}" if osversion else "",
        "platform": f"-platform {platform}" if platform else "",
        "arch": f"-arch {arch}" if arch else "",
        "flavor": f"-flavor {flavor}" if flavor else "",
        "major_version": f"-major-version {major_version}" if major_version else "",
        "cws_supported_osversion": f"-cws-supported-osversion {cws_supported_osversion}"
        if cws_supported_osversion
        else "",
        "src_agent_version": f"-src-agent-version {src_agent_version}" if src_agent_version else "",
        "dest_agent_version": f"-dest-agent-version {dest_agent_version}" if dest_agent_version else "",
        "keep_stacks": '-keep-stacks' if keep_stacks else "",
        "extra_flags": extra_flags,
    }

    to_teardown: set[tuple[str, str]] = set()
    result_jsons: list[str] = []
    result_junits: list[str] = []
    for attempt in range(max_retries + 1):
        remaining_tries = max_retries - attempt
        if remaining_tries > 0:
            # If any tries are left, avoid destroying infra on failure
            env_vars["E2E_SKIP_DELETE_ON_FAILURE"] = "true"
        else:
            env_vars.pop("E2E_SKIP_DELETE_ON_FAILURE", None)

        partial_result_json = f"{result_json}.{attempt}.part"
        result_jsons.append(partial_result_json)

        partial_result_junit = f"junit-out-{str(AgentFlavor.base)}-{attempt}.xml"
        result_junits.append(partial_result_junit)

        test_res = test_flavor(
            ctx,
            flavor=AgentFlavor.base,
            build_tags=tags,
            modules=[e2e_module],
            args=args,
            cmd=cmd,
            env=env_vars,
            result_junit=partial_result_junit,
            result_json=partial_result_json,
            test_profiler=None,
        )

        washer = TestWasher(test_output_json_file=partial_result_json)

        if remaining_tries > 0:
            failed_tests = filter_only_leaf_tests(
                (package, test_name) for package, tests in washer.get_failing_tests().items() for test_name in tests
            )

            # Note: `get_flaky_failures` can return some unexpected things due to its logic for detecting failing tests by looking at its eventual children.
            # By using an `intersection` we ensure that we only get tests that have actually failed.
            known_flaky_failures = failed_tests.intersection(
                {(package, test_name) for package, tests in washer.get_flaky_failures().items() for test_name in tests}
            )

            # Retry any failed tests that are not known to be flaky
            to_retry = failed_tests - known_flaky_failures

            if known_flaky_failures:
                print(
                    color_message(
                        f"{len(known_flaky_failures)} tests failed but are known flaky. They will not be retried !",
                        "yellow",
                    )
                )
                # Schedule teardown for all known flaky failures, so that they are not left hanging after the retry loop
                to_teardown.update(known_flaky_failures)

            if to_retry:
                failed_tests_printout = '\n- '.join(f'{package} {test_name}' for package, test_name in sorted(to_retry))
                print(
                    color_message(
                        f"Retrying {len(to_retry)} failed tests:\n- {failed_tests_printout}",
                        "yellow",
                    )
                )

                # Retry the failed tests only
                affected_packages = {
                    os.path.relpath(package, "github.com/DataDog/datadog-agent/test/new-e2e/")
                    for package, _ in to_retry
                }
                e2e_module.test_targets = list(affected_packages)
                args["run"] = '-test.run ' + create_test_selection_gotest_regex([test for _, test in to_retry])
            else:
                break

    # Make sure that any non-successful test suites that were not retried (i.e., fully-known-flaky-failing suites) are torn down
    # Do this by calling the tests with the E2E_TEARDOWN_ONLY env var set, which will only run the teardown logic
    if to_teardown:
        print(
            color_message(
                f"Tearing down {len(to_teardown)} leftover test infras",
                "yellow",
            )
        )
        affected_packages = {
            os.path.relpath(package, "github.com/DataDog/datadog-agent/test/new-e2e/") for package, _ in to_teardown
        }
        e2e_module.test_targets = list(affected_packages)
        args["run"] = '-test.run ' + create_test_selection_gotest_regex([test for _, test in to_teardown])
        env_vars["E2E_TEARDOWN_ONLY"] = "true"
        test_flavor(
            ctx,
            flavor=AgentFlavor.base,
            build_tags=tags,
            modules=[e2e_module],
            args=args,
            cmd=cmd,
            env=env_vars,
            result_junit="",  # No need to store JUnit results for teardown-only runs
            result_json="",  # No need to store results for teardown-only runs
            test_profiler=None,
        )

    # Merge all the partial result JSON files into the final result JSON
    with open(result_json, "w") as merged_file:
        for partial_file in result_jsons:
            with open(partial_file) as f:
                merged_file.writelines(line.strip() + "\n" for line in f.readlines())

    success = process_test_result(test_res, junit_tar, result_junits, AgentFlavor.base, test_washer)

    if running_in_ci():
        # Do not print all the params, they could contain secrets needed only in the CI
        params = [f"--targets {t}" for t in targets]

        param_keys = ("osversion", "platform", "arch")
        for param_key in param_keys:
            if args.get(param_key):
                params.append(f"-{args[param_key]}")

        configparams_to_retain = {
            "ddagent:imagePullRegistry",
            "ddagent:imagePullUsername",
        }

        registry_to_password_commands = {
            "669783387624.dkr.ecr.us-east-1.amazonaws.com": "aws-vault exec sso-agent-qa-read-only -- aws ecr get-login-password"
        }

        for configparam in configparams:
            parts = configparam.split("=", 1)
            key = parts[0]
            if key in configparams_to_retain:
                params.append(f"-c {configparam}")

                if key == "ddagent:imagePullRegistry" and len(parts) > 1:
                    registry = parts[1]
                    password_cmd = registry_to_password_commands.get(registry)
                    if password_cmd is not None:
                        params.append(f"-c ddagent:imagePullPassword=$({password_cmd})")

        command = f"E2E_PIPELINE_ID={os.environ.get('CI_PIPELINE_ID')} E2E_COMMIT_SHA={os.environ.get('CI_COMMIT_SHORT_SHA')} dda inv -- -e new-e2e-tests.run {' '.join(params)}"
        print(
            f"To run this test locally, use: `{command}`. "
            'You can also add `E2E_DEV_MODE="true"` to run in dev mode which will leave the environment up after the tests.'
            "\nYou can troubleshoot e2e test failures with this documentation: https://datadoghq.atlassian.net/wiki/x/7gIo0"
        )

    if logs_post_processing:
        post_processed_output = post_process_output(
            test_res.result_json_path, test_depth=logs_post_processing_test_depth
        )
        os.makedirs(logs_folder, exist_ok=True)
        write_result_to_log_files(
            post_processed_output,
            logs_folder,
            test_depth=logs_post_processing_test_depth,
        )

        pretty_print_logs(
            test_res.result_json_path,
            post_processed_output,
            test_depth=logs_post_processing_test_depth,
        )

    if not success:
        raise Exit(code=1)


@task(
    help={
        "locks": "Cleans up lock files, default True",
        "stacks": "Cleans up local stack state, default False",
        "output": "Cleans up local test output directory, default False",
        "skip_destroy": "Skip stack's resources removal. Use it only if your resources are already removed by other means, default False",
    },
)
def clean(ctx, locks=True, stacks=False, output=False, skip_destroy=False):
    """
    Clean any environment created with invoke tasks or e2e tests
    By default removes only lock files.
    """
    if not _is_local_state(_get_pulumi_about(ctx)):
        print("Cleanup supported for local state only, run `pulumi login --local` to switch to local state")
        return

    if locks:
        _clean_locks()
        if not stacks:
            print("If you still have issues, try running with -s option to clean up stacks")

    if stacks:
        _clean_stacks(ctx, skip_destroy)

    if output:
        _clean_output()


@task
def cleanup_remote_stacks(ctx, stack_regex, pulumi_backend):
    """
    Clean up remote stacks created by the pipeline
    """
    if not running_in_ci():
        raise Exit("This task should be run in CI only", 1)

    stack_regex = re.compile(stack_regex)

    # Ideally we'd use the pulumi CLI to list all the stacks. However we have way too much stacks in the bucket so the commands hang forever.
    # Once the bucket is cleaned up we can switch to the pulumi CLI
    res = ctx.run(
        "pulumi stack ls --all --json",
        hide=True,
        warn=True,
    )
    if res.exited != 0:
        print(f"Failed to list stacks in {pulumi_backend}:", res.stdout, res.stderr)
        return
    to_delete_stacks = set()
    stacks = json.loads(res.stdout)
    print(stacks)
    for stack in stacks:
        stack_id = (
            stack.get("name", "")
            .split("/")[-1]
            .replace(".json.bak", "")
            .replace(".json", "")
            .replace(".pulumi/stacks/e2eci", "")
        )
        if stack_regex.match(stack_id):
            to_delete_stacks.add(f"organization/e2eci/{stack_id}")

    if len(to_delete_stacks) == 0:
        print("No stacks to delete")
        return

    print("About to delete the following stacks:", to_delete_stacks)
    with multiprocessing.Pool(len(to_delete_stacks)) as pool:
        res = pool.map(destroy_remote_stack, to_delete_stacks)
        destroyed_stack = set()
        failed_stack = set()
        for r, stack in res:
            if r.returncode != 0:
                failed_stack.add(stack)
            else:
                destroyed_stack.add(stack)
            print(f"Stack {stack}: {r.stdout} {r.stderr}")

    for stack in destroyed_stack:
        print(f"Stack {stack} destroyed successfully")
    for stack in failed_stack:
        print(f"Failed to destroy stack {stack}")


def post_process_output(path: str, test_depth: int = 1) -> list[tuple[str, str, list[str]]]:
    """
    Post process the test results to add the test run name
    path: path to the test result json file
    test_depth: depth of the test name to consider

    By default the test_depth is set to 1, which means that the logs will be splitted depending on the test suite name.
    If we use a single test suite to run multiple tests we can increase the test_depth to split the logs per test.
    For example with:
    TestPackages/run_ubuntu
    TestPackages/run_centos
    TestPackages/run_debian
    We should set test_depth to 2 to avoid mixing all the logs of the different tested platform

    Returns:
        A list of (package name, test name, logs) tuples
    """

    def is_parent(parent: list[str], child: list[str]) -> bool:
        if len(parent) > len(child):
            return False

        for i, parent_part in enumerate(parent):
            if parent_part != child[i]:
                return False

        return True

    result_json = ResultJson.from_file(path)

    lines = [line for line in result_json.lines if line.output and line.test]

    tests: dict[tuple[str, str], list] = {(json_line.package, json_line.test): [] for json_line in lines}  # type: ignore

    # Used to preserve order, line where a test appeared first
    test_order = {(json_line.package, json_line.test): i for (i, json_line) in list(enumerate(lines))[::-1]}

    for json_line in lines:
        assert json_line.output and json_line.test  # Just making mypy happy
        if json_line.action == ActionType.OUTPUT:
            output: str = json_line.output
            if "===" in output:
                continue

            # Append logs to all children tests + this test
            current_test_name_splitted = json_line.test.split("/")
            for (package, test_name), logs in tests.items():
                if package != json_line.package:
                    continue

                if is_parent(current_test_name_splitted, test_name.split("/")):
                    logs.append(json_line.output)

    # Rebuild order
    return sorted(
        [(package, name, logs) for (package, name), logs in tests.items()],
        key=lambda x: test_order[x[:2]],
    )


def write_result_to_log_files(logs_per_test, log_folder, test_depth=1):
    # Merge tests given their depth
    # (package, test_name) -> logs
    merged_logs = defaultdict(list)
    for package, test_name, logs in logs_per_test:
        merged_logs[package, "/".join(test_name.split("/")[:test_depth])].extend(logs)

    for (package, test), logs in merged_logs.items():
        sanitized_package_name = re.sub(r"[^\w_. -]", "_", package)
        sanitized_test_name = re.sub(r"[^\w_. -]", "_", test)
        with open(f"{log_folder}/{sanitized_package_name}.{sanitized_test_name}.log", "w") as f:
            f.write("".join(logs))


class TooManyLogsError(Exception):
    pass


def pretty_print_test_logs(logs_per_test: dict[tuple[str, str], str], max_size):
    # Compute size in bytes of what we are about to print. If it exceeds max_size, we skip printing because it will make the Gitlab logs almost completely collapsed.
    # By default Gitlab has a limit of 500KB per job log, so we want to avoid printing too much.
    size = 0
    for logs in logs_per_test.values():
        size += len("".join(logs).encode())
    if size > max_size and running_in_ci():
        raise TooManyLogsError
    for (package, test), logs in logs_per_test.items():
        with gitlab_section("Complete logs for " + package + "." + test, collapsed=True):
            print("".join(logs).strip())

    return size


def pretty_print_logs(result_json_path, logs_per_test, max_size=250000, test_depth=1, flakes_files=None):
    """Pretty prints logs with a specific order.

    Print order:
        1. Failing and non flaky tests
        2. Failing and flaky tests
        3. Successful and non flaky tests
        4. Successful and flaky tests
    """
    if flakes_files is None:
        flakes_files = []

    washer = TestWasher(test_output_json_file=result_json_path, flakes_file_paths=flakes_files)
    failing_tests = washer.get_failing_tests()
    flaky_failures = washer.get_flaky_failures()

    try:
        # (failing, flaky) -> [(package, test_name, logs)]
        categorized_logs = defaultdict(list)

        # Split flaky / non flaky tests
        for package, test_name, logs in logs_per_test:
            # The name of the parent / nth parent if test_depth is lower than the test name depth
            group_name = "/".join(test_name.split("/")[:test_depth])

            package_flaky = flaky_failures.get(package, set())
            package_failing = failing_tests.get(package, set())

            # Flaky if one of its parents is flaky as well
            is_flaky = False
            for i in range(test_name.count("/") + 1):
                parent_name = "/".join(test_name.split("/")[: i + 1])
                if parent_name in package_flaky:
                    is_flaky = True
                    break

            state = test_name in package_failing, is_flaky
            categorized_logs[state].append((package, group_name, logs))

        for failing, flaky in [
            TestState.FAILED,
            TestState.FLAKY_FAILED,
            TestState.SUCCESS,
            TestState.FLAKY_SUCCESS,
        ]:
            logs_to_print = categorized_logs[failing, flaky]
            if not logs_to_print:
                continue

            # Merge tests given their depth
            # (package, test_name) -> logs
            merged_logs = defaultdict(list)
            for package, test_name, logs in logs_to_print:
                merged_logs[package, test_name].extend(logs)

            print(f"* {color_message(TestState.get_human_readable_state(failing, flaky), Color.BOLD)} job logs:")
            # Print till the size limit is reached
            max_size -= pretty_print_test_logs(merged_logs, max_size)
    except TooManyLogsError:
        print(
            color_message("WARNING", "yellow")
            + f": Too many logs to print, skipping logs printing to avoid Gitlab collapse. You can find your logs properly organized in the job artifacts: https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/{os.getenv('CI_JOB_ID')}/artifacts/browse/e2e-output/logs/"
        )


@task
def deps(ctx, verbose=False):
    """
    Setup Go dependencies
    """
    download_go_dependencies(ctx, paths=["test/new-e2e"], verbose=verbose, max_retry=3)


def _get_default_env():
    return {"PULUMI_SKIP_UPDATE_CHECK": "true"}


def _get_home_dir():
    # TODO: Go os.UserHomeDir() uses a different algorithm than Python Path.home()
    #       so a different directory may be returned in some cases.
    return Path.home()


def _load_test_infra_config():
    with open(_get_home_dir().joinpath(".test_infra_config.yaml")) as f:
        config = yaml.safe_load(f)
    return config


def _get_test_output_dir():
    config = _load_test_infra_config()
    # default is $HOME/e2e-output
    default_output_dir = _get_home_dir().joinpath("e2e-output")
    # read config option, if not set use default
    configParams = config.get("configParams", {})
    output_dir = configParams.get("outputDir", default_output_dir)
    return Path(output_dir)


def _clean_output():
    output_dir = _get_test_output_dir()
    print(f"🧹 Clean up output directory {output_dir}")

    if not output_dir.exists():
        # nothing to do if output directory does not exist
        return

    if not output_dir.is_dir():
        raise Exit(
            message=f"e2e-output directory {output_dir} is not a directory, aborting",
            code=1,
        )

    # sanity check to avoid deleting the wrong directory, e2e-output should only contain directories
    for entry in output_dir.iterdir():
        if not entry.is_dir():
            raise Exit(
                message=f"e2e-output directory {output_dir} contains more than just directories, aborting",
                code=1,
            )

    shutil.rmtree(output_dir)


def _clean_locks():
    print("🧹 Clean up lock files")
    lock_dir = os.path.join(Path.home(), ".pulumi", "locks")

    for entry in os.listdir(Path(lock_dir)):
        path = os.path.join(lock_dir, entry)
        if os.path.isdir(path):
            shutil.rmtree(path)
            print(f"🗑️  Deleted lock: {path}")
        elif os.path.isfile(path) and entry.endswith(".json"):
            os.remove(path)
            print(f"🗑️  Deleted lock: {path}")


def _clean_stacks(ctx: Context, skip_destroy: bool):
    print("🧹 Clean up stack")

    if not skip_destroy:
        stacks = _get_existing_stacks(ctx)
        for stack in stacks:
            print(f"🔥 Destroying stack {stack}")
            _destroy_stack(ctx, stack)

    # get stacks again as they may have changed after destroy
    stacks = _get_existing_stacks(ctx)
    for stack in stacks:
        print(f"🗑️ Removing stack {stack}")
        _remove_stack(ctx, stack)


def _get_existing_stacks(ctx: Context) -> list[str]:
    e2e_stacks: list[str] = []
    output = ctx.run(
        "pulumi stack ls --all --project e2elocal --json",
        hide=True,
        env=_get_default_env(),
    )
    if output is None or not output:
        return []
    stacks_data = json.loads(output.stdout)
    for stack in stacks_data:
        if "name" not in stack:
            print(f"Skipping stack {stack} as it does not have a name")
            continue
        stack_name = stack["name"]
        print(f"Adding stack {stack_name}")
        e2e_stacks.append(stack_name)
    return e2e_stacks


def _destroy_stack(ctx: Context, stack: str):
    # running in temp dir as this is where datadog-agent test
    # stacks are stored. It is expected to fail on stacks existing locally
    # with resources removed by agent-sandbox clean up job
    with ctx.cd(tempfile.gettempdir()):
        ret = ctx.run(
            f"pulumi destroy --stack {stack} --yes --remove --skip-preview",
            warn=True,
            hide=True,
            env=_get_default_env(),
        )
        if ret is not None and ret.exited != 0:
            if "No valid credential sources found" in ret.stdout:
                print(
                    "No valid credentials sources found, if you set the AWS_PROFILE environment variable ensure it is valid"
                )
                print(ret.stdout)
                raise Exit(
                    color_message(
                        f"Failed to destroy stack {stack}, no valid credentials sources found, if you set the AWS_PROFILE environment variable ensure it is valid",
                        "red",
                    ),
                    1,
                )
            # run with refresh on first destroy attempt failure
            ret = ctx.run(
                f"pulumi destroy --stack {stack} -r --yes --remove --skip-preview",
                warn=True,
                hide=True,
                env=_get_default_env(),
            )
        if ret is not None and ret.exited != 0:
            raise Exit(
                color_message(f"Failed to destroy stack {stack}: {ret.stdout, ret.stderr}", "red"),
                1,
            )


def _remove_stack(ctx: Context, stack: str):
    ctx.run(
        f"pulumi stack rm --force --yes --stack {stack}",
        hide=True,
        env=_get_default_env(),
    )


def _get_pulumi_about(ctx: Context) -> dict:
    output = ctx.run("pulumi about --json", hide=True, env=_get_default_env())
    if output is None or not output:
        return {}
    return json.loads(output.stdout)


def _is_local_state(pulumi_about: dict) -> bool:
    # check output contains
    # Backend
    # Name           xxxxxxxxxx
    # URL            file://xxx
    # User           xxxxx.xxxxx
    # Organizations
    backend_group = pulumi_about.get("backend")
    if backend_group is None or not isinstance(backend_group, dict):
        return False
    url = backend_group.get("url")
    if url is None or not isinstance(url, str):
        return False
    return url.startswith("file://")
