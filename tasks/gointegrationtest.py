import os
import sys
import traceback
from dataclasses import dataclass

from invoke import Context, task
from invoke.exceptions import Exit

from tasks.build_tags import get_default_build_tags
from tasks.libs.common.utils import TestsNotSupportedError, gitlab_section


@dataclass
class IntegrationTest:
    """
    Integration tests
    """

    prefix: str
    dir: str = None
    extra_args: str = None


@dataclass
class IntegrationTestsConfig:
    """
    Integration tests configuration
    """

    name: str
    go_build_tags: list[str]
    tests: list[IntegrationTest]
    env: dict[str, str] = None


CORE_AGENT_LINUX = IntegrationTestsConfig(
    name="Core Agent Linux",
    go_build_tags=get_default_build_tags(build="test"),
    tests=[
        IntegrationTest(prefix="./test/integration/config_providers/..."),
        IntegrationTest(prefix="./test/integration/corechecks/..."),
        IntegrationTest(prefix="./test/integration/listeners/..."),
        IntegrationTest(prefix="./test/integration/util/kubelet/..."),
    ],
)

CORE_AGENT_WINDOWS = IntegrationTestsConfig(
    name="Core Agent Windows",
    go_build_tags=get_default_build_tags(build="test"),
    tests=[
        # Run eventlog tests with the Windows API, which depend on the EventLog service
        IntegrationTest(dir="./pkg/util/winutil/", prefix="./eventlog/...", extra_args="-evtapi Windows"),
        # Run eventlog tailer tests with the Windows API, which depend on the EventLog service
        IntegrationTest(dir=".", prefix="./pkg/logs/tailers/windowsevent/...", extra_args="-evtapi Windows"),
        # Run eventlog check tests with the Windows API, which depend on the EventLog service
        # Don't include submodules, since the `-evtapi` flag is not defined in them
        IntegrationTest(
            dir=".", prefix="./comp/checks/windowseventlog/windowseventlogimpl/check", extra_args="-evtapi Windows"
        ),
    ],
)

DOGSTATSD = IntegrationTestsConfig(
    name="DogStatsD",
    go_build_tags=get_default_build_tags(build="test"),
    tests=[IntegrationTest(prefix="./test/integration/dogstatsd/...")],
)

CLUSTER_AGENT = IntegrationTestsConfig(
    name="Cluster Agent",
    go_build_tags=get_default_build_tags(build="cluster-agent") + ["docker", "test"],
    tests=[
        IntegrationTest(prefix="./test/integration/util/kube_apiserver"),
        IntegrationTest(prefix="./test/integration/util/leaderelection"),
    ],
)

TRACE_AGENT = IntegrationTestsConfig(
    name="Trace Agent",
    go_build_tags=get_default_build_tags(build="test"),
    tests=[IntegrationTest(prefix="./cmd/trace-agent/test/testsuite/...")],
    env={"INTEGRATION": "yes"},
)


def containerized_integration_tests(
    ctx: Context,
    integration_tests_config: IntegrationTestsConfig,
    race=False,
    remote_docker=False,
    go_mod="readonly",
    timeout="",
):
    test_args = {
        "go_mod": go_mod,
        "go_build_tags": " ".join(integration_tests_config.go_build_tags),
        "race_opt": "-race" if race else "",
        "exec_opts": "",
        "timeout_opt": f"-timeout {timeout}" if timeout else "",
    }

    # since Go 1.13, the -exec flag of go test could add some parameters such as -test.timeout
    # to the call, we don't want them because while calling invoke below, invoke
    # thinks that the parameters are for it to interpret.
    # we're calling an intermediate script which only pass the binary name to the invoke task.
    if remote_docker:
        test_args["exec_opts"] = f"-exec \"{os.getcwd()}/test/integration/dockerize_tests.sh\""

    go_cmd = 'go test {timeout_opt} -mod={go_mod} {race_opt} -tags "{go_build_tags}" {exec_opts}'.format(**test_args)  # noqa: FS002

    for it in integration_tests_config.tests:
        if it.dir:
            with ctx.cd(f"{it.dir}"):
                ctx.run(f"{go_cmd} {it.prefix}", env=integration_tests_config.env)
        else:
            ctx.run(f"{go_cmd} {it.prefix}", env=integration_tests_config.env)


def dsd_integration_tests(ctx, race=False, remote_docker=False, go_mod="readonly", timeout=""):
    """
    Run integration tests for dogstatsd
    """
    if sys.platform == 'win32':
        raise TestsNotSupportedError('DogStatsD integration tests are not supported on Windows')
    containerized_integration_tests(
        ctx,
        DOGSTATSD,
        race=race,
        remote_docker=remote_docker,
        go_mod=go_mod,
        timeout=timeout,
    )


def dca_integration_tests(ctx, race=False, remote_docker=False, go_mod="readonly", timeout=""):
    """
    Run integration tests for cluster-agent
    """
    if sys.platform == 'win32':
        raise TestsNotSupportedError('Cluster Agent integration tests are not supported on Windows')
    containerized_integration_tests(
        ctx,
        CLUSTER_AGENT,
        race=race,
        remote_docker=remote_docker,
        go_mod=go_mod,
        timeout=timeout,
    )


def trace_integration_tests(ctx, race=False, go_mod="readonly", timeout="10m"):
    """
    Run integration tests for trace agent
    """
    if sys.platform == 'win32':
        raise TestsNotSupportedError('Trace Agent integration tests are not supported on Windows')
    containerized_integration_tests(
        ctx,
        TRACE_AGENT,
        race=race,
        remote_docker=False,
        go_mod=go_mod,
        timeout=timeout,
    )


def core_integration_tests(ctx, race=False, remote_docker=False, go_mod="readonly", timeout=""):
    """
    Run integration tests for the Agent
    """
    if sys.platform == 'win32':
        return containerized_integration_tests(ctx, CORE_AGENT_WINDOWS, race=race, go_mod=go_mod, timeout=timeout)
    return containerized_integration_tests(
        ctx,
        CORE_AGENT_LINUX,
        race=race,
        remote_docker=remote_docker,
        go_mod=go_mod,
        timeout=timeout,
    )


@task
def integration_tests(ctx, race=False, remote_docker=False, timeout=""):
    """
    Run all the available integration tests
    """
    tests = {
        "Agent": lambda: core_integration_tests(ctx, race=race, remote_docker=remote_docker, timeout=timeout),
        "DogStatsD": lambda: dsd_integration_tests(ctx, race=race, remote_docker=remote_docker, timeout=timeout),
        "Cluster Agent": lambda: dca_integration_tests(ctx, race=race, remote_docker=remote_docker, timeout=timeout),
        "Trace Agent": lambda: trace_integration_tests(ctx, race=race, timeout=timeout),
    }
    tests_failures = {}
    for t_name, t in tests.items():
        with gitlab_section(f"Running the {t_name} integration tests", collapsed=True, echo=True):
            try:
                t()
            except TestsNotSupportedError as e:
                print(f"Skipping {t_name}: {e}")
            except Exception:
                # Keep printing the traceback not to have to wait until all tests are done to see what failed
                traceback.print_exc()
                # Storing the traceback to print it at the end without directly raising the exception
                tests_failures[t_name] = traceback.format_exc()
    if tests_failures:
        print("Integration tests failed:")
        for t_name, t_failure in tests_failures.items():
            print(f"{t_name}:\n{t_failure}")
        raise Exit(code=1)
