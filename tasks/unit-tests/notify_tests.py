from __future__ import annotations

import json
import os
import pathlib
import unittest
from unittest.mock import MagicMock, patch

from codeowners import CodeOwners
from gitlab.v4.objects import ProjectJob
from invoke import MockContext, Result
from invoke.exceptions import UnexpectedExit

from tasks import notify
from tasks.libs.pipeline.notifications import find_job_owners
from tasks.libs.types.types import FailedJobReason, FailedJobs, FailedJobType


def get_fake_jobs() -> list[ProjectJob]:
    with open("tasks/unit-tests/testdata/jobs.json") as f:
        jobs = json.load(f)

    return [ProjectJob(MagicMock(), attrs=job) for job in jobs]


class TestSendMessage(unittest.TestCase):
    @patch("builtins.print", new=MagicMock())
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    def test_merge(self, api_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        repo_mock.jobs.get.return_value.trace.return_value = b"Log trace"
        list_mock = repo_mock.pipelines.get.return_value.jobs.list
        list_mock.side_effect = [get_fake_jobs(), []]
        notify.send_message(MockContext(), notification_type="merge", print_to_stdout=True)
        list_mock.assert_called()

    @patch("builtins.print", new=MagicMock())
    @patch("tasks.notify.get_failed_jobs")
    def test_merge_without_get_failed_call(self, get_failed_jobs_mock):
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
                    "allow_failure": True,
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
        get_failed_jobs_mock.return_value = failed
        notify.send_message(MockContext(), notification_type="merge", print_to_stdout=True)

        get_failed_jobs_mock.assert_called()

    @patch("builtins.print", new=MagicMock())
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
        job1 @DataDog/agent-ci-experience
        job2 @DataDog/agent-ci-experience
        job3 @DataDog/agent-ci-experience @DataDog/agent-developer-tools
        not* @DataDog/agent-build-and-releases
        """
        read_owners_mock.return_value = CodeOwners(jobowners)
        owners = find_job_owners(failed)
        # Should send notifications to agent-e2e-testing and ci-experience
        self.assertIn("@DataDog/agent-e2e-testing", owners)
        self.assertIn("@DataDog/agent-ci-experience", owners)
        self.assertNotIn("@DataDog/agent-developer-tools", owners)
        self.assertNotIn("@DataDog/agent-build-and-releases", owners)

    @patch("builtins.print", new=MagicMock())
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    def test_merge_with_get_failed_call(self, api_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        trace_mock = repo_mock.jobs.get.return_value.trace
        list_mock = repo_mock.pipelines.get.return_value.jobs.list

        trace_mock.return_value = b"no basic auth credentials"
        list_mock.return_value = get_fake_jobs()

        notify.send_message(MockContext(), notification_type="merge", print_to_stdout=True)

        trace_mock.assert_called()
        list_mock.assert_called()

    def test_post_to_channel1(self):
        self.assertTrue(notify._should_send_message_to_channel("main", default_branch="main"))

    def test_post_to_channel2(self):
        self.assertTrue(notify._should_send_message_to_channel("7.52.x", default_branch="main"))

    def test_post_to_channel3(self):
        self.assertTrue(notify._should_send_message_to_channel("7.52.0", default_branch="main"))

    def test_post_to_channel4(self):
        self.assertTrue(notify._should_send_message_to_channel("7.52.0-rc.1", default_branch="main"))

    def test_post_to_author1(self):
        self.assertFalse(notify._should_send_message_to_channel("7.52.0-beta-test-feature", default_branch="main"))

    def test_post_to_author2(self):
        self.assertFalse(notify._should_send_message_to_channel("7.52.0-rc.1-beta-test-feature", default_branch="main"))

    def test_post_to_author3(self):
        self.assertFalse(notify._should_send_message_to_channel("celian/7.52.0", default_branch="main"))

    def test_post_to_author4(self):
        self.assertFalse(notify._should_send_message_to_channel("a.b.c", default_branch="main"))

    def test_post_to_author5(self):
        self.assertFalse(notify._should_send_message_to_channel("my-feature", default_branch="main"))


class TestSendStats(unittest.TestCase):
    @patch("builtins.print", new=MagicMock())
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    @patch("tasks.notify.create_count", new=MagicMock())
    def test_nominal(self, api_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        trace_mock = repo_mock.jobs.get.return_value.trace
        pipeline_mock = repo_mock.pipelines.get

        trace_mock.return_value = b"E2E INTERNAL ERROR"
        attrs = {"jobs.list.return_value": get_fake_jobs(), "created_at": "2024-03-12T10:00:00.000Z"}
        pipeline_mock.return_value = MagicMock(**attrs)

        notify.send_stats(MockContext(), print_to_stdout=True)

        trace_mock.assert_called()
        pipeline_mock.assert_called()
        self.assertEqual(pipeline_mock.call_count, 2)


class TestCheckConsistentFailures(unittest.TestCase):
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    def test_nominal(self, api_mock):
        os.environ["CI_PIPELINE_ID"] = "456"

        repo_mock = api_mock.return_value.projects.get.return_value
        trace_mock = repo_mock.jobs.get.return_value.trace
        list_mock = repo_mock.pipelines.get.return_value.jobs.list

        trace_mock.return_value = b"net/http: TLS handshake timeout"
        list_mock.return_value = get_fake_jobs()

        notify.check_consistent_failures(
            MockContext(run=Result("test")),
            "tasks/unit-tests/testdata/job_executions.json",
        )

        trace_mock.assert_called()
        list_mock.assert_called()


class TestRetrieveJobExecutionsCreated(unittest.TestCase):
    job_executions = None
    job_file = "job_executions.json"

    def setUp(self) -> None:
        self.job_executions = notify.create_initial_job_executions(self.job_file)

    def tearDown(self) -> None:
        pathlib.Path(self.job_file).unlink(missing_ok=True)

    def test_retrieved(self):
        ctx = MockContext(run=Result("test"))
        j = notify.retrieve_job_executions(ctx, "job_executions.json")
        self.assertEqual(j, self.job_executions)


class TestRetrieveJobExecutions(unittest.TestCase):
    test_json = "tasks/unit-tests/testdata/job_executions.json"

    def test_not_found(self):
        ctx = MagicMock()
        ctx.run.side_effect = UnexpectedExit(Result(stderr="This is a 404 not found"))
        j = notify.retrieve_job_executions(ctx, self.test_json)
        self.assertEqual(j, {"pipeline_id": 0, "jobs": {}})

    def test_other_error(self):
        ctx = MagicMock()
        ctx.run.side_effect = UnexpectedExit(Result(stderr="This is another error"))
        with self.assertRaises(UnexpectedExit):
            notify.retrieve_job_executions(ctx, self.test_json)


class TestUpdateStatistics(unittest.TestCase):
    @patch("tasks.notify.get_failed_jobs")
    def test_nominal(self, mock_get_failed):
        failed_jobs = mock_get_failed.return_value
        failed_jobs.all_failures.return_value = [
            ProjectJob(MagicMock(), attrs=a) for a in [{"name": "nifnif"}, {"name": "nafnaf"}]
        ]
        j = {
            "jobs": {
                "nafnaf": {
                    "consecutive_failures": 2,
                    "cumulative_failures": [0, 0, 0, 0, 0, 0, 0, 0, 1, 1],
                },
                "noufnouf": {
                    "consecutive_failures": 2,
                    "cumulative_failures": [1, 0, 1, 1],
                },
            }
        }
        a, j = notify.update_statistics(j)
        self.assertEqual(j["jobs"]["nifnif"]["consecutive_failures"], 1)
        self.assertEqual(j["jobs"]["nifnif"]["cumulative_failures"], [1])
        self.assertEqual(j["jobs"]["nafnaf"]["consecutive_failures"], 3)
        self.assertEqual(j["jobs"]["nafnaf"]["cumulative_failures"], [0, 0, 0, 0, 0, 0, 0, 1, 1, 1])
        self.assertEqual(j["jobs"]["noufnouf"]["consecutive_failures"], 0)
        self.assertEqual(j["jobs"]["noufnouf"]["cumulative_failures"], [1, 0, 1, 1, 0])
        self.assertEqual(len(a["consecutive"]), 1)
        self.assertEqual(len(a["cumulative"]), 0)
        self.assertIn("nafnaf", a["consecutive"])
        mock_get_failed.assert_called()

    @patch("tasks.notify.get_failed_jobs")
    def test_multiple_failures(self, mock_get_failed):
        failed_jobs = mock_get_failed.return_value
        failed_jobs.all_failures.return_value = [
            ProjectJob(MagicMock(), attrs=a) for a in [{"name": "poulidor"}, {"name": "virenque"}, {"name": "bardet"}]
        ]
        j = {
            "jobs": {
                "poulidor": {
                    "consecutive_failures": 8,
                    "cumulative_failures": [0, 0, 1, 1, 1, 1, 1, 1, 1, 1],
                },
                "virenque": {
                    "consecutive_failures": 2,
                    "cumulative_failures": [0, 0, 0, 0, 1, 0, 1, 0, 1, 1],
                },
                "bardet": {"consecutive_failures": 2, "cumulative_failures": [1, 1]},
            }
        }
        a, j = notify.update_statistics(j)
        self.assertEqual(j["jobs"]["poulidor"]["consecutive_failures"], 9)
        self.assertEqual(j["jobs"]["virenque"]["consecutive_failures"], 3)
        self.assertEqual(j["jobs"]["bardet"]["consecutive_failures"], 3)
        self.assertEqual(len(a["consecutive"]), 2)
        self.assertEqual(len(a["cumulative"]), 1)
        self.assertIn("virenque", a["consecutive"])
        self.assertIn("bardet", a["consecutive"])
        self.assertIn("virenque", a["cumulative"])
        mock_get_failed.assert_called()


class TestSendNotification(unittest.TestCase):
    @patch("tasks.notify.send_slack_message")
    def test_consecutive(self, mock_slack):
        alert_jobs = {"consecutive": ["foo"], "cumulative": []}
        notify.send_notification(alert_jobs)
        mock_slack.assert_called_with(
            "#agent-platform-ops",
            f"Job(s) `foo` failed {notify.CONSECUTIVE_THRESHOLD} times in a row.\n",
        )

    @patch("tasks.notify.send_slack_message")
    def test_cumulative(self, mock_slack):
        alert_jobs = {"consecutive": [], "cumulative": ["bar", "baz"]}
        notify.send_notification(alert_jobs)
        mock_slack.assert_called_with(
            "#agent-platform-ops",
            f"Job(s) `bar`, `baz` failed {notify.CUMULATIVE_THRESHOLD} times in last {notify.CUMULATIVE_LENGTH} executions.\n",
        )

    @patch("tasks.notify.send_slack_message")
    def test_both(self, mock_slack):
        alert_jobs = {"consecutive": ["foo"], "cumulative": ["bar", "baz"]}
        notify.send_notification(alert_jobs)
        mock_slack.assert_called_with(
            "#agent-platform-ops",
            f"Job(s) `foo` failed {notify.CONSECUTIVE_THRESHOLD} times in a row.\nJob(s) `bar`, `baz` failed {notify.CUMULATIVE_THRESHOLD} times in last {notify.CUMULATIVE_LENGTH} executions.\n",
        )

    @patch("tasks.notify.send_slack_message")
    def test_none(self, mock_slack):
        alert_jobs = {"consecutive": [], "cumulative": []}
        notify.send_notification(alert_jobs)
        mock_slack.assert_not_called()
