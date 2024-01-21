import os
import re
import sys

from invoke import task

UNIT_TEST_FILE_FORMAT = re.compile(r'[^a-zA-Z0-9_\-]')


@task
def invoke_unit_tests(ctx):
    """
    Run the unit tests on the invoke tasks
    """
    for _, _, files in os.walk("tasks/unit-tests/"):
        for file in files:
            if file[-3:] == ".py" and file != "__init__.py" and not bool(UNIT_TEST_FILE_FORMAT.search(file[:-3])):
                ctx.run(f"{sys.executable} -m tasks.unit-tests.{file[:-3]}", env={"GITLAB_TOKEN": "fake_token"})
