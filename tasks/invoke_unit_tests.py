import time
import os
import glob
from collections import defaultdict

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.ci_visibility import ci_visibility_section, CIVisibilitySection

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
def run(ctx, tests: str = '', buffer: bool = True, verbosity: int = 1, debug: bool = True, directory: str = '.'):
    """
    Run the unit tests on the invoke tasks

    - buffer: Buffer stdout / stderr from tests, useful to avoid interleaving output from tests
    - verbosity: Level of verbosity
    - debug: If True, will propagate errors to the debugger
    - directory: Directory where the tests are located
    """

    tests = [test for test in tests.split(',') if test]

    if tests:
        error = False
        for i, test in enumerate(tests):
            if i > 0:
                print()
            print(color_message('Running tests from module', Color.BLUE), color_message(f'{test}_tests', Color.BOLD))

            pattern = '*_tests.py' if len(tests) == 0 else test + '_tests.py'
            if not run_unit_tests(ctx, pattern, buffer, verbosity, debug, directory):
                error = True

        # Throw error if more than one module fails
        if error:
            if len(tests) > 1:
                raise Exit(color_message('Some tests are failing', Color.RED), code=1)
            else:
                raise Exit(code=1)
    else:
        pattern = '*_tests.py'
        if not run_unit_tests(ctx, pattern, buffer, verbosity, debug, directory):
            raise Exit(code=1)


def run_unit_tests(_, pattern, buffer, verbosity, debug, directory):
    import unittest

    old_environ = os.environ.copy()

    try:
        # Update env
        for key, value in TEST_ENV.items():
            if key not in os.environ:
                os.environ[key] = value

        loader = unittest.TestLoader()
        suite = loader.discover(directory, pattern=pattern)
        if debug and 'TASKS_DEBUG' in os.environ:
            suite.debug()

            # Will raise an error if the tests fail
            return True
        else:
            start_time = time.time()
            runner = unittest.TextTestRunner(buffer=buffer, verbosity=verbosity)
            res = runner.run(suite)

            t = start_time
            for name, duration in res.collectedDurations:
                print(f'Test {name} took {duration:.2f}s')
                simple_name = name.split(' ')[0]
                CIVisibilitySection.create(simple_name, t, t + duration, tags={'test-name': name, 'agent-category': 'invoke-unit-tests'})

                t += duration

            return res.wasSuccessful()
    finally:
        # Restore env
        os.environ.clear()
        os.environ.update(old_environ)


# # TODO: Remove
# @task
# def run_and_profile(ctx):
#     import unittest

#     # tests = sorted(list(glob.glob('tasks/unit_tests/**/*_tests.py', recursive=True)))
#     tests = sorted(list(glob.glob('tasks/unit_tests/**/*y_tests.py', recursive=True)))
#     print(len(tests), 'tests found')
#     print(tests)

#     error = False
#     times = []
#     for test in tests:
#         start_time = time.time()
#         dir, filename = os.path.split(test)
#         test_name = filename.removesuffix("_tests.py")
#         with ci_visibility_section(test_name):
#             try:
#                 loader = unittest.TestLoader()
#                 suite = loader.discover(dir, pattern=filename)
#                 runner = unittest.TextTestRunner()
#                 res = runner.run(suite)
#                 error = not res.wasSuccessful()
#                 print('durations', res.collectedDurations)
#             except:
#                 error = True

#         if error:
#             print('Error in', test)
#             error = True

#         times.append((test_name, start_time, time.time()))

#     # # Create ci visibility sections
#     # print('Creating CI visibility spans')
#     # for test_name, start_time, end_time in times:
#     #     create_ci_visibility_section(ctx, test_name, start_time, end_time)

#     if error:
#         raise Exit(color_message('Some tests are failing', Color.RED), code=1)
