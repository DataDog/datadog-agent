import sys

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message


@task
def invoke_unit_tests(ctx, tests: str = '', flags: str = '-b'):
    """
    Run the unit tests on the invoke tasks
    """

    tests = [test for test in tests.split(',') if test]

    if tests:
        error = False
        for i, test in enumerate(tests):
            if i > 0:
                print()
            print(color_message('Running tests from module', Color.BLUE), color_message(f'{test}_tests', Color.BOLD))

            pattern = '*_tests.py' if len(tests) == 0 else test + '_tests.py'
            command = f"'{sys.executable}' -m unittest discover {flags} -s tasks -p '{pattern}'"
            if not run_unit_tests_command(ctx, command):
                error = True

        # Throw error if more than one module fails
        if error and len(tests) > 1:
            raise Exit(color_message('Some tests are failing', Color.RED))
    else:
        command = f"'{sys.executable}' -m unittest discover {flags} -s tasks -p '*_tests.py'"
        run_unit_tests_command(ctx, command)


def run_unit_tests_command(ctx, command):
    return ctx.run(
        command,
        env={
            "INVOKE_UNIT_TESTS": "1",
            "GITLAB_TOKEN": "fake_token",
            'CI_COMMIT_REF_NAME': 'mybranch',
            'CI_DEFAULT_BRANCH': 'themainbranch',
            'CI_PIPELINE_ID': '42',
        },
        warn=True,
    )
