import os
import unittest
from unittest.mock import MagicMock, patch

from invoke import MockContext
from invoke.exceptions import Exit

from .. import go_test


class TestLintSkipQA(unittest.TestCase):
    @patch('builtins.print')
    def test_on_default(self, mock_print):
        os.environ["BRANCH_NAME"] = "main"
        os.environ["PR_ID"] = "42"
        go_test.lint_skip_qa(MockContext())
        mock_print.assert_called_with(f"Running on {go_test.DEFAULT_BRANCH}, skipping check for skip-qa label.")

    @patch('builtins.print')
    def test_no_pr(self, mock_print):
        os.environ["BRANCH_NAME"] = "pied"
        go_test.lint_skip_qa(MockContext())
        mock_print.assert_called_with("PR not found, skipping check for skip-qa.")

    @patch('builtins.print')
    @patch('requests.get')
    def test_no_skip_qa(self, mock_requests_get, mock_print):
        os.environ["BRANCH_NAME"] = "oak"
        os.environ["PR_ID"] = "51"
        issue = {'labels': [{'name': 'de_cadix_a_des_yeux_de_velours'}]}
        mock_response = MagicMock()
        mock_response.json.return_value = issue
        mock_requests_get.return_value = mock_response
        go_test.lint_skip_qa(MockContext())
        mock_print.assert_not_called()

    @patch('requests.get')
    def test_skip_qa_alone(self, mock_requests_get):
        os.environ["BRANCH_NAME"] = "mapple"
        os.environ["PR_ID"] = "69"
        issue = {'labels': [{'name': 'qa/skip-qa'}]}
        mock_response = MagicMock()
        mock_response.json.return_value = issue
        mock_requests_get.return_value = mock_response
        with self.assertRaises(Exit):
            go_test.lint_skip_qa(MockContext())

    @patch('requests.get')
    def test_skip_qa_bad_label(self, mock_requests_get):
        os.environ["BRANCH_NAME"] = "ash"
        os.environ["PR_ID"] = "666"
        issue = {'labels': [{'name': 'qa/skip-qa'}, {"name": "qa/lity-streets"}]}
        mock_response = MagicMock()
        mock_response.json.return_value = issue
        mock_requests_get.return_value = mock_response
        with self.assertRaises(Exit):
            go_test.lint_skip_qa(MockContext())

    @patch('builtins.print')
    @patch('requests.get')
    def test_skip_qa_done(self, mock_requests_get, mock_print):
        os.environ["BRANCH_NAME"] = "gingko"
        os.environ["PR_ID"] = "1337"
        issue = {'labels': [{'name': 'qa/skip-qa'}, {'name': 'qa/done'}]}
        mock_response = MagicMock()
        mock_response.json.return_value = issue
        mock_requests_get.return_value = mock_response
        go_test.lint_skip_qa(MockContext())
        mock_print.assert_not_called()

    @patch('builtins.print')
    @patch('requests.get')
    def test_skip_qa_done_alone(self, mock_requests_get, mock_print):
        os.environ["BRANCH_NAME"] = "beech"
        os.environ["PR_ID"] = "1515"
        issue = {'labels': [{'name': 'qa/done'}]}
        mock_response = MagicMock()
        mock_response.json.return_value = issue
        mock_requests_get.return_value = mock_response
        go_test.lint_skip_qa(MockContext())
        mock_print.assert_not_called()

    @patch('builtins.print')
    @patch('requests.get')
    def test_skip_qa_no_code(self, mock_requests_get, mock_print):
        os.environ["BRANCH_NAME"] = "sequoia"
        os.environ["PR_ID"] = "1664"
        issue = {'labels': [{'name': 'qa/skip-qa'}, {'name': 'qa/no-code-change'}]}
        mock_response = MagicMock()
        mock_response.json.return_value = issue
        mock_requests_get.return_value = mock_response
        go_test.lint_skip_qa(MockContext())
        mock_print.assert_not_called()


if __name__ == "__main__":
    unittest.main()
