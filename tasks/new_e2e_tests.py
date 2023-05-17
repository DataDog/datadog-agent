"""
Running E2E Tests with infra based on Pulumi
"""

import json
import shutil

from invoke import task
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

    cmd = 'gotestsum --format pkgname --packages="{packages}" -- {verbose} -mod={go_mod} -vet=off -timeout {timeout} -tags {go_build_tags} {nocache}'
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
