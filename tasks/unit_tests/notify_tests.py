from __future__ import annotations

import json
import sys
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
    @patch('tasks.libs.pipeline.notifications.get_pr_from_commit', new=MagicMock(return_value=""))
    @patch('tasks.libs.notify.pipeline_status.get_jobs_skipped_on_pr', new=MagicMock(return_value=([], "")))
    @patch('slack_sdk.WebClient', new=MagicMock())
    @patch('builtins.print')
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    def test_merge(self, api_mock, print_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        repo_mock.jobs.get.return_value.artifact.return_value = b"{}"
        repo_mock.jobs.get.return_value.trace.return_value = b"Log trace"
        repo_mock.pipelines.get.return_value.ref = "test"
        repo_mock.pipelines.get.return_value.source = "push"
        repo_mock.pipelines.get.return_value.started_at = "2025-02-07T05:59:48.396Z"
        repo_mock.pipelines.get.return_value.finished_at = "2025-02-07T06:59:48.396Z"
        list_mock = repo_mock.pipelines.get.return_value.jobs.list
        list_mock.side_effect = [get_fake_jobs(), []]
        with patch.dict('os.environ', {'SLACK_DATADOG_AGENT_BOT_TOKEN': 'coin'}, clear=True):
            notify.send_message(MockContext(), "42", dry_run=True)
        list_mock.assert_called()
        repo_mock.pipelines.get.assert_called_with("42")
        self.assertTrue("merge" in print_mock.mock_calls[0].args[0])
        repo_mock.jobs.get.assert_called()

    @patch('tasks.libs.pipeline.notifications.get_pr_from_commit', new=MagicMock(return_value=""))
    @patch('tasks.libs.notify.pipeline_status.get_jobs_skipped_on_pr', new=MagicMock(return_value=([], "")))
    @patch('slack_sdk.WebClient', new=MagicMock())
    @patch('builtins.print')
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    def test_merge_ddci(self, api_mock, print_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        repo_mock.jobs.get.return_value.artifact.return_value = b"{}"
        repo_mock.jobs.get.return_value.trace.return_value = b"Log trace"
        repo_mock.pipelines.get.return_value.ref = "test"
        repo_mock.pipelines.get.return_value.source = "api"
        repo_mock.pipelines.get.return_value.started_at = "2025-02-07T05:59:48.396Z"
        repo_mock.pipelines.get.return_value.finished_at = "2025-02-07T06:59:48.396Z"
        list_mock = repo_mock.pipelines.get.return_value.jobs.list
        list_mock.side_effect = [get_fake_jobs(), []]
        with patch.dict('os.environ', {'SLACK_DATADOG_AGENT_BOT_TOKEN': 'coin'}, clear=True):
            notify.send_message(MockContext(), "42", dry_run=True)
        list_mock.assert_called()
        repo_mock.pipelines.get.assert_called_with("42")
        self.assertTrue("merge" in print_mock.mock_calls[0].args[0])
        repo_mock.jobs.get.assert_called()

    @patch('tasks.libs.pipeline.notifications.get_pr_from_commit', new=MagicMock(return_value=""))
    @patch('tasks.libs.notify.pipeline_status.get_jobs_skipped_on_pr', new=MagicMock(return_value=([], "")))
    @patch('slack_sdk.WebClient', new=MagicMock())
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    @patch('tasks.libs.notify.pipeline_status.get_failed_jobs')
    @patch('builtins.print')
    def test_merge_without_get_failed_call(self, print_mock, get_failed_jobs_mock, api_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        repo_mock.jobs.get.return_value.artifact.return_value = b"{}"
        repo_mock.jobs.get.return_value.trace.return_value = b"Log trace"
        repo_mock.pipelines.get.return_value.ref = "test"
        repo_mock.pipelines.get.return_value.source = "push"
        repo_mock.pipelines.get.return_value.started_at = "2025-02-07T05:59:48.396Z"
        repo_mock.pipelines.get.return_value.finished_at = "2025-02-07T06:59:48.396Z"

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
        with patch.dict('os.environ', {'SLACK_DATADOG_AGENT_BOT_TOKEN': 'meuh'}, clear=True):
            notify.send_message(MockContext(), "42", dry_run=True)
        self.assertTrue("merge" in print_mock.mock_calls[0].args[0])
        get_failed_jobs_mock.assert_called()
        repo_mock.jobs.get.assert_called()

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

    @patch('tasks.libs.pipeline.notifications.get_pr_from_commit', new=MagicMock(return_value=""))
    @patch('tasks.libs.notify.pipeline_status.get_jobs_skipped_on_pr', new=MagicMock(return_value=([], "")))
    @patch('slack_sdk.WebClient', new=MagicMock())
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    @patch('builtins.print')
    def test_merge_with_get_failed_call(self, print_mock, api_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        trace_mock = repo_mock.jobs.get.return_value.trace
        list_mock = repo_mock.pipelines.get.return_value.jobs.list

        trace_mock.return_value = b"no basic auth credentials"
        list_mock.return_value = get_fake_jobs()
        repo_mock.jobs.get.return_value.artifact.return_value = b"{}"
        repo_mock.pipelines.get.return_value.ref = "test"
        repo_mock.pipelines.get.return_value.source = "push"
        repo_mock.pipelines.get.return_value.started_at = "2025-02-07T05:59:48.396Z"
        repo_mock.pipelines.get.return_value.finished_at = None

        with patch.dict('os.environ', {'SLACK_DATADOG_AGENT_BOT_TOKEN': 'ouaf'}, clear=True):
            notify.send_message(MockContext(), "42", dry_run=True)
        self.assertTrue("merge" in print_mock.mock_calls[0].args[0])
        trace_mock.assert_called()
        list_mock.assert_called()
        repo_mock.jobs.get.assert_called()

    @patch('tasks.libs.pipeline.notifications.get_pr_from_commit', new=MagicMock(return_value=""))
    @patch('tasks.libs.notify.pipeline_status.get_jobs_skipped_on_pr', new=MagicMock(return_value=([], "")))
    @patch('slack_sdk.WebClient', new=MagicMock())
    @patch.dict('os.environ', {'DEPLOY_AGENT': 'true', 'SLACK_DATADOG_AGENT_BOT_TOKEN': 'hihan'})
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    @patch('builtins.print')
    def test_deploy_with_get_failed_call(self, print_mock, api_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        trace_mock = repo_mock.jobs.get.return_value.trace
        list_mock = repo_mock.pipelines.get.return_value.jobs.list

        trace_mock.return_value = b"no basic auth credentials"
        list_mock.return_value = get_fake_jobs()
        repo_mock.jobs.get.return_value.artifact.return_value = b"{}"
        repo_mock.pipelines.get.return_value.ref = "test"
        repo_mock.pipelines.get.return_value.source = "push"
        repo_mock.pipelines.get.return_value.started_at = "2025-02-07T05:59:48.396Z"
        repo_mock.pipelines.get.return_value.finished_at = "2025-02-07T08:21:48.396Z"

        notify.send_message(MockContext(), "42", dry_run=True)
        self.assertTrue("rocket" in print_mock.mock_calls[0].args[0])
        self.assertTrue("[:hourglass: 142 min]" in print_mock.mock_calls[0].args[0])
        trace_mock.assert_called()
        list_mock.assert_called()
        repo_mock.jobs.get.assert_called()

    @patch('tasks.libs.pipeline.notifications.get_pr_from_commit', new=MagicMock(return_value=""))
    @patch('tasks.libs.notify.pipeline_status.get_jobs_skipped_on_pr', new=MagicMock(return_value=([], "")))
    @patch('slack_sdk.WebClient', new=MagicMock())
    @patch.dict('os.environ', {'SLACK_DATADOG_AGENT_BOT_TOKEN': 'miaou', 'TRIGGERED_PIPELINE': 'true'}, clear=True)
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    @patch('builtins.print')
    def test_trigger_with_get_failed_call(self, print_mock, api_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        trace_mock = repo_mock.jobs.get.return_value.trace
        list_mock = repo_mock.pipelines.get.return_value.jobs.list

        trace_mock.return_value = b"no basic auth credentials"
        list_mock.return_value = get_fake_jobs()
        repo_mock.jobs.get.return_value.artifact.return_value = b"{}"
        repo_mock.pipelines.get.return_value.ref = "test"
        repo_mock.pipelines.get.return_value.source = "api"
        repo_mock.pipelines.get.return_value.started_at = "2025-02-07T05:59:48.396Z"
        repo_mock.pipelines.get.return_value.finished_at = "2025-02-07T06:59:48.396Z"

        notify.send_message(MockContext(), "42", dry_run=True)
        self.assertTrue("arrow_forward" in print_mock.mock_calls[0].args[0])
        self.assertTrue("[:hourglass: 60 min]" in print_mock.mock_calls[0].args[0])
        trace_mock.assert_called()
        list_mock.assert_called()
        repo_mock.jobs.get.assert_called()

    @patch('tasks.libs.pipeline.notifications.get_pr_from_commit', new=MagicMock(return_value=""))
    @patch('tasks.libs.notify.pipeline_status.get_jobs_skipped_on_pr', new=MagicMock(return_value=([], "")))
    @patch('slack_sdk.WebClient', new=MagicMock())
    @patch.dict(
        'os.environ',
        {'DDR': 'true', 'DDR_WORKFLOW_ID': '1337', 'DEPLOY_AGENT': 'false', 'SLACK_DATADOG_AGENT_BOT_TOKEN': 'ni'},
    )
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    @patch('builtins.print')
    def test_trigger_with_get_failed_call_conductor(self, print_mock, api_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        trace_mock = repo_mock.jobs.get.return_value.trace
        list_mock = repo_mock.pipelines.get.return_value.jobs.list

        trace_mock.return_value = b"no basic auth credentials"
        list_mock.return_value = get_fake_jobs()
        repo_mock.jobs.get.return_value.artifact.return_value = b"{}"
        repo_mock.pipelines.get.return_value.ref = "test"
        repo_mock.pipelines.get.return_value.source = "pipeline"
        repo_mock.pipelines.get.return_value.started_at = "2025-02-07T05:59:48.396Z"
        repo_mock.pipelines.get.return_value.finished_at = "2025-02-07T06:41:48.396Z"

        notify.send_message(MockContext(), "42", dry_run=True)
        self.assertTrue("arrow_forward" in print_mock.mock_calls[0].args[0])
        self.assertTrue("[:hourglass: 42 min]" in print_mock.mock_calls[0].args[0])
        trace_mock.assert_called()
        list_mock.assert_called()
        repo_mock.jobs.get.assert_called()

    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    @patch('builtins.print')
    def test_dismiss_notification(self, print_mock, api_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        repo_mock.pipelines.get.return_value.source = "pipeline"

        with patch.dict('os.environ', {}, clear=True):
            notify.send_message(MockContext(), "42", dry_run=True)
        repo_mock.jobs.get.assert_not_called()
        print_mock.assert_called_with("This pipeline is a non-conductor downstream pipeline, skipping notifications")

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
        self.assertEqual(pipeline_mock.call_count, 1)


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


class TestFailureSummarySendNotifications(unittest.TestCase):
    @patch.dict('os.environ', {'CI_PIPELINE_CREATED_AT': '2025-02-07T11:11:11.111Z'})
    @patch('builtins.print')
    def test_ignore_non_scheduled_conductor(self, print_mock):
        notify.failure_summary_send_notifications(MockContext(), daily_summary=True)
        print_mock.assert_called_with(
            "Failure summary notifications are only sent during the conductor scheduled pipeline, skipping",
            file=sys.stderr,
        )

    @patch.dict('os.environ', {'CI_PIPELINE_CREATED_AT': '2025-02-07T06:11:11.111Z'})
    @patch('tasks.notify.failure_summary', new=MagicMock())
    @patch('builtins.print')
    def test_sheduled_conductor(self, print_mock):
        notify.failure_summary_send_notifications(MockContext(), daily_summary=True)
        print_mock.assert_not_called()

    @patch.dict('os.environ', {'CI_PIPELINE_CREATED_AT': '2025-02-07T05:22:22.222Z'})
    @patch('tasks.notify.failure_summary', new=MagicMock())
    @patch('builtins.print')
    def test_sheduled_conductor_dst(self, print_mock):
        notify.failure_summary_send_notifications(MockContext(), daily_summary=True)
        print_mock.assert_not_called()
