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
import sys
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
from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.color import Color
from tasks.libs.common.git import get_commit_sha, get_current_branch, get_modified_files
from tasks.libs.common.go import download_go_dependencies
from tasks.libs.common.gomodules import get_default_modules
from tasks.libs.common.utils import (
    REPO_PATH,
    color_message,
    environ,
    gitlab_section,
    running_in_ci,
)
from tasks.libs.dynamic_test.backend import S3Backend
from tasks.libs.dynamic_test.executor import DynTestExecutor
from tasks.libs.dynamic_test.index import IndexKind
from tasks.libs.releasing.json import load_release_json
from tasks.libs.releasing.version import get_version
from tasks.libs.testing.e2e import create_test_selection_gotest_regex, filter_only_leaf_tests
from tasks.libs.testing.result_json import ActionType, ResultJson
from tasks.test_core import DEFAULT_E2E_TEST_OUTPUT_JSON
from tasks.testwasher import TestWasher
from tasks.tools.e2e_stacks import destroy_remote_stack_api, destroy_remote_stack_local

DEFAULT_DYNTEST_BUCKET_URI = "s3://dd-ci-persistent-artefacts-build-stable/datadog-agent"


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
        cmd = f"orchestrion go test -c -tags '{build_tags}' -ldflags='-w -s -X {REPO_PATH}/test/new-e2e/tests/containers.GitCommit={get_commit_sha(ctx, short=True)}' -o {binary_path} ./{pkg}"

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
    if "test" not in tags:
        tags = tags + ["test"]

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
        "impacted": "Only run tests that are impacted by the changes (only available in CI for now)",
        "keep_stack": "Keep the stack after running the test, you are responsible for destroying the stack later.",
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
    impacted=False,
    flavor="",
    cws_supported_osdescriptors="",
    src_agent_version="",
    dest_agent_version="",
    keep_stack=False,
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
    osdescriptors="",
    module_name="test/new-e2e",
):
    """
    Run E2E Tests based on test-infra-definitions infrastructure provisioning.
    """
    if "test" not in tags:
        tags = tags + ["test"]

    if shutil.which("pulumi") is None:
        raise Exit(
            "pulumi CLI not found, Pulumi needs to be installed on the system (see https://github.com/DataDog/datadog-agent/blob/main/test/e2e-framework/README.md)",
            1,
        )

    e2e_module = get_default_modules()[module_name]

    e2e_module.should_test_condition = "always"
    if targets:
        e2e_module.test_targets = targets

    if impacted and running_in_ci():
        try:
            print(color_message("Using dynamic tests", "yellow"))
            # DynTestExecutor needs to access build stable account to retrieve the index. Temporarly remove the AWS_PROFILE to avoid connecting on agent-qa account
            with environ({"AWS_PROFILE": "DELETE"}):
                backend = S3Backend(DEFAULT_DYNTEST_BUCKET_URI)
                executor = DynTestExecutor(ctx, backend, IndexKind.DIFFED_PACKAGE, get_commit_sha(ctx, short=True))
                changed_files = get_modified_files(ctx)
                changed_packages = list({os.path.dirname(change) for change in changed_files})
                print(color_message(f"The following changes were detected: {changed_files}", "yellow"))
                test_job_name = os.getenv("CI_JOB_NAME")
                if test_job_name.endswith("-init"):
                    test_job_name = test_job_name.removesuffix("-init")
                to_skip = executor.tests_to_skip(test_job_name, changed_packages + changed_files)
                ctx.run(f"datadog-ci measure --level job --measures 'e2e.skipped_tests:{len(to_skip)}'", warn=True)
                print(color_message(f"The following tests will be skipped: {to_skip}", "yellow"))
                skip.extend(to_skip)
        except Exception as e:
            print(color_message(f"Error using dynamic tests: {e}", "red"))
            print(color_message("Continuing with static tests", "yellow"))

    env_vars = {}
    if profile:
        env_vars["E2E_PROFILE"] = profile

    parsed_params = {}

    # Outside of CI try to automatically configure the secret to pull agent image
    if not running_in_ci():
        # Authentication against agent-qa is required for all kubernetes tests, to use the cache
        ecr_password = _get_agent_qa_ecr_password(ctx)
        if ecr_password:
            parsed_params["ddagent:imagePullPassword"] = ecr_password
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

    cmd += f'{{junit_file_flag}} {{json_flag}} --packages="{{packages}}" {raw_command} -- -ldflags="-X {{REPO_PATH}}/test/new-e2e/tests/containers.GitCommit={{commit}}" {{verbose}} -mod={{go_mod}} -vet=off -timeout {{timeout}} -tags "{{go_build_tags}}" {{nocache}} {{run}} {{skip}} {{test_run_arg}} -args {{osdescriptors}} {{flavor}} {{cws_supported_osdescriptors}} {{src_agent_version}} {{dest_agent_version}} {{extra_flags}}'
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
        "flavor": f"-flavor {flavor}" if flavor else "",
        "osdescriptors": f"-osdescriptors {osdescriptors}" if osdescriptors else "",
        "cws_supported_osdescriptors": f"-cws-supported-osdescriptors {cws_supported_osdescriptors}"
        if cws_supported_osdescriptors
        else "",
        "src_agent_version": f"-src-agent-version {src_agent_version}" if src_agent_version else "",
        "dest_agent_version": f"-dest-agent-version {dest_agent_version}" if dest_agent_version else "",
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

        if keep_stack is True:
            env_vars["E2E_DEV_MODE"] = "true"

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
        if test_res is None:
            ctx.run("datadog-ci tag --level job --tags 'e2e.skipped_all_tests:true'")
            return

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

        param_keys = ("osversion", "osdescriptors", "platform", "arch")
        for param_key in param_keys:
            if args.get(param_key):
                params.append(f"-{args[param_key]}")

        params.extend(f"-c {configparam}" for configparam in configparams)

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


