import re
import tempfile
import unittest
from pathlib import Path
from typing import cast

from invoke.context import Context, MockContext

from tasks.kernel_matrix_testing.setup import _topological_sort_requirements
from tasks.kernel_matrix_testing.setup.common import Pulumi
from tasks.kernel_matrix_testing.setup.requirement import Requirement
from tasks.kernel_matrix_testing.setup.utils import _patch_config_lines, ensure_options_in_config
from tasks.libs.common.status import Status


class TestPulumiRequirement(unittest.TestCase):
    def test_pulumi_user_logged_in(self):
        logged_out_output = """
        Backend
Name           pulumi.com
URL            https://app.pulumi.com
User           Unknown
Organizations
Token type     personal
"""

        ctx = MockContext(run={"pulumi about": logged_out_output})
        self.assertEqual(Pulumi()._check_user_logged_in(ctx).state, Status.FAIL)

        logged_in_output = """
Backend
Name           COMPNAME
URL            file://~
User           user.name
Organizations
Token type     personal
"""

        ctx = MockContext(run={"pulumi about": logged_in_output})
        self.assertEqual(Pulumi()._check_user_logged_in(ctx).state, Status.OK)


class MockContextWithTempFile:
    def __init__(self, command_regex: re.Pattern):
        self.command_regex = command_regex
        self.internal_temp_file_contents: str = ""

    def run(self, command: str, **_):
        match = self.command_regex.match(command)
        if match:
            file = match.group(1)
            with open(file) as f:
                self.internal_temp_file_contents = f.read()


class TestOptionsInConfig(unittest.TestCase):
    options_file_content = """option1 = "value1"
option2 = nothing
option3 = 3
# option4 = nothing
something = else
# option5 = nothing
# intopt = 4
"""

    options = {
        "option1": "value1",
        "option2": "value2",
        "option3": 3,
        "option4": "value4",
        "intopt": 4,
    }

    expected_file_content = """option1 = "value1"
option2 = "value2"
option3 = 3
option4 = "value4"
something = else
# option5 = nothing
intopt = 4
"""

    def test_patch_config_lines__no_change(self):
        lines = self.options_file_content.splitlines()

        incorrect_options, updated_lines = _patch_config_lines(lines, self.options, change=False)
        self.assertEqual(incorrect_options, ["option2", "option4", "intopt"])
        self.assertEqual(updated_lines, lines)

    def test_patch_config_lines__change(self):
        lines = self.options_file_content.splitlines()
        expected_lines = self.expected_file_content.splitlines()

        incorrect_options, updated_lines = _patch_config_lines(lines, self.options, change=True)
        self.assertEqual(incorrect_options, ["option2", "option4", "intopt"])
        self.assertEqual(updated_lines, expected_lines)

    def test_ensure_options_in_config__no_change(self):
        with tempfile.NamedTemporaryFile(delete_on_close=False) as temp_file:
            temp_file.write(self.options_file_content.encode("utf-8"))
            temp_file.close()

            # Any call to ctx.run() will fail as we are not specifying any command to mock
            ctx = MockContext()

            incorrect_options = ensure_options_in_config(ctx, Path(temp_file.name), self.options, change=False)
            self.assertEqual(incorrect_options, ["option2", "option4", "intopt"])

    def test_ensure_options_in_config__change(self):
        with tempfile.NamedTemporaryFile(delete_on_close=False) as temp_file:
            temp_file.write(self.options_file_content.encode("utf-8"))
            temp_file.close()

            ctx = MockContextWithTempFile(re.compile(rf"mv ([^ ]+) {temp_file.name}"))

            incorrect_options = ensure_options_in_config(
                cast(Context, ctx), Path(temp_file.name), self.options, change=True, write_with_sudo=False
            )
            self.assertEqual(incorrect_options, ["option2", "option4", "intopt"])

            # Check that the file was modified
            self.assertEqual(ctx.internal_temp_file_contents.strip(), self.expected_file_content.strip())

    def test_ensure_options_in_config__change_with_sudo(self):
        with tempfile.NamedTemporaryFile(delete_on_close=False) as temp_file:
            temp_file.write(self.options_file_content.encode("utf-8"))
            temp_file.close()

            ctx = MockContextWithTempFile(re.compile(rf"sudo mv ([^ ]+) {temp_file.name}"))

            incorrect_options = ensure_options_in_config(
                cast(Context, ctx), Path(temp_file.name), self.options, change=True, write_with_sudo=True
            )
            self.assertEqual(incorrect_options, ["option2", "option4", "intopt"])

            # Check that the file was modified
            self.assertEqual(ctx.internal_temp_file_contents.strip(), self.expected_file_content.strip())


class TestTopologicalSortRequirements(unittest.TestCase):
    def test_no_dependencies(self):
        class Req1(Requirement):
            pass

        class Req2(Requirement):
            pass

        reqs = [Req1(), Req2()]
        result = _topological_sort_requirements(reqs)

        self.assertEqual(len(result), 2)
        self.assertIn(reqs[0], result)
        self.assertIn(reqs[1], result)

    def test_with_dependencies(self):
        class ReqBase(Requirement):
            pass

        class ReqDependent(Requirement):
            dependencies: list[type[Requirement]] = [ReqBase]

        class ReqOther(Requirement):
            pass

        reqs = [ReqDependent(), ReqOther(), ReqBase()]
        result = _topological_sort_requirements(reqs)
        self.assertEqual(len(result), 3)
        baseIndex = result.index(reqs[2])
        dependentIndex = result.index(reqs[0])

        self.assertGreater(dependentIndex, baseIndex)
