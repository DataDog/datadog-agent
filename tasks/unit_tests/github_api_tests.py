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

    @patch("tasks.libs.ciproviders.github_api.Github", autospec=True)
    def test_org(self, _):
        github = GithubAPI(repository="test", public_repo=True)
        self.assertIsNone(github._organization)

        github = GithubAPI(repository="org/test", public_repo=True)
        self.assertEqual(github._organization, "org")


class TestUpdateReviewComplexityLabel(unittest.TestCase):
    @patch("tasks.libs.ciproviders.github_api.Github", autospec=True)
    def test_add_label(self, _):
        github = GithubAPI(repository="test", public_repo=True)
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_labels.return_value = [Label("changelog/no-changelog")]
            github.update_review_complexity_labels(1, "short review")
            mock_pr.remove_from_labels.assert_not_called()
            mock_pr.add_to_labels.assert_called_with("short review")

    @patch("tasks.libs.ciproviders.github_api.Github", autospec=True)
    def test_add_same_label(self, _):
        github = GithubAPI(repository="test", public_repo=True)
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_labels.return_value = [Label("changelog/no-changelog"), Label("short review")]
            github.update_review_complexity_labels(1, "short review")
            mock_pr.remove_from_labels.assert_not_called()
            mock_pr.add_to_labels.assert_not_called()

    @patch("tasks.libs.ciproviders.github_api.Github", autospec=True)
    def test_add_new_label(self, _):
        github = GithubAPI(repository="test", public_repo=True)
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_labels.return_value = [Label("changelog/no-changelog"), Label("short review")]
            github.update_review_complexity_labels(1, "medium review")
            mock_pr.remove_from_labels.assert_called_with("short review")
            mock_pr.add_to_labels.assert_called_with("medium review")
