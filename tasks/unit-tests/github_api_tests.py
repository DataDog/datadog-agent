import unittest
from unittest.mock import patch

from tasks.libs.ciproviders.github_api import GithubAPI


class Label:
    def __init__(self, name):
        self.name = name


class Myfile:
    def __init__(self, name):
        self.filename = name


class TestReleaseNoteNeeded(unittest.TestCase):
    @patch("tasks.libs.ciproviders.github_api.Github", autospec=True)
    def test_needed(self, _):
        github = GithubAPI(repository="test", public_repo=True)
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_labels.return_value = [Label("changeling/no-changelog")]
            self.assertTrue(github.is_release_note_needed(1))

    @patch("tasks.libs.ciproviders.github_api.Github", autospec=True)
    def test_not_needed(self, _):
        github = GithubAPI(repository="test", public_repo=True)
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_labels.return_value = [Label("changelog/no-changelog")]
            self.assertFalse(github.is_release_note_needed(2))


class TestContainsReleaseNote(unittest.TestCase):
    @patch("tasks.libs.ciproviders.github_api.Github", autospec=True)
    def test_contains(self, _):
        github = GithubAPI(repository="test", public_repo=True)
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_files.return_value = [Myfile("releasenotes/notes/do/re/mi")]
            self.assertTrue(github.contains_release_note(1))

    @patch("tasks.libs.ciproviders.github_api.Github", autospec=True)
    def test_do_not_contain(self, _):
        github = GithubAPI(repository="test", public_repo=True)
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_files.return_value = [Myfile("release_notes/notes")]
            self.assertFalse(github.contains_release_note(2))