def _get_pulumi_backend_url(ctx: Context) -> str | None:
    """
    Get the Pulumi backend URL using 'pulumi whoami --json'.
    Returns the backend URL or None if it cannot be determined.
    """
    res = ctx.run(
        "pulumi whoami --json",
        hide=True,
        warn=True,
        env=_get_default_env(),
    )
    if res is None or res.exited != 0:
        return None
    try:
        whoami = json.loads(res.stdout)
        return whoami.get("url")
    except json.JSONDecodeError:
        return None


def _list_stacks_from_s3(backend_url: str, project: str = "e2eci") -> list[dict]:
    """
    List Pulumi stacks directly from S3 backend.
    Much faster than 'pulumi stack ls' for buckets with many stacks.

    Args:
        backend_url: S3 backend URL (e.g., 's3://bucket-name' or 's3://bucket-name/path')
        project: Pulumi project name (default: 'e2eci')

    Returns:
        List of stack dictionaries with 'name' key matching pulumi stack ls format
    """
    import boto3
    from botocore.exceptions import ClientError

    # Parse S3 URL: s3://bucket-name/optional/path
    s3_path = backend_url.removeprefix("s3://")
    parts = s3_path.split("/", 1)
    bucket_name = parts[0]
    base_prefix = parts[1] if len(parts) > 1 else ""
    bucket_name = bucket_name.split("?")[0]  # Remove query parameters for AWS url

    # Pulumi stores stacks at: {base_prefix}/.pulumi/stacks/{project}/
    stacks_prefix = f"{base_prefix}/.pulumi/stacks/{project}/".lstrip("/")

    try:
        s3_client = boto3.client('s3')
        stacks = []

        paginator = s3_client.get_paginator('list_objects_v2')
        page_iterator = paginator.paginate(Bucket=bucket_name, Prefix=stacks_prefix)

        for page in page_iterator:
            for obj in page.get('Contents', []):
                key = obj['Key']
                # Stack files are {stack_name}.json (skip .json.bak files)
                if key.endswith('.json'):
                    # Extract stack name from path
                    filename = key.split('/')[-1]
                    stack_name = filename.removesuffix('.json')
                    stacks.append(
                        {
                            'name': f"organization/{project}/{stack_name}",
                            'lastUpdate': obj.get('LastModified', '').isoformat() if obj.get('LastModified') else None,
                        }
                    )

        return stacks

    except ClientError as e:
        print(f"Failed to list stacks from S3: {e}")
        return []


