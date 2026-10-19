import unittest
from unittest.mock import MagicMock, patch

from tasks.libs.ciproviders.github_api import GithubAPI


class GithubAPITestCase(unittest.TestCase):
    def setUp(self):
        # Mocked GitHub clients must not request real local credentials.
        auth = patch("tasks.libs.ciproviders.github_api.generate_local_github_token", return_value="test-token")
        auth.start()
        self.addCleanup(auth.stop)


class Label:
    def __init__(self, name):
        self.name = name


class TestSignedCommit(unittest.TestCase):
    def setUp(self):
        self.github = GithubAPI.__new__(GithubAPI)
        self.github._repository = MagicMock(full_name="DataDog/datadog-agent")
        self.github._auth = MagicMock(token="test-token")

    def test_graphql_passes_variables_separately(self):
        with patch("tasks.libs.ciproviders.github_api.requests.post") as post:
            post.return_value.status_code = 200
            post.return_value.json.return_value = {"data": {}}
            self.github.graphql("query($name: String!) { field }", {"name": 'a"b'})
        self.assertEqual(
            post.call_args.kwargs["json"],
            {"query": "query($name: String!) { field }", "variables": {"name": 'a"b'}},
        )

    def test_graphql_preserves_existing_query_only_calls(self):
        with patch("tasks.libs.ciproviders.github_api.requests.post") as post:
            post.return_value.status_code = 200
            self.github.graphql("query { field }")
        self.assertEqual(post.call_args.kwargs["json"], {"query": "query { field }"})

    def test_signed_commit_uses_atomic_expected_head(self):
        commit = {"oid": "commit", "tree": {"oid": "tree"}}
        with patch.object(
            self.github, "graphql", return_value={"data": {"createCommitOnBranch": {"commit": commit}}}
        ) as graphql:
            self.assertEqual(self.github.create_signed_commit("branch", "head", "title", "body", {}), commit)
        self.assertEqual(
            graphql.call_args.kwargs["variables"]["input"],
            {
                "branch": {"repositoryNameWithOwner": "DataDog/datadog-agent", "branchName": "branch"},
                "expectedHeadOid": "head",
                "message": {"headline": "title", "body": "body"},
                "fileChanges": {},
            },
        )

    def test_graphql_errors_are_failures_even_on_http_success(self):
        with (
            patch.object(self.github, "graphql", return_value={"errors": [{"message": "Head changed"}]}),
            self.assertRaisesRegex(RuntimeError, "Head changed"),
        ):
            self.github.create_signed_commit("branch", "head", "title", "body", {})

    def test_missing_commit_or_tree_is_not_success(self):
        for result in ({"data": None}, {"data": {"createCommitOnBranch": {"commit": {"oid": "commit"}}}}):
            with (
                self.subTest(result=result),
                patch.object(self.github, "graphql", return_value=result),
                self.assertRaisesRegex(RuntimeError, "missing the commit or tree"),
            ):
                self.github.create_signed_commit("branch", "head", "title", "body", {})


class Myfile:
    def __init__(self, name):
        self.filename = name


class TestReleaseNoteNeeded(GithubAPITestCase):
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


class TestContainsReleaseNote(GithubAPITestCase):
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


class TestUpdateReviewComplexityLabel(GithubAPITestCase):
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


class TestFindAllTeams(GithubAPITestCase):
    @patch("tasks.libs.ciproviders.github_api.Github", autospec=True)
    def test_nested_teams(self, _):
        github = GithubAPI(repository="test", public_repo=True)
        with patch.object(github, '_repository') as mock_repo:
            team_a = MagicMock()
            team_b = MagicMock()
            team_c = MagicMock()
            team_d = MagicMock()
            team_a.get_teams.return_value = [team_b, team_c]
            team_b.get_teams.return_value = [team_d]
            mock_repo.get_teams.return_value = [team_a]
            self.assertCountEqual(github.find_teams(mock_repo), [team_a, team_b, team_c, team_d])

    @patch("tasks.libs.ciproviders.github_api.Github", autospec=True)
    def test_ignore_dev_apm(self, _):
        github = GithubAPI(repository="test", public_repo=True)
        with patch.object(github, '_repository') as mock_repo:
            team_a = MagicMock()
            team_b = MagicMock()
            team_c = MagicMock()
            team_d = MagicMock()
            team_e = MagicMock()
            team_f = MagicMock()
            team_a.get_teams.return_value = [team_b, team_c]
            team_b.get_teams.return_value = [team_d]
            team_a.name = "Dev"
            team_e.name = "apm"
            mock_repo.get_teams.return_value = [team_a, team_e, team_f]
            self.assertEqual(github.find_teams(mock_repo, exclude_teams=['Dev', 'apm']), [team_f])

    @patch("tasks.libs.ciproviders.github_api.Github", autospec=True)
    def test_ignore_permissions(self, _):
        github = GithubAPI(repository="test", public_repo=True)
        with patch.object(github, '_repository') as mock_repo:
            team_a = MagicMock()
            team_b = MagicMock()
            team_c = MagicMock()
            team_d = MagicMock()
            team_e = MagicMock()
            team_f = MagicMock()
            team_a.get_teams.return_value = [team_b, team_c]
            team_b.get_teams.return_value = [team_d]
            team_f.permission = "pull"
            team_e.permission = "triage"
            mock_repo.get_teams.return_value = [team_a, team_e, team_f]
            self.assertCountEqual(
                github.find_teams(mock_repo, exclude_permissions=['pull', 'triage']),
                [team_a, team_b, team_c, team_d],
            )

    @patch("tasks.libs.ciproviders.github_api.Github", autospec=True)
    def test_empty_team(self, _):
        github = GithubAPI(repository="test", public_repo=True)
        with patch.object(github, '_repository') as mock_repo:
            self.assertListEqual(github.find_teams(mock_repo), [])
