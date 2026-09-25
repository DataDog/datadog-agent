import unittest
from unittest.mock import patch

from invoke import Context, Exit

from tasks.linter import dotslash


class TestDotSlashTask(unittest.TestCase):
    @patch("tasks.linter.get_repo_root", return_value="checkout")
    @patch("tasks.linter.validate_dotslash", return_value=True)
    def test_options_are_forwarded(self, validate, _root):
        dotslash.body(Context(), download=True, smoke=True, changed_since="origin/main", report="report.json")
        validate.assert_called_once_with(
            "checkout", download=True, smoke=True, changed_since="origin/main", report="report.json"
        )

    @patch("tasks.linter.get_repo_root", return_value="checkout")
    @patch("tasks.linter.validate_dotslash", return_value=False)
    def test_failure_sets_exit_status(self, _validate, _root):
        with self.assertRaises(Exit) as failure:
            dotslash.body(Context())
        self.assertEqual(failure.exception.code, 1)
