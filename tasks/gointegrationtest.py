import os
import sys
import traceback
from dataclasses import dataclass, field

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
    dir: str = ""
    extra_args: str = ""


@dataclass
class IntegrationTestsConfig:
    """
    Integration tests configuration
    """

    name: str
    go_build_tags: list[str]
    tests: list[IntegrationTest]
    env: dict[str, str] = field(default_factory=dict)
    is_windows_supported: bool = True


CORE_AGENT_WINDOWS_IT_CONF = IntegrationTestsConfig(
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
        # Run software inventory integration tests that compare against PowerShell Get-Package
        IntegrationTest(dir=".", prefix="./pkg/inventory/software", extra_args=""),
    ],
)

TRACE_AGENT_IT_CONF = IntegrationTestsConfig(
    name="Trace Agent",
    go_build_tags=get_default_build_tags(build="test"),
    tests=[IntegrationTest(prefix="./cmd/trace-agent/test/testsuite/...")],
    env={"INTEGRATION": "yes"},
    is_windows_supported=False,
)


def containerized_integration_tests(
    ctx: Context,
    integration_tests_config: IntegrationTestsConfig,
    race=False,
    go_mod="readonly",
    timeout="",
):
    if sys.platform == 'win32' and not integration_tests_config.is_windows_supported:
        raise TestsNotSupportedError(f'{integration_tests_config.name} integration tests are not supported on Windows')

    # On Windows, add current directory to PATH so libdatadog-interop.dll can be found
    env = integration_tests_config.env.copy()
    if sys.platform == 'win32':
        current_dir = os.getcwd()
        if 'PATH' in os.environ:
            env['PATH'] = f"{current_dir};{os.environ['PATH']}"
        else:
            env['PATH'] = current_dir

    test_args = {
        "go_mod": go_mod,
        "go_build_tags": " ".join(integration_tests_config.go_build_tags),
        "race_opt": "-race" if race else "",
        "timeout_opt": f"-timeout {timeout}" if timeout else "",
    }

    go_cmd = 'go test {timeout_opt} -mod={go_mod} {race_opt} -tags "{go_build_tags}"'.format(**test_args)  # noqa: FS002

    for it in integration_tests_config.tests:
        if it.dir:
            with ctx.cd(f"{it.dir}"):
                ctx.run(f"{go_cmd} {it.prefix}", env=env)
        else:
            ctx.run(f"{go_cmd} {it.prefix}", env=env)


@task(iterable=["only"])
def integration_tests(ctx, race=False, timeout="", only: list[str] | None = None):
    """
    Run all the available integration tests

    Args:
        only: Filter tests to run.
    """

    tests = {
        "Trace Agent": lambda: containerized_integration_tests(ctx, TRACE_AGENT_IT_CONF, race=race, timeout=timeout),
    }

    if sys.platform == 'win32':
        tests["Agent Core"] = lambda: containerized_integration_tests(
            ctx, CORE_AGENT_WINDOWS_IT_CONF, race=race, timeout=timeout
        )

    if only:
        tests = {name: tests[name] for name in tests if name in only}

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
        print("The following integration tests failed:")
        for t_name in tests_failures:
            print(f"- {t_name}")
        print("See the above logs to get the full traceback.")
        raise Exit(code=1)
