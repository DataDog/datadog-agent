import sys

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message

TEST_ENV = {
    'INVOKE_UNIT_TESTS': '1',
    'GITLAB_TOKEN': 'fake_token',
    'CI_COMMIT_REF_NAME': 'mybranch',
    'CI_COMMIT_SHA': 'abcdef42',
    'CI_DEFAULT_BRANCH': 'themainbranch',
    'CI_PIPELINE_ID': '42',
    'CI_JOB_TOKEN': '618',
    'CI_PROJECT_DIR': '.',
}


@task
def run(ctx, tests: str = '', flags: str = '-b'):
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
        if error:
            if len(tests) > 1:
                raise Exit(color_message('Some tests are failing', Color.RED), code=1)
            else:
                raise Exit(code=1)
    else:
        command = f"'{sys.executable}' -m unittest discover {flags} -s tasks -p '*_tests.py'"
        if not run_unit_tests_command(ctx, command):
            raise Exit(code=1)


def run_unit_tests_command(ctx, command):
    return ctx.run(
        command,
        env=TEST_ENV,
        warn=True,
    )
