import os

from invoke import task
from invoke.exceptions import Exit


@task(default=True)
def run(ctx, buffer: bool = True, verbosity: int = 1, debug: bool = True):
    """
    Run the unit tests for omnubus python-scripts

    - buffer: Buffer stdout / stderr from tests, useful to avoid interleaving output from tests
    - verbosity: Level of verbosity
    - debug: If True, will propagate errors to the debugger
    """

    cwd = os.getcwd()

    # change to the directory where the tests are located
    file_path = os.path.abspath(__file__)
    project_root = os.path.dirname(os.path.dirname(file_path))
    python_scripts_dir = os.path.join(project_root, "omnibus", "python-scripts")
    print(f"Changing to directory: {python_scripts_dir}")
    os.chdir(python_scripts_dir)

    pattern = '*_tests.py'

    result = run_unit_tests(ctx, pattern, buffer, verbosity, debug)

    # change back to the original directory
    print(f"Changing back to directory: {cwd}")
    os.chdir(cwd)

    if not result:
        raise Exit(code=1)


def run_unit_tests(_, pattern, buffer, verbosity, debug):
    import unittest

    old_environ = os.environ.copy()
    try:
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
