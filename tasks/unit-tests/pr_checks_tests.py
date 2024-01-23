import os
import unittest
from unittest.mock import patch

from invoke import MockContext
from invoke.exceptions import Exit

from tasks import pr_checks


class TestLintSkipQA(unittest.TestCase):
    @patch('builtins.print')
    def test_on_default(self, mock_print):
        os.environ["BRANCH_NAME"] = "main"
        os.environ["PR_ID"] = "42"
        pr_checks.lint_skip_qa(MockContext())
        mock_print.assert_called_with(f"Running on {pr_checks.DEFAULT_BRANCH}, skipping check for skip-qa.")

    @patch('builtins.print')
    def test_no_pr(self, mock_print):
        os.environ["BRANCH_NAME"] = "pied"
        os.environ["PR_ID"] = ""
        pr_checks.lint_skip_qa(MockContext())
        mock_print.assert_called_with("PR not found, skipping check for skip-qa.")

    @patch('builtins.print')
    @patch.object(pr_checks.GithubAPI, 'is_qa_skip_ok')
    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_no_skip_qa(self, _, mock_pr_check, mock_print):
        os.environ["BRANCH_NAME"] = "oak"
        os.environ["PR_ID"] = "51"
        mock_pr_check.return_value = False
        with self.assertRaises(Exit):
            pr_checks.lint_skip_qa(MockContext())
        mock_print.assert_called_with(
            "PR 51 request to skip QA without justification. Requires an additional `qa/done` or `qa/no-code-change`."
        )

    @patch('builtins.print')
    @patch.object(pr_checks.GithubAPI, 'is_qa_skip_ok')
    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_skip_qa(self, _, mock_pr_check, mock_print):
        os.environ["BRANCH_NAME"] = "mapple"
        os.environ["PR_ID"] = "69"
        mock_pr_check.return_value = True
        pr_checks.lint_skip_qa(MockContext())
        mock_print.assert_not_called()


class TestLintMilestone(unittest.TestCase):
    @patch('builtins.print')
    @patch.object(pr_checks.GithubAPI, 'get_pr_milestone')
    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_no_milestone(self, _, mock_pr_check, mock_print):
        os.environ["BRANCH_NAME"] = "bismutotantalite"
        os.environ["PR_ID"] = "5"
        mock_pr_check.return_value = None
        with self.assertRaises(Exit):
            pr_checks.lint_milestone(MockContext())
            mock_print.assert_called_with(f"PR {os.environ['PR_ID']} requires a non-Triage milestone.")

    @patch('builtins.print')
    @patch.object(pr_checks.GithubAPI, 'get_pr_milestone')
    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_triage(self, _, mock_pr_check, mock_print):
        os.environ["BRANCH_NAME"] = "dolomite"
        os.environ["PR_ID"] = "4"
        mock_pr_check.return_value = "Triage"
        with self.assertRaises(Exit):
            pr_checks.lint_milestone(MockContext())
            mock_print.assert_called_with(f"PR {os.environ['PR_ID']} requires a non-Triage milestone.")

    @patch('builtins.print')
    @patch.object(pr_checks.GithubAPI, 'get_pr_milestone')
    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_milestone_ok(self, _, mock_pr_check, mock_print):
        os.environ["BRANCH_NAME"] = "zircon"
        os.environ["PR_ID"] = "6"
        mock_pr_check.return_value = "Milestone"
        pr_checks.lint_milestone(MockContext())
        mock_print.assert_called_with("Milestone: Milestone")
