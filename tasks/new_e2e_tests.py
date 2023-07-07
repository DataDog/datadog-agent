"""
Running E2E Tests with infra based on Pulumi
"""

import json
import os
import os.path
import shutil
import subprocess
import tempfile
from pathlib import Path
from typing import List

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from .flavor import AgentFlavor
from .libs.junit_upload import produce_junit_tar
from .modules import DEFAULT_MODULES
from .test import test_flavor
from .utils import REPO_PATH, get_git_commit


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
    cache=False,
    junit_tar="",
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

    cmd = f'gotestsum --format {gotestsum_format} '
    cmd += '{junit_file_flag} --packages="{packages}" -- -ldflags="-X {REPO_PATH}/test/new-e2e/containers.GitCommit={commit}" {verbose} -mod={go_mod} -vet=off -timeout {timeout} -tags {go_build_tags} {nocache} {run} {skip}'
    args = {
        "go_mod": "mod",
        "timeout": "4h",
        "verbose": '-v' if verbose else '',
        "nocache": '-count=1' if not cache else '',
        "REPO_PATH": REPO_PATH,
        "commit": get_git_commit(),
        "run": '-test.run ' + run if run else '',
        "skip": '-test.skip ' + skip if skip else '',
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
    },
)
def clean(ctx, locks=True, stacks=False):
    """
    Clean any environment created with invoke tasks or e2e tests
    By default removes only lock files.
    """
    if not _is_local_state(_get_pulumi_about()):
        print("Cleanup supported for local state only, run `pulumi login --local` to switch to local state")
        return

    if locks:
        _clean_locks()

    if stacks:
        _clean_stacks(ctx)


def _clean_locks():
    print("🧹 Clean up lock files")
    lock_dir = os.path.join(Path.home(), ".pulumi", "locks")

    for entry in os.listdir(Path(lock_dir)):
        subdir = os.path.join(lock_dir, entry)
        for filename in os.listdir(Path(subdir)):
            path = os.path.join(subdir, filename)
            if os.path.isfile(path) and filename.endswith(".json"):
                os.remove(path)
                print(f"🗑️ Deleted lock: {path}")
            elif os.path.isdir(path):
                shutil.rmtree(path)


def _clean_stacks(ctx: Context):
    print("🧹 Clean up stacks")
    stacks = _get_existing_stacks(ctx)

    for stack in stacks:
        print(f"🗑️ Cleaning up stack {stack}")
        _remove_stack(ctx, stack)


def _get_existing_stacks(ctx: Context) -> List[str]:
    # ensure we deal with local stacks
    with ctx.cd(tempfile.gettempdir()):
        output = ctx.run("pulumi stack ls --all", pty=True)
        if output is None or not output:
            return []
        lines = output.stdout.splitlines()
        lines = lines[1:]  # skip headers
        e2e_stacks: List[str] = []
        for line in lines:
            stack_name = line.split(" ")[0]
            e2e_stacks.append(stack_name)
        return e2e_stacks


def _remove_stack(ctx: Context, stack_name: str):
    ctx.run(f"pulumi stack rm --force --yes --stack {stack_name}", pty=True)


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