def list_stacks(ctx: Context, project: str = "e2eci") -> list[dict]:
    """
    List Pulumi stacks. Uses S3 SDK for S3 backends (much faster),
    falls back to pulumi CLI for other backends.

    Args:
        ctx: Invoke context
        project: Pulumi project name (default: 'e2eci')

    Returns:
        List of stack dictionaries with 'name' key
    """
    backend_url = _get_pulumi_backend_url(ctx)

    if backend_url and backend_url.startswith("s3://"):
        return _list_stacks_from_s3(backend_url, project)

    # Fallback to pulumi CLI for non-S3 backends (local, etc.)
    res = ctx.run(
        "pulumi stack ls --all --json",
        hide=True,
        warn=True,
    )
    if res is None or res.exited != 0:
        return []
    return json.loads(res.stdout)


@task
def cleanup_remote_stacks(ctx, stack_regex):
    """
    Clean up remote stacks created by the pipeline
    """
    remote_stack_cleaning = os.getenv("REMOTE_STACK_CLEANING") == "true"
    if remote_stack_cleaning:
        print("Using remote stack cleaning")
    else:
        print("Using local stack cleaning")

    stack_regex = re.compile(stack_regex)

    # Use S3 SDK for listing stacks (much faster than pulumi CLI)
    stacks = list_stacks(ctx)
    if not stacks:
        print("No stacks found or failed to list stacks")
        return
    to_delete_stacks = set()
    for stack in stacks:
        if stack_regex.match(stack["name"].split("/")[-1]):
            to_delete_stacks.add(stack["name"])

    if len(to_delete_stacks) == 0:
        print("No stacks to delete")
        return

    print("About to delete the following stacks:", to_delete_stacks)

    with multiprocessing.Pool(len(to_delete_stacks)) as pool:
        destroy_func = destroy_remote_stack_api if remote_stack_cleaning else destroy_remote_stack_local
        res = pool.map(destroy_func, to_delete_stacks)
        destroyed_stack = set()
        failed_stack = set()
        for exit_code, stdout, stderr, stack in res:
            if exit_code != 0:
                failed_stack.add(stack)
            else:
                destroyed_stack.add(stack)
            print(f"Stack {stack}: {stdout} {stderr}")

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
    return {
        "PULUMI_SKIP_UPDATE_CHECK": "true",
    }


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

    destroy_env = _get_default_env()
    destroy_env["PULUMI_K8S_DELETE_UNREACHABLE"] = "true"

    with ctx.cd(tempfile.gettempdir()):
        ret = ctx.run(
            f"pulumi destroy --stack {stack} --yes --remove --skip-preview",
            warn=True,
            hide=True,
            env=destroy_env,
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
                env=destroy_env,
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


def _get_agent_qa_ecr_password(ctx: Context) -> str:
    ecr_password_res = ctx.run(
        "aws-vault exec sso-agent-qa-read-only -- aws ecr get-login-password", hide=True, warn=True
    )
    if ecr_password_res.exited != 0:
        ecr_password_res = ctx.run(
            "aws-vault exec sso-agent-qa-account-admin -- aws ecr get-login-password", hide=True, warn=True
        )
    if ecr_password_res.exited != 0:
        print(
            "WARNING: Could not get ECR password for agent-qa account, if your test need to pull image from agent-qa ECR it is likely to fail"
        )
        return ""
    return ecr_password_res.stdout.strip()


def _find_recent_successful_pipeline(ctx: Context, branch: str | None = None) -> str | None:
    """
    Find the most recent successful pipeline on the given branch or current branch if not specified.
    Returns pipeline_id or None if not found.
    """
    try:
        # Explicitly use GITLAB_TOKEN if set
        token = os.environ.get('GITLAB_TOKEN')
        repo = get_gitlab_repo(token=token)

        # Try the specified branch or current branch
        branch_to_try = ""
        if branch:
            branch_to_try = branch
        else:
            try:
                current = get_current_branch(ctx)
                if current:
                    branch_to_try = current
            except Exception as e:
                raise Exit(f"Could not get current branch: {e}", code=1) from e

        # Get pipelines on this branch, ordered by most recent
        pipelines = repo.pipelines.list(ref=branch_to_try, per_page=10, order_by='updated_at', get_all=False)
        for pipeline in pipelines:
            if pipeline.status == "success":
                return str(pipeline.id)

        return None
    except Exception as e:
        print(f"Warning: Could not query GitLab for recent pipelines: {e}")
        if 'GITLAB_TOKEN' not in os.environ:
            print(
                "No GITLAB_TOKEN environment variable found, set it with a GitLab Personal Access Token (read_api scope)"
            )
        return None


def _find_local_msi_build(pkg: str | None = None) -> str | None:
    """
    Find a local MSI build in the omnibus/pkg directory.

    Args:
        pkg: Optional package name or pattern to search for.
             Can be a full filename (e.g., "datadog-agent-7.75.0-devel.git.59.ac0523a-1-x86_64.msi"),
             a partial name (e.g., "datadog-agent-7.75"), or None to find the most recent MSI.

    Returns the absolute path to the MSI file, or None if not found.
    """
    import glob

    # Standard output directory for local MSI builds
    output_dir = Path.cwd() / "omnibus" / "pkg"

    if not output_dir.is_dir():
        return None

    if pkg:
        # If pkg is provided, search for it
        # Check if it's an absolute path first
        if os.path.isabs(pkg) and os.path.isfile(pkg):
            return pkg

        # Check if it's a file in the output directory
        direct_path = os.path.join(output_dir, pkg)
        if os.path.isfile(direct_path):
            return direct_path

        # Try as a glob pattern
        if '*' not in pkg:
            pkg = f"*{pkg}*"
        pattern = os.path.join(output_dir, pkg)
        if not pattern.endswith('.msi'):
            pattern = f"{pattern}*.msi"
        msi_files = glob.glob(pattern)
    else:
        # Look for agent MSI files (both regular and FIPS)
        patterns = [
            os.path.join(output_dir, "datadog-agent-*.msi"),
            os.path.join(output_dir, "datadog-fips-agent-*.msi"),
        ]
        msi_files = []
        for pattern in patterns:
            msi_files.extend(glob.glob(pattern))

    if not msi_files:
        return None

    # Return the most recently modified MSI
    return max(msi_files, key=os.path.getmtime)


def _parse_version_from_msi_filename(ctx, msi_path: str) -> tuple[str, str] | None:
    """
    Parse version information from MSI filename.

    MSI filename format: datadog-agent-{version}-{arch}.msi
    Example: datadog-agent-7.75.0-devel.git.59.ac0523a-1-x86_64.msi

    Returns (display_version, package_version) tuple, or None if parsing fails.
    - display_version: e.g., "7.75.0-devel"
    - package_version: e.g., "7.75.0-devel.git.59.ac0523a-1"
    """
    import re

    filename = os.path.basename(msi_path)

    # Try to use cached agent.version first
    try:
        expected_version = f"{get_version(ctx, include_git=True, url_safe=True)}-1"
        if expected_version in filename:
            # Version matches what we expect from git state
            package_version = expected_version

            # Extract display version
            if '.git.' in package_version:
                display_version = package_version.split('.git.')[0]
            elif package_version.endswith('-1'):
                display_version = package_version[:-2]
            else:
                display_version = package_version

            return display_version, package_version
    except Exception:
        print("Warning: Could not determine version from cached agent.version. Falling back to regex parsing")
        pass

    # Pattern to match: datadog-agent-{version}-{arch}.msi or datadog-fips-agent-{version}-{arch}.msi
    # Version format: 7.75.0-devel.git.59.ac0523a-1
    # Arch is typically: x86_64
    pattern = r'^datadog(?:-fips)?-agent-(.+)-(x86_64|amd64)\.msi$'
    match = re.match(pattern, filename)

    if not match:
        return None

    package_version = match.group(1)  # e.g., "7.75.0-devel.git.59.ac0523a-1"

    # Extract display version (everything before .git. or the full version if no .git.)
    # e.g., "7.75.0-devel.git.59.ac0523a-1" -> "7.75.0-devel"
    # e.g., "7.75.0-1" -> "7.75.0"
    if '.git.' in package_version:
        display_version = package_version.split('.git.')[0]
    elif package_version.endswith('-1'):
        # Remove trailing -1 for display version (e.g., "7.75.0-1" -> "7.75.0")
        display_version = package_version[:-2]
    else:
        display_version = package_version

    return display_version, package_version


def _path_to_file_url(file_path: str) -> str:
    """Convert a file path to a file:// URL."""
    # Normalize the path and convert to forward slashes
    abs_path = os.path.abspath(file_path)
    if os.name == 'nt':
        return f"file://{abs_path.replace(os.sep, '/')}"
    return f"file://{abs_path}"


@task(
    help={
        "fmt": "Output format: 'bash' for export commands, 'powershell' for $env: commands, 'json' for JSON output",
        "build": "Build source: 'local' for local build in omnibus/pkg, 'pipeline' for CI pipeline artifacts (default: pipeline)",
        "pkg": "Local MSI to use instead of using the most recent one. Only used with --build local",
        "branch": "Git branch to find pipeline from (default: current branch, falls back to main). Only used with --build pipeline",
        "pipeline_id": "Override pipeline ID instead of auto-detecting. Only used with --build pipeline",
    }
)
def setup_env(ctx, fmt="bash", build="pipeline", pkg=None, branch=None, pipeline_id=None):
    """
    Generate environment variables for running E2E Fleet Automation tests locally.

    This task derives version information and artifact locations to set the required
    environment variables for running E2E tests.

    Usage:
        # Use local MSI build from omnibus/pkg directory (Windows)
        dda inv new-e2e-tests.setup-env --build local

        # Use artifacts from a CI pipeline (auto-detects most recent successful pipeline)
        dda inv new-e2e-tests.setup-env --build pipeline

        # Use a specific pipeline
        dda inv new-e2e-tests.setup-env --build pipeline --pipeline-id 12345678

        # Bash/WSL - eval the output to apply the environment variables
        eval "$(dda inv new-e2e-tests.setup-env --build local)"

        # PowerShell - pipe to Invoke-Expression to execute the commands
        dda inv new-e2e-tests.setup-env --build local --fmt powershell | Invoke-Expression

        # JSON - for programmatic use
        dda inv new-e2e-tests.setup-env --build local --fmt json

    Note: The task outputs shell commands (e.g., 'export VAR=value' for bash).
    Using eval (bash) or Invoke-Expression (PowerShell) executes these commands
    to actually set the environment variables in your current shell session.
    Without eval/Invoke-Expression, the commands are just printed but not executed.
    """
    env_vars = {}

    valid_formats = ["bash", "powershell", "json"]
    if fmt not in valid_formats:
        raise Exit(f"Invalid --fmt option: {fmt}. Use one of: {', '.join(valid_formats)}", code=1)

    if build == "local":
        # Find local MSI build (Windows)
        msi_path = _find_local_msi_build(pkg)
        if msi_path:
            env_vars["CURRENT_AGENT_MSI_URL"] = _path_to_file_url(msi_path)
            print(f"# Found local MSI: {msi_path}", file=sys.stderr)

            # Extract version from MSI filename
            version_info = _parse_version_from_msi_filename(ctx, msi_path)
            if version_info:
                display_version, package_version = version_info
                env_vars["CURRENT_AGENT_VERSION"] = display_version
                env_vars["CURRENT_AGENT_VERSION_PACKAGE"] = package_version
            else:
                print("Warning: Could not parse version from MSI filename, falling back to git", file=sys.stderr)
                try:
                    env_vars["CURRENT_AGENT_VERSION"] = get_version(ctx, include_git=False, include_pre=True)
                    package_version = get_version(ctx, include_git=True, url_safe=True)
                    env_vars["CURRENT_AGENT_VERSION_PACKAGE"] = f"{package_version}-1"
                except Exception as e:
                    raise Exit(f"Could not determine current agent version: {e}", code=1) from e

            # Check for matching OCI package
            if "CURRENT_AGENT_VERSION_PACKAGE" in env_vars:
                oci_filename = f"datadog-agent-{env_vars['CURRENT_AGENT_VERSION_PACKAGE']}-windows-amd64.oci.tar"
                oci_path = os.path.join(os.path.dirname(msi_path), oci_filename)
                if os.path.isfile(oci_path):
                    env_vars["CURRENT_AGENT_OCI_URL"] = _path_to_file_url(oci_path)
                    print(f"# Found local OCI: {oci_path}", file=sys.stderr)
                else:
                    print(f"# Note: No OCI package found at {oci_filename}", file=sys.stderr)
        else:
            if pkg:
                raise Exit(f"No MSI matching '{pkg}' found in omnibus/pkg/.", code=1)
            else:
                raise Exit("No local MSI build found in omnibus/pkg/. Run 'dda inv msi.build' first.", code=1)

    elif build == "pipeline":
        # Find pipeline ID
        if pipeline_id:
            env_vars["E2E_PIPELINE_ID"] = pipeline_id
            print(f"# Using pipeline: {pipeline_id}", file=sys.stderr)
        else:
            result = _find_recent_successful_pipeline(ctx, branch)
            if result:
                env_vars["E2E_PIPELINE_ID"] = result
                print(f"# Found pipeline: {env_vars['E2E_PIPELINE_ID']}", file=sys.stderr)
            else:
                raise Exit("Could not find a recent successful pipeline.", code=1)

        try:
            current_version = get_version(ctx, include_git=False, include_pre=True, pipeline_id=pipeline_id)
            env_vars["CURRENT_AGENT_VERSION"] = current_version
        except Exception as e:
            raise Exit(f"Could not determine current agent version: {e}", code=1) from e

    else:
        raise Exit(f"Invalid --build option: {build}. Use 'local' or 'pipeline'.", code=1)

    # Get stable version from release.json
    try:
        release_json = load_release_json()
        stable_version = release_json["last_stable"]["7"]
        env_vars["STABLE_AGENT_VERSION"] = stable_version
        env_vars["STABLE_AGENT_VERSION_PACKAGE"] = f"{stable_version}-1"
        env_vars["STABLE_AGENT_MSI_URL"] = (
            f"https://s3.amazonaws.com/ddagent-windows-stable/ddagent-cli-{stable_version}.msi"
        )
    except Exception as e:
        print(f"# Warning: Could not read stable version from release.json: {e}", file=sys.stderr)
        print("# Using fallback stable version", file=sys.stderr)
        env_vars["STABLE_AGENT_VERSION"] = "7.75.0"
        env_vars["STABLE_AGENT_VERSION_PACKAGE"] = "7.75.0-1"
        env_vars["STABLE_AGENT_MSI_URL"] = "https://s3.amazonaws.com/ddagent-windows-stable/ddagent-cli-7.75.0.msi"

    # Output in requested format
    if fmt == "json":
        print(json.dumps(env_vars, indent=2))
    elif fmt == "powershell":
        for key, value in env_vars.items():
            print(f'$env:{key}="{value}"')
    else:  # bash
        for key, value in env_vars.items():
            print(f'export {key}="{value}"')
