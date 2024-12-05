import os
import traceback

from invoke import task
from invoke.exceptions import Exit

from tasks.agent import integration_tests as agent_integration_tests
from tasks.cluster_agent import integration_tests as dca_integration_tests
from tasks.dogstatsd import integration_tests as dsd_integration_tests
from tasks.libs.common.utils import TestsNotSupportedError, gitlab_section
from tasks.trace_agent import integration_tests as trace_integration_tests


@task
def integration_tests(ctx, race=False, remote_docker=False, timeout=""):
    """
    Run all the available integration tests
    """
    tests = {
        "Agent": lambda: agent_integration_tests(ctx, race=race, remote_docker=remote_docker, timeout=timeout),
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


def containerized_integration_tests(
    ctx, prefixes, go_build_tags, env=None, race=False, remote_docker=False, go_mod="readonly", timeout=""
):
    test_args = {
        "go_mod": go_mod,
        "go_build_tags": " ".join(go_build_tags),
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

    for prefix in prefixes:
        ctx.run(f"{go_cmd} {prefix}", env=env)
