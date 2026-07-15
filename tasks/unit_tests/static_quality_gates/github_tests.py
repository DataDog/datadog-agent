import unittest
from unittest.mock import MagicMock, patch

from tasks.static_quality_gates.github import (
    get_pr_author,
    get_pr_for_branch,
    get_pr_number_from_commit,
)


class TestGetPrForBranch(unittest.TestCase):
    """Test the get_pr_for_branch helper function."""

    @patch("tasks.static_quality_gates.github.GithubAPI")
    def test_returns_pr_when_found(self, mock_github_class):
        """Should return PR object when a PR exists for the branch."""
        mock_pr = MagicMock()
        mock_pr.number = 12345
        mock_pr.title = "Test PR"
        mock_github = MagicMock()
        mock_github.get_pr_for_branch.return_value = [mock_pr]
        mock_github_class.return_value = mock_github

        result = get_pr_for_branch("test-branch")

        self.assertEqual(result, mock_pr)
        self.assertEqual(result.number, 12345)
        mock_github.get_pr_for_branch.assert_called_once_with("test-branch")

    @patch("tasks.static_quality_gates.github.GithubAPI")
    def test_returns_none_when_no_pr(self, mock_github_class):
        """Should return None when no PR exists for the branch."""
        mock_github = MagicMock()
        mock_github.get_pr_for_branch.return_value = []
        mock_github_class.return_value = mock_github

        result = get_pr_for_branch("test-branch")

        self.assertIsNone(result)

    @patch("tasks.static_quality_gates.github.GithubAPI")
    def test_returns_none_on_exception(self, mock_github_class):
        """Should return None and not raise when GitHub API fails."""
        mock_github_class.side_effect = Exception("API error")

        result = get_pr_for_branch("test-branch")

        self.assertIsNone(result)

    @patch("tasks.static_quality_gates.github.GithubAPI")
    def test_returns_first_pr_when_multiple(self, mock_github_class):
        """Should return first PR when multiple PRs exist for branch."""
        mock_pr1 = MagicMock()
        mock_pr1.number = 111
        mock_pr2 = MagicMock()
        mock_pr2.number = 222
        mock_github = MagicMock()
        mock_github.get_pr_for_branch.return_value = [mock_pr1, mock_pr2]
        mock_github_class.return_value = mock_github

        result = get_pr_for_branch("test-branch")

        self.assertEqual(result.number, 111)


