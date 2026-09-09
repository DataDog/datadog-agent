import unittest
from datetime import datetime, timezone
from unittest.mock import MagicMock, patch

from gitlab.exceptions import GitlabGetError
from invoke import Exit, MockContext, Result

from tasks import pipeline


class TestCompareToItself(unittest.TestCase):
    context = MockContext(
        run={
            "git checkout -b compare/Football/900284400": Result(),
            "git remote set-url origin https://x-access-token:zidane@github.com/DataDog/datadog-agent.git": Result(),
            "git config --global user.name 'github-actions[bot]'": Result(),
            "git config --global user.email 'github-app[bot]@users.noreply.github.com'": Result(),
            "git commit -m 'Initial push of the compare/to branch' --allow-empty": Result(),
            "git push origin compare/Football/900284400": Result(),
            "git commit -am 'Commit to compare to itself'": Result(),
            "git checkout Football": Result(),
            "git branch -D compare/Football/900284400": Result(),
            "git push origin :compare/Football/900284400": Result(),
        }
    )
    now = datetime(1998, 7, 12, 23, 0, 0, tzinfo=timezone.utc)

    @staticmethod
    def side(x):
        if x == "c0mm1t":
            return MagicMock(author_name=pipeline.BOT_NAME, title="Commit to compare to itself")
        return MagicMock(author_name="Aimee Jaquet")

    def setUp(self) -> None:
        self.gh = MagicMock()
        self.gh._auth.token = "zidane"
        self.commits = MagicMock()
        self.commits.get.side_effect = self.side

    @patch('tasks.pipeline.gitlab_configuration_is_modified', new=MagicMock(return_value=True))
    @patch('builtins.open', new=MagicMock())
    @patch.dict('os.environ', {"CI_COMMIT_REF_NAME": "Football"})
    @patch('tasks.pipeline.time', new=MagicMock())
    @patch('tasks.libs.releasing.json.load_release_json')
    @patch('tasks.pipeline.datetime')
    @patch('tasks.pipeline.GithubAPI')
    @patch('tasks.pipeline.get_gitlab_repo')
    def test_nominal(self, repo_mock, gh_mock, dt_mock, release_mock):
        dt_mock.now.return_value = self.now
        gh_mock.return_value = self.gh
        release_mock.return_value = {"base_branch": "main"}
        created_pipeline = MagicMock()
        created_pipeline.jobs.list.return_value = [1, 2, 3]
        agent = MagicMock()
        agent.pipelines.create.return_value = created_pipeline
        agent.commits = self.commits
        repo_mock.return_value = agent
        pipeline.compare_to_itself(self.context)
        self.assertEqual(1, agent.pipelines.list.call_count)

    @patch('tasks.pipeline.gitlab_configuration_is_modified', new=MagicMock(return_value=True))
    @patch('builtins.open', new=MagicMock())
    @patch.dict('os.environ', {"CI_COMMIT_REF_NAME": "Football"})
    @patch('tasks.pipeline.time', new=MagicMock())
    @patch('tasks.libs.releasing.json.load_release_json')
    @patch('tasks.pipeline.datetime')
    @patch('tasks.pipeline.GithubAPI')
    @patch('tasks.pipeline.get_gitlab_repo')
    def test_branch_not_mirrored_yet(self, repo_mock, gh_mock, dt_mock, release_mock):
        dt_mock.now.return_value = self.now
        gh_mock.return_value = self.gh
        release_mock.return_value = {"base_branch": "main"}
        created_pipeline = MagicMock()
        created_pipeline.jobs.list.return_value = [1, 2, 3]
        agent = MagicMock()
        agent.pipelines.create.return_value = created_pipeline
        agent.commits = self.commits
        # The branch hasn't mirrored to GitLab yet on the first two checks (404),
        # then shows up on the third — the loop must keep retrying instead of
        # crashing on the first GitlabGetError.
        agent.branches.get.side_effect = [
            GitlabGetError(response_code=404),
            GitlabGetError(response_code=404),
            MagicMock(),
        ]
        repo_mock.return_value = agent
        pipeline.compare_to_itself(self.context)
        self.assertEqual(3, agent.branches.get.call_count)
        self.assertEqual(1, agent.pipelines.list.call_count)

    @patch('tasks.pipeline.gitlab_configuration_is_modified', new=MagicMock(return_value=True))
    @patch('builtins.open', new=MagicMock())
    @patch.dict('os.environ', {"CI_COMMIT_REF_NAME": "Football"})
    @patch('tasks.pipeline.time', new=MagicMock())
    @patch('tasks.libs.releasing.json.load_release_json')
    @patch('tasks.pipeline.datetime')
    @patch('tasks.pipeline.GithubAPI')
    @patch('tasks.pipeline.get_gitlab_repo')
    def test_branch_never_mirrors(self, repo_mock, gh_mock, dt_mock, release_mock):
        dt_mock.now.return_value = self.now
        gh_mock.return_value = self.gh
        release_mock.return_value = {"base_branch": "main"}
        agent = MagicMock()
        agent.commits = self.commits
        # The branch 404s on every single attempt (e.g. the mirror is stuck) — the
        # retry loop must exhaust all attempts and raise a clear RuntimeError
        # instead of hanging forever or propagating the last GitlabGetError.
        agent.branches.get.side_effect = GitlabGetError(response_code=404)
        repo_mock.return_value = agent
        with self.assertRaises(RuntimeError):
            pipeline.compare_to_itself(self.context)
        self.assertEqual(18, agent.branches.get.call_count)
        agent.pipelines.create.assert_not_called()

    @patch('tasks.pipeline.gitlab_configuration_is_modified', new=MagicMock(return_value=True))
    @patch('builtins.open', new=MagicMock())
    @patch.dict('os.environ', {"CI_COMMIT_REF_NAME": "Football"})
    @patch('tasks.pipeline.time', new=MagicMock())
    @patch('tasks.libs.releasing.json.load_release_json')
    @patch('tasks.pipeline.datetime')
    @patch('tasks.pipeline.GithubAPI')
    @patch('tasks.pipeline.get_gitlab_repo')
    def test_branch_get_non_404_error_propagates(self, repo_mock, gh_mock, dt_mock, release_mock):
        dt_mock.now.return_value = self.now
        gh_mock.return_value = self.gh
        release_mock.return_value = {"base_branch": "main"}
        agent = MagicMock()
        agent.commits = self.commits
        # A non-404 error (e.g. bad credentials) must fail fast instead of
        # being mistaken for mirror latency and retried for ~3 minutes.
        agent.branches.get.side_effect = GitlabGetError(response_code=401)
        repo_mock.return_value = agent
        with self.assertRaises(GitlabGetError):
            pipeline.compare_to_itself(self.context)
        self.assertEqual(1, agent.branches.get.call_count)

    @patch('tasks.pipeline.gitlab_configuration_is_modified', new=MagicMock(return_value=True))
    @patch('builtins.open', new=MagicMock())
    @patch.dict('os.environ', {"CI_COMMIT_REF_NAME": "Football"})
    @patch('tasks.pipeline.time', new=MagicMock())
    @patch('tasks.libs.releasing.json.load_release_json')
    @patch('tasks.pipeline.datetime')
    @patch('tasks.pipeline.GithubAPI')
    @patch('tasks.pipeline.get_gitlab_repo')
    def test_no_branch_found(self, repo_mock, gh_mock, dt_mock, release_mock):
        dt_mock.now.return_value = self.now
        gh_mock.return_value = self.gh
        release_mock.return_value = {"base_branch": "main"}
        agent = MagicMock()
        agent.branches.get.return_value = None
        agent.commits = self.commits
        repo_mock.return_value = agent
        with self.assertRaises(RuntimeError):
            pipeline.compare_to_itself(self.context)

    @patch('tasks.pipeline.gitlab_configuration_is_modified', new=MagicMock(return_value=True))
    @patch('builtins.open', new=MagicMock())
    @patch.dict('os.environ', {"CI_COMMIT_REF_NAME": "Football"})
    @patch('tasks.pipeline.time', new=MagicMock())
    @patch('tasks.libs.releasing.json.load_release_json')
    @patch('tasks.pipeline.datetime')
    @patch('tasks.pipeline.GithubAPI')
    @patch('tasks.pipeline.get_gitlab_repo')
    def test_cannot_trigger_pipeline(self, repo_mock, gh_mock, dt_mock, release_mock):
        dt_mock.now.return_value = self.now
        gh_mock.return_value = self.gh
        release_mock.return_value = {"base_branch": "main"}
        agent = MagicMock()
        agent.pipelines.create.side_effect = RuntimeError("Cannot trigger the pipeline")
        agent.commits = self.commits
        repo_mock.return_value = agent
        with self.assertRaises(RuntimeError):
            pipeline.compare_to_itself(self.context)

    @patch('tasks.pipeline.gitlab_configuration_is_modified', new=MagicMock(return_value=True))
    @patch('builtins.open', new=MagicMock())
    @patch.dict('os.environ', {"CI_COMMIT_REF_NAME": "Football"})
    @patch('tasks.pipeline.time', new=MagicMock())
    @patch('tasks.libs.releasing.json.load_release_json')
    @patch('tasks.pipeline.datetime')
    @patch('tasks.pipeline.GithubAPI')
    @patch('tasks.pipeline.get_gitlab_repo')
    def test_pipeline_with_no_jobs(self, repo_mock, gh_mock, dt_mock, release_mock):
        dt_mock.now.return_value = self.now
        gh_mock.return_value = self.gh
        release_mock.return_value = {"base_branch": "main"}
        created_pipeline = MagicMock()
        created_pipeline.jobs.list.return_value = []
        agent = MagicMock()
        agent.pipelines.create.return_value = created_pipeline
        agent.commits = self.commits
        repo_mock.return_value = agent
        with self.assertRaises(Exit):
            pipeline.compare_to_itself(self.context)

    @patch('tasks.pipeline.gitlab_configuration_is_modified', new=MagicMock(return_value=True))
    @patch('builtins.open', new=MagicMock())
    @patch.dict('os.environ', {"CI_COMMIT_REF_NAME": "compare/Football"})
    @patch('tasks.pipeline.time', new=MagicMock())
    @patch('tasks.pipeline.datetime')
    @patch('tasks.pipeline.GithubAPI')
    @patch('tasks.pipeline.get_gitlab_repo')
    def test_prevent_loop(self, repo_mock, gh_mock, dt_mock):
        dt_mock.now.return_value = self.now
        gh_mock.return_value = self.gh
        created_pipeline = MagicMock()
        created_pipeline.jobs.list.return_value = [1, 2, 3]
        agent = MagicMock()
        agent.pipelines.create.return_value = created_pipeline
        agent.commits = self.commits
        repo_mock.return_value = agent
        pipeline.compare_to_itself(self.context)
        agent.pipelines.create.assert_not_called()
