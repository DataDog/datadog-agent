"""
Running E2E Tests with infra based on Pulumi
"""

import getpass
import json
import shutil
import os
import os.path
from pathlib import Path
import subprocess
import tempfile
from typing import List

from invoke.tasks import task
from invoke.context import Context
from invoke.exceptions import Exit

from .flavor import AgentFlavor
from .libs.junit_upload import produce_junit_tar
from .modules import DEFAULT_MODULES
from .test import test_flavor


@task(
    iterable=['tags', 'targets', 'configparams'],
    help={
        'profile': 'Override auto-detected runner profile (local or CI)',
        'tags': 'Build tags to use',
        'targets': 'Target packages (same as inv test)',
        'configparams': 'Set overrides for ConfigMap parameters (same as -c option in test-infra-definitions)',
        'verbose': 'Verbose output: log all tests as they are run (same as gotest -v) [default: True]',
    },
)
def run(ctx, profile="", tags=[], targets=[], configparams=[], verbose=True, cache=False, junit_tar=""):  # noqa: B006
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

    cmd = f'gotestsum --format {gotestsum_format} '
    cmd += (
        '--packages="{packages}" -- {verbose} -mod={go_mod} -vet=off -timeout {timeout} -tags {go_build_tags} {nocache}'
    )
    args = {
        "go_mod": "mod",
        "timeout": "4h",
        "verbose": '-v' if verbose else '',
        "nocache": '-count=1' if not cache else '',
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


@task
def clean(_):
    """
    Clean any environment created with invoke tasks or e2e tests
    """
    if not _is_local_state(_get_pulumi_about()):
        print("Cleanup supported for local state only, run `pulumi login --local` to switch to local state")
        return

    print("🧹 Clean up lock files")
    lock_dir = os.path.join(Path.home(), ".pulumi", "locks")

    for entry in os.listdir(Path(lock_dir)):
        subdir = os.path.join(lock_dir, entry)
        for filename in os.listdir(Path(subdir)):
            file_path = os.path.join(subdir, filename)
            if os.path.isfile(file_path) and filename.endswith(".json"):
                os.remove(file_path)
                print(f"🗑️ Deleted lock: {file_path}")

    print("🧹 Clean up stacks")
    stacks = _get_existing_stacks()

    for stack in stacks:
        print(f"🗑️ Cleaning up stack {stack}")
        _destroy_stack(stack)


def _get_existing_stacks() -> List[str]:
    # ensure we deal with local stacks
    output = subprocess.check_output(["pulumi", "stack", "ls", "--all"], cwd=tempfile.gettempdir())
    output = output.decode("utf-8")
    lines = output.splitlines()
    lines = lines[1:]  # skip headers
    e2e_stacks: List[str] = []
    stack_name_prefix = _get_stack_name_prefix()
    for line in lines:
        stack_name = line.split(" ")[0]
        # skip stacks created out of e2e tests
        if stack_name.startswith(stack_name_prefix):
            e2e_stacks.append(stack_name)
    return e2e_stacks


def _get_stack_name_prefix() -> str:
    user_name = f"{getpass.getuser()}-"
    return user_name.replace(".", "-")  # EKS doesn't support '.'


def _destroy_stack(stack_name: str):
    subprocess.call(
        [
            "aws-vault",
            "exec",
            "sso-agent-sandbox-account-admin",
            "--",
            "pulumi",
            "destroy",
            "--remove",
            "-s",
            stack_name,
        ]
    )


def _get_pulumi_about() -> str:
    return subprocess.getoutput("pulumi about")


def _is_local_state(pulumi_about: str) -> bool:
    # check output contains
    # Backend
    # Name           xxxxxxxxxx
    # URL            file://xxx
    # User           xxxxx.xxxxx
    # Organizations
    about_groups = pulumi_about.split("\n\n")

    for about_group in about_groups:
        lines = about_group.splitlines()
        if not lines[0].startswith("Backend"):
            continue
        url_lines = [x for x in lines[1:] if x.startswith("URL")]
        if len(url_lines) > 0 and "file://" in url_lines[0]:
            return True
        return False
