import sys

from invoke import task


@task(iterable=["tests"])
def invoke_unit_tests(ctx, tests=None):
    """
    Run the unit tests on the invoke tasks
    """

    if tests:
        command = f"'{sys.executable}' -m unittest -b {' '.join(tests)}"
    else:
        command = f"'{sys.executable}' -m unittest discover -b -s tasks -p '*_tests.py'"

    ctx.run(command, env={"GITLAB_TOKEN": "fake_token", "INVOKE_UNIT_TESTS": "1"})