class TestGetPrNumberFromCommit(unittest.TestCase):
    """Test the get_pr_number_from_commit helper function."""

    def test_extracts_pr_number_standard_format(self):
        """Should extract PR number from standard merge commit format."""
        mock_ctx = MagicMock()
        mock_result = MagicMock()
        mock_result.stdout = "Fix bug in quality gates (#44462)\n"
        mock_ctx.run.return_value = mock_result

        result = get_pr_number_from_commit(mock_ctx)

        self.assertEqual(result, "44462")
        mock_ctx.run.assert_called_once_with("git log -1 --pretty=%s HEAD", hide=True)

    def test_extracts_pr_number_with_trailing_whitespace(self):
        """Should extract PR number even with trailing whitespace."""
        mock_ctx = MagicMock()
        mock_result = MagicMock()
        mock_result.stdout = "Add new feature (#12345)   \n"
        mock_ctx.run.return_value = mock_result

        result = get_pr_number_from_commit(mock_ctx)

        self.assertEqual(result, "12345")

    def test_extracts_pr_number_long_number(self):
        """Should handle PR numbers of various lengths."""
        mock_ctx = MagicMock()
        mock_result = MagicMock()
        mock_result.stdout = "Update docs (#1)\n"
        mock_ctx.run.return_value = mock_result

        result = get_pr_number_from_commit(mock_ctx)

        self.assertEqual(result, "1")

    def test_returns_none_when_no_pr_pattern(self):
        """Should return None when commit message doesn't contain PR pattern."""
        mock_ctx = MagicMock()
        mock_result = MagicMock()
        mock_result.stdout = "Initial commit\n"
        mock_ctx.run.return_value = mock_result

        result = get_pr_number_from_commit(mock_ctx)

        self.assertIsNone(result)

    def test_returns_none_when_pr_pattern_not_at_end(self):
        """Should return None when PR pattern is not at the end."""
        mock_ctx = MagicMock()
        mock_result = MagicMock()
        mock_result.stdout = "Fix (#123) issue with something\n"
        mock_ctx.run.return_value = mock_result

        result = get_pr_number_from_commit(mock_ctx)

        self.assertIsNone(result)

    def test_returns_none_on_git_error(self):
        """Should return None when git command fails."""
        mock_ctx = MagicMock()
        mock_ctx.run.side_effect = Exception("git command failed")

        result = get_pr_number_from_commit(mock_ctx)

        self.assertIsNone(result)

    def test_handles_squash_merge_format(self):
        """Should handle squash merge commit format."""
        mock_ctx = MagicMock()
        mock_result = MagicMock()
        mock_result.stdout = "[backport/7.x] Fix security issue (#99999)\n"
        mock_ctx.run.return_value = mock_result

        result = get_pr_number_from_commit(mock_ctx)

        self.assertEqual(result, "99999")

    def test_extracts_revert_pr_not_original(self):
        """Should extract the revert PR number, not the original PR from the reverted commit."""
        mock_ctx = MagicMock()
        mock_result = MagicMock()
        # Revert commit format: the revert PR (#44639) is at the end, original PR (#44326) is inside
        mock_result.stdout = 'Revert "build krb5 with bazel (#44326)" (#44639)\n'
        mock_ctx.run.return_value = mock_result

        result = get_pr_number_from_commit(mock_ctx)

        # Should extract 44639 (the revert PR), not 44326 (the original)
        self.assertEqual(result, "44639")


class TestGetPrAuthor(unittest.TestCase):
    """Test the get_pr_author helper function."""

    @patch("tasks.static_quality_gates.github.GithubAPI")
    def test_returns_author_when_found(self, mock_github_class):
        """Should return PR author login when a PR exists."""
        mock_pr = MagicMock()
        mock_pr.user = MagicMock()
        mock_pr.user.login = "octocat"
        mock_github = MagicMock()
        mock_github.get_pr.return_value = mock_pr
        mock_github_class.return_value = mock_github

        result = get_pr_author("12345")

        self.assertEqual(result, "octocat")
        mock_github.get_pr.assert_called_once_with(12345)

    @patch("tasks.static_quality_gates.github.GithubAPI")
    def test_returns_none_when_pr_not_found(self, mock_github_class):
        """Should return None when PR is not found."""
        mock_github = MagicMock()
        mock_github.get_pr.return_value = None
        mock_github_class.return_value = mock_github

        result = get_pr_author("12345")

        self.assertIsNone(result)

    @patch("tasks.static_quality_gates.github.GithubAPI")
    def test_returns_none_when_user_is_none(self, mock_github_class):
        """Should return None when PR exists but user is None."""
        mock_pr = MagicMock()
        mock_pr.user = None
        mock_github = MagicMock()
        mock_github.get_pr.return_value = mock_pr
        mock_github_class.return_value = mock_github

        result = get_pr_author("12345")

        self.assertIsNone(result)

    @patch("tasks.static_quality_gates.github.GithubAPI")
    def test_returns_none_on_exception(self, mock_github_class):
        """Should return None and not raise when GitHub API fails."""
        mock_github_class.side_effect = Exception("API error")

        result = get_pr_author("12345")

        self.assertIsNone(result)

    @patch("tasks.static_quality_gates.github.GithubAPI")
    def test_handles_string_pr_number(self, mock_github_class):
        """Should correctly convert string PR number to int."""
        mock_pr = MagicMock()
        mock_pr.user = MagicMock()
        mock_pr.user.login = "datadog-bot"
        mock_github = MagicMock()
        mock_github.get_pr.return_value = mock_pr
        mock_github_class.return_value = mock_github

        result = get_pr_author("99999")

        self.assertEqual(result, "datadog-bot")
        mock_github.get_pr.assert_called_once_with(99999)


if __name__ == '__main__':
    unittest.main()
