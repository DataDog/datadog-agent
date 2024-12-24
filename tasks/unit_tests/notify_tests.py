from __future__ import annotations

import json
import unittest
from unittest.mock import MagicMock, patch

from codeowners import CodeOwners
from gitlab.v4.objects import ProjectJob
from invoke import MockContext

from tasks import notify
from tasks.libs.notify import pipeline_status
from tasks.libs.pipeline.notifications import find_job_owners, load_and_validate
from tasks.libs.types.types import FailedJobReason, FailedJobs, FailedJobType


def get_fake_jobs() -> list[ProjectJob]:
    with open("tasks/unit_tests/testdata/jobs.json") as f:
        jobs = json.load(f)

    return [ProjectJob(MagicMock(), attrs=job) for job in jobs]


def get_github_slack_map():
    return load_and_validate(
        "tasks/unit_tests/testdata/github_slack_map.yaml",
        "DEFAULT_SLACK_CHANNEL",
        '#channel-everything',
        relpath=False,
    )


class TestSendMessage(unittest.TestCase):
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    @patch('tasks.libs.pipeline.notifications.get_pr_from_commit', new=MagicMock(return_value=""))
    def test_merge(self, api_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        repo_mock.jobs.get.return_value.artifact.return_value = b"{}"
        repo_mock.jobs.get.return_value.trace.return_value = b"Log trace"
        repo_mock.pipelines.get.return_value.ref = "test"
        list_mock = repo_mock.pipelines.get.return_value.jobs.list
        list_mock.side_effect = [get_fake_jobs(), []]
        notify.send_message(MockContext(), notification_type="merge", dry_run=True)
        list_mock.assert_called()

    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    @patch('tasks.libs.notify.pipeline_status.get_failed_jobs')
    @patch('tasks.libs.pipeline.notifications.get_pr_from_commit', new=MagicMock(return_value=""))
    def test_merge_without_get_failed_call(self, get_failed_jobs_mock, api_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        repo_mock.jobs.get.return_value.artifact.return_value = b"{}"
        repo_mock.jobs.get.return_value.trace.return_value = b"Log trace"
        repo_mock.pipelines.get.return_value.ref = "test"

        failed = FailedJobs()
        failed.add_failed_job(
            ProjectJob(
                MagicMock(),
                attrs={
                    "id": 1,
                    "name": "job1",
                    "stage": "stage1",
                    "retry_summary": [],
                    "web_url": "http://www.job.com",
                    "failure_type": FailedJobType.INFRA_FAILURE,
                    "failure_reason": FailedJobReason.EC2_SPOT,
                    "allow_failure": False,
                },
            )
        )
        failed.add_failed_job(
            ProjectJob(
                MagicMock(),
                attrs={
                    "id": 2,
                    "name": "job2",
                    "stage": "stage2",
                    "retry_summary": [],
                    "web_url": "http://www.job.com",
                    "failure_type": FailedJobType.INFRA_FAILURE,
                    "failure_reason": FailedJobReason.E2E_INFRA_FAILURE,
                    "allow_failure": True,
                },
            )
        )
        failed.add_failed_job(
            ProjectJob(
                MagicMock(),
                attrs={
                    "id": 3,
                    "name": "job3",
                    "stage": "stage3",
                    "retry_summary": [],
                    "web_url": "http://www.job.com",
                    "failure_type": FailedJobType.JOB_FAILURE,
                    "failure_reason": FailedJobReason.FAILED_JOB_SCRIPT,
                    "allow_failure": False,
                },
            )
        )
        failed.add_failed_job(
            ProjectJob(
                MagicMock(),
                attrs={
                    "id": 4,
                    "name": "job4",
                    "stage": "stage4",
                    "retry_summary": [],
                    "web_url": "http://www.job.com",
                    "failure_type": FailedJobType.JOB_FAILURE,
                    "failure_reason": FailedJobReason.FAILED_JOB_SCRIPT,
                    "allow_failure": True,
                },
            )
        )
        get_failed_jobs_mock.return_value = failed
        notify.send_message(MockContext(), notification_type="merge", dry_run=True)

        get_failed_jobs_mock.assert_called()

    @patch("tasks.libs.owners.parsing.read_owners")
    def test_route_e2e_internal_error(self, read_owners_mock):
        failed = FailedJobs()
        failed.add_failed_job(
            ProjectJob(
                MagicMock(),
                attrs={
                    "name": "job1",
                    "stage": "stage1",
                    "retry_summary": [],
                    "web_url": "http://www.job.com",
                    "failure_type": FailedJobType.INFRA_FAILURE,
                    "failure_reason": FailedJobReason.EC2_SPOT,
                    "allow_failure": False,
                },
            )
        )
        failed.add_failed_job(
            ProjectJob(
                MagicMock(),
                attrs={
                    "name": "job2",
                    "stage": "stage2",
                    "retry_summary": [],
                    "web_url": "http://www.job.com",
                    "failure_type": FailedJobType.INFRA_FAILURE,
                    "failure_reason": FailedJobReason.E2E_INFRA_FAILURE,
                    "allow_failure": False,
                },
            )
        )
        failed.add_failed_job(
            ProjectJob(
                MagicMock(),
                attrs={
                    "name": "job3",
                    "stage": "stage3",
                    "retry_summary": [],
                    "web_url": "http://www.job.com",
                    "failure_type": FailedJobType.JOB_FAILURE,
                    "failure_reason": FailedJobReason.FAILED_JOB_SCRIPT,
                    "allow_failure": False,
                },
            )
        )
        failed.add_failed_job(
            ProjectJob(
                MagicMock(),
                attrs={
                    "name": "job4",
                    "stage": "stage4",
                    "retry_summary": [],
                    "web_url": "http://www.job.com",
                    "failure_type": FailedJobType.JOB_FAILURE,
                    "failure_reason": FailedJobReason.FAILED_JOB_SCRIPT,
                    "allow_failure": True,
                },
            )
        )
        jobowners = """\
        job1 @DataDog/agent-devx-infra
        job2 @DataDog/agent-devx-infra
        job3 @DataDog/agent-devx-infra @DataDog/agent-devx-loops
        not* @DataDog/agent-delivery
        """
        read_owners_mock.return_value = CodeOwners(jobowners)
        owners = find_job_owners(failed)
        # Should send notifications to agent-e2e-testing and ci-experience
        self.assertIn("@DataDog/agent-e2e-testing", owners)
        self.assertIn("@DataDog/agent-devx-infra", owners)
        self.assertNotIn("@DataDog/agent-devx-loops", owners)
        self.assertNotIn("@DataDog/agent-delivery", owners)

    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    @patch('tasks.libs.pipeline.notifications.get_pr_from_commit', new=MagicMock(return_value=""))
    def test_merge_with_get_failed_call(self, api_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        trace_mock = repo_mock.jobs.get.return_value.trace
        list_mock = repo_mock.pipelines.get.return_value.jobs.list

        trace_mock.return_value = b"no basic auth credentials"
        list_mock.return_value = get_fake_jobs()
        repo_mock.jobs.get.return_value.artifact.return_value = b"{}"
        repo_mock.pipelines.get.return_value.ref = "test"

        notify.send_message(MockContext(), notification_type="merge", dry_run=True)

        trace_mock.assert_called()
        list_mock.assert_called()

    def test_post_to_channel1(self):
        self.assertFalse(pipeline_status.should_send_message_to_author("main", default_branch="main"))

    def test_post_to_channel2(self):
        self.assertFalse(pipeline_status.should_send_message_to_author("7.52.x", default_branch="main"))

    def test_post_to_channel3(self):
        self.assertFalse(pipeline_status.should_send_message_to_author("7.52.0", default_branch="main"))

    def test_post_to_channel4(self):
        self.assertFalse(pipeline_status.should_send_message_to_author("7.52.0-rc.1", default_branch="main"))

    def test_post_to_author1(self):
        self.assertTrue(
            pipeline_status.should_send_message_to_author("7.52.0-beta-test-feature", default_branch="main")
        )

    def test_post_to_author2(self):
        self.assertTrue(
            pipeline_status.should_send_message_to_author("7.52.0-rc.1-beta-test-feature", default_branch="main")
        )

    def test_post_to_author3(self):
        self.assertTrue(pipeline_status.should_send_message_to_author("celian/7.52.0", default_branch="main"))

    def test_post_to_author4(self):
        self.assertTrue(pipeline_status.should_send_message_to_author("a.b.c", default_branch="main"))

    def test_post_to_author5(self):
        self.assertTrue(pipeline_status.should_send_message_to_author("my-feature", default_branch="main"))


class TestSendStats(unittest.TestCase):
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    @patch("tasks.libs.notify.alerts.create_count", new=MagicMock())
    def test_nominal(self, api_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        trace_mock = repo_mock.jobs.get.return_value.trace
        pipeline_mock = repo_mock.pipelines.get

        trace_mock.return_value = b"E2E INTERNAL ERROR"
        attrs = {"jobs.list.return_value": get_fake_jobs(), "created_at": "2024-03-12T10:00:00.000Z"}
        pipeline_mock.return_value = MagicMock(**attrs)

        notify.send_stats(MockContext(), dry_run=True)

        trace_mock.assert_called()
        pipeline_mock.assert_called()
        self.assertEqual(pipeline_mock.call_count, 2)


class TestJobOwners(unittest.TestCase):
    def test_partition(self):
        from tasks.owners import make_partition

        jobs = [
            'tests_team_a_42',
            'tests_team_a_618',
            'this_is_a_test',
            'tests_team_b_1',
            'tests_letters_0',
            'hello_world',
        ]

        partition = make_partition(jobs, "tasks/unit_tests/testdata/jobowners.txt")
        partition = sorted(partition.items())

        self.assertEqual(
            partition,
            [
                ('@DataDog/team-a', {'tests_team_a_42', 'tests_team_a_618', 'tests_letters_0'}),
                ('@DataDog/team-b', {'tests_team_b_1', 'tests_letters_0'}),
                ('@DataDog/team-everything', {'this_is_a_test', 'hello_world'}),
            ],
        )
