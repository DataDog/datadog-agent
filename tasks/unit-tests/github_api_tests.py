import unittest
from unittest.mock import patch

from tasks.libs.common.github_api import GithubAPI


class Label:
    def __init__(self, name):
        self.name = name


class Myfile:
    def __init__(self, name):
        self.filename = name


class TestTeamAssignmentLabel(unittest.TestCase):
    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_skip_qa(self, _):
        github = GithubAPI(repository="test")
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_labels.return_value = [Label('qa/skip-qa'), Label("team/dream")]
            self.assertEqual(github.get_team_assignment_labels(51), (False, []))

    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_without_teams(self, _):
        github = GithubAPI(repository="test")
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_labels.return_value = [Label('de_cadix_a_des_yeux_de_velours'), Label("team/triage")]
            self.assertEqual(github.get_team_assignment_labels(69), (True, []))

    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_with_teams(self, _):
        github = GithubAPI(repository="test")
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_labels.return_value = [Label("qa/lity-streets"), Label("team/yavbou")]
            self.assertEqual(github.get_team_assignment_labels(51), (True, ["team/yavbou"]))


class TestQASkip(unittest.TestCase):
    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_skip_qa_alone(self, _):
        github = GithubAPI(repository="test")
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_labels.return_value = [Label("qa/skip-qa")]
            self.assertFalse(github.is_qa_skip_ok(666))

    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_bad_validation(self, _):
        github = GithubAPI(repository="test")
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_labels.return_value = [Label("qa/skip-qa"), Label("qa/lity-streets")]
            self.assertFalse(github.is_qa_skip_ok(1337))

    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_ok(self, _):
        github = GithubAPI(repository="test")
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_labels.return_value = [Label("qa/skip-qa"), Label("qa/done")]
            self.assertTrue(github.is_qa_skip_ok(1515))

    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_validation_alone(self, _):
        github = GithubAPI(repository="test")
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_labels.return_value = [Label("qa/done")]
            self.assertTrue(github.is_qa_skip_ok(1664))


class TestPRMilestone(unittest.TestCase):
    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_with_milestone(self, _):
        github = GithubAPI(repository="test")
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.milestone.title = "emerald"
            self.assertEqual(github.get_pr_milestone(42), "emerald")

    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_without_milestone(self, _):
        github = GithubAPI(repository="test")
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.milestone = None
            self.assertIsNone(github.get_pr_milestone(0))


class TestReleaseNoteNeeded(unittest.TestCase):
    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_needed(self, _):
        github = GithubAPI(repository="test")
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_labels.return_value = [Label("changeling/no-changelog")]
            self.assertTrue(github.is_release_note_needed(1))

    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_not_needed(self, _):
        github = GithubAPI(repository="test")
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_labels.return_value = [Label("changelog/no-changelog")]
            self.assertFalse(github.is_release_note_needed(2))


class TestContainsReleaseNote(unittest.TestCase):
    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_contains(self, _):
        github = GithubAPI(repository="test")
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_files.return_value = [Myfile("releasenotes/notes/do/re/mi")]
            self.assertTrue(github.contains_release_note(1))

    @patch("tasks.libs.common.github_api.Github", autospec=True)
    def test_do_not_contain(self, _):
        github = GithubAPI(repository="test")
        with patch.object(github, '_repository') as mock_repo:
            mock_pr = mock_repo.get_pull.return_value
            mock_pr.get_files.return_value = [Myfile("release_notes/notes")]
            self.assertFalse(github.contains_release_note(2))


if __name__ == "__main__":
    unittest.main()
