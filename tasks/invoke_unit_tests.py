import os

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


@task(default=True)
def run(ctx, tests: str = '', buffer: bool = True, verbosity: int = 1, debug: bool = True):
    """
    Run the unit tests on the invoke tasks

    - buffer: Buffer stdout / stderr from tests, useful to avoid interleaving output from tests
    - verbosity: Level of verbosity
    - debug: If True, will propagate errors to the debugger
    """

    tests = [test for test in tests.split(',') if test]

    if tests:
        error = False
        for i, test in enumerate(tests):
            if i > 0:
                print()
            print(color_message('Running tests from module', Color.BLUE), color_message(f'{test}_tests', Color.BOLD))

            pattern = '*_tests.py' if len(tests) == 0 else test + '_tests.py'
            if not run_unit_tests(ctx, pattern, buffer, verbosity, debug):
                error = True

        # Throw error if more than one module fails
        if error:
            if len(tests) > 1:
                raise Exit(color_message('Some tests are failing', Color.RED), code=1)
            else:
                raise Exit(code=1)
    else:
        pattern = '*_tests.py'
        if not run_unit_tests(ctx, pattern, buffer, verbosity, debug):
            raise Exit(code=1)


def run_unit_tests(_, pattern, buffer, verbosity, debug):
    import unittest

    old_environ = os.environ.copy()

    try:
        # Update env
        for key, value in TEST_ENV.items():
            if key not in os.environ:
                os.environ[key] = value

        loader = unittest.TestLoader()
        suite = loader.discover('.', pattern=pattern)
        if debug and 'TASKS_DEBUG' in os.environ:
            suite.debug()

            # Will raise an error if the tests fail
            return True
        else:
            runner = unittest.TextTestRunner(buffer=buffer, verbosity=verbosity)

            return runner.run(suite).wasSuccessful()
    finally:
        # Restore env
        os.environ.clear()
        os.environ.update(old_environ)
