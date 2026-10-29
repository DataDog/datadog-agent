import unittest
from contextlib import nullcontext
from unittest.mock import MagicMock, patch

from invoke import MockContext, Result

from tasks import notes


@patch('tasks.notes.create_release_pr')
@patch('tasks.notes.update_changelog_generic')
@patch('tasks.notes.get_current_branch', new=MagicMock(return_value='7.86.x'))
@patch('tasks.notes.deduce_version', new=MagicMock(return_value='7.86.0'))
@patch('tasks.notes.agent_context', new=MagicMock(return_value=nullcontext()))
class TestUpdateChangelog(unittest.TestCase):
    def _context(self):
        return MockContext(run=Result(""))

    @patch('tasks.notes.running_in_ci', new=MagicMock(return_value=True))
    @patch('tasks.notes.create_tree')
    @patch('tasks.notes.GithubAPI')
    def test_ci_commits_with_the_github_api(self, github_mock, create_tree_mock, generic_mock, create_pr_mock):
        tree = {"base_tree": "basesha", "tree": [{"path": "CHANGELOG.rst"}]}
        create_tree_mock.return_value = tree
        ctx = self._context()

        notes.update_changelog(ctx, '7.86.x')

        self.assertEqual(generic_mock.call_count, 2)
        create_tree_mock.assert_called_once_with(ctx, '7.86.x')
        github = github_mock.return_value
        github.create_branch.assert_called_once_with('changelog-update-7.86.0', 'basesha', reset=True)
        github.commit_and_push_signed.assert_called_once_with(
            'changelog-update-7.86.0', 'Changelog updates for 7.86.0 release', tree
        )
        commands = [c.args[0] for c in ctx.run.call_args_list]
        self.assertFalse(any(cmd.startswith(('git commit', 'git push')) for cmd in commands), commands)
        create_pr_mock.assert_called_once_with(
            'Changelog update for 7.86.0 release',
            '7.86.x',
            'changelog-update-7.86.0',
            '7.86.0',
            changelog_pr=True,
            milestone='7.86.0',
        )

    @patch('tasks.notes.running_in_ci', new=MagicMock(return_value=False))
    @patch('tasks.notes.try_git_command', new=MagicMock(return_value=True))
    @patch('tasks.notes.GithubAPI')
    def test_local_pushes_with_git(self, github_mock, generic_mock, create_pr_mock):
        ctx = self._context()

        notes.update_changelog(ctx, '7.86.x')

        commands = [c.args[0] for c in ctx.run.call_args_list]
        self.assertIn('git push --set-upstream origin changelog-update-7.86.0', commands)
        github_mock.assert_not_called()
        create_pr_mock.assert_called_once()
