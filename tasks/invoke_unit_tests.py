import os

from invoke import task
from invoke.exceptions import Exit

TEST_ENV = {
    'INVOKE_UNIT_TESTS': '1',
    'CI_COMMIT_REF_NAME': 'mybranch',
    'CI_COMMIT_SHA': 'abcdef42',
    'CI_DEFAULT_BRANCH': 'themainbranch',
    'CI_PIPELINE_ID': '42',
    'CI_JOB_TOKEN': '618',
    'CI_PROJECT_DIR': '.',
}


@task(default=True)
def run(
    ctx,
    buffer: bool = True,
    verbosity: int = 1,
    debug: bool = True,
    target: str = '',
):
    """
    Run the unit tests on the invoke tasks

    - buffer: Buffer stdout / stderr from tests, useful to avoid interleaving output from tests
    - verbosity: Level of verbosity
    - debug: If True, will propagate errors to the debugger
    - target: Target to run the tests on, should pytest syntax (e.g. test_foo.py::test_bar) or be a directory
    """

    if not run_unit_tests(ctx, target, buffer, verbosity, debug):
        raise Exit(code=1)


def run_unit_tests(_, target, buffer, verbosity, debug):
    import unittest

    old_environ = os.environ.copy()
    try:
        # Update env
        for key, value in TEST_ENV.items():
            if key not in os.environ:
                os.environ[key] = value

        loader = unittest.TestLoader()

        # If target is a directory, set directory to target and target to empty
        if os.path.isdir(target):
            directory = target
            target = ""

        # Check if target is a precise test target (path.py, path::TestClass::test, or path::TestClass)
        if target:
            test_names = [_target_to_test_name(t) for t in target.split(',')]
            suite = loader.loadTestsFromNames(test_names)
        else:
            suite = loader.discover(start_dir=directory, pattern='*_tests.py')
        if debug and 'TASKS_DEBUG' in os.environ:
            suite.debug()
            # Will raise an error if the tests fail
            return True

        runner = unittest.TextTestRunner(buffer=buffer, verbosity=verbosity)
        return runner.run(suite).wasSuccessful()
    finally:
        # Restore env
        os.environ.clear()
        os.environ.update(old_environ)


def _target_to_test_name(target: str) -> str:
    """
    Converts from pytest-style target to unittest-style dotted name:
    - path/to/test.py::TestClass::test_method -> path.to.test.TestClass.test_method
    """
    parts = target.split('::')
    file_path = parts[0]

    # Convert file path to module path (remove .py and replace / with .)
    module_path = file_path.replace('.py', '').replace('/', '.')

    # Build the full test name
    test_name = module_path
    if len(parts) > 1:
        test_name += '.' + '.'.join(parts[1:])
    return test_name
