import unittest
from unittest.mock import MagicMock, patch

from gitlab.v4.objects import ProjectJob

from tasks.libs.pipeline import notifications
from tasks.libs.pipeline.data import get_job_failure_context, get_jobs_skipped_on_pr
from tasks.libs.types.types import FailedJobReason, FailedJobType


class TestLoadAndValidate(unittest.TestCase):
    def test_files_loaded_correctly(self):
        # Assert that a couple of expected entries are there, including one that uses DEFAULT_JIRA_PROJECT
        self.assertEqual(notifications.GITHUB_JIRA_MAP['@datadog/agent-all'], "AGNTR")
        self.assertEqual(notifications.GITHUB_JIRA_MAP['@datadog/agent-devx-infra'], "ACIX")

        # Assert that a couple of expected entries are there, including one that uses DEFAULT_SLACK_PROJECT
        self.assertEqual(notifications.GITHUB_SLACK_MAP['@datadog/agent-all'], "#datadog-agent-pipelines")
        self.assertEqual(notifications.GITHUB_SLACK_MAP['@datadog/agent-devx-infra'], "#agent-devx-ops")


class TestFailedJobs(unittest.TestCase):
    def test_infra_failure(self):
        job = ProjectJob(
            MagicMock(),
            attrs={
                "name": "test",
                "id": 618,
                "stage": "test",
                "status": "failed",
                "tag_list": [],
                "allow_failure": False,
                "web_url": "https://sometest.test",
                "retry_summary": ["failed"],
                "failure_reason": "runner_system_failure",
            },
        )
        log = 'Empty log'

        fail_type, _fail_reason = get_job_failure_context(job, log)

        self.assertEqual(fail_type, FailedJobType.INFRA_FAILURE)

    def test_infra_failure_log(self):
        job = ProjectJob(
            MagicMock(),
            attrs={
                "name": "test",
                "id": 618,
                "stage": "test",
                "status": "failed",
                "tag_list": [],
                "allow_failure": False,
                "web_url": "https://sometest.test",
                "retry_summary": ["failed"],
                "failure_reason": "script_failure",
            },
        )
        log = 'Some test...\nE2E INTERNAL ERROR\n...\n'

        fail_type, fail_reason = get_job_failure_context(job, log)

        self.assertEqual(fail_type, FailedJobType.INFRA_FAILURE)
        self.assertEqual(fail_reason, FailedJobReason.E2E_INFRA_FAILURE)

    def test_non_infra_failure_log(self):
        job = ProjectJob(
            MagicMock(),
            attrs={
                "name": "test",
                "id": 618,
                "stage": "test",
                "status": "failed",
                "tag_list": [],
                "allow_failure": False,
                "web_url": "https://sometest.test",
                "retry_summary": ["failed"],
                "failure_reason": "script_failure",
            },
        )
        log = '...E\nTraceback...\n'

        fail_type, _fail_reason = get_job_failure_context(job, log)

        self.assertEqual(fail_type, FailedJobType.JOB_FAILURE)


