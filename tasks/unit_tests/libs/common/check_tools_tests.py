import unittest
from unittest.mock import MagicMock, patch

from invoke import Context, Exit, MockContext, Result, UnexpectedExit

from tasks.libs.common.check_tools_version import check_tools_installed
from tasks.protobuf import check_tools


class TestCheckToolsInstalled(unittest.TestCase):
    def test_single_tool_installed(self):
        self.assertTrue(check_tools_installed(["git"]))

    def test_several_tools_installed(self):
        self.assertTrue(check_tools_installed(["git", "ls"]))

    def test_tool_not_installed(self):
        self.assertFalse(check_tools_installed(["not_installed_tool", "ls"]))


class TestCheckTools(unittest.TestCase):
    @patch('tasks.protobuf.check_tools_installed', new=MagicMock(return_value=False))
    def test_tools_not_installed(self):
        c = Context()
        with self.assertRaises(Exit) as e:
            check_tools(c)
        self.assertEqual(
            e.exception.message,
            "Please install the required tools with `dda inv install-tools` before running this task.",
        )

    @patch('tasks.protobuf.check_tools_installed', new=MagicMock(return_value=True))
    def test_bad_protoc(self):
        c = MockContext(run={'protoc --version': Result("libprotoc 1.98.2")})
        with self.assertRaises(Exit) as e:
            check_tools(c)
        self.assertTrue(e.exception.message.startswith("Expected protoc version 29.3, found"))

    @patch('tasks.protobuf.check_tools_installed', new=MagicMock(return_value=True))
    def test_protoc_not_installed(self):
        c = MagicMock()
        c.run.side_effect = UnexpectedExit("protoc --version")
        with self.assertRaises(Exit) as e:
            check_tools(c)
        self.assertEqual(e.exception.message, "protoc is not installed. Please install it before running this task.")
