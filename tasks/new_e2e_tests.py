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
from collections import defaultdict
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
from tasks.libs.common.utils import REPO_PATH, color_message, gitlab_section, running_in_ci
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


@task(
    iterable=['tags', 'targets', 'configparams'],
    help={
        'profile': 'Override auto-detected runner profile (local or CI)',
        'tags': 'Build tags to use',
        'targets': 'Target packages (same as inv test)',
        'configparams': 'Set overrides for ConfigMap parameters (same as -c option in test-infra-definitions)',
        'verbose': 'Verbose output: log all tests as they are run (same as gotest -v) [default: True]',
        'run': 'Only run tests matching the regular expression',
        'skip': 'Only run tests not matching the regular expression',
        'agent_image': 'Full image path for the agent image (e.g. "repository:tag") to run the e2e tests with',
        'cluster_agent_image': 'Full image path for the cluster agent image (e.g. "repository:tag") to run the e2e tests with',
    },
)
def run(
    ctx,
    profile="",
    tags=[],  # noqa: B006
    targets=[],  # noqa: B006
    configparams=[],  # noqa: B006
    verbose=True,
    run="",
    skip="",
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
    e2e_module.should_test_condition = 'always'
    if targets:
        e2e_module.test_targets = targets

    env_vars = {}
    if profile:
        env_vars["E2E_PROFILE"] = profile

    parsed_params = {}
    for param in configparams:
        parts = param.split("=", 1)
        if len(parts) != 2:
            raise Exit(message=f"wrong format given for config parameter, expects key=value, actual: {param}", code=1)
        parsed_params[parts[0]] = parts[1]

    if agent_image:
        parsed_params["ddagent:fullImagePath"] = agent_image

    if cluster_agent_image:
        parsed_params["ddagent:clusterAgentFullImagePath"] = cluster_agent_image

    if parsed_params:
        env_vars["E2E_STACK_PARAMS"] = json.dumps(parsed_params)

    gotestsum_format = "standard-verbose" if verbose else "pkgname"

    test_run_arg = ""
    if test_run_name != "":
        test_run_arg = f"-run {test_run_name}"

    # Create temporary file for flaky patterns config
    tmp_flaky_patterns_config = tempfile.NamedTemporaryFile(suffix="flaky_patterns_config.yaml", delete=False)
    tmp_flaky_patterns_config.write(b"{}")
    tmp_flaky_patterns_config.close()
    flaky_patterns_config = tmp_flaky_patterns_config.name
    env_vars["E2E_FLAKY_PATTERNS_CONFIG"] = flaky_patterns_config

    cmd = f'gotestsum --format {gotestsum_format} '
    scrubber_raw_command = ""
    # Scrub the test output to avoid leaking API or APP keys when running in the CI
    if running_in_ci():
        scrubber_raw_command = (
            # Using custom go command piped with scrubber sed instructions https://github.com/gotestyourself/gotestsum#custom-go-test-command
            f"--raw-command {os.path.join(os.path.dirname(__file__), 'tools', 'gotest-scrubbed.sh')} {{packages}}"
        )
    cmd += f'{{junit_file_flag}} {{json_flag}} --packages="{{packages}}" {scrubber_raw_command} -- -ldflags="-X {{REPO_PATH}}/test/new-e2e/tests/containers.GitCommit={{commit}}" {{verbose}} -mod={{go_mod}} -vet=off -timeout {{timeout}} -tags "{{go_build_tags}}" {{nocache}} {{run}} {{skip}} {{test_run_arg}} -args {{osversion}} {{platform}} {{major_version}} {{arch}} {{flavor}} {{cws_supported_osversion}} {{src_agent_version}} {{dest_agent_version}} {{keep_stacks}} {{extra_flags}}'

    args = {
        "go_mod": "readonly",
        "timeout": "4h",
        "verbose": '-v' if verbose else '',
        "nocache": '-count=1' if not cache else '',
        "REPO_PATH": REPO_PATH,
        "commit": get_commit_sha(ctx, short=True),
        "run": '-test.run ' + run if run else '',
        "skip": '-test.skip ' + skip if skip else '',
        "test_run_arg": test_run_arg,
        "osversion": f"-osversion {osversion}" if osversion else '',
        "platform": f"-platform {platform}" if platform else '',
        "arch": f"-arch {arch}" if arch else '',
        "flavor": f"-flavor {flavor}" if flavor else '',
        "major_version": f"-major-version {major_version}" if major_version else '',
        "cws_supported_osversion": f"-cws-supported-osversion {cws_supported_osversion}"
        if cws_supported_osversion
        else '',
        "src_agent_version": f"-src-agent-version {src_agent_version}" if src_agent_version else '',
        "dest_agent_version": f"-dest-agent-version {dest_agent_version}" if dest_agent_version else '',
        "keep_stacks": '-keep-stacks' if keep_stacks else '',
        "flaky_patterns_config": f'--flaky-patterns-config={flaky_patterns_config}' if flaky_patterns_config else '',
        "extra_flags": extra_flags,
    }

    test_res = test_flavor(
        ctx,
        flavor=AgentFlavor.base,
        build_tags=tags,
        modules=[e2e_module],
        args=args,
        cmd=cmd,
        env=env_vars,
        junit_tar=junit_tar,
        save_result_json="",
        test_profiler=None,
    )

    success = process_test_result(
        test_res, junit_tar, AgentFlavor.base, test_washer, extra_flakes_config=flaky_patterns_config
    )

    if running_in_ci():
        # Do not print all the params, they could contain secrets needed only in the CI
        params = [f'--targets {t}' for t in targets]

        param_keys = ('osversion', 'platform', 'arch')
        for param_key in param_keys:
            if args.get(param_key):
                params.append(f'-{args[param_key]}')

        command = f"E2E_PIPELINE_ID={os.environ.get('CI_PIPELINE_ID')} inv -e new-e2e-tests.run {' '.join(params)}"
        print(
            f'To run this test locally, use: `{command}`. '
            'You can also add `E2E_DEV_MODE="true"` to run in dev mode which will leave the environment up after the tests.'
            '\nYou can troubleshoot e2e test failures with this documentation: https://datadoghq.atlassian.net/wiki/x/7gIo0'
        )

    if logs_post_processing:
        if len(test_res) == 1:
            post_processed_output = post_process_output(
                test_res[0].result_json_path, test_depth=logs_post_processing_test_depth
            )
            os.makedirs(logs_folder, exist_ok=True)
            write_result_to_log_files(post_processed_output, logs_folder)

            pretty_print_logs(
                test_res[0].result_json_path, post_processed_output, flakes_files=["flakes.yaml", flaky_patterns_config]
            )
        else:
            print(
                color_message("WARNING", "yellow")
                + f": Logs post processing expect only test result for test/new-e2e module. Skipping because result contains test for {len(test_res)} modules."
            )

    if not success:
        raise Exit(code=1)


@task(
    help={
        'locks': 'Cleans up lock files, default True',
        'stacks': 'Cleans up local stack state, default False',
        'output': 'Cleans up local test output directory, default False',
        'skip_destroy': 'Skip stack\'s resources removal. Use it only if your resources are already removed by other means, default False',
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


def post_process_output(path: str, test_depth: int = 1):
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
    """

    def is_parent(parent: list[str], child: list[str]) -> bool:
        for i in range(len(parent)):
            if parent[i] != child[i]:
                return False
        return True

    logs_per_test = {}
    with open(path) as f:
        all_lines = f.readlines()

        # Initalize logs_per_test with all test names
        for line in all_lines:
            json_line = json.loads(line)
            if "Package" not in json_line or "Test" not in json_line or "Output" not in json_line:
                continue
            splitted_test = json_line["Test"].split("/")
            if len(splitted_test) < test_depth:
                continue
            if json_line["Package"] not in logs_per_test:
                logs_per_test[json_line["Package"]] = {}

            test_name = splitted_test[: min(test_depth, len(splitted_test))]
            logs_per_test[json_line["Package"]]["/".join(test_name)] = []

        for line in all_lines:
            json_line = json.loads(line)
            if "Package" not in json_line or "Test" not in json_line or "Output" not in json_line:
                continue

            if "===" in json_line["Output"]:  # Ignore these lines that are produced when running test concurrently
                continue

            splitted_test = json_line["Test"].split("/")

            if len(splitted_test) < test_depth:  # Append logs to all children tests
                for test_name in logs_per_test[json_line["Package"]]:
                    if is_parent(splitted_test, test_name.split("/")):
                        logs_per_test[json_line["Package"]][test_name].append(json_line["Output"])
                continue

            logs_per_test[json_line["Package"]]["/".join(splitted_test[:test_depth])].append(json_line["Output"])
    return logs_per_test


def write_result_to_log_files(logs_per_test, log_folder):
    for package, tests in logs_per_test.items():
        for test, logs in tests.items():
            sanitized_package_name = re.sub(r"[^\w_. -]", "_", package)
            sanitized_test_name = re.sub(r"[^\w_. -]", "_", test)
            with open(f"{log_folder}/{sanitized_package_name}.{sanitized_test_name}.log", "w") as f:
                f.write("".join(logs))


class TooManyLogsError(Exception):
    pass


def pretty_print_test_logs(logs_per_test: list[tuple[str, str, str]], max_size):
    # Compute size in bytes of what we are about to print. If it exceeds max_size, we skip printing because it will make the Gitlab logs almost completely collapsed.
    # By default Gitlab has a limit of 500KB per job log, so we want to avoid printing too much.
    size = 0
    for _, _, logs in logs_per_test:
        size += len("".join(logs).encode())
    if size > max_size and running_in_ci():
        raise TooManyLogsError
    for package, test, logs in logs_per_test:
        with gitlab_section("Complete logs for " + package + "." + test, collapsed=True):
            print("".join(logs))

    return size


def pretty_print_logs(result_json_path, logs_per_test, max_size=250000, flakes_files=None):
    """Pretty prints logs with a specific order.

    Print order:
        1. Failing and non flaky tests
        2. Failing and flaky tests
        3. Successful and non flaky tests
        4. Successful and flaky tests
    """

    result_json_name = result_json_path.split("/")[-1]
    result_json_dir = result_json_path.removesuffix('/' + result_json_name)
    washer = TestWasher(test_output_json_file=result_json_name, flakes_file_paths=flakes_files or ["flakes.yaml"])
    failing_tests, marked_flaky_tests = washer.parse_test_results(result_json_dir)
    all_known_flakes = washer.merge_known_flakes(marked_flaky_tests)

    try:
        # (failing, flaky) -> [(package, test_name, logs)]
        categorized_logs = defaultdict(list)

        # Split flaky / non flaky tests
        for package, tests in logs_per_test.items():
            package_flaky = all_known_flakes.get(package, set())
            package_failing = failing_tests.get(package, set())
            for test_name, logs in tests.items():
                state = test_name in package_failing, test_name in package_flaky
                categorized_logs[state].append((package, test_name, logs))

        for failing, flaky in [TestState.FAILED, TestState.FLAKY_FAILED, TestState.SUCCESS, TestState.FLAKY_SUCCESS]:
            logs_to_print = categorized_logs[failing, flaky]
            if not logs_to_print:
                continue

            print(f'* {color_message(TestState.get_human_readable_state(failing, flaky), Color.BOLD)} job logs:')
            # Print till the size limit is reached
            max_size -= pretty_print_test_logs(logs_to_print, max_size)
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
        raise Exit(message=f"e2e-output directory {output_dir} is not a directory, aborting", code=1)

    # sanity check to avoid deleting the wrong directory, e2e-output should only contain directories
    for entry in output_dir.iterdir():
        if not entry.is_dir():
            raise Exit(
                message=f"e2e-output directory {output_dir} contains more than just directories, aborting", code=1
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
    output = ctx.run("pulumi stack ls --all --project e2elocal --json", hide=True, env=_get_default_env())
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
            raise Exit(color_message(f"Failed to destroy stack {stack}: {ret.stdout, ret.stderr}", "red"), 1)


def _remove_stack(ctx: Context, stack: str):
    ctx.run(f"pulumi stack rm --force --yes --stack {stack}", hide=True, env=_get_default_env())


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