class TestGetSkippedJobsOnPr(unittest.TestCase):
    @patch('tasks.libs.pipeline.data.get_gitlab_repo')
    @patch('tasks.libs.pipeline.data.GithubAPI', autospec=True)
    def test_nothing_skipped(self, gh_mock, gl_mock):
        gh, commit, pr, agent, pipeline, pr_job, job, failed_jobs = 8 * [MagicMock()]
        pr.head.ref = 'branch'
        commit.get_pulls.return_value = [pr]
        gh._repository.get_commit.return_value = commit
        pr_job.name = 'bakery'
        pr_job.status = 'success'
        job.full_name = 'bakery'
        pipeline.jobs.list.return_value = [job]
        pipeline.bridges.list.return_value = []
        pipeline.web_url = 'https://pipeline.url'
        agent.pipelines.list.return_value = [pipeline]
        gl_mock.return_value = agent
        gh_mock.return_value = gh
        failed_jobs.all_mandatory_failures.return_value = [job]
        skipped, url = get_jobs_skipped_on_pr(MagicMock(), failed_jobs)
        self.assertEqual(skipped, [])
        self.assertEqual(url, 'https://pipeline.url')

    @patch('tasks.libs.pipeline.data.get_gitlab_repo')
    @patch('tasks.libs.pipeline.data.GithubAPI', autospec=True)
    def test_job_skipped(self, gh_mock, gl_mock):
        gh, commit, pr, agent, pipeline, pr_job, job, failed_jobs = 8 * [MagicMock()]
        pr.head.ref = 'branch'
        commit.get_pulls.return_value = [pr]
        gh._repository.get_commit.return_value = commit
        pr_job.name = 'bakery'
        pr_job.status = 'skipped'
        job.name = 'bake'
        job.full_name = 'bakery'
        pipeline.jobs.list.return_value = [job]
        pipeline.bridges.list.return_value = []
        pipeline.web_url = 'https://pipeline.url'
        agent.pipelines.list.return_value = [pipeline]
        gl_mock.return_value = agent
        gh_mock.return_value = gh
        failed_jobs.all_mandatory_failures.return_value = [job]
        skipped, url = get_jobs_skipped_on_pr(MagicMock(), failed_jobs)
        self.assertEqual(skipped, ['bake'])
        self.assertEqual(url, 'https://pipeline.url')

    @patch('tasks.libs.pipeline.data.get_gitlab_repo')
    @patch('tasks.libs.pipeline.data.GithubAPI', autospec=True)
    def test_not_generated(self, gh_mock, gl_mock):
        gh, commit, pr, agent, pipeline, pr_job, job, failed_jobs = 8 * [MagicMock()]
        pr.head.ref = 'branch'
        commit.get_pulls.return_value = [pr]
        gh._repository.get_commit.return_value = commit
        pr_job.name = 'bakery [gluten free]'
        pr_job.status = 'success'
        job.full_name = 'bakery [pastry]'
        job.name = 'bakery'
        pipeline.jobs.list.return_value = [job]
        pipeline.bridges.list.return_value = []
        pipeline.web_url = 'https://pipeline.url'
        agent.pipelines.list.return_value = [pipeline]
        gl_mock.return_value = agent
        gh_mock.return_value = gh
        failed_jobs.all_mandatory_failures.return_value = [job]
        skipped, url = get_jobs_skipped_on_pr(MagicMock(), failed_jobs)
        self.assertEqual(skipped, ['bakery'])
        self.assertEqual(url, 'https://pipeline.url')

    @patch('tasks.libs.pipeline.data.get_gitlab_repo')
    @patch('tasks.libs.pipeline.data.GithubAPI', autospec=True)
    def test_no_pr(self, gh_mock, gl_mock):
        gh, commit, pr, agent, pipeline, pr_job, job, failed_jobs = 8 * [MagicMock()]
        pr.head.ref = 'branch'
        commit.get_pulls.return_value = []
        gh._repository.get_commit.return_value = commit
        pr_job.name = 'bakery'
        pr_job.status = 'skipped'
        job.full_name = 'bakery'
        pipeline.jobs.list.return_value = []
        pipeline.bridges.list.return_value = []
        pipeline.web_url = 'https://pipeline.url'
        agent.pipelines.list.return_value = [pipeline]
        gl_mock.return_value = agent
        gh_mock.return_value = gh
        failed_jobs.all_mandatory_failures.return_value = [job]
        skipped, url = get_jobs_skipped_on_pr(MagicMock(), failed_jobs)
        self.assertEqual(skipped, [])
        self.assertEqual(url, '')

    @patch('builtins.print')
    @patch('tasks.libs.pipeline.data.get_gitlab_repo')
    @patch('tasks.libs.pipeline.data.GithubAPI', autospec=True)
    def test_too_many_pr(self, gh_mock, gl_mock, print_mock):
        gh, commit, pr, agent, pipeline, pr_job, job, failed_jobs = 8 * [MagicMock()]
        pr.head.ref = 'branch'
        commit.get_pulls.return_value = [pr, pr]
        gh._repository.get_commit.return_value = commit
        pr_job.name = 'bakery'
        pr_job.status = 'skipped'
        job.full_name = 'bakery'
        pipeline.jobs.list.return_value = []
        pipeline.bridges.list.return_value = []
        pipeline.web_url = 'https://pipeline.url'
        agent.pipelines.list.return_value = [pipeline]
        gl_mock.return_value = agent
        gh_mock.return_value = gh
        failed_jobs.all_mandatory_failures.return_value = [job]
        skipped, url = get_jobs_skipped_on_pr(MagicMock(), failed_jobs)
        self.assertEqual(skipped, [])
        self.assertEqual(url, '')
        print_mock.assert_called_once()

    @patch('tasks.libs.pipeline.data.get_gitlab_repo')
    @patch('tasks.libs.pipeline.data.GithubAPI', autospec=True)
    def test_no_pipeline(self, gh_mock, gl_mock):
        gh, commit, pr, agent, pipeline, pr_job, job, failed_jobs = 8 * [MagicMock()]
        pr.head.ref = 'branch'
        commit.get_pulls.return_value = [pr]
        gh._repository.get_commit.return_value = commit
        pr_job.name = 'bakery'
        pr_job.status = 'success'
        job.full_name = 'bakery'
        pipeline.jobs.list.return_value = [job]
        pipeline.bridges.list.return_value = []
        pipeline.web_url = 'https://pipeline.url'
        agent.pipelines.list.return_value = []
        gl_mock.return_value = agent
        gh_mock.return_value = gh
        failed_jobs.all_mandatory_failures.return_value = [job]
        skipped, url = get_jobs_skipped_on_pr(MagicMock(), failed_jobs)
        self.assertEqual(skipped, [])
        self.assertEqual(url, '')

    @patch('tasks.libs.pipeline.data.get_gitlab_repo')
    @patch('tasks.libs.pipeline.data.GithubAPI', autospec=True)
    def test_no_failure(self, gh_mock, gl_mock):
        gh, commit, pr, agent, pipeline, pr_job, job, failed_jobs = 8 * [MagicMock()]
        pr.head.ref = 'branch'
        commit.get_pulls.return_value = [pr]
        gh._repository.get_commit.return_value = commit
        pr_job.name = 'bakery'
        pr_job.status = 'skipped'
        job.full_name = 'bakery'
        pipeline.jobs.list.return_value = [job]
        pipeline.bridges.list.return_value = []
        pipeline.web_url = 'https://pipeline.url'
        agent.pipelines.list.return_value = [pipeline]
        gl_mock.return_value = agent
        gh_mock.return_value = gh
        failed_jobs.all_mandatory_failures.return_value = []
        skipped, url = get_jobs_skipped_on_pr(MagicMock(), failed_jobs)
        self.assertEqual(skipped, [])
        self.assertEqual(url, 'https://pipeline.url')
