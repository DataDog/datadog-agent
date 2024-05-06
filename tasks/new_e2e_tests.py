"""
Running E2E Tests with infra based on Pulumi
"""

import json
import os
import os.path
import shutil
import tempfile
from pathlib import Path
from typing import List

import yaml
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.flavor import AgentFlavor
from tasks.go_test import test_flavor
from tasks.libs.common.junit_upload_core import produce_junit_tar
from tasks.libs.common.utils import REPO_PATH, get_git_commit
from tasks.modules import DEFAULT_MODULES


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

    envVars = dict()
    if profile:
        envVars["E2E_PROFILE"] = profile

    parsedParams = dict()
    for param in configparams:
        parts = param.split("=", 1)
        if len(parts) != 2:
            raise Exit(message=f"wrong format given for config parameter, expects key=value, actual: {param}", code=1)
        parsedParams[parts[0]] = parts[1]

    if parsedParams:
        envVars["E2E_STACK_PARAMS"] = json.dumps(parsedParams)

    gotestsum_format = "standard-verbose" if verbose else "pkgname"

    test_run_arg = ""
    if test_run_name != "":
        test_run_arg = f"-run {test_run_name}"

    cmd = f'gotestsum --format {gotestsum_format} '
    cmd += '{junit_file_flag} --packages="{packages}" -- -ldflags="-X {REPO_PATH}/test/new-e2e/tests/containers.GitCommit={commit}" {verbose} -mod={go_mod} -vet=off -timeout {timeout} -tags "{go_build_tags}" {nocache} {run} {skip} {test_run_arg} -args {osversion} {platform} {major_version} {arch} {flavor} {cws_supported_osversion} {src_agent_version} {dest_agent_version} {keep_stacks} {extra_flags}'

    args = {
        "go_mod": "mod",
        "timeout": "4h",
        "verbose": '-v' if verbose else '',
        "nocache": '-count=1' if not cache else '',
        "REPO_PATH": REPO_PATH,
        "commit": get_git_commit(),
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
        env=envVars,
        junit_tar=junit_tar,
        save_result_json="",
        test_profiler=None,
    )
    if junit_tar:
        junit_files = []
        for module_test_res in test_res:
            if module_test_res.junit_file_path:
                junit_files.append(module_test_res.junit_file_path)
        produce_junit_tar(junit_files, junit_tar)

    some_test_failed = False
    for module_test_res in test_res:
        failed, failure_string = module_test_res.get_failure(AgentFlavor.base)
        some_test_failed = some_test_failed or failed
        if failed:
            print(failure_string)

    if some_test_failed:
        # Exit if any of the modules failed
        raise Exit(code=1)


@task(
    help={
        'locks': 'Cleans up lock files, default True',
        'stacks': 'Cleans up local stack state, default False',
        'output': 'Cleans up local test output directory, default False',
    },
)
def clean(ctx, locks=True, stacks=False, output=False):
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
        _clean_stacks(ctx)

    if output:
        _clean_output()


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


def _clean_stacks(ctx: Context):
    print("🧹 Clean up stack")
    stacks = _get_existing_stacks(ctx)

    for stack in stacks:
        print(f"🗑️  Destroying stack {stack}")
        _destroy_stack(ctx, stack)

    stacks = _get_existing_stacks(ctx)
    for stack in stacks:
        print(f"🗑️ Cleaning up stack {stack}")
        _remove_stack(ctx, stack)


def _get_existing_stacks(ctx: Context) -> List[str]:
    e2e_stacks: List[str] = []
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
            # run with refresh on first destroy attempt failure
            ctx.run(
                f"pulumi destroy --stack {stack} -r --yes --remove --skip-preview",
                warn=True,
                hide=True,
                env=_get_default_env(),
            )


def _remove_stack(ctx: Context, stack: str):
    ctx.run(f"pulumi stack rm --force --yes --stack {stack}", hide=True, env=_get_default_env())


def _get_pulumi_about(ctx: Context) -> dict:
    output = ctx.run("pulumi about --json", hide=True, env=_get_default_env())
    if output is None or not output:
        return ""
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
