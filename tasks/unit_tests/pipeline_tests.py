import unittest
from datetime import datetime, timezone
from unittest.mock import MagicMock, patch

from invoke import MockContext, Result
from invoke.exceptions import Exit

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
            return MagicMock(author_name=pipeline.BOT_NAME)
        else:
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
        pipelines = MagicMock()
        compare_to = MagicMock(sha="c0mm1t")
        compare_to.jobs.list.return_value = [1, 2, 3]
        pipelines.list.side_effect = [[], [], [compare_to], [], [], []]
        agent = MagicMock()
        agent.pipelines = pipelines
        agent.commits = self.commits
        repo_mock.return_value = agent
        pipeline.compare_to_itself(self.context)
        self.assertEqual(3, agent.pipelines.list.call_count)

    @patch('tasks.pipeline.gitlab_configuration_is_modified', new=MagicMock(return_value=True))
    @patch('builtins.open', new=MagicMock())
    @patch.dict('os.environ', {"CI_COMMIT_REF_NAME": "Football"})
    @patch('tasks.pipeline.time', new=MagicMock())
    @patch('tasks.libs.releasing.json.load_release_json')
    @patch('tasks.pipeline.datetime')
    @patch('tasks.pipeline.GithubAPI')
    @patch('tasks.pipeline.get_gitlab_repo')
    def test_no_pipeline_found(self, repo_mock, gh_mock, dt_mock, release_mock):
        dt_mock.now.return_value = self.now
        gh_mock.return_value = self.gh
        release_mock.return_value = {"base_branch": "main"}
        pipelines = MagicMock()
        pipelines.list.side_effect = [[], [], [], [], [], []]
        agent = MagicMock()
        agent.pipelines = pipelines
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
    def test_no_pipeline_found_again(self, repo_mock, gh_mock, dt_mock, release_mock):
        dt_mock.now.return_value = self.now
        gh_mock.return_value = self.gh
        release_mock.return_value = {"base_branch": "main"}
        pipelines = MagicMock()
        compare_to = MagicMock(sha="w4lo0")
        compare_to.jobs.list.return_value = [1, 2, 3]
        pipelines.list.side_effect = [[], [], [compare_to], [], [], []]
        agent = MagicMock()
        agent.pipelines = pipelines
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
        pipelines = MagicMock()
        compare_to = MagicMock(sha="c0mm1t")
        pipelines.list.side_effect = [[], [], [compare_to], [], [], []]
        agent = MagicMock()
        agent.pipelines = pipelines
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
        pipelines = MagicMock()
        compare_to = MagicMock(sha="c0mm1t")
        pipelines.list.side_effect = [[], [], [compare_to], [], [], []]
        agent = MagicMock()
        agent.pipelines = pipelines
        agent.commits = self.commits
        repo_mock.return_value = agent
        pipeline.compare_to_itself(self.context)
        agent.pipelines.list.assert_not_called()
