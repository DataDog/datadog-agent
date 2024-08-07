import subprocess
import unittest
from datetime import datetime, timezone
from unittest.mock import MagicMock, patch

import yaml
from invoke import MockContext, Result
from invoke.exceptions import Exit

from tasks import pipeline


class TestVerifyWorkspace(unittest.TestCase):
    @patch('tasks.pipeline.GithubAPI', autospec=True)
    @patch('tasks.pipeline.check_clean_branch_state', new=MagicMock())
    def test_with_branch(self, mock_gh):
        branch_test_name = "tryphon_tournesol"
        context_mock = MockContext(run=Result("haddock"))
        branch = pipeline.verify_workspace(context_mock, branch_test_name)
        self.assertEqual(branch_test_name, branch)
        mock_gh.assert_not_called()

    @patch('tasks.pipeline.GithubAPI', autospec=True)
    @patch('tasks.pipeline.check_clean_branch_state', new=MagicMock())
    def test_without_branch(self, mock_gh):
        context_mock = MockContext(run=Result("haddock"))
        branch = pipeline.verify_workspace(context_mock, None)
        self.assertEqual("haddock/test_buildimages", branch)
        mock_gh.assert_called()

    @patch('tasks.pipeline.GithubAPI', autospec=True)
    def test_bad_workspace(self, _):
        with open(".gitignore", "a") as f:
            f.write("# test comment")
        with self.assertRaises(Exit):
            context_mock = MockContext(run=Result("haddock"))
            _ = pipeline.verify_workspace(context_mock)
        subprocess.run("git checkout -- .gitignore".split())


class TestUpdateGitlabCI(unittest.TestCase):
    gitlabci_file = "tasks/unit_tests/testdata/fake_gitlab-ci.yml"
    erroneous_file = "tasks/unit_tests/testdata/erroneous_gitlab-ci.yml"

    def tearDown(self) -> None:
        subprocess.run(f"git checkout -- {self.gitlabci_file} {self.erroneous_file}".split())
        return super().tearDown()

    def test_nominal(self):
        pipeline.update_gitlab_config(self.gitlabci_file, "1mageV3rsi0n", test_version=True)
        with open(self.gitlabci_file) as gl:
            gitlab_ci = yaml.safe_load(gl)
        for variable, value in gitlab_ci["variables"].items():
            # TEST_INFRA_DEFINITION_BUILDIMAGE label format differs from other buildimages
            if variable.endswith("_SUFFIX") and not variable.startswith("TEST_INFRA_DEFINITION"):
                self.assertEqual("_test_only", value)

    def test_update_no_test(self):
        pipeline.update_gitlab_config(self.gitlabci_file, "1mageV3rsi0n", test_version=False)
        with open(self.gitlabci_file) as gl:
            gitlab_ci = yaml.safe_load(gl)
        for variable, value in gitlab_ci["variables"].items():
            if variable.endswith("_SUFFIX"):
                self.assertEqual("", value)

    def test_raise(self):
        with self.assertRaises(RuntimeError):
            pipeline.update_gitlab_config(self.erroneous_file, "1mageV3rsi0n", test_version=False)


class TestUpdateCircleCI(unittest.TestCase):
    circleci_file = "tasks/unit_tests/testdata/fake_circleci_config.yml"
    erroneous_file = "tasks/unit_tests/testdata/erroneous_circleci_config.yml"

    def tearDown(self) -> None:
        subprocess.run(f"git checkout -- {self.circleci_file} {self.erroneous_file}".split())
        return super().tearDown()

    def test_nominal(self):
        pipeline.update_circleci_config(self.circleci_file, "1m4g3", test_version=True)
        with open(self.circleci_file) as gl:
            circle_ci = yaml.safe_load(gl)
        full_image = circle_ci['templates']['job_template']['docker'][0]['image']
        image, version = full_image.split(":")
        self.assertTrue(image.endswith("_test_only"))
        self.assertEqual("1m4g3", version)

    def test_update_no_test(self):
        pipeline.update_circleci_config(self.circleci_file, "1m4g3", test_version=False)
        with open(self.circleci_file) as gl:
            circle_ci = yaml.safe_load(gl)
        full_image = circle_ci['templates']['job_template']['docker'][0]['image']
        image, version = full_image.split(":")
        self.assertFalse(image.endswith("_test_only"))
        self.assertEqual("1m4g3", version)

    def test_raise(self):
        with self.assertRaises(RuntimeError):
            pipeline.update_circleci_config(self.erroneous_file, "1m4g3", test_version=False)


class TestCompareToItself(unittest.TestCase):
    context = MockContext(
        run={
            "git checkout -b compare/Football/900284400": Result(),
            "git remote set-url origin https://x-access-token:zidane@github.com/DataDog/datadog-agent.git": Result(),
            "git config --global user.name 'github-actions[bot]'": Result(),
            "git config --global user.email 'github-app[bot]@users.noreply.github.com'": Result(),
            "git commit -m 'Compare to itself' --allow-empty": Result(),
            "git push origin compare/Football/900284400": Result(),
            "git commit -am 'Compare to itself'": Result(),
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
    @patch('tasks.pipeline.datetime')
    @patch('tasks.pipeline.GithubAPI')
    @patch('tasks.pipeline.get_gitlab_repo')
    def test_nominal(self, repo_mock, gh_mock, dt_mock):
        dt_mock.now.return_value = self.now
        gh_mock.return_value = self.gh
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
    @patch('tasks.pipeline.datetime')
    @patch('tasks.pipeline.GithubAPI')
    @patch('tasks.pipeline.get_gitlab_repo')
    def test_no_pipeline_found(self, repo_mock, gh_mock, dt_mock):
        dt_mock.now.return_value = self.now
        gh_mock.return_value = self.gh
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
    @patch('tasks.pipeline.datetime')
    @patch('tasks.pipeline.GithubAPI')
    @patch('tasks.pipeline.get_gitlab_repo')
    def test_no_pipeline_found_again(self, repo_mock, gh_mock, dt_mock):
        dt_mock.now.return_value = self.now
        gh_mock.return_value = self.gh
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
    @patch('tasks.pipeline.datetime')
    @patch('tasks.pipeline.GithubAPI')
    @patch('tasks.pipeline.get_gitlab_repo')
    def test_pipeline_with_no_jobs(self, repo_mock, gh_mock, dt_mock):
        dt_mock.now.return_value = self.now
        gh_mock.return_value = self.gh
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
