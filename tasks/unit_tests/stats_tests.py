import unittest
from unittest.mock import MagicMock, patch

from gitlab.v4.objects import ProjectJob

from tasks.libs.pipeline.stats import get_max_duration


class TestGetMaxDuration(unittest.TestCase):
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    def test_required_success(self, api_mock):
        job_list = [
            {
                "name": "go_mod_tidy_check",
                "finished_at": "2024-03-12T10:10:00.000Z",
                "status": "success",
            },
            {
                "name": "tests_deb-x64-py3",
                "finished_at": "2024-03-12T10:20:00.000Z",
                "status": "success",
            },
            {
                "name": "tests_rpm-x64-py3",
                "finished_at": "2024-03-12T10:30:00.000Z",
                "status": "success",
            },
        ]
        # We must use a ProjectJob (and not a simple Mock) because we cannot override a MagicMock().name attribute
        pipeline = {
            "jobs.list.return_value": [ProjectJob(MagicMock(), attrs=job) for job in job_list],
            "created_at": "2024-03-12T10:00:00.000Z",
        }
        repo_mock = api_mock.return_value.projects.get.return_value
        pipeline_mock = repo_mock.pipelines.get
        pipeline_mock.return_value = MagicMock(**pipeline)
        max_duration, status = get_max_duration("datadog-agent")
        self.assertEqual(max_duration, 1800)
        self.assertEqual(status, "success")

    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    def test_required_success_reversed_max(self, api_mock):
        job_list = [
            {
                "name": "go_mod_tidy_check",
                "finished_at": "2024-03-12T10:40:00.000Z",
                "status": "success",
            },
            {
                "name": "tests_deb-x64-py3",
                "finished_at": "2024-03-12T10:20:00.000Z",
                "status": "success",
            },
            {
                "name": "tests_rpm-x64-py3",
                "finished_at": "2024-03-12T10:10:00.000Z",
                "status": "success",
            },
        ]
        # We must use a ProjectJob (and not a simple Mock) because we cannot override a MagicMock().name attribute
        pipeline = {
            "jobs.list.return_value": [ProjectJob(MagicMock(), attrs=job) for job in job_list],
            "created_at": "2024-03-12T10:00:00.000Z",
        }
        repo_mock = api_mock.return_value.projects.get.return_value
        pipeline_mock = repo_mock.pipelines.get
        pipeline_mock.return_value = MagicMock(**pipeline)
        max_duration, status = get_max_duration("datadog-agent")
        self.assertEqual(max_duration, 2400)
        self.assertEqual(status, "success")

    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    def test_required_failed(self, api_mock):
        job_list = [
            {
                "name": "go_mod_tidy_check",
                "finished_at": "2024-03-12T10:10:00.000Z",
                "status": "success",
            },
            {
                "name": "tests_deb-x64-py3",
                "finished_at": "2024-03-12T10:20:00.000Z",
                "status": "failed",
            },
            {
                "name": "tests_rpm-x64-py3",
                "finished_at": "2024-03-12T10:30:00.000Z",
                "status": "success",
            },
        ]
        # We must use a ProjectJob (and not a simple Mock) because we cannot override a MagicMock().name attribute
        pipeline = {
            "jobs.list.return_value": [ProjectJob(MagicMock(), attrs=job) for job in job_list],
            "created_at": "2024-03-12T10:00:00.000Z",
        }
        repo_mock = api_mock.return_value.projects.get.return_value
        pipeline_mock = repo_mock.pipelines.get
        pipeline_mock.return_value = MagicMock(**pipeline)
        _, status = get_max_duration("datadog-agent")
        self.assertEqual(status, "failed")

    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    def test_required_skipped(self, api_mock):
        job_list = [
            {
                "name": "go_mod_tidy_check",
                "finished_at": "2024-03-12T10:10:00.000Z",
                "status": "success",
            },
            {
                "name": "tests_deb-x64-py3",
                "finished_at": "2024-03-12T10:20:00.000Z",
                "status": "failed",
            },
            {
                "name": "tests_rpm-x64-py3",
                "finished_at": "2024-03-12T10:30:00.000Z",
                "status": "skipped",
            },
        ]
        # We must use a ProjectJob (and not a simple Mock) because we cannot override a MagicMock().name attribute
        pipeline = {
            "jobs.list.return_value": [ProjectJob(MagicMock(), attrs=job) for job in job_list],
            "created_at": "2024-03-12T10:00:00.000Z",
        }
        repo_mock = api_mock.return_value.projects.get.return_value
        pipeline_mock = repo_mock.pipelines.get
        pipeline_mock.return_value = MagicMock(**pipeline)
        _, status = get_max_duration("datadog-agent")
        self.assertEqual(status, "skipped")

    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    def test_no_required(self, api_mock):
        job_list = [
            {
                "name": "un",
                "finished_at": "2024-03-12T10:10:00.000Z",
                "status": "success",
            },
            {
                "name": "dos",
                "finished_at": "2024-03-12T10:20:00.000Z",
                "status": "failed",
            },
            {
                "name": "tres",
                "finished_at": "2024-03-12T10:30:00.000Z",
                "status": "success",
            },
        ]
        # We must use a ProjectJob (and not a simple Mock) because we cannot override a MagicMock().name attribute
        pipeline = {
            "jobs.list.return_value": [ProjectJob(MagicMock(), attrs=job) for job in job_list],
            "created_at": "2024-03-12T10:00:00.000Z",
        }
        repo_mock = api_mock.return_value.projects.get.return_value
        pipeline_mock = repo_mock.pipelines.get
        pipeline_mock.return_value = MagicMock(**pipeline)
        max_duration, status = get_max_duration("datadog-agent")
        self.assertEqual(max_duration, 0)
        self.assertEqual(status, "success")

    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    def test_job_with_no_finished_at(self, api_mock):
        job_list = [
            {
                "name": "go_mod_tidy_check",
                "finished_at": "2024-03-12T10:10:00.000Z",
                "status": "success",
            },
            {
                "name": "tests_deb-x64-py3",
                "finished_at": "2024-03-12T10:20:00.000Z",
                "status": "failed",
            },
            {
                "name": "tests_rpm-x64-py3",
                "finished_at": None,
                "status": "skipped",
            },
        ]
        # We must use a ProjectJob (and not a simple Mock) because we cannot override a MagicMock().name attribute
        pipeline = {
            "jobs.list.return_value": [ProjectJob(MagicMock(), attrs=job) for job in job_list],
            "created_at": "2024-03-12T10:00:00.000Z",
        }
        repo_mock = api_mock.return_value.projects.get.return_value
        pipeline_mock = repo_mock.pipelines.get
        pipeline_mock.return_value = MagicMock(**pipeline)
        max_duration, status = get_max_duration("datadog-agent")
        self.assertEqual(max_duration, 1200)
        self.assertEqual(status, "failed")
