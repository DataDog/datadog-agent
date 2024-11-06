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
from pathlib import Path

import yaml
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.flavor import AgentFlavor
from tasks.gotest import process_test_result, test_flavor
from tasks.libs.common.git import get_commit_sha
from tasks.libs.common.go import download_go_dependencies
from tasks.libs.common.utils import REPO_PATH, color_message, running_in_ci
from tasks.modules import DEFAULT_MODULES
from tasks.tools.e2e_stacks import destroy_remote_stack


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
):
    """
    Run E2E Tests based on test-infra-definitions infrastructure provisioning.
    """
    if shutil.which("pulumi") is None:
        raise Exit(
            "pulumi CLI not found, Pulumi needs to be installed on the system (see https://github.com/DataDog/test-infra-definitions/blob/main/README.md)",
            1,
        )

    e2e_module = DEFAULT_MODULES["test/new-e2e"]
    e2e_module.condition = lambda: True
    if targets:
        e2e_module.targets = targets

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

    cmd = f'gotestsum --format {gotestsum_format} '
    cmd += '{junit_file_flag} {json_flag} --packages="{packages}" -- -ldflags="-X {REPO_PATH}/test/new-e2e/tests/containers.GitCommit={commit}" {verbose} -mod={go_mod} -vet=off -timeout {timeout} -tags "{go_build_tags}" {nocache} {run} {skip} {test_run_arg} -args {osversion} {platform} {major_version} {arch} {flavor} {cws_supported_osversion} {src_agent_version} {dest_agent_version} {keep_stacks} {extra_flags}'

    args = {
        "go_mod": "mod",
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

    success = process_test_result(test_res, junit_tar, AgentFlavor.base, test_washer)

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
